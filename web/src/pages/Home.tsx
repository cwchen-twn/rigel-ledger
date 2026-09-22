import { Navigate } from '@solidjs/router';
import { createResource, Show } from 'solid-js';
import { api } from '~/api/client';
import { useSession } from '~/stores/session';

/** "/" goes to the default book, else the first book, else onboarding. */
export default function Home() {
  const { user } = useSession();
  const [books] = createResource(() => api.books());
  const target = () => {
    const bs = books();
    if (!bs) return null;
    const def = user()?.default_book_id;
    if (def && bs.some((b) => b.id === def)) return `/b/${def}`;
    return bs.length ? `/b/${bs[0].id}` : '/onboarding';
  };
  return <Show when={target()}>{(to) => <Navigate href={to()} />}</Show>;
}
