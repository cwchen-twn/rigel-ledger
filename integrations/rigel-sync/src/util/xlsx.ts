/*
 * The first worksheet of an .xlsx, as rows of cell text: a zip (stored or
 * deflated) of SpreadsheetML. Enough for an institution's export (#67,
 * ContiWeb's "Descargar extracto: XLS"), with no dependency: shared and
 * inline strings are resolved, numbers stay the text the file holds (never
 * a float), empty cells are ''.
 */
import { unzip } from './zip.ts';

const entity = (s: string) =>
  s.replace(/&(#x[0-9a-f]+|#\d+|amp|lt|gt|quot|apos);/gi, (_, e: string) => {
    if (e[0] === '#') return String.fromCodePoint(e[1] === 'x' || e[1] === 'X' ? parseInt(e.slice(2), 16) : Number(e.slice(1)));
    return ({ amp: '&', lt: '<', gt: '>', quot: '"', apos: "'" } as Record<string, string>)[e.toLowerCase()];
  });

// Tags may carry a namespace prefix (<x:row>): \w+: is optional throughout.
const texts = (xml: string) => [...xml.matchAll(/<(?:\w+:)?t(?:\s[^>]*)?>([\s\S]*?)<\/(?:\w+:)?t>/g)].map((m) => entity(m[1])).join('');

const column = (ref: string) => [...ref.replace(/\d+$/, '')].reduce((n, c) => n * 26 + c.charCodeAt(0) - 64, 0) - 1;

/** The first worksheet's rows of cell text. */
export function readXlsx(buf: Buffer): string[][] {
  const files = unzip(buf);
  const shared = [...(files.get('xl/sharedStrings.xml')?.toString('utf8') ?? '').matchAll(/<(?:\w+:)?si>([\s\S]*?)<\/(?:\w+:)?si>/g)].map((m) => texts(m[1]));
  const sheet = [...files.keys()].filter((k) => /^xl\/worksheets\/[^/]+\.xml$/.test(k)).sort()[0];
  if (!sheet) throw new Error('xlsx: no worksheet');
  const rows: string[][] = [];
  for (const r of files.get(sheet)!.toString('utf8').matchAll(/<(?:\w+:)?row\b[^>]*>([\s\S]*?)<\/(?:\w+:)?row>/g)) {
    const row: string[] = [];
    for (const c of r[1].matchAll(/<(?:\w+:)?c\b([^>]*?)(?:\/>|>([\s\S]*?)<\/(?:\w+:)?c>)/g)) {
      const attrs = c[1];
      const body = c[2] ?? '';
      const ref = /\br="([A-Z]+\d+)"/.exec(attrs)?.[1];
      const type = /\bt="(\w+)"/.exec(attrs)?.[1];
      const v = /<(?:\w+:)?v>([\s\S]*?)<\/(?:\w+:)?v>/.exec(body)?.[1];
      let text = '';
      if (type === 's') text = shared[Number(v)] ?? '';
      else if (type === 'inlineStr') text = texts(body);
      else if (v !== undefined) text = entity(v);
      const i = ref ? column(ref) : row.length;
      while (row.length < i) row.push('');
      row[i] = text;
    }
    rows.push(row);
  }
  return rows;
}
