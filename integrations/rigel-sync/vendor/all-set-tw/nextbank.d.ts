// What src/connectors/tw/nextbank.ts uses from the bundle, as upstream
// declares it (apps/worker/src/sources/nextbank/{api,protocol}.ts); keep in
// step when the pinned commit moves.

import type { ScrapedAccount, ScrapedBalance, ScrapedTransaction } from './shared.js';

export type NextbankErrorKind =
  | 'credentials' | 'captcha' | 'session_conflict' | 'session_expired'
  | 'account_unavailable' | 'rate_limit' | 'transport' | 'protocol';

export class NextbankApiError extends Error {
  readonly kind: NextbankErrorKind;
  constructor(kind: NextbankErrorKind);
}

export interface NextbankCaptcha {
  uuid: string;
  imageBase64: string; // a PNG
  expiresAt: number;
}

export class NextbankApiClient {
  constructor(options?: { fetcher?: typeof fetch; now?: () => number });
  prepareCaptcha(): Promise<NextbankCaptcha>;
  /** Consumes the CAPTCHA even when it fails; never retry a password. */
  login(credentials: { identity: string; userId: string; password: string; captchaResult: string }): Promise<{ accessToken: string }>;
  logout(accessToken: string): Promise<void>;
}

export type NextbankDepositPayloads = Record<string, unknown>;

export function collectNextbankDepositPayloads(client: NextbankApiClient, accessToken: string, now?: Date): Promise<NextbankDepositPayloads>;

export function parseNextbankDeposits(payloads: NextbankDepositPayloads, now?: Date): {
  bankAccounts: ScrapedAccount[];
  bankBalanceSnapshots: ScrapedBalance[];
  bankTransactions: ScrapedTransaction[];
};
