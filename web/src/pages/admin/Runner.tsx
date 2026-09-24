import { Copy, KeyRound, Trash2 } from 'lucide-solid';
import { createResource, createSignal, For, Show } from 'solid-js';
import { api } from '~/api/client';
import { Button } from '~/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '~/components/ui/card';
import { Dialog } from '~/components/ui/dialog';
import { Field, Input } from '~/components/ui/input';
import { Badge, EmptyState, Table, tdClass, thClass, trClass } from '~/components/ui/misc';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';
import { formatDateTime } from '~/lib/dates';

/**
 * The sync runner: its token (made here, put in the runner's secret), the
 * keys it registered and the connectors it offers. People link their own
 * institutions under Connections; the runner does the rest.
 */
export function Runner() {
  const { t, te, fieldErrors } = useI18n();
  const [status, { refetch }] = createResource(() => api.admin.runner());
  const [open, setOpen] = createSignal(false);
  const [label, setLabel] = createSignal('');
  const [made, setMade] = createSignal('');
  const [errors, setErrors] = createSignal<Record<string, string>>({});

  const create = async (e: Event) => {
    e.preventDefault();
    try {
      const res = await api.admin.createRunnerToken(label().trim(), 365);
      setMade(res.token);
      refetch();
    } catch (err) {
      setErrors(fieldErrors(err));
      toast.error(te(err));
    }
  };
  const revoke = async (id: number) => {
    try {
      await api.admin.revokeRunnerToken(id);
      toast.success(t('tokens.revoked'));
    } catch (err) {
      toast.error(te(err));
    }
    refetch();
  };
  const fingerprint = (b64: string) => b64.slice(0, 16) + '…';

  return (
    <div class="grid gap-6">
      <Card>
        <CardHeader>
          <div class="flex items-start justify-between gap-4">
            <div class="grid gap-1">
              <CardTitle>{t('runner.tokens_title')}</CardTitle>
              <CardDescription>{t('runner.tokens_hint')}</CardDescription>
            </div>
            <Button size="sm" variant="outline" onClick={() => { setLabel('sync runner'); setMade(''); setErrors({}); setOpen(true); }}>
              <KeyRound /> {t('tokens.new')}
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <Show when={(status()?.tokens ?? []).length} fallback={<p class="text-sm text-muted-foreground">{t('runner.no_tokens')}</p>}>
            <Table>
              <thead>
                <tr class="border-b">
                  <th class={thClass}>{t('tokens.label')}</th>
                  <th class={thClass}>{t('runner.created_by')}</th>
                  <th class={thClass}>{t('security.last_used')}</th>
                  <th class={thClass}>{t('tokens.expires')}</th>
                  <th class={thClass}><span class="sr-only">{t('common.actions')}</span></th>
                </tr>
              </thead>
              <tbody>
                <For each={status()?.tokens}>
                  {(x) => (
                    <tr class={trClass}>
                      <td class={`${tdClass} font-medium`}>{x.label}</td>
                      <td class={tdClass}>{x.created_by}</td>
                      <td class={`${tdClass} whitespace-nowrap`}>{x.last_used_at === x.created_at ? t('tokens.never_used') : formatDateTime(x.last_used_at)}</td>
                      <td class={`${tdClass} whitespace-nowrap`}>{formatDateTime(x.expires_at)}</td>
                      <td class={`${tdClass} text-right`}>
                        <Button size="sm" variant="ghost" onClick={() => revoke(x.id)}><Trash2 /> {t('tokens.revoke')}</Button>
                      </td>
                    </tr>
                  )}
                </For>
              </tbody>
            </Table>
          </Show>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t('runner.keys_title')}</CardTitle>
          <CardDescription>{t('runner.keys_hint')}</CardDescription>
        </CardHeader>
        <CardContent class="grid gap-2">
          <Show when={(status()?.keys ?? []).length} fallback={<p class="text-sm text-muted-foreground">{t('runner.no_keys')}</p>}>
            <For each={status()?.keys}>
              {(k, i) => (
                <div class="flex flex-wrap items-center gap-2 text-sm">
                  <code class="font-mono text-xs">{fingerprint(k.public_key)}</code>
                  <Show when={!k.retired_at && i() === 0}><Badge>{t('runner.key_current')}</Badge></Show>
                  <Show when={k.retired_at}><Badge variant="outline">{t('runner.key_retired')}</Badge></Show>
                  <span class="text-muted-foreground">{formatDateTime(k.created_at)}</span>
                </div>
              )}
            </For>
          </Show>
        </CardContent>
      </Card>

      <Card>
        <CardHeader><CardTitle>{t('runner.connectors_title')}</CardTitle></CardHeader>
        <CardContent>
          <Show when={(status()?.connectors ?? []).length} fallback={<EmptyState title={t('runner.no_connectors')} />}>
            <div class="grid gap-1 text-sm">
              <For each={status()?.connectors}>
                {(c) => (
                  <div>
                    <span class="font-medium">{c.name}</span>
                    <span class="text-muted-foreground"> · {c.id}{c.country ? ` · ${c.country}` : ''} · {c.fields.map((f) => f.label).join(', ')}</span>
                  </div>
                )}
              </For>
            </div>
          </Show>
        </CardContent>
      </Card>

      <Dialog open={open()} onOpenChange={setOpen} title={t('runner.new_token')} description={t('runner.new_token_hint')}>
        <Show
          when={made()}
          fallback={
            <form class="grid gap-4" onSubmit={create}>
              <Field label={t('tokens.label')} error={errors().label}>
                <Input required maxLength={80} value={label()} onInput={(e) => setLabel(e.currentTarget.value)} />
              </Field>
              <div class="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
                <Button type="button" variant="outline" onClick={() => setOpen(false)}>{t('common.cancel')}</Button>
                <Button type="submit" disabled={!label().trim()}>{t('common.create')}</Button>
              </div>
            </form>
          }
        >
          <div class="grid gap-3">
            <p class="text-sm">{t('tokens.shown_once')}</p>
            <code class="rounded-md border bg-muted px-3 py-2 font-mono text-sm break-all" data-testid="new-runner-token">{made()}</code>
            <div class="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
              <Button variant="outline" onClick={async () => {
                try { await navigator.clipboard.writeText(made()); toast.success(t('tokens.copied')); } catch { toast.error(t('tokens.copy_failed')); }
              }}><Copy /> {t('tokens.copy')}</Button>
              <Button onClick={() => setOpen(false)}>{t('tokens.done')}</Button>
            </div>
          </div>
        </Show>
      </Dialog>
    </div>
  );
}
