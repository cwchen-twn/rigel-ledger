import { KeyRound, Link2, Plus, RefreshCw, ShieldCheck, Trash2 } from 'lucide-solid';
import { createEffect, createMemo, createResource, createSignal, For, on, onCleanup, Show } from 'solid-js';
import { api } from '~/api/client';
import type { Connection, Connector } from '~/api/types';
import { PageHeader } from '~/components/AppShell';
import { Button } from '~/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '~/components/ui/card';
import { Dialog } from '~/components/ui/dialog';
import { Checkbox, Field, Input, Select } from '~/components/ui/input';
import { Badge, EmptyState, Skeleton } from '~/components/ui/misc';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';
import { formatDateTime } from '~/lib/dates';
import { aad, canSeal, seal } from '~/lib/seal';
import { useSession } from '~/stores/session';

const INTERVALS = [6, 12, 24, 48, 168];

const STATUS: Record<Connection['status'], 'secondary' | 'outline' | 'warning' | 'destructive'> = {
  new: 'outline',
  ok: 'secondary',
  needs_user_action: 'warning',
  failed: 'destructive',
};

/** A translation when there is one, else the runner's own English. */
function useLabel() {
  const { t } = useI18n();
  return (key: string, fallback: string) => {
    const s = t(key);
    return s === key ? fallback : s;
  };
}

/**
 * Settings -> Connections: link your own institutions. What you type into a
 * connector's fields is sealed here in the browser to the sync runner's key,
 * so the server stores ciphertext it cannot read; only the runner opens it,
 * while it signs in for you. Rows it fetches wait in the book's review queue.
 */
