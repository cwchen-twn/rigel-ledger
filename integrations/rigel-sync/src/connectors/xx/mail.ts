/*
 * Email as a source (#41; docs/ARCHITECTURE.md, "Email (Gmail) as a
 * source"): one folder of an IMAP mailbox -- in Gmail, a label a filter
 * fills -- read-only, with an app password. Email is evidence: it enriches
 * and proposes, it does not post.
 *
 *   an order or a receipt  an `invoice` row (#38): its lines join the card
 *                          payment it matches, and the PDF it attaches (or,
 *                          when it has none, the email printed to PDF) is
 *                          kept with it. One that states no amount (an
 *                          invoice notice) is evidence only: amount 0,
 *                          matched by its seller and day, or by the
 *                          電子發票 number it names (#57)
 *   a card alert (刷卡通知) a pending card row: the earliest estimate of a
 *                          charge, which the statement's posted row clears
 *
 * Parsing is local (src/mail/parse.ts). Each run reads the last 14 days
 * again (120 the first time), so a batch the app refused is not lost: the
 * rows it resends are duplicates. Only emails new since the last run are
 * printed.
 */
import { simpleParser } from 'mailparser';
import { type Mailbox, MailboxError, type MailboxLogin, openImap } from '../../mail/imap.ts';
import { type Mail, mailDay, type Parsed, type Parser, parseMail, parsers } from '../../mail/parse.ts';
import { MailPrinter } from '../../mail/pdf.ts';
import { short, taipeiDay } from '../rows.ts';
import type { Account, Batch, BatchFile, Connector, Row } from '../types.ts';
import { SyncError } from '../types.ts';

const FIRST_DAYS = 120;
const OVERLAP_DAYS = 14;
// Evidence travels inside the batch (16 MiB, base64): what does not fit
// waits for the next run, when its email is no longer new... so keep well under.
const FILE_BUDGET = 9 * 1024 * 1024;

interface MailState {
  uidValidity?: string;
  lastUid?: number;
  lastDay?: string; // the last run, YYYY-MM-DD
}

