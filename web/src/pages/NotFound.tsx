import { A } from '@solidjs/router';
import { buttonVariants } from '~/components/ui/button';
import { useI18n } from '~/i18n';

export default function NotFound() {
  const { t } = useI18n();
  return (
    <div class="flex min-h-[60vh] flex-col items-center justify-center gap-3 text-center">
      <p class="text-5xl font-semibold text-muted-foreground">404</p>
      <p class="text-lg">{t('not_found.title')}</p>
      <A href="/" class={buttonVariants({ variant: 'outline' })}>{t('not_found.home')}</A>
    </div>
  );
}
