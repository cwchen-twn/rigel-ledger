import { useNavigate } from '@solidjs/router';
import { createResource, createSignal, Show } from 'solid-js';
import { api } from '~/api/client';
import { Button } from '~/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '~/components/ui/card';
import { Field, Input } from '~/components/ui/input';
import { SearchSelect } from '~/components/ui/search-select';
import { commodityOptions } from '~/lib/options';
import { useI18n } from '~/i18n';
import { useSession } from '~/stores/session';

export default function Onboarding() {
  const { t, te, fieldErrors } = useI18n();
  const { user, currencies, refresh } = useSession();
  const navigate = useNavigate();
  const [name, setName] = createSignal(t('onboarding.book_name_default'));
  // Reached again through "New book": say plainly that a bank account is not a book.
  const [books] = createResource(() => api.books().catch(() => []));
  const another = () => (books()?.length ?? 0) > 0;
  const [currency, setCurrency] = createSignal(user()?.display_currency ?? 'USD');
  const [errors, setErrors] = createSignal<Record<string, string>>({});
  const [error, setError] = createSignal('');
  const [busy, setBusy] = createSignal(false);

  const submit = async (e: Event) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      const b = await api.createBook(name(), currency());
      // The first book becomes the default one: reload the user to know it.
      refresh();
      // Next: the accounts that live in it (each bank, card, broker).
      navigate(`/b/${b.id}/accounts?new=bank&welcome=1`, { replace: true });
    } catch (err) {
      setErrors(fieldErrors(err));
      setError(te(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div class="flex min-h-[70vh] items-center justify-center">
      <Card class="w-full max-w-md">
        <CardHeader>
          <CardTitle class="text-xl">{another() ? t('onboarding.another_title') : t('onboarding.title')}</CardTitle>
          <CardDescription>{another() ? t('onboarding.another_description') : t('onboarding.description')}</CardDescription>
        </CardHeader>
        <CardContent>
          <form class="grid gap-4" onSubmit={submit}>
            <Field label={t('onboarding.book_name')} error={errors().name}>
              <Input required placeholder={t('onboarding.book_name_placeholder')} value={name()} onInput={(e) => setName(e.currentTarget.value)} />
            </Field>
            <Field label={t('onboarding.base_currency')} hint={t('onboarding.base_currency_hint')} error={errors().base_currency}>
              <SearchSelect options={commodityOptions(currencies())} value={currency()} onChange={setCurrency} />
            </Field>
            <Show when={error() && Object.keys(errors()).length === 0}>
              <p role="alert" class="text-sm text-destructive">{error()}</p>
            </Show>
            <Button type="submit" disabled={busy()}>{t('common.create')}</Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
