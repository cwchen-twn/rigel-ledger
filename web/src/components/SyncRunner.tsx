import { Copy, KeyRound, Unlink } from 'lucide-solid';
import { createEffect, createResource, createSignal, on, onCleanup, Show } from 'solid-js';
import { api } from '~/api/client';
import { Button } from '~/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '~/components/ui/card';
import { Badge } from '~/components/ui/misc';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';
import { formatDateTime } from '~/lib/dates';

/** The compose file a person runs their runner with (docs/ARCHITECTURE.md). */
const compose = (origin: string) => `services:
  rigel-sync:
    image: ghcr.io/cwchen-twn/rigel-ledger-sync:latest
    restart: unless-stopped
    environment:
      RIGEL_URL: ${origin}
      RUNNER_TOKEN: \${RUNNER_TOKEN}
    volumes:
      - rigel-sync:/data
volumes:
  rigel-sync:
`;

/**
 * Settings -> Sync runner: a person's own runner, like a self-hosted CI
 * runner. It runs wherever they put it (a computer they leave on, or the
 * server next to the app) and is linked with a token made here. One per
 * person: linking again revokes the token before.
 */
export function SyncRunner() {
  const { t, te } = useI18n();
  const [status, { refetch }] = createResource(() => api.runner());
  const [made, setMade] = createSignal('');
  const [busy, setBusy] = createSignal(false);
  const token = () => status()?.tokens[0];
  const key = () => status()?.keys.find((k) => !k.retired_at);
  const seen = () => token()?.last_used_at ?? undefined;
  // Linked once the runner has used its token and registered a key; until
  // then (a new token too), look again now and then.
  const state = () => (!token() ? 'not_linked' : key() && seen() ? 'linked' : 'waiting');
  let timer: ReturnType<typeof setInterval> | undefined;
  createEffect(
    on(state, (st) => {
      clearInterval(timer);
      if (st === 'waiting') timer = setInterval(() => refetch(), 3000);
    }),
  );
  onCleanup(() => clearInterval(timer));
  // Connections links here (/settings#runner); the router does not scroll.
  createEffect(
    on(status, (st) => {
      if (st && location.hash === '#runner') document.getElementById('runner')?.scrollIntoView({ block: 'start' });
    }),
  );

  const link = async () => {
    if (token() && !confirm(t('runner.relink_confirm'))) return;
    setBusy(true);
    try {
      const res = await api.linkRunner(t('runner.default_label'), 365);
      setMade(res.token);
      refetch();
    } catch (err) {
      toast.error(te(err));
    } finally {
      setBusy(false);
    }
  };
  const unlink = async (id: number) => {
    if (!confirm(t('runner.unlink_confirm'))) return;
    try {
      await api.unlinkRunner(id);
      setMade('');
      toast.success(t('runner.unlinked'));
    } catch (err) {
      toast.error(te(err));
    }
    refetch();
  };
  const copy = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      toast.success(t('tokens.copied'));
    } catch {
      toast.error(t('tokens.copy_failed'));
    }
  };

  return (
    <Card id="runner" data-testid="sync-runner">
      <CardHeader>
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="grid gap-1">
            <CardTitle class="flex flex-wrap items-center gap-2">
              {t('runner.title')}
              <Show when={status()}>
                <Badge variant={state() === 'linked' ? 'secondary' : state() === 'waiting' ? 'warning' : 'outline'}>{t(`runner.${state()}`)}</Badge>
              </Show>
            </CardTitle>
            <CardDescription>{t('runner.hint')}</CardDescription>
          </div>
          <div class="flex flex-wrap gap-2">
            <Button size="sm" variant="outline" disabled={busy() || !status()} onClick={link}>
              <KeyRound /> {token() ? t('runner.relink') : t('runner.link')}
            </Button>
            <Show when={token()}>
              {(x) => (
                <Button size="sm" variant="ghost" onClick={() => unlink(x().id)}>
                  <Unlink /> {t('runner.unlink')}
                </Button>
              )}
            </Show>
          </div>
        </div>
      </CardHeader>
      <CardContent class="grid gap-3 text-sm">
        <Show when={state() === 'linked'}>
          <p class="text-muted-foreground">
            {[seen() ? t('runner.seen', { when: formatDateTime(seen()!) }) : '', t('runner.offers', { n: status()?.connectors.length ?? 0 })]
              .filter(Boolean)
              .join(' · ')}
          </p>
        </Show>
        <Show when={made()}>
          <div class="grid gap-2 rounded-md border border-amber-500/40 p-3">
            <p>{t('runner.token_hint')}</p>
            <code class="rounded-md border bg-muted px-3 py-2 font-mono break-all" data-testid="new-runner-token">{made()}</code>
            <div class="flex flex-wrap gap-2">
              <Button size="sm" variant="outline" onClick={() => copy(made())}><Copy /> {t('tokens.copy')}</Button>
              <Button size="sm" onClick={() => setMade('')}>{t('tokens.done')}</Button>
            </div>
          </div>
        </Show>
        <Show when={state() !== 'linked'}>
          <ol class="list-decimal space-y-1 pl-5 text-muted-foreground">
            <li>{t('runner.step_install')}</li>
            <li>{t('runner.step_token')}</li>
            <li>{t('runner.step_run')}</li>
          </ol>
          <div class="grid gap-2">
            <p class="text-muted-foreground">{t('runner.compose_hint')}</p>
            <pre class="overflow-x-auto rounded-md border bg-muted px-3 py-2 font-mono text-xs">{compose(location.origin)}</pre>
            <Button size="sm" variant="outline" class="w-fit" onClick={() => copy(compose(location.origin))}><Copy /> {t('tokens.copy')}</Button>
          </div>
        </Show>
      </CardContent>
    </Card>
  );
}
