import { useNavigate } from '@solidjs/router';
import { createResource, createSignal, Show } from 'solid-js';
import { api } from '~/api/client';
import { Button } from '~/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '~/components/ui/card';
import { Field, Input } from '~/components/ui/input';
import { SearchSelect } from '~/components/ui/search-select';
import { toast } from '~/components/ui/toast';
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

  // Or start from a book file exported elsewhere: read in the tab, sent as JSON.
  const [importing, setImporting] = createSignal<number | null>(null);
  const [importError, setImportError] = createSignal('');
  const importFile = async (file: File | undefined) => {
    if (!file) return;
    setImportError('');
    let parsed: { format?: string; transactions?: unknown[] };
    try {
      parsed = JSON.parse(await file.text());
    } catch {
      setImportError(t('onboarding.import_bad_file'));
      return;
    }
    if (parsed?.format !== 'rigelledger-book') {
      setImportError(t('onboarding.import_bad_file'));
      return;
    }
    setImporting(parsed.transactions?.length ?? 0);
    try {
      const res = await api.importBook(parsed);
      refresh();
      toast.success(t('onboarding.imported', { transactions: String(res.transactions), name: res.book.name }));
      navigate(`/b/${res.book.id}`, { replace: true });
    } catch (err) {
      setImportError(te(err));
    } finally {
      setImporting(null);
    }
  };

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
            <Button type="submit" disabled={busy() || importing() !== null}>{t('common.create')}</Button>
          </form>
          <div class="mt-6 grid gap-3 border-t pt-4">
            <div>
              <h3 class="text-sm font-medium">{t('onboarding.import_title')}</h3>
              <p class="text-sm text-muted-foreground">{t('onboarding.import_hint')}</p>
            </div>
            <Field label={t('onboarding.import_file')}>
              <Input type="file" accept=".json,application/json" disabled={importing() !== null}
                onChange={(e) => { void importFile(e.currentTarget.files?.[0]); e.currentTarget.value = ''; }} />
            </Field>
            <Show when={importing() !== null}>
              <p role="status" class="text-sm text-muted-foreground">{t('onboarding.importing', { n: String(importing()) })}</p>
            </Show>
            <Show when={importError()}>
              <p role="alert" class="text-sm text-destructive">{importError()}</p>
            </Show>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
