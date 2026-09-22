import { Plus, ReceiptText, Search } from 'lucide-solid';
import { createEffect, createResource, createSignal, For, on, Show } from 'solid-js';
import { api } from '~/api/client';
import type { Transaction } from '~/api/types';
import { AccountCombobox } from '~/components/AccountCombobox';
import { PageHeader } from '~/components/AppShell';
import { Money } from '~/components/Money';
import { TransactionSheet } from '~/components/TransactionSheet';
import { Button } from '~/components/ui/button';
import { Input, Select } from '~/components/ui/input';
import { Badge, EmptyState, Skeleton } from '~/components/ui/misc';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';
import { formatDate } from '~/lib/dates';
import { cmp, sum } from '~/lib/money';
import { useBook } from '~/stores/book';
import { useSession } from '~/stores/session';

const PAGE = 50;

export default function Transactions() {
  const { t, te } = useI18n();
  const { user } = useSession();
  const book = useBook();
  const [tags] = createResource(book.id, (id) => api.tags(id));

  const [from, setFrom] = createSignal('');
  const [to, setTo] = createSignal('');
  const [account, setAccount] = createSignal<number | null>(null);
  const [q, setQ] = createSignal('');
  const [tag, setTag] = createSignal('');
  const [rows, setRows] = createSignal<Transaction[]>([]);
  const [cursor, setCursor] = createSignal('');
  const [loading, setLoading] = createSignal(true);
  const [editing, setEditing] = createSignal<Transaction | null>(null);
  const [sheetOpen, setSheetOpen] = createSignal(false);

  const filters = () => ({ from: from(), to: to(), account: account() ?? undefined, q: q(), tag: tag(), limit: PAGE });

  async function load(reset: boolean) {
    setLoading(true);
    try {
      const page = await api.transactions(book.id(), { ...filters(), cursor: reset ? undefined : cursor() });
      setRows(reset ? page.transactions : [...rows(), ...page.transactions]);
      setCursor(page.next_cursor);
    } catch (err) {
      toast.error(te(err));
    } finally {
      setLoading(false);
    }
  }

  // Reload from the top whenever a filter changes (search is debounced).
  let timer: ReturnType<typeof setTimeout> | undefined;
  createEffect(
    on([book.id, from, to, account, tag, q], () => {
      clearTimeout(timer);
      timer = setTimeout(() => load(true), 200);
    }),
  );

  const base = () => book.book()?.base_currency ?? 'USD';
  /** The size of a transaction: the sum of its debits in base currency. */
  const size = (tx: Transaction) => sum(tx.postings.filter((p) => cmp(p.base_amount, '0') > 0).map((p) => p.base_amount));
  const summary = (tx: Transaction) => {
    if (tx.postings.length !== 2) return t('transactions.lines', { count: tx.postings.length });
    const [debit, credit] = cmp(tx.postings[0].amount, '0') > 0 ? tx.postings : [tx.postings[1], tx.postings[0]];
    const name = (id: number) => {
      const a = book.byId().get(id);
      return a ? book.name(a) : `#${id}`;
    };
    return `${name(credit.account_id)} → ${name(debit.account_id)}`;
  };
  const foreignDebit = (tx: Transaction) => tx.postings.find((p) => p.commodity !== base() && cmp(p.amount, '0') > 0);
  // An uncleared card (liability) line is an estimate still waiting for the
  // issuer's figure -- the "record now, settle later" flow. Cash never clears,
  // so asset lines are not flagged.
  const pending = (tx: Transaction) =>
    tx.source !== 'opening' &&
    tx.postings.some((p) => p.status === 'uncleared' && book.byId().get(p.account_id)?.class === 'liability');

  const openNew = () => {
    setEditing(null);
    setSheetOpen(true);
  };

  return (
    <>
      <PageHeader
        title={t('transactions.title')}
        actions={
          <Show when={book.canEdit()}>
            <Button onClick={openNew}>
              <Plus /> {t('transactions.new')}
            </Button>
          </Show>
        }
      />

      <div class="mb-4 grid gap-2 sm:grid-cols-2 lg:grid-cols-[1fr_12rem_9rem_9rem_9rem]">
        <div class="relative">
          <Search class="pointer-events-none absolute top-2.5 left-3 size-4 text-muted-foreground" />
          <Input class="pl-9" placeholder={t('transactions.search_placeholder')} value={q()} onInput={(e) => setQ(e.currentTarget.value)} />
        </div>
        <AccountCombobox value={account()} onChange={setAccount} allowPlaceholders placeholder={t('transactions.filter_account')} />
        <Select value={tag()} onChange={(e) => setTag(e.currentTarget.value)} aria-label={t('transactions.filter_tag')}>
          <option value="">{t('transactions.filter_tag')}: {t('common.all')}</option>
          <For each={tags() ?? []}>{(tg) => <option value={tg}>{tg}</option>}</For>
        </Select>
        <Input type="date" aria-label={t('transactions.filter_from')} value={from()} onChange={(e) => setFrom(e.currentTarget.value)} />
        <Input type="date" aria-label={t('transactions.filter_to')} value={to()} onChange={(e) => setTo(e.currentTarget.value)} />
      </div>

      <Show when={!loading() || rows().length} fallback={<Skeleton class="h-64 w-full" />}>
        <Show when={rows().length} fallback={<EmptyState icon={<ReceiptText />} title={t('transactions.empty')} />}>
          <div class="divide-y rounded-xl border bg-card">
            <For each={rows()}>
              {(tx) => (
                <button
                  type="button"
                  class="grid w-full grid-cols-[6.5rem_1fr_auto] items-center gap-3 px-4 py-3 text-left hover:bg-muted/50"
                  onClick={() => {
                    setEditing(tx);
                    setSheetOpen(true);
                  }}
                >
                  <span class="text-sm whitespace-nowrap text-muted-foreground tabular-nums">{formatDate(tx.date, user()?.date_format ?? 'YYYY-MM-DD')}</span>
                  <span class="grid min-w-0 gap-0.5">
                    <span class="flex items-center gap-2 truncate text-sm font-medium">
                      {tx.payee || (tx.source === 'opening' ? t('transactions.opening') : summary(tx))}
                      <Show when={pending(tx)}>
                        <Badge variant="warning">{t('transactions.uncleared')}</Badge>
                      </Show>
                      <For each={tx.tags}>{(tg) => <Badge variant="outline">{tg}</Badge>}</For>
                    </span>
                    <span class="truncate text-xs text-muted-foreground">
                      {/* The title already is the summary when there is no payee. */}
                      {[tx.payee || tx.source === 'opening' ? summary(tx) : '', tx.memo].filter(Boolean).join(' · ')}
                    </span>
                  </span>
                  <span class="grid justify-items-end gap-0.5">
                    <Money class="text-sm font-medium" amount={size(tx)} currency={base()} />
                    <Show when={foreignDebit(tx)}>
                      {(p) => <Money class="text-xs text-muted-foreground" amount={p().amount} currency={p().commodity} />}
                    </Show>
                  </span>
                </button>
              )}
            </For>
          </div>
          <Show when={cursor()}>
            <div class="mt-4 flex justify-center">
              <Button variant="outline" disabled={loading()} onClick={() => load(false)}>{t('common.load_more')}</Button>
            </div>
          </Show>
        </Show>
      </Show>

      <TransactionSheet open={sheetOpen()} onOpenChange={setSheetOpen} transaction={editing()} onSaved={() => load(true)} />
    </>
  );
}
