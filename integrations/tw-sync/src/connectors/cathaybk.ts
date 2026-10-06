/*
 * 國泰世華 (Cathay United Bank): deposit accounts and the credit card, from
 * all-set-tw's connector (vendor/all-set-tw, a pinned commit, unedited),
 * which drives the bank's website in Chrome for the whole run.
 *
 * Upstream runs on Cloudflare, where a run cannot wait for a person: when
 * the bank wants a one-time code the connector throws, leaving the browser
 * open, and the next call carries the channel, then the code. This runs
 * those calls in one sync, asking the person through ctx.ask in between.
 * After the code the bank trusts this browser (a CUB.eBank.DeviceId cookie,
 * kept in the connection's state), so later runs ask nothing.
 */
import { TimeoutError } from 'puppeteer-core';
import {
  type CathaybkConfig,
  type CathaybkResult,
  CathayOtpChannelRequiredError,
  CathayOtpInvalidError,
  CathayOtpRequiredError,
  CathayOtpSessionExpiredError,
  CathayVerificationRequiredError,
  createCathaybkConnector,
} from '../../vendor/all-set-tw/cathaybk.js';
import { closeSession } from '../browser/cloudflare.ts';
import { toDecimal } from '../money.ts';
import { dayOf, last4, short, taipeiDay } from './rows.ts';
import type { Account, Batch, Connector, Row } from './types.ts';
import { SyncError } from './types.ts';

const OTP_TRIES = 3;
const OTP_TTL_S = 300;

export const cathaybk: Connector = {
  id: 'cathaybk',
  name: '國泰世華銀行 Cathay United Bank',
  country: 'TW',
  fields: [
    { name: 'id_number', label: 'ID number (身分證字號)', kind: 'id_number' },
    { name: 'user_code', label: 'User code (用戶代號)', kind: 'text' },
    { name: 'password', label: 'Password', kind: 'secret' },
    { name: 'otp_channel', label: 'Send one-time codes by', kind: 'choice', options: ['sms', 'email'], optional: true },
  ],
  async sync(ctx) {
    const { id_number, user_code, password } = ctx.credentials;
    if (!id_number || !user_code || !password) throw new SyncError('bad_credentials');
    const channel = ctx.credentials.otp_channel === 'email' ? 'email' : 'sms';
    const bank = createCathaybkConnector({ syncSignal: ctx.signal });
    const later = () => new Date(Date.now() + 2 * OTP_TTL_S * 1000).toISOString();

    let cfg: CathaybkConfig = {
      userId: id_number,
      account: user_code,
      password,
      sessionCookies: typeof ctx.state.sessionCookies === 'string' ? ctx.state.sessionCookies : undefined,
    };
    let session: string | undefined;
    let wrong = 0;
    let result: CathaybkResult;
    try {
      for (;;) {
        try {
          result = await bank.sync(cfg);
          break;
        } catch (err) {
          if (err instanceof CathayOtpChannelRequiredError) {
            ctx.log.info('the bank wants a one-time code', { channel });
            session = err.browserSessionId;
            cfg = { ...cfg, browserSessionId: session, browserSessionExpiresAt: later(), otpChannel: channel, otp: undefined };
            continue;
          }
          if (err instanceof CathayOtpRequiredError || err instanceof CathayOtpInvalidError) {
            if (err instanceof CathayOtpInvalidError && ++wrong >= OTP_TRIES) throw new SyncError('bad_otp');
            const otp = await ctx.ask({
              kind: 'otp',
              prompt: wrong
                ? '國泰世華: that code was not accepted; enter it again'
                : `國泰世華: enter the code sent by ${channel === 'email' ? 'email' : 'SMS'}`,
              ttlSeconds: OTP_TTL_S,
            });
            cfg = { ...cfg, otp, browserSessionExpiresAt: later() };
            continue;
          }
          throw failure(err);
        }
      }
    } finally {
      if (session) await closeSession(session);
    }

    const cursor = result.cursor ? (JSON.parse(result.cursor) as { sessionCookies?: string }) : {};
    if (cursor.sessionCookies && cursor.sessionCookies !== '[]') await ctx.saveState({ sessionCookies: cursor.sessionCookies });
    return toBatch(result, taipeiDay(new Date()));
  },
};

/** A stable code for what the person can act on; anything else stays an error for the log. */
function failure(err: unknown): unknown {
  if (err instanceof CathayOtpSessionExpiredError) return new SyncError('challenge_expired');
  if (err instanceof CathayVerificationRequiredError) return new SyncError('verification_failed', err.message);
  if (err instanceof TimeoutError) return new SyncError('institution_down', err.message);
  // Upstream's message when the bank answered the login with an error page.
  if (err instanceof Error && err.message === 'Cathay United Bank login failed.') return new SyncError('bad_credentials');
  return err;
}

export { taipeiDay };

/**
 * Upstream's result as one batch. Account ids keep the last four digits
 * only. Card rows are signed from the statement itself: a charge credits
 * the card, a refund or a payment debits it (upstream makes every card row
 * negative). The card's "balance" upstream is the unpaid statement, not
 * what is owed today, so it is not sent as a balance: it would show a drift
 * as soon as anything new is charged.
 */
export function toBatch(r: CathaybkResult, day: string): Batch {
  const accounts: Account[] = [];
  const rows: Row[] = [];
  const ours = new Map<string, Account>(); // upstream sourceId -> ours
  const credit = new Set<string>();

  for (const a of r.bankAccounts ?? []) {
    let id = a.accountType === 'credit' ? 'card' : `deposit-${last4(a.sourceId)}`;
    if (accounts.some((x) => x.id === id)) id = `deposit-${a.sourceId.replace(/\D/g, '')}`; // two numbers ending alike
    const label = a.accountType === 'credit' ? (a.accountName ?? '國泰信用卡') : `${a.accountName ?? '國泰世華存款'} ***${last4(a.sourceId)}`;
    const acc = { id, label, currency: a.currency };
    accounts.push(acc);
    ours.set(a.sourceId, acc);
    if (a.accountType === 'credit') credit.add(a.sourceId);
  }
  const accountOf = (sourceId: string) => {
    const a = ours.get(sourceId);
    if (!a) throw new Error('cathaybk: a row for an account it did not list');
    return a;
  };

  for (const b of r.bankBalanceSnapshots ?? []) {
    if (credit.has(b.accountId)) continue;
    const a = accountOf(b.accountId);
    rows.push({ kind: 'balance', account: a.id, id: `${a.id}:${day}:balance`, date: day, amount: toDecimal(b.balance, b.currency), currency: b.currency });
  }

  const seen = new Map<string, number>();
  for (const t of r.bankTransactions ?? []) {
    const a = accountOf(t.accountId);
    let amount = t.amount;
    if (credit.has(t.accountId)) {
      const charged = (t.raw as { amount?: unknown } | undefined)?.amount;
      if (typeof charged !== 'number') throw new Error('cathaybk: a card row without its statement amount');
      amount = -charged;
    }
    if (amount === 0) continue;
    const date = dayOf('cathaybk', t);
    const value = toDecimal(amount, t.currency);
    const description = (t.description ?? '').trim();
    const key = `${a.id}:${date}:${short(value, description)}`;
    const n = (seen.get(key) ?? 0) + 1; // the same amount and text twice in a day
    seen.set(key, n);
    rows.push({ kind: 'transaction', account: a.id, id: `${key}:${n}`, date, amount: value, currency: t.currency, description: description || undefined });
  }

  return { label: `國泰世華 ${day}`, accounts, rows };
}
