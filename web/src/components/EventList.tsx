import { For, Show } from 'solid-js';
import type { AuthEvent } from '~/api/types';
import { Badge, EmptyState, Table, tdClass, thClass, trClass } from '~/components/ui/misc';
import { useI18n } from '~/i18n';
import { formatDateTime } from '~/lib/dates';

/** The security audit: sign-ins, failures, verifications, admin changes. */
export function EventList(props: { events: AuthEvent[]; showUser?: boolean }) {
  const { t } = useI18n();
  const label = (e: string) => {
    const s = t(`event.${e}`);
    return s === `event.${e}` ? e : s;
  };
  return (
    <Show when={props.events.length > 0} fallback={<EmptyState title={t('security.no_events')} />}>
      <Table>
        <thead>
          <tr class="border-b">
            <th class={thClass}>{t('security.when')}</th>
            <Show when={props.showUser}><th class={thClass}>{t('auth.username')}</th></Show>
            <th class={thClass}>{t('security.event')}</th>
            <th class={thClass}>{t('security.ip')}</th>
            <th class={`${thClass} hidden md:table-cell`}>{t('security.device')}</th>
          </tr>
        </thead>
        <tbody>
          <For each={props.events}>
            {(e) => (
              <tr class={trClass}>
                <td class={`${tdClass} whitespace-nowrap`}>{formatDateTime(e.created_at)}</td>
                <Show when={props.showUser}><td class={tdClass}>{e.username || '—'}</td></Show>
                <td class={tdClass}>
                  <Show when={e.failure} fallback={label(e.event)}>
                    <Badge variant="warning">{label(e.event)}</Badge>
                  </Show>
                </td>
                <td class={`${tdClass} font-mono text-xs`}>{e.ip || '—'}</td>
                <td class={`${tdClass} hidden max-w-xs truncate text-xs text-muted-foreground md:table-cell`} title={e.user_agent}>{e.user_agent}</td>
              </tr>
            )}
          </For>
        </tbody>
      </Table>
    </Show>
  );
}
