import assert from 'node:assert/strict';
import { afterEach, beforeEach, describe, test } from 'node:test';
import { einvoice, invoiceDay } from '../src/connectors/tw/einvoice.ts';
import { type State, type SyncContext, SyncError } from '../src/connectors/types.ts';
import { BARCODE, type FakeEInvoice, installFakeEInvoice } from './fake-einvoice.ts';

test('an invoice\'s day is Taiwan\'s, whatever form upstream gives', () => {
  assert.equal(invoiceDay('2026-09-02'), '2026-09-02');
  assert.equal(invoiceDay('2026-09-11T16:00:00.000Z'), '2026-09-12'); // Taiwan midnight
  assert.equal(invoiceDay('nonsense'), undefined);
});

describe('tw-einvoice against a pretend e-invoice API', () => {
  let api: FakeEInvoice;
  beforeEach(() => {
    api = installFakeEInvoice();
  });
  afterEach(() => api.restore());

  const run = (credentials: Record<string, string>, state: State) => {
    const saved: State[] = [];
    const warned: Array<Record<string, unknown> | undefined> = [];
    const ctx: SyncContext = {
      credentials,
      state,
      saveState: async (s) => void saved.push(s),
      ask: async () => {
        throw new Error('the e-invoice app asks nothing');
      },
      log: { info() {}, warn: (_m, f) => void warned.push(f) },
      signal: AbortSignal.timeout(30_000),
    };
    return { saved, warned, batch: einvoice.sync(ctx) };
  };
  const creds = { mobile: '0912-345-678', password: 'pw' };

  test('the last two periods, each invoice with its lines, keyed by number and Taiwan day', async () => {
    const r = run(creds, {});
    const batch = await r.batch;
    assert.deepEqual(batch.accounts, [{ id: 'carrier', label: `手機條碼 ${BARCODE}`, currency: 'TWD' }]);
    const rows = batch.rows.map((x) => `${x.id} ${x.date} ${x.amount} ${x.counterparty} [${x.items?.map((i) => `${i.description} ${i.quantity ?? ''}x${i.unit_price ?? ''}=${i.amount}`).join('; ')}]`);
    assert.deepEqual(rows.sort(), [
      'AB12345678:2026-09-02 2026-09-02 -320 全聯福利中心 [光泉鮮乳 936ml 1x90=90; 舒潔衛生紙 12入 1x230=230]',
      // its lines failed to load: one line of its total, and a warning
      'CD00000002:2026-09-12 2026-09-12 -85 豪大雞排 [豪大雞排 x=85]',
      'EF00000003:2026-08-20 2026-08-20 -45 7-ELEVEN [美式咖啡 1x45=45]',
    ]);
    assert.ok(batch.rows.every((x) => x.kind === 'invoice' && x.account === 'carrier' && x.currency === 'TWD'));
    assert.equal(r.warned.length, 1);
    assert.equal(api.logins, 1);

    // The session is kept: the next run signs in no more.
    const state = r.saved.at(-1)!;
    const again = await run(creds, state).batch;
    assert.equal(api.logins, 1);
    assert.deepEqual(again.rows.map((x) => x.id), batch.rows.map((x) => x.id));
    assert.equal((state as { androidId?: string }).androidId?.length, 16);

    // Refused, it signs in again, once.
    api.expireSessions();
    await run(creds, state).batch;
    assert.equal(api.logins, 2);
  });

  test('a wrong password is bad_credentials', async () => {
    await assert.rejects(run({ ...creds, password: 'wrong' }, {}).batch, (e) => e instanceof SyncError && e.code === 'bad_credentials');
  });
});
