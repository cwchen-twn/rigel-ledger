import { Copy, Download, KeyRound, Mail, Smartphone } from 'lucide-solid';
import { createSignal, For, Show, type JSX } from 'solid-js';
import { api } from '~/api/client';
import type { Enrolled, TOTPSetup } from '~/api/types';
import { Button } from '~/components/ui/button';
import { Field, Input } from '~/components/ui/input';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';
import { createPasskey, isCancelled, passkeysSupported } from '~/lib/webauthn';

/** The recovery codes, shown once: copy or download them now. */
export function RecoveryCodes(props: { codes: string[] }) {
  const { t } = useI18n();
  const text = () => props.codes.join('\n') + '\n';
  const copy = async () => {
    await navigator.clipboard.writeText(text());
    toast.success(t('mfa.codes_copied'));
  };
  const download = () => {
    const a = document.createElement('a');
    a.href = URL.createObjectURL(new Blob([text()], { type: 'text/plain' }));
    a.download = 'rigel-ledger-recovery-codes.txt';
    a.click();
    URL.revokeObjectURL(a.href);
  };
  return (
    <div class="grid gap-3 rounded-lg border bg-muted/40 p-4">
      <p class="text-sm font-medium">{t('mfa.codes_title')}</p>
      <p class="text-xs text-muted-foreground">{t('mfa.codes_hint')}</p>
      <ol class="grid grid-cols-2 gap-x-6 gap-y-1 font-mono text-sm tabular-nums">
        <For each={props.codes}>{(c) => <li>{c}</li>}</For>
      </ol>
      <div class="flex flex-wrap gap-2">
        <Button type="button" size="sm" variant="outline" onClick={copy}><Copy /> {t('mfa.copy')}</Button>
        <Button type="button" size="sm" variant="outline" onClick={download}><Download /> {t('mfa.download')}</Button>
      </div>
    </div>
  );
}

type Done = (e: Enrolled) => void;

function useRun() {
  const { te } = useI18n();
  const [busy, setBusy] = createSignal(false);
  const [error, setError] = createSignal('');
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
  return { busy, error, run };
}

function Section(props: { icon: JSX.Element; title: string; hint: string; children: JSX.Element }) {
  return (
    <div class="grid gap-3">
      <div class="flex items-start gap-3">
        <div class="mt-0.5 text-muted-foreground [&_svg]:size-5">{props.icon}</div>
        <div class="grid gap-0.5">
          <p class="text-sm font-medium">{props.title}</p>
          <p class="text-xs text-muted-foreground">{props.hint}</p>
        </div>
      </div>
      <div class="pl-8">{props.children}</div>
    </div>
  );
}

/** Authenticator app: a QR code, then a first code to prove it works. */
export function TOTPEnroll(props: { onDone: Done }) {
  const { t } = useI18n();
  const { busy, error, run } = useRun();
  const [setup, setSetup] = createSignal<TOTPSetup | null>(null);
  const [code, setCode] = createSignal('');
  return (
    <Section icon={<Smartphone />} title={t('mfa.method_totp')} hint={t('mfa.totp_setup_hint')}>
      <Show
        when={setup()}
        fallback={<Button type="button" variant="outline" disabled={busy()} onClick={() => run(async () => { setSetup(await api.mfa.startTOTP()); })}>{t('mfa.set_up')}</Button>}
      >
        {(s) => (
          <form
            class="grid gap-3"
            onSubmit={(e) => {
              e.preventDefault();
              void run(async () => props.onDone(await api.mfa.confirmTOTP(code().trim())));
            }}
          >
            <img src={s().qr} alt={t('mfa.qr_alt')} class="size-44 rounded-md border bg-white p-2" />
            <p class="text-xs text-muted-foreground">
              {t('mfa.secret')}: <code class="select-all font-mono">{s().secret}</code>
            </p>
            <div class="flex flex-wrap items-end gap-2">
              <Field label={t('email.code')}>
                <Input inputmode="numeric" autocomplete="one-time-code" maxLength={6} required class="w-36 font-mono tracking-widest"
                  value={code()} onInput={(e) => setCode(e.currentTarget.value.replace(/\D/g, ''))} />
              </Field>
              <Button type="submit" disabled={busy() || code().length !== 6}>{t('email.verify')}</Button>
            </div>
            <Show when={error()}><p role="alert" class="text-sm text-destructive">{error()}</p></Show>
          </form>
        )}
      </Show>
    </Section>
  );
}

/** Email codes: a code goes to the verified address to prove the mailbox. */
export function EmailEnroll(props: { onDone: Done; email: string }) {
  const { t } = useI18n();
  const { busy, error, run } = useRun();
  const [challenge, setChallenge] = createSignal('');
  const [code, setCode] = createSignal('');
  return (
    <Section icon={<Mail />} title={t('mfa.method_email')} hint={t('mfa.email_setup_hint', { email: props.email })}>
      <Show
        when={challenge()}
        fallback={<Button type="button" variant="outline" disabled={busy()} onClick={() => run(async () => { setChallenge((await api.mfa.startEmail()).challenge); })}>{t('email.send_code')}</Button>}
      >
        <form
          class="flex flex-wrap items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            void run(async () => props.onDone(await api.mfa.confirmEmail(challenge(), code().trim())));
          }}
        >
          <Field label={t('email.code')}>
            <Input inputmode="numeric" autocomplete="one-time-code" maxLength={6} required class="w-36 font-mono tracking-widest"
              value={code()} onInput={(e) => setCode(e.currentTarget.value.replace(/\D/g, ''))} />
          </Field>
          <Button type="submit" disabled={busy() || code().length !== 6}>{t('email.verify')}</Button>
        </form>
      </Show>
      <Show when={error()}><p role="alert" class="mt-2 text-sm text-destructive">{error()}</p></Show>
    </Section>
  );
}

/** A passkey on this device (or a security key / phone). */
export function PasskeyEnroll(props: { onDone: Done }) {
  const { t } = useI18n();
  const { busy, error, run } = useRun();
  const [name, setName] = createSignal('');
  return (
    <Section icon={<KeyRound />} title={t('mfa.method_passkey')} hint={t('mfa.passkey_setup_hint')}>
      <Show when={passkeysSupported()} fallback={<p class="text-xs text-muted-foreground">{t('mfa.passkey_unsupported')}</p>}>
        <form
          class="flex flex-wrap items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            void run(async () => {
              const begin = await api.mfa.passkeyBegin(name());
              const cred = await createPasskey(begin.options);
              props.onDone(await api.mfa.passkeyFinish(begin.challenge, cred));
              setName('');
            });
          }}
        >
          <Field label={t('mfa.passkey_name')}>
            <Input placeholder={t('mfa.passkey_name_placeholder')} maxLength={60} value={name()} onInput={(e) => setName(e.currentTarget.value)} />
          </Field>
          <Button type="submit" disabled={busy()}><KeyRound /> {t('mfa.add_passkey')}</Button>
        </form>
      </Show>
      <Show when={error()}><p role="alert" class="mt-2 text-sm text-destructive">{error()}</p></Show>
    </Section>
  );
}
