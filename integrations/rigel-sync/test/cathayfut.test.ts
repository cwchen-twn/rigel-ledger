// 國泰期貨 monthly statements (#42) against invented ones: every account
// number and figure here is made up.
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { readStatements } from '../src/connectors/xx/mail.ts';
import { isCathayFutMonthly, monthRows, readMonth } from '../src/statements/cathayfut.ts';
import { pdfLines } from '../src/statements/pdf.ts';

const table = (v: Record<string, string>) => Object.entries(v).map(([k, n]) => [k, n, n, '.00', '.00']);
const statement = (v: Record<string, string>) => [
  ['國泰期貨股份有限公司'], ['月對帳單'],
  ['交易人帳號： 123 4567890', '日期', '2026/03/01', '至', '2026/03/31'],
  ['交易日', '買入', '賣出', '商品明細', '成交價格', '支出', '存入'],
  ['03/10/26', '1', '202604 TIMEX', '小台指', '網', '20000.0000'],
  ['手續費', '18.00TWD'],
  ['************************************** 保證金及權利金專戶餘額 ***********'],
  ['幣別', '國內基幣', '(NTT)', '台幣國內', '(TWD)'],
  ...table(v),
];
const march = {
  月初餘額: '100,000.00', 存提: '50,000.00', 權利金收入與支出: '-1,000.00', 本月期貨平倉損益淨額: '12,345.00', 到期履約損益: '.00',
  手續費: '120.00', 期交稅: '75.00', 本月餘額: '161,150.00', 未沖銷期貨浮動損益: '-2,500.00', 權益數: '158,650.00',
  原始保證金: '50,000.00', 未沖銷買方選擇權市值: '800.00', 未沖銷賣方選擇權市值: '.00', 權益總值: '159,450.00',
};

test('a month: realised, fees, tax, transfers, unrealised reversed the next day, equity asserted', () => {
  const m = readMonth(statement(march));
  assert.ok(m);
  const { account, rows } = monthRows(m, 'f1');
  assert.deepEqual(account, { id: 'tw-cathayfut-1234567890', label: '國泰期貨 123 ***7890', currency: 'TWD' });
  assert.deepEqual(rows.map((r) => [r.kind, r.id, r.date, r.amount, r.file]), [
    ['transaction', '2026-03:realised', '2026-03-31', '11345', 'f1'],
    ['transaction', '2026-03:fees', '2026-03-31', '-120', undefined],
    ['transaction', '2026-03:tax', '2026-03-31', '-75', undefined],
    ['transaction', '2026-03:transfers', '2026-03-31', '50000', undefined],
    ['transaction', '2026-03:unrealised', '2026-03-31', '-1700', undefined],
    ['transaction', '2026-03:unrealised-reversal', '2026-04-01', '1700', undefined],
    ['balance', '2026-03:balance', '2026-03-31', '159450', undefined],
  ]);
  // The rows take the account from the opening balance to the equity stated.
  const sum = rows.filter((r) => r.kind === 'transaction' && r.date <= '2026-03-31').reduce((a, r) => a + Number(r.amount), 100000);
  assert.equal(sum, 159450);
});

test('a table that does not add up, or a page that is not a statement, sends nothing', () => {
  const m = readMonth(statement({ ...march, 本月餘額: '161,151.00', 權益數: '158,651.00', 權益總值: '159,451.00' }));
  assert.throws(() => monthRows(m!), /does not add up/);
  assert.equal(readMonth([['國泰期貨股份有限公司'], ['買賣報告書']]), null);
  assert.ok(isCathayFutMonthly({ from: 'e-notification@ebill1.cathayfut.com.tw', subject: '國泰期貨國內月買賣報告書' }));
  assert.ok(!isCathayFutMonthly({ from: 'e-notification@ebill1.cathayfut.com.tw', subject: '國泰期貨國內買賣報告書' }), 'the daily report is left alone');
  assert.ok(!isCathayFutMonthly({ from: 'x@notcathayfut.com.tw', subject: '月買賣報告書' }));
});

/** A one-page PDF with words at positions: [x, y, text]. */
function pdf(words: Array<[number, number, string]>): Buffer {
  const content = words.map(([x, y, t]) => `BT /F1 9 Tf ${x} ${y} Td (${t}) Tj ET`).join('\n');
  const objs = [
    '<< /Type /Catalog /Pages 2 0 R >>',
    '<< /Type /Pages /Kids [3 0 R] /Count 1 >>',
    '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>',
    '<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>',
    `<< /Length ${content.length} >>\nstream\n${content}\nendstream`,
  ];
  let out = '%PDF-1.4\n';
  const offsets: number[] = [];
  objs.forEach((o, i) => {
    offsets.push(out.length);
    out += `${i + 1} 0 obj\n${o}\nendobj\n`;
  });
  const xref = out.length;
  out += `xref\n0 ${objs.length + 1}\n0000000000 65535 f \n${offsets.map((o) => `${String(o).padStart(10, '0')} 00000 n \n`).join('')}`;
  out += `trailer\n<< /Size ${objs.length + 1} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF\n`;
  return Buffer.from(out, 'latin1');
}

test('a PDF read as lines, left to right, top to bottom', async () => {
  const lines = await pdfLines(pdf([[200, 700, 'B'], [20, 700, 'A'], [20, 680, 'next line'], [90, 680, '1,000.00']]));
  assert.deepEqual(lines, [['A', 'B'], ['next line', '1,000.00']]);
});

test('statements without the ID number, or unreadable, are left out without a figure in the log', async () => {
  const warned: Array<[string, Record<string, unknown> | undefined]> = [];
  const log = { warn: (m: string, d?: Record<string, unknown>) => void warned.push([m, d]) };
  const st = [{ uid: 3, filename: 'm.pdf', content: pdf([[20, 700, 'hello']]) }];
  assert.deepEqual(await readStatements(st, '', log), { accounts: [], rows: [], files: [] });
  assert.deepEqual(await readStatements(st, 'A123456789', log), { accounts: [], rows: [], files: [] });
  assert.deepEqual(warned.map((w) => w[0]), ['statements need the 身分證字號 field', 'a statement was not recognised']);
});
