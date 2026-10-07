/*
 * Statement PDFs read in the tab (#42), the way CSVs are: the file and its
 * password (often the holder's 身分證字號) never leave the browser; only the
 * rows and the original file are sent. The parsers are the sync runner's
 * own (@sync/statements), so a PDF uploaded here and one read from email
 * give the same rows.
 *
 * pdf.js (Apache-2.0) is 1.7 MB, so it is not in the bundle: its two
 * prebuilt files (content-hashed by Vite) are added as module scripts the
 * first time a PDF is chosen. The worker runs in this thread, which the CSP
 * allows without a worker-src.
 */
import { readStatementLines, StatementUnbalanced } from '@sync/statements/index.ts';
import { toLines } from '@sync/statements/lines.ts';
import type { Account, Row } from '@sync/connectors/types.ts';
import pdfUrl from 'pdfjs-dist/build/pdf.min.mjs?url';
import workerUrl from 'pdfjs-dist/build/pdf.worker.min.mjs?url';

export interface PdfPage {
  getTextContent(): Promise<{ items: unknown[] }>;
  getViewport(p: { scale: number }): { width: number; height: number };
  render(p: { canvas: HTMLCanvasElement; viewport: { width: number; height: number } }): { promise: Promise<void> };
}

interface PdfJs {
  getDocument(p: { data: Uint8Array; password?: string; verbosity?: number; isEvalSupported?: boolean }): {
    promise: Promise<{ numPages: number; getPage(n: number): Promise<PdfPage> }>;
    destroy(): Promise<void>;
  };
}

let loading: Promise<PdfJs> | undefined;

export function script(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const s = document.createElement('script');
    s.type = 'module';
    s.src = src;
    s.onload = () => resolve();
    s.onerror = () => reject(new Error(`could not load ${src}`));
    document.head.append(s);
  });
}

function pdfjs(): Promise<PdfJs> {
  loading ??= (async () => {
    const g = globalThis as { pdfjsLib?: PdfJs; pdfjsWorker?: unknown };
    await script(workerUrl); // sets pdfjsWorker: pdf.js then needs no Worker
    await script(pdfUrl);
    if (!g.pdfjsLib || !g.pdfjsWorker) throw new Error('pdf.js did not load');
    return g.pdfjsLib;
  })();
  return loading;
}

export type StatementError = 'password' | 'unrecognised' | 'unbalanced';

export interface Statement {
  name: string; // what it is, for the preview ("國泰期貨 月對帳單 2026-09")
  account: Account;
  rows: Row[];
}

/** A PDF's lines; throws 'password' when it needs one, or another. */
export function pdfLines(data: ArrayBuffer, password = ''): Promise<string[][]> {
  return withPdf(data, password, async (doc) => {
    const out: string[][] = [];
    for (let n = 1; n <= doc.numPages; n++) out.push(...toLines((await (await doc.getPage(n)).getTextContent()).items));
    return out;
  });
}

/** fn on the open PDF; throws 'password' when it needs one. */
export async function withPdf<T>(data: ArrayBuffer, password: string, fn: (doc: { numPages: number; getPage(n: number): Promise<PdfPage> }) => Promise<T>): Promise<T> {
  const lib = await pdfjs();
  const task = lib.getDocument({ data: new Uint8Array(data.slice(0)), password, verbosity: 0, isEvalSupported: false });
  try {
    return await fn(await task.promise);
  } catch (err) {
    if ((err as { name?: string }).name === 'PasswordException') throw new Error('password' satisfies StatementError);
    throw err;
  } finally {
    await task.destroy();
  }
}

/** The statement a PDF is, by the parsers known; the file's ref goes on its first row. */
export async function readStatement(data: ArrayBuffer, password: string, file: string): Promise<Statement> {
  const lines = await pdfLines(data, password);
  let st;
  try {
    st = readStatementLines(lines, file);
  } catch (err) {
    if (err instanceof StatementUnbalanced) throw new Error('unbalanced' satisfies StatementError);
    throw err;
  }
  if (!st) throw new Error('unrecognised' satisfies StatementError);
  return st;
}
