/**
 * A page's text items as lines: each line's words left to right, lines top
 * to bottom. Shared by the runner (pdf.ts) and the web app's PDF import.
 */
export function toLines(items: ReadonlyArray<unknown>): string[][] {
  const rows = new Map<number, Array<{ x: number; s: string }>>();
  for (const it of items as Array<{ str?: string; transform?: number[] }>) {
    if (typeof it.str !== 'string' || !it.str.trim() || !it.transform) continue;
    const y = Math.round(it.transform[5]);
    const row = rows.get(y) ?? [];
    row.push({ x: it.transform[4], s: it.str.trim() });
    rows.set(y, row);
  }
  return [...rows].sort((a, b) => b[0] - a[0]).map(([, row]) => row.sort((a, b) => a.x - b.x).map((w) => w.s));
}
