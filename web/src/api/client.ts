import type * as T from './types';

/** An API failure: code is stable and translated as error.<code>. */
export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public fields: Record<string, string> = {},
  ) {
    super(message || code);
  }
}

/**
 * The bundle this tab is running, from its own <script> URL (content-hashed,
 * e.g. main-CWqCP0iw.js). The server names the current one in X-App-Build.
 */
const loadedBuild = (() => {
  const src = document.querySelector<HTMLScriptElement>('script[src^="/static/dist/"]')?.getAttribute('src') ?? '';
  return src.split('/').pop() ?? '';
})();

let onNewBuild: () => void = () => {};
/** Called once when the server reports a newer frontend than this tab runs. */
export function setNewBuildHandler(fn: () => void): void {
  onNewBuild = fn;
}
let newBuildSeen = false;

/** Called on any 401 so the app can send the user to /login. */
let onUnauthenticated: () => void = () => {};
export function setUnauthenticatedHandler(fn: () => void): void {
  onUnauthenticated = fn;
}

async function request<R>(method: string, path: string, body?: unknown): Promise<R> {
  const headers: Record<string, string> = { Accept: 'application/json' };
  if (method !== 'GET') {
    // Required on every cookie-authenticated write: the server's CSRF check.
    headers['X-Rigel-Client'] = 'web';
  }
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  const res = await fetch(path, {
    method,
    headers,
    credentials: 'same-origin',
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const build = res.headers.get('X-App-Build');
  if (build && loadedBuild && build !== loadedBuild && !newBuildSeen) {
    newBuildSeen = true;
    onNewBuild();
  }
  if (res.status === 204) return undefined as R;
  const data = await res.json().catch(() => null);
  if (!res.ok) {
    const e = data?.error ?? {};
    // A 401 mid-session means the session ended. Signed-out pages expect their
    // own 401s, and the session probe (/api/me) handles its answer itself --
    // otherwise an anonymous visit to /invite/... would bounce to /login.
    if (res.status === 401 && !path.startsWith('/api/auth/') && path !== '/api/me') onUnauthenticated();
    throw new ApiError(res.status, e.code ?? 'internal', e.message ?? res.statusText, e.fields ?? {});
  }
  return data as R;
}

const get = <R>(p: string) => request<R>('GET', p);
const post = <R>(p: string, b?: unknown) => request<R>('POST', p, b ?? {});
const patch = <R>(p: string, b: unknown) => request<R>('PATCH', p, b);
const put = <R>(p: string, b: unknown) => request<R>('PUT', p, b);
const del = (p: string) => request<void>('DELETE', p);

function qs(params: Record<string, string | number | undefined | null>): string {
  const u = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== null && v !== '') u.set(k, String(v));
  const s = u.toString();
  return s ? `?${s}` : '';
}

const book = (id: number) => `/api/books/${id}`;

export const api = {
  login: (username: string, password: string) => post<T.LoginResult>('/api/auth/login', { username, password }),
  verifyMFA: (challenge: string, method: T.SignInMethod, code: string) => post<T.LoginResult>('/api/auth/mfa', { challenge, method, code }),
  sendSignInCode: (challenge: string) => post<void>('/api/auth/mfa/email', { challenge }),
  passkeyLoginBegin: (challenge = '') => post<{ options: Record<string, unknown>; challenge: string }>('/api/auth/passkey/begin', { challenge }),
  passkeyLoginFinish: (challenge: string, credential: unknown) =>
    post<T.LoginResult>(`/api/auth/passkey/finish${qs({ challenge })}`, credential),

  mfa: {
    status: () => get<T.MFAStatus>('/api/me/mfa'),
    startTOTP: () => post<T.TOTPSetup>('/api/me/mfa/totp'),
    confirmTOTP: (code: string) => post<T.Enrolled>('/api/me/mfa/totp/confirm', { code }),
    removeTOTP: (current_password: string) => request<void>('DELETE', '/api/me/mfa/totp', { current_password }),
    startEmail: () => post<{ challenge: string }>('/api/me/mfa/email'),
    confirmEmail: (challenge: string, code: string) => post<T.Enrolled>('/api/me/mfa/email/confirm', { challenge, code }),
    removeEmail: (current_password: string) => request<void>('DELETE', '/api/me/mfa/email', { current_password }),
    passkeyBegin: (name: string) => post<{ options: Record<string, unknown>; challenge: string }>('/api/me/mfa/passkeys/begin', { name }),
    passkeyFinish: (challenge: string, credential: unknown) =>
      post<T.Enrolled>(`/api/me/mfa/passkeys/finish${qs({ challenge })}`, credential),
    removePasskey: (id: number, current_password: string) => request<void>('DELETE', `/api/me/mfa/passkeys/${id}`, { current_password }),
    regenerateRecovery: (current_password: string) => post<T.Enrolled>('/api/me/mfa/recovery-codes', { current_password }),
    setAlerts: (enabled: boolean) => patch<void>('/api/me/mfa/alerts', { enabled }),
  },
  logout: () => post<void>('/api/auth/logout'),
  me: () => get<T.User>('/api/me'),
  updateSettings: (s: T.Settings) => patch<T.User>('/api/me/settings', s),
  changePassword: (current_password: string, new_password: string) =>
    post<void>('/api/me/password', { current_password, new_password }),
  updateUsername: (username: string, current_password: string) =>
    patch<T.User>('/api/me/account', { username, current_password }),

  // Signed-out flows.
  authConfig: () => get<T.AuthConfig>('/api/auth/config'),
  register: (r: { username: string; email: string; password: string; language: string }) => post<void>('/api/auth/register', r),
  verifyLink: (token: string) => post<{ user: T.User }>('/api/auth/verify-link', { token }),
  requestAccess: (r: { username: string; email: string; message: string }) => post<void>('/api/auth/request-access', r),
  invitation: (token: string) => get<T.Invitation>(`/api/auth/invite/${encodeURIComponent(token)}`),
  acceptInvite: (token: string, p: T.Profile & { password: string }) =>
    post<{ user: T.User }>(`/api/auth/invite/${encodeURIComponent(token)}`, p),

  // Email verification, the first-login wizard, sessions.
  startEmail: (email: string, current_password = '') => post<void>('/api/me/email', { email, current_password }),
  confirmEmail: (code: string) => post<T.User>('/api/me/email/confirm', { code }),
  cancelEmail: () => del('/api/me/email/pending'),
  onboarding: (p: T.Profile & { new_password?: string }) => post<T.User>('/api/me/onboarding', p),
  sessions: () => get<T.SessionInfo[]>('/api/me/sessions'),
  revokeSession: (id: number) => del(`/api/me/sessions/${id}`),
  createToken: (label: string, days: number) => post<T.TokenCreated>('/api/me/tokens', { label, days }),

  /** Credentials and answers go out sealed (lib/seal.ts): the server cannot read them. */
  connectors: () => get<T.Catalog>('/api/connectors'),
  connections: () => get<T.Connection[]>('/api/me/connections'),
  createConnection: (c: { book_id: number; connector: string; label: string; interval_hours: number; key_id: number; sealed: string }) =>
    post<{ id: number }>('/api/me/connections', c),
  updateConnection: (id: number, c: { book_id?: number; label?: string; enabled?: boolean; interval_hours?: number }) =>
    patch<void>(`/api/me/connections/${id}`, c),
  replaceCredentials: (id: number, key_id: number, sealed: string) => put<void>(`/api/me/connections/${id}/credentials`, { key_id, sealed }),
  syncConnection: (id: number) => post<void>(`/api/me/connections/${id}/sync`),
  deleteConnection: (id: number) => del(`/api/me/connections/${id}`),
  answerChallenge: (id: number, challengeId: number, sealed: string) =>
    post<void>(`/api/me/connections/${id}/challenges/${challengeId}`, { sealed }),
  myEvents: () => get<T.AuthEvent[]>('/api/me/events'),

  // Administration.
  admin: {
    settings: () => get<T.SystemSettings>('/api/admin/settings'),
    updateSettings: (s: T.SystemSettingsInput) => patch<T.SystemSettings>('/api/admin/settings', s),
    updateMail: (m: T.MailSettingsInput) => patch<T.SystemSettings>('/api/admin/mail', m),
    testMail: () => post<{ driver: T.MailDriver; to: string }>('/api/admin/mail/test'),
    users: () => get<T.AdminUser[]>('/api/admin/users'),
    updateUser: (id: number, c: { is_active?: boolean; is_admin?: boolean }) => patch<T.AdminUser>(`/api/admin/users/${id}`, c),
    invite: (username: string, email: string) => post<T.AdminUser>('/api/admin/invitations', { username, email }),
    resendInvite: (id: number) => post<void>(`/api/admin/invitations/${id}/resend`),
    revokeInvite: (id: number) => del(`/api/admin/invitations/${id}`),
    accessRequests: (status?: string) => get<T.AccessRequest[]>(`/api/admin/access-requests${qs({ status })}`),
    approve: (id: number) => post<void>(`/api/admin/access-requests/${id}/approve`),
    reject: (id: number) => post<void>(`/api/admin/access-requests/${id}/reject`),
    events: (limit = 100) => get<T.AuthEvent[]>(`/api/admin/events${qs({ limit })}`),
    resetMFA: (id: number) => post<void>(`/api/admin/users/${id}/reset-mfa`),
    rates: () => get<T.RateStatus>('/api/admin/rates'),
    refreshRates: (date?: string) => post<T.RateFetch>('/api/admin/rates/refresh', date ? { date } : {}),
    runner: () => get<T.RunnerStatus>('/api/admin/runner'),
    createRunnerToken: (label: string, days: number) => post<T.TokenCreated>('/api/admin/runner/tokens', { label, days }),
    revokeRunnerToken: (id: number) => del(`/api/admin/runner/tokens/${id}`),
  },
  currencies: () => get<T.Currency[]>('/api/currencies'),
  commodities: () => get<T.Commodity[]>('/api/commodities'),
  createCommodity: (id: number, c: T.CommodityInput) => post<T.Commodity>(`${book(id)}/commodities`, c),

  books: () => get<T.Book[]>('/api/books'),
  createBook: (name: string, base_currency: string) => post<T.Book>('/api/books', { name, base_currency }),
  book: (id: number) => get<T.Book>(book(id)),
  deleteBook: (id: number, confirm: string) => request<void>('DELETE', book(id), { confirm }),
  updateBook: (id: number, b: { name: string; lock_date: string | null; interest_dividend_cf_class: T.CfClass }) =>
    patch<T.Book>(book(id), b),

  members: (id: number) => get<T.Member[]>(`${book(id)}/members`),
  addMember: (id: number, username: string, role: T.Role) => post<void>(`${book(id)}/members`, { username, role }),
  updateMember: (id: number, userId: number, role: T.Role) => patch<void>(`${book(id)}/members/${userId}`, { role }),
  removeMember: (id: number, userId: number) => del(`${book(id)}/members/${userId}`),

  accounts: (id: number) => get<T.Account[]>(`${book(id)}/accounts`),
  createAccount: (id: number, a: T.AccountInput) => post<T.Account>(`${book(id)}/accounts`, a),
  updateAccount: (id: number, accountId: number, a: T.AccountInput) =>
    patch<T.Account>(`${book(id)}/accounts/${accountId}`, {
      parent_id: a.parent_id, name: a.name, code: a.code, is_current: a.is_current,
      is_cash: a.is_cash, cf_class: a.cf_class, is_placeholder: a.is_placeholder,
    }),
  archiveAccount: (id: number, accountId: number, archived: boolean) =>
    post<T.Account>(`${book(id)}/accounts/${accountId}/archive`, { archived }),
  deleteAccount: (id: number, accountId: number) => del(`${book(id)}/accounts/${accountId}`),
  costBasis: (id: number, accountId: number, asOf: string, exclude?: number) =>
    get<T.CostBasis>(`${book(id)}/accounts/${accountId}/cost${qs({ as_of: asOf, exclude })}`),

  transactions: (id: number, f: { from?: string; to?: string; account?: number; q?: string; tag?: string; cursor?: string; limit?: number }) =>
    get<T.TransactionPage>(`${book(id)}/transactions${qs(f)}`),
  createTransaction: (id: number, t: T.TransactionInput) => post<T.Transaction>(`${book(id)}/transactions`, t),
  updateTransaction: (id: number, txnId: number, t: T.TransactionInput) =>
    put<T.Transaction>(`${book(id)}/transactions/${txnId}`, t),
  deleteTransaction: (id: number, txnId: number) => del(`${book(id)}/transactions/${txnId}`),
  tags: (id: number) => get<string[]>(`${book(id)}/tags`),

  balances: (id: number, asOf: string) => get<T.Balances>(`${book(id)}/balances${qs({ as_of: asOf })}`),

  /** Manual rates only: the daily snapshots would bury them. */
  prices: (id: number) => get<T.Price[]>(`${book(id)}/prices${qs({ source: 'manual' })}`),
  currentRates: (id: number) => get<{ currency: string; rate: string | null }[]>(`${book(id)}/rates/current`),
  addPrice: (id: number, p: { commodity: string; quote: string; date: string; rate: string }) =>
    post<T.Price>(`${book(id)}/prices`, p),
  deletePrice: (id: number, priceId: number) => del(`${book(id)}/prices/${priceId}`),
  balanceSheet: (id: number, as_of: string, currency: string) =>
    get<T.BalanceSheet>(`${book(id)}/reports/balance-sheet${qs({ as_of, currency })}`),
  incomeStatement: (id: number, from: string, to: string, currency: string) =>
    get<T.IncomeStatement>(`${book(id)}/reports/income-statement${qs({ from, to, currency })}`),
  cashFlow: (id: number, from: string, to: string, currency: string) =>
    get<T.CashFlow>(`${book(id)}/reports/cash-flow${qs({ from, to, currency })}`),
  tagReport: (id: number, currency: string) => get<T.TagReport>(`${book(id)}/reports/tags${qs({ currency })}`),
  tagDetail: (id: number, name: string, currency: string) => get<T.TagDetail>(`${book(id)}/reports/tag${qs({ name, currency })}`),
  rebase: (id: number, base_currency: string, dry_run: boolean) =>
    post<T.RebasePlan>(`${book(id)}/rebase`, { base_currency, dry_run }),
  rate: (id: number, from: string, to: string, date: string) =>
    get<T.Rate>(`${book(id)}/rate${qs({ from, to, date })}`),

  importBatch: (id: number, b: T.ImportBatch) => post<T.ImportResult>(`${book(id)}/imports`, b),
  importSources: (id: number) => get<T.SourceAccount[]>(`${book(id)}/imports/sources`),
  mapSource: (id: number, sourceId: number, account_id: number | null) =>
    patch<void>(`${book(id)}/imports/sources/${sourceId}`, { account_id }),
  importQueue: (id: number) => get<T.ImportRow[]>(`${book(id)}/imports/queue`),
  acceptRows: (id: number, row_ids: number[], account_id: number | null = null) =>
    post<T.AcceptResult>(`${book(id)}/imports/accept`, { row_ids, account_id }),
  ignoreRows: (id: number, row_ids: number[]) => post<void>(`${book(id)}/imports/ignore`, { row_ids }),
  importRules: (id: number) => get<T.ImportRule[]>(`${book(id)}/imports/rules`),
  createRule: (id: number, r: { pattern: string; account_id: number; source_account_id: number | null }) =>
    post<T.ImportRule>(`${book(id)}/imports/rules`, r),
  deleteRule: (id: number, ruleId: number) => del(`${book(id)}/imports/rules/${ruleId}`),
  drift: (id: number) => get<T.Drift[]>(`${book(id)}/drift`),
};
