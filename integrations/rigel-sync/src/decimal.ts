/*
 * Exact decimal arithmetic on plain decimal strings, with no imports: the
 * web app shares it with the statement parsers (src/statements), so it must
 * run in a browser too.
 */

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
