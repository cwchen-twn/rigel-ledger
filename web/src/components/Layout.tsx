import { createEffect, Show, type JSXElement } from 'solid-js';
import { useNavigate, type RouteSectionProps } from '@solidjs/router';
import { useAuth } from '../stores/auth';
import Sidebar from './Sidebar';
import TokenRefresh from './TokenRefresh';

export default function Layout(props: RouteSectionProps): JSXElement {
  const navigate = useNavigate();
  const { auth } = useAuth();

  createEffect(() => {
    if (!auth.loading && auth() === null) {
      sessionStorage.setItem('redirectAfterLogin', window.location.pathname);
      navigate('/login', { replace: true });
    }
  });

  return (
    <Show
      when={auth()}
      fallback={
        <div class="d-flex justify-content-center align-items-center vh-100">
          <div class="spinner-border text-primary" role="status">
            <span class="visually-hidden">Loading...</span>
          </div>
        </div>
      }
    >
      {(authData) => (
        <div class="d-flex flex-column flex-md-row min-vh-100">
          <TokenRefresh initialLeftTime={authData().access_token_left_time} />
          <Sidebar username={authData().username} />
          <div class="flex-grow-1 overflow-auto">
            {props.children}
          </div>
        </div>
      )}
    </Show>
  );
}
