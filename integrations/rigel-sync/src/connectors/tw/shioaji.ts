/*
 * 永豐金證券 through Shioaji (#83), Sinopac's official API, read-only: the
 * stock account's holdings and trades with their cash, and the futures
 * account's realised P&L, fees, tax and balance. Shioaji's own server is
 * run for the sync (src/util/shioaji.ts); an API key with the Account
 * permission only is enough (no CA certificate: that is for orders).
 *
 * Shioaji keeps no list of past fills (list_trades is today's only), but it
 * keeps lots: an open position's lots (position_detail: date, shares,
 * price, fee) and every closed one (profit_loss for a range of sell days,
 * each with the buy legs it closed: profit_loss_detail, with their cost).
 * So trades are rebuilt a day at a time, per security and side:
 *
 *   buy   the lots bought that day, open and closed: units, and cash = -(the
 *         legs' cost + the open lots' price x shares + fee)
 *   sell  the lots sold that day: units, and cash = the legs' cost + the
 *         realised P&L (net of the sale's fee and tax)
 *
 * A row's id is its day, security and side: a lot closing later moves from
 * open to closed and the day's total stays the same. Every run reads the
 * closed lots from the oldest open lot's day, so a buy is whole the first
 * time it is sent. Rows of today wait until after the close (15:00, Taipei)
 * with the holdings, so a day is never sent half done.
 *
 * Only cash trades are rebuilt: margin (融資), short sales and their lots
 * are logged and skipped, and so is anything that does not add up (lots
 * against their position). Holdings are every long position, in shares.
 *
 * The futures account: each closed position's P&L, fee and tax as rows of
 * their own, and the day's balance (today_balance, without the open
 * positions' unrealised P&L, which is exposure, not booked) as an
 * assertion. Money moved in or out that day is a row too, for the bank's
 * side to pair with.
 *
 * 複委託 (H) accounts are not in Shioaji's portfolio API: logged and skipped.
 */
import { join } from 'node:path';
import { addDecimal, mulDecimal, plainDecimal, short, taipeiDay } from '../rows.ts';
import type { Account, Batch, Connector, Logger, Row, SyncContext } from '../types.ts';
import { SyncError } from '../types.ts';
import { type ShioajiServer, shioajiBinary, startShioaji } from '../../util/shioaji.ts';

const FIRST_WINDOW_DAYS = 365; // the first run's history, unless a lot is older
const OVERLAP_DAYS = 7;
const CHUNK_DAYS = 180;
const CLOSE_HOUR = 15; // Taipei: the market and the day's lots are settled

type Json = Record<string, unknown>;

interface ShioajiState {
  /** Per account (S or F + its number): the first day the next run asks for. */
  since?: Record<string, string>;
}

export const shioaji: Connector = {
  id: 'tw-shioaji',
  name: '永豐金證券 Shioaji',
  country: 'TW',
  fields: [
    { name: 'api_key', label: 'API key (Account permission only)', kind: 'secret' },
    { name: 'secret_key', label: 'Secret key', kind: 'secret' },
    { name: 'environment', label: 'Environment', kind: 'choice', options: ['production', 'simulation'] },
  ],
  async sync(ctx) {
    const { api_key, secret_key, environment } = ctx.credentials;
    if (!api_key || !secret_key) throw new SyncError('bad_credentials');
    const bin = await shioajiBinary(ctx.signal, ctx.log);
    const server = await startShioaji({
      bin, apiKey: api_key, secretKey: secret_key, production: environment !== 'simulation', signal: ctx.signal,
      // One home per key: its token pool is that login's.
      home: join(process.env.DATA_DIR || '/data', 'shioaji', short(api_key)),
    });
    try {
      return await syncShioaji(ctx, server, new Date());
    } finally {
      await server.stop();
    }
  },
};

