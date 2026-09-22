// Form controls styled after shadcn/ui (MIT). Selects and checkboxes are native
// elements: accessible, keyboard- and mobile-friendly with no extra JS.
import { children, createEffect, splitProps, type JSX } from 'solid-js';
import { cn } from '~/lib/cn';

const control =
  'w-full min-w-0 rounded-md border border-input bg-transparent px-3 text-sm shadow-xs transition-[color,box-shadow] ' +
  'placeholder:text-muted-foreground focus-visible:outline-none focus-visible:border-ring focus-visible:ring-[3px] ' +
  'focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive ' +
  'aria-invalid:ring-destructive/20 dark:bg-input/30';

export function Input(props: JSX.InputHTMLAttributes<HTMLInputElement>) {
  const [local, rest] = splitProps(props, ['class']);
  return <input class={cn(control, 'h-9 py-1', local.class)} {...rest} />;
}

export function Textarea(props: JSX.TextareaHTMLAttributes<HTMLTextAreaElement>) {
  const [local, rest] = splitProps(props, ['class']);
  return <textarea class={cn(control, 'min-h-16 py-2', local.class)} {...rest} />;
}

export function Select(props: JSX.SelectHTMLAttributes<HTMLSelectElement>) {
  const [local, rest] = splitProps(props, ['class', 'children']);
  let el: HTMLSelectElement | undefined;
  const options = children(() => local.children);
  // Solid sets `value` once; when the options arrive later (currencies,
  // accounts, books all load async) the browser falls back to the first
  // option -- a USD line showed "AED". Re-apply the value whenever the
  // options change.
  createEffect(() => {
    options();
    if (el && props.value !== undefined) el.value = String(props.value);
  });
  return (
    <select ref={el} class={cn(control, 'h-9 py-1 pr-8', local.class)} {...rest}>
      {options()}
    </select>
  );
}

export function Checkbox(props: Omit<JSX.InputHTMLAttributes<HTMLInputElement>, 'type'> & { label?: JSX.Element }) {
  const [local, rest] = splitProps(props, ['class', 'label']);
  return (
    <label class={cn('inline-flex cursor-pointer items-center gap-2 text-sm', local.class)}>
      <input type="checkbox" class="size-4 rounded border-input accent-primary" {...rest} />
      {local.label}
    </label>
  );
}

export function Label(props: JSX.LabelHTMLAttributes<HTMLLabelElement>) {
  const [local, rest] = splitProps(props, ['class']);
  return <label class={cn('text-sm font-medium leading-none select-none', local.class)} {...rest} />;
}

/** A label, a control and an optional error or hint under it. */
export function Field(props: { label: JSX.Element; error?: string; hint?: JSX.Element; class?: string; children: JSX.Element }) {
  return (
    <div class={cn('grid content-start gap-1.5', props.class)}>
      <Label>{props.label}</Label>
      {props.children}
      {props.error ? (
        <p class="text-xs text-destructive">{props.error}</p>
      ) : props.hint ? (
        <p class="text-xs text-muted-foreground">{props.hint}</p>
      ) : null}
    </div>
  );
}
