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
  email_verified: boolean;
  /** False until the first-login wizard is done; most of the API answers 403 onboarding_required. */
  initialized: boolean;
  password_must_change: boolean;
  /** An address a code was sent to and not yet confirmed (GET /api/me only). */
  pending_email?: string;
  /** GET /api/me only: 1 = password only, 2 = passed a second factor. */
  session_aal?: number;
  mfa_required?: boolean;
  mfa_enrolled?: boolean;
}

export type SignInMethod = 'totp' | 'email' | 'passkey' | 'recovery';

/** Either signed in (user) or a second step to take (challenge + methods). */
export interface LoginResult {
  user?: User;
  token?: string;
  mfa_required?: boolean;
  challenge?: string;
  methods?: SignInMethod[];
}

export interface PasskeyInfo {
  id: number;
  name: string;
  created_at: string;
  last_used_at: string | null;
}

export interface MFAStatus {
  totp: boolean;
  email: boolean;
  passkeys: PasskeyInfo[];
  recovery_left: number;
  allowed: MfaMethod[];
  required: boolean;
  signin_alerts: boolean;
  session_aal: number;
}

export interface TOTPSetup {
  secret: string;
  uri: string;
  qr: string;
}

/** Recovery codes come back once, when the first factor is added. */
export interface Enrolled {
  recovery_codes?: string[];
}

export type Registration = 'closed' | 'request' | 'open';
export type MfaMethod = 'email' | 'totp' | 'passkey';

export interface AuthConfig {
  registration: Registration;
}

/** What the wizard and the invitation page collect. */
export interface Profile {
  username: string;
  display_name: string;
  language: Language;
  display_currency: string;
  timezone: string;
  date_format: string;
  theme: Theme;
}

export interface Invitation extends Profile {
  email: string;
}

export interface SessionInfo {
  id: number;
  kind: 'web' | 'api';
  user_agent: string;
  ip: string;
  created_at: string;
  last_used_at: string;
  expires_at: string;
  current: boolean;
}

export interface AuthEvent {
  id: number;
  username: string;
  user_id: number | null;
  event: string;
  failure: boolean;
  ip: string;
  user_agent: string;
  detail: Record<string, unknown>;
  created_at: string;
}

export interface SystemSettingsInput {
  registration: Registration;
  mfa_required: boolean;
  mfa_methods: MfaMethod[];
  default_language: Language;
  default_display_currency: string;
  default_timezone: string;
  default_date_format: string;
  default_theme: Theme;
  /** null: SESSION_TTL from the environment. */
  session_ttl_seconds: number | null;
  invite_ttl_seconds: number;
  login_max_failures: number;
  login_ip_max_failures: number;
  login_user_max_failures: number;
  login_window_seconds: number;
}

export type MailDriver = 'smtp' | 'log' | 'off';
export type SmtpSecurity = 'starttls' | 'tls' | 'none';

export interface MailSettingsInput {
  mail_driver: MailDriver;
  smtp_host: string;
  smtp_port: number;
  smtp_security: SmtpSecurity;
  smtp_user: string;
  /** Empty or absent keeps the stored password. */
  smtp_password?: string;
  clear_smtp_password?: boolean;
  mail_from: string;
  mail_from_name: string;
}

export interface SystemSettings extends SystemSettingsInput, Omit<MailSettingsInput, 'smtp_password' | 'clear_smtp_password'> {
  mail_configured: boolean;
  smtp_password_set: boolean;
  updated_at: string;
}

export interface AdminUser {
  id: number;
  username: string;
  email: string;
  display_name: string;
  is_admin: boolean;
  is_active: boolean;
  email_verified: boolean;
  initialized: boolean;
  invite_pending: boolean;
  /** Has not accepted the invitation yet. */
  invited: boolean;
  last_login_at: string | null;
  created_at: string;
}

export interface AccessRequest {
  id: number;
  username: string;
  email: string;
  message: string;
  ip: string;
  status: 'pending' | 'approved' | 'rejected';
  decided_at: string | null;
  created_at: string;
}

export interface Currency {
  code: string;
  name: string;
  decimals: number;
}

export type CommodityKind = 'currency' | 'security' | 'points';

/** Anything an account can hold. Securities carry a quote currency; points carry nothing (valued at cost). */
export interface Commodity extends Currency {
  kind: CommodityKind;
  quote_currency: string | null;
  exchange_mic: string | null;
  contract_size: string | null;
}

export interface CommodityInput {
  code: string;
  kind: 'security' | 'points';
  name: string;
  decimals?: number;
  quote_currency?: string | null;
  contract_size?: string | null;
}

export interface CostBasis {
  quantity: string;
  cost: string;
  unit_cost: string;
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
  unit_cost: string | null;
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
  unit_cost?: string;
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

export interface RateFetch {
  id: number;
  source: string;
  rate_date: string | null;
  requested: string | null;
  rates: number;
  skipped: number;
  error: string;
  fetched_at: string;
}

export interface RateStatus {
  scheduler: boolean;
  recent: RateFetch[];
}

export interface ReportLine {
  account_id: number;
  amount: string;
  total: string;
  historical?: string;
  holdings?: { commodity: string; amount: string }[];
  revalued?: boolean;
}

export interface RateUsed {
  from: string;
  to: string;
  rate: string;
  date: string;
  path: string;
}

interface ReportBase {
  base_currency: string;
  currency: string;
  rates_used: RateUsed[];
  missing: string[];
}

export interface BalanceSheet extends ReportBase {
  as_of: string;
  lines: ReportLine[];
  current_assets: string;
  non_current_assets: string;
  total_assets: string;
  current_liabilities: string;
  non_current_liabilities: string;
  total_liabilities: string;
  equity_accounts: string;
  accumulated_result: string;
  unrealised: string;
  total_equity: string;
}

export interface IncomeStatement extends ReportBase {
  from: string;
  to: string;
  lines: ReportLine[];
  income: string;
  expenses: string;
  unrealised: string;
  net_result: string;
}

export interface CashFlow extends ReportBase {
  from: string;
  to: string;
  opening: string;
  lines: { class: CfClass; account_id: number; amount: string }[];
  operating: string;
  investing: string;
  financing: string;
  fx_effect: string;
  closing: string;
}
