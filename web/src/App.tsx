import { Navigate, Route, Router, useParams, type RouteSectionProps } from '@solidjs/router';
import { Match, Show, Switch, type ParentComponent } from 'solid-js';
import { AppShell } from '~/components/AppShell';
import { Toaster } from '~/components/ui/toast';
import { I18nProvider } from '~/i18n';
import Accounts from '~/pages/Accounts';
import BookSettings from '~/pages/BookSettings';
import Home from '~/pages/Home';
import Login from '~/pages/Login';
import NotFound from '~/pages/NotFound';
import Onboarding from '~/pages/Onboarding';
import Overview from '~/pages/Overview';
import Transactions from '~/pages/Transactions';
import UserSettings from '~/pages/UserSettings';
import { BookProvider } from '~/stores/book';
import { SessionProvider, useSession } from '~/stores/session';

/** Providers that need the router (navigation on 401) wrap every route. */
function Root(props: RouteSectionProps) {
  return (
    <SessionProvider>
      {props.children}
      <Toaster />
    </SessionProvider>
  );
}

/** Wait for the session; send anonymous visitors to /login. */
const RequireUser: ParentComponent = (props) => {
  const { user } = useSession();
  return (
    <Switch>
      <Match when={user.loading}>{null}</Match>
      <Match when={user() === null}>
        <Navigate href="/login" />
      </Match>
      <Match when={user()}>{props.children}</Match>
    </Switch>
  );
};

/** Signed-in pages outside a book: the shell points at the default book. */
function UserLayout(props: RouteSectionProps) {
  const { user } = useSession();
  return (
    <RequireUser>
      <AppShell bookId={user()?.default_book_id ?? null}>{props.children}</AppShell>
    </RequireUser>
  );
}

function BookLayout(props: RouteSectionProps) {
  const params = useParams();
  const id = () => Number(params.bookId);
  return (
    <RequireUser>
      <Show when={Number.isInteger(id()) && id() > 0} fallback={<NotFound />}>
        {/* keyed: switching books remounts the providers with fresh data */}
        <Show when={id()} keyed>
          {(bookId) => (
            <BookProvider bookId={bookId}>
              <AppShell bookId={bookId}>{props.children}</AppShell>
            </BookProvider>
          )}
        </Show>
      </Show>
    </RequireUser>
  );
}

function LoginRoute() {
  const { user } = useSession();
  return (
    <Show when={!user.loading}>
      <Show when={user()} fallback={<Login />}>
        <Navigate href="/" />
      </Show>
    </Show>
  );
}

export default function App() {
  return (
    <I18nProvider>
      <Router root={Root}>
        <Route path="/login" component={LoginRoute} />
        <Route path="/" component={() => <RequireUser><Home /></RequireUser>} />
        <Route path="/" component={UserLayout}>
          <Route path="/onboarding" component={Onboarding} />
          <Route path="/settings" component={UserSettings} />
        </Route>
        <Route path="/b/:bookId" component={BookLayout}>
          <Route path="/" component={Overview} />
          <Route path="/transactions" component={Transactions} />
          <Route path="/accounts" component={Accounts} />
          <Route path="/settings" component={BookSettings} />
        </Route>
        <Route path="*" component={NotFound} />
      </Router>
    </I18nProvider>
  );
}
