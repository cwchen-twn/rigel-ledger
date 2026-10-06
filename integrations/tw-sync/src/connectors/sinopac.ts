/*
 * 永豐銀行 (Bank SinoPac): deposit accounts (TWD and foreign) and the credit
 * card, from all-set-tw's connector (vendor/all-set-tw, a pinned commit,
 * unedited). It signs in on the mobile site in Chrome, which asks for a
 * six-digit image CAPTCHA, then reads everything from the bank's app JSON
 * API with the session's cookies, no browser.
 *
 * The cookies are kept in the connection's state and tried first. When the
 * bank no longer takes them, the runner signs in again reading the CAPTCHA
 * itself (src/ocr/captcha.ts, three fresh images, as upstream does with
 * its Workers AI model), and only then asks the person: the image goes to
 * Connections ("Needs you") and the run waits for the six digits.
 */
import { TimeoutError } from 'puppeteer-core';
import {
  createSinopacConnector,
  loginSinopacWithOcr,
  prepareSinopacCaptcha,
  SINOPAC_SESSION_PROTOCOL,
  SinopacCaptchaRejectedError,
  type SinopacConfig,
  SinopacCredentialRejectedError,
  type SinopacResult,
  SinopacVerificationRequiredError,
} from '../../vendor/all-set-tw/sinopac.js';
import { closeSession } from '../browser/cloudflare.ts';
import { toDecimal } from '../money.ts';
import { readDigits } from '../ocr/captcha.ts';
import { dayOf, last4, short, taipeiDay } from './rows.ts';
import type { Account, Batch, Connector, Row, SyncContext } from './types.ts';
import { SyncError } from './types.ts';

const CAPTCHA_TRIES = 3;
const CAPTCHA_TTL_S = 300;

/** The connector; `fetch` reaches the bank's JSON API (a test points it elsewhere). */
export function makeSinopac(opts: { fetch?: typeof fetch } = {}): Connector {
  return {
    id: 'sinopac',
    name: '永豐銀行 Bank SinoPac',
    country: 'TW',
    fields: [
      { name: 'id_number', label: 'ID number (身分證字號)', kind: 'id_number' },
      { name: 'user_code', label: 'User code (使用者代碼)', kind: 'text' },
      { name: 'password', label: 'Password', kind: 'secret' },
    ],
    async sync(ctx) {
      const { id_number, user_code, password } = ctx.credentials;
      if (!id_number || !user_code || !password) throw new SyncError('bad_credentials');
      const binding = { syncSignal: ctx.signal };
      const base: SinopacConfig = { userId: id_number, account: user_code, password };
      const bank = createSinopacConnector(binding, opts.fetch);
      let result: SinopacResult | undefined;

      try {
        const saved = typeof ctx.state.sessionCookies === 'string' ? ctx.state.sessionCookies : undefined;
        if (saved) {
          try {
            result = await bank.sync({ ...base, sessionCookies: saved, protocol: SINOPAC_SESSION_PROTOCOL });
          } catch (err) {
            if (!(err instanceof SinopacVerificationRequiredError)) throw err;
            ctx.log.info('the saved session has ended; signing in again');
          }
        }
        if (!result) result = await withOcr(ctx, bank, base);
        if (!result) result = await byHand(ctx, bank, base);
      } catch (err) {
        throw failure(err);
      }

      const cursor = result.cursor ? (JSON.parse(result.cursor) as { sessionCookies?: string }) : {};
      if (cursor.sessionCookies) await ctx.saveState({ sessionCookies: cursor.sessionCookies });
      return toBatch(result, taipeiDay(new Date()));
    },
  };
}

export const sinopac = makeSinopac();

type Bank = ReturnType<typeof createSinopacConnector>;

/** Signs in reading the CAPTCHA here; undefined when the person must read it. */
async function withOcr(ctx: SyncContext, bank: Bank, base: SinopacConfig): Promise<SinopacResult | undefined> {
  let session: Awaited<ReturnType<typeof loginSinopacWithOcr>>;
  try {
    session = await loginSinopacWithOcr({ syncSignal: ctx.signal }, base, async (image) => readDigits(new Uint8Array(image)));
  } catch (err) {
    if (err instanceof SinopacCredentialRejectedError || err instanceof TimeoutError) throw err;
    // Three images not read (or no model to read them with): the person reads one.
    ctx.log.warn('could not read the CAPTCHA here; asking', { error: err instanceof Error ? err.message : String(err) });
    return undefined;
  }
  await ctx.saveState({ sessionCookies: session.sessionCookies });
  return bank.sync({ ...base, ...session });
}

