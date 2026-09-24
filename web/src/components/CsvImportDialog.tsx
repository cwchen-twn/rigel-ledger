import { FileUp } from 'lucide-solid';
import { createEffect, createMemo, createSignal, For, Show } from 'solid-js';
import { api } from '~/api/client';
import { AccountCombobox } from '~/components/AccountCombobox';
import { Money } from '~/components/Money';
import { Button } from '~/components/ui/button';
import { Dialog } from '~/components/ui/dialog';
import { Checkbox, Field, Input, Select } from '~/components/ui/input';
import { Table, tdClass, thClass, trClass } from '~/components/ui/misc';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';
import {
  applyMapping,
  DATE_FORMATS,
  decode,
  externalIds,
  guessMapping,
  parseCSV,
  sniffDelimiter,
  type DateFormat,
  type Encoding,
  type Mapping,
} from '~/lib/csv';
import { useBook } from '~/stores/book';

const PREVIEW = 8;
const BLANK: Mapping = { date: 0, description: 1, counterparty: -1, amount: 2, out: -1, in: -1, flip: false, dateFormat: 'YYYY-MM-DD', decimalComma: false };

/**
 * A bank statement CSV into the review queue: pick the account it belongs to,
 * map the columns, check the preview. Parsing happens here in the tab; only
 * the parsed rows are sent, as connector "csv".
 */
