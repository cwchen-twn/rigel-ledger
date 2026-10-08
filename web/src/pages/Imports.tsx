import { Check, FileText, FileUp, Inbox, Paperclip, Trash2, WandSparkles, X } from 'lucide-solid';
import { createEffect, createMemo, createResource, createSignal, For, on, Show } from 'solid-js';
import { api } from '~/api/client';
import type { Drift, ImportRow, Proposal, SourceAccount } from '~/api/types';
import { AccountCombobox } from '~/components/AccountCombobox';
import { PageHeader } from '~/components/AppShell';
import { BalanceDialog } from '~/components/BalanceDialog';
import { CsvImportDialog } from '~/components/CsvImportDialog';
import { PdfImportDialog } from '~/components/PdfImportDialog';
import { Money, MoneyInput } from '~/components/Money';
import { Button } from '~/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '~/components/ui/card';
import { Dialog } from '~/components/ui/dialog';
import { Checkbox, Field, Input } from '~/components/ui/input';
import { Badge, EmptyState, Skeleton } from '~/components/ui/misc';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';
import { formatDate } from '~/lib/dates';
import { abs, cmp, isZero, mul, neg, parseAmount, strip, sub } from '~/lib/money';
import { useBook } from '~/stores/book';
import { useSession } from '~/stores/session';

const BADGE: Record<Proposal, 'default' | 'secondary' | 'outline' | 'warning'> = {
  new: 'default',
  duplicate: 'secondary',
  clears: 'secondary',
  transfer: 'outline',
  enrich: 'secondary',
  same_invoice: 'secondary',
  waiting: 'outline',
};

/** Pattern text a rule would start from: the counterparty, or the description without trailing digits (POS numbers, dates). */
/** YYYY-MM-DD plus n days. */
const addDays = (d: string, n: number) => new Date(Date.parse(`${d}T00:00:00Z`) + n * 86_400_000).toISOString().slice(0, 10);

