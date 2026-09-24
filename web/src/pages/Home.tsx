import { Navigate } from '@solidjs/router';
import { createResource, Match, Switch } from 'solid-js';
import { api } from '~/api/client';
import { ErrorState } from '~/components/ui/misc';
import { useI18n } from '~/i18n';
import { useSession } from '~/stores/session';

/** "/" goes to the default book, else the first book, else onboarding. */
export default function Home() {
  const { t, te } = useI18n();
  const { user } = useSession();
  const [books, { refetch }] = createResource(() => api.books());
  const target = () => {
    const bs = books();
    if (!bs) return null;
    const def = user()?.default_book_id;
    if (def && bs.some((b) => b.id === def)) return `/b/${def}`;
    return bs.length ? `/b/${bs[0].id}` : '/onboarding';
  };
  return (
    <Switch>
      {/* Before: any failure here left the page empty (an old bundle meeting
          a new API rendered nothing at all). */}
      <Match when={books.error}>
        <div class="flex min-h-screen items-center justify-center p-4">
          <div class="w-full max-w-md">
            <ErrorState title={t('error.load_failed')} message={te(books.error)} retryLabel={t('common.retry')} onRetry={refetch} />
          </div>
        </div>
      </Match>
      <Match when={target()}>{(to) => <Navigate href={to()} />}</Match>
    </Switch>
  );
}
