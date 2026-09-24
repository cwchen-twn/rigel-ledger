import { A, useLocation, useNavigate } from '@solidjs/router';
import { BookOpen, ChartColumn, ChevronsUpDown, Inbox, LayoutDashboard, List, LogOut, Menu, Plus, Settings, Shield, SlidersHorizontal, Wallet } from 'lucide-solid';
import { createResource, createSignal, Show, type JSX, type ParentComponent } from 'solid-js';
import { api } from '~/api/client';
import { DropdownMenu, type MenuItem } from '~/components/ui/dropdown-menu';
import { Sheet } from '~/components/ui/dialog';
import { useI18n } from '~/i18n';
import { cn } from '~/lib/cn';
import { useSession } from '~/stores/session';

function NavLink(props: { href: string; icon: JSX.Element; label: string; end?: boolean; onClick?: () => void }) {
  return (
    <A
      href={props.href}
      end={props.end}
      onClick={props.onClick}
      class="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground [&_svg]:size-4"
      activeClass="!bg-accent !text-accent-foreground font-medium"
    >
      {props.icon}
      {props.label}
    </A>
  );
}

/**
 * The frame around every signed-in page: a sidebar on desktop, a slide-in
 * sheet on phones. `bookId` is the book the navigation points at -- the one
 * in the URL, or the user's default on pages outside a book.
 */
export const AppShell: ParentComponent<{ bookId: number | null }> = (props) => {
  const { t } = useI18n();
  const { user, logout } = useSession();
  const navigate = useNavigate();
  const location = useLocation();
  const [books] = createResource(() => api.books());
  const [mobileOpen, setMobileOpen] = createSignal(false);
  const current = () => books()?.find((b) => b.id === props.bookId);

  const bookItems = (): MenuItem[] => [
    ...(books() ?? []).map((b) => ({
      label: `${b.name} · ${b.base_currency}`,
      icon: <BookOpen />,
      onSelect: () => {
        // Stay on the same kind of page in the other book.
        const rest = location.pathname.replace(/^\/b\/\d+/, '');
        navigate(`/b/${b.id}${location.pathname.startsWith('/b/') ? rest : ''}`);
      },
    })),
    { label: t('nav.new_book'), icon: <Plus />, onSelect: () => navigate('/onboarding'), separatorBefore: true },
  ];

  const nav = (close?: () => void) => (
    <div class="flex h-full flex-col gap-4 p-3">
      <div class="flex items-center gap-2 px-2 pt-1">
        <div class="flex size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground [&_svg]:size-4">
          <Wallet />
        </div>
        <div class="grid leading-tight">
          <span class="text-sm font-semibold">{t('app.name')}</span>
          <span class="text-xs text-muted-foreground">{t('app.tagline')}</span>
        </div>
      </div>

      {/* A book is the household's whole ledger, not an account: with one
          book there is nothing to pick, so it is a label, not a menu. */}
      <div class="grid gap-1 px-2">
        <span class="text-[11px] uppercase tracking-wide text-muted-foreground">{t('nav.book')}</span>
        <Show
          when={(books()?.length ?? 0) > 1}
          fallback={
            <span class="truncate text-sm font-medium">
              {current()?.name ?? '—'}
              <Show when={current()}><span class="ml-1 text-xs font-normal text-muted-foreground">{current()!.base_currency}</span></Show>
            </span>
          }
        >
          <DropdownMenu
            label={t('nav.books')}
            triggerClass="-mx-2 flex items-center justify-between gap-2 rounded-md px-2 py-1 text-left text-sm font-medium hover:bg-accent"
            trigger={
              <>
                <span class="truncate">
                  {current()?.name ?? t('nav.books')}
                  <Show when={current()}>
                    <span class="ml-1 text-xs font-normal text-muted-foreground">{current()!.base_currency}</span>
                  </Show>
                </span>
                <ChevronsUpDown class="size-4 shrink-0 text-muted-foreground" />
              </>
            }
            items={bookItems()}
          />
        </Show>
      </div>

      <Show when={props.bookId}>
        {(id) => (
          <nav class="grid gap-0.5">
            <NavLink href={`/b/${id()}`} end icon={<LayoutDashboard />} label={t('nav.overview')} onClick={close} />
            <NavLink href={`/b/${id()}/transactions`} icon={<List />} label={t('nav.transactions')} onClick={close} />
            <NavLink href={`/b/${id()}/accounts`} icon={<BookOpen />} label={t('nav.accounts')} onClick={close} />
            <NavLink href={`/b/${id()}/reports`} icon={<ChartColumn />} label={t('nav.reports')} onClick={close} />
            <NavLink href={`/b/${id()}/imports`} icon={<Inbox />} label={t('nav.imports')} onClick={close} />
            <NavLink href={`/b/${id()}/settings`} icon={<SlidersHorizontal />} label={t('nav.book_settings')} onClick={close} />
          </nav>
        )}
      </Show>

      <div class="mt-auto grid gap-0.5 border-t pt-3">
        <Show when={user()?.is_admin}>
          <NavLink href="/admin" icon={<Shield />} label={t('nav.admin')} onClick={close} />
        </Show>
        <NavLink href="/settings" icon={<Settings />} label={t('nav.settings')} onClick={close} />
        <button
          type="button"
          onClick={() => logout()}
          class="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground [&_svg]:size-4"
        >
          <LogOut />
          {t('nav.logout')}
        </button>
        <p class="truncate px-2 pt-2 text-xs text-muted-foreground">{user()?.display_name || user()?.username}</p>
      </div>
    </div>
  );

  return (
    <div class="flex min-h-screen">
      <aside class="sticky top-0 hidden h-screen w-60 shrink-0 border-r bg-card md:block">{nav()}</aside>
      <div class="flex min-w-0 flex-1 flex-col">
        <header class="sticky top-0 z-30 flex h-12 items-center gap-2 border-b bg-background/95 px-3 backdrop-blur md:hidden">
          <button type="button" aria-label={t('nav.menu')} class="rounded-md p-2 hover:bg-accent" onClick={() => setMobileOpen(true)}>
            <Menu class="size-5" />
          </button>
          <span class="truncate text-sm font-medium">{current()?.name ?? t('app.name')}</span>
        </header>
        <main class={cn('mx-auto w-full max-w-6xl flex-1 p-4 md:p-8')}>{props.children}</main>
      </div>
      <Sheet open={mobileOpen()} onOpenChange={setMobileOpen} title={t('app.name')} class="max-w-72 p-0 sm:max-w-72">
        {nav(() => setMobileOpen(false))}
      </Sheet>
    </div>
  );
};

/** Page title row with optional actions on the right. */
export function PageHeader(props: { title: string; description?: string; actions?: JSX.Element }) {
  return (
    <div class="mb-6 flex flex-wrap items-end justify-between gap-3">
      <div class="grid gap-1">
        <h1 class="text-2xl font-semibold tracking-tight">{props.title}</h1>
        <Show when={props.description}>
          <p class="text-sm text-muted-foreground">{props.description}</p>
        </Show>
      </div>
      <Show when={props.actions}>
        <div class="flex flex-wrap items-center gap-2">{props.actions}</div>
      </Show>
    </div>
  );
}
