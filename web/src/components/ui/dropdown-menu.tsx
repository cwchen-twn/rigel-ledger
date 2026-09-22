// A menu on Kobalte's DropdownMenu, styled after shadcn/ui (MIT).
import { DropdownMenu as K } from '@kobalte/core/dropdown-menu';
import { For, Show, type JSX } from 'solid-js';
import { cn } from '~/lib/cn';

export interface MenuItem {
  label: JSX.Element;
  icon?: JSX.Element;
  onSelect: () => void;
  destructive?: boolean;
  disabled?: boolean;
  separatorBefore?: boolean;
}

export function DropdownMenu(props: { trigger: JSX.Element; items: MenuItem[]; triggerClass?: string; label?: string }) {
  return (
    <K placement="bottom-end">
      <K.Trigger class={props.triggerClass} aria-label={props.label}>
        {props.trigger}
      </K.Trigger>
      <K.Portal>
        <K.Content class="z-50 min-w-40 overflow-hidden rounded-md border bg-popover p-1 text-popover-foreground shadow-md">
          <For each={props.items}>
            {(item) => (
              <>
                <Show when={item.separatorBefore}>
                  <K.Separator class="-mx-1 my-1 h-px bg-border" />
                </Show>
                <K.Item
                  disabled={item.disabled}
                  onSelect={item.onSelect}
                  class={cn(
                    'relative flex cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none select-none',
                    'data-[highlighted]:bg-accent data-[highlighted]:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50',
                    "[&_svg]:size-4 [&_svg]:text-muted-foreground",
                    item.destructive && 'text-destructive [&_svg]:!text-destructive',
                  )}
                >
                  {item.icon}
                  {item.label}
                </K.Item>
              </>
            )}
          </For>
        </K.Content>
      </K.Portal>
    </K>
  );
}
