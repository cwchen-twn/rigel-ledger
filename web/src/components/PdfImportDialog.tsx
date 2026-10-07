import { FileUp } from 'lucide-solid';
import { createEffect, createSignal, For, Show } from 'solid-js';
import { api } from '~/api/client';
import type { ImportRowInput } from '~/api/types';
import { Money } from '~/components/Money';
import { Button } from '~/components/ui/button';
import { Dialog } from '~/components/ui/dialog';
import { Field, Input } from '~/components/ui/input';
import { Table, tdClass, thClass, trClass } from '~/components/ui/misc';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';
import { prepareFile } from '~/lib/attachments';
import { readReceipt } from '~/lib/receipts';
import { readStatement, type Statement, type StatementError } from '~/lib/statements';
import { useBook } from '~/stores/book';

const base64 = (buf: ArrayBuffer) => {
  let s = '';
  const b = new Uint8Array(buf);
  for (let i = 0; i < b.length; i += 0x8000) s += String.fromCharCode(...b.subarray(i, i + 0x8000));
  return btoa(s);
};

type Problem = StatementError | 'unreadable' | 'other';
const problems: Problem[] = ['password', 'unrecognised', 'unbalanced', 'unreadable'];

/**
 * A statement PDF (#42) or a KuDE receipt (#44, a photo or a PDF) into the
 * review queue: read in this tab, with a PDF's password, which is never
 * sent; the rows and the original file are.
 */
export function PdfImportDialog(props: { open: boolean; onOpenChange: (o: boolean) => void; onImported: () => void }) {
  const { t, te } = useI18n();
  const book = useBook();
  const [file, setFile] = createSignal<{ name: string; type: string; buf: ArrayBuffer; upload: { name: string; blob: Blob } } | null>(null);
  const [password, setPassword] = createSignal('');
  const [statement, setStatement] = createSignal<Statement | null>(null);
  const [problem, setProblem] = createSignal<Problem | null>(null);
  const [busy, setBusy] = createSignal(false);

  createEffect(() => {
    if (!props.open) return;
    setFile(null);
    setPassword('');
    setStatement(null);
    setProblem(null);
  });

  const read = async () => {
    const f = file();
    if (!f) return;
    setBusy(true);
    setProblem(null);
    setStatement(null);
    try {
      if (f.type !== 'application/pdf') {
        setStatement(await readReceipt(f.buf, f.type, '', 'statement'));
        return;
      }
      try {
        setStatement(await readStatement(f.buf, password(), 'statement'));
      } catch (err) {
        // Not a statement: perhaps a KuDE someone saved as a PDF.
        if (!(err instanceof Error && err.message === 'unrecognised')) throw err;
        setStatement(await readReceipt(f.buf, f.type, password(), 'statement'));
      }
    } catch (err) {
      const m = (err instanceof Error ? err.message : '') as Problem;
      setProblem(problems.includes(m) ? m : 'other');
    } finally {
      setBusy(false);
    }
  };

  const pick = async (input: HTMLInputElement) => {
    const f = input.files?.[0];
    if (!f) return;
    const pdf = f.type === 'application/pdf' || /\.pdf$/i.test(f.name);
    // The QR is read from the original; a photo is sent downscaled, as attachments are.
    const upload = pdf ? { name: f.name, blob: f } : await prepareFile(f);
    setFile({ name: f.name, type: pdf ? 'application/pdf' : f.type, buf: await f.arrayBuffer(), upload });
    await read();
  };

  const submit = async () => {
    const st = statement();
    const f = file();
    if (!st || !f) return;
    setBusy(true);
    try {
      const res = await api.importBatch(book.id(), {
        connector: 'statement',
        label: st.name,
        accounts: [{ id: st.account.id, label: st.account.label, currency: st.account.currency }],
        rows: st.rows.map((r) => ({ ...r, kind: r.kind as ImportRowInput['kind'], amount: r.amount ?? '0' })),
        files: [{ ref: 'statement', filename: f.upload.name, data: base64(await f.upload.blob.arrayBuffer()) }],
      });
      toast.success(t('imports.csv_done', { staged: res.staged, duplicates: res.duplicates }));
      props.onImported();
      props.onOpenChange(false);
    } catch (err) {
      toast.error(te(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange} title={t('imports.pdf_title')} description={t('imports.pdf_hint')} class="max-w-2xl">
      <div class="grid gap-4">
        <div class="grid gap-4 sm:grid-cols-2">
          <Field label={t('imports.pdf_file')}>
            <Input type="file" accept=".pdf,application/pdf,image/*" onChange={(e) => pick(e.currentTarget)} />
          </Field>
          <Field label={t('imports.pdf_password')} hint={t('imports.pdf_password_hint')}>
            <Input
              type="password"
              autocomplete="off"
              value={password()}
              onInput={(e) => setPassword(e.currentTarget.value)}
              onKeyDown={(e) => e.key === 'Enter' && read()}
            />
          </Field>
        </div>
        <Show when={problem()}>
          {(p) => (
            <div class="flex flex-wrap items-center gap-3 text-sm text-destructive">
              {t(`imports.pdf_error_${p()}`)}
              <Show when={p() === 'password'}>
                <Button size="sm" variant="outline" disabled={busy() || !password()} onClick={read}>{t('imports.pdf_open')}</Button>
              </Show>
            </div>
          )}
        </Show>
        <Show when={statement()}>
          {(st) => (
            <div class="grid gap-2">
              <p class="text-sm">
                <span class="font-medium">{st().name}</span>
                <span class="text-muted-foreground"> · {st().account.label}</span>
              </p>
              <div class="overflow-x-auto rounded-md border">
                <Table>
                  <thead>
                    <tr class="border-b">
                      <th class={thClass}>{t('imports.csv_date')}</th>
                      <th class={thClass}>{t('imports.csv_description')}</th>
                      <th class={`${thClass} text-right`}>{t('imports.csv_amount')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <For each={st().rows}>
                      {(r) => (
                        <tr class={trClass}>
                          <td class={`${tdClass} whitespace-nowrap tabular-nums`}>{r.date}</td>
                          <td class={`${tdClass} max-w-xs truncate`}>{r.kind === 'balance' ? t('imports.pdf_balance') : r.description}</td>
                          <td class={`${tdClass} text-right`}><Money amount={r.amount ?? '0'} currency={r.currency ?? st().account.currency} signed={r.kind !== 'balance'} /></td>
                        </tr>
                      )}
                    </For>
                  </tbody>
                </Table>
              </div>
            </div>
          )}
        </Show>
        <div class="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>{t('common.cancel')}</Button>
          <Button disabled={busy() || !statement()} onClick={submit}>
            <FileUp /> {t('imports.csv_submit', { rows: statement()?.rows.length ?? 0 })}
          </Button>
        </div>
      </div>
    </Dialog>
  );
}
