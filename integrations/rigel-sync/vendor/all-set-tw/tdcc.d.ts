// What src/connectors/tw/tdcc.ts uses from the bundle, as upstream declares
// it (apps/worker/src/sources/tdcc/{protocol,epassbook-client}.ts); keep in
// step when the pinned commit moves.

export interface TdccConfig {
  userId?: string;
  password?: string;
  deviceId?: string;
  devType?: string;
  devModel?: string;
  otp?: string;
  otpChannel?: 'email' | 'sms';
  requestOtp: boolean;
  tradeHistoryMaxPages: number;
  holdings: unknown[];
  cashBalances: unknown[];
  cashMovements: unknown[];
}

/** Fills upstream's defaults (requestOtp, tradeHistoryMaxPages, the empty lists). */
export function parseTdccConfig(config: unknown): TdccConfig;

export interface EPassbookSession {
  tokenId: string | null;
  richUrl: string | null;
}

/** A holding as 集保 reports it: strings as they came. */
export interface TdccHolding {
  accountId?: string; // brokerNo:brokerAccount, or a fund's sales organisation
  brokerNo?: string;
  brokerAccount?: string;
  brokerName?: string;
  securityName: string;
  symbol?: string;
  securityType?: 'stock' | 'etf' | 'fund' | 'bond' | 'unknown';
  quantity: string | number;
  currency?: string;
  asOfDate: string; // YYYYMMDD, or a ROC date
  raw?: unknown;
}

export interface TdccStockAccount {
  brokerNo: string;
  brokerAccount: string;
  brokerName?: string;
}

/** A settlement (交割) bank account. */
export interface TdccBankEntry {
  bankId: string;
  accountNo: string;
  accountType?: string;
  currency: string;
  balanceAmt: string;
  availableBalance?: string;
}

export interface EPassbookBankTransaction {
  txnId: string;
  occurredAt: string; // YYYY-MM-DDTHH:MM:SS, Taiwan time
  amount: string;
  memo?: string;
}

export interface EPassbookClient {
  getBankTransactions(bankNo: string, acctNo: string, currency: string): Promise<{ transactions: EPassbookBankTransaction[] }>;
  exportSession(): EPassbookSession;
}

export interface TdccSnapshotInitialization {
  client: EPassbookClient;
  identity: { deviceId: string; devType: string; devModel: string; session?: EPassbookSession };
  session: EPassbookSession;
  stockAccounts: TdccStockAccount[];
  bankEntries: TdccBankEntry[];
  snapshot: { holdings: TdccHolding[] };
}

/** Signs in (or reuses the session in the cursor) and reads holdings and settlement balances. */
export function initializeTdccSnapshot(config: TdccConfig, cursor?: string, signal?: AbortSignal): Promise<TdccSnapshotInitialization>;

/** A movement in a stock account (TR002). */
export interface TdccInvestmentTransaction {
  accountId: string; // brokerNo:brokerAccount
  sourceId: string;
  brokerNo: string;
  brokerAccount: string;
  brokerName?: string;
  symbol?: string;
  name?: string;
  assetType: 'stock' | 'etf' | 'fund';
  transactionCode?: string;
  transactionName?: string;
  currency: string;
  raw: { fields: Record<string, string> };
}

/** Pages each stock account's movements, backfilling over runs (the cursor's tradeCursors). */
export function syncTdccTradeHistory(config: TdccConfig, cursor?: string): Promise<{
  investmentTransactions: TdccInvestmentTransaction[];
  cursor: string;
}>;

export class TdccVerificationRequiredError extends Error {
  readonly channel: 'email' | 'sms';
  readonly deliveryTriggered: boolean;
}
export class TdccOtpExpiredError extends Error {}
export class TdccConnectionError extends Error {
  readonly code: string;
}
