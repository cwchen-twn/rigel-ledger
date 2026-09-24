import { useNavigate } from '@solidjs/router';
import { createContext, createEffect, createResource, onCleanup, useContext, type ParentComponent, type Resource } from 'solid-js';
import { api, ApiError, setUnauthenticatedHandler } from '~/api/client';
import type { Commodity, CommodityKind, Settings, Theme, User } from '~/api/types';
import { useI18n } from '~/i18n';

interface Session {
  user: Resource<User | null>;
  /** Everything an account can hold: currencies, securities, points. */
  commodities: Resource<Commodity[]>;
  refetchCommodities: () => void;
  /** ISO currencies only: book base, display currency, exchange rates. */
  currencies: () => Commodity[] | undefined;
  commodity: (code: string) => Commodity | undefined;
  kind: (code: string) => CommodityKind;
  decimals: (code: string) => number;
  login: (username: string, password: string) => Promise<User>;
  logout: () => Promise<void>;
  saveSettings: (s: Settings) => Promise<User>;
  /** Replace the signed-in user after an update elsewhere (identity change, sign-in by link). */
  setUser: (u: User) => void;
  /** Reload /api/me (e.g. to pick up pending_email). */
  refresh: () => void;
}

const Ctx = createContext<Session>();

async function fetchMe(): Promise<User | null> {
  try {
    return await api.me();
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) return null;
    throw e;
  }
}

/** Put the .dark class on <html> for the chosen theme, following the OS for "system". */
function applyTheme(theme: Theme) {
  const root = document.documentElement;
  root.dataset.theme = theme;
  const dark = theme === 'dark' || (theme === 'system' && matchMedia('(prefers-color-scheme: dark)').matches);
  root.classList.toggle('dark', dark);
}

export const SessionProvider: ParentComponent = (props) => {
  const navigate = useNavigate();
  const { setLocale } = useI18n();
  const [user, { mutate, refetch }] = createResource(fetchMe);
  const [commodities, { refetch: refetchCommodities }] = createResource(() => api.commodities());
  const currencies = () => commodities()?.filter((c) => c.kind === 'currency');
  const commodity = (code: string) => commodities()?.find((c) => c.code === code);

  setUnauthenticatedHandler(() => {
    mutate(null);
    navigate('/login', { replace: true });
  });

  // Language and theme apply the moment settings change -- no reload.
  // Signed out, the pages follow the browser's language.
  createEffect(() => {
    const u = user();
    if (u === null) {
      const l = navigator.language?.slice(0, 2);
      if (l === 'en' || l === 'zh' || l === 'es') setLocale(l);
      return;
    }
    if (!u) return;
    setLocale(u.language);
    applyTheme(u.theme);
  });
  const media = matchMedia('(prefers-color-scheme: dark)');
  const onMedia = () => applyTheme((user()?.theme ?? 'system') as Theme);
  media.addEventListener('change', onMedia);
  onCleanup(() => media.removeEventListener('change', onMedia));

  const decimals = (code: string) => commodity(code)?.decimals ?? 2;
  const kind = (code: string): CommodityKind => commodity(code)?.kind ?? 'currency';

  const session: Session = {
    user,
    commodities,
    refetchCommodities,
    currencies,
    commodity,
    kind,
    decimals,
    async login(username, password) {
      const { user: u } = await api.login(username, password);
      mutate(u);
      return u;
    },
    async logout() {
      try {
        await api.logout();
      } finally {
        mutate(null);
        navigate('/login', { replace: true });
      }
    },
    async saveSettings(s) {
      const u = await api.updateSettings(s);
      mutate(u);
      return u;
    },
    setUser: (u) => mutate(u),
    refresh: () => void refetch(),
  };

  return <Ctx.Provider value={session}>{props.children}</Ctx.Provider>;
};

export function useSession(): Session {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error('useSession must be used within SessionProvider');
  return ctx;
}
