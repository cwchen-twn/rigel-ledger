import dayjs from 'dayjs';

/** Today as YYYY-MM-DD in the browser's zone (dates are calendar days, no time). */
export function today(): string {
  return dayjs().format('YYYY-MM-DD');
}

/** Display a YYYY-MM-DD date in the user's chosen format (same tokens as the setting). */
export function formatDate(iso: string, format: string): string {
  return dayjs(iso).format(format || 'YYYY-MM-DD');
}

/** A timestamp from the API, in the browser's locale and time zone. */
export function formatDateTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString(document.documentElement.lang || undefined, { dateStyle: 'medium', timeStyle: 'short' });
}
