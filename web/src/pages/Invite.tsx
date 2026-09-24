import { useNavigate, useParams } from '@solidjs/router';
import { createEffect, createResource, createSignal, Match, Show, Switch } from 'solid-js';
import { createStore } from 'solid-js/store';
import { api } from '~/api/client';
import type { Profile } from '~/api/types';
import { AuthCard } from '~/components/AuthCard';
import { browserLanguage, browserTimeZone, ProfileFields } from '~/components/ProfileFields';
import { Button } from '~/components/ui/button';
import { Field, Input } from '~/components/ui/input';
import { useI18n } from '~/i18n';
import { useSession } from '~/stores/session';

/** /invite/:token -- an invited user chooses settings and a password, then is signed in. */
export default function Invite() {
  const { t, te, fieldErrors, setLocale } = useI18n();
  const { setUser } = useSession();
  const params = useParams();
  const navigate = useNavigate();
  const [invitation] = createResource(() => params.token, (tok) => api.invitation(tok));

  const [profile, setProfile] = createStore<Profile>({
    username: '', display_name: '', language: 'en', display_currency: 'USD',
    timezone: 'UTC', date_format: 'YYYY-MM-DD', theme: 'system',
  });
  const [loaded, setLoaded] = createSignal(false);
  const [password, setPassword] = createSignal('');
  const [confirm, setConfirm] = createSignal('');
  const [errors, setErrors] = createSignal<Record<string, string>>({});
  const [error, setError] = createSignal('');
  const [busy, setBusy] = createSignal(false);

  const prefill = () => {
    const inv = invitation();
    if (!inv || loaded()) return;
    setProfile({
      username: inv.username, display_name: inv.display_name,
      language: browserLanguage() ?? inv.language, display_currency: inv.display_currency,
      timezone: inv.timezone === 'UTC' ? (browserTimeZone() ?? 'UTC') : inv.timezone,
      date_format: inv.date_format, theme: inv.theme,
    });
    setLoaded(true);
  };

  createEffect(prefill);
  createEffect(() => loaded() && setLocale(profile.language));

  const submit = async (e: Event) => {
    e.preventDefault();
    if (password() !== confirm()) {
      setErrors({ confirm_password: t('welcome.passwords_differ') });
      return;
    }
    setBusy(true);
    setError('');
    try {
      const { user } = await api.acceptInvite(params.token ?? '', { ...profile, password: password() });
      setUser(user);
      setLocale(user.language);
      navigate('/', { replace: true });
    } catch (err) {
      setErrors(fieldErrors(err));
      setError(te(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Switch>
      <Match when={invitation.error}>
        <AuthCard title={t('invite.invalid_title')} description={t('invite.invalid')} footer={<a href="/login" class="underline-offset-4 hover:underline">{t('auth.sign_in')}</a>} />
      </Match>
      <Match when={invitation()}>
        {(inv) => (
            <AuthCard wide title={t('invite.title')} description={t('invite.description', { email: inv().email })}>
              <form class="grid gap-6" onSubmit={submit}>
                <ProfileFields profile={profile} set={setProfile} errors={errors()} />
                <div class="grid gap-4 sm:grid-cols-2">
                  <Field label={t('auth.password')} error={errors().password}>
                    <Input type="password" autocomplete="new-password" minLength={8} required value={password()} onInput={(e) => setPassword(e.currentTarget.value)} />
                  </Field>
                  <Field label={t('welcome.confirm_password')} error={errors().confirm_password}>
                    <Input type="password" autocomplete="new-password" minLength={8} required value={confirm()} onInput={(e) => setConfirm(e.currentTarget.value)} />
                  </Field>
                </div>
                <Show when={error()}>
                  <p role="alert" class="text-sm text-destructive">{error()}</p>
                </Show>
                <Button type="submit" disabled={busy()}>{busy() ? t('common.saving') : t('invite.accept')}</Button>
              </form>
            </AuthCard>
        )}
      </Match>
    </Switch>
  );
}
