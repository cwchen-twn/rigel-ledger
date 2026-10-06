/*
 * 集保 e存摺 (TDCC ePassbook): what every broker account holds, the units
 * that moved in it, and the settlement (交割) bank accounts behind them,
 * from all-set-tw's connector (vendor/all-set-tw, a pinned commit, unedited),
 * which speaks the e存摺 app's API -- no browser.
 *
 * The first sign-in from a new device sends a code by email (sometimes
 * followed by one by SMS); this runner then is that device (its id is kept
 * in the connection's state with the session), and later runs ask nothing.
 *
 * Mapped from 集保's own strings, not upstream's numbers and ISO dates:
 * share counts and prices stay exact decimals, and upstream's dates, built
 * in the runner's time zone and printed in UTC, would be a day early on a
 * laptop in Taiwan.
 *
 * A trade's direction is read from its name (買進, 賣出, 配股, ...): 集保
 * sends no signed quantity. A name not in the lists below is skipped and
 * logged, so a new one shows in `rigel-sync try` instead of being guessed.
 * 集保 sends no cash for a trade (no fees, no tax): the queue asks for it.
 */
import { randomBytes } from 'node:crypto';
import {
  initializeTdccSnapshot,
  parseTdccConfig,
  syncTdccTradeHistory,
  type TdccBankEntry,
  TdccConnectionError,
  type TdccInvestmentTransaction,
  TdccOtpExpiredError,
  type TdccSnapshotInitialization,
  type TdccStockAccount,
  TdccVerificationRequiredError,
} from '../../../vendor/all-set-tw/tdcc.js';
import { last4, plainDecimal, short, taipeiDay } from '../rows.ts';
import type { Account, Batch, Connector, Logger, Row, SyncContext } from '../types.ts';
import { SyncError } from '../types.ts';

const OTP_TRIES = 3;
const OTP_TTL_S = 600; // an email can take a while

// What a movement's name says about its direction. Units in: bought,
// allotted, transferred in, subscribed. Out: sold, transferred out,
// redeemed, cancelled.
const UNITS_IN = ['買進', '配股', '配發', '轉入', '撥入', '申購', '增資', '存入', '認購', '無償'];
const UNITS_OUT = ['賣出', '轉出', '撥出', '贖回', '減資', '提出', '註銷', '領回'];

interface TdccState {
  cursor?: string; // upstream's: device id, session, each account's trade-history paging
}

export const tdcc: Connector = {
  id: 'tw-tdcc',
  name: '集保 e存摺 TDCC ePassbook',
  country: 'TW',
  fields: [
    { name: 'id_number', label: 'ID number (身分證字號)', kind: 'id_number' },
    { name: 'password', label: 'e存摺 password', kind: 'secret' },
  ],
  async sync(ctx) {
    const { id_number, password } = ctx.credentials;
    if (!id_number || !password) throw new SyncError('bad_credentials');
    const state = ctx.state as TdccState;
    // This runner is one device to 集保: the same id every run, kept before
    // the first sign-in so a code answered once is not asked again.
    let cursor = state.cursor;
    if (!cursor) {
      cursor = JSON.stringify({ deviceId: randomBytes(8).toString('hex'), devType: 'Android:14', devModel: 'SM-G991B' });
      await ctx.saveState({ cursor });
    }
    const base = parseTdccConfig({ userId: id_number, password });

    const snap = await signIn(ctx, base, cursor);
    const movements = await settlementMovements(snap, ctx.log);
    // The trade history reuses this session, and pages on where it stopped.
    const previous = JSON.parse(cursor) as { tradeCursors?: unknown };
    const history = await syncTdccTradeHistory(
      base,
      JSON.stringify({ ...snap.identity, session: snap.session, tradeCursors: previous.tradeCursors }),
    ).catch((err: unknown) => {
      throw failure(err, false);
    });
    await ctx.saveState({ cursor: history.cursor });
    return toBatch(snap, movements, history.investmentTransactions, taipeiDay(new Date()), ctx.log);
  },
};

