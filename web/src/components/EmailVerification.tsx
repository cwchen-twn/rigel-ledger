import { createSignal, Show } from 'solid-js';
import { api } from '~/api/client';
import type { User } from '~/api/types';
import { Button } from '~/components/ui/button';
import { Field, Input } from '~/components/ui/input';
import { Badge } from '~/components/ui/misc';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';

/**
 * Verify the current address or change to a new one: a 6-digit code is
 * mailed, and the address changes only when the code comes back. After the
 * first-login wizard the current password is asked for too.
 */
export function EmailVerification(props: {
  user: User;
  needsPassword: boolean;
  onChange: (u: User) => void;
  /** Called after a code is sent or a pending change is dropped. */
  onPending?: () => void;
}) {
  const { t, te, fieldErrors } = useI18n();
  const [email, setEmail] = createSignal(props.user.pending_email || props.user.email);
  const [password, setPassword] = createSignal('');
  const [code, setCode] = createSignal('');
  const [sentTo, setSentTo] = createSignal(props.user.pending_email ?? '');
  const [errors, setErrors] = createSignal<Record<string, string>>({});
  const [busy, setBusy] = createSignal(false);

  const verified = () => props.user.email_verified && email().trim().toLowerCase() === props.user.email.toLowerCase();

  const send = async (e?: Event) => {
    e?.preventDefault();
    setBusy(true);
    try {
      await api.startEmail(email().trim(), password());
      setSentTo(email().trim());
      setErrors({});
      setCode('');
      toast.success(t('email.code_sent', { email: email().trim() }));
      props.onPending?.();
    } catch (err) {
      setErrors(fieldErrors(err));
      toast.error(te(err));
    } finally {
      setBusy(false);
    }
  };

  const confirm = async (e: Event) => {
    e.preventDefault();
    setBusy(true);
    try {
      const u = await api.confirmEmail(code().trim());
      setSentTo('');
      setPassword('');
      setErrors({});
      props.onChange(u);
      toast.success(t('email.verified'));
    } catch (err) {
      setErrors(fieldErrors(err));
      toast.error(te(err));
    } finally {
      setBusy(false);
    }
  };

  const cancel = async () => {
    await api.cancelEmail();
    setSentTo('');
    setEmail(props.user.email);
    props.onPending?.();
  };

  return (
    <div class="grid gap-4">
      <form class="grid gap-4 sm:grid-cols-[1fr_1fr_auto] sm:items-end" onSubmit={send}>
        <Field
          label={
            <span class="flex items-center gap-2">
              {t('settings.email')}
              <Show when={props.user.email}>
                <Badge variant={props.user.email_verified ? 'secondary' : 'warning'}>
                  {props.user.email_verified ? t('email.verified_badge') : t('email.unverified_badge')}
                </Badge>
              </Show>
            </span>
          }
          error={errors().email}
        >
          <Input type="email" autocomplete="email" required value={email()} onInput={(e) => setEmail(e.currentTarget.value)} />
        </Field>
        <Show when={props.needsPassword} fallback={<span class="hidden sm:block" />}>
          <Field label={t('settings.current_password')} error={errors().current_password}>
            <Input type="password" autocomplete="current-password" required value={password()} onInput={(e) => setPassword(e.currentTarget.value)} />
          </Field>
        </Show>
        <Button type="submit" variant="outline" disabled={busy() || verified()}>
          {sentTo() ? t('email.resend') : t('email.send_code')}
        </Button>
      </form>

      <Show when={sentTo()}>
        <form class="grid gap-3 rounded-lg border bg-muted/40 p-4" onSubmit={confirm}>
          <p class="text-sm">{t('email.enter_code', { email: sentTo() })}</p>
          <div class="flex flex-wrap items-end gap-2">
            <Field label={t('email.code')} error={errors().code}>
              <Input
                inputmode="numeric"
                autocomplete="one-time-code"
                pattern="[0-9]{6}"
                maxLength={6}
                required
                class="w-36 font-mono tracking-widest"
                value={code()}
                onInput={(e) => setCode(e.currentTarget.value.replace(/\D/g, ''))}
              />
            </Field>
            <Button type="submit" disabled={busy() || code().length !== 6}>{t('email.verify')}</Button>
            <Button type="button" variant="ghost" onClick={cancel}>{t('common.cancel')}</Button>
          </div>
        </form>
      </Show>
    </div>
  );
}