const str = (v: unknown) => (v === undefined || v === null ? '' : String(v));
const num = (v: unknown) => plainDecimal(v) ?? '0';
const neg = (s: string) => (s === '0' ? '0' : s.startsWith('-') ? s.slice(1) : `-${s}`);
const addDays = (day: string, n: number) => new Date(Date.parse(`${day}T00:00:00Z`) + n * 86_400_000).toISOString().slice(0, 10);
const list = (v: unknown) => (Array.isArray(v) ? (v as Json[]) : []);
/** Shioaji's dates: "2026-09-02", "2026/09/02" or "20260902". */
export function sjDay(v: unknown): string | undefined {
  const m = /^(\d{4})[-/]?(\d{2})[-/]?(\d{2})/.exec(str(v));
  return m ? `${m[1]}-${m[2]}-${m[3]}` : undefined;
}

/** The hour in Taipei, 0-23. */
const taipeiHour = (d: Date) => Number(new Intl.DateTimeFormat('en-GB', { timeZone: 'Asia/Taipei', hour: '2-digit', hourCycle: 'h23' }).format(d));

/** Holdings in shares, whether unit "Share" lists every share or only odd lots beside "Common" (lots of 1,000). */
export function stockHoldings(account: string, common: Json[], share: Json[], today: string, log: Logger): Row[] {
  const sum = (ps: Json[]) => {
    const m = new Map<string, string>();
    for (const p of ps) {
      if (str(p.direction) !== 'Buy') continue; // a short sale holds nothing
      const code = str(p.code).trim();
      m.set(code, addDecimal(m.get(code) ?? '0', num(p.quantity)));
    }
    return m;
  };
  const lots = sum(common);
  const shares = sum(share);
  const last = new Map(share.concat(common).map((p) => [str(p.code).trim(), plainDecimal(p.last_price)]));
  const rows: Row[] = [];
  let oddOnly = 0;
  for (const code of new Set([...lots.keys(), ...shares.keys()])) {
    const inLots = mulDecimal(lots.get(code) ?? '0', '1000');
    const s = shares.get(code) ?? '0';
    // "Share" either counts every share (>= the lots') or only the odd ones.
    let units = s;
    if (addDecimal(s, neg(inLots)).startsWith('-')) {
      units = addDecimal(inLots, s);
      oddOnly++;
    }
    if (units === '0') continue;
    const price = last.get(code);
    rows.push({
      kind: 'holding', account, id: `${account}:${code}:${today}`, date: today, security: `XTAI:${code}`, quote_currency: 'TWD', units,
      ...(price && price !== '0' ? { price } : {}),
    });
  }
  if (oddOnly) log.info('shioaji: unit Share listed odd lots only', { positions: oddOnly });
  return rows;
}

export interface OpenLot {
  code: string;
  date: string;
  quantity: string; // shares
  price: string;
  fee: string;
}
export interface ClosedSale {
  code: string;
  date: string; // the sale's day
  quantity: string; // shares sold
  pnl: string; // realised, net of the sale's fee and tax
  legs: Array<{ date: string; quantity: string; cost: string }>; // the buys it closed
}

/**
 * The day's trades per security and side, from open lots and closed sales.
 * A day after `upTo` is left out (today, before the close).
 */
export function stockTrades(account: string, open: OpenLot[], closed: ClosedSale[], upTo: string): Row[] {
  const days = new Map<string, { code: string; date: string; side: 'buy' | 'sell'; units: string; cash: string }>();
  const add = (code: string, date: string, side: 'buy' | 'sell', units: string, cash: string) => {
    if (date > upTo) return;
    const k = `${date}:${code}:${side}`;
    const d = days.get(k) ?? { code, date, side, units: '0', cash: '0' };
    d.units = addDecimal(d.units, units);
    d.cash = addDecimal(d.cash, cash);
    days.set(k, d);
  };
  for (const l of open) add(l.code, l.date, 'buy', l.quantity, neg(addDecimal(mulDecimal(l.price, l.quantity), l.fee)));
  for (const s of closed) {
    let cost = '0';
    for (const leg of s.legs) {
      add(s.code, leg.date, 'buy', leg.quantity, neg(leg.cost));
      cost = addDecimal(cost, leg.cost);
    }
    add(s.code, s.date, 'sell', neg(s.quantity), addDecimal(cost, s.pnl));
  }
  return [...days.entries()]
    .filter(([, d]) => d.units !== '0' && d.cash !== '0')
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([k, d]) => ({
      kind: 'trade', account, id: `${account}:${k}`, date: d.date, security: `XTAI:${d.code}`, quote_currency: 'TWD', currency: 'TWD',
      units: d.units, cash: d.cash, description: `${d.side === 'buy' ? '買進' : '賣出'} ${d.code}`, counterparty: '永豐金證券',
    }));
}

