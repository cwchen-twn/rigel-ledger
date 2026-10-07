import assert from 'node:assert/strict';
import { test } from 'node:test';
import { kudeRow, readKude } from '../src/statements/kude.ts';

// An invented KuDE: seller RUC 80012345-6, invoice 001-002-0001234 of 2026-09-15.
const CDC = '01800123456001002000123412026091511234567893';
const hex = (s: string) => [...s].map((c) => c.charCodeAt(0).toString(16).padStart(2, '0')).join('');
const qr = (total: string, date = '2026-09-15T18:20:05') =>
  `https://ekuatia.set.gov.py/consultas/qr?nVersion=150&Id=${CDC}&dFeEmiDE=${hex(date)}&dRucRec=1234567&dTotGralOpe=${total}&dTotIVA=0&cItems=3&DigestValue=00&IdCSC=1&cHashQR=00`;

test('a KuDE QR is the invoice: CDC, seller, number, date, total', () => {
  const k = readKude(qr('185000'));
  assert.deepEqual(k, { cdc: CDC, ruc: '80012345-6', number: '001-002-0001234', date: '2026-09-15', total: '185000', items: 3 });
  assert.deepEqual(kudeRow(k!, 'receipt'), {
    kind: 'invoice', account: 'kude', id: CDC, date: '2026-09-15', amount: '-185000', currency: 'PYG',
    description: 'Factura 001-002-0001234', counterparty: 'RUC 80012345-6', reference: CDC,
    items: [{ description: 'Factura 001-002-0001234 (3 items)', amount: '185000' }], file: 'receipt',
  });
});

test('guaraníes written with dots between thousands are still guaraníes', () => {
  assert.equal(readKude(qr('185.000'))?.total, '185000');
  assert.equal(readKude(qr('1.185.000'))?.total, '1185000');
  assert.equal(readKude(qr('185000.00'))?.total, '185000');
  // A real fraction is not guaraníes: refused rather than guessed.
  assert.equal(readKude(qr('185.5')), null);
  assert.equal(readKude(qr('18.50')), null);
});

test('the date comes from the CDC when dFeEmiDE is not one', () => {
  assert.equal(readKude(qr('1000', 'garbage'))?.date, '2026-09-15');
});

test('anything else is not a KuDE', () => {
  assert.equal(readKude('https://example.com/qr?Id=' + CDC + '&dTotGralOpe=1'), null);
  assert.equal(readKude('not a url'), null);
  assert.equal(readKude(qr('1').replace(CDC, '123')), null);
});
