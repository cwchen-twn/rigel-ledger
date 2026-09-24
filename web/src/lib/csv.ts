import { neg, parseAmount, strip, sub } from '~/lib/money';

/*
 * Bank statement CSVs, read in the browser: the file never leaves the tab
 * until its parsed rows go to the import API. Every bank has its own columns,
 * date format, decimal comma and encoding (Taiwanese banks still export
 * Big5), so the person maps them once per file.
 */

export type Encoding = 'utf-8' | 'big5' | 'windows-1252';
export type DateFormat = 'YYYY-MM-DD' | 'YYYY/MM/DD' | 'DD/MM/YYYY' | 'MM/DD/YYYY' | 'ROC';
export const DATE_FORMATS: DateFormat[] = ['YYYY-MM-DD', 'YYYY/MM/DD', 'DD/MM/YYYY', 'MM/DD/YYYY', 'ROC'];

export function decode(buf: ArrayBuffer, enc: Encoding): string {
  const s = new TextDecoder(enc).decode(buf);
  return s.charCodeAt(0) === 0xfeff ? s.slice(1) : s;
}

/** The delimiter used most on the first lines: comma, semicolon or tab. */
export function sniffDelimiter(text: string): string {
  const head = text.split(/\r?\n/, 5).join('\n');
  let best = ',';
  let most = -1;
  for (const d of [',', ';', '\t']) {
    const n = head.split(d).length;
    if (n > most) [best, most] = [d, n];
  }
  return best;
}

/** RFC 4180: quoted fields, doubled quotes, newlines inside quotes. Blank lines stay (as ['']), so indexes are file lines. */
export function parseCSV(text: string, delim: string): string[][] {
  const rows: string[][] = [];
  let row: string[] = [];
  let field = '';
  let quoted = false;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (quoted) {
      if (c === '"' && text[i + 1] === '"') {
        field += '"';
        i++;
      } else if (c === '"') {
        quoted = false;
      } else {
        field += c;
      }
    } else if (c === '"' && field === '') {
      quoted = true;
    } else if (c === delim) {
      row.push(field);
      field = '';
    } else if (c === '\n' || c === '\r') {
      if (c === '\r' && text[i + 1] === '\n') i++;
      row.push(field);
      rows.push(row);
      row = [];
      field = '';
    } else {
      field += c;
    }
  }
  row.push(field);
  if (row.some((f) => f.trim() !== '')) rows.push(row); // no phantom row after a final newline
  return rows;
}

const pad = (n: number) => String(n).padStart(2, '0');

/** A statement date as YYYY-MM-DD, or null. ROC is the Minguo year (115/09/24 = 2026-09-24). */
export function parseDate(raw: string, fmt: DateFormat): string | null {
  const parts = raw.trim().split(/[^\d]+/).filter(Boolean).map(Number);
  if (parts.length < 3) {
    // 20260924 or 1150924
    const digits = raw.trim().replace(/\D/g, '');
    if (fmt === 'ROC' && digits.length === 7) return parseDate(`${digits.slice(0, 3)}/${digits.slice(3, 5)}/${digits.slice(5)}`, fmt);
    if (digits.length === 8 && fmt.startsWith('YYYY')) return parseDate(`${digits.slice(0, 4)}-${digits.slice(4, 6)}-${digits.slice(6)}`, fmt);
    return null;
  }
  let [y, m, d] = [0, 0, 0];
  switch (fmt) {
    case 'YYYY-MM-DD':
    case 'YYYY/MM/DD':
      [y, m, d] = parts;
      break;
    case 'ROC':
      [y, m, d] = parts;
      y += 1911;
      break;
    case 'DD/MM/YYYY':
      [d, m, y] = parts;
      break;
    case 'MM/DD/YYYY':
      [m, d, y] = parts;
      break;
  }
  if (y < 100) y += 2000;
  const dt = new Date(Date.UTC(y, m - 1, d));
  if (dt.getUTCFullYear() !== y || dt.getUTCMonth() !== m - 1 || dt.getUTCDate() !== d) return null;
  return `${y}-${pad(m)}-${pad(d)}`;
}

/**
 * An amount cell as a decimal string. decimalComma reads "1.234,50" as
 * 1234.50. "(12.00)" and a trailing minus are negative; currency symbols
 * and letters are ignored. Empty is null.
 */
