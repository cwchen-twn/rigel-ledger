// Per-sender parsers against invented emails shaped like the real ones:
// every name, number and amount here is made up.
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { evidenceOf, toBatch } from '../src/connectors/xx/mail.ts';
import { type Mail, parseMail } from '../src/mail/parse.ts';

const mail = (from: string, subject: string, body: { html?: string; text?: string }, fromName = ''): Mail => ({
  messageId: `<${subject.length}@example>`, from, fromName, subject, day: '2026-09-04', ...body,
});

test('Hetzner: the amount to be debited, $ for a US account', () => {
  const [p] = parseMail(mail('noreply.billing@hetzner.com', 'Hetzner Online GmbH - Invoice 000012345678 (K0000000001)', {
    text: 'Dear Customer,\nEnclosed you will find your current invoice 000012345678 in PDF format.\nThe open invoice amount of $ 12.34 will soon be debited from your credit card.',
  }));
  assert.deepEqual(p, { kind: 'order', day: '2026-09-04', seller: 'Hetzner Online GmbH', number: '000012345678', total: '12.34', currency: 'USD',
    items: [{ description: 'Invoice 000012345678', amount: '12.34' }] });
  const [eur] = parseMail(mail('noreply.billing@hetzner.com', 'Invoice 1', { text: 'The open invoice amount of € 5.00 will soon be debited' }));
  assert.equal(eur.kind === 'order' && eur.currency, 'EUR');
});

test('DigitalOcean: the invoice total in USD', () => {
  const [p] = parseMail(mail('support@digitalocean.com', '[DigitalOcean] Your 2026-08 invoice is available', {
    text: 'Usage charges for 2026-08: $7.50\n━━━\nInvoice Total: $7.50\nAmount paid: -$7.50',
  }));
  assert.deepEqual(p.kind === 'order' && [p.seller, p.number, p.total, p.currency], ['DigitalOcean', '2026-08', '7.5', 'USD']);
});

test('Apple: the total, and the 電子發票 number it names', () => {
  const [p] = parseMail(mail('no_reply@email.apple.com', 'Your Invoice Notification from Apple', {
    html: '<div>INVOICE DATE 1 Sep 2026</div><div>ORDER ID MABCDE1234</div><div>INVOICE NUMBER ZZ12345678</div>'
      + '<div>App Store</div><div>Puzzle Game Gem Pack</div><div>In-App Purchase</div><div>My Phone</div><div>Report a Problem</div>'
      + '<div>NT$ 90</div><div>TOTAL</div><div>NT$ 90</div>',
  }));
  assert.deepEqual(p, { kind: 'order', day: '2026-09-04', seller: 'Apple', number: 'MABCDE1234', total: '90', currency: 'TWD', reference: 'ZZ12345678',
    items: [{ description: 'Puzzle Game Gem Pack', amount: '90' }] });
});

test('Gotogate: the total from HTML in the text part', () => {
  const [p] = parseMail(mail('noreply.us@gotogate.com', 'Your trip is confirmed. Order number 9999-000-111', {
    text: '<table><tr><td><div style="font-weight:bold">Total</div></td><td><div> 2,345.60 USD </div></td></tr></table>',
  }));
  assert.deepEqual(p.kind === 'order' && [p.number, p.total, p.currency], ['9999-000-111', '2345.6', 'USD']);
});

test('遠傳: the amount due, last period included', () => {
  const [p] = parseMail(mail('service@mebill-mail.fetnet.net', '115年8月綜合email帳單(08/25)', {
    html: '<p>上期應繳 $100 - 已繳金額 $100 + 本期金額 $455 = 應繳金額 $455</p><p>繳費方式：信用卡自動轉帳</p>',
  }));
  assert.deepEqual(p.kind === 'order' && [p.seller, p.number, p.total, p.currency], ['遠傳電信', '115年8月', '455', 'TWD']);
});

test('a 電子發票 notice and a Trip.com order state no amount: evidence', () => {
  const [n] = parseMail(mail('b2ceci@ecimail1.tradevan.com.tw', 'Music Co電子發票開立通知', {
    html: '<p>電子發票已開立 Invoice has been issued</p><p>Music Co</p><p>發票號碼 Invoice Number</p><p>QQ00000001</p><p>開立時間 2026/09/04</p>',
  }, 'Music Co'));
  assert.deepEqual(n, { kind: 'order', day: '2026-09-04', seller: 'Music Co', number: 'QQ00000001', currency: 'TWD', reference: 'QQ00000001', items: [] });
  const [t] = parseMail(mail('hk_ant_noreply@trip.com', '您的 eSIM 訂單已確認', { html: '<p>訂單編號： 1234567890123456</p><p>eSIM 2天</p>' }));
  assert.deepEqual(t.kind === 'order' && [t.seller, t.number, t.total, t.items], ['Trip.com', '1234567890123456', undefined, []]);

  const batch = toBatch([{ mail: mail('x@trip.com', 's', {}), parsed: [n, t], file: 'mail-1' }], [], '2026-09-05');
  assert.deepEqual(batch.rows.map((r) => [r.id, r.amount, r.items, r.reference]), [
    ['order:music-co:QQ00000001', '0', [], 'QQ00000001'],
    ['order:trip-com:1234567890123456', '0', [], undefined],
  ]);
});

test('a sender that is not known, or says nothing usable, is left alone', () => {
  assert.deepEqual(parseMail(mail('noreply.billing@hetzner.com', 'Your server is ready', { text: 'Welcome' })), []);
  assert.deepEqual(parseMail(mail('promo@trip.com', 'Deals', { html: '<p>Sale</p>' })), []);
});

test('evidence: the attached PDF named like a receipt, never an inline logo', () => {
  const pdf = (s: string) => Buffer.from(`%PDF-1.4 ${s}`);
  const gif = Buffer.from('GIF89a');
  const jpeg = Buffer.from([0xff, 0xd8, 0xff, 0xe0, 1, 2]);
  assert.equal(evidenceOf([
    { filename: '訂單確認_1.pdf', content: pdf('a'), contentDisposition: 'attachment' },
    { filename: '電子收據.pdf', content: pdf('b'), contentDisposition: 'attachment' },
  ])?.filename, '電子收據.pdf');
  assert.equal(evidenceOf([{ filename: 'x.bin', content: pdf('c'), contentDisposition: 'attachment' }])?.filename, 'x.bin', 'a PDF sent as octet-stream');
  assert.equal(evidenceOf([{ filename: 'logo.gif', content: gif, contentDisposition: 'inline', related: true }]), undefined);
  assert.equal(evidenceOf([{ filename: 'photo.jpg', content: jpeg, contentDisposition: 'attachment' }])?.filename, 'photo.jpg');
  assert.equal(evidenceOf([{ filename: 'smime.p7s', content: Buffer.from('0\x82'), contentDisposition: 'attachment' }]), undefined);
});
