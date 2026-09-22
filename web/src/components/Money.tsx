import { splitProps, type JSX } from 'solid-js';
import { useI18n } from '~/i18n';
import { cn } from '~/lib/cn';
import { cmp, formatMoney } from '~/lib/money';
import { useSession } from '~/stores/session';

/** A formatted amount. Negative values are shown in the destructive colour when `signed`. */
export function Money(props: { amount: string; currency: string; signed?: boolean; class?: string }) {
  const { intl } = useI18n();
  const { decimals } = useSession();
  return (
    <span
      class={cn(
        'tabular-nums whitespace-nowrap',
        props.signed && cmp(props.amount, '0') < 0 && 'text-destructive',
        props.class,
      )}
    >
      {formatMoney(props.amount, props.currency, intl(), decimals(props.currency))}
    </span>
  );
}

/** A text input for decimal amounts: numeric keypad on phones, right-aligned digits. */
export function MoneyInput(props: JSX.InputHTMLAttributes<HTMLInputElement> & { invalid?: boolean }) {
  const [local, rest] = splitProps(props, ['class', 'invalid']);
  return (
    <input
      inputmode="decimal"
      autocomplete="off"
      aria-invalid={local.invalid || undefined}
      class={cn(
        'h-9 w-full min-w-0 rounded-md border border-input bg-transparent px-3 text-right text-sm tabular-nums shadow-xs',
        'placeholder:text-muted-foreground focus-visible:outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50',
        'aria-invalid:border-destructive disabled:opacity-50 dark:bg-input/30',
        local.class,
      )}
      {...rest}
    />
  );
}
