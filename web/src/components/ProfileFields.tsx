import { For } from 'solid-js';
import type { SetStoreFunction } from 'solid-js/store';
import type { Language, Profile, Theme } from '~/api/types';
import { Field, Input, Select } from '~/components/ui/input';
import { SearchSelect } from '~/components/ui/search-select';
import { commodityOptions, timeZoneOptions } from '~/lib/options';
import { useI18n } from '~/i18n';
import { formatDate, today } from '~/lib/dates';
import { useSession } from '~/stores/session';

export const DATE_FORMATS = ['YYYY-MM-DD', 'YYYY/MM/DD', 'DD/MM/YYYY', 'MM/DD/YYYY'];
export const LANGUAGES: Language[] = ['en', 'zh', 'es'];
export const THEMES: Theme[] = ['system', 'light', 'dark'];

export function timeZones(): string[] {
  try {
    return (Intl as unknown as { supportedValuesOf(k: string): string[] }).supportedValuesOf('timeZone');
  } catch {
    return ['UTC'];
  }
}

/** The browser's own choices, offered when a profile starts out on the defaults. */
export function browserTimeZone(): string | undefined {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone;
  } catch {
    return undefined;
  }
}

export function browserLanguage(): Language | undefined {
  const l = navigator.language?.slice(0, 2);
  return LANGUAGES.includes(l as Language) ? (l as Language) : undefined;
}

type Key = keyof Profile;

/**
 * The profile form shared by the first-login wizard, the invitation page and
 * (in parts) the settings page. `only` limits which fields are shown.
 */
export function ProfileFields(props: {
  profile: Profile;
  set: SetStoreFunction<Profile>;
  errors: Record<string, string>;
  only?: Key[];
  class?: string;
}) {
  const { t } = useI18n();
  const { currencies } = useSession();
  const show = (k: Key) => !props.only || props.only.includes(k);
  return (
    <div class={props.class ?? 'grid gap-4 sm:grid-cols-2'}>
      {show('username') && (
        <Field label={t('auth.username')} hint={t('profile.username_hint')} error={props.errors.username}>
          <Input autocomplete="username" required value={props.profile.username} onInput={(e) => props.set('username', e.currentTarget.value)} />
        </Field>
      )}
      {show('display_name') && (
        <Field label={t('settings.display_name')} error={props.errors.display_name}>
          <Input autocomplete="name" value={props.profile.display_name} onInput={(e) => props.set('display_name', e.currentTarget.value)} />
        </Field>
      )}
      {show('language') && (
        <Field label={t('settings.language')} error={props.errors.language}>
          <Select value={props.profile.language} onChange={(e) => props.set('language', e.currentTarget.value as Language)}>
            <For each={LANGUAGES}>{(l) => <option value={l}>{t(`languages.${l}`)}</option>}</For>
          </Select>
        </Field>
      )}
      {show('display_currency') && (
        <Field label={t('settings.display_currency')} hint={t('settings.display_currency_hint')} error={props.errors.display_currency}>
          <SearchSelect options={commodityOptions(currencies())} value={props.profile.display_currency} onChange={(v) => props.set('display_currency', v)} />
        </Field>
      )}
      {show('timezone') && (
        <Field label={t('settings.timezone')} error={props.errors.timezone}>
          <SearchSelect options={timeZoneOptions(timeZones())} value={props.profile.timezone} onChange={(v) => props.set('timezone', v)} />
        </Field>
      )}
      {show('date_format') && (
        <Field label={t('settings.date_format')} error={props.errors.date_format}>
          <Select value={props.profile.date_format} onChange={(e) => props.set('date_format', e.currentTarget.value)}>
            <For each={DATE_FORMATS}>{(f) => <option value={f}>{f} · {formatDate(today(), f)}</option>}</For>
          </Select>
        </Field>
      )}
      {show('theme') && (
        <Field label={t('settings.theme')} error={props.errors.theme}>
          <Select value={props.profile.theme} onChange={(e) => props.set('theme', e.currentTarget.value as Theme)}>
            <For each={THEMES}>{(th) => <option value={th}>{t(`settings.theme_${th}`)}</option>}</For>
          </Select>
        </Field>
      )}
    </div>
  );
}
