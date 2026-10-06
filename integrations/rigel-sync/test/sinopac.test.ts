import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { readdirSync, readFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { after, before, describe, test } from 'node:test';
import { chromePath, liveSessions } from '../src/browser/cloudflare.ts';
import { taipeiDay } from '../src/connectors/rows.ts';
import { makeSinopac, toBatch } from '../src/connectors/tw/sinopac.ts';
import { type Ask, type State, type SyncContext, SyncError } from '../src/connectors/types.ts';
import { MODEL, readDigits } from '../src/ocr/captcha.ts';
import type { SinopacResult } from '../vendor/all-set-tw/sinopac.js';
import { ACCOUNT, type FakeSinopac, startFakeSinopac } from './fake-sinopac.ts';

// The model is downloaded once (pinned, sha256 checked) and kept here between runs.
process.env.RIGEL_SYNC_OCR_MODEL ||= join(tmpdir(), 'rigel-sync-test-models', MODEL.file);

const CAPTCHAS = join(import.meta.dirname, 'captcha');

test('the OCR reads real 永豐 CAPTCHAs, and what it misses comes out short', async () => {
  const misses = new Set(['125142', '677090']); // the heavy bold font
  for (const f of readdirSync(CAPTCHAS)) {
    const digits = f.slice(0, 6);
    const read = await readDigits(readFileSync(join(CAPTCHAS, f)));
    if (misses.has(digits)) assert.ok(read.length < 6, `${digits} read as ${read}`);
    else assert.equal(read, digits);
  }
});

test('toBatch keeps a currency per account, last four digits only, and pending card rows apart', () => {
  const dep = (cur: string) => `bank:sinopac:8909:4f1e2d3c4b5a69788796a5b4c3d2e1f0:${cur}`;
  const r: SinopacResult = {
    records: [],
    bankAccounts: [
      { sourceId: dep('TWD'), accountName: '末四碼 8909', accountType: 'savings', currency: 'TWD' },
      { sourceId: dep('USD'), accountName: '末四碼 8909', accountType: 'savings', currency: 'USD' },
      { sourceId: 'credit:sinopac:main', accountName: '永豐信用卡 末四碼 4321', accountType: 'credit', currency: 'TWD' },
    ],
    bankBalanceSnapshots: [
      { accountId: dep('TWD'), sourceId: 'a', balance: 52300, currency: 'TWD', asOfAt: '' },
      { accountId: dep('USD'), sourceId: 'b', balance: 1250.5, currency: 'USD', asOfAt: '' },
      { accountId: 'credit:sinopac:main', sourceId: 'c', balance: -2500, currency: 'TWD', asOfAt: '' },
    ],
    bankTransactions: [
      { accountId: dep('TWD'), sourceId: 'sinopac:deposit:tx:aa:1', authorizedAt: '2026-10-02', amount: -120, currency: 'TWD', description: 'ATM提款', status: 'posted' },
      { accountId: dep('TWD'), sourceId: 'sinopac:deposit:tx:aa:2', authorizedAt: '2026-10-02', amount: -120, currency: 'TWD', description: 'ATM提款', status: 'posted' },
      { accountId: 'credit:sinopac:main', sourceId: 'sinopac:card:tx:v2:TWD:2026-10-04:-350:4321:1', authorizedAt: '2026-10-04T18:05:00+08:00', amount: -350, currency: 'TWD', status: 'pending' },
      { accountId: 'credit:sinopac:main', sourceId: 'sinopac:card:tx:v2:TWD:2026-10-04:-350:4321:1', authorizedAt: '2026-10-04', amount: -350, currency: 'TWD', status: 'posted' },
      { accountId: 'credit:sinopac:main', sourceId: 'z', authorizedAt: '2026-10-04', amount: 0, currency: 'TWD' },
    ],
  };
  const b = toBatch(r, '2026-10-06');
  assert.deepEqual(b.accounts, [
    { id: 'deposit-8909', label: '永豐存款 ***8909', currency: 'TWD' },
    { id: 'deposit-8909-USD', label: '永豐存款 USD ***8909', currency: 'USD' },
    { id: 'card', label: '永豐信用卡 末四碼 4321', currency: 'TWD' },
  ]);
  const rows = b.rows.map(({ kind, account, date, amount, pending }) => [kind, account, date, amount, pending ?? false].join(' '));
  assert.deepEqual(b.rows.slice(0, 2).map((x) => x.id), ['deposit-8909:2026-10-06:balance', 'deposit-8909-USD:2026-10-06:balance']);
  assert.deepEqual(rows, [
    'balance deposit-8909 2026-10-06 52300 false',
    'balance deposit-8909-USD 2026-10-06 1250.5 false',
    'transaction deposit-8909 2026-10-02 -120 false',
    'transaction deposit-8909 2026-10-02 -120 false',
    'transaction card 2026-10-04 -350 true',
    'transaction card 2026-10-04 -350 false',
  ]);
  assert.equal(new Set(b.rows.map((x) => x.id)).size, b.rows.length, 'repeats, and pending then posted, keep their own ids');
  assert.deepEqual(toBatch(r, '2026-10-06').rows.map((x) => x.id), b.rows.map((x) => x.id));
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

// CI sets RIGEL_SYNC_REQUIRE_CHROME=1, so a missing browser fails there instead of skipping.
const skip = !hasChrome() && process.env.RIGEL_SYNC_REQUIRE_CHROME !== '1' && 'no Chrome or openssl here';

describe('sinopac against a pretend bank, in Chrome', { skip, timeout: 240_000 }, () => {
  let bank: FakeSinopac;
  const given = process.env.CHROME_ARGS; // CI's --no-sandbox, kept
  before(async () => {
    bank = await startFakeSinopac();
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
      signal: AbortSignal.timeout(180_000),
    };
    return { asked, saved, batch: makeSinopac({ fetch: bank.fetch }).sync(ctx) };
  };
  const creds = { id_number: 'A123456789', user_code: 'me', password: 'pw' };
  let state: State = {};

  test('the runner reads the CAPTCHA itself, taking a new image when it misses one', async () => {
    bank.images = ['noise', '866419']; // nothing read from the first, so it never reaches the bank
    const first = run(creds, {}, []);
    const batch = await first.batch;
    assert.equal(first.asked.length, 0, 'nobody was asked');
    assert.equal(bank.served, 2);
    assert.equal(bank.logins, 1);
    assert.equal(liveSessions(), 0, 'no Chrome left running');
    state = first.saved.at(-1)!;
    assert.match(String(state.sessionCookies), /MBSID/);

    assert.deepEqual(batch.accounts.map((a) => a.id), ['deposit-8909', 'deposit-8909-USD', 'card']);
    const day = taipeiDay(new Date());
    const rows = batch.rows.map(({ kind, account, date, amount, description, pending }) =>
      `${kind} ${account} ${date} ${amount}${pending ? ' pending' : ''} ${description ?? ''}`.trim());
    assert.deepEqual(rows, [
      `balance deposit-8909 ${day} 52300`,
      `balance deposit-8909-USD ${day} 1250.5`,
      'transaction deposit-8909 2026-10-01 42000 薪資',
      'transaction deposit-8909 2026-10-02 -120 ATM提款 · 轉入 ****1234',
      'transaction deposit-8909 2026-10-02 -1580 信用卡款',
      'transaction card 2026-09-28 -1580 台灣大哥大',
      'transaction card 2026-09-30 200 退貨 全聯',
      'transaction card 2026-10-04 -350 pending 全聯福利中心',
    ]);
    assert.ok(!JSON.stringify(batch).includes(ACCOUNT), 'the full account number stays here');
  });

  test('a saved session needs no sign-in, and an expired one is signed in again', async () => {
    const before = { served: bank.served, logins: bank.logins };
    const again = run(creds, state, []);
    const ids = (await again.batch).rows.map((r) => r.id);
    assert.deepEqual({ served: bank.served, logins: bank.logins }, before, 'no CAPTCHA, no sign-in');

    bank.expireSessions();
    const later = run(creds, state, []);
    assert.deepEqual((await later.batch).rows.map((r) => r.id), ids);
    assert.equal(later.asked.length, 0);
    assert.equal(bank.logins, before.logins + 1);
    assert.equal(liveSessions(), 0);
  });

  test('when the runner cannot read three images, the person reads one', async () => {
    bank.images = ['noise', 'noise', 'noise', '042710', '417931'];
    const r = run(creds, {}, ['000000', '417931']); // a wrong answer first: a new image
    await r.batch;
    assert.equal(r.asked.length, 2);
    assert.equal(r.asked[0].kind, 'captcha');
    assert.ok(r.asked[0].image && r.asked[0].image.length > 1000, 'the image goes with the question');
    assert.match(r.asked[1].prompt, /new image/);
    assert.equal(liveSessions(), 0);
  });

  test('a wrong password is bad_credentials, three wrong answers bad_captcha', async () => {
    bank.images = ['866419'];
    await assert.rejects(run({ ...creds, password: 'wrong' }, {}, []).batch, (e) => e instanceof SyncError && e.code === 'bad_credentials');
    assert.equal(liveSessions(), 0);

    bank.images = ['noise', 'noise', 'noise'];
    await assert.rejects(run(creds, {}, ['000000', '111111', '222222']).batch, (e) => e instanceof SyncError && e.code === 'bad_captcha');
    assert.equal(liveSessions(), 0);
  });
});
