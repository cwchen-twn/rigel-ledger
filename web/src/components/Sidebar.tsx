import { A, useLocation } from '@solidjs/router';
import { type JSXElement } from 'solid-js';

interface Props {
  username: string;
}

interface NavItem {
  path: string;
  label: string;
  icon: string;
}

const NAV_ITEMS: NavItem[] = [
  { path: '/', label: 'Home', icon: 'bi-house' },
  { path: '/ledgers', label: 'Ledgers', icon: 'bi-journal-text' },
  { path: '/reports', label: 'Reports', icon: 'bi-bar-chart' },
  { path: '/settings', label: 'Settings', icon: 'bi-gear' },
];

export default function Sidebar(props: Props): JSXElement {
  const location = useLocation();

  function isActive(path: string): boolean {
    const fullPath = `/${props.username}${path === '/' ? '' : path}`;
    if (path === '/') return location.pathname === `/${props.username}` || location.pathname === `/${props.username}/`;
    return location.pathname.startsWith(fullPath);
  }

  const navContent = () => (
    <ul class="nav nav-pills flex-column mb-auto">
      {NAV_ITEMS.map(item => (
        <li class="nav-item">
          <A
            href={`/${props.username}${item.path}`}
            class={`nav-link ${isActive(item.path) ? 'active' : 'link-body-emphasis'}`}
            data-bs-dismiss="offcanvas"
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
      <a href="/logout" class="btn btn-outline-danger btn-sm w-100">
        <i class="bi bi-box-arrow-right me-1"></i>Logout
      </a>
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
          <span class="fw-bold">RigelLedger</span>
        </div>
        <hr />
        {navContent()}
        {footerContent()}
      </div>

      {/* Mobile topbar */}
      <nav class="d-md-none navbar bg-body-tertiary border-bottom px-3" style="position:sticky;top:0;z-index:100">
        <span class="navbar-brand mb-0">
          <i class="bi bi-gem me-2 text-primary"></i>RigelLedger
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
            <i class="bi bi-gem me-2 text-primary"></i>RigelLedger
          </h5>
          <button type="button" class="btn-close" data-bs-dismiss="offcanvas"></button>
        </div>
        <div class="offcanvas-body d-flex flex-column">
          {navContent()}
          {footerContent()}
        </div>
      </div>
    </>
  );
}
