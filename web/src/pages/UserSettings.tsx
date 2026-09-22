import { createEffect, createResource, createSignal, For, on } from 'solid-js';
import { api } from '~/api/client';
import type { Language, Theme } from '~/api/types';
import { PageHeader } from '~/components/AppShell';
import { Button } from '~/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '~/components/ui/card';
import { Field, Input, Select } from '~/components/ui/input';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';
import { formatDate, today } from '~/lib/dates';
import { useSession } from '~/stores/session';

const DATE_FORMATS = ['YYYY-MM-DD', 'YYYY/MM/DD', 'DD/MM/YYYY', 'MM/DD/YYYY'];
const LANGUAGES: Language[] = ['en', 'zh', 'es'];
const THEMES: Theme[] = ['system', 'light', 'dark'];

function timeZones(): string[] {
  try {
    return (Intl as unknown as { supportedValuesOf(k: string): string[] }).supportedValuesOf('timeZone');
  } catch {
    return ['UTC'];
  }
}

export default function UserSettings() {
  const { t, te, fieldErrors } = useI18n();
  const { user, currencies, saveSettings } = useSession();
  const [books] = createResource(() => api.books());

  const [displayName, setDisplayName] = createSignal('');
  const [language, setLanguage] = createSignal<Language>('en');
  const [displayCurrency, setDisplayCurrency] = createSignal('USD');
  const [timezone, setTimezone] = createSignal('UTC');
  const [dateFormat, setDateFormat] = createSignal('YYYY-MM-DD');
  const [theme, setTheme] = createSignal<Theme>('system');
  const [defaultBook, setDefaultBook] = createSignal<number | null>(null);
  const [errors, setErrors] = createSignal<Record<string, string>>({});
  const [busy, setBusy] = createSignal(false);

  createEffect(on(user, (u) => {
    if (!u) return;
    setDisplayName(u.display_name);
    setLanguage(u.language);
    setDisplayCurrency(u.display_currency);
    setTimezone(u.timezone);
    setDateFormat(u.date_format);
    setTheme(u.theme);
    setDefaultBook(u.default_book_id);
  }, { defer: false }));

  const save = async (e: Event) => {
    e.preventDefault();
    setBusy(true);
    try {
      // The session store applies the new language and theme immediately.
      await saveSettings({
        display_name: displayName(), language: language(), display_currency: displayCurrency(),
        timezone: timezone(), date_format: dateFormat(), theme: theme(), default_book_id: defaultBook(),
      });
      setErrors({});
      toast.success(t('common.saved'));
    } catch (err) {
      setErrors(fieldErrors(err));
      toast.error(te(err));
    } finally {
      setBusy(false);
    }
  };

  const [current, setCurrent] = createSignal('');
  const [next, setNext] = createSignal('');
  const [pwErrors, setPwErrors] = createSignal<Record<string, string>>({});
  const changePassword = async (e: Event) => {
    e.preventDefault();
    try {
      await api.changePassword(current(), next());
      setCurrent('');
      setNext('');
      setPwErrors({});
      toast.success(t('settings.password_changed'));
    } catch (err) {
      setPwErrors(fieldErrors(err));
      toast.error(te(err));
    }
  };

  return (
    <>
      <PageHeader title={t('settings.title')} />
      <div class="grid gap-6">
        <Card>
          <CardHeader><CardTitle>{t('settings.profile')}</CardTitle></CardHeader>
          <CardContent>
            <form class="grid gap-4 sm:grid-cols-2" onSubmit={save}>
              <Field label={t('settings.display_name')}>
                <Input value={displayName()} onInput={(e) => setDisplayName(e.currentTarget.value)} />
              </Field>
              <Field label={t('settings.language')} error={errors().language}>
                <Select value={language()} onChange={(e) => setLanguage(e.currentTarget.value as Language)}>
                  <For each={LANGUAGES}>{(l) => <option value={l}>{t(`languages.${l}`)}</option>}</For>
                </Select>
              </Field>
              <Field label={t('settings.display_currency')} hint={t('settings.display_currency_hint')} error={errors().display_currency}>
                <Select value={displayCurrency()} onChange={(e) => setDisplayCurrency(e.currentTarget.value)}>
                  <For each={currencies() ?? []}>{(c) => <option value={c.code}>{c.code} · {c.name}</option>}</For>
                </Select>
              </Field>
              <Field label={t('settings.timezone')} error={errors().timezone}>
                <Select value={timezone()} onChange={(e) => setTimezone(e.currentTarget.value)}>
                  <For each={timeZones()}>{(z) => <option value={z}>{z}</option>}</For>
                </Select>
              </Field>
              <Field label={t('settings.date_format')}>
                <Select value={dateFormat()} onChange={(e) => setDateFormat(e.currentTarget.value)}>
                  <For each={DATE_FORMATS}>{(f) => <option value={f}>{f} · {formatDate(today(), f)}</option>}</For>
                </Select>
              </Field>
              <Field label={t('settings.theme')}>
                <Select value={theme()} onChange={(e) => setTheme(e.currentTarget.value as Theme)}>
                  <For each={THEMES}>{(th) => <option value={th}>{t(`settings.theme_${th}`)}</option>}</For>
                </Select>
              </Field>
              <Field label={t('settings.default_book')} error={errors().default_book_id}>
                <Select value={defaultBook() ?? ''} onChange={(e) => setDefaultBook(e.currentTarget.value ? Number(e.currentTarget.value) : null)}>
                  <option value="">{t('common.none')}</option>
                  <For each={books() ?? []}>{(b) => <option value={b.id}>{b.name}</option>}</For>
                </Select>
              </Field>
              <div class="sm:col-span-2">
                <Button type="submit" disabled={busy()}>{busy() ? t('common.saving') : t('common.save')}</Button>
              </div>
            </form>
          </CardContent>
        </Card>

        <Card>
          <CardHeader><CardTitle>{t('settings.password')}</CardTitle></CardHeader>
          <CardContent>
            <form class="grid gap-4 sm:grid-cols-2" onSubmit={changePassword}>
              <Field label={t('settings.current_password')} error={pwErrors().current_password}>
                <Input type="password" autocomplete="current-password" required value={current()} onInput={(e) => setCurrent(e.currentTarget.value)} />
              </Field>
              <Field label={t('settings.new_password')} error={pwErrors().new_password}>
                <Input type="password" autocomplete="new-password" required minLength={8} value={next()} onInput={(e) => setNext(e.currentTarget.value)} />
              </Field>
              <div class="sm:col-span-2">
                <Button type="submit" variant="outline">{t('settings.change_password')}</Button>
              </div>
            </form>
          </CardContent>
        </Card>
      </div>
    </>
  );
}