export default function Connections() {
  const { t, te } = useI18n();
  const label = useLabel();
  const [catalog, { refetch: refetchCatalog }] = createResource(() => api.connectors());
  const [list, { refetch }] = createResource(() => api.connections());
  const [supported] = createResource(() => canSeal());
  const [adding, setAdding] = createSignal(false);
  const [replacing, setReplacing] = createSignal<Connection | null>(null);
  const [deleting, setDeleting] = createSignal<Connection | null>(null);

  const byId = createMemo(() => new Map((catalog()?.connectors ?? []).map((c) => [c.id, c])));
  const name = (c: Connection) => {
    const k = byId().get(c.connector);
    return label(`connector.${c.connector}.name`, k?.name ?? c.connector);
  };
  const ready = () => !!catalog()?.key && (catalog()?.connectors.length ?? 0) > 0 && supported() === true;

  // While the runner is (or is about to be) working on one of them, follow along.
  const busy = () =>
    (list() ?? []).some((c) => c.enabled && !c.key_retired && (c.run_requested || c.status === 'new' || c.status === 'needs_user_action'));
  let timer: ReturnType<typeof setInterval> | undefined;
  createEffect(
    on(busy, (b) => {
      clearInterval(timer);
      if (b) timer = setInterval(() => refetch(), 3000);
    }),
  );
  onCleanup(() => clearInterval(timer));

  const act = async (fn: () => Promise<void>, ok?: string) => {
    try {
      await fn();
      if (ok) toast.success(ok);
    } catch (err) {
      toast.error(te(err));
    }
    refetch();
  };

  return (
    <>
      <PageHeader
        title={t('connections.title')}
        description={t('connections.description')}
        actions={
          <Button disabled={!ready()} onClick={() => setAdding(true)}>
            <Plus /> {t('connections.add')}
          </Button>
        }
      />
      <div class="grid gap-6">
        <Show when={supported() === false}>
          <p class="rounded-md border border-destructive/40 p-3 text-sm text-destructive">{t('connections.browser_too_old')}</p>
        </Show>
        <Show when={catalog() && !catalog()!.key}>
          <p class="rounded-md border p-3 text-sm text-muted-foreground">{t('connections.no_runner')}</p>
        </Show>

        <Show when={!list.loading || list()} fallback={<Skeleton class="h-40 w-full" />}>
          <Show
            when={(list() ?? []).length}
            fallback={
              <EmptyState icon={<Link2 />} title={t('connections.empty')}>
                <p class="text-sm text-muted-foreground">{t('connections.empty_hint')}</p>
              </EmptyState>
            }
          >
            <For each={list()}>
              {(c) => (
                <Card data-testid={`connection-${c.id}`}>
                  <CardHeader>
                    <div class="flex flex-wrap items-start justify-between gap-3">
                      <div class="grid min-w-0 gap-1">
                        <CardTitle class="flex flex-wrap items-center gap-2">
                          {c.label || name(c)}
                          <Badge variant={STATUS[c.status]}>{t(`connections.status_${c.status}`)}</Badge>
                          <Show when={!c.enabled}><Badge variant="outline">{t('connections.paused')}</Badge></Show>
                        </CardTitle>
                        <CardDescription>
                          {[c.label ? name(c) : '', t('connections.into_book', { book: c.book_name }),
                            c.last_run_at ? t('connections.last_run', { when: formatDateTime(c.last_run_at) }) : t('connections.never_run')]
                            .filter(Boolean).join(' · ')}
                        </CardDescription>
                      </div>
                      <div class="flex flex-wrap gap-2">
                        <Button size="sm" variant="outline" disabled={!c.enabled || c.run_requested || c.key_retired}
                          onClick={() => act(() => api.syncConnection(c.id), t('connections.sync_requested'))}>
                          <RefreshCw class={c.run_requested ? 'animate-spin' : ''} /> {c.run_requested ? t('connections.syncing') : t('connections.sync_now')}
                        </Button>
                        <Button size="sm" variant="outline" disabled={!ready()} onClick={() => setReplacing(c)}>
                          <KeyRound /> {t('connections.replace')}
                        </Button>
                        <Button size="sm" variant="ghost" aria-label={t('common.delete')} onClick={() => setDeleting(c)}>
                          <Trash2 />
                        </Button>
                      </div>
                    </div>
                  </CardHeader>
                  <CardContent class="grid gap-3">
                    <Show when={c.key_retired}>
                      <p class="text-sm text-destructive">{t('connections.key_retired')}</p>
                    </Show>
                    <Show when={c.status === 'failed' && c.last_error}>
                      <p class="text-sm text-destructive">{label(`connections.error_${c.last_error}`, t('connections.error_other', { code: c.last_error }))}</p>
                    </Show>
                    <Show when={c.challenge}>{(ch) => <ChallengeForm connection={c} challenge={ch()} onDone={refetch} />}</Show>
                    <div class="flex flex-wrap items-center gap-x-6 gap-y-2">
                      <Checkbox checked={c.enabled} label={t('connections.enabled')}
                        onChange={(e) => act(() => api.updateConnection(c.id, { enabled: e.currentTarget.checked }))} />
                      <label class="flex items-center gap-2 text-sm">
                        {t('connections.every')}
                        <Select class="h-8 w-auto" value={String(c.interval_hours)}
                          onChange={(e) => act(() => api.updateConnection(c.id, { interval_hours: Number(e.currentTarget.value) }))}>
                          <For each={INTERVALS.includes(c.interval_hours) ? INTERVALS : [...INTERVALS, c.interval_hours]}>
                            {(h) => <option value={String(h)}>{t('connections.hours', { n: h })}</option>}
                          </For>
                        </Select>
                      </label>
                    </div>
                  </CardContent>
                </Card>
              )}
            </For>
          </Show>
        </Show>

        <Card>
          <CardHeader>
            <CardTitle class="flex items-center gap-2"><ShieldCheck class="size-4" /> {t('connections.how_title')}</CardTitle>
          </CardHeader>
          <CardContent class="grid gap-2 text-sm text-muted-foreground">
            <p>{t('connections.how_sealed')}</p>
            <p>{t('connections.how_review')}</p>
            <p>{t('connections.how_csv')}</p>
          </CardContent>
        </Card>
      </div>

      <Show when={catalog()}>
        {(cat) => (
          <>
            <CredentialsDialog
              open={adding()}
              mode="add"
              connectors={cat().connectors}
              keyInfo={cat().key}
              onClose={() => setAdding(false)}
              onSaved={() => {
                refetch();
                refetchCatalog();
              }}
            />
            <CredentialsDialog
              open={!!replacing()}
              mode="replace"
              connection={replacing()}
              connectors={cat().connectors}
              keyInfo={cat().key}
              onClose={() => setReplacing(null)}
              onSaved={refetch}
            />
          </>
        )}
      </Show>
      <Dialog open={!!deleting()} onOpenChange={(o) => !o && setDeleting(null)} title={t('connections.delete_title')}
        description={t('connections.delete_hint')}>
        <div class="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
          <Button variant="outline" onClick={() => setDeleting(null)}>{t('common.cancel')}</Button>
          <Button variant="destructive" onClick={() => {
            const c = deleting()!;
            setDeleting(null);
            act(() => api.deleteConnection(c.id), t('connections.deleted'));
          }}>{t('connections.delete')}</Button>
        </div>
      </Dialog>
    </>
  );
}