export function CsvImportDialog(props: { open: boolean; onOpenChange: (o: boolean) => void; onImported: () => void }) {
  const { t, te } = useI18n();
  const book = useBook();
  const [account, setAccount] = createSignal<number | null>(null);
  const [file, setFile] = createSignal<{ name: string; buf: ArrayBuffer } | null>(null);
  const [encoding, setEncoding] = createSignal<Encoding>('utf-8');
  const [skip, setSkip] = createSignal(0);
  const [header, setHeader] = createSignal(true);
  const [m, setM] = createSignal<Mapping>(BLANK);
  const [busy, setBusy] = createSignal(false);

  createEffect(() => {
    if (!props.open) return;
    setFile(null);
    setSkip(0);
    setHeader(true);
    setM(BLANK);
  });

  const table = createMemo(() => {
    const f = file();
    if (!f) return [];
    const text = decode(f.buf, encoding());
    return parseCSV(text, sniffDelimiter(text)).slice(skip());
  });
  const columns = createMemo(() => {
    const first = table()[0] ?? [];
    return first.map((h, i) => (header() && h.trim() ? h.trim() : t('imports.csv_column', { n: i + 1 })));
  });
  const body = () => (header() ? table().slice(1) : table());
  const parsed = createMemo(() => applyMapping(body(), skip() + (header() ? 2 : 1), m()));

  // A new file or header row: guess the columns again.
  createEffect(() => {
    const cols = table()[0];
    if (!cols || !header()) return;
    setM({ ...BLANK, ...guessMapping(cols) });
  });

  const pick = async (input: HTMLInputElement) => {
    const f = input.files?.[0];
    if (!f) return;
    setFile({ name: f.name, buf: await f.arrayBuffer() });
  };
  const set = <K extends keyof Mapping>(k: K, v: Mapping[K]) => setM({ ...m(), [k]: v });
  const acct = () => (account() ? book.byId().get(account()!) : undefined);
  const currency = () => acct()?.commodity ?? book.book()?.base_currency ?? 'USD';

  const columnSelect = (k: 'date' | 'description' | 'counterparty' | 'amount' | 'out' | 'in', optional = false) => (
    <Select value={String(m()[k])} onChange={(e) => set(k, Number(e.currentTarget.value))}>
      <Show when={optional}><option value="-1">{t('imports.csv_none')}</option></Show>
      <For each={columns()}>{(c, i) => <option value={String(i())}>{c}</option>}</For>
    </Select>
  );

  const submit = async () => {
    const a = acct();
    if (!a || !parsed().rows.length) return;
    setBusy(true);
    try {
      const key = `account-${a.id}`;
      const ids = await externalIds(parsed().rows, key);
      const res = await api.importBatch(book.id(), {
        connector: 'csv',
        label: file()?.name ?? '',
        accounts: [{ id: key, label: book.name(a), currency: currency() }],
        rows: parsed().rows.map((r, i) => ({
          kind: 'transaction',
          account: key,
          id: ids[i],
          date: r.date,
          amount: r.amount,
          description: r.description,
          counterparty: r.counterparty,
          raw: { line: r.line, cells: r.cells },
        })),
      });
      // A CSV names its account: map it on the first import.
      const src = (await api.importSources(book.id())).find((s) => s.connector === 'csv' && s.external_id === key);
      if (src && src.account_id === null) await api.mapSource(book.id(), src.id, a.id);
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
    <Dialog open={props.open} onOpenChange={props.onOpenChange} title={t('imports.csv_title')} description={t('imports.csv_hint')} class="max-w-3xl">
      <div class="grid gap-4">
        <div class="grid gap-4 sm:grid-cols-2">
          <Field label={t('imports.csv_account')}>
            <AccountCombobox value={account()} onChange={setAccount} filter={(x) => x.class === 'asset' || x.class === 'liability'} />
          </Field>
          <Field label={t('imports.csv_file')}>
            <Input type="file" accept=".csv,.txt,text/csv" onChange={(e) => pick(e.currentTarget)} />
          </Field>
        </div>

        <Show when={file()}>
          <div class="grid gap-4 sm:grid-cols-3">
            <Field label={t('imports.csv_encoding')} hint={t('imports.csv_encoding_hint')}>
              <Select value={encoding()} onChange={(e) => setEncoding(e.currentTarget.value as Encoding)}>
                <option value="utf-8">UTF-8</option>
                <option value="big5">Big5 (繁體)</option>
                <option value="windows-1252">Windows-1252</option>
              </Select>
            </Field>
            <Field label={t('imports.csv_skip')}>
              <Input type="number" min="0" value={skip()} onInput={(e) => setSkip(Math.max(0, Number(e.currentTarget.value) || 0))} />
            </Field>
            <div class="flex items-end pb-2">
              <Checkbox checked={header()} onChange={(e) => setHeader(e.currentTarget.checked)} label={t('imports.csv_header')} />
            </div>
          </div>

          <div class="grid gap-4 sm:grid-cols-3">
            <Field label={t('imports.csv_date')}>{columnSelect('date')}</Field>
            <Field label={t('imports.csv_date_format')}>
              <Select value={m().dateFormat} onChange={(e) => set('dateFormat', e.currentTarget.value as DateFormat)}>
                <For each={DATE_FORMATS}>{(f) => <option value={f}>{f === 'ROC' ? t('imports.csv_roc') : f}</option>}</For>
              </Select>
            </Field>
            <Field label={t('imports.csv_description')}>{columnSelect('description')}</Field>
            <Field label={t('imports.csv_counterparty')}>{columnSelect('counterparty', true)}</Field>
            <Field label={t('imports.csv_amount_mode')}>
              <Select
                value={m().amount >= 0 ? 'one' : 'two'}
                onChange={(e) => (e.currentTarget.value === 'one' ? setM({ ...m(), amount: 0, out: -1, in: -1 }) : setM({ ...m(), amount: -1, out: 0, in: 0 }))}
              >
                <option value="one">{t('imports.csv_one_column')}</option>
                <option value="two">{t('imports.csv_two_columns')}</option>
              </Select>
            </Field>
            <Show
              when={m().amount >= 0}
              fallback={
                <>
                  <Field label={t('imports.csv_out')}>{columnSelect('out')}</Field>
                  <Field label={t('imports.csv_in')}>{columnSelect('in')}</Field>
                </>
              }
            >
              <Field label={t('imports.csv_amount')}>{columnSelect('amount')}</Field>
            </Show>
          </div>
          <div class="flex flex-wrap gap-x-6 gap-y-2">
            <Checkbox checked={m().decimalComma} onChange={(e) => set('decimalComma', e.currentTarget.checked)} label={t('imports.csv_decimal_comma')} />
            <Checkbox checked={m().flip} onChange={(e) => set('flip', e.currentTarget.checked)} label={t('imports.csv_flip')} />
          </div>

          <div class="grid gap-2">
            <p class="text-sm text-muted-foreground">
              {t('imports.csv_preview', { rows: parsed().rows.length })}
              <Show when={parsed().errors.length}>
                {' '}
                <span class="text-destructive">
                  {t('imports.csv_errors', { count: parsed().errors.length, lines: parsed().errors.slice(0, 5).map((e) => e.line).join(', ') })}
                </span>
              </Show>
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
                  <For each={parsed().rows.slice(0, PREVIEW)}>
                    {(r) => (
                      <tr class={trClass}>
                        <td class={`${tdClass} whitespace-nowrap tabular-nums`}>{r.date}</td>
                        <td class={`${tdClass} max-w-xs truncate`}>{[r.description, r.counterparty].filter(Boolean).join(' · ')}</td>
                        <td class={`${tdClass} text-right`}><Money amount={r.amount} currency={currency()} signed /></td>
                      </tr>
                    )}
                  </For>
                </tbody>
              </Table>
            </div>
            <p class="text-xs text-muted-foreground">{t('imports.csv_sign_hint')}</p>
          </div>
        </Show>

        <div class="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>{t('common.cancel')}</Button>
          <Button disabled={busy() || !account() || !parsed().rows.length} onClick={submit}>
            <FileUp /> {t('imports.csv_submit', { rows: parsed().rows.length })}
          </Button>
        </div>
      </div>
    </Dialog>
  );
}
