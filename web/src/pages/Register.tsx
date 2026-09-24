import { createSignal, Show } from 'solid-js';
import { api } from '~/api/client';
import { AuthCard } from '~/components/AuthCard';
import { Button } from '~/components/ui/button';
import { Field, Input } from '~/components/ui/input';
import { useI18n } from '~/i18n';

/** /register -- open sign-up; the account exists once the mailed link is clicked. */
export default function Register() {
  const { t, te, fieldErrors, locale } = useI18n();
  const [username, setUsername] = createSignal('');
  const [email, setEmail] = createSignal('');
  const [password, setPassword] = createSignal('');
  const [errors, setErrors] = createSignal<Record<string, string>>({});
  const [error, setError] = createSignal('');
  const [sent, setSent] = createSignal(false);
  const [busy, setBusy] = createSignal(false);

  const submit = async (e: Event) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      await api.register({ username: username(), email: email(), password: password(), language: locale() });
      setSent(true);
    } catch (err) {
      setErrors(fieldErrors(err));
      setError(te(err));
    } finally {
      setBusy(false);
    }
  };

  const footer = <a href="/login" class="underline-offset-4 hover:underline">{t('register.have_account')}</a>;
  return (
    <Show
      when={!sent()}
      fallback={<AuthCard title={t('register.check_mail')} description={t('register.check_mail_hint', { email: email() })} footer={footer} />}
    >
      <AuthCard title={t('register.title')} description={t('register.description')} footer={footer}>
        <form class="grid gap-4" onSubmit={submit}>
          <Field label={t('auth.username')} hint={t('profile.username_hint')} error={errors().username}>
            <Input autocomplete="username" required value={username()} onInput={(e) => setUsername(e.currentTarget.value)} />
          </Field>
          <Field label={t('settings.email')} error={errors().email}>
            <Input type="email" autocomplete="email" required value={email()} onInput={(e) => setEmail(e.currentTarget.value)} />
          </Field>
          <Field label={t('auth.password')} error={errors().password}>
            <Input type="password" autocomplete="new-password" minLength={8} required value={password()} onInput={(e) => setPassword(e.currentTarget.value)} />
          </Field>
          <Show when={error() && Object.keys(errors()).length === 0}>
            <p role="alert" class="text-sm text-destructive">{error()}</p>
          </Show>
          <Button type="submit" disabled={busy()}>{t('register.submit')}</Button>
        </form>
      </AuthCard>
    </Show>
  );
}
