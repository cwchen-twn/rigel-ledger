// tw-nextbank against a pretend 將來 web API, through the client's fetch:
// upstream's own requests and parsing, our sign-in loop and rows. Every
// number here is made up.
import assert from 'node:assert/strict';
import { mkdtempSync, readdirSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { makeNextbank } from '../src/connectors/tw/nextbank.ts';
import type { Ask, SyncContext } from '../src/connectors/types.ts';
import { SyncError } from '../src/connectors/types.ts';

const MAIN = '88800012345678';
const POCKET = '88800098765432';
const PNG = Buffer.from('89504e470d0a1a0a', 'hex').toString('base64'); // the client passes it on; the test reads it

function pretendBank(answer = 'Ab3x9') {
  const seen = { logins: 0, logouts: 0, captchas: 0 };
  const ok = (data: unknown) => new Response(JSON.stringify({ success: true, data }), { headers: { 'content-type': 'application/json' } });
  const fail = (errorCode: string) => new Response(JSON.stringify({ success: false, error: { errorCode } }), { headers: { 'content-type': 'application/json' } });
  const fetcher: typeof fetch = async (input, init) => {
    const path = new URL(String(input)).pathname;
    const body = JSON.parse(String(init?.body ?? '{}'));
    switch (path) {
      case '/ap2/open/common/v1.0/Captcha':
        seen.captchas++;
        return ok({ uuid: `u${seen.captchas}`, captchaImage: PNG });
      case '/ap2/api/v1.1/membership/CaptchaLogin':
        seen.logins++;
        if (body.captchaResult !== answer) return fail('CAPTCHA_ERROR');
        if (body.identity !== 'A123456789') return fail('LOGIN_ERROR');
        return ok({ acstkn: 'token-1' });
      case '/ap2/api/v1.0/membership/Logout':
        seen.logouts++;
        return ok(null);
      case '/ap2/api/v2.1/membership/AllInOne':
        return ok({ mainAccount: { accountId: MAIN, workingBalance: 120000, availableBalance: 118000 } });
      case '/ap1/api/v3.0/AppMainPage/PocketInfo':
        return ok({ pocketDetails: [{ depositType: 'DEPOSIT', accNo: POCKET, name: '旅遊基金', amount: 30000 }], depositTotalAmount: 30000, termDepositTotalAmount: 0 });
      case '/ap1/api/v3.0/AppMainPage/CurrentDepositDetail': {
        const day = new Date(body.endDateTime + 8 * 3600_000).toISOString().slice(0, 10);
        const thisMonth = body.endDateTime >= Date.now() - 1000;
        return ok({ trades: thisMonth ? [
          { tradeID: 't1', tradeAmount: -5000, tradeDateTime: `${day} 10:00:00`, tradeChannel: 'POCKET', detail: { txnDateTime: `${day} 10:00:00`, deductionDate: day, descript: '轉入口袋' } },
          { tradeID: 't2', tradeAmount: -250, tradeDateTime: `${day} 12:30:00`, tradeChannel: 'UNBILLED', detail: { txnDateTime: `${day} 12:30:00`, descript: '簽帳金融卡 全家' } },
        ] : [] });
      }
      case '/ap1/api/v1.0/demandDeposit/GetPocketTxDetail': {
        const day = new Date(Date.now() + 8 * 3600_000).toISOString().slice(0, 10);
        return ok({ trades: [{ tradeID: 'p1', tradeAmount: 5000, tradeDateTime: `${day} 10:00:00`, tradeChannel: 'POCKET', detail: { txnDateTime: `${day} 10:00:00`, deductionDate: day, descript: '主帳戶轉入' } }], pageInfo: { hasNext: false } });
      }
    }
    return new Response('not here', { status: 404 });
  };
  return { fetcher, seen };
}

function context(answers: string[], id = 'a123456789') {
  const asked: Ask[] = [];
  const ctx: SyncContext = {
    credentials: { id_number: id, user_code: 'walker01', password: 'Passw0rd!' },
    state: {},
    saveState: async () => {},
    ask: async (q) => {
      asked.push(q);
      const a = answers.shift();
      if (a === undefined) throw new Error('asked once too often');
      return a;
    },
    log: { info() {}, warn() {} },
    signal: AbortSignal.timeout(10_000),
  };
  return { ctx, asked };
}

test('signs in reading the CAPTCHA, sends the main account and a pocket, signs out', async () => {
  const bank = pretendBank();
  const keep = mkdtempSync(join(tmpdir(), 'captchas-'));
  process.env.RIGEL_SYNC_KEEP_CAPTCHAS = keep;
  const reads = ['Ab3', 'Ab3xQ', 'Ab3x9']; // unsure, wrong, right
  const c = context([]);
  const batch = await makeNextbank({ fetch: bank.fetcher, read: async () => reads.shift() ?? '' }).sync(c.ctx);
  delete process.env.RIGEL_SYNC_KEEP_CAPTCHAS;
  assert.equal(c.asked.length, 0);
  assert.deepEqual([bank.seen.captchas, bank.seen.logins, bank.seen.logouts], [3, 2, 1]);
  assert.deepEqual(readdirSync(keep).map((f) => f.split('-').slice(2).join('-')).sort(), ['accepted-Ab3x9.png', 'rejected-Ab3xQ.png', 'unsure-Ab3.png']);
  assert.deepEqual(batch.accounts.map((a) => [a.id.replace(/[0-9a-f]{8}$/, 'h'), a.label]), [['main', '將來銀行 主帳戶 末四碼 5678'], ['pocket-h', '將來銀行 旅遊基金']]);
  const rows = batch.rows.map((r) => `${r.kind} ${r.account.replace(/[0-9a-f]{8}$/, 'h')} ${r.amount}${r.pending ? ' pending' : ''}`);
  assert.deepEqual(rows.sort(), ['balance main 120000', 'balance pocket-h 30000', 'transaction main -250 pending', 'transaction main -5000', 'transaction pocket-h 5000'].sort());
  assert.ok(!JSON.stringify(batch).includes(MAIN) && !JSON.stringify(batch).includes(POCKET), 'no account number leaves');
});

test('three unreadable images: the person reads one; a refused login stops at once', async () => {
  const bank = pretendBank();
  const c = context(['Ab3x9']);
  await makeNextbank({ fetch: bank.fetcher, read: async () => '' }).sync(c.ctx);
  assert.deepEqual(c.asked.map((a) => a.kind), ['captcha']);

  const wrongId = pretendBank();
  await assert.rejects(makeNextbank({ fetch: wrongId.fetcher, read: async () => 'Ab3x9' }).sync(context([], 'B987654321').ctx),
    (e) => e instanceof SyncError && e.code === 'bad_credentials');
  assert.equal(wrongId.seen.logins, 1, 'a password is never retried');
});
