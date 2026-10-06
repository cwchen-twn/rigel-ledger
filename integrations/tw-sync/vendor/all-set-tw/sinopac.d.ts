// What src/connectors/sinopac.ts uses from the bundle, as upstream declares
// it (apps/worker/src/sources/sinopac/{connector,protocol}.ts); keep in step
// when the pinned commit moves.

import type { ScrapedAccount, ScrapedBalance, ScrapedTransaction } from './shared.js';

export interface SinopacConfig {
  userId?: string;
  account?: string;
  password?: string;
  sessionCookies?: string;
  browserSessionId?: string;
  browserSessionExpiresAt?: string;
  captcha?: string;
  protocol?: 'sinopac-mobile-app-json-v1';
}

export interface SinopacResult {
  records: never[];
  bankAccounts?: ScrapedAccount[];
  bankBalanceSnapshots?: ScrapedBalance[];
  bankTransactions?: ScrapedTransaction[];
  creditCardBills?: unknown[];
  cardAuthorizations?: ScrapedTransaction[];
  /** JSON: { sessionCookies, protocol, syncedAt } */
  cursor?: string;
}

/** `browser` is the Workers binding upstream; here only its syncSignal is read. */
type Binding = { syncSignal?: AbortSignal };

export function createSinopacConnector(
  browser?: Binding,
  fetchImpl?: typeof fetch,
): {
  id: 'sinopac';
  name: string;
  sync(config: SinopacConfig, cursor?: string): Promise<SinopacResult>;
};

/** Opens the login page in a browser left running, and returns its CAPTCHA. */
export function prepareSinopacCaptcha(
  browser: Binding | undefined,
  config: SinopacConfig,
): Promise<{ browserSessionId: string; browserSessionExpiresAt: string; captchaImage: string }>;

/** Signs in reading each CAPTCHA with recognizeCaptcha, up to SINOPAC_AUTO_LOGIN_ATTEMPTS times. */
export function loginSinopacWithOcr(
  browser: Binding | undefined,
  config: SinopacConfig,
  recognizeCaptcha: (imageBytes: ArrayBuffer) => Promise<string>,
): Promise<{ sessionCookies: string; protocol: 'sinopac-mobile-app-json-v1' }>;

export const SINOPAC_SESSION_PROTOCOL: 'sinopac-mobile-app-json-v1';
export const SINOPAC_AUTO_LOGIN_ATTEMPTS: number;

export class SinopacVerificationRequiredError extends Error {}
export class SinopacCaptchaRejectedError extends SinopacVerificationRequiredError {}
export class SinopacCredentialRejectedError extends SinopacVerificationRequiredError {}
