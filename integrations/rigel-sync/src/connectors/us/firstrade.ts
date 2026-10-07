/*
 * Firstrade (#82): the JSON API its mobile app speaks (api3x.firstrade.com),
 * as MaxxRK/firstrade-api (MIT) reverse-engineered it, ported here so no
 * Python is needed. The response shapes follow morristai/firstrade's models.
 * Unofficial: Firstrade may change it without notice.
 *
 * Sign-in: username and password, then the account's second factor -- a
 * PIN, an authenticator app (its secret, when given, answers by itself;
 * otherwise the code is asked), or a code sent by email or SMS (asked). The
 * answer is sent with remember_for=30, and the session token it returns
 * (ftat) is kept in the connection's state, so for 30 days a run asks
 * nothing.
 *
 * Per account: the cash balance; the positions as holdings; the history as
 * trades (buys and sells, with their cash) on the brokerage account and as
 * transactions (dividends, interest, fees, withholding, deposits) on its
 * cash account. Map the brokerage account's settlement account to the
 * ledger account the cash account maps to: one account, two sources.
 *
 * Securities are US:<symbol>. The API names no exchange, and a listing that
 * moves between exchanges must stay one commodity. Only stocks and ETFs
 * (sec_type 1) are read; options and the rest are logged and skipped.
 *
 * Numbers arrive as JSON numbers and are read as their text (parseExact),
 * never through a double. The first run logs each endpoint's field names
 * (never values), so a changed API shows in `rigel-sync try`.
 */
import { plainDecimal, short } from '../rows.ts';
import type { Account, Batch, Connector, Logger, Row, SyncContext } from '../types.ts';
import { SyncError } from '../types.ts';
import { parseExact } from '../../util/json.ts';
import { totp } from '../../util/totp.ts';

export const ORIGIN = 'https://api3x.firstrade.com';
// The app's own request header, the same for every user and published in
// firstrade-api: not a secret.
export const ACCESS_TOKEN = '833w3XuIFycv18ybi'; // gitleaks:allow
const OTP_TTL_S = 600;
const HISTORY_PAGE = 1000;
const FIRST_YEAR = 2000; // the first run walks back a year at a time until two are empty
const OVERLAP_DAYS = 7;

interface FirstradeState {
  ftat?: string;
  /** Per account: the first day the next run asks history for. */
  since?: Record<string, string>;
}

type Json = Record<string, unknown>;

export const firstrade: Connector = {
  id: 'us-firstrade',
  name: 'Firstrade',
  country: 'US',
  fields: [
    { name: 'username', label: 'Username', kind: 'text' },
    { name: 'password', label: 'Password', kind: 'secret' },
    { name: 'pin', label: 'PIN (if your account uses one)', kind: 'secret', optional: true },
    { name: 'mfa_secret', label: 'Authenticator secret (optional: answers the app code by itself)', kind: 'secret', optional: true },
  ],
  sync: (ctx) => syncFirstrade(ctx, ORIGIN, new Date()),
};

/** The US day (New York) of a moment, YYYY-MM-DD. */
export const usDay = (d: Date) => new Intl.DateTimeFormat('en-CA', { timeZone: 'America/New_York' }).format(d);

const addDays = (day: string, n: number) => new Date(Date.parse(`${day}T00:00:00Z`) + n * 86_400_000).toISOString().slice(0, 10);

/** A history date: YYYY-MM-DD, MM/DD/YYYY or YYYYMMDD; undefined otherwise. */
export function firstradeDay(s: unknown): string | undefined {
  const v = String(s ?? '').trim();
  let m = /^(\d{4})-(\d{2})-(\d{2})/.exec(v);
  if (m) return `${m[1]}-${m[2]}-${m[3]}`;
  m = /^(\d{1,2})\/(\d{1,2})\/(\d{4})$/.exec(v);
  if (m) return `${m[3]}-${m[1].padStart(2, '0')}-${m[2].padStart(2, '0')}`;
  m = /^(\d{4})(\d{2})(\d{2})$/.exec(v);
  return m ? `${m[1]}-${m[2]}-${m[3]}` : undefined;
}

