// What src/connectors/cathaybk.ts uses from the bundle, as upstream declares
// it (apps/worker/src/sources/cathaybk/connector.ts); keep in step when the
// pinned commit moves.

import type { ScrapedAccount, ScrapedBalance, ScrapedTransaction } from './shared.js';

export type { ScrapedAccount, ScrapedBalance, ScrapedTransaction };

export interface CathaybkConfig {
  userId?: string;
  account?: string;
  password?: string;
  sessionCookies?: string;
  sessionExpiresAt?: string;
  browserSessionId?: string;
  browserSessionExpiresAt?: string;
  otp?: string;
  otpChannel?: 'email' | 'sms';
}

export interface CathaybkResult {
  records: never[];
  bankAccounts?: ScrapedAccount[];
  bankBalanceSnapshots?: ScrapedBalance[];
  bankTransactions?: ScrapedTransaction[];
  creditCardBills?: unknown[];
  /** JSON: { sessionCookies, sessionExpiresAt, syncedAt } */
  cursor?: string;
}

/** `browser` is the Workers binding upstream; here only its syncSignal is read. */
export function createCathaybkConnector(browser?: { syncSignal?: AbortSignal }): {
  id: 'cathaybk';
  name: string;
  sync(config: CathaybkConfig, cursor?: string): Promise<CathaybkResult>;
};

export class CathayVerificationRequiredError extends Error {}
export class CathayOtpChannelRequiredError extends CathayVerificationRequiredError {
  readonly browserSessionId: string;
  readonly browserSessionExpiresAt: string;
}
export class CathayOtpRequiredError extends CathayVerificationRequiredError {
  readonly channel: 'email' | 'sms';
}
export class CathayOtpInvalidError extends CathayVerificationRequiredError {}
export class CathayOtpSessionExpiredError extends CathayVerificationRequiredError {}
