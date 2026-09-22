import { useNavigate } from '@solidjs/router';
import { Wallet } from 'lucide-solid';
import { createSignal, Show } from 'solid-js';
import { Button } from '~/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '~/components/ui/card';
import { Field, Input } from '~/components/ui/input';
import { useI18n } from '~/i18n';
import { useSession } from '~/stores/session';

export default function Login() {
  const { t, te } = useI18n();
  const { login } = useSession();
  const navigate = useNavigate();
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

  return (
    <div class="flex min-h-screen items-center justify-center bg-muted/40 p-4">
      <Card class="w-full max-w-sm">
        <CardHeader class="justify-items-center text-center">
          <div class="mb-2 flex size-10 items-center justify-center rounded-xl bg-primary text-primary-foreground">
            <Wallet class="size-5" />
          </div>
          <CardTitle class="text-xl">{t('app.name')}</CardTitle>
          <CardDescription>{t('auth.welcome')}</CardDescription>
        </CardHeader>
        <CardContent>
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
        </CardContent>
      </Card>
    </div>
  );
}