export function parseCell(raw: string, decimalComma: boolean): string | null {
  let s = raw.trim();
  if (s === '') return null;
  let negative = false;
  if (/^\(.*\)$/.test(s)) {
    negative = true;
    s = s.slice(1, -1);
  }
  if (s.endsWith('-')) {
    negative = true;
    s = s.slice(0, -1);
  }
  s = s.replace(/[^\d.,-]/g, '');
  s = decimalComma ? s.replace(/\./g, '').replace(',', '.') : s.replace(/,/g, '');
  const v = parseAmount(s);
  if (v === null) return null;
  return negative ? neg(v) : v;
}

export interface Mapping {
  date: number;
  description: number;
  counterparty: number; // -1: none
  /** One signed column, or separate money-out / money-in columns. */
  amount: number; // -1 when out/in are used
  out: number;
  in: number;
  /** A card statement lists spending as positive: flip the sign. */
  flip: boolean;
  dateFormat: DateFormat;
  decimalComma: boolean;
}

export interface ParsedRow {
  line: number; // 1-based, in the file
  date: string;
  amount: string;
  description: string;
  counterparty: string;
  cells: string[];
}

export interface ParseResult {
  rows: ParsedRow[];
  errors: { line: number; reason: 'date' | 'amount' }[];
}

export function applyMapping(table: string[][], firstLine: number, m: Mapping): ParseResult {
  const out: ParseResult = { rows: [], errors: [] };
  table.forEach((cells, i) => {
    if (cells.every((c) => c.trim() === '')) return;
    const line = firstLine + i;
    const cell = (c: number) => (c >= 0 ? (cells[c] ?? '').trim() : '');
    const date = parseDate(cell(m.date), m.dateFormat);
    if (!date) {
      out.errors.push({ line, reason: 'date' });
      return;
    }
    let amount: string | null;
    if (m.amount >= 0) {
      amount = parseCell(cell(m.amount), m.decimalComma);
    } else {
      // Money in minus money out; banks leave the unused one blank (or 0).
      const o = parseCell(cell(m.out), m.decimalComma);
      const n = parseCell(cell(m.in), m.decimalComma);
      amount = o === null && n === null ? null : strip(sub(n ?? '0', o ?? '0'));
    }
    if (amount === null) {
      out.errors.push({ line, reason: 'amount' });
      return;
    }
    if (m.flip) amount = neg(amount);
    out.rows.push({ line, date, amount, description: cell(m.description), counterparty: cell(m.counterparty), cells });
  });
  return out;
}

async function sha256(s: string): Promise<string> {
  const buf = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(s));
  return Array.from(new Uint8Array(buf).slice(0, 12), (b) => b.toString(16).padStart(2, '0')).join('');
}

/**
 * A stable id per row: the hash of what the row says, plus how many identical
 * rows came before it in the file (two 50.00 coffees on one day are two rows).
 * Importing the same file, or an overlapping export, again stages nothing new.
 */
export async function externalIds(rows: ParsedRow[], account: string): Promise<string[]> {
  const seen = new Map<string, number>();
  const out: string[] = [];
  for (const r of rows) {
    // The account is part of the key: ids are unique per book, and a transfer
    // can read the same on both statements.
    const key = `${account}|${r.date}|${r.amount}|${r.description}|${r.counterparty}`;
    const n = seen.get(key) ?? 0;
    seen.set(key, n + 1);
    out.push(`${await sha256(key)}-${n}`);
  }
  return out;
}

/** A best guess at the columns from header names in English, Spanish and Chinese. */
export function guessMapping(header: string[]): Partial<Mapping> {
  const find = (re: RegExp) => header.findIndex((h) => re.test(h.trim().toLowerCase()));
  const m: Partial<Mapping> = {};
  const date = find(/date|fecha|日期|交易日|入帳日/);
  if (date >= 0) m.date = date;
  const desc = find(/desc|concepto|detalle|memo|摘要|說明|備註|明細/);
  if (desc >= 0) m.description = desc;
  const party = find(/payee|counterparty|beneficiar|merchant|comercio|對方|商店|特店/);
  m.counterparty = party;
  const amount = find(/^amount$|^importe$|^monto$|金額/);
  const out = find(/debit|withdraw|out|débito|cargo|支出|提款/);
  const inn = find(/credit|deposit|\bin\b|crédito|abono|存入|收入/);
  if (out >= 0 && inn >= 0) Object.assign(m, { amount: -1, out, in: inn });
  else if (amount >= 0) m.amount = amount;
  return m;
}