/** Shows the person the CAPTCHA, up to three images. */
async function byHand(ctx: SyncContext, bank: Bank, base: SinopacConfig): Promise<SinopacResult> {
  let browserSessionId: string | undefined;
  let wrong = 0;
  try {
    for (;;) {
      const p = await prepareSinopacCaptcha({ syncSignal: ctx.signal }, { ...base, browserSessionId });
      browserSessionId = p.browserSessionId;
      const answer = await ctx.ask({
        kind: 'captcha',
        prompt: wrong ? '永豐銀行: that was not it; type the six digits in this new image' : '永豐銀行: type the six digits in the image',
        image: Buffer.from(p.captchaImage.slice(p.captchaImage.indexOf(',') + 1), 'base64'),
        ttlSeconds: CAPTCHA_TTL_S,
      });
      const captcha = answer.replace(/\s/g, '');
      try {
        if (!/^\d{6}$/.test(captcha)) throw new SinopacCaptchaRejectedError('not six digits');
        // Upstream gives the image two minutes; a person may take longer, and the bank says if it is too late.
        const expires = new Date(Date.now() + 2 * CAPTCHA_TTL_S * 1000).toISOString();
        return await bank.sync({ ...base, browserSessionId, browserSessionExpiresAt: expires, captcha });
      } catch (err) {
        if (!(err instanceof SinopacCaptchaRejectedError)) throw err;
        if (++wrong >= CAPTCHA_TRIES) throw new SyncError('bad_captcha');
        await closeSession(browserSessionId); // a new page, a new image
        browserSessionId = undefined;
      }
    }
  } finally {
    if (browserSessionId) await closeSession(browserSessionId);
  }
}

/** A stable code for what the person can act on; anything else stays an error for the log. */
function failure(err: unknown): unknown {
  if (err instanceof SyncError) return err;
  if (err instanceof SinopacCredentialRejectedError) return new SyncError('bad_credentials');
  if (err instanceof SinopacVerificationRequiredError) return new SyncError('verification_failed', err.message);
  if (err instanceof TimeoutError) return new SyncError('institution_down', err.message);
  return err;
}

/**
 * Upstream's result as one batch. A deposit account is `deposit-<last4>`
 * (with its currency when not TWD: one account number holds several), the
 * card `card` (`card-<currency>` for a foreign one). Card rows are signed
 * as upstream signs them (refunds and payments in), and an authorisation
 * not yet posted is a pending row with its own id, so the posted row later
 * clears it. As with 國泰, the card's balance is the statement's, not what
 * is owed today, so only deposits send one.
 */
export function toBatch(r: SinopacResult, day: string): Batch {
  const accounts: Account[] = [];
  const rows: Row[] = [];
  const ours = new Map<string, Account>(); // upstream sourceId -> ours
  const credit = new Set<string>();

  for (const a of r.bankAccounts ?? []) {
    const isCard = a.accountType === 'credit';
    const foreign = a.currency === 'TWD' ? '' : `-${a.currency}`;
    // bank:sinopac:<last4>:<sha256 of the number>:<currency>
    const [, , four = '', digest = ''] = a.sourceId.split(':');
    let id = isCard ? `card${foreign}` : `deposit-${last4(four)}${foreign}`;
    if (accounts.some((x) => x.id === id)) id = `deposit-${last4(four)}-${digest.slice(0, 8)}${foreign}`; // two numbers ending alike
    const label = isCard
      ? (a.accountName ?? `永豐信用卡${foreign}`)
      : `永豐存款${a.currency === 'TWD' ? '' : ` ${a.currency}`} ***${last4(four)}`;
    const acc = { id, label, currency: a.currency };
    accounts.push(acc);
    ours.set(a.sourceId, acc);
    if (isCard) credit.add(a.sourceId);
  }
  const accountOf = (sourceId: string) => {
    const a = ours.get(sourceId);
    if (!a) throw new Error('sinopac: a row for an account it did not list');
    return a;
  };

  for (const b of r.bankBalanceSnapshots ?? []) {
    if (credit.has(b.accountId)) continue;
    const a = accountOf(b.accountId);
    rows.push({ kind: 'balance', account: a.id, id: `${a.id}:${day}:balance`, date: day, amount: toDecimal(b.balance, b.currency), currency: b.currency });
  }

  for (const t of r.bankTransactions ?? []) {
    if (t.amount === 0) continue;
    const a = accountOf(t.accountId);
    const pending = t.status === 'pending';
    const date = dayOf('sinopac', t);
    const description = (t.description ?? '').trim();
    // Upstream's sourceId is already stable (and counts repeats); the same
    // charge pending and then posted shares it, so the status is part of ours.
    rows.push({
      kind: 'transaction',
      account: a.id,
      id: `${a.id}:${date}:${short(pending ? 'pending' : 'posted', t.sourceId)}`,
      date,
      amount: toDecimal(t.amount, t.currency),
      currency: t.currency,
      description: description || undefined,
      pending: pending || undefined,
    });
  }

  return { label: `永豐銀行 ${day}`, accounts, rows };
}