export function makeMail(opts: { open?: (l: MailboxLogin) => Promise<Mailbox>; parsers?: Parser[]; now?: () => Date } = {}): Connector {
  const open = opts.open ?? openImap;
  const now = opts.now ?? (() => new Date());
  return {
    id: 'xx-mail',
    name: 'Email (IMAP: Gmail, Outlook, ...)',
    country: 'XX',
    fields: [
      { name: 'address', label: 'Email address', kind: 'text' },
      { name: 'password', label: 'App password', kind: 'secret' },
      { name: 'folder', label: 'Label or folder (default: rigel)', kind: 'text', optional: true },
      { name: 'host', label: 'IMAP server (default: imap.gmail.com)', kind: 'text', optional: true },
    ],
    async sync(ctx) {
      const user = ctx.credentials.address?.trim();
      const pass = ctx.credentials.password?.replace(/\s/g, ''); // Google shows app passwords in groups of four
      if (!user || !pass) throw new SyncError('bad_credentials');
      const state = ctx.state as MailState;
      let box: Mailbox;
      try {
        box = await open({
          host: ctx.credentials.host?.trim() || 'imap.gmail.com',
          user,
          pass,
          folder: ctx.credentials.folder?.trim() || 'rigel',
        });
      } catch (err) {
        if (err instanceof MailboxError) {
          if (err.reason === 'auth') throw new SyncError('bad_credentials', err.message);
          if (err.reason === 'connect') throw new SyncError('institution_down', err.message);
          throw new SyncError('verification_failed', err.message);
        }
        throw err;
      }

      const today = now();
      const sameBox = state.uidValidity === box.uidValidity;
      const since = new Date(today.getTime() - (state.lastDay && sameBox ? OVERLAP_DAYS : FIRST_DAYS) * 86_400_000);
      const lastUid = sameBox ? (state.lastUid ?? 0) : 0;
      const printer = new MailPrinter(ctx.log);
      const found: Array<{ uid: number; mail: Mail; raw: { from: string; date: string }; parsed: Parsed[]; attached?: Attached }> = [];
      let read = 0;
      let maxUid = lastUid;
      try {
        for await (const m of box.since(since)) {
          read++;
          maxUid = Math.max(maxUid, m.uid);
          const p = await simpleParser(m.source, { skipImageLinks: true, skipTextToHtml: true });
          const from = p.from?.value[0];
          const dateHeader = p.headers.get('date');
          const mail: Mail = {
            messageId: p.messageId ?? `uid-${box.uidValidity}-${m.uid}`,
            from: (from?.address ?? '').toLowerCase(),
            fromName: from?.name ?? '',
            subject: p.subject ?? '',
            day: mailDay(typeof dateHeader === 'string' ? dateHeader : undefined, p.date ?? today),
            html: typeof p.html === 'string' ? p.html : undefined,
            text: p.text,
          };
          const parsed = parseMail(mail, opts.parsers ?? parsers);
          if (parsed.length) {
            found.push({
              uid: m.uid, mail, parsed, attached: evidenceOf(p.attachments),
              raw: { from: p.from?.text ?? mail.from, date: typeof dateHeader === 'string' ? dateHeader : mail.day },
            });
          }
        }

        // The new ones that become invoices keep their own PDF (a receipt, an
        // invoice), or are printed when they carry none; within the budget.
        const files: BatchFile[] = [];
        const fileOf = new Map<number, string>();
        let used = 0;
        for (const f of found) {
          if (f.uid <= lastUid || !f.parsed.some((x) => x.kind === 'order')) continue;
          const subject = f.mail.subject.replace(/[\\/:*?"<>|\r\n]+/g, ' ').trim().slice(0, 80) || 'email';
          const file = f.attached ?? { filename: `${f.mail.day} ${subject}.pdf`, content: await printer.print(f.mail, f.raw) };
          if (!file.content || used + file.content.length > FILE_BUDGET) continue;
          used += file.content.length;
          const ref = `mail-${f.uid}`;
          files.push({ ref, filename: file.filename, data: file.content.toString('base64') });
          fileOf.set(f.uid, ref);
        }
        ctx.log.info('mail read', { read, recognised: found.length, files: files.length, attached: found.filter((f) => f.attached).length });
        await ctx.saveState({ uidValidity: box.uidValidity, lastUid: maxUid, lastDay: taipeiDay(today) } satisfies MailState);
        return toBatch(found.map((f) => ({ mail: f.mail, parsed: f.parsed, file: fileOf.get(f.uid) })), files, taipeiDay(today));
      } finally {
        await printer.close();
        await box.close();
      }
    },
  };
}

export const mail = makeMail();

interface Attached {
  filename: string;
  content: Buffer | null;
}

const EVIDENCE_NAME = /receipt|invoice|bill|收據|發票|帳單|inv\b|\.inv\./i;
const isPdf = (b: Buffer) => b.subarray(0, 5).toString('latin1') === '%PDF-';
const isImage = (b: Buffer) =>
  b[0] === 0xff && b[1] === 0xd8 || b.subarray(1, 4).toString('latin1') === 'PNG' || b.subarray(8, 12).toString('latin1') === 'WEBP';

/**
 * The file an email carries that is its evidence: a PDF it attaches (the
 * one named like a receipt or an invoice first), else an attached photo.
 * Inline pictures (logos) are not attachments. The content is not read.
 */
export function evidenceOf(attachments: Array<{ filename?: string; content: Buffer; contentDisposition?: string; related?: boolean }>): Attached | undefined {
  const files = attachments.filter((a) => !a.related && a.contentDisposition !== 'inline' && a.content?.length);
  const pdfs = files.filter((a) => isPdf(a.content));
  const pick = pdfs.find((a) => EVIDENCE_NAME.test(a.filename ?? '')) ?? pdfs[0] ?? files.find((a) => isImage(a.content));
  if (!pick) return undefined;
  const name = (pick.filename ?? '').replace(/[\\/:*?"<>|\r\n]+/g, ' ').trim() || (isPdf(pick.content) ? 'evidence.pdf' : 'evidence');
  return { filename: name, content: pick.content };
}

const slug = (s: string) => s.toLowerCase().replace(/[^\p{L}\p{N}]+/gu, '-').replace(/^-|-$/g, '').slice(0, 40) || 'seller';

/**
 * What the emails said, as one batch: orders on one `orders` account (map it
 * to what pays for online orders: it books only an order nothing else
 * paid for), card alerts as pending rows on `card-<issuer>-<last4>`.
 */
export function toBatch(found: Array<{ mail: Mail; parsed: Parsed[]; file?: string }>, files: BatchFile[], day: string): Batch {
  const accounts = new Map<string, Account>();
  const rows: Row[] = [];
  for (const { mail, parsed, file } of found) {
    for (const [i, p] of parsed.entries()) {
      if (p.kind === 'order') {
        accounts.set('orders', { id: 'orders', label: 'Orders by email', currency: '' });
        // An order's own number when it has one, so a second email about it is the same row.
        const id = p.number ? `order:${slug(p.seller)}:${p.number}` : `order:${short(mail.messageId)}:${i}`;
        // An email that states no amount is evidence: amount 0, no items (#57).
        rows.push({
          kind: 'invoice', account: 'orders', id, date: p.day, amount: p.total ? `-${p.total}` : '0', currency: p.currency,
          counterparty: p.seller, description: mail.subject || undefined, items: p.total ? p.items : [],
          reference: p.reference, file: i === 0 ? file : undefined,
        });
      } else {
        const account = `card-${p.issuer}-${p.card}`;
        accounts.set(account, { id: account, label: `${p.issuer} card ***${p.card} (alerts)`, currency: p.currency });
        rows.push({
          kind: 'transaction', account, id: `alert:${short(mail.messageId)}:${i}`, date: p.day, amount: `-${p.amount}`,
          currency: p.currency, description: p.merchant, pending: true,
        });
      }
    }
  }
  const used = new Set(rows.map((r) => r.file).filter(Boolean));
  return { label: `Email ${day}`, accounts: [...accounts.values()], rows, files: files.filter((f) => used.has(f.ref)) };
}
