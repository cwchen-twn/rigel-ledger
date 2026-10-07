import assert from 'node:assert/strict';
import { afterEach, describe, test } from 'node:test';
import { firstradeDay, ORIGIN, syncFirstrade } from '../src/connectors/us/firstrade.ts';
import { type Ask, type State, type SyncContext, SyncError } from '../src/connectors/types.ts';
import { parseExact } from '../src/util/json.ts';
import { base32, totpAt } from '../src/util/totp.ts';
import { EMAIL_CODE, type FakeFirstrade, installFakeFirstrade, PIN, SECRET } from './fake-firstrade.ts';

test('JSON numbers stay the text they were sent as', () => {
  assert.deepEqual(parseExact('{"a":1000000.07,"b":[-0.37,0.1,12e3],"c":"9.99 \\"x\\" 1","d":true,"e":null}'), {
    a: '1000000.07', b: ['-0.37', '0.1', '12e3'], c: '9.99 "x" 1', d: true, e: null,
  });
  assert.throws(() => parseExact('{"a":}'));
});

test('TOTP matches RFC 6238 (SHA-1)', () => {
  const key = base32('GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ'); // "12345678901234567890"
  assert.equal(key.toString(), '12345678901234567890');
  assert.equal(totpAt(key, 59_000, 8), '94287082');
  assert.equal(totpAt(key, 1_111_111_109_000, 8), '07081804');
  assert.equal(totpAt(key, 1_234_567_890_000, 8), '89005924');
  assert.equal(totpAt(key, 2_000_000_000_000, 8), '69279037');
  assert.equal(totpAt(key, 59_000), '287082');
});

test('Firstrade dates', () => {
  assert.equal(firstradeDay('2026-09-15'), '2026-09-15');
  assert.equal(firstradeDay('2026-09-15T00:00:00'), '2026-09-15');
  assert.equal(firstradeDay('9/2/2026'), '2026-09-02');
  assert.equal(firstradeDay('20260902'), '2026-09-02');
  assert.equal(firstradeDay(''), undefined);
});

