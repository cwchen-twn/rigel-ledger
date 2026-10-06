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

/** A plain decimal from an institution's "1,234.5000"; undefined when it is not a number. */
export function plainDecimal(value: unknown): string | undefined {
  const s = String(value ?? '').replace(/,/g, '').trim();
  if (!/^-?\d+(\.\d+)?$/.test(s)) return undefined;
  const [int, frac = ''] = s.replace(/^(-?)0+(?=\d)/, '$1').split('.');
  const f = frac.replace(/0+$/, '');
  return f ? `${int}.${f}` : int;
}

// Exact decimal arithmetic on plain decimal strings, for amounts a source
// gives in parts (unit price x quantity, a sum of lines).
const scaleOf = (s: string) => (s.split('.')[1] ?? '').length;
const toUnits = (s: string, scale: number) => {
  const [int, frac = ''] = s.replace('-', '').split('.');
  const v = BigInt(int + frac.padEnd(scale, '0'));
  return s.startsWith('-') ? -v : v;
};
const fromUnits = (v: bigint, scale: number) => {
  const neg = v < 0n;
  const digits = (neg ? -v : v).toString().padStart(scale + 1, '0');
  const s = scale ? `${digits.slice(0, -scale)}.${digits.slice(-scale)}` : digits;
  return plainDecimal(`${neg ? '-' : ''}${s}`)!;
};

/** a + b, exactly. */
export function addDecimal(a: string, b: string): string {
  const scale = Math.max(scaleOf(a), scaleOf(b));
  return fromUnits(toUnits(a, scale) + toUnits(b, scale), scale);
}

/** a x b, exactly. */
export function mulDecimal(a: string, b: string): string {
  return fromUnits(toUnits(a, scaleOf(a)) * toUnits(b, scaleOf(b)), scaleOf(a) + scaleOf(b));
}
