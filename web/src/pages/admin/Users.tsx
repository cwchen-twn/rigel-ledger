import { MailPlus, MoreHorizontal } from 'lucide-solid';
import { createResource, createSignal, For, Show } from 'solid-js';
import { api } from '~/api/client';
import type { AdminUser } from '~/api/types';
import { Button } from '~/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '~/components/ui/card';
import { DropdownMenu, type MenuItem } from '~/components/ui/dropdown-menu';
import { Field, Input } from '~/components/ui/input';
import { Badge, Table, tdClass, thClass, trClass } from '~/components/ui/misc';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';
import { formatDateTime } from '~/lib/dates';
import { useSession } from '~/stores/session';

export function Users() {
  const { t, te, fieldErrors } = useI18n();
  const { user } = useSession();
  const [users, { refetch }] = createResource(() => api.admin.users());
  const [username, setUsername] = createSignal('');
  const [email, setEmail] = createSignal('');
  const [errors, setErrors] = createSignal<Record<string, string>>({});
  const [busy, setBusy] = createSignal(false);

  const invite = async (e: Event) => {
    e.preventDefault();
    setBusy(true);
    try {
      await api.admin.invite(username(), email());
      toast.success(t('admin.invite_sent', { email: email() }));
      setUsername('');
      setEmail('');
      setErrors({});
    } catch (err) {
      setErrors(fieldErrors(err));
      toast.error(te(err));
    } finally {
      setBusy(false);
      refetch();
    }
  };

  const act = async (fn: () => Promise<unknown>, done: string) => {
    try {
      await fn();
      toast.success(done);
    } catch (err) {
      toast.error(te(err));
    }
    refetch();
  };

  const items = (u: AdminUser): MenuItem[] => {
    if (u.invited) {
      return [
        { label: t('admin.resend_invite'), onSelect: () => act(() => api.admin.resendInvite(u.id), t('admin.invite_sent', { email: u.email })) },
        { label: t('admin.revoke_invite'), onSelect: () => confirm(t('common.confirm')) && act(() => api.admin.revokeInvite(u.id), t('common.deleted')), separatorBefore: true },
      ];
    }
    return [
      u.is_admin
        ? { label: t('admin.demote'), onSelect: () => act(() => api.admin.updateUser(u.id, { is_admin: false }), t('common.saved')) }
        : { label: t('admin.promote'), onSelect: () => act(() => api.admin.updateUser(u.id, { is_admin: true }), t('common.saved')) },
      { label: t('admin.reset_mfa'), onSelect: () => confirm(t('admin.reset_mfa_confirm')) && act(() => api.admin.resetMFA(u.id), t('admin.mfa_was_reset')) },
      u.is_active
        ? { label: t('admin.deactivate'), onSelect: () => confirm(t('admin.deactivate_confirm')) && act(() => api.admin.updateUser(u.id, { is_active: false }), t('common.saved')), separatorBefore: true }
        : { label: t('admin.activate'), onSelect: () => act(() => api.admin.updateUser(u.id, { is_active: true }), t('common.saved')), separatorBefore: true },
    ];
  };

  return (
    <div class="grid gap-6">
      <Card>
        <CardHeader>
          <CardTitle>{t('admin.invite_title')}</CardTitle>
          <CardDescription>{t('admin.invite_hint')}</CardDescription>
        </CardHeader>
        <CardContent>
          <form class="grid gap-4 sm:grid-cols-[1fr_1fr_auto] sm:items-end" onSubmit={invite}>
            <Field label={t('auth.username')} error={errors().username}>
              <Input required value={username()} onInput={(e) => setUsername(e.currentTarget.value)} />
            </Field>
            <Field label={t('settings.email')} error={errors().email}>
              <Input type="email" required value={email()} onInput={(e) => setEmail(e.currentTarget.value)} />
            </Field>
            <Button type="submit" disabled={busy()}><MailPlus /> {t('admin.invite')}</Button>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader><CardTitle>{t('admin.users')}</CardTitle></CardHeader>
        <CardContent>
          <Table>
            <thead>
              <tr class="border-b">
                <th class={thClass}>{t('auth.username')}</th>
                <th class={thClass}>{t('settings.email')}</th>
                <th class={thClass}>{t('admin.status')}</th>
                <th class={thClass}>{t('admin.last_login')}</th>
                <th class={thClass}><span class="sr-only">{t('common.actions')}</span></th>
              </tr>
            </thead>
            <tbody>
              <For each={users() ?? []}>
                {(u) => (
                  <tr class={trClass}>
                    <td class={tdClass}>
                      <div class="font-medium">{u.username}</div>
                      <Show when={u.display_name}><div class="text-xs text-muted-foreground">{u.display_name}</div></Show>
                    </td>
                    <td class={tdClass}>
                      {u.email}
                      <Show when={u.email && !u.email_verified}>
                        <Badge variant="warning" class="ml-2">{t('email.unverified_badge')}</Badge>
                      </Show>
                    </td>
                    <td class={tdClass}>
                      <div class="flex flex-wrap gap-1">
                        <Show when={u.is_admin}><Badge>{t('admin.admin')}</Badge></Show>
                        <Show when={u.invited}><Badge variant="outline">{u.invite_pending ? t('admin.invited') : t('admin.invite_expired')}</Badge></Show>
                        <Show when={!u.invited && !u.initialized}><Badge variant="outline">{t('admin.setting_up')}</Badge></Show>
                        <Show when={!u.is_active}><Badge variant="destructive">{t('admin.inactive')}</Badge></Show>
                      </div>
                    </td>
                    <td class={tdClass}>{u.last_login_at ? formatDateTime(u.last_login_at) : '—'}</td>
                    <td class={`${tdClass} text-right`}>
                      <Show when={u.id !== user()?.id}>
                        <DropdownMenu
                          label={t('common.actions')}
                          triggerClass="rounded-md p-1.5 hover:bg-accent"
                          trigger={<MoreHorizontal class="size-4" />}
                          items={items(u)}
                        />
                      </Show>
                    </td>
                  </tr>
                )}
              </For>
            </tbody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
