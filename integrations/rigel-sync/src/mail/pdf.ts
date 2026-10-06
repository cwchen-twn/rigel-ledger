/*
 * An email as a PDF, the evidence kept on the transaction it matches (#41).
 * Printed by the runner's own Chrome (the one the bank connectors use) with
 * script off and EVERY REQUEST REFUSED but inline data: -- an order email's
 * images are mostly tracking pixels, and loading them would tell the sender
 * when the books were synced. The email's From, Subject and Date head the
 * page, so the PDF says what it is.
 */
import shim, { closeSession } from '../browser/cloudflare.ts';
import type { Logger } from '../connectors/types.ts';
import type { Mail } from './parse.ts';

type Session = Awaited<ReturnType<typeof shim.launch>>;

const escape = (s: string) => s.replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`);

export class MailPrinter {
  private session?: Session;
  private failed = false;
  private readonly log: Pick<Logger, 'warn'>;

  constructor(log: Pick<Logger, 'warn'>) {
    this.log = log;
  }

  /** The PDF, or undefined when Chrome is missing or the email would not print (logged once). */
  async print(mail: Mail, raw: { from: string; date: string }): Promise<Buffer | undefined> {
    if (this.failed) return undefined;
    try {
      this.session ??= await shim.launch(null);
      const page = await this.session.newPage();
      try {
        await page.setJavaScriptEnabled(false);
        await page.setRequestInterception(true);
        page.on('request', (r) => {
          const url = r.url();
          if (url.startsWith('data:') || url === 'about:blank') void r.continue();
          else void r.abort('blockedbyclient');
        });
        const head = `<div style="font:12px/1.5 sans-serif;border-bottom:1px solid #ccc;padding:0 0 8px;margin:0 0 12px">
          <div><b>From:</b> ${escape(raw.from)}</div><div><b>Subject:</b> ${escape(mail.subject)}</div><div><b>Date:</b> ${escape(raw.date)}</div></div>`;
        const body = mail.html ?? `<pre style="white-space:pre-wrap;font:13px/1.5 sans-serif">${escape(mail.text ?? '')}</pre>`;
        await page.setContent(`<!doctype html><meta charset="utf-8">${head}${body}`, { waitUntil: 'load', timeout: 15_000 });
        return Buffer.from(await page.pdf({ format: 'A4', printBackground: true, margin: { top: '12mm', bottom: '12mm', left: '10mm', right: '10mm' } }));
      } finally {
        await page.close().catch(() => {});
      }
    } catch (err) {
      this.failed = true;
      this.log.warn('emails are kept without a PDF: Chrome could not print them', { error: err instanceof Error ? err.message : String(err) });
      return undefined;
    }
  }

  async close() {
    if (this.session) await closeSession(this.session.sessionId());
  }
}
