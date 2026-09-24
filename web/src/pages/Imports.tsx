import { Check, FileUp, Inbox, Trash2, WandSparkles, X } from 'lucide-solid';
import { createEffect, createMemo, createResource, createSignal, For, on, Show } from 'solid-js';
import { api } from '~/api/client';
import type { ImportRow, Proposal } from '~/api/types';
import { AccountCombobox } from '~/components/AccountCombobox';
import { PageHeader } from '~/components/AppShell';
import { CsvImportDialog } from '~/components/CsvImportDialog';
import { Money } from '~/components/Money';
import { Button } from '~/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '~/components/ui/card';
import { Dialog } from '~/components/ui/dialog';
import { Checkbox, Field, Input } from '~/components/ui/input';
import { Badge, EmptyState, Skeleton } from '~/components/ui/misc';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';
import { formatDate } from '~/lib/dates';
import { sub } from '~/lib/money';
import { useBook } from '~/stores/book';
import { useSession } from '~/stores/session';

const BADGE: Record<Proposal, 'default' | 'secondary' | 'outline' | 'warning'> = {
  new: 'default',
  duplicate: 'secondary',
  clears: 'secondary',
  transfer: 'outline',
};

/** Pattern text a rule would start from: the counterparty, or the description without trailing digits (POS numbers, dates). */
const suggestPattern = (r: ImportRow) => (r.counterparty || r.description).replace(/[\s\d#*/.-]+$/, '').trim();

/**
 * The review queue. Nothing a source sends reaches the books until it is
 * accepted here: map each source account once, then accept or ignore rows,
 * teaching rules along the way so next month's rows arrive categorised.
 */
export default function Imports() {
  const { t, te } = useI18n();
  const { user } = useSession();
  const book = useBook();
  const [queue, { refetch: refetchQueue }] = createResource(book.id, (id) => api.importQueue(id));
  const [sources, { refetch: refetchSources }] = createResource(book.id, (id) => api.importSources(id));
  const [rules, { refetch: refetchRules }] = createResource(book.id, (id) => api.importRules(id));
  const [drift, { refetch: refetchDrift }] = createResource(book.id, (id) => api.drift(id));
  const [chosen, setChosen] = createSignal<Record<number, number | null>>({});
  const [selected, setSelected] = createSignal<Set<number>>(new Set());
  const [busy, setBusy] = createSignal(false);
  const [csvOpen, setCsvOpen] = createSignal(false);
  const [ruleFor, setRuleFor] = createSignal<ImportRow | null>(null);

  const reload = () => {
    refetchQueue();
    refetchSources();
    refetchDrift();
    setSelected(new Set<number>());
  };
  const fmt = (d: string) => formatDate(d, user()?.date_format ?? 'YYYY-MM-DD');
  const accountName = (id: number | null) => {
    const a = id ? book.byId().get(id) : undefined;
    return a ? book.path(a.id) : '';
  };
  const rowsById = createMemo(() => new Map((queue() ?? []).map((r) => [r.id, r])));
  const category = (r: ImportRow) => (r.id in chosen() ? chosen()[r.id] : r.proposed_account_id);
  const acceptable = (r: ImportRow) => r.kind === 'transaction' && r.account_id !== null;
  const unmapped = () => (sources() ?? []).filter((s) => s.account_id === null);

  const toggle = (id: number, on: boolean) => {
    const s = new Set(selected());
    if (on) s.add(id);
    else s.delete(id);
    setSelected(s);
  };
  const allSelectable = () => (queue() ?? []).filter(acceptable).map((r) => r.id);

  /** Accept rows; a row whose category the person changed goes with that category. */
  const accept = async (ids: number[]) => {
    const groups = new Map<number | null, number[]>();
    for (const id of ids) {
      const r = rowsById().get(id);
      if (!r) continue;
      const override = r.id in chosen() && chosen()[r.id] !== r.proposed_account_id ? chosen()[r.id] : null;
      groups.set(override ?? null, [...(groups.get(override ?? null) ?? []), id]);
    }
    setBusy(true);
    let accepted = 0;
    let needCategory = 0;
    let other = 0;
    try {
      for (const [acct, rowIds] of groups) {
        const res = await api.acceptRows(book.id(), rowIds, acct);
        accepted += res.accepted.length;
        for (const f of res.failed) {
          if (f.code === 'required') needCategory++;
          else other++;
        }
      }
      if (accepted) toast.success(t('imports.accepted', { count: accepted }));
      if (needCategory) toast.error(t('imports.need_category', { count: needCategory }));
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
  const map = async (sourceId: number, accountId: number | null) => {
    try {
      await api.mapSource(book.id(), sourceId, accountId);
      toast.success(t('imports.mapped'));
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
    if (r.account_id === null) return t('imports.unmapped_row');
    switch (r.proposal) {
      case 'duplicate':
        return t('imports.explain_duplicate');
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
            <Button onClick={() => setCsvOpen(true)}>
              <FileUp /> {t('imports.csv_open')}
            </Button>
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
                    <div class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 text-sm">
                      <span class="font-medium">{accountName(d.account_id)}</span>
                      <span class="text-muted-foreground">
                        {t('imports.drift_line', { date: fmt(d.date) })} <Money amount={d.asserted} currency={cur()} /> ·{' '}
                        {t('imports.drift_books')} <Money amount={d.booked} currency={cur()} /> ·{' '}
                        {t('imports.drift_diff')} <Money class="text-foreground" amount={sub(d.asserted, d.booked)} currency={cur()} signed />
                      </span>
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
                        <Show when={s.account_id === null}><Badge variant="warning">{t('imports.unmapped')}</Badge></Show>
                      </div>
                      <div class="truncate text-xs text-muted-foreground">
                        {s.connector} · {s.external_id}
                        <Show when={s.pending}> · {t('imports.waiting', { count: s.pending })}</Show>
                      </div>
                    </div>
                    <AccountCombobox
                      value={s.account_id}
                      onChange={(id) => book.canEdit() && map(s.id, id)}
                      filter={(a) => a.class === 'asset' || a.class === 'liability'}
                      placeholder={t('imports.map_to')}
                    />
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
                            {r.kind === 'balance' ? t('imports.balance_title') : r.counterparty || r.description || '—'}
                          </span>
                          <Show when={r.kind === 'transaction' && r.account_id !== null}>
                            <Badge variant={BADGE[r.proposal]}>{t(`imports.proposal_${r.proposal}`)}</Badge>
                          </Show>
                          <Show when={r.pending}><Badge variant="warning">{t('imports.pending')}</Badge></Show>
                        </div>
                        <div class="truncate text-xs text-muted-foreground">
                          {[r.counterparty ? r.description : '', accountName(r.account_id) || r.source_label, explain(r)].filter(Boolean).join(' · ')}
                        </div>
                      </div>
                      <div class="min-w-0">
                        <Show when={acceptable(r) && r.proposal === 'new'}>
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
                        <Money class="text-sm font-medium" amount={r.amount} currency={r.currency} signed />
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
