import { useNavigate } from '@solidjs/router';
import { KeyRound } from 'lucide-solid';
import { createResource, createSignal, For, Match, Show, Switch } from 'solid-js';
import { api } from '~/api/client';
import type { LoginResult, SignInMethod } from '~/api/types';
import { AuthCard } from '~/components/AuthCard';
import { Button } from '~/components/ui/button';
import { Field, Input } from '~/components/ui/input';
import { useI18n } from '~/i18n';
import { cn } from '~/lib/cn';
import { getPasskey, isCancelled, passkeysSupported } from '~/lib/webauthn';
import { useSession } from '~/stores/session';

export default function Login() {
  const { t, te } = useI18n();
  const { login, signedIn } = useSession();
  const navigate = useNavigate();
  const [config] = createResource(() => api.authConfig().catch(() => null));
  const [username, setUsername] = createSignal('');
  const [password, setPassword] = createSignal('');
  const [error, setError] = createSignal('');
  const [busy, setBusy] = createSignal(false);

  // The second step, after a correct password.
  const [challenge, setChallenge] = createSignal('');
  const [methods, setMethods] = createSignal<SignInMethod[]>([]);
  const [method, setMethod] = createSignal<SignInMethod>('totp');
  const [code, setCode] = createSignal('');
  const [codeSent, setCodeSent] = createSignal(false);

  const finish = (r: LoginResult) => {
    if (r.user) {
      signedIn(r.user);
      navigate('/', { replace: true });
      return true;
    }
    return false;
  };

  const run = async (fn: () => Promise<void>) => {
    setBusy(true);
    setError('');
    try {
      await fn();
    } catch (err) {
      if (!isCancelled(err)) setError(te(err));
    } finally {
      setBusy(false);
    }
  };

  const submitPassword = (e: Event) => {
    e.preventDefault();
    void run(async () => {
      const r = await login(username(), password());
      if (finish(r)) return;
      const ms = r.methods ?? [];
      setChallenge(r.challenge ?? '');
      setMethods(ms);
      // Offer the strongest first: a passkey, then the app, then email.
      setMethod((['passkey', 'totp', 'email', 'recovery'] as SignInMethod[]).find((m) => ms.includes(m)) ?? 'totp');
    });
  };

  const submitCode = (e: Event) => {
    e.preventDefault();
    void run(async () => {
      finish(await api.verifyMFA(challenge(), method(), code()));
    });
  };

  const sendCode = () =>
    run(async () => {
      await api.sendSignInCode(challenge());
      setCodeSent(true);
    });

  // A passkey alone (passwordless), or as the second step.
  const passkey = (second: boolean) =>
    run(async () => {
      const begin = await api.passkeyLoginBegin(second ? challenge() : '');
      const cred = await getPasskey(begin.options);
      finish(await api.passkeyLoginFinish(begin.challenge, cred));
    });

  const pick = (m: SignInMethod) => {
    setMethod(m);
    setCode('');
    setError('');
  };

  const footer = (
    <Switch>
      <Match when={challenge()}>
        <button type="button" class="underline-offset-4 hover:underline" onClick={() => { setChallenge(''); setPassword(''); setError(''); }}>
          {t('mfa.start_over')}
        </button>
      </Match>
      <Match when={config()?.registration === 'open'}>
        <a href="/register" class="underline-offset-4 hover:underline">{t('register.link')}</a>
      </Match>
      <Match when={config()?.registration === 'request'}>
        <a href="/request-access" class="underline-offset-4 hover:underline">{t('request.link')}</a>
      </Match>
    </Switch>
  );

  return (
    <AuthCard
      title={t('app.name')}
      description={challenge() ? t('mfa.second_step') : t('auth.welcome')}
      footer={challenge() || config()?.registration !== 'closed' ? footer : undefined}
    >
      <Show
        when={challenge()}
        fallback={
          <div class="grid gap-4">
            <form class="grid gap-4" onSubmit={submitPassword}>
              <Field label={t('auth.username')}>
                <Input autocomplete="username webauthn" autofocus required value={username()} onInput={(e) => setUsername(e.currentTarget.value)} />
              </Field>
              <Field label={t('auth.password')}>
                <Input type="password" autocomplete="current-password" required value={password()} onInput={(e) => setPassword(e.currentTarget.value)} />
              </Field>
              <Show when={error()}>
                <p role="alert" class="text-sm text-destructive">{error()}</p>
              </Show>
              <Button type="submit" disabled={busy()}>{busy() ? t('auth.signing_in') : t('auth.sign_in')}</Button>
            </form>
            <Show when={passkeysSupported()}>
              <div class="flex items-center gap-3 text-xs text-muted-foreground">
                <span class="h-px flex-1 bg-border" />{t('mfa.or')}<span class="h-px flex-1 bg-border" />
              </div>
              <Button type="button" variant="outline" disabled={busy()} onClick={() => passkey(false)}>
                <KeyRound /> {t('mfa.passkey_sign_in')}
              </Button>
            </Show>
          </div>
        }
      >
        <div class="grid gap-4">
          <Show when={methods().length > 1}>
            <div class="flex flex-wrap gap-1 rounded-lg bg-muted p-1" role="tablist" aria-label={t('mfa.methods')}>
              <For each={methods()}>
                {(m) => (
                  <button
                    type="button"
                    role="tab"
                    aria-selected={method() === m}
                    onClick={() => pick(m)}
                    class={cn('flex-1 rounded-md px-2 py-1 text-xs', method() === m ? 'bg-background font-medium shadow-xs' : 'text-muted-foreground hover:text-foreground')}
                  >
                    {t(`mfa.method_${m}`)}
                  </button>
                )}
              </For>
            </div>
          </Show>

          <Switch>
            <Match when={method() === 'passkey'}>
              <p class="text-sm text-muted-foreground">{t('mfa.passkey_hint')}</p>
              <Button type="button" disabled={busy()} onClick={() => passkey(true)}><KeyRound /> {t('mfa.use_passkey')}</Button>
            </Match>
            <Match when={true}>
              <form class="grid gap-4" onSubmit={submitCode}>
                <Show when={method() === 'email'}>
                  <div class="flex items-center justify-between gap-2 text-sm text-muted-foreground">
                    <span>{codeSent() ? t('mfa.email_sent') : t('mfa.email_hint')}</span>
                    <Button type="button" size="sm" variant="outline" disabled={busy()} onClick={sendCode}>
                      {codeSent() ? t('email.resend') : t('email.send_code')}
                    </Button>
                  </div>
                </Show>
                <Field label={method() === 'recovery' ? t('mfa.recovery_code') : t('email.code')} hint={method() === 'totp' ? t('mfa.totp_hint') : undefined}>
                  <Input
                    autofocus
                    required
                    autocomplete="one-time-code"
                    inputmode={method() === 'recovery' ? 'text' : 'numeric'}
                    class="font-mono tracking-widest"
                    value={code()}
                    onInput={(e) => setCode(e.currentTarget.value)}
                  />
                </Field>
                <Show when={error()}>
                  <p role="alert" class="text-sm text-destructive">{error()}</p>
                </Show>
                <Button type="submit" disabled={busy() || !code().trim()}>{t('email.verify')}</Button>
              </form>
            </Match>
          </Switch>
          <Show when={method() === 'passkey' && error()}>
            <p role="alert" class="text-sm text-destructive">{error()}</p>
          </Show>
        </div>
      </Show>
    </AuthCard>
  );
}
