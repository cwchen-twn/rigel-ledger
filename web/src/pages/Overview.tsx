import { ChevronRight, CircleCheck, CircleAlert } from 'lucide-solid';
import { createMemo, createResource, createSignal, For, Show } from 'solid-js';
import { api } from '~/api/client';
import type { Account, AccountBalance, AccountClass } from '~/api/types';
import { PageHeader } from '~/components/AppShell';
import { Money } from '~/components/Money';
import { Card, CardContent, CardHeader, CardTitle } from '~/components/ui/card';
import { Checkbox, Input } from '~/components/ui/input';
import { EmptyState, Skeleton } from '~/components/ui/misc';
import { useI18n } from '~/i18n';
import { cn } from '~/lib/cn';
import { today } from '~/lib/dates';
import { add, isZero, neg } from '~/lib/money';
import { CLASS_ORDER, naturalAmount, useBook } from '~/stores/book';

/**
 * Every account's balance as of a date, rolled up the tree. This is the only
 * "trial balance" the system keeps: the check at the bottom is zero by
 * construction and shown so it can be seen to be.
 */
export default function Overview() {
  const { t } = useI18n();
  const book = useBook();
  const [asOf, setAsOf] = createSignal(today());
  const [showZero, setShowZero] = createSignal(false);
  const [collapsed, setCollapsed] = createSignal<Set<number>>(new Set());
  const [balances] = createResource(() => [book.id(), asOf()] as const, ([id, d]) => api.balances(id, d));

  const byAccount = createMemo(() => new Map((balances()?.accounts ?? []).map((b) => [b.account_id, b])));
  const base = () => balances()?.base_currency ?? book.book()?.base_currency ?? 'USD';
  const total = (cls: AccountClass) => naturalAmount(balances()?.class_totals[cls] ?? '0', cls);
  const netIncome = () => add(total('income'), neg(total('expense')));
  const netWorth = () => add(total('asset'), neg(total('liability')));

  const isEmpty = (b?: AccountBalance) => !b || (isZero(b.total_base_amount) && b.total_amounts.length === 0);
  const visibleChildren = (parent: number | null, cls: AccountClass) =>
    (book.children().get(parent) ?? []).filter(
      (a) => a.class === cls && (showZero() || !isEmpty(byAccount().get(a.id))) && (!a.archived || !isEmpty(byAccount().get(a.id))),
    );

  const toggle = (id: number) => {
    const s = new Set(collapsed());
    s.has(id) ? s.delete(id) : s.add(id);
    setCollapsed(s);
  };

  function Row(props: { account: Account; depth: number }) {
    const bal = () => byAccount().get(props.account.id);
    const kids = () => visibleChildren(props.account.id, props.account.class);
    const expanded = () => !collapsed().has(props.account.id);
    const foreign = () => (bal()?.total_amounts ?? []).filter((a) => a.commodity !== base());
    return (
      <>
        <div
          class={cn('flex items-center gap-2 border-b py-2 pr-1 text-sm last:border-b-0', props.account.is_placeholder && 'font-medium')}
          style={{ 'padding-left': `${props.depth * 1.25}rem` }}
        >
          <Show when={kids().length > 0} fallback={<span class="w-4" />}>
            <button type="button" aria-label="toggle" class="text-muted-foreground" onClick={() => toggle(props.account.id)}>
              <ChevronRight class={cn('size-4 transition-transform', expanded() && 'rotate-90')} />
            </button>
          </Show>
          <span class="min-w-0 flex-1 truncate">{book.name(props.account)}</span>
          <For each={foreign()}>
            {(a) => (
              <span class="hidden text-xs text-muted-foreground sm:inline">
                <Money amount={naturalAmount(a.amount, props.account.class)} currency={a.commodity} />
              </span>
            )}
          </For>
          <Money class="w-36 text-right" signed amount={naturalAmount(bal()?.total_base_amount ?? '0', props.account.class)} currency={base()} />
        </div>
        <Show when={expanded()}>
          <For each={kids()}>{(k) => <Row account={k} depth={props.depth + 1} />}</For>
        </Show>
      </>
    );
  }

  return (
    <>
      <PageHeader
        title={t('overview.title')}
        actions={
          <>
            <Checkbox checked={showZero()} onChange={(e) => setShowZero(e.currentTarget.checked)} label={t('overview.show_zero')} />
            <label class="flex items-center gap-2 text-sm text-muted-foreground">
              {t('overview.as_of')}
              <Input type="date" class="w-40" value={asOf()} onChange={(e) => e.currentTarget.value && setAsOf(e.currentTarget.value)} />
            </label>
          </>
        }
      />

      <div class="mb-6 grid gap-4 sm:grid-cols-3">
        <For each={[
          { label: t('overview.net_worth'), value: netWorth },
          { label: t('overview.total_assets'), value: () => total('asset') },
          { label: t('overview.total_liabilities'), value: () => total('liability') },
        ]}>
          {(kpi) => (
            <Card class="gap-1 py-4">
              <CardHeader class="px-4">
                <span class="text-sm text-muted-foreground">{kpi.label}</span>
              </CardHeader>
              <CardContent class="px-4">
                <Show when={balances()} fallback={<Skeleton class="h-7 w-32" />}>
                  <Money class="text-2xl font-semibold" signed amount={kpi.value()} currency={base()} />
                </Show>
              </CardContent>
            </Card>
          )}
        </For>
      </div>

      <Show when={balances() && book.accounts()} fallback={<Skeleton class="h-64 w-full" />}>
        <Show when={balances()!.accounts.some((a) => !isEmpty(a)) || showZero()} fallback={<EmptyState title={t('overview.empty')} />}>
          <div class="grid gap-4 lg:grid-cols-2">
            <For each={CLASS_ORDER}>
              {(cls) => (
                <Card class="gap-2 py-4">
                  <CardHeader class="flex flex-row items-center justify-between px-4">
                    <CardTitle class="text-base">{t(`class.${cls}`)}</CardTitle>
                    {/* Equity includes the result to date, so assets = liabilities + equity reads off the page. */}
                    <Money class="font-semibold" signed amount={cls === 'equity' ? add(total('equity'), netIncome()) : total(cls)} currency={base()} />
                  </CardHeader>
                  <CardContent class="px-4">
                    <For each={visibleChildren(null, cls)}>{(a) => <Row account={a} depth={0} />}</For>
                    <Show when={cls === 'equity'}>
                      <div class="flex items-center gap-2 py-2 pl-6 text-sm italic text-muted-foreground">
                        <span class="flex-1">{t('overview.net_income')}</span>
                        <Money class="w-36 text-right" signed amount={netIncome()} currency={base()} />
                      </div>
                    </Show>
                  </CardContent>
                </Card>
              )}
            </For>
          </div>
          <div class="mt-4 flex items-center justify-end gap-2 text-sm text-muted-foreground">
            <span>{t('overview.check')}:</span>
            <Show
              when={isZero(balances()!.check)}
              fallback={<span class="flex items-center gap-1 text-destructive"><CircleAlert class="size-4" />{t('overview.check_bad')} {balances()!.check}</span>}
            >
              <span class="flex items-center gap-1 text-positive"><CircleCheck class="size-4" />{t('overview.check_ok')}</span>
            </Show>
          </div>
        </Show>
      </Show>
    </>
  );
}
