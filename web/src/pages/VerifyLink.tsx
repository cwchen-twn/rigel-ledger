import { useNavigate, useSearchParams } from '@solidjs/router';
import { onMount, Show, createSignal } from 'solid-js';
import { api } from '~/api/client';
import { AuthCard } from '~/components/AuthCard';
import { useI18n } from '~/i18n';
import { useSession } from '~/stores/session';

/** /verify?token=... -- the sign-up link: creates the account and signs in. */
export default function VerifyLink() {
  const { t, te } = useI18n();
  const { setUser } = useSession();
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const [error, setError] = createSignal('');

  onMount(async () => {
    const token = typeof params.token === 'string' ? params.token : '';
    try {
      const { user } = await api.verifyLink(token);
      setUser(user);
      navigate('/', { replace: true });
    } catch (err) {
      setError(te(err));
    }
  });

  return (
    <Show when={error()} fallback={<AuthCard title={t('verify.working')} />}>
      <AuthCard
        title={t('verify.failed')}
        description={error()}
        footer={<a href="/login" class="underline-offset-4 hover:underline">{t('auth.sign_in')}</a>}
      />
    </Show>
  );
}
