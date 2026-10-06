// Banco Continental statements (#43) as pdf.js lines, invented: every
// number, name and amount here is made up.
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { gs, readContinentalAccount, readContinentalCard } from '../src/statements/continental.ts';
import { readStatementLines, StatementUnbalanced } from '../src/statements/index.ts';

test('Paraguayan figures: a dot between thousands, a comma before decimals', () => {
  assert.equal(gs('2.658.678'), '2658678');
  assert.equal(gs('0,0'), '0');
  assert.equal(gs('18,20'), '18.2');
  assert.equal(gs('102.000 CR'), '102000');
  assert.equal(gs('SI'), undefined);
});

const account = (rows: string[][], contable = '1.230', totales = ['600', '1.830']) => [
  ['Banco Continental S.A.E.C.A.'], ['Movimientos de Cuenta'],
  ['Cuenta:', '100200300400', 'DOE, JANE'], ['Tipo:', 'CTA.CTE-U$'],
  ['Desde el', '15/01/2026', 'hasta el', '14/02/2026'],
  ['Total hoy', '0,0', 'Contable', contable], ['Mas de 48 Horas', '0,0', 'Saldo Anterior', '0,0'],
  ['Dia', 'Hora', 'Movimiento', 'Descripción', 'Debe', 'Haber', 'Saldo'],
  ...rows,
  ['Totales', ...totales],
];

test('an account: the running Saldo says which way each row went; months roll over', () => {
  const st = readContinentalAccount(account([
    ['20 10:00:00', '56 111 25-CM-WEB', 'ALICE | 999', '1.830', '1.830'],
    ['31 09:00:00', '0 777 97--WEB', 'DEBITO X OPERAC.CAMBIOS', '500', '1.330'],
    ['02 08:00:00', '47 222 24-CM-SDJ', 'Comision', '100', '1.230'],
  ]));
  assert.ok(st);
  assert.deepEqual(st.account, { id: 'py-continental-100200300400', label: 'Continental CTA.CTE-U$ ***0400', currency: 'USD' });
  assert.deepEqual(st.rows.map((r) => [r.kind, r.date, r.amount, r.description, r.counterparty, r.reference]), [
    ['transaction', '2026-01-20', '1830', 'ALICE', '999', '56-111'],
    ['transaction', '2026-01-31', '-500', 'DEBITO X OPERAC.CAMBIOS', undefined, '0-777'],
    ['transaction', '2026-02-02', '-100', 'Comision', undefined, '47-222'],
    ['balance', '2026-02-14', '1230', undefined, undefined, undefined],
  ]);
});

test('the two sides of an exchange, on two accounts, have different ids', () => {
  const usd = readContinentalAccount(account([['04 21:44:13', '0 2145 97--WEB', 'DEBITO X OPERAC.CAMBIOS', '1.830', '1.830']], '1.830', ['0', '1.830']))!;
  const pyg = readContinentalAccount(account([['04 21:44:13', '0 2145 97--WEB', 'CREDITO X OPERAC.CAMBIOS', '1.830', '1.830']], '1.830', ['0', '1.830'])
    .map((l) => (l[0] === 'Cuenta:' ? ['Cuenta:', '555666777888', 'DOE'] : l)))!;
  assert.notEqual(usd.rows[0].id, pyg.rows[0].id);
  assert.equal(usd.rows[0].reference, pyg.rows[0].reference);
});

test('an account whose rows do not reach the Contable, or a stranger PDF', () => {
  assert.throws(() => readContinentalAccount(account([['20 10:00:00', '56 111 25-CM-WEB', 'X', '1.830', '1.831']])), /does not lead/);
  assert.throws(() => readStatementLines(account([['20 10:00:00', '56 111 25-CM-WEB', 'X', '1.830', '1.830']], '1.830', ['5', '1.830'])), StatementUnbalanced);
  assert.equal(readStatementLines([['Some other bank']]), null);
});

const card = (extra: string[][] = []) => [
  ['DOE , JANE', '020-00-000000/1'],
  ['TARJETA', '1234 5678 9012 3456', 'VISA', 'GOLD'],
  ['FECHAS DE:', 'CIERRE', 'VENCIMIENTO', 'RESUMEN ESTADO FINANCIERO'],
  ['Actual', '25/02/26', '05/03/26', 'Deuda Anterior'], ['MARZO/2026', '500.000'],
  ['D', 'L', 'Anterior', '25/01/26', '05/02/26', '(-) Pagos'], ['500.000'],
  ['4', '5', '(+) Compra y cargos del mes'], ['1.100.000'],
  ['11', '12', '(=) DEUDA TOTAL DEL PERIODO'], ['1.100.000'],
  ['FEC. OPERACIÓN', 'FEC. PROCESO', 'NRO. CUPÓN', 'DETALLE', 'FIN', 'IVA', 'MONTO'],
  ['01/02/26', '02/02/26', '600000000001', 'SUPER', 'MERCADO', 'ASUPY', 'SI', '900.000'],
  ['02/02/26', '02/02/26', '30000001', 'CONTIDESCUENTOS', '100.000 CR'],
  ['03/02/26', '04/02/26', '600000000002', 'Streaming', 'SI'],
  ['60.000', 'LIQ.', 'MARCA', 'U$:', '10.00', 'COTIZ.:', '6100.00', '61.000'],
  ['05/02/26', '05/02/26', '0', 'SU', 'PAGO', 'GRACIAS', '500.000 CR'],
  ['25/02/26', '25/02/26', '0', 'MANTENIM.', 'MENSUAL', 'NO', '10%', '19.000'],
  ['26/02/26', '25/02/26', '0', 'SEGURO', 'NO', '10%', '20.000'],
  ['Compras no Financiables +', 'Gastos Financieros +', '%S/Compra Financiable', 'Importe en Mora =', 'Pago mínimo'],
  ['39.000', '200.000', '0', '0', '100.000'],
  ...extra,
];

test('a card: purchases, credits, a charge abroad, financial charges, and the debt asserted', () => {
  const st = readContinentalCard(card());
  assert.ok(st);
  assert.equal(st.account.id, 'py-continental-card-3456');
  assert.deepEqual(st.rows.map((r) => [r.kind, r.date, r.amount, r.description]), [
    ['transaction', '2026-02-01', '-900000', 'SUPER MERCADO ASUPY'],
    ['transaction', '2026-02-02', '100000', 'CONTIDESCUENTOS'],
    ['transaction', '2026-02-03', '-61000', 'Streaming (U$ 10.00 @ 6100.00)'],
    ['transaction', '2026-02-05', '500000', 'SU PAGO GRACIAS'],
    ['transaction', '2026-02-25', '-19000', 'MANTENIM. MENSUAL'],
    ['transaction', '2026-02-26', '-20000', 'SEGURO'],
    ['transaction', '2026-02-25', '-200000', 'Gastos financieros'],
    ['balance', '2026-02-26', '-1100000', undefined],
  ]);
});

test('a card whose lines do not add up to its summary sends nothing', () => {
  const broken = card().map((l) => (l[0] === '01/02/26' ? [...l.slice(0, -1), '900.001'] : l));
  assert.throws(() => readContinentalCard(broken), /do not add up/);
});