/** Signs in, answering 集保's codes through ctx.ask. */
async function signIn(ctx: SyncContext, base: ReturnType<typeof parseTdccConfig>, cursor: string): Promise<TdccSnapshotInitialization> {
  let cfg = base;
  let wrong = 0;
  for (;;) {
    try {
      return await initializeTdccSnapshot(cfg, cursor, ctx.signal);
    } catch (err) {
      const answered = cfg.otp !== undefined;
      if (err instanceof TdccVerificationRequiredError) {
        // A code was sent (by email, or by SMS after the email one).
        const otp = await ctx.ask({
          kind: 'otp',
          prompt: wrong
            ? '集保 e存摺: that code was not accepted; enter it again'
            : `集保 e存摺: enter the code sent by ${err.channel === 'sms' ? 'SMS' : 'email'}`,
          ttlSeconds: OTP_TTL_S,
        });
        // requestOtp stays on: with a code entered, upstream sends no new email,
        // but it must still be free to send the SMS that may follow.
        cfg = { ...cfg, otp: otp.trim(), otpChannel: err.channel };
        continue;
      }
      if (err instanceof TdccOtpExpiredError) {
        if (++wrong >= OTP_TRIES) throw new SyncError('challenge_expired');
        cfg = { ...cfg, otp: undefined, requestOtp: true }; // a fresh code
        continue;
      }
      if (answered && err instanceof TdccConnectionError && !err.code.startsWith('HTTP_')) {
        // Rejected after a code was entered: the code, most likely.
        if (++wrong >= OTP_TRIES) throw new SyncError('bad_otp');
        const otp = await ctx.ask({ kind: 'otp', prompt: '集保 e存摺: that code was not accepted; enter it again', ttlSeconds: OTP_TTL_S });
        cfg = { ...cfg, otp: otp.trim() };
        continue;
      }
      throw failure(err, answered);
    }
  }
}

/** A stable code for what the person can act on; anything else stays an error for the log. */
function failure(err: unknown, answered: boolean): unknown {
  if (err instanceof SyncError) return err;
  if (err instanceof TdccConnectionError) {
    if (err.code.startsWith('HTTP_')) return new SyncError('institution_down', err.message);
    // Refused before any code: 集保 names no single code for a wrong password.
    return new SyncError(answered ? 'bad_otp' : 'verification_failed', err.message);
  }
  if (err instanceof TdccVerificationRequiredError) return new SyncError('verification_failed', err.message);
  return err;
}

/** Each settlement account's movements; one that fails is logged and skipped. */
async function settlementMovements(snap: TdccSnapshotInitialization, log: Logger) {
  const out = new Map<TdccBankEntry, Awaited<ReturnType<TdccSnapshotInitialization['client']['getBankTransactions']>>['transactions']>();
  for (const e of snap.bankEntries) {
    try {
      out.set(e, (await snap.client.getBankTransactions(e.bankId, e.accountNo, e.currency)).transactions);
    } catch (err) {
      log.warn('a settlement account\'s movements could not be read', { bank: e.bankId, account: `***${last4(e.accountNo)}`, error: String(err) });
    }
  }
  return out;
}

/** 集保 dates: YYYYMMDD, or a ROC year (7 digits, or 8 zero-padded). */
export function tdccDay(value: string | undefined): string | undefined {
  const v = (value ?? '').trim().replace(/[/-]/g, '');
  const m = /^(\d{3,4})(\d{2})(\d{2})/.exec(v);
  if (!m) return undefined;
  let year = Number(m[1]);
  if (year < 1000) year += 1911;
  if (year < 1950) return undefined; // 19000101: "no date"
  return `${year}-${m[2]}-${m[3]}`;
}

/** A plain decimal from 集保's "1,234.5000" (rows.ts). */
export const tdccDecimal = plainDecimal;

const brokerId = (brokerNo: string, brokerAccount: string) => `broker-${brokerNo}-${last4(brokerAccount)}`;
const security = (symbol: string, fund: boolean) => `${fund ? 'FUND' : 'XTAI'}:${symbol.toUpperCase()}`;

function direction(t: TdccInvestmentTransaction): 1 | -1 | 0 {
  const name = t.transactionName ?? t.raw.fields.txnName ?? '';
  if (UNITS_OUT.some((w) => name.includes(w))) return -1;
  if (UNITS_IN.some((w) => name.includes(w))) return 1;
  return 0;
}

/**
 * 集保's snapshot as one batch. A broker account is `broker-<no>-<last4>`
 * (kind brokerage), a fund holder `funds-<org>`, a settlement account
 * `bank-<bank>-<last4>` (with its currency when not TWD). Account numbers
 * keep their last four digits only.
 */
