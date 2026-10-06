/*
 * 國泰期貨 國內月對帳單 (#42): history only -- futures move to 永豐 Shioaji,
 * which reports equity every day. So a month is booked the way a day will
 * be then, from the statement's 保證金及權利金專戶餘額 table alone (its fills
 * are already summed there), on the margin account the source maps to:
 *
 *   realised    權利金 + 平倉損益 + 到期履約損益, on the last day
 *   手續費, 期交稅  each a row of its own (rules give them their categories)
 *   存提         deposits and withdrawals: the bank's side is the transfer
 *   unrealised  權益總值 - 本月餘額 (open futures, option market values) on
 *               the last day, reversed the next day: the account then
 *               shows the equity the broker states, like Shioaji will
 *   balance     權益總值 on the last day, an assertion
 *
 * The table must add up (月初 + movements = 本月餘額) or nothing is sent.
 * The daily 買賣報告書 are left alone: the month has the same fills.
 *
 * Shared with the web app's PDF import (web/src/lib/statements.ts): no
 * imports but src/decimal.ts and types, so it runs in a browser too.
 */
import { addDecimal, plainDecimal } from '../decimal.ts';
import type { Account, Row } from '../connectors/types.ts';

export interface FuturesMonth {
  account: string; // 交易人帳號, digits only
  from: string;
  to: string; // YYYY-MM-DD
  values: Record<string, string>; // the table's 國內基幣 (TWD) column, by label
}

const LABELS = ['月初餘額', '存提', '權利金收入與支出', '本月期貨平倉損益淨額', '到期履約損益', '手續費', '期交稅', '本月餘額',
  '未沖銷期貨浮動損益', '權益數', '未沖銷買方選擇權市值', '未沖銷賣方選擇權市值', '權益總值'] as const;

const money = (s: string | undefined) => plainDecimal((s ?? '').replace(/TWD$/, '').replace(/^(-?)\./, '$10.'));
const neg = (s: string) => (s === '0' ? '0' : s.startsWith('-') ? s.slice(1) : `-${s}`);
const slashDay = (s: string) => /^(\d{4})\/(\d{2})\/(\d{2})$/.exec(s)?.slice(1).join('-');

export function isCathayFutMonthly(m: { from: string; subject: string }): boolean {
  return /(^|[@.])cathayfut\.com\.tw$/.test(m.from) && m.subject.includes('月買賣報告書');
}

/** The month a statement's lines describe, or null when they are not one. */
export function readMonth(lines: string[][]): FuturesMonth | null {
  let account = '';
  let from = '';
  let to = '';
  const values: Record<string, string> = {};
  let inTable = false;
  for (const words of lines) {
    const text = words.join(' ');
    if (!account) {
      const a = /交易人帳號[：:]\s*([\d ]+)/.exec(text);
      const d = /(\d{4}\/\d{2}\/\d{2})\s*至\s*(\d{4}\/\d{2}\/\d{2})/.exec(text);
      if (a && d) [account, from, to] = [a[1].replace(/\s/g, ''), slashDay(d[1]) ?? '', slashDay(d[2]) ?? ''];
    }
    if (text.includes('保證金及權利金專戶餘額')) inTable = true;
    if (!inTable) continue;
    const label = LABELS.find((l) => words[0] === l);
    // The first figure after the label is the 國內基幣 (all-TWD) column.
    if (label && !(label in values)) {
      const v = money(words[1]);
      if (v !== undefined) values[label] = v;
    }
  }
  if (!account || !from || !to || LABELS.some((l) => !(l in values))) return null;
  return { account, from, to, values };
}

/** The month's rows; throws when its table does not add up. */
export function monthRows(m: FuturesMonth, file?: string): { account: Account; rows: Row[] } {
  const v = m.values;
  const realised = [v['權利金收入與支出'], v['本月期貨平倉損益淨額'], v['到期履約損益']].reduce(addDecimal, '0');
  const end = [v['月初餘額'], v['存提'], realised, neg(v['手續費']), neg(v['期交稅'])].reduce(addDecimal, '0');
  if (end !== v['本月餘額']) throw new Error(`國泰期貨 ${m.to}: the balance table does not add up (${end} vs ${v['本月餘額']})`);
  const equity = [v['權益數'], v['未沖銷買方選擇權市值'], neg(v['未沖銷賣方選擇權市值'])].reduce(addDecimal, '0');
  if (equity !== v['權益總值'] || addDecimal(v['本月餘額'], v['未沖銷期貨浮動損益']) !== v['權益數']) {
    throw new Error(`國泰期貨 ${m.to}: the equity lines do not add up`);
  }
  const unrealised = addDecimal(v['權益總值'], neg(v['本月餘額']));
  const id = `tw-cathayfut-${m.account}`;
  const month = m.to.slice(0, 7);
  const next = new Date(`${m.to}T00:00:00Z`);
  next.setUTCDate(next.getUTCDate() + 1);
  const rows: Row[] = [];
  const row = (key: string, date: string, amount: string, description: string) => {
    if (amount !== '0') rows.push({ kind: 'transaction', account: id, id: `${month}:${key}`, date, amount, currency: 'TWD', description, counterparty: '國泰期貨' });
  };
  row('realised', m.to, realised, `期貨已實現損益 ${month}`);
  row('fees', m.to, neg(v['手續費']), `期貨手續費 ${month}`);
  row('tax', m.to, neg(v['期交稅']), `期貨交易稅 ${month}`);
  row('transfers', m.to, v['存提'], `期貨保證金存提 ${month}`);
  row('unrealised', m.to, unrealised, `期貨未實現損益 ${m.to}`);
  row('unrealised-reversal', next.toISOString().slice(0, 10), neg(unrealised), `期貨未實現損益迴轉 ${m.to}`);
  if (file && rows.length) rows[0].file = file;
  rows.push({ kind: 'balance', account: id, id: `${month}:balance`, date: m.to, amount: v['權益總值'], currency: 'TWD' });
  return { account: { id, label: `國泰期貨 ${m.account.slice(0, 3)} ***${m.account.slice(-4)}`, currency: 'TWD' }, rows };
}
