import { Plus, Trash2 } from 'lucide-solid';
import { batch, createEffect, createMemo, createSignal, For, on, Show } from 'solid-js';
import { createStore, produce } from 'solid-js/store';
import { api } from '~/api/client';
import type { Account, LineInput, Transaction } from '~/api/types';
import { AccountCombobox } from '~/components/AccountCombobox';
import { Money, MoneyInput } from '~/components/Money';
import { Button } from '~/components/ui/button';
import { Sheet } from '~/components/ui/dialog';
import { Checkbox, Field, Input, Select } from '~/components/ui/input';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';
import { cn } from '~/lib/cn';
import { today } from '~/lib/dates';
import { abs, cmp, isZero, minorUnit, mul, neg, parseAmount, round, sub, sum } from '~/lib/money';
import { useBook } from '~/stores/book';
import { useSession } from '~/stores/session';

/*
 * Entry has two modes over one model:
 *
 *   Simple -- "paid from A, for B, this much", optionally "in another
 *            currency". Covers almost every daily entry, including a card
 *            purchase abroad: the original amount goes on the expense, the
 *            (estimated) charge on the card, and the charge pins the base
 *            amount so the expense ends up at the issuer's rate.
 *   Split  -- the general journal: any number of debit/credit lines, a
 *            currency per line, rates pre-filled, base amounts overridable.
 *
 * Simple always converts to split lines before saving, so there is one
 * payload shape and one balance check.
 */

interface Line {
  key: number;
  accountId: number | null;
  debit: string;
  credit: string;
  /** Only used when the account holds no commodity (income/expense). */
  commodity: string;
  /** Base-currency magnitude typed by the user; takes the amount's sign. */
  baseOverride: string;
  cleared: boolean;
  clearedOn: string | null;
  memo: string;
}

let lineKey = 0;
const emptyLine = (commodity: string): Line => ({
  key: ++lineKey, accountId: null, debit: '', credit: '', commodity, baseOverride: '', cleared: false, clearedOn: null, memo: '',
});

