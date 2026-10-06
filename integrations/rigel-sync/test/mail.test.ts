import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import type { AddressInfo } from 'node:net';
import { after, before, describe, test } from 'node:test';
import { chromePath } from '../src/browser/cloudflare.ts';
import { type State, type SyncContext, SyncError } from '../src/connectors/types.ts';
import { makeMail } from '../src/connectors/xx/mail.ts';
import { type Fetched, type Mailbox, MailboxError } from '../src/mail/imap.ts';
import { type Mail, mailDay, type Parser, parseMail, schemaOrg } from '../src/mail/parse.ts';

test('a Date header\'s day is the sender\'s', () => {
  const now = new Date('2026-10-01T00:00:00Z');
  assert.equal(mailDay('Tue, 2 Sep 2026 23:30:00 +0800', now), '2026-09-02');
  assert.equal(mailDay('Tue, 2 Sep 2026 23:30:00 -0300', now), '2026-09-02');
  assert.equal(mailDay('Wed, 3 Sep 2026 01:00:00 +0800 (CST)', now), '2026-09-03');
  assert.equal(mailDay(undefined, now), '2026-10-01');
});

const ld = (o: unknown) => `<html><head><script type="application/ld+json">${JSON.stringify(o)}</script></head><body>Thanks</body></html>`;
const mail = (html?: string): Mail => ({ messageId: '<m1@shop>', from: 'orders@shop.example', fromName: 'Shop', subject: 'Your order', day: '2026-09-02', html });

test('schema.org: an Order with its offers, a total, quantities multiplied exactly', () => {
  const found = schemaOrg.parse(mail(ld({
    '@context': 'http://schema.org', '@type': 'Order', merchant: { '@type': 'Organization', name: 'Books Ltd' },
    orderNumber: 'B-1001', orderDate: '2026-09-01T23:10:00-04:00', priceCurrency: 'usd', price: '41.97',
    acceptedOffer: [
      { '@type': 'Offer', itemOffered: { '@type': 'Book', name: 'Accounting 101' }, price: '19.99', priceCurrency: 'USD', eligibleQuantity: { value: 2 } },
      { '@type': 'Offer', itemOffered: { name: 'Bookmark' }, price: 1.99, priceCurrency: 'USD' },
    ],
  })));
  assert.deepEqual(found, [{
    kind: 'order', seller: 'Books Ltd', number: 'B-1001', day: '2026-09-01', total: '41.97', currency: 'USD',
    items: [
      { description: 'Accounting 101', quantity: '2', unit_price: '19.99', amount: '39.98' },
      { description: 'Bookmark', quantity: '1', unit_price: '1.99', amount: '1.99' },
    ],
  }]);
});

test('schema.org: an Invoice in a @graph, its total from the lines when it states none', () => {
  const found = schemaOrg.parse(mail(ld({ '@graph': [{ '@type': 'Organization', name: 'x' }, {
    '@type': 'Invoice', provider: { name: 'Cloud Co' }, paymentDueDate: '2026-09-15',
    referencesOrder: { acceptedOffer: [{ itemOffered: { name: 'Plan' }, price: '300', priceCurrency: 'TWD' }] },
  }] })));
  assert.equal(found?.length, 1);
  assert.equal(found![0].kind, 'order');
  assert.deepEqual(found![0].kind === 'order' && [found![0].seller, found![0].day, found![0].total, found![0].currency], ['Cloud Co', '2026-09-15', '300', 'TWD']);
});

test('schema.org: nothing from mail without an order, broken markup, or no money', () => {
  assert.equal(schemaOrg.parse(mail('<p>Newsletter</p>')), null);
  assert.equal(schemaOrg.parse(mail('<script type="application/ld+json">{oops</script>')), null);
  assert.equal(schemaOrg.parse(mail(ld({ '@type': 'Order', merchant: { name: 'x' } }))), null);
  assert.equal(schemaOrg.parse(mail()), null);
  assert.deepEqual(parseMail(mail('<p>hi</p>')), []);
});

// ---- the connector, against a mailbox in memory ----

const eml = (uid: number, from: string, subject: string, date: string, html: string) =>
  Buffer.from([
    `From: ${from}`, `To: me@example.com`, `Subject: =?UTF-8?B?${Buffer.from(subject).toString('base64')}?=`, `Date: ${date}`,
    `Message-ID: <msg-${uid}@example.com>`, 'MIME-Version: 1.0', 'Content-Type: text/html; charset=utf-8', 'Content-Transfer-Encoding: base64', '',
    Buffer.from(html).toString('base64'),
  ].join('\r\n'));

const hasChrome = (() => {
  try {
    chromePath();
    return true;
  } catch {
    return false;
  }
})();

