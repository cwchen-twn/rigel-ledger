/*
 * One mail folder over IMAP, read-only: the folder is opened with EXAMINE,
 * so nothing is marked read, moved or deleted. In Gmail a label is a
 * folder: a filter puts the mails worth reading under it ("rigel").
 */
import { ImapFlow } from 'imapflow';

export interface Fetched {
  uid: number;
  source: Buffer;
}

export interface Mailbox {
  /** Changes when the folder is recreated: uids from before mean nothing. */
  uidValidity: string;
  /** The folder's messages received on or after a day, oldest first. */
  since(day: Date): AsyncIterable<Fetched>;
  close(): Promise<void>;
}

export interface MailboxLogin {
  host: string;
  user: string;
  pass: string;
  folder: string;
}

export class MailboxError extends Error {
  readonly reason: 'auth' | 'folder' | 'connect';
  constructor(reason: 'auth' | 'folder' | 'connect', message: string) {
    super(message);
    this.reason = reason;
  }
}

export async function openImap(login: MailboxLogin): Promise<Mailbox> {
  const client = new ImapFlow({
    host: login.host,
    port: 993,
    secure: true,
    auth: { user: login.user, pass: login.pass },
    logger: false,
    disableAutoIdle: true,
  });
  try {
    await client.connect();
  } catch (err) {
    const e = err as { authenticationFailed?: boolean; message?: string };
    throw new MailboxError(e.authenticationFailed ? 'auth' : 'connect', e.message ?? String(err));
  }
  let box;
  try {
    box = await client.mailboxOpen(login.folder, { readOnly: true });
  } catch (err) {
    await client.logout().catch(() => {});
    throw new MailboxError('folder', `no folder or label "${login.folder}": ${err instanceof Error ? err.message : String(err)}`);
  }
  return {
    uidValidity: String(box.uidValidity),
    async *since(day: Date) {
      const uids = (await client.search({ since: day }, { uid: true })) || [];
      if (!uids.length) return;
      for await (const m of client.fetch(uids, { uid: true, source: true }, { uid: true })) {
        if (m.source) yield { uid: m.uid, source: m.source };
      }
    },
    close: async () => {
      await client.logout().catch(() => {});
    },
  };
}