/** The futures account's closed positions as P&L, fee and tax rows, and the day's balance. */
export function futuresRows(account: string, closed: Json[], margin: Json | undefined, today: string, upTo: string): Row[] {
  const rows: Row[] = [];
  const seen = new Map<string, number>();
  for (const c of closed) {
    const date = sjDay(c.date);
    if (!date || date > upTo) continue;
    const code = str(c.code);
    const key = short(date, code, str(c.direction), str(c.quantity), str(c.entry_price), str(c.cover_price), str(c.pnl), str(c.fee), str(c.tax));
    const n = (seen.get(key) ?? 0) + 1;
    seen.set(key, n);
    const id = `${account}:${date}:${key}:${n}`;
    const row = (part: string, amount: string, description: string) => {
      if (amount !== '0') rows.push({ kind: 'transaction', account, id: `${id}:${part}`, date, amount, currency: 'TWD', description, counterparty: '永豐期貨' });
    };
    row('pnl', num(c.pnl), `期貨平倉損益 ${code}`);
    row('fee', neg(num(c.fee)), `期貨手續費 ${code}`);
    row('tax', neg(num(c.tax)), `期貨交易稅 ${code}`);
  }
  if (margin && today <= upTo) {
    const moved = num(margin.deposit_withdrawal);
    if (moved !== '0') {
      rows.push({ kind: 'transaction', account, id: `${account}:${today}:transfers`, date: today, amount: moved, currency: 'TWD', description: '期貨保證金存提', counterparty: '永豐期貨' });
    }
    rows.push({ kind: 'balance', account, id: `${account}:${today}:balance`, date: today, amount: num(margin.today_balance), currency: 'TWD' });
  }
  return rows;
}

/** Days from..to in chunks the API is asked for one at a time. */
function* chunks(from: string, to: string): Generator<[string, string]> {
  for (let a = from; a <= to; a = addDays(a, CHUNK_DAYS)) {
    const b = addDays(a, CHUNK_DAYS - 1);
    yield [a, b < to ? b : to];
  }
}

export async function syncShioaji(ctx: SyncContext, sj: ShioajiServer, now: Date): Promise<Batch> {
  const state = ctx.state as ShioajiState;
  const since: Record<string, string> = { ...(state.since ?? {}) };
  const today = taipeiDay(now);
  const afterClose = taipeiHour(now) >= CLOSE_HOUR;
  const upTo = afterClose ? today : addDays(today, -1);
  if (!afterClose) ctx.log.info('shioaji: before the close, today waits for the next run');

  const accounts: Account[] = [];
  const rows: Row[] = [];
  for (const a of list(await sj.get('auth/accounts'))) {
    const type = str(a.account_type);
    const number = str(a.account_id);
    const ref = { account_type: type, broker_id: str(a.broker_id), account_id: number };
    const key = `${type}${number}`;
    const first = since[key] ?? addDays(today, -FIRST_WINDOW_DAYS);
    if (type === 'S') {
      const id = `sinopac-stock-${number.slice(-4)}`;
      accounts.push({ id, label: `永豐金證券 ····${number.slice(-4)}`, currency: 'TWD', kind: 'brokerage' });
      rows.push(...(await stockAccount(ctx.log, sj, ref, id, today, first, upTo, afterClose)));
    } else if (type === 'F') {
      const id = `sinopac-futures-${number.slice(-4)}`;
      accounts.push({ id, label: `永豐期貨 ····${number.slice(-4)}`, currency: 'TWD' });
      const closed: Json[] = [];
      for (const [b, e] of chunks(first, today)) closed.push(...list(await sj.post('portfolio/profit_loss', { ...ref, begin_date: b, end_date: e })));
      const margin = (await sj.post('portfolio/margin', ref)) as Json;
      rows.push(...futuresRows(id, closed, margin, today, upTo));
    } else {
      ctx.log.info('shioaji: an account its portfolio API does not cover is skipped', { type });
      continue;
    }
    since[key] = addDays(upTo, -OVERLAP_DAYS);
  }
  await ctx.saveState({ ...state, since });
  ctx.log.info('shioaji synced', { accounts: accounts.length, rows: rows.length });
  return { label: `永豐金證券 ${today}`, accounts, rows };
}

