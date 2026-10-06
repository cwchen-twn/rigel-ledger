/*
 * Banco Continental (Paraguay) statements (#43), as its online banking
 * prints them to PDF: "Movimientos de Cuenta" for a USD or a PYG account,
 * and the credit card's monthly 'extracto'. Amounts are written 1.234.567,89
 * (a dot between thousands, a comma before decimals).
 *
 *   account  every row with its running Saldo, which says which way it went;
 *            the rows must walk from Saldo Anterior to Contable and match
 *            the Totales. The movement number is the row's reference: the
 *            two sides of a USD -> PYG exchange carry the same one, which
 *            pairs them (one transfer across currencies).
 *   card     purchases (< 0), credits marked CR (> 0: CONTIDESCUENTOS,
 *            payments); a charge abroad brings its U$ amount and rate on the
 *            next line; the period's Gastos Financieros (on no line, in the
 *            footer) are a row on the closing day. Deuda Anterior - Pagos +
 *            Compras = Deuda Total must hold, and the rows must sum to the
 *            Pagos and the Compras.
 *
 * Shared with the web app's PDF import: no imports but src/decimal.ts and
 * types, so it runs in a browser too.
 */
import type { Account, Row } from '../connectors/types.ts';
import { addDecimal, plainDecimal } from '../decimal.ts';

export interface ParsedStatement {
  name: string;
  account: Account;
  rows: Row[];
}

/** "2.658.678" -> 2658678, "0,0" -> 0, "18,20" -> 18.2 (and "102.000 CR" -> 102000). */
export function gs(s: string | undefined): string | undefined {
  if (s === undefined) return undefined;
  const t = s.replace(/\s*CR$/, '').trim();
  if (!/^-?[\d.]+(,\d+)?$/.test(t)) return undefined;
  return plainDecimal(t.replace(/\./g, '').replace(',', '.'));
}

const neg = (s: string) => (s === '0' ? '0' : s.startsWith('-') ? s.slice(1) : `-${s}`);
const sum = (xs: string[]) => xs.reduce(addDecimal, '0');
const dmy = (s: string) => {
  const m = /^(\d{2})\/(\d{2})\/(\d{2}|\d{4})$/.exec(s);
  return m ? `${m[3].length === 2 ? `20${m[3]}` : m[3]}-${m[2]}-${m[1]}` : undefined;
};
const after = (words: string[], label: string) => {
  const i = words.indexOf(label);
  return i >= 0 ? words[i + 1] : undefined;
};

class Unbalanced extends Error {}
export { Unbalanced as ContinentalUnbalanced };

/** A "Movimientos de Cuenta" statement, or null when the lines are not one. */
export function readContinentalAccount(lines: string[][]): ParsedStatement | null {
  if (!lines.some((l) => l.includes('Movimientos de Cuenta')) || !lines.some((l) => l[0]?.startsWith('Banco Continental'))) return null;
  let number = '';
  let type = '';
  let from = '';
  let to = '';
  let opening: string | undefined;
  let closing: string | undefined;
  let totals: [string, string] | undefined;
  const moves: Array<{ day: number; time: string; mov: string; description: string; amount: string; saldo: string }> = [];
  for (const w of lines) {
    if (w[0] === 'Cuenta:' && !number) number = w[1] ?? '';
    if (w[0] === 'Tipo:' && !type) type = w[1] ?? '';
    if (w[0] === 'Desde el' && !from) [from, to] = [dmy(w[1]) ?? '', dmy(after(w, 'hasta el') ?? '') ?? ''];
    if (opening === undefined && w.includes('Saldo Anterior')) opening = gs(after(w, 'Saldo Anterior'));
    if (closing === undefined && w.includes('Contable')) closing = gs(after(w, 'Contable'));
    if (w[0] === 'Totales') totals = [gs(w[1]) ?? '', gs(w[2]) ?? ''];
    const m = /^(\d{2}) (\d{2}:\d{2}:\d{2})$/.exec(w[0] ?? '');
    if (m && w.length >= 5) {
      const amount = gs(w[w.length - 2]);
      const saldo = gs(w[w.length - 1]);
      if (amount && saldo) moves.push({ day: Number(m[1]), time: m[2], mov: w[1], description: w.slice(2, -2).join(' '), amount, saldo });
    }
  }
  const currency = /U\$|USD/.test(type) ? 'USD' : /GS|PYG/.test(type) ? 'PYG' : '';
  if (!number || !currency || !from || !to || opening === undefined || closing === undefined || !totals) return null;

  const id = `py-continental-${number}`;
  const rows: Row[] = [];
  const seen = new Map<string, number>();
  let balance = opening;
  let [year, month] = from.split('-').map(Number);
  let lastDay = 0;
  const outs: string[] = [];
  const ins: string[] = [];
  for (const mv of moves) {
    if (mv.day < lastDay) [year, month] = month === 12 ? [year + 1, 1] : [year, month + 1];
    lastDay = mv.day;
    let signed: string;
    if (addDecimal(balance, neg(mv.amount)) === mv.saldo) {
      signed = neg(mv.amount);
      outs.push(mv.amount);
    } else if (addDecimal(balance, mv.amount) === mv.saldo) {
      signed = mv.amount;
      ins.push(mv.amount);
    } else {
      throw new Unbalanced(`Continental ${number}: a row does not lead to its Saldo`);
    }
    balance = mv.saldo;
    const date = `${year}-${String(month).padStart(2, '0')}-${String(mv.day).padStart(2, '0')}`;
    const parts = mv.mov.split(/\s+/);
    // The account is in the id: both sides of an exchange share date, time and movement.
    const key = `${number}:${date}T${mv.time}:${parts.slice(0, 2).join('-')}`;
    const n = (seen.get(key) ?? 0) + 1;
    seen.set(key, n);
    const [description, counterparty] = mv.description.split(' | ');
    rows.push({
      kind: 'transaction', account: id, id: n > 1 ? `${key}:${n}` : key, date, amount: signed, currency,
      description: description.trim(), counterparty: counterparty?.trim() || undefined, reference: parts.slice(0, 2).join('-'),
    });
  }
  if (balance !== closing || sum(outs) !== totals[0] || sum(ins) !== totals[1]) {
    throw new Unbalanced(`Continental ${number}: the rows do not match the Totales or the Contable balance`);
  }
  rows.push({ kind: 'balance', account: id, id: `${number}:${to}:balance`, date: to, amount: closing, currency });
  return {
    name: `Banco Continental ${type} ${from.slice(0, 7)}`,
    account: { id, label: `Continental ${type} ***${number.slice(-4)}`, currency },
    rows,
  };
}

