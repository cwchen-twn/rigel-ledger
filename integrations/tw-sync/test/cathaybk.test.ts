import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { after, before, describe, test } from 'node:test';
import { chromePath, liveSessions } from '../src/browser/cloudflare.ts';
import { cathaybk, taipeiDay, toBatch } from '../src/connectors/cathaybk.ts';
import { type Ask, type State, type SyncContext, SyncError } from '../src/connectors/types.ts';
import type { CathaybkResult } from '../vendor/all-set-tw/cathaybk.js';
import { ACCOUNT, type FakeCathay, OTP, startFakeCathay } from './fake-cathay.ts';

test('toBatch keeps the last four digits, signs card rows from the statement, and sends no card balance', () => {
  const r: CathaybkResult = {
    records: [],
    bankAccounts: [
      { sourceId: 'bank:cathaybk:012345678901', accountName: '臺幣活存', accountType: 'savings', currency: 'TWD' },
      { sourceId: 'credit:cathaybk:main', accountName: '國泰信用卡 末四碼 4321', accountType: 'credit', currency: 'TWD' },
    ],
    bankBalanceSnapshots: [
      { accountId: 'bank:cathaybk:012345678901', sourceId: 'x', balance: 40300, currency: 'TWD', asOfAt: '' },
      { accountId: 'credit:cathaybk:main', sourceId: 'y', balance: -1380, currency: 'TWD', asOfAt: '' },
    ],
    bankTransactions: [
      { accountId: 'bank:cathaybk:012345678901', sourceId: 'a', authorizedAt: '2026-09-30T12:01:00+08:00', amount: -120, currency: 'TWD', description: '7-ELEVEN' },
      { accountId: 'bank:cathaybk:012345678901', sourceId: 'b', authorizedAt: '2026-09-30T18:40:00+08:00', amount: -120, currency: 'TWD', description: '7-ELEVEN' },
      { accountId: 'bank:cathaybk:012345678901', sourceId: 'c', postedDate: '2026-10-01T00:00:00.000Z', amount: 0, currency: 'TWD' },
      // upstream signs every card row negative; the statement amount decides
      { accountId: 'credit:cathaybk:main', sourceId: 'd', authorizedAt: '2026-09-02', amount: -1580, currency: 'TWD', description: '全聯', raw: { amount: 1580 } },
      { accountId: 'credit:cathaybk:main', sourceId: 'e', authorizedAt: '2026-09-05', amount: -200, currency: 'TWD', description: '退貨', raw: { amount: -200 } },
    ],
  };
  const b = toBatch(r, '2026-10-05');
  assert.deepEqual(b.accounts, [
    { id: 'deposit-8901', label: '臺幣活存 ***8901', currency: 'TWD' },
    { id: 'card', label: '國泰信用卡 末四碼 4321', currency: 'TWD' },
  ]);
  assert.ok(!JSON.stringify(b).includes('012345678901'), 'the full account number stays here');
  const rows = b.rows.map(({ kind, account, date, amount, id }) => ({ kind, account, date, amount, n: id.split(':').at(-1) }));
  assert.deepEqual(rows, [
    { kind: 'balance', account: 'deposit-8901', date: '2026-10-05', amount: '40300', n: 'balance' },
    { kind: 'transaction', account: 'deposit-8901', date: '2026-09-30', amount: '-120', n: '1' },
    { kind: 'transaction', account: 'deposit-8901', date: '2026-09-30', amount: '-120', n: '2' },
    { kind: 'transaction', account: 'card', date: '2026-09-02', amount: '-1580', n: '1' },
    { kind: 'transaction', account: 'card', date: '2026-09-05', amount: '200', n: '1' },
  ]);
  // the same rows give the same ids, so the app finds the duplicates
  assert.deepEqual(toBatch(r, '2026-10-05').rows.map((x) => x.id), b.rows.map((x) => x.id));
  assert.equal(new Set(b.rows.map((x) => x.id)).size, b.rows.length);
});

test('taipeiDay is the date in Taiwan', () => {
  assert.equal(taipeiDay(new Date('2026-10-04T16:30:00Z')), '2026-10-05');
});

