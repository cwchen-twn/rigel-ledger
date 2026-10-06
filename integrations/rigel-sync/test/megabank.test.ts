// tw-megabank's sign-in loop and rows, against a pretend upstream API: the
// app protocol itself is upstream's (and tested there). Every number is made up.
import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  type MegabankConfig,
  type MegabankDevice,
  MegabankOtpInvalidError,
  MegabankOtpRequiredError,
  type MegabankResult,
  MegabankVerificationRequiredError,
} from '../vendor/all-set-tw/megabank.js';
import { type MegabankApi, makeMegabank, toBatch } from '../src/connectors/tw/megabank.ts';
import type { Ask, State, SyncContext } from '../src/connectors/types.ts';
import { SyncError } from '../src/connectors/types.ts';

const DEVICE: MegabankDevice = { deviceCode: 'dev-1', deviceUKey: 'key-1', deviceSeed: 'seed-1' };
const RESULT: MegabankResult = {
  bankAccounts: [
    { sourceId: 'bank:megabank:1234:abcdef0123:TWD', accountName: '活期儲蓄存款', accountType: 'savings', currency: 'TWD' },
    { sourceId: 'bank:megabank:5678:9876543210:USD', accountName: '外幣活存', accountType: 'savings', currency: 'USD' },
    { sourceId: 'megabank:credit:TWD', accountType: 'credit', currency: 'TWD' },
  ],
  bankBalanceSnapshots: [
    { accountId: 'bank:megabank:1234:abcdef0123:TWD', sourceId: 's1', balance: 50123, currency: 'TWD', asOfAt: '2026-10-06T01:00:00Z' },
    { accountId: 'bank:megabank:5678:9876543210:USD', sourceId: 's2', balance: 100.25, currency: 'USD', asOfAt: '2026-10-06T01:00:00Z' },
  ],
  bankTransactions: [
    { accountId: 'bank:megabank:1234:abcdef0123:TWD', sourceId: 'megabank:deposit:tx:aa:0', postedDate: '2026-10-01', authorizedAt: '2026-10-01', amount: -1200, currency: 'TWD', description: '電費', status: 'posted' },
    { accountId: 'megabank:credit:TWD', sourceId: 'megabank:card:tx:bb:0', authorizedAt: '2026-10-04', amount: -350, currency: 'TWD', description: '全聯', status: 'pending' },
    { accountId: 'megabank:credit:TWD', sourceId: 'megabank:card:tx:cc:0', authorizedAt: '2026-09-28', postedDate: '2026-09-30', amount: 500, currency: 'TWD', description: '退貨', status: 'posted' },
  ],
};

test('rows: deposits with balances, a USD account apart, the card without one, a pending charge', () => {
  const b = toBatch(RESULT, '2026-10-06');
  assert.deepEqual(b.accounts.map((a) => [a.id, a.label, a.currency]), [
    ['deposit-1234', '活期儲蓄存款 ***1234', 'TWD'],
    ['deposit-5678-USD', '外幣活存 USD ***5678', 'USD'],
    ['card', '兆豐信用卡', 'TWD'],
  ]);
  assert.deepEqual(b.rows.map((r) => [r.kind, r.account, r.date, r.amount, r.pending ?? false]), [
    ['balance', 'deposit-1234', '2026-10-06', '50123', false],
    ['balance', 'deposit-5678-USD', '2026-10-06', '100.25', false],
    ['transaction', 'deposit-1234', '2026-10-01', '-1200', false],
    ['transaction', 'card', '2026-10-04', '-350', true],
    ['transaction', 'card', '2026-09-28', '500', false],
  ]);
});

