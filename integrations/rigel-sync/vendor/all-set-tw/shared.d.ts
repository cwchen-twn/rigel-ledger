// Upstream's scraped rows (@taiwan-fin-hub/shared, less id and connectorId),
// as far as the wrappers in src/connectors read them; keep in step when the
// pinned commit moves.

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
  status?: 'pending' | 'posted';
  raw?: unknown;
}
