import { createSignal, createEffect, Show, type JSXElement } from 'solid-js';
import { useNavigate } from '@solidjs/router';
import { login } from '../api/auth';
import { useAuth } from '../stores/auth';

export default function Login(): JSXElement {
  const navigate = useNavigate();
  const { auth, refetch } = useAuth();
  const [username, setUsername] = createSignal('');
  const [password, setPassword] = createSignal('');
  const [error, setError] = createSignal('');
  const [submitting, setSubmitting] = createSignal(false);

  // Redirect if already authenticated
  createEffect(() => {
    const authData = auth();
    if (!auth.loading && authData) {
      const redirect = sessionStorage.getItem('redirectAfterLogin') ?? `/${authData.username}/`;
      sessionStorage.removeItem('redirectAfterLogin');
      navigate(redirect, { replace: true });
    }
  });

  async function handleSubmit(e: SubmitEvent): Promise<void> {
    e.preventDefault();
    if (!username().trim() || !password()) return;

    setSubmitting(true);
    setError('');

    try {
      const loggedInAs = await login(username(), password());
      refetch();
      const redirect = sessionStorage.getItem('redirectAfterLogin') ?? `/${loggedInAs}/`;
      sessionStorage.removeItem('redirectAfterLogin');
      navigate(redirect, { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed');
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div class="d-flex vh-100 align-items-center justify-content-center bg-body-secondary">
      <div class="card shadow-sm" style="width:380px;max-width:95vw">
        <div class="card-body p-4">
          <div class="text-center mb-4">
            <i class="bi bi-gem fs-1 text-primary"></i>
            <h4 class="mt-2 mb-0">RigelLedger</h4>
            <p class="text-muted small mt-1">Personal Finance Management</p>
          </div>

          <form onSubmit={handleSubmit} noValidate>
            <div class="mb-3">
              <label class="form-label" for="username">Username</label>
              <input
                id="username"
                type="text"
                class="form-control"
                value={username()}
                onInput={e => setUsername(e.currentTarget.value)}
                required
                autocomplete="username"
                disabled={submitting()}
              />
            </div>
            <div class="mb-3">
              <label class="form-label" for="password">Password</label>
              <input
                id="password"
                type="password"
                class="form-control"
                value={password()}
                onInput={e => setPassword(e.currentTarget.value)}
                required
                autocomplete="current-password"
                disabled={submitting()}
              />
            </div>

            <Show when={error()}>
              <div class="alert alert-danger py-2 small mb-3" role="alert">
                {error()}
              </div>
            </Show>

            <button type="submit" class="btn btn-primary w-100" disabled={submitting()}>
              <Show when={submitting()} fallback={<>Sign In</>}>
                <span class="spinner-border spinner-border-sm me-2" role="status"></span>
                Signing in…
              </Show>
            </button>
          </form>
        </div>
      </div>
    </div>
  );
}