const suggestPattern = (r: ImportRow) => (r.counterparty || r.description).replace(/[\s\d#*/.-]+$/, '').trim();

/**
 * The review queue. Nothing a source sends reaches the books until it is
 * accepted here: map each source account once, then accept or ignore rows,
 * teaching rules along the way so next month's rows arrive categorised.
 */
export default function Imports() {
  const { t, te, intl } = useI18n();
  const { user, refetchCommodities } = useSession();
  const book = useBook();
  const [queue, { refetch: refetchQueue }] = createResource(book.id, (id) => api.importQueue(id));
  const [sources, { refetch: refetchSources }] = createResource(book.id, (id) => api.importSources(id));
  const [rules, { refetch: refetchRules }] = createResource(book.id, (id) => api.importRules(id));
  const [drift, { refetch: refetchDrift }] = createResource(book.id, (id) => api.drift(id));
  const [chosen, setChosen] = createSignal<Record<number, number | null>>({});
  // A trade's settled cash as typed, unsigned: the trade's direction signs it.
  const [cashTyped, setCashTyped] = createSignal<Record<number, string>>({});
  // Invoices: split by item category unless the person unticks it.
  const [noSplit, setNoSplit] = createSignal<Set<number>>(new Set());
  const [selected, setSelected] = createSignal<Set<number>>(new Set());
  const [busy, setBusy] = createSignal(false);
  const [csvOpen, setCsvOpen] = createSignal(false);
  const [pdfOpen, setPdfOpen] = createSignal(false);
  const [ruleFor, setRuleFor] = createSignal<ImportRow | null>(null);
  const [adjusting, setAdjusting] = createSignal<Drift | null>(null);

  const reload = () => {
    refetchQueue();
    refetchSources();
    refetchDrift();
    // A mapping or a trade can make accounts (a security's) and register a
    // security; the drift card and the pickers need to know them.
    book.refetchAccounts();
    refetchCommodities();
    setSelected(new Set<number>());
  };
  const fmt = (d: string) => formatDate(d, user()?.date_format ?? 'YYYY-MM-DD');
  const accountName = (id: number | null) => {
    const a = id ? book.byId().get(id) : undefined;
    return a ? book.path(a.id) : '';
  };
  const rowsById = createMemo(() => new Map((queue() ?? []).map((r) => [r.id, r])));
  const category = (r: ImportRow) => (r.id in chosen() ? chosen()[r.id] : r.proposed_account_id);

  // ---- holdings and trades (#37) ----
  const symbol = (r: ImportRow) => (r.security ?? '').split(':').pop() ?? '';
  const buying = (r: ImportRow) => cmp(r.units ?? '0', '0') > 0;
  /** The cash shown for a trade: typed, else sent, else units x price. */
  const cashShown = (r: ImportRow) =>
    r.id in cashTyped() ? cashTyped()[r.id] : r.cash ? abs(r.cash) : r.price ? abs(mul(r.units ?? '0', r.price)) : '';
  const cashSigned = (r: ImportRow): string | null => {
    const v = parseAmount(cashShown(r));
    if (v === null || cmp(v, '0') <= 0) return null;
    return buying(r) ? neg(v) : v;
  };
  const tradeTitle = (r: ImportRow) =>
    `${buying(r) ? t('imports.trade_buy') : t('imports.trade_sell')} ${symbol(r)} ${r.security_name ?? ''}`.trim();
  // Shares are whole; fund units are not: as many places as the source sent.
  const units = (r: ImportRow) => new Intl.NumberFormat(intl(), { maximumFractionDigits: 8 }).format(abs(r.units ?? '0') as unknown as number);

  // ---- invoices (#38) ----
  /** An invoice matched to a payment in another currency (#56): what was charged. */
  const foreignMatch = (r: ImportRow) =>
    r.kind === 'invoice' && r.match_amount !== null && r.match_currency !== null && r.match_currency !== r.currency
      ? { amount: r.match_amount, currency: r.match_currency }
      : undefined;
  /** An email that states no amount (#57): evidence for the payment that names its seller. */
  const evidence = (r: ImportRow) => r.kind === 'invoice' && isZero(r.amount);
  /** Lines a rule gives another category: what a split would move (in the payment's currency only). */
  const splittable = (r: ImportRow) =>
    !foreignMatch(r) && (r.items ?? []).some((it) => it.account_id && it.account_id !== category(r));
  const splitting = (r: ImportRow) => splittable(r) && !noSplit().has(r.id);

  /** A trade another source of the same broker brings too (#81): settled with that one. */
  const tradeDuplicate = (r: ImportRow) => r.kind === 'trade' && r.proposal === 'duplicate';
  const acceptable = (r: ImportRow) =>
    r.kind === 'trade'
      ? tradeDuplicate(r)
        ? r.match_transaction_id !== null
        : r.account_id !== null && r.settlement_account_id !== null && cashSigned(r) !== null
      : r.kind === 'invoice'
        ? r.proposal === 'enrich' || r.proposal === 'same_invoice'
          ? r.match_transaction_id !== null
          : r.proposal === 'waiting'
            ? !evidence(r) && r.account_id !== null && chosen()[r.id] != null // booked as cash only by choice
            : r.account_id !== null
        : r.kind === 'transaction' && r.account_id !== null &&
          // the same charge as a waiting row: settled when that one is accepted
          !(r.proposal === 'duplicate' && r.match_transaction_id === null && r.match_row_id !== null);
  const sourceUnmapped = (s: SourceAccount) => !s.ignored && (s.account_id === null || (s.kind === 'brokerage' && s.settlement_account_id === null));
  const unmapped = () => (sources() ?? []).filter(sourceUnmapped);

  const toggle = (id: number, on: boolean) => {
    const s = new Set(selected());
    if (on) s.add(id);
    else s.delete(id);
    setSelected(s);
  };
  const allSelectable = () => (queue() ?? []).filter(acceptable).map((r) => r.id);

  /** Accept rows; a row whose category the person changed goes with that category. */
  const accept = async (ids: number[]) => {
    // One request per (category, split): both apply to every row it carries.
    const groups = new Map<string, { acct: number | null; split: boolean; ids: number[] }>();
    const cash: Record<number, string> = {};
    for (const id of ids) {
      const r = rowsById().get(id);
      if (!r) continue;
      if (r.kind === 'trade' && !tradeDuplicate(r)) {
        const c = cashSigned(r);
        if (c !== null) cash[id] = c;
      }
      const override = r.id in chosen() && chosen()[r.id] !== r.proposed_account_id ? chosen()[r.id] : null;
      const split = r.kind === 'invoice' && splitting(r);
      const key = `${override ?? ''}|${split}`;
      const g = groups.get(key) ?? { acct: override ?? null, split, ids: [] };
      g.ids.push(id);
      groups.set(key, g);
    }
    setBusy(true);
    let accepted = 0;
    let needCategory = 0;
    let waiting = 0;
    let other = 0;
    try {
      for (const { acct, split, ids: rowIds } of groups.values()) {
        const res = await api.acceptRows(book.id(), rowIds, acct, cash, split);
        accepted += res.accepted.length;
        for (const f of res.failed) {
          if (f.code === 'required') needCategory++;
          else if (f.code === 'match_pending') waiting++;
          else other++;
        }
      }
      if (accepted) toast.success(t('imports.accepted', { count: accepted }));
      if (needCategory) toast.error(t('imports.need_category', { count: needCategory }));
      if (waiting) toast.error(t('imports.invoice_waiting_toast', { count: waiting }));
      if (other) toast.error(t('imports.accept_failed', { count: other }));
    } catch (err) {
      toast.error(te(err));
    } finally {
      setBusy(false);
      reload();
    }
  };
  const ignore = async (ids: number[]) => {
    setBusy(true);
    try {
      await api.ignoreRows(book.id(), ids);
      toast.success(t('imports.ignored', { count: ids.length }));
    } catch (err) {
      toast.error(te(err));
    } finally {
      setBusy(false);
      reload();
    }
  };
  const map = async (sourceId: number, accountId: number | null, settlementId: number | null = null, ignored = false) => {
    try {
      await api.mapSource(book.id(), sourceId, accountId, settlementId, ignored);
      toast.success(t(ignored ? 'imports.source_ignored' : 'imports.mapped'));
    } catch (err) {
      toast.error(te(err));
    }
    reload();
  };
  const removeRule = async (id: number) => {
    try {
      await api.deleteRule(book.id(), id);
    } catch (err) {
      toast.error(te(err));
    }
    refetchRules();
  };

  const explain = (r: ImportRow) => {
    if (r.kind === 'balance') return t('imports.balance_row', { date: fmt(r.date) });
    if (r.kind === 'invoice' && r.proposal === 'enrich') {
      return r.match_transaction_id !== null ? t('imports.explain_enrich') : t('imports.explain_enrich_waiting');
    }
    if (r.kind === 'invoice' && r.proposal === 'same_invoice') {
      return r.match_transaction_id !== null ? t('imports.explain_same_invoice') : t('imports.explain_same_invoice_waiting');
    }
    if (evidence(r) && r.proposal === 'waiting') return t('imports.explain_evidence_waiting', { seller: r.counterparty });
    if (r.kind === 'invoice' && r.account_id === null) return t('imports.invoice_unmapped');
    if (r.kind === 'invoice' && r.proposal === 'waiting') {
      return t('imports.explain_waiting', { date: fmt(addDays(r.date, 7)) });
    }
    if (r.account_id === null) return t('imports.unmapped_row');
    if (tradeDuplicate(r)) {
      if (r.match_transaction_id !== null || r.match_row_id === null) return t('imports.explain_trade_duplicate');
      const other = rowsById().get(r.match_row_id);
      return t('imports.explain_trade_duplicate_row', { source: other ? `${other.connector} · ${other.source_label}` : '?' });
    }
    if (r.kind === 'trade') {
      const at = r.price ? ` @ ${strip(r.price)}` : '';
      return r.settlement_account_id === null
        ? t('imports.settlement_missing')
        : `${t('imports.trade_units', { units: units(r) })}${at} · ${t('imports.trade_cash_hint')}`;
    }
    switch (r.proposal) {
      case 'duplicate': {
        if (r.match_transaction_id !== null || r.match_row_id === null) return t('imports.explain_duplicate');
        const other = rowsById().get(r.match_row_id);
        return t('imports.explain_duplicate_row', { source: other ? `${other.connector} · ${other.counterparty || other.description}` : '?' });
      }
      case 'clears':
        return t('imports.explain_clears');
      case 'transfer': {
        const other = r.match_row_id ? rowsById().get(r.match_row_id) : undefined;
        return t('imports.explain_transfer', { account: other ? accountName(other.account_id) || other.source_label : '?' });
      }
      default:
        return r.rule_id ? t('imports.explain_rule') : '';
    }
  };

  return (
    <>
      <PageHeader
        title={t('imports.title')}
        actions={
          <Show when={book.canEdit()}>
            <div class="flex flex-wrap gap-2">
              <Button variant="outline" onClick={() => setPdfOpen(true)}>
                <FileText /> {t('imports.pdf_open_dialog')}
              </Button>
              <Button onClick={() => setCsvOpen(true)}>
                <FileUp /> {t('imports.csv_open')}
              </Button>
            </div>
          </Show>
        }
      />

      <div class="grid gap-6">
        <Show when={(drift() ?? []).length}>
          <Card class="border-amber-500/40">
            <CardHeader>
              <CardTitle>{t('imports.drift_title')}</CardTitle>
              <CardDescription>{t('imports.drift_hint')}</CardDescription>
            </CardHeader>
            <CardContent class="grid gap-2">
              <For each={drift()}>
                {(d) => {
                  const cur = () => book.byId().get(d.account_id)?.commodity ?? book.book()?.base_currency ?? 'USD';
                  return (
                    <div class="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 text-sm">
                      <div class="min-w-0">
                        <span class="font-medium">{accountName(d.account_id)}</span>
                        <div class="text-muted-foreground">
                          {t(d.source === 'manual' ? 'imports.drift_line_manual' : 'imports.drift_line', { date: fmt(d.date) })} <Money amount={d.asserted} currency={cur()} /> ·{' '}
                          {t('imports.drift_books')} <Money amount={d.booked} currency={cur()} /> ·{' '}
                          {t('imports.drift_diff')} <Money class="text-foreground" amount={sub(d.asserted, d.booked)} currency={cur()} signed />
                        </div>
                        <Show when={d.first !== d.date || d.since}>
                          <div class="text-xs text-muted-foreground">
                            {d.since ? t('imports.drift_since', { since: fmt(d.since), first: fmt(d.first) }) : t('imports.drift_never', { first: fmt(d.first) })}
                          </div>
                        </Show>
                      </div>
                      <Show when={book.canEdit()}>
                        <Button size="sm" variant="outline" onClick={() => setAdjusting(d)}>{t('imports.drift_adjust')}</Button>
                      </Show>
                    </div>
                  );
                }}
              </For>
            </CardContent>
          </Card>
        </Show>

        <Show when={(sources() ?? []).length}>
          <Card>
            <CardHeader>
              <CardTitle>{t('imports.sources_title')}</CardTitle>
              <CardDescription>{unmapped().length ? t('imports.sources_unmapped', { count: unmapped().length }) : t('imports.sources_hint')}</CardDescription>
            </CardHeader>
            <CardContent class="grid gap-3">
              <For each={sources()}>
                {(s) => (
                  <div class="grid items-center gap-2 sm:grid-cols-[1fr_20rem]" data-testid={`source-${s.external_id}`}>
                    <div class="min-w-0">
                      <div class="flex items-center gap-2 truncate text-sm font-medium">
                        {s.label || s.external_id}
                        <Show when={sourceUnmapped(s)}>
                          <Badge variant="warning">{t('imports.unmapped')}</Badge>
                        </Show>
                        <Show when={s.ignored}>
                          <Badge variant="outline">{t('imports.not_imported')}</Badge>
                        </Show>
                        <Show when={s.kind === 'brokerage'}><Badge variant="outline">{t('imports.brokerage')}</Badge></Show>
                      </div>
                      <div class="truncate text-xs text-muted-foreground">
                        {s.connector} · {s.external_id}
                        <Show when={s.pending}> · {t('imports.waiting', { count: s.pending })}</Show>
                      </div>
                    </div>
                    <Show
                      when={!s.ignored}
                      fallback={
                        <div class="flex items-center justify-between gap-2 sm:justify-end">
                          <span class="text-xs text-muted-foreground">{t('imports.not_imported_hint')}</span>
                          <Show when={book.canEdit()}>
                            <Button size="sm" variant="outline" onClick={() => map(s.id, null)}>
                              {t('imports.import_again')}
                            </Button>
                          </Show>
                        </div>
                      }
                    >
                      <div class="grid min-w-0 gap-1">
                        <Show
                          when={s.kind === 'brokerage'}
                          fallback={
                            <AccountCombobox
                              value={s.account_id}
                              onChange={(id) => book.canEdit() && map(s.id, id)}
                              filter={(a) => a.class === 'asset' || a.class === 'liability'}
                              placeholder={t('imports.map_to')}
                            />
                          }
                        >
                          <div class="grid min-w-0 gap-2">
                            <AccountCombobox
                              value={s.account_id}
                              onChange={(id) => book.canEdit() && map(s.id, id, s.settlement_account_id)}
                              filter={(a) => a.class === 'asset'}
                              allowPlaceholders
                              placeholder={t('imports.securities_under')}
                              aria-label={t('imports.securities_under')}
                            />
                            <AccountCombobox
                              value={s.settlement_account_id}
                              onChange={(id) => book.canEdit() && map(s.id, s.account_id, id)}
                              filter={(a) => a.class === 'asset'}
                              placeholder={t('imports.settles_through')}
                              aria-label={t('imports.settles_through')}
                            />
                          </div>
                        </Show>
                        <Show when={book.canEdit()}>
                          <Button
                            size="sm"
                            variant="ghost"
                            class="justify-self-end text-muted-foreground"
                            title={t('imports.dont_import_hint')}
                            onClick={() => map(s.id, null, null, true)}
                          >
                            {t('imports.dont_import')}
                          </Button>
                        </Show>
                      </div>
                    </Show>
                  </div>
                )}
              </For>
            </CardContent>
          </Card>
        </Show>

        <Show when={!queue.loading || queue()} fallback={<Skeleton class="h-64 w-full" />}>
          <Show
            when={(queue() ?? []).length}
            fallback={
              <EmptyState icon={<Inbox />} title={t('imports.empty')}>
                <p class="text-sm text-muted-foreground">{t('imports.empty_hint')}</p>
              </EmptyState>
            }
          >
            <div class="grid gap-3">
              <Show when={book.canEdit()}>
                <div class="flex flex-wrap items-center gap-3">
                  <Checkbox
                    checked={selected().size > 0 && selected().size === allSelectable().length}
                    onChange={(e) => setSelected(new Set(e.currentTarget.checked ? allSelectable() : []))}
                    label={t('imports.select_all')}
                  />
                  <Button size="sm" disabled={busy() || !selected().size} onClick={() => accept([...selected()])}>
                    <Check /> {t('imports.accept_selected', { count: selected().size })}
                  </Button>
                  <Button size="sm" variant="outline" disabled={busy() || !selected().size} onClick={() => ignore([...selected()])}>
                    <X /> {t('imports.ignore_selected')}
                  </Button>
                </div>
              </Show>
              <div class="divide-y rounded-xl border bg-card">
                <For each={queue()}>
                  {(r) => (
                    <div class="grid gap-2 px-4 py-3 lg:grid-cols-[1.5rem_6.5rem_minmax(0,1fr)_auto_auto] lg:items-center" data-testid={`row-${r.external_id}`}>
                      <input
                        type="checkbox"
                        class="hidden size-4 accent-primary lg:block"
                        aria-label={t('imports.select_row')}
                        disabled={!acceptable(r) || !book.canEdit()}
                        checked={selected().has(r.id)}
                        onChange={(e) => toggle(r.id, e.currentTarget.checked)}
                      />
                      <span class="text-sm whitespace-nowrap text-muted-foreground tabular-nums">{fmt(r.date)}</span>
                      <div class="grid min-w-0 gap-0.5">
                        <div class="flex items-center gap-2">
                          <span class="truncate text-sm font-medium">
                            {r.kind === 'balance'
                              ? t('imports.balance_title')
                              : r.kind === 'holding'
                                ? `${t('imports.holding_title')} ${symbol(r)} ${r.security_name ?? ''}`
                                : r.kind === 'trade'
                                  ? tradeTitle(r)
                                  : r.counterparty || r.description || '—'}
                          </span>
                          <Show when={(r.kind === 'transaction' && r.account_id !== null) || r.kind === 'invoice' || tradeDuplicate(r)}>
                            <Badge variant={BADGE[r.proposal]}>
                              {r.kind === 'invoice' && r.proposal === 'new'
                                ? t('imports.proposal_cash')
                                : r.proposal === 'duplicate' && r.match_transaction_id === null && r.match_row_id !== null
                                  ? t(r.kind === 'trade' ? 'imports.proposal_same_trade' : 'imports.proposal_same_charge')
                                  : t(`imports.proposal_${r.proposal}`)}
                            </Badge>
                          </Show>
                          <Show when={r.pending}><Badge variant="warning">{t('imports.pending')}</Badge></Show>
                          <Show when={r.attachment_id}>
                            {(id) => (
                              <a
                                href={api.fileUrl(book.id(), id())}
                                target="_blank"
                                rel="noopener"
                                class="shrink-0 text-muted-foreground hover:text-foreground"
                                aria-label={t('imports.evidence')}
                                title={t('imports.evidence')}
                              >
                                <Paperclip class="size-3.5" />
                              </a>
                            )}
                          </Show>
                        </div>
                        <div class="truncate text-xs text-muted-foreground">
                          {[r.counterparty ? r.description : '', accountName(r.account_id) || r.source_label, explain(r)].filter(Boolean).join(' · ')}
                        </div>
                        <Show when={r.kind === 'invoice' && r.items?.length}>
                          <details class="text-xs text-muted-foreground">
                            <summary class="w-fit cursor-pointer select-none hover:text-foreground">
                              {t('imports.invoice_items', { count: r.items!.length })}
                            </summary>
                            <ul class="mt-1 grid gap-0.5">
                              <For each={r.items}>
                                {(it) => (
                                  <li class="flex min-w-0 items-baseline justify-between gap-3">
                                    <span class="min-w-0 truncate">
                                      {it.description}
                                      <Show when={it.account_id}>{(acct) => <span class="text-foreground"> → {accountName(acct())}</span>}</Show>
                                    </span>
                                    <Money class="shrink-0" amount={it.amount} currency={r.currency} />
                                  </li>
                                )}
                              </For>
                            </ul>
                          </details>
                          <Show when={splittable(r) && book.canEdit()}>
                            <Checkbox
                              checked={splitting(r)}
                              onChange={(e) => {
                                const s = new Set(noSplit());
                                if (e.currentTarget.checked) s.delete(r.id);
                                else s.add(r.id);
                                setNoSplit(s);
                              }}
                              label={t('imports.split_by_items')}
                            />
                          </Show>
                        </Show>
                      </div>
                      <div class="min-w-0">
                        <Show
                          when={
                            (r.kind === 'transaction' || r.kind === 'invoice') && r.account_id !== null && !evidence(r) &&
                            (r.proposal === 'new' || (r.kind === 'invoice' && r.proposal === 'waiting'))
                          }
                        >
                          <div class="flex items-center gap-1 lg:w-72">
                            <AccountCombobox
                              class="min-w-0 flex-1"
                              value={category(r)}
                              onChange={(id) => setChosen({ ...chosen(), [r.id]: id })}
                              placeholder={t('imports.choose_category')}
                            />
                            <Show when={book.canEdit()}>
                              <Button size="icon" variant="ghost" aria-label={t('imports.make_rule')} title={t('imports.make_rule')} onClick={() => setRuleFor(r)}>
                                <WandSparkles />
                              </Button>
                            </Show>
                          </div>
                        </Show>
                      </div>
                      <div class="flex items-center justify-between gap-2 lg:justify-end">
                        <Show
                          when={r.kind === 'trade' && !tradeDuplicate(r)}
                          fallback={
                            <Show
                              when={r.kind === 'holding' || tradeDuplicate(r)}
                              fallback={
                                <span class="flex flex-col items-end">
                                  <Show when={!evidence(r)} fallback={<span class="text-sm text-muted-foreground">{t('imports.amount_not_stated')}</span>}>
                                    <Money class="text-sm font-medium" amount={r.amount} currency={r.currency} signed />
                                  </Show>
                                  <Show when={foreignMatch(r)}>
                                    {(m) => (
                                      <span class="text-xs text-muted-foreground" title={t('imports.charged_as_hint')}>
                                        {t('imports.charged_as')} <Money amount={m().amount} currency={m().currency} signed />
                                      </span>
                                    )}
                                  </Show>
                                </span>
                              }
                            >
                              <span class="text-sm font-medium tabular-nums">{t('imports.trade_units', { units: units(r) })}</span>
                            </Show>
                          }
                        >
                          <label class="flex items-center gap-2 text-xs text-muted-foreground">
                            {buying(r) ? t('imports.cash_paid') : t('imports.cash_received')}
                            <MoneyInput
                              class="w-32"
                              value={cashShown(r)}
                              disabled={!book.canEdit()}
                              onInput={(e) => setCashTyped({ ...cashTyped(), [r.id]: e.currentTarget.value })}
                            />
                            <span>{r.currency}</span>
                          </label>
                        </Show>
                        <Show when={book.canEdit()}>
                          <div class="flex gap-1">
                            <Button size="sm" disabled={busy() || !acceptable(r)} onClick={() => accept([r.id])} aria-label={t('imports.accept')}>
                              <Check /> <span class="sm:inline">{t('imports.accept')}</span>
                            </Button>
                            <Button size="sm" variant="ghost" disabled={busy()} onClick={() => ignore([r.id])} aria-label={t('imports.ignore')} title={t('imports.ignore')}>
                              <X />
                            </Button>
                          </div>
                        </Show>
                      </div>
                    </div>
                  )}
                </For>
              </div>
            </div>
          </Show>
        </Show>

        <Show when={(rules() ?? []).length}>
          <Card>
            <CardHeader>
              <CardTitle>{t('imports.rules_title')}</CardTitle>
              <CardDescription>{t('imports.rules_hint')}</CardDescription>
            </CardHeader>
            <CardContent class="grid gap-1">
              <For each={rules()}>
                {(x) => (
                  <div class="flex items-center justify-between gap-2 text-sm">
                    <span class="min-w-0 truncate">
                      <span class="font-mono">“{x.pattern}”</span> → {accountName(x.account_id)}
                      <Show when={x.source_account_id}>
                        {(sid) => <span class="text-muted-foreground"> · {sources()?.find((s) => s.id === sid())?.label}</span>}
                      </Show>
                    </span>
                    <Show when={book.canEdit()}>
                      <Button size="icon" variant="ghost" aria-label={t('common.delete')} onClick={() => removeRule(x.id)}>
                        <Trash2 />
                      </Button>
                    </Show>
                  </div>
                )}
              </For>
            </CardContent>
          </Card>
        </Show>
      </div>

      <CsvImportDialog open={csvOpen()} onOpenChange={setCsvOpen} onImported={reload} />
      <BalanceDialog
        open={!!adjusting()}
        onOpenChange={(o) => !o && setAdjusting(null)}
        account={adjusting() ? book.byId().get(adjusting()!.account_id) ?? null : null}
        known={adjusting() ?? undefined}
        onDone={reload}
      />
      <PdfImportDialog open={pdfOpen()} onOpenChange={setPdfOpen} onImported={reload} />
      <RuleDialog
        row={ruleFor()}
        category={ruleFor() ? category(ruleFor()!) : null}
        onClose={() => setRuleFor(null)}
        onSaved={() => {
          refetchRules();
          refetchQueue();
          setChosen({});
        }}
      />
    </>
  );
}

function RuleDialog(props: { row: ImportRow | null; category: number | null; onClose: () => void; onSaved: () => void }) {
  const { t, te, fieldErrors } = useI18n();
  const book = useBook();
  const [pattern, setPattern] = createSignal('');
  const [account, setAccount] = createSignal<number | null>(null);
  const [onlySource, setOnlySource] = createSignal(false);
  const [errors, setErrors] = createSignal<Record<string, string>>({});

  // Fill the form each time it opens for a row.
  createEffect(
    on(
      () => props.row,
      (r) => {
        if (!r) return;
        setPattern(suggestPattern(r));
        setAccount(props.category);
        setOnlySource(false);
        setErrors({});
      },
    ),
  );

  const save = async (e: Event) => {
    e.preventDefault();
    const r = props.row;
    if (!r || !account()) return;
    try {
      await api.createRule(book.id(), { pattern: pattern(), account_id: account()!, source_account_id: onlySource() ? r.source_account_id : null });
      toast.success(t('imports.rule_saved'));
      props.onSaved();
      props.onClose();
    } catch (err) {
      setErrors(fieldErrors(err));
      toast.error(te(err));
    }
  };

  return (
    <Dialog
      open={!!props.row}
      onOpenChange={(o) => !o && props.onClose()}
      title={t('imports.make_rule')}
      description={t('imports.rule_hint')}
    >
      <form class="grid gap-4" onSubmit={save}>
        <Field label={t('imports.rule_pattern')} error={errors().pattern}>
          <Input required value={pattern()} onInput={(e) => setPattern(e.currentTarget.value)} />
        </Field>
        <Field label={t('imports.rule_account')} error={errors().account_id}>
          <AccountCombobox value={account()} onChange={setAccount} />
        </Field>
        <Checkbox checked={onlySource()} onChange={(e) => setOnlySource(e.currentTarget.checked)} label={t('imports.rule_only_source', { source: props.row?.source_label ?? '' })} />
        <div class="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
          <Button type="button" variant="outline" onClick={props.onClose}>{t('common.cancel')}</Button>
          <Button type="submit" disabled={!pattern().trim() || !account()}>{t('common.save')}</Button>
        </div>
      </form>
    </Dialog>
  );
}
