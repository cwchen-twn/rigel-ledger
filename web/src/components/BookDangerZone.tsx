import { A, useNavigate } from '@solidjs/router';
import { createSignal, Show } from 'solid-js';
import { api } from '~/api/client';
import { buttonVariants } from '~/components/ui/button';
import { Button } from '~/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '~/components/ui/card';
import { Field, Input } from '~/components/ui/input';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';
import { useBook } from '~/stores/book';
import { useSession } from '~/stores/session';

/** Book settings' last card: another book (rarely wanted), or this one gone. */
export function BookDangerZone() {
  const { t, te } = useI18n();
  const { refresh } = useSession();
  const book = useBook();
  const navigate = useNavigate();
  const [confirm, setConfirm] = createSignal('');
  const [busy, setBusy] = createSignal(false);

  const remove = async (e: Event) => {
    e.preventDefault();
    setBusy(true);
    try {
      await api.deleteBook(book.id(), confirm());
      toast.success(t('book.deleted', { name: book.book()?.name ?? '' }));
      await refresh(); // the default book may have been this one
      navigate('/', { replace: true });
    } catch (err) {
      toast.error(te(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card class="border-destructive/40">
      <CardHeader>
        <CardTitle>{t('book.books_title')}</CardTitle>
        <CardDescription>{t('book.books_hint')}</CardDescription>
      </CardHeader>
      <CardContent class="grid gap-6">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <p class="text-sm text-muted-foreground">{t('book.another_hint')}</p>
          <A href="/onboarding" class={buttonVariants({ variant: 'outline', size: 'sm' })}>{t('nav.new_book')}</A>
        </div>
        <Show when={book.isOwner()}>
          <form class="grid gap-3 border-t pt-4" onSubmit={remove}>
            <p class="text-sm font-medium text-destructive">{t('book.delete_title')}</p>
            <p class="text-sm text-muted-foreground">{t('book.delete_hint')}</p>
            <div class="flex flex-wrap items-end gap-2">
              <Field label={t('book.delete_type', { name: book.book()?.name ?? '' })} class="min-w-64">
                <Input value={confirm()} onInput={(e) => setConfirm(e.currentTarget.value)} autocomplete="off" />
              </Field>
              <Button type="submit" variant="destructive" disabled={busy() || confirm().trim() !== book.book()?.name}>
                {t('book.delete_button')}
              </Button>
            </div>
          </form>
        </Show>
      </CardContent>
    </Card>
  );
}
