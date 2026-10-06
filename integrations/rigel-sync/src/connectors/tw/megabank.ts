/*
 * 兆豐銀行 (Mega Bank, #65): deposit accounts (TWD and foreign) and the
 * credit card, from all-set-tw's connector (vendor/all-set-tw, a pinned
 * commit, unedited). It talks to the bank's mobile app API, no browser.
 *
 * Signing in takes a five-digit image CAPTCHA, read here
 * (src/ocr/captcha.ts, up to three fresh images) before the person is
 * asked, as for 永豐. A sign-in the bank finds unusual (異地登入) is then
 * checked by an SMS code, asked of the person. The app's virtual device
 * (code, key, seed) is kept in the connection's state from the first
 * CAPTCHA on: the bank then knows it, and the SMS check is needed once,
 * not every run.
 */
import {
  createMegabankConnector,
  MegabankConnectionError,
  type MegabankConfig,
  type MegabankDevice,
  MegabankOtpInvalidError,
  MegabankOtpRequiredError,
  type MegabankResult,
  MegabankVerificationRequiredError,
  prepareMegabankCaptcha,
} from '../../../vendor/all-set-tw/megabank.js';
import { toDecimal } from '../../money.ts';
import { readDigits } from '../../ocr/captcha.ts';
import { dayOf, last4, short, taipeiDay } from '../rows.ts';
import type { Account, Batch, Connector, Row, SyncContext } from '../types.ts';
import { SyncError } from '../types.ts';

const OCR_TRIES = 3;
const ASK_TRIES = 3;
// Upstream gives a session two or three minutes; a person may take longer,
// and the bank says when it is too late.
const ASK_TTL_S = 300;

/** What the connector needs from upstream; a test gives a pretend one. */
export interface MegabankApi {
  prepare(config: MegabankConfig): ReturnType<typeof prepareMegabankCaptcha>;
  sync(config: MegabankConfig): Promise<MegabankResult>;
}

const upstream = (fetcher: typeof fetch = globalThis.fetch.bind(globalThis)): MegabankApi => ({
  prepare: (config) => prepareMegabankCaptcha(config, fetcher),
  // Every run here is one the person can answer (an SMS code reaches them in Connections).
  sync: (config) => createMegabankConnector(fetcher, undefined, { allowOtpRequest: true }).sync(config),
});

const isDevice = (v: unknown): v is MegabankDevice =>
  typeof v === 'object' && v !== null && ['deviceCode', 'deviceUKey', 'deviceSeed'].every((k) => typeof (v as Record<string, unknown>)[k] === 'string');

const later = () => new Date(Date.now() + 2 * ASK_TTL_S * 1000).toISOString();

export function makeMegabank(opts: { api?: MegabankApi; read?: (jpeg: Uint8Array) => Promise<string> } = {}): Connector {
  const read = opts.read ?? readDigits;
  return {
    id: 'tw-megabank',
    name: '兆豐銀行 Mega Bank',
    country: 'TW',
    fields: [
      { name: 'id_number', label: 'ID number (身分證字號)', kind: 'id_number' },
      { name: 'user_code', label: 'User code (使用者代號)', kind: 'text' },
      { name: 'password', label: 'Password', kind: 'secret' },
    ],
    async sync(ctx) {
      const { id_number, user_code, password } = ctx.credentials;
      if (!id_number || !user_code || !password) throw new SyncError('bad_credentials');
      const api = opts.api ?? upstream();
      let device = isDevice(ctx.state.device) ? ctx.state.device : undefined;
      const keep = async (d: MegabankDevice) => {
        if (device && device.deviceCode === d.deviceCode && device.deviceUKey === d.deviceUKey && device.deviceSeed === d.deviceSeed) return;
        device = d;
        await ctx.saveState({ device: d });
      };
      const base: MegabankConfig = { userId: id_number, account: user_code, password };
      try {
        const result = await signIn(ctx, api, read, () => ({ ...base, ...device }), keep);
        return toBatch(result, taipeiDay(new Date()));
      } catch (err) {
        throw failure(err);
      }
    },
  };
}

export const megabank = makeMegabank();

/** A CAPTCHA read here (or by the person), then the SMS code if the bank asks for one. */
async function signIn(
  ctx: SyncContext,
  api: MegabankApi,
  read: (jpeg: Uint8Array) => Promise<string>,
  config: () => MegabankConfig,
  keep: (d: MegabankDevice) => Promise<void>,
): Promise<MegabankResult> {
  let ocr = 0;
  let wrong = 0;
  for (;;) {
    const p = await api.prepare(config());
    await keep(p.device);
    const image = new Uint8Array(p.imageBytes);
    let captcha: string;
    let byHand = false;
    if (ocr < OCR_TRIES) {
      ocr++;
      captcha = await read(image).catch(() => '');
      if (!/^\d{5}$/.test(captcha)) continue; // unsure: a fresh image
    } else {
      if (ocr === OCR_TRIES) ctx.log.warn('could not read the CAPTCHA here; asking');
      ocr++;
      byHand = true;
      captcha = (await ctx.ask({
        kind: 'captcha',
        prompt: wrong ? '兆豐銀行: that was not it; type the five digits in this new image' : '兆豐銀行: type the five digits in the image',
        image: Buffer.from(image),
        ttlSeconds: ASK_TTL_S,
      })).replace(/\s/g, '');
      if (!/^\d{5}$/.test(captcha)) {
        if (++wrong >= ASK_TRIES) throw new SyncError('bad_captcha');
        continue;
      }
    }
    try {
      return await api.sync({
        ...config(), captcha, pendingSession: p.pendingSession,
        pendingSessionExpiresAt: byHand ? later() : p.pendingSessionExpiresAt,
      });
    } catch (err) {
      if (err instanceof MegabankOtpRequiredError) return smsCode(ctx, api, config, keep, err);
      if (err instanceof MegabankVerificationRequiredError && /圖形驗證碼錯誤/.test(err.message)) {
        if (byHand && ++wrong >= ASK_TRIES) throw new SyncError('bad_captcha');
        continue;
      }
      throw err;
    }
  }
}

