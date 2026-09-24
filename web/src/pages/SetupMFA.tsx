import { useNavigate } from '@solidjs/router';
import { createResource, createSignal, Show } from 'solid-js';
import { api } from '~/api/client';
import type { Enrolled } from '~/api/types';
import { AuthCard } from '~/components/AuthCard';
import { EmailEnroll, PasskeyEnroll, RecoveryCodes, TOTPEnroll } from '~/components/MFAEnroll';
import { Button } from '~/components/ui/button';
import { Checkbox } from '~/components/ui/input';
import { useI18n } from '~/i18n';
import { useSession } from '~/stores/session';

/**
 * /setup-2fa: two-factor sign-in is required and this session has not passed
 * a second factor. Adding one raises the session; the recovery codes are
 * shown once before the app opens.
 */
export default function SetupMFA() {
  const { t } = useI18n();
  const { user, refresh, logout } = useSession();
  const navigate = useNavigate();
  const [status] = createResource(() => api.mfa.status());
  const [codes, setCodes] = createSignal<string[] | null>(null);
  const [saved, setSaved] = createSignal(false);
  const allowed = (m: string) => status()?.allowed.includes(m as never) ?? false;

  const done = (e: Enrolled) => {
    if (e.recovery_codes?.length) {
      setCodes(e.recovery_codes);
      return;
    }
    void finish();
  };
  // Wait for the raised session before leaving: the guard reads it, and a
  // stale level-1 user would send us straight back here.
  const finish = async () => {
    await refresh();
    navigate('/', { replace: true });
  };

  return (
    <AuthCard
      wide
      title={codes() ? t('mfa.codes_title') : t('mfa.setup_title')}
      description={codes() ? undefined : t('mfa.setup_description')}
      footer={<button type="button" class="underline-offset-4 hover:underline" onClick={() => logout()}>{t('nav.logout')}</button>}
    >
      <Show
        when={codes()}
        fallback={
          <div class="grid gap-6">
            <Show when={allowed('passkey')}><PasskeyEnroll onDone={done} /></Show>
            <Show when={allowed('totp')}><TOTPEnroll onDone={done} /></Show>
            <Show when={allowed('email') && user()?.email_verified}><EmailEnroll onDone={done} email={user()?.email ?? ''} /></Show>
          </div>
        }
      >
        {(c) => (
          <div class="grid gap-4">
            <RecoveryCodes codes={c()} />
            <Checkbox label={t('mfa.codes_saved')} checked={saved()} onChange={(e) => setSaved(e.currentTarget.checked)} />
            <Button type="button" disabled={!saved()} onClick={finish}>{t('welcome.finish')}</Button>
          </div>
        )}
      </Show>
    </AuthCard>
  );
}
