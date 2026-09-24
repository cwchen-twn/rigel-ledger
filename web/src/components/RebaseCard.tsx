import { createSignal, For, Show } from 'solid-js';
import { api } from '~/api/client';
import type { RebasePlan } from '~/api/types';
import { Button } from '~/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '~/components/ui/card';
import { Field } from '~/components/ui/input';
import { SearchSelect } from '~/components/ui/search-select';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';
import { commodityOptions } from '~/lib/options';
import { useBook } from '~/stores/book';
import { useSession } from '~/stores/session';

/**
 * Change the book's base (functional) currency: always a check first, which
 * lists what would change and any missing rate, then the change itself.
 */
export function RebaseCard() {
  const { t, te } = useI18n();
  const { currencies } = useSession();
  const book = useBook();
  const [to, setTo] = createSignal('');
  const [plan, setPlan] = createSignal<RebasePlan | null>(null);
  const [busy, setBusy] = createSignal(false);

  const check = async () => {
    setBusy(true);
    try {
      setPlan(await api.rebase(book.id(), to(), true));
    } catch (err) {
      toast.error(te(err));
    } finally {
      setBusy(false);
    }
  };

  const apply = async () => {
    if (!confirm(t('book.rebase_confirm', { from: book.book()?.base_currency ?? '', to: to() }))) return;
    setBusy(true);
    try {
      await api.rebase(book.id(), to(), false);
      toast.success(t('book.rebased', { to: to() }));
      setPlan(null);
      book.refetchBook();
      book.refetchAccounts();
    } catch (err) {
      toast.error(te(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('book.rebase_title')}</CardTitle>
        <CardDescription>{t('book.rebase_hint', { base: book.book()?.base_currency ?? '' })}</CardDescription>
      </CardHeader>
      <CardContent class="grid gap-4">
        <div class="flex flex-wrap items-end gap-2">
          <Field label={t('book.rebase_to')} class="w-48">
            <SearchSelect
              options={commodityOptions(currencies()).filter((c) => c.value !== book.book()?.base_currency)}
              value={to()}
              onChange={(v) => { setTo(v); setPlan(null); }}
            />
          </Field>
          <Button type="button" variant="outline" disabled={busy() || !to()} onClick={check}>{t('book.rebase_check')}</Button>
        </div>
        <Show when={plan()}>
          {(p) => (
            <div class="grid gap-3 rounded-lg border bg-muted/40 p-4 text-sm">
              <p>{t('book.rebase_plan', { transactions: p().transactions, postings: p().postings, from: p().from, to: p().to })}</p>
              <Show when={p().adjusted > 0}>
                <p class="text-muted-foreground">{t('book.rebase_residue', { n: p().adjusted, amount: p().residue, to: p().to })}</p>
              </Show>
              <Show
                when={p().gaps.length === 0}
                fallback={
                  <div class="grid gap-1">
                    <p class="font-medium text-destructive">{t('book.rebase_gaps', { n: p().gaps.length })}</p>
                    <ul class="grid gap-0.5 text-xs tabular-nums">
                      <For each={p().gaps.slice(0, 12)}>{(g) => <li>{g.date} · {g.from} → {g.to}</li>}</For>
                    </ul>
                    <Show when={p().gaps.length > 12}><p class="text-xs text-muted-foreground">…</p></Show>
                  </div>
                }
              >
                <Button type="button" variant="destructive" class="justify-self-start" disabled={busy()} onClick={apply}>
                  {t('book.rebase_apply', { to: p().to })}
                </Button>
              </Show>
            </div>
          )}
        </Show>
      </CardContent>
    </Card>
  );
}
