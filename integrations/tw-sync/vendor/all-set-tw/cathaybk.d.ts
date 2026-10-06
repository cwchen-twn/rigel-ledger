// What src/connectors/cathaybk.ts uses from the bundle, as upstream declares
// it (apps/worker/src/sources/cathaybk/connector.ts); keep in step when the
// pinned commit moves.

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

export interface ScrapedAccount {
  sourceId: string;
  institutionName?: string;
  accountName?: string;
  accountType?: 'checking' | 'savings' | 'credit' | 'loan' | 'settlement_cash' | 'time_deposit' | 'stored_value' | 'unknown';
  currency: string;
  creditLimit?: number;
  raw?: unknown;
}

export interface ScrapedBalance {
  accountId: string;
  sourceId: string;
  balance: number;
  availableBalance?: number;
  currency: string;
  asOfAt: string;
  raw?: unknown;
}

export interface ScrapedTransaction {
  accountId: string;
  sourceId: string;
  postedDate?: string;
  authorizedAt?: string;
  amount: number;
  currency: string;
  description?: string;
  counterparty?: string;
  raw?: unknown;
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
