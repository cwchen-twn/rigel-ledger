// Small presentational pieces styled after shadcn/ui (MIT).
import { cva, type VariantProps } from 'class-variance-authority';
import { splitProps, type JSX } from 'solid-js';
import { cn } from '~/lib/cn';

const badgeVariants = cva(
  'inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-xs font-medium whitespace-nowrap',
  {
    variants: {
      variant: {
        default: 'border-transparent bg-primary text-primary-foreground',
        secondary: 'border-transparent bg-secondary text-secondary-foreground',
        outline: 'text-foreground',
        destructive: 'border-transparent bg-destructive text-white',
        warning: 'border-transparent bg-amber-500/15 text-amber-700 dark:text-amber-400',
      },
    },
    defaultVariants: { variant: 'secondary' },
  },
);

export function Badge(props: JSX.HTMLAttributes<HTMLSpanElement> & VariantProps<typeof badgeVariants>) {
  const [local, rest] = splitProps(props, ['class', 'variant']);
  return <span class={cn(badgeVariants({ variant: local.variant }), local.class)} {...rest} />;
}

export function Separator(props: { class?: string }) {
  return <div role="separator" class={cn('h-px w-full shrink-0 bg-border', props.class)} />;
}

export function Skeleton(props: { class?: string }) {
  return <div class={cn('animate-pulse rounded-md bg-accent', props.class)} />;
}

export function Table(props: JSX.HTMLAttributes<HTMLTableElement>) {
  const [local, rest] = splitProps(props, ['class']);
  return (
    <div class="relative w-full overflow-x-auto">
      <table class={cn('w-full caption-bottom text-sm', local.class)} {...rest} />
    </div>
  );
}
export const thClass = 'h-10 px-2 text-left align-middle font-medium text-muted-foreground whitespace-nowrap';
export const tdClass = 'p-2 align-middle';
export const trClass = 'border-b transition-colors hover:bg-muted/50';

export function EmptyState(props: { icon?: JSX.Element; title: string; children?: JSX.Element }) {
  return (
    <div class="flex flex-col items-center justify-center gap-2 rounded-xl border border-dashed p-10 text-center">
      {props.icon && <div class="text-muted-foreground [&_svg]:size-8">{props.icon}</div>}
      <p class="font-medium">{props.title}</p>
      {props.children && <div class="text-sm text-muted-foreground">{props.children}</div>}
    </div>
  );
}

/** A failed load, with a way to try again: never a blank page. */
export function ErrorState(props: { title: string; message?: string; retryLabel: string; onRetry: () => void }) {
  return (
    <div role="alert" class="flex flex-col items-center justify-center gap-3 rounded-xl border border-destructive/40 p-10 text-center">
      <p class="font-medium">{props.title}</p>
      {props.message && <p class="text-sm text-muted-foreground">{props.message}</p>}
      <button
        type="button"
        onClick={() => props.onRetry()}
        class="rounded-md border px-3 py-1.5 text-sm shadow-xs hover:bg-accent hover:text-accent-foreground"
      >
        {props.retryLabel}
      </button>
    </div>
  );
}