/** A credit card 'extracto', or null when the lines are not one. */
export function readContinentalCard(lines: string[][]): ParsedStatement | null {
  const head = lines.find((l) => l[0] === 'TARJETA');
  if (!head || !lines.some((l) => l.includes('RESUMEN ESTADO FINANCIERO'))) return null;
  const last4 = (head[1] ?? '').replace(/\s/g, '').slice(-4);
  const closing = dmy(lines.find((l) => l[0] === 'Actual')?.[1] ?? '');
  // A summary label ends a line; its figure is the next line's last word.
  const summary = (label: string) => {
    const i = lines.findIndex((l) => l[l.length - 1] === label);
    return i >= 0 ? gs(lines[i + 1]?.[lines[i + 1].length - 1]) : undefined;
  };
  const previous = summary('Deuda Anterior');
  const payments = summary('(-) Pagos');
  const purchases = summary('(+) Compra y cargos del mes');
  const total = summary('(=) DEUDA TOTAL DEL PERIODO');
  if (!last4 || !closing || !previous || !payments || !purchases || !total) return null;

  const id = `py-continental-card-${last4}`;
  const rows: Row[] = [];
  const seen = new Map<string, number>();
  const paid: string[] = [];
  const charged: string[] = [];
  for (let i = 0; i < lines.length; i++) {
    const w = lines[i];
    const date = dmy(w[0] ?? '');
    if (!date || !dmy(w[1] ?? '') || w.length < 4) continue;
    const coupon = w[2];
    let tail = w.slice(3);
    let note = '';
    let raw = tail[tail.length - 1];
    if (raw === 'SI' || raw === 'NO') {
      // A charge abroad: "28.300 LIQ. MARCA U$: 4.83 COTIZ.: 6033.00 29.139".
      const next = lines[i + 1] ?? [];
      if (next[1] !== 'LIQ.') continue;
      raw = next[next.length - 1];
      note = ` (U$ ${after(next, 'U$:')} @ ${after(next, 'COTIZ.:')})`;
      i++;
    } else {
      tail = tail.slice(0, -1);
    }
    const amount = gs(raw);
    if (!amount) continue;
    const credit = /\sCR$/.test(raw);
    const flag = tail.findIndex((x) => x === 'SI' || x === 'NO');
    const detail = (flag >= 0 ? tail.slice(0, flag) : tail).join(' ').replace(/\s+/g, ' ').trim();
    const payment = credit && /^SU PAGO/.test(detail);
    (payment ? paid : charged).push(credit ? neg(amount) : amount);
    const key = `${last4}:${date}:${coupon}`;
    const n = (seen.get(key) ?? 0) + 1;
    seen.set(key, n);
    rows.push({
      kind: 'transaction', account: id, id: n > 1 ? `${key}:${n}` : key, date, amount: credit ? amount : neg(amount),
      currency: 'PYG', description: detail + note,
    });
  }
  // Financial charges are in the Compras but on no line: the footer's table
  // "Compras no Financiables + | Gastos Financieros + | ..." has them second.
  const fh = lines.findIndex((l) => l[0] === 'Compras no Financiables +' && l[1] === 'Gastos Financieros +');
  const finance = fh >= 0 ? gs(lines[fh + 1]?.[1]) : undefined;
  if (finance && finance !== '0') {
    charged.push(finance);
    rows.push({ kind: 'transaction', account: id, id: `${last4}:${closing}:gastos-financieros`, date: closing, amount: neg(finance), currency: 'PYG', description: 'Gastos financieros' });
  }
  const paidSum = neg(sum(paid));
  if (addDecimal(addDecimal(previous, neg(payments)), purchases) !== total || paidSum !== payments || sum(charged) !== purchases) {
    throw new Unbalanced(`Continental card ${last4}: the rows do not add up to the summary`);
  }
  // A charge dated after the closing is still on it: the assertion follows it.
  const asOf = rows.reduce((d, r) => (r.date > d ? r.date : d), closing);
  rows.push({ kind: 'balance', account: id, id: `${last4}:${closing}:balance`, date: asOf, amount: neg(total), currency: 'PYG' });
  return {
    name: `Banco Continental card ***${last4} ${closing.slice(0, 7)}`,
    account: { id, label: `Continental card ***${last4}`, currency: 'PYG' },
    rows,
  };
}
