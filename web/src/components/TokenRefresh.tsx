import { onCleanup, onMount } from 'solid-js';
import { refreshToken } from '../api/auth';
import dayjs from 'dayjs';

interface Props {
  initialLeftTime: number;
}

const CHECK_INTERVAL_SEC = 60;
const REFRESH_THRESHOLD_SEC = 2 * 60;
const ACTIVITY_TIMEOUT_SEC = 5 * 60;

export default function TokenRefresh(props: Props) {
  let tokenExpiry = dayjs().add(props.initialLeftTime, 's');
  let lastActivity = dayjs();
  let isRefreshing = false;
  let refreshInterval: ReturnType<typeof setInterval> | undefined;
  let activityInterval: ReturnType<typeof setInterval> | undefined;

  function updateActivity(): void {
    lastActivity = dayjs();
  }

  function logout(): void {
    window.location.href = '/logout';
  }

  async function doRefresh(): Promise<void> {
    if (isRefreshing) return;
    isRefreshing = true;
    try {
      const leftTime = await refreshToken();
      tokenExpiry = dayjs().add(leftTime, 's');
    } catch {
      logout();
    } finally {
      isRefreshing = false;
    }
  }

  function checkState(): void {
    if (dayjs().diff(lastActivity, 's') >= ACTIVITY_TIMEOUT_SEC) {
      logout();
      return;
    }
    if (tokenExpiry.diff(dayjs(), 's') <= REFRESH_THRESHOLD_SEC) {
      void doRefresh();
    }
  }

  onMount(() => {
    const events = ['mousedown', 'mousemove', 'keypress', 'scroll', 'touchstart', 'click'] as const;
    events.forEach(e => document.addEventListener(e, updateActivity, true));
    document.addEventListener('visibilitychange', () => {
      if (!document.hidden) updateActivity();
    });
    window.addEventListener('beforeunload', () => {
      clearInterval(refreshInterval);
      clearInterval(activityInterval);
    });
    refreshInterval = setInterval(checkState, CHECK_INTERVAL_SEC * 1000);
    activityInterval = setInterval(() => {
      if (dayjs().diff(lastActivity, 's') >= ACTIVITY_TIMEOUT_SEC) logout();
    }, ACTIVITY_TIMEOUT_SEC * 1000);
  });

  onCleanup(() => {
    clearInterval(refreshInterval);
    clearInterval(activityInterval);
  });

  return <></>;
}