/** A Firstrade client: the app's headers plus the session's. */
class Client {
  readonly headers: Record<string, string> = { 'User-Agent': 'okhttp/4.9.2', 'access-token': ACCESS_TOKEN };
  private readonly origin: string;
  private readonly signal: AbortSignal;
  constructor(origin: string, signal: AbortSignal) {
    this.origin = origin;
    this.signal = signal;
  }

  async call(method: 'GET' | 'POST', path: string, form?: Record<string, string>): Promise<Json> {
    let res: Response;
    try {
      res = await fetch(this.origin + path, {
        method,
        headers: form ? { ...this.headers, 'Content-Type': 'application/x-www-form-urlencoded' } : this.headers,
        body: form ? new URLSearchParams(form).toString() : undefined,
        signal: AbortSignal.any([this.signal, AbortSignal.timeout(30_000)]),
      });
    } catch (err) {
      if (this.signal.aborted) throw new SyncError('cancelled');
      throw new SyncError('institution_down', (err as Error).message);
    }
    const text = await res.text();
    let body: unknown;
    try {
      body = parseExact(text);
    } catch {
      throw new SyncError('institution_down', `${path}: HTTP ${res.status}, not JSON`);
    }
    if (res.status >= 500) throw new SyncError('institution_down', `${path}: HTTP ${res.status}`);
    return (body && typeof body === 'object' ? body : {}) as Json;
  }

  session(ftat: string, sid: string) {
    if (ftat) this.headers.ftat = ftat;
    if (sid) this.headers.sid = sid;
  }
}

const str = (v: unknown) => (typeof v === 'string' ? v : v === undefined || v === null ? '' : String(v));
const failed = (j: Json) => str(j.error) !== '';

/** Signs in, answering the second factor; the session ends up in the client's headers. */
async function signIn(ctx: SyncContext, c: Client, state: FirstradeState): Promise<void> {
  const { username, password, pin, mfa_secret } = ctx.credentials;
  if (!username || !password) throw new SyncError('bad_credentials');
  if (state.ftat) c.headers.ftat = state.ftat; // a remembered device: no second factor
  const login = await c.call('POST', '/sess/login', { username, password });
  if (failed(login)) throw new SyncError('bad_credentials');
  const done = (j: Json) => {
    c.session(str(j.ftat) || str(login.ftat), str(j.sid) || str(login.sid));
    if (!c.headers.ftat) throw new SyncError('verification_failed', 'no session after sign-in');
  };
  if (!login.mfa && !login.otp && str(login.ftat)) {
    done(login);
    return;
  }
  const t_token = str(login.t_token);
  const verify = (form: Record<string, string>) => c.call('POST', '/sess/verify_pin', { ...form, remember_for: '30', t_token });
  let answer: Json;
  if (pin) {
    answer = await verify({ pin });
  } else if (login.mfa) {
    // An authenticator app: its secret answers, or the person does.
    let wrong = 0;
    for (;;) {
      const code = mfa_secret
        ? totp(mfa_secret)
        : (await ctx.ask({ kind: 'otp', prompt: wrong ? 'Firstrade: that code was not accepted; enter a new one' : 'Firstrade: enter the code from your authenticator app', ttlSeconds: OTP_TTL_S })).trim();
      answer = await verify({ mfaCode: code });
      if (!failed(answer) || mfa_secret || ++wrong >= 3) break;
    }
  } else {
    // A code by email or SMS: the first place Firstrade offers.
    const options = Array.isArray(login.otp) ? (login.otp as Json[]) : [];
    const to = options.find((o) => str(o.recipientId));
    if (!to) throw new SyncError('verification_failed', 'Firstrade offered no way to send a code');
    const sent = await c.call('POST', '/sess/request_code', { recipientId: str(to.recipientId), t_token });
    if (failed(sent)) throw new SyncError('verification_failed', 'Firstrade did not send a code');
    const verificationSid = str(sent.verificationSid);
    let wrong = 0;
    for (;;) {
      const where = `${str(to.channel) || 'a message'} to ${str(to.recipientMask)}`.trim();
      const code = await ctx.ask({ kind: 'otp', prompt: wrong ? 'Firstrade: that code was not accepted; enter it again' : `Firstrade: enter the code sent by ${where}`, ttlSeconds: OTP_TTL_S });
      answer = await verify({ otpCode: code.trim(), verificationSid });
      if (!failed(answer) || ++wrong >= 3) break;
    }
  }
  if (failed(answer)) throw new SyncError(pin ? 'bad_credentials' : 'bad_otp');
  done(answer);
  await ctx.saveState({ ...state, ftat: c.headers.ftat });
}