describe('xx-mail against a mailbox in memory', () => {
  let pixel: ReturnType<typeof createServer>;
  let pixelHits = 0;
  let pixelUrl = '';
  before(async () => {
    pixel = createServer((_q, r) => {
      pixelHits++;
      r.end('GIF89a');
    });
    await new Promise<void>((r) => pixel.listen(0, '127.0.0.1', r));
    pixelUrl = `http://127.0.0.1:${(pixel.address() as AddressInfo).port}/open.gif`;
  });
  after(() => new Promise<void>((r) => pixel.close(() => r())));

  const messages = (): Fetched[] => [
    { uid: 7, source: eml(7, 'Books Ltd <orders@books.example>', '訂單確認 B-1001', 'Tue, 2 Sep 2026 10:00:00 +0800', ld({
      '@type': 'Order', merchant: { name: 'Books Ltd' }, orderNumber: 'B-1001', priceCurrency: 'TWD', price: '598',
      acceptedOffer: [{ itemOffered: { name: '會計學' }, price: '299', priceCurrency: 'TWD', eligibleQuantity: { value: 2 } }],
    }).replace('Thanks', `Thanks <img src="${pixelUrl}">`)) },
    { uid: 8, source: eml(8, 'news@shop.example', 'Weekly deals', 'Wed, 3 Sep 2026 10:00:00 +0800', '<p>Deals</p>') },
    { uid: 9, source: eml(9, 'alerts@bank.example', '刷卡通知', 'Thu, 4 Sep 2026 12:30:00 +0800', '<p>card 4321 NT$350 全聯</p>') },
  ];
  // A stand-in for the card-alert parsers that will come from real samples.
  const alerts: Parser = {
    name: 'test alerts',
    parse: (m) => (m.from === 'alerts@bank.example' ? [{ kind: 'card_alert', issuer: 'tw-cathaybk', card: '4321', day: m.day, amount: '350', currency: 'TWD', merchant: '全聯' }] : null),
  };
  let opened = 0;
  const box = (validity = '1'): Mailbox => ({
    uidValidity: validity,
    async *since() {
      opened++;
      yield* messages();
    },
    close: async () => {},
  });

  const run = (state: State, open: () => Promise<Mailbox> = async () => box()) => {
    const saved: State[] = [];
    const ctx: SyncContext = {
      credentials: { address: 'me@example.com', password: 'abcd efgh ijkl mnop' },
      state,
      saveState: async (s) => void saved.push(s),
      ask: async () => {
        throw new Error('mail asks nothing');
      },
      log: { info() {}, warn() {} },
      signal: AbortSignal.timeout(60_000),
    };
    const connector = makeMail({ open, parsers: [alerts, schemaOrg], now: () => new Date('2026-09-10T00:00:00Z') });
    return { saved, batch: connector.sync(ctx) };
  };

  test('an order becomes an invoice with its PDF, an alert a pending card row; nothing else', { timeout: 120_000 }, async () => {
    const r = run({});
    const batch = await r.batch;
    assert.deepEqual(batch.accounts.map((a) => a.id).sort(), ['card-tw-cathaybk-4321', 'orders']);
    const order = batch.rows.find((x) => x.kind === 'invoice')!;
    assert.deepEqual(
      { id: order.id, date: order.date, amount: order.amount, currency: order.currency, seller: order.counterparty, items: order.items },
      { id: 'order:books-ltd:B-1001', date: '2026-09-02', amount: '-598', currency: 'TWD', seller: 'Books Ltd',
        items: [{ description: '會計學', quantity: '2', unit_price: '299', amount: '598' }] },
    );
    const alert = batch.rows.find((x) => x.kind === 'transaction')!;
    assert.deepEqual([alert.account, alert.date, alert.amount, alert.pending, alert.description], ['card-tw-cathaybk-4321', '2026-09-04', '-350', true, '全聯']);
    assert.equal(batch.rows.length, 2, 'the newsletter is left alone');
    if (hasChrome) {
      assert.equal(batch.files?.length, 1);
      assert.equal(order.file, batch.files![0].ref);
      assert.ok(Buffer.from(batch.files![0].data, 'base64').subarray(0, 5).toString() === '%PDF-');
      assert.match(batch.files![0].filename, /^2026-09-02 訂單確認 B-1001\.pdf$/);
      assert.equal(pixelHits, 0, 'printing loaded nothing from the network');
    }
    assert.deepEqual(r.saved.at(-1), { uidValidity: '1', lastUid: 9, lastDay: '2026-09-10' });

    // The next run reads the overlap again: the same rows, nothing printed twice.
    const again = await run(r.saved.at(-1)!).batch;
    assert.deepEqual(again.rows.map((x) => x.id), batch.rows.map((x) => x.id));
    assert.equal(again.files?.length ?? 0, 0);
    // A recreated folder (new uid validity) prints again.
    const fresh = await run(r.saved.at(-1)!, async () => box('2')).batch;
    assert.equal(fresh.files?.length ?? 0, hasChrome ? 1 : 0);
  });

  test('a refused login is bad_credentials, a missing label verification_failed', async () => {
    await assert.rejects(run({}, async () => { throw new MailboxError('auth', 'Invalid credentials'); }).batch,
      (e) => e instanceof SyncError && e.code === 'bad_credentials');
    await assert.rejects(run({}, async () => { throw new MailboxError('folder', 'no folder or label "rigel"'); }).batch,
      (e) => e instanceof SyncError && e.code === 'verification_failed');
    assert.ok(opened >= 0);
  });
});
