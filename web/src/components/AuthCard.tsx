import { Wallet } from 'lucide-solid';
import { Show, type JSX, type ParentComponent } from 'solid-js';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '~/components/ui/card';
import { useI18n } from '~/i18n';
import { cn } from '~/lib/cn';

/** The centred card every signed-out page (and the first-login wizard) sits in. */
export const AuthCard: ParentComponent<{ title: string; description?: JSX.Element; wide?: boolean; footer?: JSX.Element }> = (props) => {
  const { t } = useI18n();
  return (
    <div class="flex min-h-screen items-center justify-center bg-muted/40 p-4">
      <div class={cn('grid w-full gap-4', props.wide ? 'max-w-2xl' : 'max-w-sm')}>
        <Card>
          <CardHeader class="justify-items-center text-center">
            <div class="mb-2 flex size-10 items-center justify-center rounded-xl bg-primary text-primary-foreground" aria-label={t('app.name')}>
              <Wallet class="size-5" />
            </div>
            <CardTitle class="text-xl">{props.title}</CardTitle>
            <Show when={props.description}>
              <CardDescription>{props.description}</CardDescription>
            </Show>
          </CardHeader>
          <CardContent>{props.children}</CardContent>
        </Card>
        <Show when={props.footer}>
          <div class="text-center text-sm text-muted-foreground">{props.footer}</div>
        </Show>
      </div>
    </div>
  );
};
