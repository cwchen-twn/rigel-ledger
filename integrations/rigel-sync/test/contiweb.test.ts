// py-continental from ContiWeb (#67): its XLS export, its card page, and the
// xlsx reader. Every number, name and amount here is made up.
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { crc32, deflateRawSync } from 'node:zlib';
import { connectors } from '../src/connectors/index.ts';
import { continentalCurrency, fechaLarga, readContinentalCardScreen, readContinentalSheet } from '../src/statements/continental.ts';
import { readXlsx } from '../src/util/xlsx.ts';

/** A zip of the given files, deflated (as ContiWeb's are) unless stored. */
function zip(files: Record<string, string>, store = false): Buffer {
  const locals: Buffer[] = [];
  const central: Buffer[] = [];
  let offset = 0;
  for (const [name, text] of Object.entries(files)) {
    const raw = Buffer.from(text);
    const data = store ? raw : deflateRawSync(raw);
    const n = Buffer.from(name);
    const head = Buffer.alloc(30);
    head.writeUInt32LE(0x04034b50, 0);
    head.writeUInt16LE(store ? 0 : 8, 8);
    head.writeUInt32LE(crc32(raw), 14);
    head.writeUInt32LE(data.length, 18);
    head.writeUInt32LE(raw.length, 22);
    head.writeUInt16LE(n.length, 26);
    const dir = Buffer.alloc(46);
    dir.writeUInt32LE(0x02014b50, 0);
    dir.writeUInt16LE(store ? 0 : 8, 10);
    dir.writeUInt32LE(crc32(raw), 16);
    dir.writeUInt32LE(data.length, 20);
    dir.writeUInt32LE(raw.length, 24);
    dir.writeUInt16LE(n.length, 28);
    dir.writeUInt32LE(offset, 42);
    locals.push(head, n, data);
    central.push(dir, n);
    offset += 30 + n.length + data.length;
  }
  const cd = Buffer.concat(central);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0);
  end.writeUInt16LE(Object.keys(files).length, 8);
  end.writeUInt16LE(Object.keys(files).length, 10);
  end.writeUInt32LE(cd.length, 12);
  end.writeUInt32LE(offset, 16);
  return Buffer.concat([...locals, cd, end]);
}

// The shape of ContiWeb's export: an x: prefix, shared strings, numbers as numbers,
// SALDO as text, dates as Excel day numbers (46297 = 2026-10-02), a TOTAL formula.
const HEAD = ['DIACONT', 'FECHA', 'MOVIMIENTO', 'DESCRIP', 'DEBE', 'HABER', 'SALDO', 'FECHAMOVI', 'FECHACONT'];
function contiweb(rows: Array<[string, string, string, string, string, string, string, number]>): Buffer {
  const strings: string[] = [];
  const s = (t: string) => {
    let i = strings.indexOf(t);
    if (i < 0) i = strings.push(t) - 1;
    return `t="s"><x:v>${i}</x:v>`;
  };
  const col = 'ABCDEFGHI';
  const cell = (c: number, r: number, body: string) => `<x:c r="${col[c]}${r}" ${body}</x:c>`;
  const lines = [HEAD.map((h, c) => cell(c, 1, s(h))).join('')];
  rows.forEach(([day, time, mov, desc, debe, haber, saldo, serial], i) => {
    const r = i + 2;
    const num = (v: string) => (v ? `t="n"><x:v>${v}</x:v>` : s(' '));
    lines.push([s(day), s(time), s(mov), s(desc), num(debe), num(haber), s(saldo), `s="3"><x:v>${serial}</x:v>`, `s="3"><x:v>${serial}</x:v>`].map((b, c) => cell(c, r, b)).join(''));
  });
  lines.push(`<x:c r="D${rows.length + 2}" ${s('TOTAL')}</x:c><x:c r="E${rows.length + 2}"><x:f>SUMIF(E9:E99,"&lt;&gt;",E9:E99)</x:f></x:c>`);
  const ns = 'xmlns:x="http://schemas.openxmlformats.org/spreadsheetml/2006/main"';
  return zip({
    'xl/sharedStrings.xml': `<?xml version="1.0"?><x:sst ${ns}>${strings.map((t) => `<x:si><x:t>${t.replace(/&/g, '&amp;')}</x:t></x:si>`).join('')}</x:sst>`,
    'xl/worksheets/sheet1.xml': `<?xml version="1.0"?><x:worksheet ${ns}><x:sheetData>${lines.map((l, i) => `<x:row r="${i + 1}">${l}</x:row>`).join('')}</x:sheetData></x:worksheet>`,
  });
}

const PYG = { number: '100200300400', label: 'Cuenta De Ahorro Gs', currency: 'PYG' as const };

test('the xlsx reader: shared, inline and numeric cells, gaps, stored or deflated', () => {
  const ns = 'xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"';
  const sheet = `<worksheet ${ns}><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>A &amp; B</t></is></c><c r="C1"><v>12.5</v></c></row></sheetData></worksheet>`;
  for (const store of [false, true]) assert.deepEqual(readXlsx(zip({ 'xl/worksheets/sheet1.xml': sheet }, store)), [['A & B', '', '12.5']]);
  assert.throws(() => readXlsx(Buffer.from('not a zip at all, not even close to one')), /not a zip/);
});