export function TransactionSheet(props: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The transaction to edit, or null for a new one. */
  transaction: Transaction | null;
  onSaved: () => void;
}) {
  const { t, te, fieldErrors } = useI18n();
  const { currencies, decimals } = useSession();
  const book = useBook();
  const base = () => book.book()?.base_currency ?? 'USD';

  const [mode, setMode] = createSignal<'simple' | 'split'>('simple');
  const [date, setDate] = createSignal(today());
  const [payee, setPayee] = createSignal('');
  const [memo, setMemo] = createSignal('');
  const [tags, setTags] = createSignal('');
  const [lines, setLines] = createStore<Line[]>([]);
  // Simple mode fields
  const [from, setFrom] = createSignal<number | null>(null);
  const [to, setTo] = createSignal<number | null>(null);
  const [charged, setCharged] = createSignal('');
  const [foreign, setForeign] = createSignal(false);
  const [origAmount, setOrigAmount] = createSignal('');
  const [origCurrency, setOrigCurrency] = createSignal('USD');

  const [errors, setErrors] = createSignal<Record<string, string>>({});
  const [busy, setBusy] = createSignal(false);
  const [rates, setRates] = createSignal<Record<string, string | null>>({});

  const acct = (id: number | null): Account | undefined => (id ? book.byId().get(id) : undefined);
  const lineCommodity = (l: Line) => acct(l.accountId)?.commodity ?? l.commodity;

  // Reset the form whenever the sheet opens.
  createEffect(
    on(
      () => props.open,
      (open) => {
        if (!open) return;
        const tx = props.transaction;
        batch(() => {
          setErrors({});
          setDate(tx?.date ?? today());
          setPayee(tx?.payee ?? '');
          setMemo(tx?.memo ?? '');
          setTags(tx?.tags.join(', ') ?? '');
          setFrom(null); setTo(null); setCharged(''); setForeign(false); setOrigAmount('');
          if (tx) {
            setMode('split');
            setLines(
              tx.postings.map((p) => ({
                key: ++lineKey,
                accountId: p.account_id,
                debit: cmp(p.amount, '0') > 0 ? p.amount : '',
                credit: cmp(p.amount, '0') < 0 ? neg(p.amount) : '',
                commodity: p.commodity,
                // Keep the stored base amount for foreign lines, so an edit
                // does not silently re-rate them.
                baseOverride: p.commodity !== base() ? abs(p.base_amount) : '',
                cleared: p.status !== 'uncleared',
                clearedOn: p.cleared_on,
                memo: p.memo,
              })),
            );
          } else {
            setMode('simple');
            setLines([emptyLine(base()), emptyLine(base())]);
          }
        });
      },
    ),
  );

  // ---- simple mode -> lines ----
  const fromAcct = () => acct(from());
  const toAcct = () => acct(to());
  const fromCurrency = () => fromAcct()?.commodity ?? base();
  // A transfer into an account held in another currency must say how much arrived.
  const foreignForced = () => {
    const c = toAcct()?.commodity;
    return !!c && c !== fromCurrency();
  };
  const foreignOn = () => foreign() || foreignForced();
  const toCurrency = () => toAcct()?.commodity ?? (foreignOn() ? origCurrency() : fromCurrency());

  function simpleToLines(): Line[] {
    const amt = parseAmount(charged()) ?? '';
    const orig = parseAmount(origAmount()) ?? '';
    return [
      {
        ...emptyLine(toCurrency()),
        accountId: to(),
        debit: foreignOn() ? orig : amt,
        commodity: toCurrency(),
        // The charge in base currency is the truth for what the purchase cost.
        baseOverride: foreignOn() && fromCurrency() === base() && toCurrency() !== base() ? amt : '',
      },
      { ...emptyLine(fromCurrency()), accountId: from(), credit: amt, commodity: fromCurrency() },
    ];
  }

  const switchToSplit = () => {
    if (mode() === 'simple' && (from() || to() || charged())) setLines(simpleToLines());
    setMode('split');
  };

  // ---- rates and balance ----
  const effectiveLines = createMemo(() => (mode() === 'simple' ? simpleToLines() : lines));

  const lineAmount = (l: Line): string | null => {
    const d = parseAmount(l.debit) ?? '0';
    const c = parseAmount(l.credit) ?? '0';
    const a = sub(d, c);
    return isZero(a) ? null : a;
  };

  // Fetch the rate for every foreign currency used on the date.
  createEffect(() => {
    const d = date();
    const b = base();
    const needed = new Set(effectiveLines().map(lineCommodity).filter((c) => c && c !== b));
    for (const c of needed) {
      const key = `${c}|${d}`;
      if (key in rates()) continue;
      setRates((r) => ({ ...r, [key]: null }));
      api
        .rate(book.id(), c, b, d)
        .then((res) => setRates((r) => ({ ...r, [key]: res.rate })))
        .catch(() => undefined);
    }
  });

  const rateFor = (commodity: string) => rates()[`${commodity}|${date()}`] ?? null;

  /** The line's base amount, or null when it cannot be known (no rate, no override). */
  const lineBase = (l: Line): string | null => {
    const a = lineAmount(l);
    if (!a) return null;
    const c = lineCommodity(l);
    if (c === base()) return a;
    const o = parseAmount(l.baseOverride);
    if (o) return cmp(a, '0') < 0 ? neg(abs(o)) : abs(o);
    const r = rateFor(c);
    return r ? round(mul(a, r), decimals(base())) : null;
  };

  const activeLines = () => effectiveLines().filter((l) => lineAmount(l) !== null);
  const unknownBase = () => activeLines().some((l) => lineBase(l) === null);
  const imbalance = () => sum(activeLines().map((l) => lineBase(l) ?? '0'));
  const hasForeign = () => activeLines().some((l) => lineCommodity(l) !== base());
  // The server books a residue of one minor unit to FX gains/losses.
  const balanced = () =>
    !unknownBase() &&
    (isZero(imbalance()) || (hasForeign() && cmp(abs(imbalance()), minorUnit(decimals(base()))) <= 0));

  // ---- save / delete ----
  const payload = () => ({
    date: date(),
    payee: payee(),
    memo: memo(),
    tags: tags().split(',').map((s) => s.trim()).filter(Boolean),
    lines: activeLines().map((l): LineInput => {
      const a = acct(l.accountId);
      const c = lineCommodity(l);
      const input: LineInput = { account_id: l.accountId ?? 0, amount: lineAmount(l)! };
      if (!a?.commodity) input.commodity = c;
      if (c !== base() && parseAmount(l.baseOverride)) input.base_amount = lineBase(l)!;
      if (l.cleared) {
        input.status = 'cleared';
        input.cleared_on = l.clearedOn ?? today();
      }
      if (l.memo) input.memo = l.memo;
      return input;
    }),
  });

  const save = async (e: Event) => {
    e.preventDefault();
    setBusy(true);
    setErrors({});
    try {
      if (props.transaction) await api.updateTransaction(book.id(), props.transaction.id, payload());
      else await api.createTransaction(book.id(), payload());
      toast.success(t('common.saved'));
      props.onSaved();
      props.onOpenChange(false);
    } catch (err) {
      setErrors(fieldErrors(err));
      toast.error(te(err));
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    if (!props.transaction || !confirm(t('transactions.delete_confirm'))) return;
    try {
      await api.deleteTransaction(book.id(), props.transaction.id);
      toast.success(t('common.deleted'));
      props.onSaved();
      props.onOpenChange(false);
    } catch (err) {
      toast.error(te(err));
    }
  };

  const err = (path: string) => errors()[path];

  return (
    <Sheet
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={props.transaction ? t('transactions.edit') : t('transactions.new')}
      footer={
        <>
          <Show when={props.transaction && book.canEdit()}>
            <Button variant="ghost" class="mr-auto text-destructive" onClick={remove}>
              <Trash2 /> {t('common.delete')}
            </Button>
          </Show>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>{t('common.cancel')}</Button>
          <Button type="submit" form="txn-form" disabled={busy() || !book.canEdit() || activeLines().length < 2}>
            {busy() ? t('common.saving') : t('common.save')}
          </Button>
        </>
      }
    >
      <form id="txn-form" class="grid gap-5" onSubmit={save}>
        <div class="grid gap-4 sm:grid-cols-[10rem_1fr]">
          <Field label={t('transactions.date')} error={err('date')}>
            <Input type="date" required value={date()} onChange={(e) => setDate(e.currentTarget.value)} />
          </Field>
          <Field label={t('transactions.payee')}>
            <Input placeholder={t('transactions.payee_placeholder')} value={payee()} onInput={(e) => setPayee(e.currentTarget.value)} />
          </Field>
        </div>

        <div class="inline-flex w-fit rounded-lg bg-muted p-1 text-sm">
          <For each={['simple', 'split'] as const}>
            {(m) => (
              <button
                type="button"
                class={cn('rounded-md px-3 py-1', mode() === m ? 'bg-background shadow-xs' : 'text-muted-foreground')}
                onClick={() => (m === 'split' ? switchToSplit() : setMode('simple'))}
              >
                {t(`transactions.${m}`)}
              </button>
            )}
          </For>
        </div>

        <Show when={mode() === 'simple'}>
          <div class="grid gap-4">
            <Field label={t('transactions.from')} error={err('lines[1].account_id')}>
              <AccountCombobox
                value={from()}
                onChange={setFrom}
                filter={(a) => a.class === 'asset' || a.class === 'liability'}
                invalid={!!err('lines[1].account_id')}
              />
            </Field>
            <Field label={t('transactions.to')} error={err('lines[0].account_id')}>
              <AccountCombobox value={to()} onChange={setTo} invalid={!!err('lines[0].account_id')} />
            </Field>
            <Field label={foreignOn() ? t('transactions.charged') : t('transactions.amount')} error={err('lines[1].amount')}
              hint={foreignOn() ? t('transactions.foreign_hint') : undefined}>
              <div class="flex items-center gap-2">
                <MoneyInput value={charged()} onInput={(e) => setCharged(e.currentTarget.value)} invalid={!!err('lines[1].amount')} />
                <span class="w-12 text-sm text-muted-foreground">{fromCurrency()}</span>
              </div>
            </Field>
            <Checkbox
              checked={foreignOn()}
              disabled={foreignForced()}
              onChange={(e) => setForeign(e.currentTarget.checked)}
              label={t('transactions.foreign')}
            />
            <Show when={foreignOn()}>
              <Field label={t('transactions.original_amount')} error={err('lines[0].amount') ?? err('lines[0].base_amount')}>
                <div class="flex items-center gap-2">
                  <MoneyInput value={origAmount()} onInput={(e) => setOrigAmount(e.currentTarget.value)} />
                  <Show when={!toAcct()?.commodity} fallback={<span class="w-24 text-sm text-muted-foreground">{toCurrency()}</span>}>
                    <Select class="w-24" value={origCurrency()} onChange={(e) => setOrigCurrency(e.currentTarget.value)}>
                      <For each={currencies() ?? []}>{(c) => <option value={c.code}>{c.code}</option>}</For>
                    </Select>
                  </Show>
                </div>
              </Field>
            </Show>
          </div>
        </Show>

        <Show when={mode() === 'split'}>
          <div class="grid gap-2">
            <div class="hidden grid-cols-[1fr_7rem_7rem_5rem] gap-2 px-1 text-xs font-medium text-muted-foreground sm:grid">
              <span>{t('transactions.account')}</span>
              <span class="text-right">{t('transactions.debit')}</span>
              <span class="text-right">{t('transactions.credit')}</span>
              <span>{t('transactions.currency')}</span>
            </div>
            <For each={lines}>
              {(line, i) => {
                const c = () => lineCommodity(line);
                const isForeign = () => c() !== base() && lineAmount(line) !== null;
                const set = <K extends keyof Line>(k: K, v: Line[K]) => setLines(i(), k, v);
                return (
                  <div class="grid gap-2 rounded-lg border p-2 sm:border-0 sm:p-0">
                    <div class="grid grid-cols-2 gap-2 sm:grid-cols-[1fr_7rem_7rem_5rem]">
                      <div class="col-span-2 sm:col-span-1">
                        <AccountCombobox
                          value={line.accountId}
                          onChange={(id) => set('accountId', id)}
                          invalid={!!err(`lines[${i()}].account_id`)}
                        />
                      </div>
                      <MoneyInput placeholder={t('transactions.debit')} value={line.debit}
                        onInput={(e) => { set('debit', e.currentTarget.value); if (e.currentTarget.value) set('credit', ''); }}
                        invalid={!!err(`lines[${i()}].amount`)} />
                      <MoneyInput placeholder={t('transactions.credit')} value={line.credit}
                        onInput={(e) => { set('credit', e.currentTarget.value); if (e.currentTarget.value) set('debit', ''); }}
                        invalid={!!err(`lines[${i()}].amount`)} />
                      <Show when={!acct(line.accountId)?.commodity} fallback={<span class="flex items-center text-sm text-muted-foreground">{c()}</span>}>
                        <Select value={line.commodity} onChange={(e) => set('commodity', e.currentTarget.value)}>
                          <For each={currencies() ?? []}>{(cur) => <option value={cur.code}>{cur.code}</option>}</For>
                        </Select>
                      </Show>
                    </div>
                    <div class="flex flex-wrap items-center gap-x-4 gap-y-2 px-1 text-xs text-muted-foreground">
                      <Show when={isForeign()}>
                        <label class="flex items-center gap-2" title={t('transactions.base_hint')}>
                          {t('transactions.base_amount')} ({base()})
                          <MoneyInput
                            class="h-7 w-28"
                            placeholder={lineBase({ ...line, baseOverride: '' }) ? abs(lineBase({ ...line, baseOverride: '' })!) : '—'}
                            value={line.baseOverride}
                            onInput={(e) => set('baseOverride', e.currentTarget.value)}
                            invalid={!!err(`lines[${i()}].base_amount`)}
                          />
                        </label>
                        <Show when={lineBase(line) === null}>
                          <span class="text-destructive">{t('transactions.rate_missing')}</span>
                        </Show>
                      </Show>
                      <Checkbox checked={line.cleared} onChange={(e) => set('cleared', e.currentTarget.checked)} label={t('transactions.cleared')} />
                      <input
                        class="min-w-24 flex-1 bg-transparent outline-none placeholder:text-muted-foreground/60"
                        placeholder={t('transactions.memo')}
                        value={line.memo}
                        onInput={(e) => set('memo', e.currentTarget.value)}
                      />
                      <Show when={lines.length > 2}>
                        <button type="button" aria-label={t('common.remove')} class="hover:text-destructive"
                          onClick={() => setLines(produce((ls) => ls.splice(i(), 1)))}>
                          <Trash2 class="size-4" />
                        </button>
                      </Show>
                    </div>
                  </div>
                );
              }}
            </For>
            <Button variant="outline" size="sm" class="w-fit" onClick={() => setLines(lines.length, emptyLine(base()))}>
              <Plus /> {t('transactions.add_line')}
            </Button>
          </div>
        </Show>

        <div class={cn('flex items-center justify-between rounded-lg px-3 py-2 text-sm', balanced() ? 'bg-muted' : 'bg-destructive/10 text-destructive')}>
          <span>{balanced() ? t('transactions.balanced') : t('transactions.imbalance')}</span>
          <Show when={!balanced() && !unknownBase()}>
            <Money amount={imbalance()} currency={base()} />
          </Show>
          <Show when={unknownBase()}>
            <span>{t('transactions.rate_missing')}</span>
          </Show>
        </div>
        <Show when={err('lines')}>
          <p class="text-sm text-destructive">{err('lines')}</p>
        </Show>

        <div class="grid gap-4 sm:grid-cols-2">
          <Field label={t('transactions.tags')} hint={t('transactions.tags_hint')}>
            <Input value={tags()} onInput={(e) => setTags(e.currentTarget.value)} />
          </Field>
          <Field label={t('transactions.memo')}>
            <Input value={memo()} onInput={(e) => setMemo(e.currentTarget.value)} />
          </Field>
        </div>
      </form>
    </Sheet>
  );
}
