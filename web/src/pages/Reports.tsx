import { A, useParams, useSearchParams } from '@solidjs/router';
import { createMemo, createResource, For, Match, Show, Switch, type JSX } from 'solid-js';
import { api } from '~/api/client';
import type { AccountClass, CfClass, RateUsed, ReportLine } from '~/api/types';
import { PageHeader } from '~/components/AppShell';
import { Money } from '~/components/Money';
import { Card, CardContent, CardHeader, CardTitle } from '~/components/ui/card';
import { Field, Input } from '~/components/ui/input';
import { ErrorState, Skeleton } from '~/components/ui/misc';
import { SearchSelect } from '~/components/ui/search-select';
import { useI18n } from '~/i18n';
import { cn } from '~/lib/cn';
import { formatDate, today } from '~/lib/dates';
import { commodityOptions } from '~/lib/options';
import { useBook } from '~/stores/book';
import { useSession } from '~/stores/session';

const TABS = ['balance-sheet', 'income-statement', 'cash-flow'] as const;
type Tab = (typeof TABS)[number];

/** A row: label left, amount right; `strong` for subtotals. */
function Row(props: { label: JSX.Element; amount: string; currency: string; depth?: number; strong?: boolean; hint?: JSX.Element; signed?: boolean }) {
  return (
    <div class={cn('flex items-baseline justify-between gap-4 py-1.5 text-sm', props.strong && 'border-t font-medium')}>
      <span class="min-w-0" style={{ 'padding-left': `${(props.depth ?? 0) * 1.25}rem` }}>
        {props.label}
        <Show when={props.hint}><span class="ml-2 text-xs text-muted-foreground">{props.hint}</span></Show>
      </span>
      <Money amount={props.amount} currency={props.currency} signed={props.signed} />
    </div>
  );
}

/** The account tree of one class, from report lines keyed by account. */
function Tree(props: { lines: ReportLine[]; cls: AccountClass | AccountClass[]; currency: string; baseCurrency?: string }) {
  const { t } = useI18n();
  const book = useBook();
  const byId = createMemo(() => new Map(props.lines.map((l) => [l.account_id, l])));
  const classes = () => (Array.isArray(props.cls) ? props.cls : [props.cls]);
  const kids = (parent: number | null) =>
    (book.children().get(parent) ?? []).filter((a) => classes().includes(a.class) && byId().has(a.id));
  const Node = (p: { id: number; depth: number }): JSX.Element => {
    const acc = () => book.byId().get(p.id)!;
    const line = () => byId().get(p.id)!;
    return (
      <>
        <Row
          depth={p.depth}
          label={book.name(acc())}
          amount={line().total}
          currency={props.currency}
          hint={line().revalued && props.baseCurrency ? <>{t('reports.cost')} <Money amount={line().historical ?? '0'} currency={props.baseCurrency} /></> : undefined}
        />
        <For each={kids(p.id)}>{(k) => <Node id={k.id} depth={p.depth + 1} />}</For>
      </>
    );
  };
  return <For each={kids(null)}>{(a) => <Node id={a.id} depth={0} />}</For>;
}

function RatesFootnote(props: { rates: RateUsed[]; missing: string[]; currency: string; base: string }) {
  const { t } = useI18n();
  const { user } = useSession();
  return (
    <div class="grid gap-2 text-xs text-muted-foreground">
      <Show when={props.missing.length}>
        <p class="rounded-md bg-amber-500/10 px-3 py-2 text-amber-700 dark:text-amber-400">
          {t('reports.missing', { list: props.missing.join(', ') })}
        </p>
      </Show>
      <Show when={props.currency !== props.base}>
        <p>{t('reports.translated', { base: props.base, currency: props.currency })}</p>
      </Show>
      <Show when={props.rates.length}>
        <p class="font-medium">{t('reports.rates_used')}</p>
        <ul class="grid gap-0.5 sm:grid-cols-2">
          <For each={props.rates}>
            {(r) => (
              <li class="tabular-nums">
                1 {r.from} = {r.rate} {r.to} · {formatDate(r.date, user()?.date_format ?? 'YYYY-MM-DD')}
                <Show when={r.path === 'via USD'}> · {t('reports.via_usd')}</Show>
              </li>
            )}
          </For>
        </ul>
      </Show>
    </div>
  );
}

