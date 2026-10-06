import { FileText, Paperclip, X } from 'lucide-solid';
import { For, Show, createMemo, onCleanup } from 'solid-js';
import { api } from '~/api/client';
import type { Attachment } from '~/api/types';
import { Button } from '~/components/ui/button';
import { useI18n } from '~/i18n';
import { ACCEPT, formatSize } from '~/lib/attachments';

/**
 * A transaction's receipts and files (#36): thumbnails that open the file in
 * a new tab, a remove button for editors, and files chosen for a transaction
 * not saved yet, which upload when it is. On a phone the file picker offers
 * the camera.
 */
export function Attachments(props: {
  bookId: number;
  attachments: Attachment[];
  /** Chosen before the transaction exists; uploaded on save. */
  queued: File[];
  canEdit: boolean;
  busy: boolean;
  onAdd: (files: File[]) => void;
  onRemove: (a: Attachment) => void;
  onUnqueue: (index: number) => void;
}) {
  const { t, intl } = useI18n();
  let input!: HTMLInputElement;
  const empty = () => !props.attachments.length && !props.queued.length;

  // Previews of the files not uploaded yet, released when they go.
  const previews = createMemo<string[]>((prev) => {
    for (const url of prev) URL.revokeObjectURL(url);
    return props.queued.map((f) => (f.type.startsWith('image/') ? URL.createObjectURL(f) : ''));
  }, []);
  onCleanup(() => previews().forEach((url) => url && URL.revokeObjectURL(url)));

  const tile = 'relative grid size-20 shrink-0 place-items-center overflow-hidden rounded-md border bg-muted text-muted-foreground';
  const removeButton = (label: string, onClick: () => void) => (
    <button
      type="button"
      class="absolute top-1 right-1 grid size-5 place-items-center rounded-full bg-background/90 text-foreground shadow-xs hover:bg-background"
      aria-label={label}
      onClick={onClick}
    >
      <X class="size-3" />
    </button>
  );

  return (
    <div class="grid gap-2">
      <div class="flex items-center justify-between gap-2">
        <span class="text-sm font-medium">{t('attachments.title')}</span>
        <Show when={props.canEdit}>
          <Button size="sm" variant="outline" disabled={props.busy} onClick={() => input.click()}>
            <Paperclip /> {props.busy ? t('attachments.uploading') : t('attachments.add')}
          </Button>
          <input
            ref={input}
            type="file"
            class="hidden"
            accept={ACCEPT}
            multiple
            onChange={(e) => {
              const files = [...(e.currentTarget.files ?? [])];
              e.currentTarget.value = ''; // the same file may be chosen again
              if (files.length) props.onAdd(files);
            }}
          />
        </Show>
      </div>
      <Show when={!empty()} fallback={<p class="text-sm text-muted-foreground">{t('attachments.none')}</p>}>
        <div class="flex flex-wrap gap-2">
          <For each={props.attachments}>
            {(a) => (
              <div class="grid w-20 gap-1">
                <div class={tile}>
                  <a
                    href={api.fileUrl(props.bookId, a.id)}
                    target="_blank"
                    rel="noopener"
                    class="grid size-full place-items-center"
                    title={`${a.filename} · ${formatSize(a.size, intl())}`}
                  >
                    <Show when={a.mime !== 'application/pdf'} fallback={<FileText class="size-7" />}>
                      <img src={api.fileUrl(props.bookId, a.id)} alt={a.filename} loading="lazy" class="size-full object-cover" />
                    </Show>
                  </a>
                  <Show when={props.canEdit}>{removeButton(t('attachments.remove', { name: a.filename }), () => props.onRemove(a))}</Show>
                </div>
                <span class="truncate text-xs text-muted-foreground" title={a.filename}>{a.filename}</span>
              </div>
            )}
          </For>
          <For each={props.queued}>
            {(f, i) => (
              <div class="grid w-20 gap-1">
                <div class={`${tile} border-dashed`}>
                  <Show when={previews()[i()]} fallback={<FileText class="size-7" />}>
                    <img src={previews()[i()]} alt={f.name} class="size-full object-cover opacity-80" />
                  </Show>
                  {removeButton(t('attachments.remove', { name: f.name }), () => props.onUnqueue(i()))}
                </div>
                <span class="truncate text-xs text-muted-foreground" title={f.name}>{f.name}</span>
              </div>
            )}
          </For>
        </div>
        <Show when={props.queued.length}>
          <p class="text-xs text-muted-foreground">{t('attachments.on_save')}</p>
        </Show>
      </Show>
    </div>
  );
}
