/*
 * A statement PDF's text, line by line, read in this process with pdf.js:
 * nothing is written to disk and no other program sees the password (a
 * Taiwan statement is encrypted with the holder's 身分證字號).
 */
import { getDocument } from 'pdfjs-dist/legacy/build/pdf.mjs';

export class PdfPasswordError extends Error {}

/** Each line's words, left to right, pages in order; a line is one baseline. */
export async function pdfLines(data: Buffer, password = ''): Promise<string[][]> {
  const task = getDocument({
    data: new Uint8Array(data), password, verbosity: 0,
    disableFontFace: true, useSystemFonts: false, stopAtErrors: false,
  });
  let doc;
  try {
    doc = await task.promise;
  } catch (err) {
    if ((err as { name?: string }).name === 'PasswordException') throw new PdfPasswordError('the PDF did not open with this password');
    throw err;
  }
  try {
    const out: string[][] = [];
    for (let n = 1; n <= doc.numPages; n++) {
      const page = await doc.getPage(n);
      const rows = new Map<number, Array<{ x: number; s: string }>>();
      for (const it of (await page.getTextContent()).items) {
        if (!('str' in it) || !it.str.trim()) continue;
        const y = Math.round(it.transform[5]);
        const row = rows.get(y) ?? [];
        row.push({ x: it.transform[4], s: it.str.trim() });
        rows.set(y, row);
      }
      for (const [, row] of [...rows].sort((a, b) => b[0] - a[0])) out.push(row.sort((a, b) => a.x - b.x).map((w) => w.s));
    }
    return out;
  } finally {
    await task.destroy();
  }
}
