import { createContext, createMemo, createResource, useContext, type Accessor, type ParentComponent, type Resource } from 'solid-js';
import { api } from '~/api/client';
import type { Account, AccountClass, Book } from '~/api/types';
import { useI18n } from '~/i18n';
import { neg } from '~/lib/money';

export const CLASS_ORDER: AccountClass[] = ['asset', 'liability', 'equity', 'income', 'expense'];

/** Credit-normal classes are shown with the sign flipped, so balances read naturally. */
export function naturalAmount(amount: string, cls: AccountClass): string {
  return cls === 'asset' || cls === 'expense' ? amount : neg(amount);
}

interface BookCtx {
  id: Accessor<number>;
  book: Resource<Book>;
  accounts: Resource<Account[]>;
  refetchAccounts: () => void;
  refetchBook: () => void;
  byId: Accessor<Map<number, Account>>;
  children: Accessor<Map<number | null, Account[]>>;
  /** The account's own name, translated while it still has its template name. */
  name: (a: Account) => string;
  /** "Expenses › Food › Groceries" */
  path: (id: number) => string;
  canEdit: Accessor<boolean>;
  isOwner: Accessor<boolean>;
}

const Ctx = createContext<BookCtx>();

export const BookProvider: ParentComponent<{ bookId: number }> = (props) => {
  const { t } = useI18n();
  const id = () => props.bookId;
  const [book, { refetch: refetchBook }] = createResource(id, (b) => api.book(b));
  const [accounts, { refetch: refetchAccounts }] = createResource(id, (b) => api.accounts(b));

  const byId = createMemo(() => new Map((accounts() ?? []).map((a) => [a.id, a])));
  const children = createMemo(() => {
    const m = new Map<number | null, Account[]>();
    for (const a of accounts() ?? []) {
      const list = m.get(a.parent_id) ?? [];
      list.push(a);
      m.set(a.parent_id, list);
    }
    return m;
  });
  const name = (a: Account) => a.name ?? (a.template_key ? t(`account.template.${a.template_key}`) : `#${a.id}`);
  const path = (accountId: number) => {
    const parts: string[] = [];
    let a = byId().get(accountId);
    while (a) {
      parts.unshift(name(a));
      a = a.parent_id ? byId().get(a.parent_id) : undefined;
    }
    return parts.join(' › ');
  };
  const canEdit = () => book()?.role === 'editor' || book()?.role === 'owner';
  const isOwner = () => book()?.role === 'owner';

  return (
    <Ctx.Provider value={{ id, book, accounts, refetchAccounts, refetchBook, byId, children, name, path, canEdit, isOwner }}>
      {props.children}
    </Ctx.Provider>
  );
};

export function useBook(): BookCtx {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error('useBook must be used within BookProvider');
  return ctx;
}
