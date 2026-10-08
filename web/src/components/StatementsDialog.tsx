import { FileUp, X } from 'lucide-solid';
import { createEffect, createResource, createSignal, For, Match, Show, Switch } from 'solid-js';
import { api } from '~/api/client';
import type { ImportRowInput, SourceAccount } from '~/api/types';
import { Button } from '~/components/ui/button';
import { Dialog } from '~/components/ui/dialog';
import { Field, Input, Select } from '~/components/ui/input';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';
import { prepareFile } from '~/lib/attachments';
import { cn } from '~/lib/cn';
import { readReceipt } from '~/lib/receipts';
import { readSheetStatement, readStatement, type Statement, type StatementError } from '~/lib/statements';
import { xlsxRows } from '~/lib/xlsx';
import { useBook } from '~/stores/book';
import { continentalSheetNumber, isContinentalSheet } from '@sync/statements/continental.ts';

const base64 = (buf: ArrayBuffer) => {
  let s = '';
  const b = new Uint8Array(buf);
  for (let i = 0; i < b.length; i += 0x8000) s += String.fromCharCode(...b.subarray(i, i + 0x8000));
  return btoa(s);
};

type Problem = StatementError | 'unreadable' | 'other';
const problems: Problem[] = ['password', 'unrecognised', 'unbalanced', 'unreadable'];
type Status = 'reading' | 'ready' | 'account' | 'importing' | 'imported' | 'failed' | Problem;
type Kind = 'pdf' | 'xlsx' | 'image';

interface Item {
  key: number;
  file: File;
  kind: Kind;
  status: Status;
  statement?: Statement;
  sheet?: string[][]; // a ContiWeb export waiting for its account
  result?: { staged: number; duplicates: number };
}

const kindOf = (f: File): Kind | null =>
  f.type === 'application/pdf' || /\.pdf$/i.test(f.name)
    ? 'pdf'
    : /\.xlsx?$/i.test(f.name) || f.type.includes('spreadsheet') || f.type === 'application/vnd.ms-excel'
      ? 'xlsx'
      : f.type.startsWith('image/')
        ? 'image'
        : null;

const continentalNumber = (s: SourceAccount) => s.external_id.replace(/^py-continental-/, '');

/**
 * Statement files into the review queue (#42, #88): as many PDFs, ContiWeb
 * XLS exports and receipt photos (#44) as the person drops, read one at a
 * time in this tab. A password is tried on every locked PDF and never sent;
 * each recognised file is staged as its own batch with the original
 * attached, so a file imported twice stages nothing the second time.
 */
