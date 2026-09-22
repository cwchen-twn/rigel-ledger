import { createContext, createSignal, useContext, type ParentComponent } from 'solid-js';
import { ApiError } from '~/api/client';
import en from './en.json';
import zh from './zh.json';
import es from './es.json';

export type Locale = 'en' | 'zh' | 'es';

// en.json is the source of truth; zh.json and es.json are generated from the
// same table and carry the same keys.
const dicts: Record<Locale, unknown> = { en, zh, es };

/** BCP 47 tags for Intl (number and currency formatting). */
export const intlLocale: Record<Locale, string> = { en: 'en-US', zh: 'zh-TW', es: 'es-ES' };

export interface I18n {
  t: (key: string, vars?: Record<string, string | number>) => string;
  /** Translate an API error, falling back to its message and then to a generic one. */
  te: (err: unknown) => string;
  /** Translate per-field error codes into messages. */
  fieldErrors: (err: unknown) => Record<string, string>;
  locale: () => Locale;
  intl: () => string;
  setLocale: (l: Locale) => void;
}

const Ctx = createContext<I18n>();

function lookup(dict: unknown, key: string): string | undefined {
  let node: unknown = dict;
  for (const part of key.split('.')) {
    if (node == null || typeof node !== 'object') return undefined;
    node = (node as Record<string, unknown>)[part];
  }
  return typeof node === 'string' ? node : undefined;
}

export const I18nProvider: ParentComponent = (props) => {
  const [locale, setLocale] = createSignal<Locale>(
    (document.documentElement.lang as Locale) in dicts ? (document.documentElement.lang as Locale) : 'en',
  );

  const t = (key: string, vars?: Record<string, string | number>): string => {
    const s = lookup(dicts[locale()], key) ?? lookup(dicts.en, key) ?? key;
    return vars ? s.replace(/\{(\w+)\}/g, (_, k) => String(vars[k] ?? `{${k}}`)) : s;
  };

  const te = (err: unknown): string => {
    if (err instanceof ApiError) {
      const s = lookup(dicts[locale()], `error.${err.code}`);
      return s ?? (err.message || t('error.generic'));
    }
    if (err instanceof TypeError) return t('error.network');
    return t('error.generic');
  };

  const fieldErrors = (err: unknown): Record<string, string> => {
    if (!(err instanceof ApiError)) return {};
    const out: Record<string, string> = {};
    for (const [field, code] of Object.entries(err.fields)) out[field] = t(`field.${code}`);
    return out;
  };

  const set = (l: Locale) => {
    setLocale(l);
    document.documentElement.lang = l;
  };

  return (
    <Ctx.Provider value={{ t, te, fieldErrors, locale, intl: () => intlLocale[locale()], setLocale: set }}>
      {props.children}
    </Ctx.Provider>
  );
};

export function useI18n(): I18n {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error('useI18n must be used within I18nProvider');
  return ctx;
}
