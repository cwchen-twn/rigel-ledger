// Styled after shadcn/ui's Card (MIT).
import { splitProps, type JSX } from 'solid-js';
import { cn } from '~/lib/cn';

type DivProps = JSX.HTMLAttributes<HTMLDivElement>;

const wrap = (base: string) => (props: DivProps) => {
  const [local, rest] = splitProps(props, ['class']);
  return <div class={cn(base, local.class)} {...rest} />;
};

export const Card = wrap('flex min-w-0 flex-col gap-4 rounded-xl border bg-card py-5 text-card-foreground shadow-xs');
export const CardHeader = wrap('grid gap-1 px-5');
export const CardTitle = wrap('font-semibold leading-none');
export const CardDescription = wrap('text-sm text-muted-foreground');
export const CardContent = wrap('px-5');
export const CardFooter = wrap('flex items-center gap-2 px-5');
