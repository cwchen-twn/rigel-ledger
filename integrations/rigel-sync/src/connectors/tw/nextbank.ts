/*
 * 將來銀行 (Next Bank, #66): the main account, its 口袋 (demand pockets, and
 * term-deposit pockets as balances), from all-set-tw's client for the
 * bank's web API (vendor/all-set-tw, a pinned commit, unedited). No browser.
 *
 * Signing in takes a five-character CAPTCHA of letters and digits (a PNG),
 * read here (src/ocr/captcha.ts, letters-and-digits mode, up to three
 * fresh images) before the person is asked, three tries. A wrong ID, user
 * code or password stops at once: upstream never retries a password, and
 * the bank locks the login after five.
 *
 * Moves between the main account and a pocket appear on both, as two
 * rows; the import queue pairs them as one transfer.
 */
import {
  collectNextbankDepositPayloads,
  NextbankApiClient,
  NextbankApiError,
  parseNextbankDeposits,
} from '../../../vendor/all-set-tw/nextbank.js';
import { toDecimal } from '../../money.ts';
import { keepCaptcha, readAlphanumeric } from '../../ocr/captcha.ts';
import { short, taipeiDay } from '../rows.ts';
import type { Account, Batch, Connector, Row, SyncContext } from '../types.ts';
import { SyncError } from '../types.ts';

const OCR_TRIES = 3;
const ASK_TRIES = 3;
const ASK_TTL_S = 110; // upstream holds a CAPTCHA for two minutes

type Result = ReturnType<typeof parseNextbankDeposits>;

export function makeNextbank(opts: { fetch?: typeof fetch; read?: (png: Uint8Array) => Promise<string> } = {}): Connector {
  const read = opts.read ?? readAlphanumeric;
  return {
    id: 'tw-nextbank',
    name: '將來銀行 Next Bank',
    country: 'TW',
    fields: [
      { name: 'id_number', label: 'ID number (身分證字號)', kind: 'id_number' },
      { name: 'user_code', label: 'User code (使用者代碼)', kind: 'text' },
      { name: 'password', label: 'Password', kind: 'secret' },
    ],
    async sync(ctx) {
      const { id_number, user_code, password } = ctx.credentials;
      if (!id_number || !user_code || !password) throw new SyncError('bad_credentials');
      const client = new NextbankApiClient({ fetcher: opts.fetch });
      let token: string | undefined;
      try {
        token = await signIn(ctx, client, read, { identity: id_number.trim().toUpperCase(), userId: user_code.trim(), password });
        const result = parseNextbankDeposits(await collectNextbankDepositPayloads(client, token));
        return toBatch(result, taipeiDay(new Date()));
      } catch (err) {
        throw failure(err);
      } finally {
        if (token) await client.logout(token).catch(() => ctx.log.warn('sign-out not confirmed'));
      }
    },
  };
}

export const nextbank = makeNextbank();

async function signIn(
  ctx: SyncContext,
  client: NextbankApiClient,
  read: (png: Uint8Array) => Promise<string>,
  who: { identity: string; userId: string; password: string },
): Promise<string> {
  let ocr = 0;
  let wrong = 0;
  for (;;) {
    const challenge = await client.prepareCaptcha();
    const image = new Uint8Array(Buffer.from(challenge.imageBase64, 'base64'));
    let answer: string;
    const byHand = ocr >= OCR_TRIES;
    if (!byHand) {
      ocr++;
      answer = await read(image).catch(() => '');
      if (!/^[A-Za-z0-9]{5}$/.test(answer)) {
        await keepCaptcha('nextbank', image, answer, 'unsure');
        continue; // unsure: a fresh image
      }
    } else {
      if (ocr++ === OCR_TRIES) ctx.log.warn('could not read the CAPTCHA here; asking');
      answer = (await ctx.ask({
        kind: 'captcha',
        prompt: wrong ? '將來銀行: that was not it; type the five letters and digits in this new image' : '將來銀行: type the five letters and digits in the image',
        image: Buffer.from(image),
        ttlSeconds: ASK_TTL_S,
      })).replace(/\s/g, '');
      if (!/^[A-Za-z0-9]{1,5}$/.test(answer)) {
        if (++wrong >= ASK_TRIES) throw new SyncError('bad_captcha');
        continue;
      }
    }
    try {
      const { accessToken } = await client.login({ ...who, captchaResult: answer });
      if (!byHand) await keepCaptcha('nextbank', image, answer, 'accepted');
      return accessToken;
    } catch (err) {
      if (!(err instanceof NextbankApiError) || err.kind !== 'captcha') throw err;
      if (!byHand) await keepCaptcha('nextbank', image, answer, 'rejected');
      if (byHand && ++wrong >= ASK_TRIES) throw new SyncError('bad_captcha');
    }
  }
}