async function smsCode(
  ctx: SyncContext,
  api: MegabankApi,
  config: () => MegabankConfig,
  keep: (d: MegabankDevice) => Promise<void>,
  asked: MegabankOtpRequiredError | MegabankOtpInvalidError,
): Promise<MegabankResult> {
  let challenge = asked;
  for (let wrong = 0; ; ) {
    await keep(challenge.device);
    // Upstream's message carries the SMS's check code (簡訊檢核碼), so the person matches the text.
    const code = (await ctx.ask({
      kind: 'otp',
      prompt: wrong ? `兆豐銀行: that code was not accepted. ${challenge.message}` : `兆豐銀行: ${challenge.message}`,
      ttlSeconds: ASK_TTL_S,
    })).replace(/\s/g, '');
    try {
      return await api.sync({ ...config(), ...challenge.device, otp: code, pendingSession: challenge.pendingSession, pendingSessionExpiresAt: later() });
    } catch (err) {
      if (!(err instanceof MegabankOtpInvalidError)) throw err;
      if (++wrong >= ASK_TRIES) throw new SyncError('bad_otp');
      challenge = err;
    }
  }
}

/** A stable code for what the person can act on; anything else stays an error for the log. */
function failure(err: unknown): unknown {
  if (err instanceof SyncError) return err;
  if (err instanceof MegabankConnectionError) return new SyncError('institution_down', err.message);
  if (err instanceof MegabankVerificationRequiredError) {
    if (/逾時/.test(err.message)) return new SyncError('challenge_expired', err.message);
    // "登入未通過": a wrong ID, user code or password, or something the app must confirm.
    if (/登入未通過|身分證字號、使用者代號與登入密碼/.test(err.message)) return new SyncError('bad_credentials', err.message);
    return new SyncError('verification_failed', err.message);
  }
  return err;
}

/**
 * Upstream's result as one batch, the shape of 永豐's: a deposit account is
 * `deposit-<last4>` (with its currency when not TWD), the card `card`
 * (`card-<currency>` for a foreign one). Card rows are signed as upstream
 * signs them (refunds and payments in); a charge not yet posted is a
 * pending row with its own id, so the posted one clears it. Only deposits
 * send a balance: the card's is the statement's, not what is owed today.
 */
export function toBatch(r: MegabankResult, day: string): Batch {
  const accounts: Account[] = [];
  const rows: Row[] = [];
  const ours = new Map<string, Account>(); // upstream sourceId -> ours
  const credit = new Set<string>();

  for (const a of r.bankAccounts ?? []) {
    const isCard = a.accountType === 'credit';
    const foreign = a.currency === 'TWD' ? '' : `-${a.currency}`;
    // bank:megabank:<last4>:<hash of the number>:<currency>; megabank:credit:<currency>
    const [, , four = '', digest = ''] = a.sourceId.split(':');
    let id = isCard ? `card${foreign}` : `deposit-${last4(four)}${foreign}`;
    if (accounts.some((x) => x.id === id)) id = `deposit-${last4(four)}-${digest.slice(0, 8)}${foreign}`; // two numbers ending alike
    const label = isCard ? `兆豐信用卡${foreign ? ` ${a.currency}` : ''}` : `${a.accountName ?? '兆豐存款'}${foreign ? ` ${a.currency}` : ''} ***${last4(four)}`;
    const acc = { id, label, currency: a.currency };
    accounts.push(acc);
    ours.set(a.sourceId, acc);
    if (isCard) credit.add(a.sourceId);
  }
  const accountOf = (sourceId: string) => {
    const a = ours.get(sourceId);
    if (!a) throw new Error('megabank: a row for an account it did not list');
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
    const date = dayOf('megabank', t);
    rows.push({
      kind: 'transaction',
      account: a.id,
      id: `${a.id}:${date}:${short(pending ? 'pending' : 'posted', t.sourceId)}`,
      date,
      amount: toDecimal(t.amount, t.currency),
      currency: t.currency,
      description: t.description?.trim() || undefined,
      pending: pending || undefined,
    });
  }

  return { label: `兆豐銀行 ${day}`, accounts, rows };
}