export function StatementsDialog(props: { open: boolean; onOpenChange: (o: boolean) => void; onImported: () => void }) {
  const { t, te } = useI18n();
  const book = useBook();
  const [items, setItems] = createSignal<Item[]>([]);
  const [password, setPassword] = createSignal('');
  const [busy, setBusy] = createSignal(false);
  const [dragging, setDragging] = createSignal(false);
  // A ContiWeb export does not say which account it is: the ones already known (from a sync or a PDF).
  const [sources] = createResource(
    () => (props.open ? book.id() : false),
    (id) => api.importSources(id).catch(() => [] as SourceAccount[]),
  );
  const continental = () => (sources() ?? []).filter((s) => s.external_id.startsWith('py-continental-') && s.currency);
  let next = 0;

  createEffect(() => {
    if (!props.open) return;
    setItems([]);
    setPassword('');
  });

  const update = (key: number, patch: Partial<Item>) => setItems((xs) => xs.map((x) => (x.key === key ? { ...x, ...patch } : x)));
  const fail = (err: unknown): Status => {
    const m = (err instanceof Error ? err.message : '') as Problem;
    return problems.includes(m) ? m : 'other';
  };

  const fromSheet = (item: Item, sheet: string[][], source?: SourceAccount) => {
    const number = source ? continentalNumber(source) : continentalSheetNumber(sheet);
    const known = source ?? continental().find((s) => continentalNumber(s) === number);
    if (!known || !number) {
      update(item.key, { status: 'account', sheet });
      return;
    }
    try {
      const label = known.label.replace(/^Continental\s+/, '').replace(/\s+\*+\d+$/, '');
      const st = readSheetStatement(sheet, { number, label, currency: known.currency as 'PYG' | 'USD' });
      update(item.key, { status: 'ready', statement: st, sheet: undefined });
    } catch (err) {
      update(item.key, { status: fail(err), sheet: undefined });
    }
  };

  const read = async (item: Item) => {
    update(item.key, { status: 'reading' });
    try {
      const buf = await item.file.arrayBuffer();
      if (item.kind === 'xlsx') {
        const sheet = await xlsxRows(buf);
        if (!isContinentalSheet(sheet)) throw new Error('unrecognised');
        fromSheet(item, sheet);
        return;
      }
      if (item.kind === 'image') {
        update(item.key, { status: 'ready', statement: await readReceipt(buf, item.file.type, '', 'statement') });
        return;
      }
      try {
        update(item.key, { status: 'ready', statement: await readStatement(buf, password(), 'statement') });
      } catch (err) {
        // Not a statement: perhaps a KuDE someone saved as a PDF.
        if (!(err instanceof Error && err.message === 'unrecognised')) throw err;
        update(item.key, { status: 'ready', statement: await readReceipt(buf, 'application/pdf', password(), 'statement') });
      }
    } catch (err) {
      update(item.key, { status: fail(err) });
    }
  };

  const add = async (files: FileList | null | undefined) => {
    const fresh: Item[] = [];
    for (const f of [...(files ?? [])]) {
      const kind = kindOf(f);
      // The same file dropped twice is listed once.
      if (items().some((x) => x.file.name === f.name && x.file.size === f.size)) continue;
      fresh.push({ key: next++, file: f, kind: kind ?? 'pdf', status: kind ? 'reading' : 'unrecognised' });
    }
    setItems((xs) => [...xs, ...fresh]);
    for (const item of fresh) if (item.status === 'reading') await read(item); // one at a time: memory stays flat
  };

  const retryLocked = async () => {
    for (const item of items().filter((x) => x.status === 'password')) await read(item);
  };

  const importAll = async () => {
    setBusy(true);
    let files = 0;
    let staged = 0;
    let duplicates = 0;
    for (const item of items().filter((x) => x.status === 'ready')) {
      const st = item.statement!;
      update(item.key, { status: 'importing' });
      try {
        // The QR was read from the original; a photo is sent downscaled, as attachments are.
        const upload = item.kind === 'image' ? await prepareFile(item.file) : { name: item.file.name, blob: item.file as Blob };
        // Only images and PDFs can be attached: an XLS export brings its rows alone, as a sync does.
        const attach = item.kind !== 'xlsx';
        const res = await api.importBatch(book.id(), {
          connector: 'statement',
          label: st.name,
          accounts: [{ id: st.account.id, label: st.account.label, currency: st.account.currency }],
          rows: st.rows.map((r) => ({ ...r, kind: r.kind as ImportRowInput['kind'], amount: r.amount ?? '0', file: attach ? r.file : undefined })),
          files: attach ? [{ ref: 'statement', filename: upload.name, data: base64(await upload.blob.arrayBuffer()) }] : [],
        });
        update(item.key, { status: 'imported', result: res });
        files++;
        staged += res.staged;
        duplicates += res.duplicates;
      } catch (err) {
        update(item.key, { status: 'failed' });
        toast.error(`${item.file.name}: ${te(err)}`);
      }
    }
    setBusy(false);
    if (files) {
      toast.success(t('imports.files_done', { files, staged, duplicates }));
      props.onImported();
    }
  };

  const ready = () => items().filter((x) => x.status === 'ready').length;
  const locked = () => items().some((x) => x.status === 'password');
  const period = (st: Statement) => {
    const days = st.rows.map((r) => r.date).sort();
    return days.length ? `${days[0]} – ${days[days.length - 1]}` : '';
  };

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange} title={t('imports.files_title')} description={t('imports.files_hint')} class="max-w-2xl">
      <div class="grid gap-4">
        <label
          class={cn(
            'grid cursor-pointer place-items-center gap-1 rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground',
            dragging() && 'border-primary bg-accent/40',
          )}
          onDragOver={(e) => {
            e.preventDefault();
            setDragging(true);
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={(e) => {
            e.preventDefault();
            setDragging(false);
            void add(e.dataTransfer?.files);
          }}
        >
          <FileUp class="size-5" />
          <span class="font-medium text-foreground">{t('imports.files_pick')}</span>
          <span>{t('imports.files_drop')}</span>
          <input
            type="file"
            multiple
            class="sr-only"
            accept=".pdf,application/pdf,.xls,.xlsx,image/*"
            aria-label={t('imports.files_pick')}
            onChange={(e) => {
              void add(e.currentTarget.files);
              e.currentTarget.value = '';
            }}
          />
        </label>

        <Field label={t('imports.pdf_password')} hint={t('imports.pdf_password_hint')}>
          <div class="flex gap-2">
            <Input
              type="password"
              autocomplete="off"
              value={password()}
              onInput={(e) => setPassword(e.currentTarget.value)}
              onKeyDown={(e) => e.key === 'Enter' && retryLocked()}
            />
            <Show when={locked()}>
              <Button variant="outline" disabled={busy() || !password()} onClick={retryLocked}>{t('imports.pdf_open')}</Button>
            </Show>
          </div>
        </Field>

        <Show when={items().length}>
          <ul class="grid divide-y rounded-md border text-sm" data-testid="statement-files">
            <For each={items()}>
              {(item) => (
                <li class="flex items-start gap-3 p-3">
                  <div class="min-w-0 flex-1">
                    <div class="truncate font-medium">{item.statement?.name ?? item.file.name}</div>
                    <Show when={item.statement}>
                      <div class="truncate text-xs text-muted-foreground">{item.file.name}</div>
                    </Show>
                    <div class="mt-1">
                      <Switch fallback={<span class="text-destructive">{t(`imports.pdf_error_${item.status}`)}</span>}>
                        <Match when={item.status === 'reading' || item.status === 'importing'}>
                          <span class="text-muted-foreground">{t('imports.files_reading')}</span>
                        </Match>
                        <Match when={item.status === 'ready' && item.statement}>
                          {(st) => (
                            <span class="text-muted-foreground tabular-nums">
                              {st().account.label} · {t('imports.files_rows', { rows: st().rows.length })} · {period(st())}
                            </span>
                          )}
                        </Match>
                        <Match when={item.status === 'imported' && item.result}>
                          {(r) => <span class="text-positive">{t('imports.csv_done', { staged: r().staged, duplicates: r().duplicates })}</span>}
                        </Match>
                        <Match when={item.status === 'failed'}>
                          <span class="text-destructive">{t('imports.files_failed')}</span>
                        </Match>
                        <Match when={item.status === 'account'}>
                          <Show when={continental().length} fallback={<span class="text-destructive">{t('imports.files_no_account')}</span>}>
                            <Field label={t('imports.files_account')}>
                              <Select
                                value=""
                                onChange={(e) => {
                                  const s = continental().find((x) => x.id === Number(e.currentTarget.value));
                                  if (s && item.sheet) fromSheet(item, item.sheet, s);
                                }}
                              >
                                <option value="" disabled>{t('imports.files_choose')}</option>
                                <For each={continental()}>{(s) => <option value={s.id}>{s.label}</option>}</For>
                              </Select>
                            </Field>
                          </Show>
                        </Match>
                      </Switch>
                    </div>
                  </div>
                  <Show when={item.status !== 'importing' && item.status !== 'imported'}>
                    <button
                      type="button"
                      class="rounded-md p-1 text-muted-foreground hover:bg-accent"
                      aria-label={t('common.remove')}
                      onClick={() => setItems((xs) => xs.filter((x) => x.key !== item.key))}
                    >
                      <X class="size-4" />
                    </button>
                  </Show>
                </li>
              )}
            </For>
          </ul>
        </Show>

        <div class="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>{t('imports.files_close')}</Button>
          <Button disabled={busy() || !ready()} onClick={importAll}>
            <FileUp /> {t('imports.files_submit', { files: ready() })}
          </Button>
        </div>
      </div>
    </Dialog>
  );
}
