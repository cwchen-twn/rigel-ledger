import { Navigate, Route, Router, useLocation, useParams, type RouteSectionProps } from '@solidjs/router';
import { Match, Show, Switch, type JSX, type ParentComponent } from 'solid-js';
import { AppShell } from '~/components/AppShell';
import { Toaster } from '~/components/ui/toast';
import { I18nProvider } from '~/i18n';
import Accounts from '~/pages/Accounts';
import BookSettings from '~/pages/BookSettings';
import Admin from '~/pages/admin/Admin';
import Home from '~/pages/Home';
import Invite from '~/pages/Invite';
import Login from '~/pages/Login';
import Register from '~/pages/Register';
import RequestAccess from '~/pages/RequestAccess';
import VerifyLink from '~/pages/VerifyLink';
import Welcome from '~/pages/Welcome';
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

/**
 * Wait for the session; send anonymous visitors to /login, and anyone who
 * has not finished the first-login wizard to /welcome (the API refuses them
 * everything else anyway).
 */
const RequireUser: ParentComponent = (props) => {
  const { user } = useSession();
  const location = useLocation();
  const pending = () => {
    const u = user();
    return !!u && (!u.initialized || u.password_must_change);
  };
  return (
    <Switch>
      <Match when={user.loading && !user()}>{null}</Match>
      <Match when={user() === null}>
        <Navigate href={`/login`} />
      </Match>
      <Match when={pending() && location.pathname !== '/welcome'}>
        <Navigate href="/welcome" />
      </Match>
      <Match when={user()}>{props.children}</Match>
    </Switch>
  );
};

function WelcomeRoute() {
  const { user } = useSession();
  return (
    <RequireUser>
      <Show when={!user()?.initialized || user()?.password_must_change} fallback={<Navigate href="/" />}>
        <Welcome />
      </Show>
    </RequireUser>
  );
}

/** Signed-out pages: a signed-in visitor goes home instead. */
function SignedOut(props: { page: () => JSX.Element }) {
  const { user } = useSession();
  return (
    <Show when={!user.loading}>
      <Show when={user()} fallback={props.page()}>
        <Navigate href="/" />
      </Show>
    </Show>
  );
}

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


export default function App() {
  return (
    <I18nProvider>
      <Router root={Root}>
        <Route path="/login" component={() => <SignedOut page={() => <Login />} />} />
        <Route path="/register" component={() => <SignedOut page={() => <Register />} />} />
        <Route path="/request-access" component={() => <SignedOut page={() => <RequestAccess />} />} />
        {/* These two sign the visitor in themselves. */}
        <Route path="/invite/:token" component={() => <SignedOut page={() => <Invite />} />} />
        <Route path="/verify" component={VerifyLink} />
        <Route path="/welcome" component={WelcomeRoute} />
        <Route path="/" component={() => <RequireUser><Home /></RequireUser>} />
        <Route path="/" component={UserLayout}>
          <Route path="/onboarding" component={Onboarding} />
          <Route path="/settings" component={UserSettings} />
          <Route path="/admin/:tab?" component={Admin} />
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
