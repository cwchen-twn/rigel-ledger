import assert from 'node:assert/strict';
import { test } from 'node:test';
import { sjDay, stockTrades, syncShioaji } from '../src/connectors/tw/shioaji.ts';
import type { State, SyncContext } from '../src/connectors/types.ts';
import { parseExact } from '../src/util/json.ts';
import type { ShioajiServer } from '../src/util/shioaji.ts';

// A pretend Shioaji server: invented accounts, lots and figures, numbers as JSON numbers.
function fakeServer(): ShioajiServer & { asked: string[] } {
  const asked: string[] = [];
  const reply: Record<string, (b: Record<string, unknown>) => string> = {
    'auth/accounts': () => '[{"account_type":"F","broker_id":"F002000","account_id":"7654321","signed":true},' +
      '{"account_type":"S","broker_id":"9A9c","account_id":"0000123","signed":true},' +
      '{"account_type":"H","broker_id":"9A9c","account_id":"00000999","signed":false}]',
    'portfolio/position_unit': (b) => b.unit === 'Common'
      ? '[{"id":0,"code":"2330","direction":"Buy","quantity":1,"price":500.0,"last_price":612.0,"pnl":0.0,"yd_quantity":1,"cond":"Cash"}]'
      : '[{"id":0,"code":"2330","direction":"Buy","quantity":1000,"price":500.0,"last_price":612.0,"pnl":0.0,"yd_quantity":1000,"cond":"Cash"},' +
        '{"id":1,"code":"0050","direction":"Buy","quantity":37,"price":180.5,"last_price":181.25,"pnl":0.0,"yd_quantity":0,"cond":"Cash"},' +
        '{"id":2,"code":"2603","direction":"Buy","quantity":2000,"price":50.0,"last_price":51.0,"pnl":0.0,"yd_quantity":2000,"cond":"MarginTrading"}]',
    'portfolio/position_detail': (b) => b.detail_id === '0'
      ? '[{"date":"2026-03-02","code":"2330","quantity":1000,"price":500.0,"last_price":612.0,"dseq":"WA001","direction":"Buy","pnl":0.0,"currency":"TWD","fee":712.0}]'
      : '[{"date":"2026-10-07","code":"0050","quantity":37,"price":180.5,"last_price":181.25,"dseq":"WA002","direction":"Buy","pnl":0.0,"currency":"TWD","fee":1.0}]',
    'portfolio/profit_loss': (b) => b.account_type === 'F'
      ? (String(b.begin_date) <= '2026-09-15' && String(b.end_date) >= '2026-09-15'
        ? '[{"id":0,"code":"TXFJ6","quantity":1,"pnl":12000.0,"date":"2026-09-15","entry_price":21000.0,"cover_price":21060.0,"tax":46,"fee":100,"direction":"Buy"}]' : '[]')
      : (String(b.begin_date) <= '2026-09-20' && String(b.end_date) >= '2026-09-20'
        ? '[{"id":7,"code":"2330","quantity":500,"pnl":48317.0,"date":"2026-09-20","dseq":"WB009","price":600.0,"seqno":"1","cond":"Cash"}]' : '[]'),
    'portfolio/profit_loss_detail': () =>
      '[{"date":"2026-03-02","code":"2330","quantity":500,"dseq":"WA001","fee":356,"tax":0,"currency":"TWD","price":500.0,"cost":250356,"trade_type":"Common","cond":"Cash"}]',
    'portfolio/margin': () => '{"yesterday_balance":500445.0,"today_balance":512345.0,"deposit_withdrawal":0.0,"fee":100.0,"tax":46.0,"equity":515345.0}',
  };
  const call = async (path: string, body: Record<string, unknown> = {}) => {
    asked.push(`${path} ${JSON.stringify(body)}`);
    // The body goes through JSON as it would over HTTP; numbers in it stay numbers.
    const b = Object.fromEntries(Object.entries(body).map(([k, v]) => [k, String(v)]));
    return parseExact(reply[path](b));
  };
  return { asked, post: call, get: (p) => call(p), stop: async () => {} };
}

const ctx = (state: State) => {
  const saved: State[] = [];
  const logged: string[] = [];
  const c: SyncContext = {
    credentials: {}, state, saveState: async (s) => void saved.push(s), ask: async () => { throw new Error('nothing to ask'); },
    log: { info: (m) => void logged.push(m), warn: (m) => void logged.push(m) }, signal: AbortSignal.timeout(10_000),
  };
  return { c, saved, logged };
};

test('Shioaji dates', () => {
  assert.equal(sjDay('2026-09-02'), '2026-09-02');
  assert.equal(sjDay('2026/09/02'), '2026-09-02');
  assert.equal(sjDay('20260902'), '2026-09-02');
  assert.equal(sjDay('2026-09-02T00:00:00'), '2026-09-02');
  assert.equal(sjDay(''), undefined);
});

