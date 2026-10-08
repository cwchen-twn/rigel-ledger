/*
 * The first worksheet of an .xlsx, as rows of cell text: a zip (stored or
 * deflated) of SpreadsheetML. Enough for an institution's export (#67,
 * ContiWeb's "Descargar extracto: XLS"), with no dependency: shared and
 * inline strings are resolved, numbers stay the text the file holds (never
 * a float), empty cells are ''.
 */
import { sheetRows } from './sheet.ts';
import { unzip } from './zip.ts';

/** The first worksheet's rows of cell text. */
export function readXlsx(buf: Buffer): string[][] {
  const files = new Map<string, string>();
  for (const [name, data] of unzip(buf, (n) => n.endsWith('.xml'))) files.set(name, data.toString('utf8'));
  return sheetRows(files);
}
