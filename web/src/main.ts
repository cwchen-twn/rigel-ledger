import Alpine from 'alpinejs';
import 'bootstrap';
import 'bootstrap-icons/font/bootstrap-icons.css';
import 'bootstrap/dist/css/bootstrap.min.css';
import 'datatables.net-bs5/css/dataTables.bootstrap5.min.css';
import 'datatables.net-responsive-bs5/css/responsive.bootstrap5.min.css';
import 'tom-select/dist/css/tom-select.bootstrap5.min.css';

import { initAutoLogout } from './auth';
import './pages/login';
import { initLedgersPage } from './pages/ledgers';

declare global {
  interface Window {
    Alpine: typeof Alpine;
  }
}

window.Alpine = Alpine;

const config = window.__APP_CONFIG__;
const page = config?.page ?? '';

if (page === 'home') {
  void import('./pages/home').then(m => m.initHomePage(config.username));
}

if (page === 'ledgers') {
  initLedgersPage(config.username);
}

if (config?.accessTokenLeftTime !== undefined) {
  initAutoLogout(config.accessTokenLeftTime);
}

Alpine.start();