test('a buy is the day\'s lots, open and closed; a sale its legs\' cost plus the realised P&L', () => {
  const rows = stockTrades('s',
    [{ code: '2330', date: '2026-03-02', quantity: '1000', price: '500', fee: '712' }],
    [{ code: '2330', date: '2026-09-20', quantity: '500', pnl: '48317', legs: [{ date: '2026-03-02', quantity: '500', cost: '250356' }] }],
    '2026-10-07');
  assert.deepEqual(rows.map((r) => [r.id, r.units, r.cash]), [
    ['s:2026-03-02:2330:buy', '1500', '-751068'],
    ['s:2026-09-20:2330:sell', '-500', '298673'],
  ]);
  // Once the rest is sold too, the buy's day is the same row: same id, same total.
  const later = stockTrades('s', [],
    [{ code: '2330', date: '2026-09-20', quantity: '500', pnl: '48317', legs: [{ date: '2026-03-02', quantity: '500', cost: '250356' }] },
     { code: '2330', date: '2026-10-01', quantity: '1000', pnl: '99000', legs: [{ date: '2026-03-02', quantity: '1000', cost: '500712' }] }],
    '2026-10-07');
  assert.deepEqual(later[0], { ...rows[0] });
});

test('tw-shioaji after the close: holdings in shares, trades with cash, futures P&L and balance', async () => {
  const sj = fakeServer();
  const { c, saved, logged } = ctx({});
  const batch = await syncShioaji(c, sj, new Date('2026-10-07T08:00:00Z')); // 16:00 in Taipei
  assert.deepEqual(batch.accounts.map((a) => `${a.id} ${a.kind ?? 'cash'}`), ['sinopac-futures-4321 cash', 'sinopac-stock-0123 brokerage']);
  const of = (kind: string) => batch.rows.filter((r) => r.kind === kind);
  assert.deepEqual(of('holding').map((r) => [r.security, r.units, r.price]), [
    ['XTAI:2330', '1000', '612'], ['XTAI:0050', '37', '181.25'], ['XTAI:2603', '2000', '51'],
  ]);
  assert.deepEqual(of('trade').map((r) => [r.date, r.security, r.units, r.cash]), [
    ['2026-03-02', 'XTAI:2330', '1500', '-751068'],
    ['2026-09-20', 'XTAI:2330', '-500', '298673'],
    ['2026-10-07', 'XTAI:0050', '37', '-6679.5'],
  ]);
  assert.deepEqual(of('transaction').map((r) => [r.date, r.amount, r.description]), [
    ['2026-09-15', '12000', '期貨平倉損益 TXFJ6'], ['2026-09-15', '-100', '期貨手續費 TXFJ6'], ['2026-09-15', '-46', '期貨交易稅 TXFJ6'],
  ]);
  assert.deepEqual(of('balance').map((r) => [r.account, r.date, r.amount]), [['sinopac-futures-4321', '2026-10-07', '512345']]);
  // The margin position is held but not traded; the 複委託 account is skipped.
  assert.ok(logged.some((m) => /margin, short/.test(m)));
  assert.ok(logged.some((m) => /does not cover/.test(m)));
  assert.deepEqual(saved.at(-1), { since: { F7654321: '2026-09-30', S0000123: '2026-09-30' } });

  // The next run reads closed lots from the oldest open lot's day, not from the week before:
  // its buy day comes out whole again, so the id is resent with the same figures.
  const next = fakeServer();
  const b2 = await syncShioaji(ctx(saved.at(-1)!).c, next, new Date('2026-10-08T08:00:00Z'));
  const firstAsk = (t: string) => next.asked.find((a) => a.startsWith(`portfolio/profit_loss {"account_type":"${t}"`));
  assert.match(firstAsk('S')!, /"begin_date":"2026-03-02"/);
  assert.match(firstAsk('F')!, /"begin_date":"2026-09-30"/);
  const buy = (b: typeof batch) => b.rows.find((r) => r.id === 'sinopac-stock-0123:2026-03-02:2330:buy');
  assert.deepEqual(buy(b2), buy(batch));
});

test('tw-shioaji before the close: today waits, holdings too', async () => {
  const { c } = ctx({});
  const batch = await syncShioaji(c, fakeServer(), new Date('2026-10-07T02:00:00Z')); // 10:00 in Taipei
  assert.equal(batch.rows.filter((r) => r.kind === 'holding' || r.kind === 'balance').length, 0);
  assert.ok(!batch.rows.some((r) => r.date === '2026-10-07'));
  assert.ok(batch.rows.some((r) => r.kind === 'trade' && r.date === '2026-09-20'));
});
