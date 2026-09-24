import { Copy, KeyRound, Trash2 } from 'lucide-solid';
import { createSignal, For, Show } from 'solid-js';
import { api } from '~/api/client';
import type { SessionInfo } from '~/api/types';
import { Button } from '~/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '~/components/ui/card';
import { Dialog } from '~/components/ui/dialog';
import { Field, Input } from '~/components/ui/input';
import { Table, tdClass, thClass, trClass } from '~/components/ui/misc';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';
import { formatDateTime } from '~/lib/dates';

/**
 * Tokens for the sync runners: each may only send import batches (and see
 * which books exist), never read the books or accept its own rows. The
 * token is shown once, when it is made.
 */
export function APITokens(props: { tokens: SessionInfo[]; onChange: () => void }) {
  const { t, te, fieldErrors } = useI18n();
  const [open, setOpen] = createSignal(false);
  const [label, setLabel] = createSignal('');
  const [days, setDays] = createSignal('365');
  const [errors, setErrors] = createSignal<Record<string, string>>({});
  const [made, setMade] = createSignal('');

  const start = () => {
    setLabel('');
    setDays('365');
    setErrors({});
    setMade('');
    setOpen(true);
  };
  const create = async (e: Event) => {
    e.preventDefault();
    try {
      const res = await api.createToken(label().trim(), Number(days()) || 365);
      setMade(res.token);
      props.onChange();
    } catch (err) {
      setErrors(fieldErrors(err));
      toast.error(te(err));
    }
  };
  const revoke = async (id: number) => {
    try {
      await api.revokeSession(id);
      toast.success(t('tokens.revoked'));
    } catch (err) {
      toast.error(te(err));
    }
    props.onChange();
  };
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(made());
      toast.success(t('tokens.copied'));
    } catch {
      toast.error(t('tokens.copy_failed'));
    }
  };

  return (
    <Card>
      <CardHeader>
        <div class="flex items-start justify-between gap-4">
          <div class="grid gap-1">
            <CardTitle>{t('tokens.title')}</CardTitle>
            <CardDescription>{t('tokens.hint')}</CardDescription>
          </div>
          <Button size="sm" variant="outline" onClick={start}>
            <KeyRound /> {t('tokens.new')}
          </Button>
        </div>
      </CardHeader>
      <Show when={props.tokens.length}>
        <CardContent>
          <Table>
            <thead>
              <tr class="border-b">
                <th class={thClass}>{t('tokens.label')}</th>
                <th class={thClass}>{t('security.last_used')}</th>
                <th class={thClass}>{t('tokens.expires')}</th>
                <th class={thClass}><span class="sr-only">{t('common.actions')}</span></th>
              </tr>
            </thead>
            <tbody>
              <For each={props.tokens}>
                {(s) => (
                  <tr class={trClass}>
                    <td class={`${tdClass} font-medium`}>{s.label}</td>
                    <td class={`${tdClass} whitespace-nowrap`}>{s.last_used_at === s.created_at ? t('tokens.never_used') : formatDateTime(s.last_used_at)}</td>
                    <td class={`${tdClass} whitespace-nowrap`}>{formatDateTime(s.expires_at)}</td>
                    <td class={`${tdClass} text-right`}>
                      <Button size="sm" variant="ghost" onClick={() => revoke(s.id)}>
                        <Trash2 /> {t('tokens.revoke')}
                      </Button>
                    </td>
                  </tr>
                )}
              </For>
            </tbody>
          </Table>
        </CardContent>
      </Show>

      <Dialog open={open()} onOpenChange={setOpen} title={t('tokens.new')} description={t('tokens.new_hint')}>
        <Show
          when={made()}
          fallback={
            <form class="grid gap-4" onSubmit={create}>
              <Field label={t('tokens.label')} hint={t('tokens.label_hint')} error={errors().label}>
                <Input required maxLength={80} value={label()} onInput={(e) => setLabel(e.currentTarget.value)} />
              </Field>
              <Field label={t('tokens.days')} error={errors().days}>
                <Input type="number" min="1" max="730" value={days()} onInput={(e) => setDays(e.currentTarget.value)} />
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
            <code class="rounded-md border bg-muted px-3 py-2 font-mono text-sm break-all" data-testid="new-token">{made()}</code>
            <div class="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
              <Button variant="outline" onClick={copy}><Copy /> {t('tokens.copy')}</Button>
              <Button onClick={() => setOpen(false)}>{t('tokens.done')}</Button>
            </div>
          </div>
        </Show>
      </Dialog>
    </Card>
  );
}
