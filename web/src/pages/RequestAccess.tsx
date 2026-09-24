import { createSignal, Show } from 'solid-js';
import { api } from '~/api/client';
import { AuthCard } from '~/components/AuthCard';
import { Button } from '~/components/ui/button';
import { Field, Input, Textarea } from '~/components/ui/input';
import { useI18n } from '~/i18n';

/** /request-access -- ask the administrators for an invitation. */
export default function RequestAccess() {
  const { t, te, fieldErrors } = useI18n();
  const [username, setUsername] = createSignal('');
  const [email, setEmail] = createSignal('');
  const [message, setMessage] = createSignal('');
  const [errors, setErrors] = createSignal<Record<string, string>>({});
  const [error, setError] = createSignal('');
  const [sent, setSent] = createSignal(false);
  const [busy, setBusy] = createSignal(false);

  const submit = async (e: Event) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      await api.requestAccess({ username: username(), email: email(), message: message() });
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
    <Show when={!sent()} fallback={<AuthCard title={t('request.sent')} description={t('request.sent_hint')} footer={footer} />}>
      <AuthCard title={t('request.title')} description={t('request.description')} footer={footer}>
        <form class="grid gap-4" onSubmit={submit}>
          <Field label={t('auth.username')} hint={t('profile.username_hint')} error={errors().username}>
            <Input autocomplete="username" required value={username()} onInput={(e) => setUsername(e.currentTarget.value)} />
          </Field>
          <Field label={t('settings.email')} error={errors().email}>
            <Input type="email" autocomplete="email" required value={email()} onInput={(e) => setEmail(e.currentTarget.value)} />
          </Field>
          <Field label={<>{t('request.message')} <span class="text-muted-foreground">({t('common.optional')})</span></>}>
            <Textarea rows={3} maxLength={1000} value={message()} onInput={(e) => setMessage(e.currentTarget.value)} />
          </Field>
          <Show when={error() && Object.keys(errors()).length === 0}>
            <p role="alert" class="text-sm text-destructive">{error()}</p>
          </Show>
          <Button type="submit" disabled={busy()}>{t('request.submit')}</Button>
        </form>
      </AuthCard>
    </Show>
  );
}