/** The runner is waiting on a code, a CAPTCHA or a device check. */
function ChallengeForm(props: { connection: Connection; challenge: NonNullable<Connection['challenge']>; onDone: () => void }) {
  const { t, te } = useI18n();
  const [answer, setAnswer] = createSignal('');
  const [catalog] = createResource(() => api.connectors());
  const [busy, setBusy] = createSignal(false);
  const submit = async (e: Event) => {
    e.preventDefault();
    const key = catalog()?.key;
    if (!key) return;
    setBusy(true);
    try {
      const sealed = await seal(key.public_key, props.challenge.kind === 'device' ? 'confirmed' : answer().trim(), aad('answer', props.challenge.id));
      await api.answerChallenge(props.connection.id, props.challenge.id, sealed);
      toast.success(t('connections.answer_sent'));
      setAnswer('');
      props.onDone();
    } catch (err) {
      toast.error(te(err));
    } finally {
      setBusy(false);
    }
  };
  return (
    <form class="grid gap-3 rounded-md border border-amber-500/40 bg-amber-500/5 p-3" onSubmit={submit}>
      <p class="text-sm font-medium">{t(`connections.challenge_${props.challenge.kind}`)}</p>
      <Show when={props.challenge.prompt}><p class="text-sm text-muted-foreground">{props.challenge.prompt}</p></Show>
      <Show when={props.challenge.image}>
        <img class="max-h-24 w-fit rounded border bg-white" alt={t('connections.captcha_alt')} src={`data:image/png;base64,${props.challenge.image}`} />
      </Show>
      <div class="flex flex-wrap items-end gap-2">
        <Show when={props.challenge.kind !== 'device'}>
          <Field label={t('connections.answer')} class="min-w-0 flex-1">
            <Input required autocomplete="one-time-code" inputmode={props.challenge.kind === 'otp' ? 'numeric' : 'text'}
              value={answer()} onInput={(e) => setAnswer(e.currentTarget.value)} />
          </Field>
        </Show>
        <Button type="submit" disabled={busy() || (props.challenge.kind !== 'device' && !answer().trim())}>
          {props.challenge.kind === 'device' ? t('connections.device_done') : t('connections.send_answer')}
        </Button>
      </div>
      <p class="text-xs text-muted-foreground">{t('connections.expires', { when: formatDateTime(props.challenge.expires_at) })}</p>
    </form>
  );
}