describe('us-firstrade against a pretend app API', () => {
  let api: FakeFirstrade;
  afterEach(() => api.restore());
  const NOW = new Date('2026-10-07T15:00:00Z');

  const run = (credentials: Record<string, string>, state: State, answers: string[] = []) => {
    const asked: Ask[] = [];
    const saved: State[] = [];
    const logged: Array<Record<string, unknown> | undefined> = [];
    const ctx: SyncContext = {
      credentials, state,
      saveState: async (s) => void saved.push(s),
      ask: async (q) => {
        asked.push(q);
        const a = answers.shift();
        if (a === undefined) throw new Error('asked more than expected');
        return a;
      },
      log: { info: (_m, f) => void logged.push(f), warn: (_m, f) => void logged.push(f) },
      signal: AbortSignal.timeout(30_000),
    };
    return { asked, saved, logged, batch: syncFirstrade(ctx, ORIGIN, NOW) };
  };
  const creds = { username: 'someone', password: 'pw' };

  test('an authenticator secret answers by itself; the batch is exact', async () => {
    api = installFakeFirstrade('app');
    const r = run({ ...creds, mfa_secret: SECRET }, {});
    const batch = await r.batch;
    assert.equal(r.asked.length, 0);
    assert.deepEqual(batch.accounts.map((a) => `${a.id} ${a.kind ?? 'cash'} ${a.currency}`), ['fr-1234 brokerage USD', 'fr-1234-cash cash USD']);
    const by = (kind: string) => batch.rows.filter((x) => x.kind === kind);
    assert.deepEqual(by('balance').map((x) => [x.account, x.date, x.amount]), [['fr-1234-cash', '2026-10-07', '1000478.5']]);
    assert.deepEqual(by('holding').map((x) => [x.security, x.units, x.price, x.security_name]), [
      ['US:AAPL', '10', '39.63', 'Apple Inc.'], ['US:VTI', '0.5', '251.2', 'Vanguard Total Stock Market ETF'],
    ]);
    // Trades on the brokerage account with their cash; the option and the split are skipped.
    assert.deepEqual(by('trade').map((x) => [x.date, x.security, x.units, x.cash, x.price]).sort(), [
      ['2025-03-03', 'US:VTI', '0.5', '-125.05', '250.1'],
      ['2026-09-15', 'US:AAPL', '15', '-597', '39.8'],
      ['2026-09-22', 'US:AAPL', '-5', '205.33', '41.07'],
    ]);
    assert.deepEqual(by('transaction').map((x) => [x.account, x.date, x.amount, x.description]).sort(), [
      ['fr-1234-cash', '2026-09-01', '1000000.07', 'ACH DEPOSIT'],
      ['fr-1234-cash', '2026-09-25', '-0.37', 'NON-RESIDENT ALIEN TAX'],
      ['fr-1234-cash', '2026-09-25', '1.23', 'APPLE INC CASH DIV'],
    ]);
    assert.equal(new Set(batch.rows.map((x) => x.id)).size, batch.rows.length);
    // The first run walked back until two years were empty.
    assert.deepEqual(api.ranges, ['2026-01-01..2026-10-07', '2025-01-01..2025-12-31', '2024-01-01..2024-12-31', '2023-01-01..2023-12-31']);
    // Field names are logged, never values.
    const fields = r.logged.filter((f) => f?.endpoint).map((f) => f!.endpoint);
    assert.deepEqual(fields, ['acct_list', 'balances', 'positions', 'account_history']);
    assert.ok(!JSON.stringify(r.logged).includes('1000478'));
    assert.ok(r.logged.some((f) => JSON.stringify(f).includes('Split')));

    // The remembered session asks nothing and no second factor; history from a week back.
    const state = r.saved.at(-1)!;
    assert.equal(state.ftat, 'ftat-remembered');
    api.calls.length = 0;
    api.ranges.length = 0;
    const again = run({ ...creds }, state);
    const b2 = await again.batch;
    assert.equal(again.asked.length, 0);
    assert.ok(!api.calls.includes('/sess/verify_pin'));
    assert.deepEqual(api.ranges, ['2026-09-30..2026-10-07']);
    assert.deepEqual(b2.rows.filter((x) => x.kind === 'trade').map((x) => x.id), []);
  });

  test('an app code is asked when no secret is given', async () => {
    api = installFakeFirstrade('app');
    const { totp } = await import('../src/util/totp.ts');
    const r = run(creds, {}, ['000000', totp(SECRET)]);
    await r.batch;
    assert.equal(r.asked.length, 2);
    assert.match(r.asked[0].prompt, /authenticator/);
    assert.match(r.asked[1].prompt, /not accepted/);
  });

  test('an email code, and a PIN', async () => {
    api = installFakeFirstrade('email');
    const r = run(creds, {}, [EMAIL_CODE]);
    await r.batch;
    assert.match(r.asked[0].prompt, /email to a\*\*\*\*@example\.com/);
    api.restore();
    api = installFakeFirstrade('pin');
    const p = run({ ...creds, pin: PIN }, {});
    await p.batch;
    assert.equal(p.asked.length, 0);
  });

  test('a wrong password or PIN is bad_credentials; three wrong codes bad_otp', async () => {
    api = installFakeFirstrade('pin');
    await assert.rejects(run({ ...creds, password: 'wrong' }, {}).batch, (e: SyncError) => e.code === 'bad_credentials');
    await assert.rejects(run({ ...creds, pin: '0000' }, {}).batch, (e: SyncError) => e.code === 'bad_credentials');
    api.restore();
    api = installFakeFirstrade('email');
    await assert.rejects(run(creds, {}, ['1', '2', '3']).batch, (e: SyncError) => e.code === 'bad_otp');
  });
});
