import { useNavigate } from '@solidjs/router';
import { Check } from 'lucide-solid';
import { createEffect, createSignal, For, Match, Show, Switch } from 'solid-js';
import { createStore } from 'solid-js/store';
import { api } from '~/api/client';
import type { Profile } from '~/api/types';
import { AuthCard } from '~/components/AuthCard';
import { EmailVerification } from '~/components/EmailVerification';
import { browserTimeZone, ProfileFields } from '~/components/ProfileFields';
import { Button } from '~/components/ui/button';
import { Field, Input } from '~/components/ui/input';
import { useI18n } from '~/i18n';
import { cn } from '~/lib/cn';
import { useSession } from '~/stores/session';

type Step = 'account' | 'profile' | 'preferences';
const STEPS: Step[] = ['account', 'profile', 'preferences'];

/**
 * The first-login wizard: every user walks it once (a bootstrap admin also
 * replaces the initial password) before the rest of the app opens up.
 */
export default function Welcome() {
  const { t, te, fieldErrors, setLocale } = useI18n();
  const { user, setUser, refresh, logout } = useSession();
  const navigate = useNavigate();
  const u = () => user()!;

  // Start from the stored values (the language may have been chosen by
  // whoever created the account), but offer the browser's time zone while the
  // account still has UTC.
  const [profile, setProfile] = createStore<Profile>({
    username: u().username,
    display_name: u().display_name,
    language: u().language,
    display_currency: u().display_currency,
    timezone: u().timezone === 'UTC' ? (browserTimeZone() ?? 'UTC') : u().timezone,
    date_format: u().date_format,
    theme: u().theme,
  });
  const [password, setPassword] = createSignal('');
  const [confirm, setConfirm] = createSignal('');
  const [step, setStep] = createSignal<Step>('account');
  const [errors, setErrors] = createSignal<Record<string, string>>({});
  const [error, setError] = createSignal('');
  const [busy, setBusy] = createSignal(false);

  // Show the wizard in the language being chosen.
  createEffect(() => setLocale(profile.language));

  const accountReady = () =>
    u().email_verified && profile.username.trim() !== '' &&
    (!u().password_must_change || (password().length >= 8 && password() === confirm()));

  const next = (e: Event) => {
    e.preventDefault();
    setError('');
    if (step() === 'account') {
      if (u().password_must_change && password() !== confirm()) {
        setErrors({ confirm_password: t('welcome.passwords_differ') });
        return;
      }
      setErrors({});
      setStep('profile');
    } else if (step() === 'profile') {
      setStep('preferences');
    } else {
      void finish();
    }
  };

  const finish = async () => {
    setBusy(true);
    try {
      const done = await api.onboarding({ ...profile, new_password: u().password_must_change ? password() : undefined });
      setUser(done);
      setLocale(done.language);
      navigate('/', { replace: true });
    } catch (err) {
      const f = fieldErrors(err);
      setErrors(f);
      setError(te(err));
      // Send the user back to the step that holds the offending field.
      if (f.username || f.email || f.new_password) setStep('account');
      else if (f.display_name) setStep('profile');
    } finally {
      setBusy(false);
    }
  };

  const idx = () => STEPS.indexOf(step());

  return (
    <AuthCard
      wide
      title={t('welcome.title')}
      description={t('welcome.description')}
      footer={<button type="button" class="underline-offset-4 hover:underline" onClick={() => logout()}>{t('nav.logout')}</button>}
    >
      <ol class="mb-6 flex items-center gap-2 text-sm" aria-label={t('welcome.steps')}>
        <For each={STEPS}>
          {(s, i) => (
            <li class="flex flex-1 items-center gap-2">
              <span
                class={cn(
                  'flex size-6 shrink-0 items-center justify-center rounded-full border text-xs',
                  i() < idx() && 'border-primary bg-primary text-primary-foreground',
                  i() === idx() && 'border-primary text-primary',
                )}
                aria-current={i() === idx() ? 'step' : undefined}
              >
                {i() < idx() ? <Check class="size-3.5" /> : i() + 1}
              </span>
              <span class={cn('truncate', i() === idx() ? 'font-medium' : 'text-muted-foreground')}>{t(`welcome.step_${s}`)}</span>
              <Show when={i() < STEPS.length - 1}>
                <span class="h-px flex-1 bg-border" />
              </Show>
            </li>
          )}
        </For>
      </ol>

      <Switch>
        <Match when={step() === 'account'}>
          <div class="grid gap-6">
            <Show when={u().password_must_change}>
              <div class="grid gap-4 sm:grid-cols-2">
                <p class="text-sm text-muted-foreground sm:col-span-2">{t('welcome.password_intro')}</p>
                <Field label={t('settings.new_password')} error={errors().new_password}>
                  <Input type="password" autocomplete="new-password" minLength={8} required value={password()} onInput={(e) => setPassword(e.currentTarget.value)} />
                </Field>
                <Field label={t('welcome.confirm_password')} error={errors().confirm_password}>
                  <Input type="password" autocomplete="new-password" minLength={8} required value={confirm()} onInput={(e) => setConfirm(e.currentTarget.value)} />
                </Field>
              </div>
            </Show>
            <ProfileFields profile={profile} set={setProfile} errors={errors()} only={['username']} class="grid gap-4" />
            <div class="grid gap-2">
              <p class="text-sm text-muted-foreground">{t('welcome.email_intro')}</p>
              <EmailVerification user={u()} needsPassword={false} onChange={setUser} onPending={refresh} />
              <Show when={errors().email}>
                <p class="text-xs text-destructive">{errors().email}</p>
              </Show>
            </div>
          </div>
        </Match>
        <Match when={step() === 'profile'}>
          <ProfileFields profile={profile} set={setProfile} errors={errors()} only={['display_name']} class="grid gap-4" />
        </Match>
        <Match when={step() === 'preferences'}>
          <ProfileFields
            profile={profile}
            set={setProfile}
            errors={errors()}
            only={['language', 'display_currency', 'timezone', 'date_format', 'theme']}
          />
        </Match>
      </Switch>

      <form class="mt-6 grid gap-3" onSubmit={next}>
        <Show when={error()}>
          <p role="alert" class="text-sm text-destructive">{error()}</p>
        </Show>
        <div class="flex justify-between gap-2">
          <Button type="button" variant="ghost" disabled={idx() === 0} onClick={() => setStep(STEPS[idx() - 1])}>
            {t('welcome.back')}
          </Button>
          <Button type="submit" disabled={busy() || (step() === 'account' && !accountReady())}>
            {step() === 'preferences' ? (busy() ? t('common.saving') : t('welcome.finish')) : t('welcome.next')}
          </Button>
        </div>
      </form>
    </AuthCard>
  );
}
