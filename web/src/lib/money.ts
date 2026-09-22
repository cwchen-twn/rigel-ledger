import bigDecimal from 'js-big-decimal';

/*
 * Money is a decimal string everywhere in the app -- it arrives from the API
 * as "1234.50" and goes back the same way. Arithmetic goes through
 * js-big-decimal; a JS number never holds an amount.
 */

const DECIMAL = /^-?\d+(\.\d+)?$/;

/** Normalise user input ("1,234.5", " 12 ") to a decimal string, or null if it is not a number. */
export function parseAmount(input: string): string | null {
  const s = input.replace(/[,\s_]/g, '');
  if (s === '') return null;
  return DECIMAL.test(s) ? bigDecimal.stripTrailingZero(s) : null;
}

export const add = (a: string, b: string): string => bigDecimal.add(a || '0', b || '0');
export const sub = (a: string, b: string): string => bigDecimal.subtract(a || '0', b || '0');
export const mul = (a: string, b: string): string => bigDecimal.multiply(a || '0', b || '0');
// js-big-decimal negates "0" to "-0", which Intl then prints as "-NT$0.00".
export const neg = (a: string): string => (bigDecimal.compareTo(a || '0', '0') === 0 ? '0' : bigDecimal.negate(a));
export const abs = (a: string): string => bigDecimal.abs(a || '0');
export const cmp = (a: string, b: string): number => bigDecimal.compareTo(a || '0', b || '0');
export const isZero = (a: string): boolean => cmp(a, '0') === 0;
export const sum = (xs: string[]): string => xs.reduce((acc, x) => add(acc, x), '0');

/** Round half away from zero, matching shopspring/decimal's Round on the server. */
export function round(a: string, places: number): string {
  return bigDecimal.round(a || '0', places, bigDecimal.RoundingModes.HALF_UP);
}

/** True when a has no more than `places` decimals. */
export function fitsPrecision(a: string, places: number): boolean {
  return cmp(round(a, places), a) === 0;
}

/** The smallest unit of a currency with `places` decimals, e.g. "0.01". */
export function minorUnit(places: number): string {
  return places === 0 ? '1' : '0.' + '0'.repeat(places - 1) + '1';
}

/**
 * Format for display. Intl.NumberFormat accepts a decimal string and keeps its
 * precision (ES2023), so no float conversion happens here either.
 */
export function formatMoney(amount: string, currency: string, locale: string, decimals?: number): string {
  try {
    const f = new Intl.NumberFormat(locale, {
      style: 'currency',
      currency,
      minimumFractionDigits: decimals,
      maximumFractionDigits: decimals,
    });
    return f.format(amount as unknown as number);
  } catch {
    return `${amount} ${currency}`;
  }
}

/** A plain number with grouping, no currency symbol (for dense tables). */
export function formatNumber(amount: string, locale: string, decimals: number): string {
  return new Intl.NumberFormat(locale, {
    minimumFractionDigits: decimals,
    maximumFractionDigits: decimals,
  }).format(amount as unknown as number);
}