/** A pretend upstream: CAPTCHA "12345" is right; OTP "246810" when the bank asks for one. */
function pretend(opts: { otp?: boolean } = {}) {
  const calls: MegabankConfig[] = [];
  let n = 0;
  const api: MegabankApi = {
    prepare: async (c) => ({
      captchaImage: 'data:image/jpeg;base64,AA==', contentType: 'image/jpeg', imageBytes: new Uint8Array([0xff, 0xd8, ++n]).buffer,
      pendingSession: `session-${n}`, pendingSessionExpiresAt: new Date(Date.now() + 120_000).toISOString(),
      device: c.deviceCode ? { deviceCode: c.deviceCode, deviceUKey: c.deviceUKey!, deviceSeed: c.deviceSeed! } : DEVICE,
    }),
    sync: async (c) => {
      calls.push(c);
      if (c.otp !== undefined) {
        if (c.otp !== '246810') throw Object.assign(new MegabankOtpInvalidError('兆豐銀行簡訊驗證碼不正確，請重新輸入。'), { pendingSession: c.pendingSession, pendingSessionExpiresAt: 'x', device: DEVICE });
        return RESULT;
      }
      if (c.captcha !== '12345') throw new MegabankVerificationRequiredError('兆豐銀行圖形驗證碼錯誤，請重新取得驗證碼。');
      if (c.password === 'wrong') throw new MegabankVerificationRequiredError('兆豐銀行登入未通過，請至官方 App 確認登入資料或驗證要求。');
      if (opts.otp && !c.deviceCode) throw new Error('the device was not kept');
      if (opts.otp) throw Object.assign(new MegabankOtpRequiredError('兆豐銀行已寄出簡訊驗證碼（簡訊檢核碼 4821），請於三分鐘內輸入。'), { pendingSession: c.pendingSession, pendingSessionExpiresAt: 'x', device: DEVICE });
      return RESULT;
    },
  };
  return { api, calls };
}

function context(state: State, answers: string[], password = 'pw') {
  const asked: Ask[] = [];
  const saved: State[] = [];
  const ctx: SyncContext = {
    credentials: { id_number: 'A123456789', user_code: 'user1', password },
    state,
    saveState: async (s) => void saved.push(s),
    ask: async (q) => {
      asked.push(q);
      const a = answers.shift();
      if (a === undefined) throw new Error(`asked once too often: ${q.prompt}`);
      return a;
    },
    log: { info() {}, warn() {} },
    signal: AbortSignal.timeout(10_000),
  };
  return { ctx, asked, saved };
}

test('the CAPTCHA read here on the second image; the device kept for next time', async () => {
  const p = pretend();
  const reads = ['1234', '12345']; // unsure, then right
  const c = context({}, []);
  const batch = await makeMegabank({ api: p.api, read: async () => reads.shift() ?? '' }).sync(c.ctx);
  assert.equal(batch.rows.length, 5);
  assert.equal(c.asked.length, 0);
  assert.deepEqual(c.saved, [{ device: DEVICE }]);
  assert.equal(p.calls.length, 1);
  assert.equal(p.calls[0].pendingSession, 'session-2');
  assert.equal(p.calls[0].deviceCode, 'dev-1');
});

test('three images not read: the person reads one; a wrong one is asked again', async () => {
  const p = pretend();
  const c = context({ device: DEVICE }, ['99999', '12345']);
  await makeMegabank({ api: p.api, read: async () => '' }).sync(c.ctx);
  assert.deepEqual(c.asked.map((a) => [a.kind, /not it/.test(a.prompt), a.image?.length]), [['captcha', false, 3], ['captcha', true, 3]]);
  assert.equal(c.saved.length, 0, 'the device it had is kept as it was');
});

test('an unusual sign-in: the SMS code is asked with its check code, a wrong one asked again', async () => {
  const p = pretend({ otp: true });
  const c = context({ device: DEVICE }, ['111111', '246810']);
  const batch = await makeMegabank({ api: p.api, read: async () => '12345' }).sync(c.ctx);
  assert.equal(batch.accounts.length, 3);
  assert.deepEqual(c.asked.map((a) => a.kind), ['otp', 'otp']);
  assert.match(c.asked[0].prompt, /簡訊檢核碼 4821/);
  assert.match(c.asked[1].prompt, /not accepted/);
  assert.equal(p.calls.at(-1)?.otp, '246810');
});

test('three wrong SMS codes are bad_otp; a refused sign-in is bad_credentials', async () => {
  const p = pretend({ otp: true });
  await assert.rejects(makeMegabank({ api: p.api, read: async () => '12345' }).sync(context({ device: DEVICE }, ['1', '2', '3']).ctx),
    (e) => e instanceof SyncError && e.code === 'bad_otp');
  await assert.rejects(makeMegabank({ api: pretend().api, read: async () => '12345' }).sync(context({}, [], 'wrong').ctx),
    (e) => e instanceof SyncError && e.code === 'bad_credentials');
});
