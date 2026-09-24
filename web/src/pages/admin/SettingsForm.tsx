import { createSignal, For, Show } from 'solid-js';
import { createStore, unwrap } from 'solid-js/store';
import { api } from '~/api/client';
import type { MfaMethod, Profile, Registration, SystemSettings, SystemSettingsInput } from '~/api/types';
import { ProfileFields } from '~/components/ProfileFields';
import { Button } from '~/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '~/components/ui/card';
import { Checkbox, Field, Input } from '~/components/ui/input';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';

const REGISTRATION: Registration[] = ['closed', 'request', 'open'];
const METHODS: MfaMethod[] = ['email', 'totp', 'passkey'];
const DAY = 86400;

function input(s: SystemSettings): SystemSettingsInput {
  return {
    registration: s.registration, mfa_required: s.mfa_required, mfa_methods: [...s.mfa_methods],
    default_language: s.default_language, default_display_currency: s.default_display_currency,
    default_timezone: s.default_timezone, default_date_format: s.default_date_format, default_theme: s.default_theme,
    session_ttl_seconds: s.session_ttl_seconds, invite_ttl_seconds: s.invite_ttl_seconds,
    login_max_failures: s.login_max_failures, login_ip_max_failures: s.login_ip_max_failures,
    login_user_max_failures: s.login_user_max_failures, login_window_seconds: s.login_window_seconds,
  };
}

/**
 * The settings row, split over two tabs: "access" (registration, two-factor,
 * sessions, throttling) and "defaults" (what a new user starts with). Both
 * save the whole row.
 */
