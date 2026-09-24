import { RefreshCw } from 'lucide-solid';
import { createResource, createSignal, For, Show } from 'solid-js';
import { api } from '~/api/client';
import { Button } from '~/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '~/components/ui/card';
import { Field, Input } from '~/components/ui/input';
import { Badge, EmptyState, Table, tdClass, thClass, trClass } from '~/components/ui/misc';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';
import { formatDateTime } from '~/lib/dates';

/** The daily exchange-rate fetch: what it got, and a manual run. */
export function Rates() {
  const { t, te } = useI18n();
  const [status, { refetch }] = createResource(() => api.admin.rates());
  const [day, setDay] = createSignal('');
  const [busy, setBusy] = createSignal(false);

  const refresh = async (date?: string) => {
    setBusy(true);
    try {
      const r = await api.admin.refreshRates(date);
      toast.success(t('admin.rates_stored', { n: r.rates, date: r.rate_date ?? '', source: r.source }));
    } catch (err) {
      toast.error(te(err));
    } finally {
      setBusy(false);
      refetch();
    }
  };

  return (
    <div class="grid gap-6">
      <Card>
        <CardHeader>
          <CardTitle>{t('admin.rates_title')}</CardTitle>
          <CardDescription>
            {t('admin.rates_hint')}{' '}
            <a href="https://www.exchangerate-api.com" target="_blank" rel="noopener noreferrer" class="underline underline-offset-4">
              Rates By Exchange Rate API
            </a>
          </CardDescription>
        </CardHeader>
        <CardContent class="grid gap-4">
          <Show when={status() && !status()!.scheduler}>
            <p class="text-sm text-muted-foreground">{t('admin.rates_off')}</p>
          </Show>
          <div class="flex flex-wrap items-end gap-3">
            <Button type="button" disabled={busy()} onClick={() => refresh()}><RefreshCw /> {t('admin.rates_refresh')}</Button>
            <Field label={t('admin.rates_backfill')}>
              <Input type="date" value={day()} onInput={(e) => setDay(e.currentTarget.value)} />
            </Field>
            <Button type="button" variant="outline" disabled={busy() || !day()} onClick={() => refresh(day())}>{t('admin.rates_fetch_day')}</Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader><CardTitle>{t('admin.rates_recent')}</CardTitle></CardHeader>
        <CardContent>
          <Show when={(status()?.recent ?? []).length} fallback={<EmptyState title={t('admin.rates_none')} />}>
            <Table>
              <thead>
                <tr class="border-b">
                  <th class={thClass}>{t('security.when')}</th>
                  <th class={thClass}>{t('book.source')}</th>
                  <th class={thClass}>{t('admin.rates_day')}</th>
                  <th class={`${thClass} text-right`}>{t('admin.rates_count')}</th>
                  <th class={thClass}>{t('admin.status')}</th>
                </tr>
              </thead>
              <tbody>
                <For each={status()?.recent ?? []}>
                  {(f) => (
                    <tr class={trClass}>
                      <td class={`${tdClass} whitespace-nowrap`}>{formatDateTime(f.fetched_at)}</td>
                      <td class={tdClass}><Badge variant="outline">{f.source}</Badge></td>
                      <td class={`${tdClass} tabular-nums`}>{f.rate_date ?? f.requested ?? t('admin.rates_latest')}</td>
                      <td class={`${tdClass} text-right tabular-nums`}>{f.error ? '—' : f.rates}{f.skipped ? ` (+${f.skipped} ${t('admin.rates_locked')})` : ''}</td>
                      <td class={`${tdClass} max-w-xs truncate`} title={f.error}>
                        <Show when={f.error} fallback={<Badge>{t('admin.rates_ok')}</Badge>}>
                          <Badge variant="warning">{f.error}</Badge>
                        </Show>
                      </td>
                    </tr>
                  )}
                </For>
              </tbody>
            </Table>
          </Show>
        </CardContent>
      </Card>
    </div>
  );
}
