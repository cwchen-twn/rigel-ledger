import { useNavigate } from '@solidjs/router';
import { createSignal, For, Show } from 'solid-js';
import { api } from '~/api/client';
import { Button } from '~/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '~/components/ui/card';
import { Field, Input, Select } from '~/components/ui/input';
import { useI18n } from '~/i18n';
import { useSession } from '~/stores/session';

export default function Onboarding() {
  const { t, te, fieldErrors } = useI18n();
  const { user, currencies } = useSession();
  const navigate = useNavigate();
  const [name, setName] = createSignal('');
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
      navigate(`/b/${b.id}`, { replace: true });
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
          <CardTitle class="text-xl">{t('onboarding.title')}</CardTitle>
          <CardDescription>{t('onboarding.description')}</CardDescription>
        </CardHeader>
        <CardContent>
          <form class="grid gap-4" onSubmit={submit}>
            <Field label={t('onboarding.book_name')} error={errors().name}>
              <Input required placeholder={t('onboarding.book_name_placeholder')} value={name()} onInput={(e) => setName(e.currentTarget.value)} />
            </Field>
            <Field label={t('onboarding.base_currency')} hint={t('onboarding.base_currency_hint')} error={errors().base_currency}>
              <Select value={currency()} onChange={(e) => setCurrency(e.currentTarget.value)}>
                <For each={currencies() ?? []}>{(c) => <option value={c.code}>{c.code} · {c.name}</option>}</For>
              </Select>
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