export function SettingsForm(props: { settings: SystemSettings; section: 'access' | 'defaults'; onSaved: (s: SystemSettings) => void }) {
  const { t, te, fieldErrors } = useI18n();
  const [form, setForm] = createStore<SystemSettingsInput>(input(props.settings));
  const [errors, setErrors] = createSignal<Record<string, string>>({});
  const [busy, setBusy] = createSignal(false);

  // The defaults reuse the profile form; map its keys onto default_*.
  const profile = (): Profile => ({
    username: '', display_name: '', language: form.default_language, display_currency: form.default_display_currency,
    timezone: form.default_timezone, date_format: form.default_date_format, theme: form.default_theme,
  });
  const setProfile = ((key: keyof Profile, value: string) => {
    setForm(`default_${key}` as keyof SystemSettingsInput, value as never);
  }) as unknown as Parameters<typeof ProfileFields>[0]['set'];
  const defaultErrors = () => {
    const out: Record<string, string> = {};
    for (const [k, v] of Object.entries(errors())) if (k.startsWith('default_')) out[k.slice(8)] = v;
    return out;
  };

  const save = async (e: Event) => {
    e.preventDefault();
    setBusy(true);
    try {
      const s = await api.admin.updateSettings(structuredClone(unwrap(form)));
      props.onSaved(s);
      setErrors({});
      toast.success(t('common.saved'));
    } catch (err) {
      setErrors(fieldErrors(err));
      toast.error(te(err));
    } finally {
      setBusy(false);
    }
  };

  const toggleMethod = (m: MfaMethod, on: boolean) =>
    setForm('mfa_methods', (ms) => (on ? [...new Set([...ms, m])] : ms.filter((x) => x !== m)));
  const num = (v: string) => (v === '' ? 0 : Number(v));

  return (
    <form class="grid gap-6" onSubmit={save}>
      <Show when={props.section === 'access'}>
        <Card>
          <CardHeader>
            <CardTitle>{t('admin.registration')}</CardTitle>
            <CardDescription>{t('admin.registration_hint')}</CardDescription>
          </CardHeader>
          <CardContent>
            <fieldset class="grid gap-3" aria-label={t('admin.registration')}>
              <For each={REGISTRATION}>
                {(r) => (
                  <label class="flex items-start gap-3 rounded-lg border p-3 has-checked:border-primary has-checked:bg-accent/40">
                    <input type="radio" name="registration" class="mt-1 accent-primary" checked={form.registration === r} onChange={() => setForm('registration', r)} />
                    <span class="grid gap-0.5">
                      <span class="text-sm font-medium">{t(`admin.registration_${r}`)}</span>
                      <span class="text-xs text-muted-foreground">{t(`admin.registration_${r}_hint`)}</span>
                    </span>
                  </label>
                )}
              </For>
              <Field label={t('admin.invite_ttl_days')} error={errors().invite_ttl_seconds} class="max-w-64">
                <Input type="number" min="1" step="1" value={Math.round(form.invite_ttl_seconds / DAY)} onInput={(e) => setForm('invite_ttl_seconds', num(e.currentTarget.value) * DAY)} />
              </Field>
            </fieldset>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t('admin.mfa')}</CardTitle>
            <CardDescription>{t('admin.mfa_hint')}</CardDescription>
          </CardHeader>
          <CardContent class="grid gap-4">
            <Checkbox label={t('admin.mfa_required')} checked={form.mfa_required} onChange={(e) => setForm('mfa_required', e.currentTarget.checked)} />
            <div class="grid gap-2">
              <span class="text-sm font-medium">{t('admin.mfa_methods')}</span>
              <div class="flex flex-wrap gap-4">
                <For each={METHODS}>
                  {(m) => (
                    <Checkbox label={t(`admin.mfa_${m}`)} checked={form.mfa_methods.includes(m)} onChange={(e) => toggleMethod(m, e.currentTarget.checked)} />
                  )}
                </For>
              </div>
              <Show when={errors().mfa_methods}><p class="text-xs text-destructive">{errors().mfa_methods}</p></Show>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t('admin.sign_in_limits')}</CardTitle>
            <CardDescription>{t('admin.sign_in_limits_hint')}</CardDescription>
          </CardHeader>
          <CardContent class="grid gap-4 sm:grid-cols-2">
            <Field label={t('admin.session_ttl_days')} hint={t('admin.session_ttl_hint')} error={errors().session_ttl_seconds}>
              <Input
                type="number" min="1" step="1"
                value={form.session_ttl_seconds == null ? '' : Math.round(form.session_ttl_seconds / DAY)}
                onInput={(e) => setForm('session_ttl_seconds', e.currentTarget.value === '' ? null : num(e.currentTarget.value) * DAY)}
              />
            </Field>
            <Field label={t('admin.login_window_minutes')} error={errors().login_window_seconds}>
              <Input type="number" min="1" step="1" value={Math.round(form.login_window_seconds / 60)} onInput={(e) => setForm('login_window_seconds', num(e.currentTarget.value) * 60)} />
            </Field>
            <Field label={t('admin.login_max_failures')} error={errors().login_max_failures}>
              <Input type="number" min="1" step="1" value={form.login_max_failures} onInput={(e) => setForm('login_max_failures', num(e.currentTarget.value))} />
            </Field>
            <Field label={t('admin.login_ip_max_failures')} error={errors().login_ip_max_failures}>
              <Input type="number" min="1" step="1" value={form.login_ip_max_failures} onInput={(e) => setForm('login_ip_max_failures', num(e.currentTarget.value))} />
            </Field>
            <Field label={t('admin.login_user_max_failures')} error={errors().login_user_max_failures}>
              <Input type="number" min="1" step="1" value={form.login_user_max_failures} onInput={(e) => setForm('login_user_max_failures', num(e.currentTarget.value))} />
            </Field>
          </CardContent>
        </Card>
      </Show>

      <Show when={props.section === 'defaults'}>
        <Card>
          <CardHeader>
            <CardTitle>{t('admin.defaults')}</CardTitle>
            <CardDescription>{t('admin.defaults_hint')}</CardDescription>
          </CardHeader>
          <CardContent>
            <ProfileFields
              profile={profile()}
              set={setProfile}
              errors={defaultErrors()}
              only={['language', 'display_currency', 'timezone', 'date_format', 'theme']}
            />
          </CardContent>
        </Card>
      </Show>

      <div>
        <Button type="submit" disabled={busy()}>{busy() ? t('common.saving') : t('common.save')}</Button>
      </div>
    </form>
  );
}
