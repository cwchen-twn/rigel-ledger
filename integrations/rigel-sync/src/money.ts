/*
 * Amounts leave the runner as decimal strings: the app never takes a float
 * (CLAUDE.md, "Money"). all-set-tw hands amounts over as JS numbers, so this
 * is the one boundary where they are turned into text, exactly.
 */

const known = new Set(Intl.supportedValuesOf('currency'));

/** Digits after the point a currency allows (ISO 4217, from ICU). */
export function minorDigits(currency: string): number {
  if (!known.has(currency)) throw new RangeError(`unknown currency ${currency}`);
  return new Intl.NumberFormat('en', { style: 'currency', currency }).resolvedOptions().maximumFractionDigits ?? 2;
}


/**
 * The amount as a decimal string. A float that is only noise past the
 * currency's digits (0.1 + 0.2) is cleaned up; one that really has more
 * digits than the currency allows is a parsing mistake and throws, rather
 * than being rounded into the books.
 */
export function toDecimal(n: number, currency: string): string {
  if (!Number.isFinite(n)) throw new RangeError(`not an amount: ${n}`);
  const digits = minorDigits(currency);
  // 15 significant digits is what a double holds exactly; past that is noise.
  const clean = Number.parseFloat(n.toPrecision(15));
  const s = clean.toLocaleString('en-US', { useGrouping: false, maximumFractionDigits: 20 });
  const frac = s.split('.')[1] ?? '';
  if (frac.length > digits) throw new RangeError(`${s} has more digits than ${currency} allows`);
  return s === '-0' ? '0' : s;
}
