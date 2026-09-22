import { Router, Route } from '@solidjs/router';
import { createEffect, type JSXElement } from 'solid-js';
import { AuthProvider, useAuth } from './stores/auth';
import { I18nProvider, useI18n, type Locale } from './i18n';
import Layout from './components/Layout';
import Login from './pages/Login';
import Home from './pages/Home';
import Ledgers from './pages/Ledgers';
import Reports from './pages/Reports';
import Settings from './pages/Settings';

// Syncs locale with the user's main_language preference once auth loads.
function LocaleSync(): null {
  const { auth } = useAuth();
  const { setLocale } = useI18n();
  createEffect(() => {
    const lang = auth()?.main_language;
    if (lang === 'en' || lang === 'zh' || lang === 'es') setLocale(lang as Locale);
  });
  return null;
}

function NotFound(): JSXElement {
  const { t } = useI18n();
  return (
    <div class="d-flex vh-100 align-items-center justify-content-center flex-column text-muted">
      <i class="bi bi-exclamation-circle fs-1 mb-3"></i>
      <h4>{t('not_found.title')}</h4>
      <a href="/login" class="btn btn-link mt-2">{t('not_found.go_to_login')}</a>
    </div>
  );
}

export default function App(): JSXElement {
  return (
    <AuthProvider>
      <I18nProvider>
        <LocaleSync />
        <Router>
          <Route path="/login" component={Login} />
          <Route path="/:username" component={Layout}>
            <Route path="/" component={Home} />
            <Route path="/ledgers" component={Ledgers} />
            <Route path="/reports" component={Reports} />
            <Route path="/settings" component={Settings} />
          </Route>
          <Route path="*" component={NotFound} />
        </Router>
      </I18nProvider>
    </AuthProvider>
  );
}