function CredentialsDialog(props: {
  open: boolean;
  mode: 'add' | 'replace';
  connection?: Connection | null;
  connectors: Connector[];
  keyInfo: { id: number; public_key: string } | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t, te, fieldErrors } = useI18n();
  const label = useLabel();
  const { user } = useSession();
  const [books] = createResource(() => props.open && props.mode === 'add', () => api.books());
  const [connector, setConnector] = createSignal('');
  const [bookId, setBookId] = createSignal(0);
  const [name, setName] = createSignal('');
  const [interval, setInterval_] = createSignal(24);
  const [values, setValues] = createSignal<Record<string, string>>({});
  const [consent, setConsent] = createSignal(false);
  const [errors, setErrors] = createSignal<Record<string, string>>({});
  const [busy, setBusy] = createSignal(false);

  const editable = () => (books() ?? []).filter((b) => b.role === 'owner' || b.role === 'editor');
  createEffect(() => {
    if (!props.open) return;
    setValues({});
    setErrors({});
    setConsent(props.mode === 'replace');
    setName('');
    setInterval_(24);
    setConnector(props.connection?.connector ?? props.connectors[0]?.id ?? '');
  });
  createEffect(() => {
    const b = editable();
    if (props.open && b.length && !b.some((x) => x.id === bookId())) setBookId(user()?.default_book_id && b.some((x) => x.id === user()!.default_book_id) ? user()!.default_book_id! : b[0].id);
  });
  const current = () => props.connectors.find((c) => c.id === connector());
  const complete = () => !!current() && current()!.fields.every((f) => f.optional || (values()[f.name] ?? '').trim() !== '');

  const submit = async (e: Event) => {
    e.preventDefault();
    const c = current();
    const u = user();
    if (!c || !u || !props.keyInfo) return;
    setBusy(true);
    try {
      const creds: Record<string, string> = {};
      for (const f of c.fields) creds[f.name] = f.kind === 'id_number' ? (values()[f.name] ?? '').trim().toUpperCase() : (values()[f.name] ?? '');
      const sealed = await seal(props.keyInfo.public_key, JSON.stringify(creds), aad('credentials', u.id, c.id));
      setValues({}); // the plaintext does not outlive the request
      if (props.mode === 'add') {
        await api.createConnection({ book_id: bookId(), connector: c.id, label: name().trim(), interval_hours: interval(), key_id: props.keyInfo.id, sealed });
        toast.success(t('connections.added'));
      } else {
        await api.replaceCredentials(props.connection!.id, props.keyInfo.id, sealed);
        toast.success(t('connections.replaced'));
      }
      props.onSaved();
      props.onClose();
    } catch (err) {
      setErrors(fieldErrors(err));
      toast.error(te(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog
      open={props.open}
      onOpenChange={(o) => !o && props.onClose()}
      title={props.mode === 'add' ? t('connections.add') : t('connections.replace_title')}
      description={props.mode === 'add' ? t('connections.add_hint') : t('connections.replace_hint')}
    >
      <form class="grid gap-4" onSubmit={submit} autocomplete="off">
        <Show when={props.mode === 'add'}>
          <Field label={t('connections.connector')}>
            <Select value={connector()} onChange={(e) => { setConnector(e.currentTarget.value); setValues({}); }}>
              <For each={props.connectors}>{(c) => <option value={c.id}>{label(`connector.${c.id}.name`, c.name)}{c.country ? ` (${c.country})` : ''}</option>}</For>
            </Select>
          </Field>
          <div class="grid gap-4 sm:grid-cols-2">
            <Field label={t('connections.book')} hint={t('connections.book_hint')} error={errors().book_id}>
              <Select value={String(bookId())} onChange={(e) => setBookId(Number(e.currentTarget.value))}>
                <For each={editable()}>{(b) => <option value={String(b.id)}>{b.name}</option>}</For>
              </Select>
            </Field>
            <Field label={t('connections.every')}>
              <Select value={String(interval())} onChange={(e) => setInterval_(Number(e.currentTarget.value))}>
                <For each={INTERVALS}>{(h) => <option value={String(h)}>{t('connections.hours', { n: h })}</option>}</For>
              </Select>
            </Field>
          </div>
          <Field label={t('connections.label')} hint={t('connections.label_hint')} error={errors().label}>
            <Input maxLength={80} value={name()} placeholder={t('common.optional')} onInput={(e) => setName(e.currentTarget.value)} />
          </Field>
        </Show>

        <div class="grid gap-4 rounded-md border p-3">
          <p class="flex items-center gap-2 text-xs text-muted-foreground"><ShieldCheck class="size-4 shrink-0" /> {t('connections.sealed_note')}</p>
          <For each={current()?.fields ?? []}>
            {(f) => (
              <Field label={label(`connector.${connector()}.field.${f.name}`, f.label)}>
                <Input
                  type={f.kind === 'secret' ? 'password' : 'text'}
                  autocomplete={f.kind === 'secret' ? 'new-password' : 'off'}
                  autocapitalize={f.kind === 'id_number' ? 'characters' : 'off'}
                  spellcheck={false}
                  required={!f.optional}
                  value={values()[f.name] ?? ''}
                  onInput={(e) => setValues({ ...values(), [f.name]: e.currentTarget.value })}
                />
              </Field>
            )}
          </For>
        </div>

        <Show when={props.mode === 'add'}>
          <div class="grid gap-2 rounded-md border border-amber-500/40 p-3 text-sm">
            <p class="font-medium">{t('connections.consent_title')}</p>
            <ul class="list-disc space-y-1 pl-5 text-muted-foreground">
              <li>{t('connections.consent_unofficial')}</li>
              <li>{t('connections.consent_sessions')}</li>
              <li>{t('connections.consent_tos')}</li>
              <li>{t('connections.consent_fetch')}</li>
            </ul>
            <Checkbox checked={consent()} onChange={(e) => setConsent(e.currentTarget.checked)} label={t('connections.consent_agree')} />
          </div>
        </Show>

        <div class="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
          <Button type="button" variant="outline" onClick={props.onClose}>{t('common.cancel')}</Button>
          <Button type="submit" disabled={busy() || !complete() || !consent() || (props.mode === 'add' && !bookId())}>
            {props.mode === 'add' ? t('connections.connect') : t('common.save')}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
