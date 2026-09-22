// Dialog and Sheet on Kobalte's accessible Dialog, styled after shadcn/ui (MIT).
import { Dialog as K } from '@kobalte/core/dialog';
import { X } from 'lucide-solid';
import { createContext, Show, useContext, type JSX } from 'solid-js';
import { cn } from '~/lib/cn';

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: JSX.Element;
  description?: JSX.Element;
  children: JSX.Element;
  footer?: JSX.Element;
  class?: string;
}

/**
 * A modal dialog marks everything outside itself aria-hidden. A popup (combobox
 * list) portaled to <body> would therefore be invisible to screen readers --
 * found when role=option matched nothing inside the transaction sheet. Popups
 * inside a dialog mount into its content element instead (usePortalMount).
 * Its portal root would not do: Kobalte hides everything outside the content.
 */
const PortalMount = createContext<() => HTMLElement | undefined>();
/** A getter for the enclosing dialog element; read it lazily (inside JSX), after refs are set. */
export const usePortalMount = () => useContext(PortalMount);


const overlay =
  'fixed inset-0 z-50 bg-black/50 data-[expanded]:animate-in data-[closed]:animate-out data-[closed]:fade-out-0 data-[expanded]:fade-in-0';

function Header(props: { title: JSX.Element; description?: JSX.Element }) {
  return (
    <div class="grid gap-1.5 pr-6">
      <K.Title class="text-lg font-semibold leading-none">{props.title}</K.Title>
      <Show when={props.description}>
        <K.Description class="text-sm text-muted-foreground">{props.description}</K.Description>
      </Show>
    </div>
  );
}

function Close() {
  return (
    <K.CloseButton class="absolute top-4 right-4 rounded-xs opacity-70 transition-opacity hover:opacity-100 focus:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 [&_svg]:size-4">
      <X />
      <span class="sr-only">Close</span>
    </K.CloseButton>
  );
}

export function Dialog(props: Props) {
  let content: HTMLDivElement | undefined;
  return (
    <K open={props.open} onOpenChange={props.onOpenChange}>
      <K.Portal>
        <K.Overlay class={overlay} />
        <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
          <K.Content ref={content} class={cn('relative grid max-h-[90vh] w-full max-w-lg gap-4 overflow-y-auto rounded-lg border bg-background p-6 shadow-lg', props.class)}>
            <Header title={props.title} description={props.description} />
            <PortalMount.Provider value={() => content}>{props.children}</PortalMount.Provider>
            <Show when={props.footer}>
              <div class="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">{props.footer}</div>
            </Show>
            <Close />
          </K.Content>
        </div>
      </K.Portal>
    </K>
  );
}

/** A panel that slides in from the right: full screen on phones. */
export function Sheet(props: Props) {
  let content: HTMLDivElement | undefined;
  return (
    <K open={props.open} onOpenChange={props.onOpenChange}>
      <K.Portal>
        <K.Overlay class={overlay} />
        <K.Content
          ref={content}
          class={cn(
            'fixed inset-y-0 right-0 z-50 flex h-full w-full flex-col gap-4 border-l bg-background p-6 shadow-lg sm:max-w-2xl',
            props.class,
          )}
        >
          <Header title={props.title} description={props.description} />
          <div class="-mx-6 flex-1 overflow-y-auto px-6">
            <PortalMount.Provider value={() => content}>{props.children}</PortalMount.Provider>
          </div>
          <Show when={props.footer}>
            <div class="flex flex-col-reverse gap-2 border-t pt-4 sm:flex-row sm:justify-end">{props.footer}</div>
          </Show>
          <Close />
        </K.Content>
      </K.Portal>
    </K>
  );
}