export default function Reports() {
  const { t, te } = useI18n();
  const { user, currencies } = useSession();
  const book = useBook();
  const params = useParams();
  const [search, setSearch] = useSearchParams();
  const tab = (): Tab => (TABS.includes(params.tab as Tab) ? (params.tab as Tab) : 'balance-sheet');
  const str = (v: string | string[] | undefined) => (typeof v === 'string' ? v : '');
  const to = () => str(search.to) || today();
  const from = () => str(search.from) || `${to().slice(0, 4)}-01-01`;
  const currency = () => str(search.currency) || user()?.display_currency || book.book()?.base_currency || 'USD';

  const [bs] = createResource(() => tab() === 'balance-sheet' && [book.id(), to(), currency()] as const, ([id, d, c]) => api.balanceSheet(id, d, c));
  const [is] = createResource(() => tab() === 'income-statement' && [book.id(), from(), to(), currency()] as const, ([id, f, d, c]) => api.incomeStatement(id, f, d, c));
  const [cf] = createResource(() => tab() === 'cash-flow' && [book.id(), from(), to(), currency()] as const, ([id, f, d, c]) => api.cashFlow(id, f, d, c));
  const current = () => (tab() === 'balance-sheet' ? bs : tab() === 'income-statement' ? is : cf);

  const cfClasses: CfClass[] = ['operating', 'investing', 'financing'];

  return (
    <>
      <PageHeader title={t('reports.title')} />
      <nav class="mb-4 flex gap-1 overflow-x-auto border-b" role="tablist" aria-label={t('reports.title')}>
        <For each={TABS}>
          {(k) => (
            <A href={`/b/${book.id()}/reports/${k}?${new URLSearchParams(Object.entries(search).filter(([, v]) => typeof v === 'string') as [string, string][])}`}
              role="tab" aria-selected={tab() === k}
              class={cn('-mb-px shrink-0 border-b-2 px-3 py-2 text-sm', tab() === k ? 'border-primary font-medium' : 'border-transparent text-muted-foreground hover:text-foreground')}>
              {t(`reports.tab_${k.replace(/-/g, '_')}`)}
            </A>
          )}
        </For>
      </nav>

      <div class="mb-6 grid gap-3 sm:grid-cols-[repeat(3,minmax(0,12rem))]">
        <Show when={tab() !== 'balance-sheet'}>
          <Field label={t('reports.from')}>
            <Input type="date" value={from()} onChange={(e) => setSearch({ from: e.currentTarget.value })} />
          </Field>
        </Show>
        <Field label={tab() === 'balance-sheet' ? t('reports.as_of') : t('reports.to')}>
          <Input type="date" value={to()} onChange={(e) => setSearch({ to: e.currentTarget.value })} />
        </Field>
        <Field label={t('reports.currency')}>
          <SearchSelect options={commodityOptions(currencies())} value={currency()} onChange={(c) => setSearch({ currency: c })} />
        </Field>
      </div>

      <Show when={current().error}>
        <ErrorState title={t('error.load_failed')} message={te(current().error)} retryLabel={t('common.retry')} onRetry={() => setSearch({ ...search })} />
      </Show>

      <Switch fallback={<Skeleton class="h-96 w-full" />}>
        <Match when={tab() === 'balance-sheet' && bs()}>
          {(r) => (
            <div class="grid gap-6 lg:grid-cols-2">
              <Card>
                <CardHeader><CardTitle>{t('class.asset')}</CardTitle></CardHeader>
                <CardContent>
                  <Tree lines={r().lines} cls="asset" currency={r().currency} baseCurrency={r().base_currency} />
                  <Row strong label={t('reports.current_assets')} amount={r().current_assets} currency={r().currency} />
                  <Row label={t('reports.non_current_assets')} amount={r().non_current_assets} currency={r().currency} />
                  <Row strong label={t('reports.total_assets')} amount={r().total_assets} currency={r().currency} />
                </CardContent>
              </Card>
              <div class="grid content-start gap-6">
                <Card>
                  <CardHeader><CardTitle>{t('class.liability')}</CardTitle></CardHeader>
                  <CardContent>
                    <Tree lines={r().lines} cls="liability" currency={r().currency} baseCurrency={r().base_currency} />
                    <Row strong label={t('reports.current_liabilities')} amount={r().current_liabilities} currency={r().currency} />
                    <Row label={t('reports.non_current_liabilities')} amount={r().non_current_liabilities} currency={r().currency} />
                    <Row strong label={t('reports.total_liabilities')} amount={r().total_liabilities} currency={r().currency} />
                  </CardContent>
                </Card>
                <Card>
                  <CardHeader><CardTitle>{t('class.equity')}</CardTitle></CardHeader>
                  <CardContent>
                    <Tree lines={r().lines} cls="equity" currency={r().currency} />
                    <Row label={t('reports.accumulated_result')} amount={r().accumulated_result} currency={r().currency} signed />
                    <Row label={t('reports.unrealised')} amount={r().unrealised} currency={r().currency} signed
                      hint={t('reports.unrealised_hint')} />
                    <Row strong label={t('reports.total_equity')} amount={r().total_equity} currency={r().currency} />
                  </CardContent>
                </Card>
              </div>
              <div class="lg:col-span-2">
                <RatesFootnote rates={r().rates_used} missing={r().missing} currency={r().currency} base={r().base_currency} />
              </div>
            </div>
          )}
        </Match>

        <Match when={tab() === 'income-statement' && is()}>
          {(r) => (
            <div class="grid gap-6">
              <Card>
                <CardContent class="pt-2">
                  <p class="pt-2 text-sm font-medium">{t('class.income')}</p>
                  <Tree lines={r().lines} cls="income" currency={r().currency} />
                  <Row strong label={t('reports.total_income')} amount={r().income} currency={r().currency} />
                  <p class="pt-4 text-sm font-medium">{t('class.expense')}</p>
                  <Tree lines={r().lines} cls="expense" currency={r().currency} />
                  <Row strong label={t('reports.total_expenses')} amount={r().expenses} currency={r().currency} />
                  <Row label={t('reports.unrealised_change')} amount={r().unrealised} currency={r().currency} signed hint={t('reports.unrealised_hint')} />
                  <Row strong label={t('reports.net_result')} amount={r().net_result} currency={r().currency} signed />
                </CardContent>
              </Card>
              <RatesFootnote rates={r().rates_used} missing={r().missing} currency={r().currency} base={r().base_currency} />
            </div>
          )}
        </Match>

        <Match when={tab() === 'cash-flow' && cf()}>
          {(r) => (
            <div class="grid gap-6">
              <Card>
                <CardContent class="pt-2">
                  <Row strong label={t('reports.opening_cash')} amount={r().opening} currency={r().currency} />
                  <For each={cfClasses}>
                    {(c) => (
                      <>
                        <p class="pt-4 text-sm font-medium">{t(`cf.${c}`)}</p>
                        <For each={r().lines.filter((l) => l.class === c)}>
                          {(l) => <Row depth={1} label={book.byId().get(l.account_id) ? book.path(l.account_id) : `#${l.account_id}`} amount={l.amount} currency={r().currency} signed />}
                        </For>
                        <Row strong label={t('reports.net_flow', { section: t(`cf.${c}`) })} amount={r()[c]} currency={r().currency} signed />
                      </>
                    )}
                  </For>
                  <Row label={t('reports.fx_effect')} amount={r().fx_effect} currency={r().currency} signed />
                  <Row strong label={t('reports.closing_cash')} amount={r().closing} currency={r().currency} />
                </CardContent>
              </Card>
              <RatesFootnote rates={r().rates_used} missing={r().missing} currency={r().currency} base={r().base_currency} />
            </div>
          )}
        </Match>
      </Switch>
    </>
  );
}
