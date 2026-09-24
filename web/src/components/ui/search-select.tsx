// A pick-one list with type-to-search, on Kobalte's Combobox, styled after
// shadcn/ui's Combobox (MIT). For long lists (currencies, time zones,
// commodities); short ones stay native <Select>, which phones render best.
import { Combobox } from '@kobalte/core/combobox';
import { Check, ChevronsUpDown } from 'lucide-solid';
import { createMemo, Show } from 'solid-js';
import { usePortalMount } from '~/components/ui/dialog';
import { useFieldProps } from '~/components/ui/input';
import { useI18n } from '~/i18n';
import { cn } from '~/lib/cn';

export interface SearchOption {
  value: string;
  /** Shown in the field once chosen, e.g. "USD". */
  label: string;
  /** Shown beside it in the list and searched too, e.g. "US Dollar". */
  description?: string;
}

interface Item extends SearchOption {
  search: string;
}

export function SearchSelect(props: {
  options: SearchOption[];
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
  invalid?: boolean;
  class?: string;
  /** Needed when the control is not inside a <Field>. */
  'aria-label'?: string;
}) {
  const { t } = useI18n();
  const mount = usePortalMount();
  const field = useFieldProps();

  // A stored value the list lacks (options still loading, or "UTC", which
  // the browser's time-zone list leaves out) is kept as an option of its own:
  // Kobalte shows only values that are in its options.
  const items = createMemo<Item[]>(() => {
    const list = props.options.map((o) => ({ ...o, search: o.description ? `${o.label} ${o.description}` : o.label }));
    if (props.value && !list.some((o) => o.value === props.value)) {
      list.unshift({ value: props.value, label: props.value, search: props.value });
    }
    return list;
  });
  const selected = createMemo<Item | null>(() => items().find((o) => o.value === props.value) ?? null);

  return (
    <Combobox<Item>
      options={items()}
      value={selected()}
      // Every use is a required choice: clearing the text keeps the old value.
      onChange={(o) => o && props.onChange(o.value)}
      optionValue="value"
      optionTextValue="search"
      optionLabel="label"
      defaultFilter="contains"
      placeholder={props.placeholder ?? t('common.search')}
      disabled={props.disabled}
      triggerMode="focus"
      // The root is the grid/flex child: it must be allowed to shrink, or the
      // input's intrinsic width (size=20) pushes it past its column.
      class={cn('w-full min-w-0', props.class)}
      itemComponent={(p) => (
        <Combobox.Item
          item={p.item}
          class="relative flex cursor-default items-center justify-between gap-3 rounded-sm px-2 py-1.5 text-sm outline-none select-none data-[highlighted]:bg-accent data-[highlighted]:text-accent-foreground"
        >
          <Combobox.ItemLabel class="flex min-w-0 items-baseline gap-2">
            <span class="shrink-0">{p.item.rawValue.label}</span>
            <Show when={p.item.rawValue.description}>
              <span class="truncate text-xs text-muted-foreground">{p.item.rawValue.description}</span>
            </Show>
          </Combobox.ItemLabel>
          <Combobox.ItemIndicator>
            <Check class="size-4 shrink-0" />
          </Combobox.ItemIndicator>
        </Combobox.Item>
      )}
    >
      <Combobox.Control
        class={cn(
          'flex h-9 w-full min-w-0 items-center rounded-md border border-input bg-transparent shadow-xs dark:bg-input/30',
          'focus-within:border-ring focus-within:ring-[3px] focus-within:ring-ring/50',
          'data-[disabled]:cursor-not-allowed data-[disabled]:opacity-50',
          (props.invalid || field['aria-invalid']) && 'border-destructive',
        )}
      >
        <Combobox.Input
          id={field.id}
          aria-describedby={field['aria-describedby']}
          aria-label={props['aria-label']}
          // Select the text on focus so typing replaces it and starts a search.
          onFocus={(e) => e.currentTarget.select()}
          size={1}
          class="h-full w-0 min-w-0 flex-1 bg-transparent px-3 text-sm outline-none placeholder:text-muted-foreground"
        />
        <Combobox.Trigger class="px-2 text-muted-foreground" aria-label={t('common.show_options')}>
          <ChevronsUpDown class="size-4" />
        </Combobox.Trigger>
      </Combobox.Control>
      <Combobox.Portal mount={mount?.()}>
        {/* Keep focus in the input on mouse press, or inside a dialog the
            press blurs it and closes the list before the click lands (the
            AccountCombobox trap). */}
        <Combobox.Content
          onMouseDown={(e) => e.preventDefault()}
          class="z-50 max-h-72 w-[max(var(--kb-popper-anchor-width),min(14rem,calc(100vw-2rem)))] max-w-[calc(100vw-2rem)] overflow-y-auto rounded-md border bg-popover p-1 text-popover-foreground shadow-md"
        >
          <Combobox.Listbox />
        </Combobox.Content>
      </Combobox.Portal>
    </Combobox>
  );
}
