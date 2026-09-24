import { useNavigate } from '@solidjs/router';
import { createResource, createSignal, Match, Show, Switch } from 'solid-js';
import { api } from '~/api/client';
import { AuthCard } from '~/components/AuthCard';
import { Button } from '~/components/ui/button';
import { Field, Input } from '~/components/ui/input';
import { useI18n } from '~/i18n';
import { useSession } from '~/stores/session';

export default function Login() {
  const { t, te } = useI18n();
  const { login } = useSession();
  const navigate = useNavigate();
  const [config] = createResource(() => api.authConfig().catch(() => null));
  const [username, setUsername] = createSignal('');
  const [password, setPassword] = createSignal('');
  const [error, setError] = createSignal('');
  const [busy, setBusy] = createSignal(false);

  const submit = async (e: Event) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      await login(username(), password());
      navigate('/', { replace: true });
    } catch (err) {
      setError(te(err));
    } finally {
      setBusy(false);
    }
  };

  const footer = (
    <Switch>
      <Match when={config()?.registration === 'open'}>
        <a href="/register" class="underline-offset-4 hover:underline">{t('register.link')}</a>
      </Match>
      <Match when={config()?.registration === 'request'}>
        <a href="/request-access" class="underline-offset-4 hover:underline">{t('request.link')}</a>
      </Match>
    </Switch>
  );

  return (
    <AuthCard title={t('app.name')} description={t('auth.welcome')} footer={config()?.registration !== 'closed' ? footer : undefined}>
      <form class="grid gap-4" onSubmit={submit}>
        <Field label={t('auth.username')}>
          <Input autocomplete="username" autofocus required value={username()} onInput={(e) => setUsername(e.currentTarget.value)} />
        </Field>
        <Field label={t('auth.password')}>
          <Input type="password" autocomplete="current-password" required value={password()} onInput={(e) => setPassword(e.currentTarget.value)} />
        </Field>
        <Show when={error()}>
          <p role="alert" class="text-sm text-destructive">{error()}</p>
        </Show>
        <Button type="submit" disabled={busy()}>
          {busy() ? t('auth.signing_in') : t('auth.sign_in')}
        </Button>
      </form>
    </AuthCard>
  );
}
