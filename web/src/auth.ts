import dayjs from 'dayjs';

export function initAutoLogout(accessTokenLeftTime: number): void {
  const CONFIG = {
    ACCESS_TOKEN_UNTIL: dayjs().add(accessTokenLeftTime, 's'),
    CHECK_INTERVAL_SEC: 60,
    REFRESH_THRESHOLD: 2 * 60,
    ACTIVITY_TIMEOUT_SEC: 5 * 60,
  };

  let lastTokenRefresh = dayjs();
  let lastActivity = dayjs();
  let refreshInterval: ReturnType<typeof setInterval> | null = null;
  let activityTimeout: ReturnType<typeof setInterval> | null = null;
  let isRefreshing = false;

  function updateActivity(): void {
    lastActivity = dayjs();
  }

  function setupActivityTracking(): void {
    const events = ['mousedown', 'mousemove', 'keypress', 'scroll', 'touchstart', 'click'] as const;
    events.forEach(event => document.addEventListener(event, updateActivity, true));
  }

  function isAccessTokenExpiringSoon(): boolean {
    const secondsSinceLastRefresh = dayjs().diff(lastTokenRefresh, 's');
    const until = CONFIG.ACCESS_TOKEN_UNTIL.subtract(CONFIG.REFRESH_THRESHOLD, 's');
    const threshold = until.diff(dayjs(), 's');
    return secondsSinceLastRefresh >= threshold;
  }

  function isUserInactive(): boolean {
    return dayjs().diff(lastActivity, 's') >= CONFIG.ACTIVITY_TIMEOUT_SEC;
  }

  function logout(): void {
    if (refreshInterval) clearInterval(refreshInterval);
    if (activityTimeout) clearInterval(activityTimeout);
    window.location.href = '/logout';
  }

  async function refreshAccessToken(): Promise<boolean> {
    if (isRefreshing) return false;
    isRefreshing = true;
    try {
      const response = await fetch('/refresh-token', { method: 'POST' });
      if (response.ok) {
        const data = await response.json() as { access_token_left_time: number };
        CONFIG.ACCESS_TOKEN_UNTIL = dayjs().add(data.access_token_left_time, 's');
        lastTokenRefresh = dayjs();
        return true;
      }
      if (response.status === 401) {
        logout();
        return false;
      }
    } catch {
      // network error, will retry next interval
    } finally {
      isRefreshing = false;
    }
    return false;
  }

  function checkTokenAndActivity(): void {
    if (isUserInactive()) { logout(); return; }
    if (isAccessTokenExpiringSoon()) { void refreshAccessToken(); }
  }

  setupActivityTracking();
  refreshInterval = setInterval(checkTokenAndActivity, CONFIG.CHECK_INTERVAL_SEC * 1000);
  activityTimeout = setInterval(() => { if (isUserInactive()) logout(); }, CONFIG.ACTIVITY_TIMEOUT_SEC * 1000);

  document.addEventListener('visibilitychange', () => {
    if (!document.hidden) updateActivity();
  });

  window.addEventListener('beforeunload', () => {
    if (refreshInterval) clearInterval(refreshInterval);
    if (activityTimeout) clearInterval(activityTimeout);
  });
}
