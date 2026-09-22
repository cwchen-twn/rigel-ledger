import { A, useLocation } from '@solidjs/router';
import { type JSXElement } from 'solid-js';
import { Offcanvas } from 'bootstrap';
import { useI18n } from '../i18n';

interface Props {
  username: string;
}

export default function Sidebar(props: Props): JSXElement {
  const location = useLocation();
  const { t } = useI18n();

  const navItems = () => [
    { path: '/',        label: t('nav.home'),     icon: 'bi-house' },
    { path: '/ledgers', label: t('nav.ledgers'),  icon: 'bi-journal-text' },
    { path: '/reports', label: t('nav.reports'),  icon: 'bi-bar-chart' },
    { path: '/settings',label: t('nav.settings'), icon: 'bi-gear' },
  ];

  function isActive(path: string): boolean {
    const fullPath = `/${props.username}${path === '/' ? '' : path}`;
    if (path === '/') return location.pathname === `/${props.username}` || location.pathname === `/${props.username}/`;
    return location.pathname.startsWith(fullPath);
  }

  function closeOffcanvas(): void {
    const el = document.getElementById('mobile-sidebar');
    if (el) Offcanvas.getInstance(el)?.hide();
  }

  const navContent = (withClose = false) => (
    <ul class="nav nav-pills flex-column mb-auto">
      {navItems().map(item => (
        <li class="nav-item">
          <A
            href={`/${props.username}${item.path}`}
            class={`nav-link ${isActive(item.path) ? 'active' : 'link-body-emphasis'}`}
            onClick={withClose ? closeOffcanvas : undefined}
          >
            <i class={`bi ${item.icon} me-2`}></i>
            {item.label}
          </A>
        </li>
      ))}
    </ul>
  );

  const footerContent = () => (
    <>
      <hr />
      <div class="d-flex align-items-center gap-2 mb-2">
        <i class="bi bi-person-circle fs-5"></i>
        <strong class="small">{props.username}</strong>
      </div>
      <button
        type="button"
        class="btn btn-outline-danger btn-sm w-100"
        onClick={() => { window.location.href = '/logout'; }}
      >
        <i class="bi bi-box-arrow-right me-1"></i>{t('nav.logout')}
      </button>
    </>
  );

  return (
    <>
      {/* Desktop sidebar */}
      <div
        class="d-none d-md-flex flex-column flex-shrink-0 p-3 border-end"
        style="width:240px;min-height:100vh;position:sticky;top:0;height:100vh;overflow-y:auto"
      >
        <div class="d-flex align-items-center mb-3 text-decoration-none">
          <i class="bi bi-gem me-2 fs-5 text-primary"></i>
          <span class="fw-bold">{t('app.name')}</span>
        </div>
        <hr />
        {navContent()}
        {footerContent()}
      </div>

      {/* Mobile topbar */}
      <nav class="d-md-none navbar bg-body-tertiary border-bottom px-3" style="position:sticky;top:0;z-index:100">
        <span class="navbar-brand mb-0">
          <i class="bi bi-gem me-2 text-primary"></i>{t('app.name')}
        </span>
        <button
          class="btn btn-sm btn-outline-secondary"
          type="button"
          data-bs-toggle="offcanvas"
          data-bs-target="#mobile-sidebar"
        >
          <i class="bi bi-list"></i>
        </button>
      </nav>

      {/* Mobile offcanvas */}
      <div class="offcanvas offcanvas-start d-md-none" id="mobile-sidebar" tabindex="-1">
        <div class="offcanvas-header">
          <h5 class="offcanvas-title">
            <i class="bi bi-gem me-2 text-primary"></i>{t('app.name')}
          </h5>
          <button type="button" class="btn-close" data-bs-dismiss="offcanvas"></button>
        </div>
        <div class="offcanvas-body d-flex flex-column">
          {navContent(/* withClose */ true)}
          {footerContent()}
        </div>
      </div>
    </>
  );
}
