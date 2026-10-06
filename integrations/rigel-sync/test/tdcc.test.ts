import assert from 'node:assert/strict';
import { afterEach, beforeEach, describe, test } from 'node:test';
import { taipeiDay } from '../src/connectors/rows.ts';
import { tdcc, tdccDay, tdccDecimal } from '../src/connectors/tw/tdcc.ts';
import { type Ask, type State, type SyncContext, SyncError } from '../src/connectors/types.ts';
import { ACCOUNT, EMAIL_OTP, type FakeTdcc, installFakeTdcc, SMS_OTP } from './fake-tdcc.ts';

test('集保 dates: western and ROC years, and none', () => {
  assert.equal(tdccDay('20260902'), '2026-09-02');
  assert.equal(tdccDay('1150902'), '2026-09-02'); // ROC 115
  assert.equal(tdccDay('01150713'), '2026-07-13'); // ROC, zero-padded
  assert.equal(tdccDay('2026/09/02'), '2026-09-02');
  assert.equal(tdccDay('19000101'), undefined); // 集保's "no date"
  assert.equal(tdccDay(''), undefined);
});

test('集保 numbers stay exact decimals', () => {
  assert.equal(tdccDecimal('1,000'), '1000');
  assert.equal(tdccDecimal('123.4560'), '123.456');
  assert.equal(tdccDecimal('0.10'), '0.1');
  assert.equal(tdccDecimal('-580826'), '-580826');
  assert.equal(tdccDecimal('007'), '7');
  assert.equal(tdccDecimal('1e3'), undefined);
  assert.equal(tdccDecimal(''), undefined);
});

describe('tw-tdcc against a pretend e存摺', () => {
  let api: FakeTdcc;
  beforeEach(() => {
    api = installFakeTdcc();
  });
  afterEach(() => api.restore());

  const run = (credentials: Record<string, string>, state: State, answers: string[]) => {
    const asked: Ask[] = [];
    const saved: State[] = [];
    const warned: Array<Record<string, unknown> | undefined> = [];
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
      log: { info() {}, warn: (_m, f) => void warned.push(f) },
      signal: AbortSignal.timeout(30_000),
    };
    return { asked, saved, warned, batch: tdcc.sync(ctx) };
  };
  const creds = { id_number: 'A123456789', password: 'pw' };

  test('the first run asks for the email code, a wrong one again; then the device is trusted', async () => {
    const first = run(creds, {}, ['000000', EMAIL_OTP]);
    const batch = await first.batch;
    assert.equal(first.asked.length, 2);
    assert.match(first.asked[0].prompt, /email/);
    assert.match(first.asked[1].prompt, /not accepted/);
    assert.equal(api.emails, 1, 'one email, however many tries');

    assert.deepEqual(batch.accounts, [
      { id: 'broker-9800-4567', label: '元大證券 ***4567', currency: 'TWD', kind: 'brokerage' },
      { id: 'funds-A01', label: '基金 A01', currency: 'TWD', kind: 'brokerage' },
      { id: 'bank-013-9012', label: '交割戶 013 ***9012', currency: 'TWD' },
    ]);
    const day = taipeiDay(new Date());
    const rows = batch.rows.map((r) => [r.kind, r.account, r.date, r.security, r.units ?? r.amount, r.price].filter(Boolean).join(' '));
    assert.deepEqual(rows, [
      'holding broker-9800-4567 2026-10-05 XTAI:2330 620',
      'holding funds-A01 2026-10-05 FUND:B00123 123.456',
      'trade broker-9800-4567 2026-09-20 XTAI:2330 -400 600',
      'trade broker-9800-4567 2026-09-02 XTAI:2330 1000 580',
      // 配股: units in, and its "1" is a placeholder, not a price
      'trade broker-9800-4567 2026-07-15 XTAI:2330 20',
      `balance bank-013-9012 ${day} 52300`,
      'transaction bank-013-9012 2026-09-04 -580826',
      'transaction bank-013-9012 2026-09-22 238500',
    ]);
    const fund = batch.rows.find((r) => r.security === 'FUND:B00123');
    assert.equal(fund?.security_name, '安聯台灣科技基金');
    assert.equal(fund?.quote_currency, 'TWD');
    // The unknown movement is left out, and said so.
    assert.deepEqual(first.warned.at(-1), { kinds: ['神秘異動 (X9)'] });
    assert.ok(!JSON.stringify(batch).includes(ACCOUNT), 'account numbers keep their last four digits');
    assert.ok(!JSON.stringify(batch).includes('1234567'));

    // Trusted now: no code, no email, the same rows (the history is not paged again).
    const state = first.saved.at(-1)!;
    const second = run(creds, state, []);
    const again = await second.batch;
    assert.equal(second.asked.length, 0);
    assert.equal(api.emails, 1);
    const ids = (b: typeof batch) => b.rows.filter((r) => r.kind !== 'trade').map((r) => r.id);
    assert.deepEqual(ids(again), ids(batch));
    assert.equal(again.rows.filter((r) => r.kind === 'trade').length, 0, 'the movements were read already');
  });

  test('when 集保 also wants the phone, the SMS code is asked for after the email one', async () => {
    api.smsStep = true;
    const r = run(creds, {}, [EMAIL_OTP, SMS_OTP]);
    await r.batch;
    assert.equal(r.asked.length, 2);
    assert.match(r.asked[1].prompt, /SMS/);
    assert.equal(api.emails, 1);
    assert.equal(api.sms, 1, 'the SMS was sent, not only asked for');
  });

  test('a wrong password is not taken for a wrong code, and three wrong codes are bad_otp', async () => {
    await assert.rejects(run({ ...creds, password: 'wrong' }, {}, []).batch,
      (e) => e instanceof SyncError && e.code === 'verification_failed');
    await assert.rejects(run(creds, {}, ['1', '2', '3']).batch, (e) => e instanceof SyncError && e.code === 'bad_otp');
  });

  test('the device id is kept before the first sign-in', async () => {
    const r = run(creds, {}, [EMAIL_OTP]);
    await r.batch;
    const first = JSON.parse(String(r.saved[0].cursor)) as { deviceId: string };
    const last = JSON.parse(String(r.saved.at(-1)!.cursor)) as { deviceId: string };
    assert.match(first.deviceId, /^[0-9a-f]{16}$/);
    assert.equal(last.deviceId, first.deviceId);
  });
});