/** Logs the field names of an endpoint's first item, once per run: never its values. */
function shapes(log: Logger) {
  const seen = new Set<string>();
  return (endpoint: string, item: unknown) => {
    if (seen.has(endpoint) || !item || typeof item !== 'object') return;
    seen.add(endpoint);
    log.info('firstrade fields', { endpoint, fields: Object.keys(item).sort().join(',') });
  };
}

const OCC = /^[A-Z]{1,6}\d{6}[CP]\d{8}$/;

const items = (j: Json) => (Array.isArray(j.items) ? (j.items as Json[]) : []);

/** A history entry's kind, from its trans_str: buy, sell, or cash (anything with an amount). */
export function historyKind(e: Json): 'buy' | 'sell' | 'cash' | undefined {
  const t = `${str(e.trans_str)} ${str(e.shortDesc)}`.toUpperCase();
  if (/\b(BUY|BOUGHT|REINVEST)/.test(t)) return 'buy';
  if (/\b(SELL|SOLD)/.test(t)) return 'sell';
  const amount = plainDecimal(e.amount);
  return amount !== undefined && amount !== '0' ? 'cash' : undefined;
}

/** The rows of one account's history, oldest first as Firstrade sends them or not. */
export function historyRows(account: string, entries: Json[], log: Logger): Row[] {
  const rows: Row[] = [];
  const seen = new Map<string, number>();
  const skipped = new Map<string, number>();
  for (const e of entries) {
    const date = firstradeDay(e.report_date);
    const kind = historyKind(e);
    const symbol = str(e.symbol).trim().toUpperCase();
    const amount = plainDecimal(e.amount);
    const quantity = plainDecimal(e.quantity)?.replace(/^-/, '');
    if (!date || !kind) {
      const what = str(e.trans_str) || '?';
      skipped.set(what, (skipped.get(what) ?? 0) + 1);
      continue;
    }
    // No id in the history: the entry itself, plus how many alike came before.
    const key = short(account, date, str(e.trans_str), symbol, str(e.quantity), str(e.amount), str(e.description));
    const n = (seen.get(key) ?? 0) + 1;
    seen.set(key, n);
    const id = `${account}:${date}:${key}:${n}`;
    const description = str(e.description).replace(/\s+/g, ' ').trim() || str(e.trans_str);
    if (kind === 'cash') {
      rows.push({ kind: 'transaction', account: `${account}-cash`, id, date, amount: amount!, currency: 'USD', description });
      continue;
    }
    // An option (an OCC symbol) is skipped, as its position is: units are contracts of 100.
    if (!symbol || OCC.test(symbol) || !quantity || quantity === '0' || !amount || amount === '0') {
      skipped.set(str(e.trans_str) || '?', (skipped.get(str(e.trans_str) || '?') ?? 0) + 1);
      continue;
    }
    const abs = amount.replace(/^-/, '');
    const price = plainDecimal(e.trade_price);
    rows.push({
      kind: 'trade', account, id, date, security: `US:${symbol}`, quote_currency: 'USD', currency: 'USD',
      units: kind === 'buy' ? quantity : `-${quantity}`,
      cash: kind === 'buy' ? `-${abs}` : abs, // net of commission and fees, as Firstrade states it
      ...(price && price !== '0' ? { price } : {}),
      description,
    });
  }
  if (skipped.size) log.warn('firstrade history entries skipped', { kinds: Object.fromEntries(skipped) });
  return rows;
}

