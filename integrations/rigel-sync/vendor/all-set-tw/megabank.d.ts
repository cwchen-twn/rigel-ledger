// What src/connectors/tw/megabank.ts uses from the bundle, as upstream
// declares it (apps/worker/src/sources/megabank/{mobile-api,protocol}.ts);
// keep in step when the pinned commit moves.

import type { ScrapedAccount, ScrapedBalance, ScrapedTransaction } from './shared.js';

export interface MegabankConfig {
  userId?: string;
  account?: string;
  password?: string;
  pendingSession?: string;
  pendingSessionExpiresAt?: string;
  captcha?: string; // five digits
  otp?: string;
  deviceCode?: string;
  deviceUKey?: string;
  deviceSeed?: string;
}

/** The fixed virtual device: kept, the bank's SMS check is needed once. */
export interface MegabankDevice {
  deviceCode: string;
  deviceUKey: string;
  deviceSeed: string;
}

export interface MegabankResult {
  records?: never[];
  bankAccounts?: ScrapedAccount[];
  bankBalanceSnapshots?: ScrapedBalance[];
  bankTransactions?: ScrapedTransaction[];
  creditCardBills?: unknown[];
  cursor?: string;
}

/** A fresh session with its CAPTCHA (a JPEG, five digits); the session lives two minutes. */
export function prepareMegabankCaptcha(
  config: MegabankConfig,
  fetcher?: typeof fetch,
): Promise<{
  captchaImage: string;
  contentType: string;
  imageBytes: ArrayBuffer;
  pendingSession: string;
  pendingSessionExpiresAt: string;
  device: MegabankDevice;
}>;

export function createMegabankConnector(
  fetcher?: typeof fetch,
  recognizeCaptcha?: (bytes: ArrayBuffer, contentType: string) => Promise<string>,
  options?: { allowOtpRequest?: boolean },
): {
  id: 'megabank';
  name: string;
  sync(config: MegabankConfig, cursor?: string): Promise<MegabankResult>;
};

export class MegabankVerificationRequiredError extends Error {}
export class MegabankOtpRequiredError extends MegabankVerificationRequiredError {
  readonly pendingSession: string;
  readonly pendingSessionExpiresAt: string;
  readonly device: MegabankDevice;
}
export class MegabankOtpInvalidError extends MegabankVerificationRequiredError {
  readonly pendingSession: string;
  readonly pendingSessionExpiresAt: string;
  readonly device: MegabankDevice;
}
export class MegabankConnectionError extends Error {}
export class MegabankProtocolError extends Error {}