/** A stable code for what the person can act on; never the bank's text. */
function failure(err: unknown): unknown {
  if (err instanceof SyncError || !(err instanceof NextbankApiError)) return err;
  switch (err.kind) {
    case 'credentials':
      return new SyncError('bad_credentials');
    case 'session_conflict':
    case 'session_expired':
    case 'account_unavailable':
      return new SyncError('verification_failed', err.kind);
    case 'rate_limit':
    case 'transport':
      return new SyncError('institution_down', err.kind);
    default:
      return err;
  }
}

/** A Taiwan day from upstream's time: a day as is, a time at +08:00. */
function dayOf(t: { authorizedAt?: string; postedDate?: string }): string {
  const v = t.postedDate ?? t.authorizedAt ?? '';
  const d = v.length > 10 ? taipeiDay(new Date(v)) : v;
  if (!/^\d{4}-\d{2}-\d{2}$/.test(d)) throw new Error('nextbank: a transaction without a date');
  return d;
}

/**
 * Upstream's result as one batch: the main account `main`, a pocket
 * `pocket-<8 hex>`, a term-deposit pocket `term-<8 hex>` (from upstream's
 * hashed ids: the account numbers are never seen here), each with its
 * balance; a transaction not yet posted is pending with its own id.
 */
export function toBatch(r: Result, day: string): Batch {
  const accounts: Account[] = [];
  const rows: Row[] = [];
  const ours = new Map<string, Account>();
  for (const a of r.bankAccounts) {
    // bank:nextbank:<hash>:TWD, bank:nextbank:term:<hash>:TWD
    const parts = a.sourceId.split(':');
    const term = parts[2] === 'term';
    const digest = (term ? parts[3] : parts[2]) ?? '';
    const id = term ? `term-${digest.slice(0, 8)}` : /^主帳戶/.test(a.accountName ?? '') ? 'main' : `pocket-${digest.slice(0, 8)}`;
    const acc = { id, label: `將來銀行 ${a.accountName ?? id}`, currency: a.currency };
    accounts.push(acc);
    ours.set(a.sourceId, acc);
  }
  const accountOf = (sourceId: string) => {
    const a = ours.get(sourceId);
    if (!a) throw new Error('nextbank: a row for an account it did not list');
    return a;
  };
  for (const b of r.bankBalanceSnapshots) {
    const a = accountOf(b.accountId);
    rows.push({ kind: 'balance', account: a.id, id: `${a.id}:${day}:balance`, date: day, amount: toDecimal(b.balance, b.currency), currency: b.currency });
  }
  for (const t of r.bankTransactions) {
    if (t.amount === 0) continue;
    const a = accountOf(t.accountId);
    const pending = t.status === 'pending';
    const date = dayOf(t);
    rows.push({
      kind: 'transaction', account: a.id, id: `${a.id}:${date}:${short(pending ? 'pending' : 'posted', t.sourceId)}`, date,
      amount: toDecimal(t.amount, t.currency), currency: t.currency, description: t.description?.trim() || undefined, pending: pending || undefined,
    });
  }
  return { label: `將來銀行 ${day}`, accounts, rows };
}
