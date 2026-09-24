import { LogOut } from 'lucide-solid';
import { createEffect, createResource, createSignal, For, on, Show } from 'solid-js';
import { api } from '~/api/client';
import type { Language, Theme } from '~/api/types';
import { PageHeader } from '~/components/AppShell';
import { APITokens } from '~/components/APITokens';
import { EmailVerification } from '~/components/EmailVerification';
import { EventList } from '~/components/EventList';
import { MFASettings } from '~/components/MFASettings';
import { DATE_FORMATS, LANGUAGES, THEMES, timeZones } from '~/components/ProfileFields';
import { Button } from '~/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '~/components/ui/card';
import { Field, Input, Select } from '~/components/ui/input';
import { SearchSelect } from '~/components/ui/search-select';
import { commodityOptions, timeZoneOptions } from '~/lib/options';
import { Badge, Table, tdClass, thClass, trClass } from '~/components/ui/misc';
import { toast } from '~/components/ui/toast';
import { useI18n } from '~/i18n';
import { formatDate, formatDateTime, today } from '~/lib/dates';
import { useSession } from '~/stores/session';

export default function UserSettings() {
  const { t, te, fieldErrors } = useI18n();
  const { user, currencies, saveSettings, setUser: mutateUser, refresh, logout } = useSession();
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

  // ---- sign-in name ----
  const [username, setUsername] = createSignal('');
  const [idPassword, setIdPassword] = createSignal('');
  const [idErrors, setIdErrors] = createSignal<Record<string, string>>({});
  createEffect(on(user, (u) => {
    if (!u) return;
    setUsername(u.username);
  }, { defer: false }));
  const saveIdentity = async (e: Event) => {
    e.preventDefault();
    try {
      mutateUser(await api.updateUsername(username(), idPassword()));
      setIdPassword('');
      setIdErrors({});
      toast.success(t('settings.identity_saved'));
    } catch (err) {
      setIdErrors(fieldErrors(err));
      toast.error(te(err));
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
      refetchSessions();
    } catch (err) {
      setPwErrors(fieldErrors(err));
      toast.error(te(err));
    }
  };

  // ---- security: sessions and sign-in history ----
  const [sessions, { refetch: refetchSessions }] = createResource(() => api.sessions());
  const [events] = createResource(() => api.myEvents());
  const revoke = async (id: number, current: boolean) => {
    if (current) {
      await logout();
      return;
    }
    try {
      await api.revokeSession(id);
      toast.success(t('security.revoked'));
    } catch (err) {
      toast.error(te(err));
    }
    refetchSessions();
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
                <SearchSelect options={commodityOptions(currencies())} value={displayCurrency()} onChange={setDisplayCurrency} />
              </Field>
              <Field label={t('settings.timezone')} error={errors().timezone}>
                <SearchSelect options={timeZoneOptions(timeZones())} value={timezone()} onChange={setTimezone} />
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
          <CardHeader><CardTitle>{t('settings.identity')}</CardTitle></CardHeader>
          <CardContent>
            <form class="grid gap-4 sm:grid-cols-[1fr_1fr_auto] sm:items-end" onSubmit={saveIdentity}>
              <Field label={t('auth.username')} error={idErrors().username}>
                <Input autocomplete="username" required value={username()} onInput={(e) => setUsername(e.currentTarget.value)} />
              </Field>
              <Field label={t('settings.current_password')} error={idErrors().current_password}>
                <Input type="password" autocomplete="current-password" required value={idPassword()} onInput={(e) => setIdPassword(e.currentTarget.value)} />
              </Field>
              <Button type="submit" variant="outline">{t('common.save')}</Button>
            </form>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t('settings.email')}</CardTitle>
            <CardDescription>{t('email.change_hint')}</CardDescription>
          </CardHeader>
          <CardContent>
            <Show when={user()} keyed>
              {(u) => <EmailVerification user={u} needsPassword onChange={mutateUser} onPending={refresh} />}
            </Show>
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

        <MFASettings />

        <Card>
          <CardHeader>
            <CardTitle>{t('security.sessions')}</CardTitle>
            <CardDescription>{t('security.sessions_hint')}</CardDescription>
          </CardHeader>
          <CardContent>
            <Table>
              <thead>
                <tr class="border-b">
                  <th class={thClass}>{t('security.device')}</th>
                  <th class={thClass}>{t('security.ip')}</th>
                  <th class={thClass}>{t('security.last_used')}</th>
                  <th class={thClass}><span class="sr-only">{t('common.actions')}</span></th>
                </tr>
              </thead>
              <tbody>
                <For each={(sessions() ?? []).filter((s) => s.kind !== 'token')}>
                  {(s) => (
                    <tr class={trClass}>
                      <td class={`${tdClass} max-w-sm`}>
                        <div class="truncate text-xs" title={s.user_agent}>{s.user_agent || s.kind}</div>
                        <Show when={s.current}><Badge class="mt-1">{t('security.this_device')}</Badge></Show>
                      </td>
                      <td class={`${tdClass} font-mono text-xs`}>{s.ip || '—'}</td>
                      <td class={`${tdClass} whitespace-nowrap`}>{formatDateTime(s.last_used_at)}</td>
                      <td class={`${tdClass} text-right`}>
                        <Button size="sm" variant="ghost" onClick={() => revoke(s.id, s.current)}>
                          <LogOut /> {t('security.sign_out')}
                        </Button>
                      </td>
                    </tr>
                  )}
                </For>
              </tbody>
            </Table>
          </CardContent>
        </Card>

        <APITokens tokens={(sessions() ?? []).filter((s) => s.kind === 'token')} onChange={refetchSessions} />

        <Card>
          <CardHeader><CardTitle>{t('security.history')}</CardTitle></CardHeader>
          <CardContent>
            <EventList events={events() ?? []} />
          </CardContent>
        </Card>
      </div>
    </>
  );
}
