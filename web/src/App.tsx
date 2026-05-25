import { Router, Route } from '@solidjs/router';
import { type JSXElement } from 'solid-js';
import { AuthProvider } from './stores/auth';
import Layout from './components/Layout';
import Login from './pages/Login';
import Home from './pages/Home';
import Ledgers from './pages/Ledgers';
import Reports from './pages/Reports';
import Settings from './pages/Settings';

function NotFound(): JSXElement {
  return (
    <div class="d-flex vh-100 align-items-center justify-content-center flex-column text-muted">
      <i class="bi bi-exclamation-circle fs-1 mb-3"></i>
      <h4>Page not found</h4>
      <a href="/login" class="btn btn-link mt-2">Go to login</a>
    </div>
  );
}

export default function App(): JSXElement {
  return (
    <AuthProvider>
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
    </AuthProvider>
  );
}