export function toBatch(
  snap: Pick<TdccSnapshotInitialization, 'stockAccounts' | 'bankEntries' | 'snapshot'>,
  movements: Map<TdccBankEntry, Array<{ txnId: string; occurredAt: string; amount: string; memo?: string }>>,
  trades: TdccInvestmentTransaction[],
  day: string,
  log: Pick<Logger, 'warn'>,
): Batch {
  const accounts = new Map<string, Account>();
  const rows: Row[] = [];
  const brokerOf = (a: TdccStockAccount) => {
    const id = brokerId(a.brokerNo, a.brokerAccount);
    if (!accounts.has(id)) {
      accounts.set(id, { id, label: `${a.brokerName || `券商 ${a.brokerNo}`} ***${last4(a.brokerAccount)}`, currency: 'TWD', kind: 'brokerage' });
    }
    return id;
  };
  for (const a of snap.stockAccounts) brokerOf(a);

  for (const h of snap.snapshot.holdings) {
    const fund = h.securityType === 'fund';
    const units = tdccDecimal(h.quantity);
    if (!h.symbol || units === undefined) continue;
    let account: string;
    if (!fund && h.brokerNo && h.brokerAccount) {
      account = brokerOf({ brokerNo: h.brokerNo, brokerAccount: h.brokerAccount, brokerName: h.brokerName });
    } else {
      const org = h.accountId ?? 'tdcc';
      account = `funds-${org}`;
      if (!accounts.has(account)) accounts.set(account, { id: account, label: `基金 ${org}`, currency: 'TWD', kind: 'brokerage' });
    }
    const date = tdccDay(h.asOfDate) ?? day;
    rows.push({
      kind: 'holding', account, id: `${account}:${h.symbol}:${date}`, date,
      security: security(h.symbol, fund), security_name: h.securityName, quote_currency: (h.currency || 'TWD').toUpperCase(), units,
    });
  }

  const unknown = new Set<string>();
  for (const t of trades) {
    const f = t.raw.fields;
    const dir = direction(t);
    const date = tdccDay(f.txnDate) ?? tdccDay(f.postDate);
    const moved = tdccDecimal(f.txnSHR);
    if (!t.symbol || !date || moved === undefined || moved === '0') continue;
    if (dir === 0) {
      unknown.add(`${t.transactionName ?? '?'} (${t.transactionCode ?? '?'})`);
      continue;
    }
    // 集保 sometimes sends 1 as a placeholder price.
    const price = tdccDecimal(f.price);
    const account = brokerOf({ brokerNo: t.brokerNo, brokerAccount: t.brokerAccount, brokerName: t.brokerName });
    rows.push({
      kind: 'trade', account, id: `${account}:${t.sourceId}`, date,
      security: security(t.symbol, t.assetType === 'fund'), security_name: t.name, quote_currency: (t.currency || 'TWD').toUpperCase(),
      units: dir < 0 ? `-${moved}` : moved,
      price: price && price !== '1' && price !== '0' ? price : undefined,
      description: [t.transactionName, t.name].filter(Boolean).join(' '),
    });
  }
  if (unknown.size) log.warn('集保 movements of a kind not recognised were left out', { kinds: [...unknown] });

  for (const e of snap.bankEntries) {
    const cur = (e.currency || 'TWD').toUpperCase();
    const id = `bank-${e.bankId}-${last4(e.accountNo)}${cur === 'TWD' ? '' : `-${cur}`}`;
    accounts.set(id, { id, label: `交割戶 ${e.bankId} ***${last4(e.accountNo)}${cur === 'TWD' ? '' : ` ${cur}`}`, currency: cur });
    const balance = tdccDecimal(e.balanceAmt);
    if (balance !== undefined) rows.push({ kind: 'balance', account: id, id: `${id}:${day}:balance`, date: day, amount: balance, currency: cur });
    const seen = new Map<string, number>(); // 集保 can send the same movement id twice
    for (const m of movements.get(e) ?? []) {
      const date = m.occurredAt.slice(0, 10);
      const amount = tdccDecimal(m.amount);
      if (date.startsWith('1970') || amount === undefined || amount === '0') continue;
      const key = `${id}:${date}:${short(m.txnId)}`;
      const n = (seen.get(key) ?? 0) + 1;
      seen.set(key, n);
      rows.push({ kind: 'transaction', account: id, id: `${key}:${n}`, date, amount, currency: cur, description: m.memo || undefined });
    }
  }

  return { label: `集保 e存摺 ${day}`, accounts: [...accounts.values()], rows };
}
