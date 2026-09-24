import { A, useParams } from '@solidjs/router';
import { createResource, For, Match, Show, Switch } from 'solid-js';
import { api } from '~/api/client';
import { PageHeader } from '~/components/AppShell';
import { useI18n } from '~/i18n';
import { cn } from '~/lib/cn';
import NotFound from '~/pages/NotFound';
import { useSession } from '~/stores/session';
import { Activity } from './Activity';
import { AccessRequests } from './AccessRequests';
import { MailSettings } from './MailSettings';
import { SettingsForm } from './SettingsForm';
import { Users } from './Users';

const TABS = ['users', 'requests', 'access', 'defaults', 'mail', 'activity'] as const;
type Tab = (typeof TABS)[number];

/** /admin/:tab -- the instance's users, invitations, sign-in rules, defaults and mail. */
export default function Admin() {
  const { t } = useI18n();
  const { user } = useSession();
  const params = useParams();
  const tab = (): Tab => (TABS.includes(params.tab as Tab) ? (params.tab as Tab) : 'users');
  const [settings, { mutate }] = createResource(() => user()?.is_admin, () => api.admin.settings());

  return (
    <Show when={user()?.is_admin} fallback={<NotFound />}>
      <PageHeader title={t('admin.title')} description={t('admin.description')} />
      <nav class="mb-6 flex gap-1 overflow-x-auto border-b" role="tablist" aria-label={t('admin.title')}>
        <For each={TABS}>
          {(k) => (
            <A
              href={`/admin/${k}`}
              role="tab"
              aria-selected={tab() === k}
              class={cn(
                '-mb-px shrink-0 border-b-2 px-3 py-2 text-sm transition-colors',
                tab() === k ? 'border-primary font-medium text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground',
              )}
            >
              {t(`admin.tab_${k}`)}
            </A>
          )}
        </For>
      </nav>
      <Switch>
        <Match when={tab() === 'users'}><Users /></Match>
        <Match when={tab() === 'requests'}><AccessRequests registration={settings()?.registration} /></Match>
        <Match when={tab() === 'activity'}><Activity /></Match>
        <Match when={settings()}>
          {(s) => (
            <Switch>
              <Match when={tab() === 'access'}><SettingsForm settings={s()} onSaved={mutate} section="access" /></Match>
              <Match when={tab() === 'defaults'}><SettingsForm settings={s()} onSaved={mutate} section="defaults" /></Match>
              <Match when={tab() === 'mail'}><MailSettings settings={s()} onSaved={mutate} /></Match>
            </Switch>
          )}
        </Match>
      </Switch>
    </Show>
  );
}
