// Mirrors internal/routes/dto.go. Money and rates are decimal strings; dates
// are "YYYY-MM-DD".

export type Role = 'viewer' | 'editor' | 'owner';
export type AccountClass = 'asset' | 'liability' | 'equity' | 'income' | 'expense';
export type CfClass = 'operating' | 'investing' | 'financing';
export type PostingStatus = 'uncleared' | 'cleared' | 'reconciled';
export type Language = 'en' | 'zh' | 'es';
export type Theme = 'system' | 'light' | 'dark';

export interface User {
  id: number;
  username: string;
  email: string;
  display_name: string;
  is_admin: boolean;
  language: Language;
  display_currency: string;
  timezone: string;
  date_format: string;
  theme: Theme;
  default_book_id: number | null;
}

export interface Currency {
  code: string;
  name: string;
  decimals: number;
}

export interface Book {
  id: number;
  name: string;
  base_currency: string;
  lock_date: string | null;
  interest_dividend_cf_class: CfClass;
  role: Role;
}

export interface Member {
  user_id: number;
  username: string;
  display_name: string;
  role: Role;
}

export interface Account {
  id: number;
  parent_id: number | null;
  class: AccountClass;
  name: string | null;
  template_key: string | null;
  code: string | null;
  commodity: string | null;
  is_current: boolean;
  is_cash: boolean;
  cf_class: CfClass;
  is_placeholder: boolean;
  archived: boolean;
}

export interface Posting {
  id: number;
  account_id: number;
  commodity: string;
  amount: string;
  base_amount: string;
  status: PostingStatus;
  cleared_on: string | null;
  memo: string;
}

export interface Transaction {
  id: number;
  date: string;
  payee: string;
  memo: string;
  source: string;
  postings: Posting[];
  tags: string[];
  created_at: string;
  updated_at: string;
}

export interface TransactionPage {
  transactions: Transaction[];
  next_cursor: string;
}

export interface LineInput {
  account_id: number;
  commodity?: string;
  amount: string;
  base_amount?: string;
  status?: PostingStatus;
  cleared_on?: string;
  memo?: string;
}

export interface TransactionInput {
  date: string;
  payee: string;
  memo: string;
  tags: string[];
  lines: LineInput[];
}

export interface CommodityAmount {
  commodity: string;
  amount: string;
}

export interface AccountBalance {
  account_id: number;
  amounts: CommodityAmount[];
  base_amount: string;
  total_amounts: CommodityAmount[];
  total_base_amount: string;
}

export interface Balances {
  as_of: string;
  base_currency: string;
  check: string;
  class_totals: Record<AccountClass, string>;
  accounts: AccountBalance[];
}

export interface Price {
  id: number;
  commodity: string;
  quote: string;
  date: string;
  rate: string;
  source: string;
}

export interface Rate {
  from: string;
  to: string;
  date: string;
  rate: string | null;
}

export interface AccountInput {
  parent_id: number | null;
  class?: AccountClass;
  name: string | null;
  code: string | null;
  commodity?: string | null;
  is_current: boolean;
  is_cash: boolean;
  cf_class: CfClass;
  is_placeholder: boolean;
  opening_balance?: { amount: string; base_amount?: string; date: string } | null;
}

export interface Settings {
  display_name: string;
  language: Language;
  display_currency: string;
  timezone: string;
  date_format: string;
  theme: Theme;
  default_book_id: number | null;
}
