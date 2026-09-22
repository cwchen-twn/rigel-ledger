import { createContext, createSignal, useContext, type ParentComponent } from 'solid-js';
import en from './en.json';
import zh from './zh.json';
import es from './es.json';

export type Locale = 'en' | 'zh' | 'es';
type Dict = typeof en;

const dicts: Record<Locale, Dict> = { en, zh, es };

export interface I18nContextValue {
  t: (key: string, vars?: Record<string, string | number>, fallback?: string) => string;
  locale: () => Locale;
  setLocale: (l: Locale) => void;
}

const I18nContext = createContext<I18nContextValue | null>(null);

export const I18nProvider: ParentComponent<{ initialLocale?: Locale }> = (props) => {
  const [locale, setLocale] = createSignal<Locale>(props.initialLocale ?? 'en');

  function t(key: string, vars?: Record<string, string | number>, fallback?: string): string {
    const dict = dicts[locale()];
    const parts = key.split('.');
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    let node: any = dict;
    for (const part of parts) {
      if (node == null || typeof node !== 'object') return fallback ?? key;
      node = node[part];
    }
    if (typeof node !== 'string') return fallback ?? key;
    if (!vars) return node;
    return node.replace(/\{(\w+)\}/g, (_, k) => String(vars[k] ?? `{${k}}`));
  }

  return (
    <I18nContext.Provider value={{ t, locale, setLocale }}>
      {props.children}
    </I18nContext.Provider>
  );
};

export function useI18n(): I18nContextValue {
  const ctx = useContext(I18nContext);
  if (!ctx) throw new Error('useI18n must be used within I18nProvider');
  return ctx;
}
