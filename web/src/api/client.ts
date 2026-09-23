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
  if (res.status === 204) return undefined as R;
  const data = await res.json().catch(() => null);
  if (!res.ok) {
    const e = data?.error ?? {};
    if (res.status === 401 && path !== '/api/auth/login') onUnauthenticated();
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
  login: (username: string, password: string) => post<{ user: T.User }>('/api/auth/login', { username, password }),
  logout: () => post<void>('/api/auth/logout'),
  me: () => get<T.User>('/api/me'),
  updateSettings: (s: T.Settings) => patch<T.User>('/api/me/settings', s),
  changePassword: (current_password: string, new_password: string) =>
    post<void>('/api/me/password', { current_password, new_password }),
  updateIdentity: (username: string, email: string, current_password: string) =>
    patch<T.User>('/api/me/account', { username, email, current_password }),
  currencies: () => get<T.Currency[]>('/api/currencies'),
  commodities: () => get<T.Commodity[]>('/api/commodities'),
  createCommodity: (id: number, c: T.CommodityInput) => post<T.Commodity>(`${book(id)}/commodities`, c),

  books: () => get<T.Book[]>('/api/books'),
  createBook: (name: string, base_currency: string) => post<T.Book>('/api/books', { name, base_currency }),
  book: (id: number) => get<T.Book>(book(id)),
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

  prices: (id: number) => get<T.Price[]>(`${book(id)}/prices`),
  addPrice: (id: number, p: { commodity: string; quote: string; date: string; rate: string }) =>
    post<T.Price>(`${book(id)}/prices`, p),
  deletePrice: (id: number, priceId: number) => del(`${book(id)}/prices/${priceId}`),
  rate: (id: number, from: string, to: string, date: string) =>
    get<T.Rate>(`${book(id)}/rate${qs({ from, to, date })}`),
};