async function stockAccount(log: Logger, sj: ShioajiServer, ref: Json, id: string, today: string, since: string, upTo: string, afterClose: boolean): Promise<Row[]> {
  const common = list(await sj.post('portfolio/position_unit', { ...ref, unit: 'Common' }));
  const share = list(await sj.post('portfolio/position_unit', { ...ref, unit: 'Share' }));
  const rows: Row[] = afterClose ? stockHoldings(id, common, share, today, log) : [];

  // Open lots, per position: they must add up to it, in shares or in lots of 1,000.
  const open: OpenLot[] = [];
  const skipped = new Set<string>();
  for (const p of share) {
    const code = str(p.code).trim();
    if (str(p.direction) !== 'Buy' || (str(p.cond) && str(p.cond) !== 'Cash')) {
      skipped.add(code);
      continue;
    }
    const lots = list(await sj.post('portfolio/position_detail', { ...ref, detail_id: p.id }));
    const total = lots.reduce((s, l) => addDecimal(s, num(l.quantity)), '0');
    const scale = total === num(p.quantity) ? '1' : mulDecimal(total, '1000') === num(p.quantity) ? '1000' : undefined;
    if (!scale) {
      log.warn('shioaji: a position whose lots do not add up is left out of the trades', { code });
      skipped.add(code);
      continue;
    }
    for (const l of lots) {
      const date = sjDay(l.date);
      if (!date) continue;
      open.push({ code, date, quantity: mulDecimal(num(l.quantity), scale), price: num(l.price), fee: num(l.fee) });
    }
  }

  // Closed lots: from the oldest open lot (so its day's buy is whole), or where the last run stopped.
  const from = open.reduce((m, l) => (l.date < m ? l.date : m), since);
  const closed: ClosedSale[] = [];
  let costDiffers = 0;
  for (const [b, e] of chunks(from, today)) {
    for (const s of list(await sj.post('portfolio/profit_loss', { ...ref, begin_date: b, end_date: e, unit: 'Share' }))) {
      const code = str(s.code).trim();
      const date = sjDay(s.date);
      if (!date || (str(s.cond) && str(s.cond) !== 'Cash')) {
        skipped.add(code);
        continue;
      }
      const legs = list(await sj.post('portfolio/profit_loss_detail', { ...ref, detail_id: s.id, unit: 'Share' }));
      const sale: ClosedSale = { code, date, quantity: num(s.quantity), pnl: num(s.pnl), legs: [] };
      for (const l of legs) {
        const d = sjDay(l.date);
        if (!d) continue;
        const cost = num(l.cost);
        if (cost !== addDecimal(mulDecimal(num(l.price), num(l.quantity)), num(l.fee))) costDiffers++;
        sale.legs.push({ date: d, quantity: num(l.quantity), cost });
      }
      const legUnits = sale.legs.reduce((t, l) => addDecimal(t, l.quantity), '0');
      if (legUnits !== sale.quantity) {
        log.warn('shioaji: a sale whose legs do not add up is left out of the trades', { code });
        skipped.add(code);
        continue;
      }
      closed.push(sale);
    }
  }
  if (costDiffers) log.info('shioaji: legs whose cost is not price x shares + fee', { legs: costDiffers });
  if (skipped.size) log.warn('shioaji: margin, short or unmatched trades left out', { securities: skipped.size });
  const keep = (code: string) => !skipped.has(code);
  rows.push(...stockTrades(id, open.filter((l) => keep(l.code)), closed.filter((s) => keep(s.code)), upTo));
  return rows;
}
