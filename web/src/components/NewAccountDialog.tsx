import { createEffect, createSignal, For, Show } from 'solid-js';
import { api } from '~/api/client';
import type { AccountClass, AccountInput, CfClass } from '~/api/types';
import { MoneyInput } from '~/components/Money';
import { Button } from '~/components/ui/button';
import { Dialog } from '~/components/ui/dialog';
import { Field, Input, Select } from '~/components/ui/input';
import { SearchSelect } from '~/components/ui/search-select';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';
import { today } from '~/lib/dates';
import { commodityOptions } from '~/lib/options';
import { parseAmount } from '~/lib/money';
import { useBook } from '~/stores/book';
import { useSession } from '~/stores/session';

/** The everyday kinds of account, each with where it lives in the chart. */
export const QUICK_KINDS = ['bank', 'card', 'cash', 'ewallet', 'broker', 'loan'] as const;
export type QuickKind = (typeof QUICK_KINDS)[number];

const SHAPE: Record<QuickKind, { parent: string | null; cls: AccountClass; cash: boolean; current: boolean; cf: CfClass }> = {
  bank: { parent: 'bank', cls: 'asset', cash: true, current: true, cf: 'operating' },
  card: { parent: 'credit_cards', cls: 'liability', cash: false, current: true, cf: 'operating' },
  cash: { parent: null, cls: 'asset', cash: true, current: true, cf: 'operating' },
  ewallet: { parent: null, cls: 'asset', cash: true, current: true, cf: 'operating' },
  broker: { parent: 'investments', cls: 'asset', cash: true, current: true, cf: 'operating' },
  loan: { parent: 'loans', cls: 'liability', cash: false, current: false, cf: 'financing' },
};

/**
 * "Add an account" in three fields: a name, what kind it is, its currency.
 * The kind decides the rest (parent, class, cash, cash-flow class), which is
 * what the full form asks for and what a household should not have to know.
 */
export function NewAccountDialog(props: { open: boolean; onOpenChange: (o: boolean) => void; kind?: QuickKind; onCreated?: () => void }) {
  const { t, te, fieldErrors } = useI18n();
  const { currencies } = useSession();
  const book = useBook();
  const [name, setName] = createSignal('');
  const [kind, setKind] = createSignal<QuickKind>('bank');
  const [currency, setCurrency] = createSignal('');
  const [opening, setOpening] = createSignal('');
  const [errors, setErrors] = createSignal<Record<string, string>>({});
  const [busy, setBusy] = createSignal(false);

  createEffect(() => {
    if (!props.open) return;
    setKind(props.kind ?? 'bank');
    setName('');
    setOpening('');
    setErrors({});
    setCurrency(book.book()?.base_currency ?? 'USD');
  });

  const submit = async (e: Event, again: boolean) => {
    e.preventDefault();
    const shape = SHAPE[kind()];
    const parent = shape.parent ? book.accounts()?.find((a) => a.template_key === shape.parent) : undefined;
    const amount = parseAmount(opening());
    const input: AccountInput = {
      parent_id: parent?.id ?? null,
      class: shape.cls,
      name: name(),
      code: null,
      commodity: currency(),
      is_current: shape.current,
      is_cash: shape.cash,
      cf_class: shape.cf,
      is_placeholder: false,
      // A card or loan balance is what is owed: entered positive, booked as a credit.
      opening_balance: amount ? { amount: shape.cls === 'liability' ? `-${amount}` : amount, date: today() } : null,
    };
    setBusy(true);
    try {
      await api.createAccount(book.id(), input);
      toast.success(t('accounts.quick_added', { name: name() }));
      book.refetchAccounts();
      props.onCreated?.();
      if (again) {
        setName('');
        setOpening('');
        setErrors({});
      } else {
        props.onOpenChange(false);
      }
    } catch (err) {
      setErrors(fieldErrors(err));
      toast.error(te(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange} title={t('accounts.quick_title')} description={t('accounts.quick_hint')}>
      <form class="grid gap-4" onSubmit={(e) => submit(e, false)}>
        <Field label={t('accounts.quick_kind')}>
          <Select value={kind()} onChange={(e) => setKind(e.currentTarget.value as QuickKind)}>
            <For each={QUICK_KINDS}>{(k) => <option value={k}>{t(`accounts.kind_${k}`)}</option>}</For>
          </Select>
        </Field>
        <Field label={t('accounts.name')} hint={t(`accounts.kind_${kind()}_example`)} error={errors().name}>
          <Input required autofocus value={name()} onInput={(e) => setName(e.currentTarget.value)} />
        </Field>
        <div class="grid gap-4 sm:grid-cols-2">
          <Field label={t('accounts.commodity')} hint={t('accounts.quick_currency_hint')} error={errors().commodity}>
            <SearchSelect options={commodityOptions(currencies())} value={currency()} onChange={setCurrency} />
          </Field>
          <Field label={kind() === 'card' || kind() === 'loan' ? t('accounts.quick_owed') : t('accounts.quick_balance')}
            hint={t('accounts.quick_balance_hint')} error={errors()['opening_balance.amount']}>
            <MoneyInput placeholder={t('common.optional')} value={opening()} onInput={(e) => setOpening(e.currentTarget.value)} />
          </Field>
        </div>
        <Show when={errors().parent_id}><p class="text-sm text-destructive">{errors().parent_id}</p></Show>
        <div class="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
          <Button type="button" variant="outline" disabled={busy() || !name().trim()} onClick={(e) => submit(e, true)}>
            {t('accounts.quick_save_another')}
          </Button>
          <Button type="submit" disabled={busy() || !name().trim()}>{t('common.save')}</Button>
        </div>
      </form>
    </Dialog>
  );
}