/** One account's holdings: stocks and ETFs only. */
export function positionRows(account: string, positions: Json[], today: string, log: Logger): Row[] {
  const rows: Row[] = [];
  const skipped = new Map<string, number>();
  for (const p of positions) {
    const symbol = str(p.symbol).trim().toUpperCase();
    const units = plainDecimal(p.quantity);
    const type = str(p.sec_type);
    if (type !== '1' || !symbol || units === undefined || units.startsWith('-')) {
      skipped.set(`sec_type ${type || '?'}`, (skipped.get(`sec_type ${type || '?'}`) ?? 0) + 1);
      continue;
    }
    const last = plainDecimal(p.last);
    rows.push({
      kind: 'holding', account, id: `${account}:${symbol}:${today}`, date: today,
      security: `US:${symbol}`, security_name: str(p.company_name).trim() || undefined, quote_currency: 'USD', units,
      ...(last && last !== '0' ? { price: last } : {}),
    });
  }
  if (skipped.size) log.warn('firstrade positions skipped', { kinds: Object.fromEntries(skipped) });
  return rows;
}

/** History from a day to another, every page. */
async function history(c: Client, account: string, from: string, to: string, saw: ReturnType<typeof shapes>): Promise<Json[]> {
  const out: Json[] = [];
  for (let page = 1; page <= 50; page++) {
    const q = new URLSearchParams({ range: 'cust', page: String(page), account, per_page: String(HISTORY_PAGE) });
    q.append('range_arr[]', from);
    q.append('range_arr[]', to);
    const j = await c.call('GET', `/private/account_history?${q}`);
    if (failed(j)) throw new SyncError('institution_down', `history: ${str(j.error)}`);
    const got = items(j);
    saw('account_history', got[0]);
    out.push(...got);
    if (got.length < HISTORY_PAGE) break;
  }
  return out;
}

export async function syncFirstrade(ctx: SyncContext, origin: string, now: Date): Promise<Batch> {
  const state = { ...(ctx.state as FirstradeState) };
  const c = new Client(origin, ctx.signal);
  const saw = shapes(ctx.log);
  await signIn(ctx, c, state);
  state.ftat = c.headers.ftat;

  await c.call('GET', '/private/userinfo');
  const list = await c.call('GET', '/private/acct_list');
  if (failed(list)) {
    // A remembered session Firstrade no longer honours: forget it, sign in afresh next run.
    await ctx.saveState({ ...state, ftat: undefined });
    throw new SyncError('verification_failed', 'Firstrade refused the session');
  }
  saw('acct_list', items(list)[0]);
  const today = usDay(now);
  const accounts: Account[] = [];
  const rows: Row[] = [];
  const since: Record<string, string> = { ...(state.since ?? {}) };
  for (const a of items(list)) {
    const number = str(a.account);
    if (!number) continue;
    const id = `fr-${number.slice(-4)}`;
    const name = `Firstrade ${str(a.alias) || str(a.type) || ''} ····${number.slice(-4)}`.replace(/\s+/g, ' ');
    accounts.push({ id, label: name, currency: 'USD', kind: 'brokerage' }, { id: `${id}-cash`, label: `${name} cash`, currency: 'USD' });

    const bal = await c.call('GET', `/private/balances?account=${encodeURIComponent(number)}`);
    const result = (bal.result ?? {}) as Json;
    saw('balances', result);
    const cash = plainDecimal(result.cash_balance);
    if (cash !== undefined) rows.push({ kind: 'balance', account: `${id}-cash`, id: `${id}-cash:${today}:balance`, date: today, amount: cash, currency: 'USD' });

    const pos = await c.call('GET', `/private/positions?account=${encodeURIComponent(number)}&per_page=200`);
    saw('positions', items(pos)[0]);
    rows.push(...positionRows(id, items(pos), today, ctx.log));

    // History: from where the last run stopped (a week back, for late entries),
    // or the first time, back a year at a time until two years are empty.
    const entries: Json[] = [];
    if (since[number]) {
      entries.push(...(await history(c, number, since[number], today, saw)));
    } else {
      let empty = 0;
      for (let y = Number(today.slice(0, 4)); y >= FIRST_YEAR && empty < 2; y--) {
        const got = await history(c, number, `${y}-01-01`, y === Number(today.slice(0, 4)) ? today : `${y}-12-31`, saw);
        empty = got.length ? 0 : empty + 1;
        entries.push(...got);
      }
    }
    rows.push(...historyRows(id, entries, ctx.log));
    since[number] = addDays(today, -OVERLAP_DAYS);
  }
  await ctx.saveState({ ...state, since });
  ctx.log.info('firstrade synced', { accounts: accounts.length / 2, rows: rows.length });
  return { label: `Firstrade ${today}`, accounts, rows };
}
