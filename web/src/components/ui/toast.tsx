// Toasts on Kobalte's Toast, styled after shadcn/ui's Sonner look (MIT).
import { Toast, toaster } from '@kobalte/core/toast';
import { CircleAlert, CircleCheck, X } from 'lucide-solid';
import { Portal } from 'solid-js/web';

export function Toaster() {
  return (
    <Portal>
      <Toast.Region duration={4000} limit={4}>
        <Toast.List class="fixed right-0 bottom-0 z-[100] flex w-full max-w-sm flex-col gap-2 p-4 outline-none" />
      </Toast.Region>
    </Portal>
  );
}

function show(kind: 'success' | 'error', title: string, description?: string) {
  toaster.show((props) => (
    <Toast
      toastId={props.toastId}
      class="flex items-start gap-3 rounded-lg border bg-popover p-4 text-popover-foreground shadow-lg data-[swipe=move]:translate-x-[var(--kb-toast-swipe-move-x)]"
    >
      <span class={kind === 'error' ? 'text-destructive [&_svg]:size-5' : 'text-positive [&_svg]:size-5'}>
        {kind === 'error' ? <CircleAlert /> : <CircleCheck />}
      </span>
      <div class="grid flex-1 gap-1">
        <Toast.Title class="text-sm font-medium">{title}</Toast.Title>
        {description && <Toast.Description class="text-sm text-muted-foreground">{description}</Toast.Description>}
      </div>
      <Toast.CloseButton class="opacity-60 hover:opacity-100 [&_svg]:size-4" aria-label="Close">
        <X />
      </Toast.CloseButton>
    </Toast>
  ));
}

export const toast = {
  success: (title: string, description?: string) => show('success', title, description),
  error: (title: string, description?: string) => show('error', title, description),
};
