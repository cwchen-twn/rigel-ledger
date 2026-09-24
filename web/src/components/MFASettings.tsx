import { Trash2 } from 'lucide-solid';
import { createResource, createSignal, For, Show } from 'solid-js';
import { api } from '~/api/client';
import type { Enrolled } from '~/api/types';
import { EmailEnroll, PasskeyEnroll, RecoveryCodes, TOTPEnroll } from '~/components/MFAEnroll';
import { Button } from '~/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '~/components/ui/card';
import { Checkbox, Field, Input } from '~/components/ui/input';
import { Badge } from '~/components/ui/misc';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';
import { formatDateTime } from '~/lib/dates';
import { useSession } from '~/stores/session';

/** The settings page's "Two-factor sign-in" card. */
export function MFASettings() {
  const { t, te } = useI18n();
  const { user, refresh } = useSession();
  const [status, { refetch }] = createResource(() => api.mfa.status());
  const [password, setPassword] = createSignal('');
  const [codes, setCodes] = createSignal<string[] | null>(null);
  const allowed = (m: string) => status()?.allowed.includes(m as never) ?? false;

  const enrolled = (e: Enrolled) => {
    if (e.recovery_codes?.length) setCodes(e.recovery_codes);
    toast.success(t('mfa.added'));
    refetch();
    refresh();
  };

  // Removing or regenerating asks for the password again.
  const guarded = async (fn: (pw: string) => Promise<unknown>, ok: string) => {
    if (!password()) {
      toast.error(t('mfa.password_first'));
      return;
    }
    try {
      const r = await fn(password());
      const rc = (r as Enrolled | undefined)?.recovery_codes;
      if (rc?.length) setCodes(rc);
      toast.success(ok);
      refetch();
      refresh();
    } catch (err) {
      toast.error(te(err));
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle class="flex items-center gap-2">
          {t('mfa.title')}
          <Show when={status()?.required}><Badge variant="outline">{t('mfa.required')}</Badge></Show>
        </CardTitle>
        <CardDescription>{t('mfa.settings_hint')}</CardDescription>
      </CardHeader>
      <CardContent class="grid gap-6">
        <Show when={codes()}>{(c) => <RecoveryCodes codes={c()} />}</Show>

        <Field label={t('settings.current_password')} hint={t('mfa.password_hint')} class="max-w-sm">
          <Input type="password" autocomplete="current-password" value={password()} onInput={(e) => setPassword(e.currentTarget.value)} />
        </Field>

        <Show when={allowed('passkey')}>
          <div class="grid gap-3">
            <Show when={(status()?.passkeys.length ?? 0) > 0}>
              <ul class="grid gap-2">
                <For each={status()?.passkeys}>
                  {(p) => (
                    <li class="flex items-center justify-between gap-2 rounded-md border px-3 py-2 text-sm">
                      <span class="grid">
                        <span class="font-medium">{p.name}</span>
                        <span class="text-xs text-muted-foreground">
                          {t('mfa.added_on', { date: formatDateTime(p.created_at) })}
                          {p.last_used_at ? ' · ' + t('mfa.used_on', { date: formatDateTime(p.last_used_at) }) : ''}
                        </span>
                      </span>
                      <Button size="sm" variant="ghost" aria-label={t('common.remove')}
                        onClick={() => guarded((pw) => api.mfa.removePasskey(p.id, pw), t('mfa.removed'))}>
                        <Trash2 />
                      </Button>
                    </li>
                  )}
                </For>
              </ul>
            </Show>
            <PasskeyEnroll onDone={enrolled} />
          </div>
        </Show>

        <Show when={allowed('totp')}>
          <Show when={status()?.totp} fallback={<TOTPEnroll onDone={enrolled} />}>
            <div class="flex items-center justify-between gap-2 text-sm">
              <span>{t('mfa.method_totp')} <Badge class="ml-1">{t('mfa.on')}</Badge></span>
              <Button size="sm" variant="outline" onClick={() => guarded((pw) => api.mfa.removeTOTP(pw), t('mfa.removed'))}>{t('common.remove')}</Button>
            </div>
          </Show>
        </Show>

        <Show when={allowed('email')}>
          <Show when={status()?.email} fallback={<Show when={user()?.email_verified}><EmailEnroll onDone={enrolled} email={user()?.email ?? ''} /></Show>}>
            <div class="flex items-center justify-between gap-2 text-sm">
              <span>{t('mfa.method_email')} <Badge class="ml-1">{t('mfa.on')}</Badge></span>
              <Button size="sm" variant="outline" onClick={() => guarded((pw) => api.mfa.removeEmail(pw), t('mfa.removed'))}>{t('common.remove')}</Button>
            </div>
          </Show>
        </Show>

        <Show when={(status()?.recovery_left ?? 0) > 0 || status()?.totp || status()?.email || (status()?.passkeys.length ?? 0) > 0}>
          <div class="flex flex-wrap items-center justify-between gap-2 text-sm">
            <span>{t('mfa.recovery_left', { n: status()?.recovery_left ?? 0 })}</span>
            <Button size="sm" variant="outline" onClick={() => guarded((pw) => api.mfa.regenerateRecovery(pw), t('mfa.codes_regenerated'))}>
              {t('mfa.regenerate')}
            </Button>
          </div>
        </Show>

        <Checkbox
          label={t('mfa.alerts')}
          checked={status()?.signin_alerts ?? true}
          onChange={async (e) => {
            try {
              await api.mfa.setAlerts(e.currentTarget.checked);
              toast.success(t('common.saved'));
            } catch (err) {
              toast.error(te(err));
            }
          }}
        />
      </CardContent>
    </Card>
  );
}
