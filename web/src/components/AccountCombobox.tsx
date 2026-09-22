// Searchable account picker on Kobalte's Combobox, styled after shadcn/ui's Combobox (MIT).
import { Combobox } from '@kobalte/core/combobox';
import { Check, ChevronsUpDown } from 'lucide-solid';
import { createMemo } from 'solid-js';
import type { Account } from '~/api/types';
import { usePortalMount } from '~/components/ui/dialog';
import { useI18n } from '~/i18n';
import { cn } from '~/lib/cn';
import { CLASS_ORDER, useBook } from '~/stores/book';

interface Option {
  id: number;
  label: string;
  account: Account;
}

export function AccountCombobox(props: {
  value: number | null;
  onChange: (id: number | null) => void;
  /** Extra filter; placeholders and archived accounts are always excluded (except the current value). */
  filter?: (a: Account) => boolean;
  placeholder?: string;
  invalid?: boolean;
  /** Filters may pick a group account (it matches its whole subtree). */
  allowPlaceholders?: boolean;
  class?: string;
}) {
  const { t } = useI18n();
  const book = useBook();
  const mount = usePortalMount();

  const options = createMemo<Option[]>(() =>
    (book.accounts() ?? [])
      .filter((a) => a.id === props.value || ((props.allowPlaceholders || !a.is_placeholder) && !a.archived && (props.filter?.(a) ?? true)))
      .map((a) => ({ id: a.id, label: book.path(a.id), account: a }))
      .sort((x, y) => {
        const c = CLASS_ORDER.indexOf(x.account.class) - CLASS_ORDER.indexOf(y.account.class);
        return c !== 0 ? c : x.label.localeCompare(y.label);
      }),
  );
  const selected = () => options().find((o) => o.id === props.value) ?? null;

  return (
    <Combobox<Option>
      options={options()}
      value={selected()}
      onChange={(o) => props.onChange(o?.id ?? null)}
      optionValue="id"
      optionTextValue="label"
      optionLabel="label"
      defaultFilter="contains"
      placeholder={props.placeholder ?? t('transactions.account')}
      triggerMode="focus"
      itemComponent={(p) => (
        <Combobox.Item
          item={p.item}
          class="relative flex cursor-default items-center justify-between gap-2 rounded-sm px-2 py-1.5 text-sm outline-none select-none data-[highlighted]:bg-accent data-[highlighted]:text-accent-foreground"
        >
          <Combobox.ItemLabel class="truncate">{p.item.rawValue.label}</Combobox.ItemLabel>
          <span class="flex shrink-0 items-center gap-2 text-xs text-muted-foreground">
            {p.item.rawValue.account.commodity}
            <Combobox.ItemIndicator>
              <Check class="size-4" />
            </Combobox.ItemIndicator>
          </span>
        </Combobox.Item>
      )}
    >
      <Combobox.Control
        aria-label={props.placeholder ?? t('transactions.account')}
        class={cn(
          'flex h-9 w-full items-center rounded-md border border-input bg-transparent shadow-xs dark:bg-input/30',
          'focus-within:border-ring focus-within:ring-[3px] focus-within:ring-ring/50',
          props.invalid && 'border-destructive',
          props.class,
        )}
      >
        <Combobox.Input class="h-full min-w-0 flex-1 bg-transparent px-3 text-sm outline-none placeholder:text-muted-foreground" />
        <Combobox.Trigger class="px-2 text-muted-foreground">
          <ChevronsUpDown class="size-4" />
        </Combobox.Trigger>
      </Combobox.Control>
      <Combobox.Portal mount={mount?.()}>
        {/* Keep focus in the input on mouse press. Inside a dialog the nearest
            focusable ancestor is the dialog itself, so the press would move
            focus there, blur the input and close the list before the click
            selected anything (keyboard selection was unaffected). */}
        <Combobox.Content onMouseDown={(e) => e.preventDefault()} class="z-50 max-h-72 min-w-[var(--kb-popper-anchor-width)] overflow-y-auto rounded-md border bg-popover p-1 text-popover-foreground shadow-md">
          <Combobox.Listbox />
        </Combobox.Content>
      </Combobox.Portal>
    </Combobox>
  );
}