function hasChrome() {
  try {
    chromePath();
    execFileSync('openssl', ['version'], { stdio: 'ignore' });
    return true;
  } catch {
    return false;
  }
}

// CI sets TW_SYNC_REQUIRE_CHROME=1, so a missing browser fails there instead of skipping.
const skip = !hasChrome() && process.env.TW_SYNC_REQUIRE_CHROME !== '1' && 'no Chrome or openssl here';

describe('cathaybk against a pretend bank, in Chrome', { skip, timeout: 180_000 }, () => {
  let bank: FakeCathay;
  const given = process.env.CHROME_ARGS; // CI's --no-sandbox, kept
  before(async () => {
    bank = await startFakeCathay();
    process.env.CHROME_ARGS = [given, bank.chromeArgs].filter(Boolean).join(' ');
  });
  after(async () => {
    if (given === undefined) delete process.env.CHROME_ARGS;
    else process.env.CHROME_ARGS = given;
    await bank?.close();
  });

  const run = (credentials: Record<string, string>, state: State, answers: string[]) => {
    const asked: Ask[] = [];
    const saved: State[] = [];
    const ctx: SyncContext = {
      credentials,
      state,
      saveState: async (s) => void saved.push(s),
      ask: async (q) => {
        asked.push(q);
        const a = answers.shift();
        if (a === undefined) throw new Error('asked more than expected');
        return a;
      },
      log: { info() {}, warn() {} },
      signal: AbortSignal.timeout(120_000),
    };
    return { asked, saved, batch: cathaybk.sync(ctx) };
  };
  const creds = { id_number: 'A123456789', user_code: 'me', password: 'pw', otp_channel: 'sms' };

  test('the first run asks for the SMS code once, then the device is trusted', async () => {
    const first = run(creds, {}, ['000000', OTP]); // a wrong code first: it asks again
    const batch = await first.batch;
    assert.equal(first.asked.length, 2);
    assert.equal(first.asked[0].kind, 'otp');
    assert.match(first.asked[1].prompt, /not accepted/);
    assert.equal(bank.codesSent, 1);
    assert.equal(first.saved.length, 1);
    assert.match(String(first.saved[0].sessionCookies), /CUB\.eBank\.DeviceId/);
    assert.equal(liveSessions(), 0, 'no Chrome left running');

    assert.deepEqual(batch.accounts.map((a) => a.id), ['deposit-8901', 'card']);
    const rows = batch.rows.map(({ kind, account, date, amount, description }) => `${kind} ${account} ${date} ${amount} ${description ?? ''}`.trim());
    const day = taipeiDay(new Date());
    assert.deepEqual(rows, [
      `balance deposit-8901 ${day} 40300`,
      'transaction deposit-8901 2026-09-30 42000 薪資',
      'transaction deposit-8901 2026-09-30 -120 7-ELEVEN',
      'transaction deposit-8901 2026-09-30 -120 7-ELEVEN',
      'transaction deposit-8901 2026-10-01 -1580 信用卡款',
      'transaction card 2026-09-02 -1580 全聯福利中心',
      'transaction card 2026-09-05 200 退貨 全聯',
    ]);
    assert.ok(!JSON.stringify(batch).includes(ACCOUNT));

    const second = run(creds, first.saved[0], []);
    const again = await second.batch;
    assert.equal(second.asked.length, 0, 'a trusted device is asked nothing');
    assert.equal(bank.codesSent, 1);
    assert.deepEqual(again.rows.map((r) => r.id), batch.rows.map((r) => r.id));
    assert.ok(bank.loggedOut >= 2, 'each run signs out');
    assert.equal(liveSessions(), 0);
  });

  test('a wrong password is bad_credentials', async () => {
    await assert.rejects(run({ ...creds, password: 'wrong' }, {}, []).batch, (e) => e instanceof SyncError && e.code === 'bad_credentials');
    assert.equal(liveSessions(), 0);
  });

  test('three wrong codes are bad_otp, and the browser is closed', async () => {
    await assert.rejects(run(creds, {}, ['000000', '000000', '000000']).batch, (e) => e instanceof SyncError && e.code === 'bad_otp');
    assert.equal(liveSessions(), 0);
  });
});