test('an account export, newest first: DEBE out, HABER in, ids as the PDF has them', () => {
  const st = readContinentalSheet(readXlsx(contiweb([
    ['07', '12:00:00', '56 333 25-VM-SYS', 'TRANS.INTERB.WEB-111', '75000', '', '856.608', 46302],
    ['07', '09:30:00', '0 4444 97-VM-WEB', 'CREDITO X OPERAC.CAMBIOS', '', '859500', '931.608', 46302],
    ['02', '08:00:00', '47 222 24-CM-WEB', 'OP.ELEC. | 999', '28000', '', '72.108', 46297],
    ['02', '08:00:00', '47 222 24-CM-WEB', 'OP.ELEC. | 999', '100', '', '100.108', 46297],
  ])), PYG);
  assert.deepEqual(st.account, { id: 'py-continental-100200300400', label: 'Continental Cuenta De Ahorro Gs ***0400', currency: 'PYG' });
  assert.deepEqual(st.rows.map((r) => [r.kind, r.id, r.date, r.amount, r.description, r.counterparty, r.reference]), [
    ['transaction', '100200300400:2026-10-02T08:00:00:47-222', '2026-10-02', '-100', 'OP.ELEC.', '999', '47-222'],
    ['transaction', '100200300400:2026-10-02T08:00:00:47-222:2', '2026-10-02', '-28000', 'OP.ELEC.', '999', '47-222'],
    ['transaction', '100200300400:2026-10-07T09:30:00:0-4444', '2026-10-07', '859500', 'CREDITO X OPERAC.CAMBIOS', undefined, '0-4444'],
    ['transaction', '100200300400:2026-10-07T12:00:00:56-333', '2026-10-07', '-75000', 'TRANS.INTERB.WEB-111', undefined, '56-333'],
    ['balance', '100200300400:2026-10-07:balance', '2026-10-07', '856608', undefined, undefined, undefined],
  ]);
});

test('an export whose saldos do not follow is refused, and an empty month is no rows', () => {
  assert.throws(() => readContinentalSheet(readXlsx(contiweb([
    ['07', '12:00:00', '56 333 25-VM-SYS', 'X', '75000', '', '856.608', 46302],
    ['02', '08:00:00', '47 222 24-CM-WEB', 'Y', '100', '', '1.000.000', 46297],
  ])), PYG), /does not lead/);
  assert.deepEqual(readContinentalSheet(readXlsx(contiweb([])), PYG).rows, []);
});

test('a dollar account: cents as numbers, the saldo with a decimal comma', () => {
  const st = readContinentalSheet(readXlsx(contiweb([
    ['03', '10:00:00', '0 4444 97-VM-WEB', 'DEBITO X OPERAC.CAMBIOS', '120.5', '', '1.379,50', 46298],
  ])), { number: '9988776655', label: 'Cuenta Corriente $', currency: 'USD' });
  assert.deepEqual(st.rows.map((r) => [r.amount, r.currency]), [['-120.5', 'USD'], ['1379.5', 'USD']]);
});

test('the card page: purchases out, credits in, the debt as the balance', () => {
  const st = readContinentalCardScreen({
    last4: '3456', debt: '2.387.335 Gs',
    items: [
      { description: 'Supermercado ejemplo', date: '4 de octubre de 2026', amount: '662.863 Gs' },
      { description: 'Supermercado ejemplo', date: '4 de octubre de 2026', amount: '662.863 Gs' },
      { description: 'Contidescuentos', date: '4 de octubre de 2026', amount: '-60.000 Gs' },
      { description: 'Su pago gracias', date: '29 de septiembre de 2026', amount: '-1.500.000 Gs' },
    ],
  }, '2026-10-07');
  assert.equal(st.account.id, 'py-continental-card-3456');
  assert.deepEqual(st.rows.map((r) => [r.kind, r.date, r.amount, r.description]), [
    ['transaction', '2026-10-04', '-662863', 'Supermercado ejemplo'],
    ['transaction', '2026-10-04', '-662863', 'Supermercado ejemplo'],
    ['transaction', '2026-10-04', '60000', 'Contidescuentos'],
    ['transaction', '2026-09-29', '1500000', 'Su pago gracias'],
    ['balance', '2026-10-07', '-2387335', undefined],
  ]);
  assert.notEqual(st.rows[0].id, st.rows[1].id); // two alike stay two
});

test('Spanish dates and account currencies', () => {
  assert.equal(fechaLarga('4 de octubre de 2026'), '2026-10-04');
  assert.equal(fechaLarga('29 de setiembre de 2026'), '2026-09-29');
  assert.equal(fechaLarga('octubre'), undefined);
  assert.equal(continentalCurrency('Cuenta De Ahorro Gs'), 'PYG');
  assert.equal(continentalCurrency('Cuenta Corriente $'), 'USD');
  assert.equal(continentalCurrency('Cuenta en Euros'), undefined);
});

test('py-continental is offered only with RIGEL_SYNC_EXPERIMENTAL=1 until it has run for real', () => {
  delete process.env.RIGEL_SYNC_EXPERIMENTAL;
  assert.ok(!connectors({ fake: false }).some((c) => c.id === 'py-continental'));
  process.env.RIGEL_SYNC_EXPERIMENTAL = '1';
  assert.ok(connectors({ fake: false }).some((c) => c.id === 'py-continental'));
  delete process.env.RIGEL_SYNC_EXPERIMENTAL;
});
