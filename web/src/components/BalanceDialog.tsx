/*
 * Closing the gap against a balance (#87): the person states what an
 * account really held at the end of a day (cash counted in a wallet, a
 * balance read off a statement), sees the books beside it, and books the
 * difference as one adjustment: against an expense for cash no source saw,
 * or opening balances for history from before the books began. Opened from
 * a drift line, the stated balance is already known and only the booking is
 * left.
 */
import { batch, createEffect, createMemo, createSignal, on, Show } from 'solid-js';
import { api } from '~/api/client';
import type { Account, Balance } from '~/api/types';
import { AccountCombobox } from '~/components/AccountCombobox';
import { Money, MoneyInput } from '~/components/Money';
import { Button } from '~/components/ui/button';
import { Dialog } from '~/components/ui/dialog';
import { Field, Input } from '~/components/ui/input';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';
import { today } from '~/lib/dates';
import { isZero, parseAmount, sub } from '~/lib/money';
import { naturalAmount, useBook } from '~/stores/book';
import { useSession } from '~/stores/session';

export function BalanceDialog(props: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  account: Account | null;
  /** From a drift line: the day and both balances, signed like postings. */
  known?: { date: string; asserted: string; booked: string };
  onDone?: () => void;
}) {
  const { t, te, fieldErrors } = useI18n();
  const { kind } = useSession();
  const book = useBook();
  const [date, setDate] = createSignal(today());
  const [amount, setAmount] = createSignal('');
  const [result, setResult] = createSignal<Balance | null>(null);
  const [counter, setCounter] = createSignal<number | null>(null);
  const [errors, setErrors] = createSignal<Record<string, string>>({});
  const [busy, setBusy] = createSignal(false);

  const cur = () => props.account?.commodity ?? book.book()?.base_currency ?? 'USD';
  const natural = (x: string) => (props.account ? naturalAmount(x, props.account.class) : x);
  const diff = createMemo(() => (result() ? sub(result()!.asserted, result()!.booked) : '0'));
  const moneyOnly = () => kind(cur()) === 'currency';
  const keyed = (key: string) => book.accounts()?.find((a) => a.template_key === key)?.id ?? null;

  createEffect(
    on(
      () => props.open,
      (open) => {
        if (!open) return;
        batch(() => {
          setErrors({});
          setAmount('');
          setDate(props.known?.date ?? today());
          setResult(props.known ? { asserted: props.known.asserted, booked: props.known.booked } : null);
          setCounter(keyed(props.account?.is_cash ? 'other_expense' : 'opening_balances'));
        });
      },
    ),
  );

  const compare = async (e: Event) => {
    e.preventDefault();
    const a = props.account;
    const parsed = parseAmount(amount());
    if (!a || parsed === null) {
      setErrors({ amount: t('field.invalid') });
      return;
    }
    setBusy(true);
    setErrors({});
    try {
      setResult(await api.setBalance(book.id(), a.id, date(), naturalAmount(parsed, a.class)));
      props.onDone?.();
    } catch (err) {
      setErrors(fieldErrors(err));
      toast.error(te(err));
    } finally {
      setBusy(false);
    }
  };

  const bookIt = async () => {
    const a = props.account;
    const c = counter();
    if (!a || c === null) {
      setErrors({ counter_id: t('field.required') });
      return;
    }
    setBusy(true);
    try {
      await api.adjustBalance(book.id(), a.id, date(), c);
      toast.success(t('accounts.balance_booked'));
      props.onDone?.();
      props.onOpenChange(false);
    } catch (err) {
      setErrors(fieldErrors(err));
      toast.error(te(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={t('accounts.balance_title', { account: props.account ? book.name(props.account) : '' })}
      description={t('accounts.balance_hint')}
      footer={
        <>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>{t('common.cancel')}</Button>
          <Show
            when={result() && !isZero(diff())}
            fallback={
              <Show when={!result()}>
                <Button type="submit" form="balance-form" disabled={busy()}>{t('accounts.balance_save')}</Button>
              </Show>
            }
          >
            <Button onClick={bookIt} disabled={busy() || !moneyOnly()}>{t('accounts.balance_book')}</Button>
          </Show>
        </>
      }
    >
      <form id="balance-form" class="grid gap-4" onSubmit={compare}>
        <div class="grid gap-3 sm:grid-cols-2">
          <Field label={t('accounts.balance_date')} error={errors().date}>
            <Input type="date" value={date()} required disabled={!!result()} onChange={(e) => setDate(e.currentTarget.value)} />
          </Field>
          <Show when={!props.known}>
            <Field label={`${t('accounts.balance_amount')} (${cur()})`} error={errors().amount}>
              <MoneyInput value={amount()} disabled={!!result()} onInput={(e) => setAmount(e.currentTarget.value)} />
            </Field>
          </Show>
        </div>
      </form>
      <Show when={result()}>
        {(r) => (
          <div class="grid gap-3">
            <div class="grid gap-1 rounded-lg border p-3 text-sm">
              <div class="flex justify-between gap-4"><span class="text-muted-foreground">{t('accounts.balance_stated')}</span><Money amount={natural(r().asserted)} currency={cur()} /></div>
              <div class="flex justify-between gap-4"><span class="text-muted-foreground">{t('accounts.balance_books')}</span><Money amount={natural(r().booked)} currency={cur()} /></div>
              <div class="flex justify-between gap-4 font-medium"><span>{t('accounts.balance_difference')}</span><Money amount={natural(diff())} currency={cur()} signed /></div>
            </div>
            <Show when={!isZero(diff())} fallback={<p class="text-sm text-muted-foreground">{t('accounts.balance_agrees')}</p>}>
              <Show when={moneyOnly()} fallback={<p class="text-sm text-muted-foreground">{t('accounts.balance_money_only')}</p>}>
                <Field label={t('accounts.balance_counter')} hint={t('accounts.balance_counter_hint')} error={errors().counter_id}>
                  <AccountCombobox value={counter()} onChange={setCounter} filter={(a) => a.id !== props.account?.id} invalid={!!errors().counter_id} />
                </Field>
              </Show>
            </Show>
          </div>
        )}
      </Show>
    </Dialog>
  );
}
