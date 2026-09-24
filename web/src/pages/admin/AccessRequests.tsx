import { Check, X } from 'lucide-solid';
import { createResource, For, Show } from 'solid-js';
import { api } from '~/api/client';
import type { Registration } from '~/api/types';
import { Button } from '~/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '~/components/ui/card';
import { Badge, EmptyState, Table, tdClass, thClass, trClass } from '~/components/ui/misc';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';
import { formatDateTime } from '~/lib/dates';

export function AccessRequests(props: { registration?: Registration }) {
  const { t, te } = useI18n();
  const [requests, { refetch }] = createResource(() => api.admin.accessRequests());

  const decide = async (id: number, approve: boolean) => {
    try {
      await (approve ? api.admin.approve(id) : api.admin.reject(id));
      toast.success(approve ? t('admin.approved') : t('admin.rejected'));
    } catch (err) {
      toast.error(te(err));
    }
    refetch();
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('admin.tab_requests')}</CardTitle>
        <Show when={props.registration && props.registration !== 'request'}>
          <CardDescription>{t('admin.requests_off')}</CardDescription>
        </Show>
      </CardHeader>
      <CardContent>
        <Show when={(requests() ?? []).length > 0} fallback={<EmptyState title={t('admin.no_requests')} />}>
          <Table>
            <thead>
              <tr class="border-b">
                <th class={thClass}>{t('auth.username')}</th>
                <th class={thClass}>{t('settings.email')}</th>
                <th class={thClass}>{t('request.message')}</th>
                <th class={thClass}>{t('admin.requested_at')}</th>
                <th class={thClass}>{t('admin.status')}</th>
              </tr>
            </thead>
            <tbody>
              <For each={requests() ?? []}>
                {(r) => (
                  <tr class={trClass}>
                    <td class={`${tdClass} font-medium`}>{r.username}</td>
                    <td class={tdClass}>{r.email}</td>
                    <td class={`${tdClass} max-w-xs whitespace-pre-wrap text-muted-foreground`}>{r.message}</td>
                    <td class={tdClass}>{formatDateTime(r.created_at)}</td>
                    <td class={tdClass}>
                      <Show when={r.status === 'pending'} fallback={<Badge variant="outline">{t(`admin.request_${r.status}`)}</Badge>}>
                        <div class="flex gap-1">
                          <Button size="sm" onClick={() => decide(r.id, true)}><Check /> {t('admin.approve')}</Button>
                          <Button size="sm" variant="outline" onClick={() => decide(r.id, false)}><X /> {t('admin.reject')}</Button>
                        </div>
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
  );
}
