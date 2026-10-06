/* Helpers the bank connectors share when turning upstream's rows into a Batch. */
import { createHash } from 'node:crypto';

/** Today's date in Taiwan, YYYY-MM-DD. */
export function taipeiDay(d: Date): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Taipei' }).format(d);
}

export const last4 = (s: string) => s.replace(/\D/g, '').slice(-4);

/** A short, stable hash for row ids. */
export const short = (...parts: string[]) => createHash('sha256').update(parts.join('\0')).digest('hex').slice(0, 12);

/** A row's day: when it happened if upstream knows, else when it posted. */
export function dayOf(who: string, t: { authorizedAt?: string; postedDate?: string }): string {
  const d = (t.authorizedAt ?? t.postedDate ?? '').slice(0, 10);
  if (!/^\d{4}-\d{2}-\d{2}$/.test(d)) throw new Error(`${who}: a transaction without a date (${t.authorizedAt ?? t.postedDate})`);
  return d;
}
