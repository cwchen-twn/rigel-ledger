/*
 * What a connector is to rigel-sync. A connector signs in to one institution
 * and returns one batch for the import queue; everything about the app
 * (claims, sealing, challenges over HTTP) stays in the runner, so the same
 * connector runs under `rigel-sync run` and `rigel-sync try`.
 */

/** One input on the connect form; the app renders the form from these. */
export interface Field {
  name: string;
  label: string; // English; the app prefers connector.<id>.field.<name> when translated
  kind: 'text' | 'secret' | 'id_number' | 'choice';
  /** A choice's values, the first the default; the app labels them connector.<id>.option.<name>.<value>. */
  options?: string[];
  optional?: boolean;
}

export interface Account {
  id: string; // the institution's own id (masked account or card number), stable across runs
  label: string;
  currency: string;
  /** brokerage: holds securities (holding and trade rows); cash is the default. */
  kind?: 'cash' | 'brokerage';
}

/** A row of the import queue; amounts are decimal strings (see money.ts). */
export interface Row {
  kind: 'transaction' | 'balance' | 'holding' | 'trade' | 'invoice';
  account: string; // Account.id
  id: string; // stable across runs: with the connector, the duplicate key
  date: string; // YYYY-MM-DD
  /** debit > 0 on the account: money in, a card payment; omitted for holdings and trades. */
  amount?: string;
  currency?: string;
  description?: string;
  counterparty?: string;
  pending?: boolean;

  // holding and trade rows (the app's import core, #37)
  security?: string; // NAMESPACE:SYMBOL, e.g. XTAI:2330
  security_name?: string;
  quote_currency?: string;
  units?: string; // held (holding), or moved: > 0 in, < 0 out (trade)
  price?: string; // per unit, in the quote currency, when known
  cash?: string; // what a trade settled for, signed on the settlement account, when known

  // invoice rows (#38): amount is the total, signed on the account that paid
  // (a purchase < 0); counterparty the seller; items its lines.
  items?: InvoiceItem[];
  /** BatchFile.ref: the row's evidence, attached to its transaction when accepted (#36). */
  file?: string;
  /** invoice rows: the number of an invoice another source also sends (a 電子發票 an email names). */
  reference?: string;
}

/** Evidence a batch carries: an image or a PDF, up to 10 MiB, inside the batch's 16 MiB. */
export interface BatchFile {
  ref: string;
  filename: string;
  data: string; // base64
}

/** A line of an invoice: what it cost (> 0; a discount < 0). */
export interface InvoiceItem {
  description: string;
  quantity?: string;
  unit_price?: string;
  amount: string;
}

export interface Batch {
  label: string;
  accounts: Account[];
  rows: Row[];
  files?: BatchFile[];
}

export type ChallengeKind = 'otp' | 'captcha' | 'device';

export interface Ask {
  kind: ChallengeKind;
  prompt: string;
  image?: Buffer; // a CAPTCHA
  ttlSeconds?: number;
}

/** Session state a connector keeps between runs: cookies, a trusted device id. */
export type State = Record<string, unknown>;

export interface Logger {
  info(msg: string, fields?: Record<string, unknown>): void;
  warn(msg: string, fields?: Record<string, unknown>): void;
}

export interface SyncContext {
  credentials: Record<string, string>;
  state: State;
  /** Keep state for the next run, as soon as it is known (a new trusted device). */
  saveState(state: State): Promise<void>;
  /** Ask the person (OTP, CAPTCHA, device check) and wait for the answer. */
  ask(q: Ask): Promise<string>;
  log: Logger;
  signal: AbortSignal;
}

export interface Connector {
  id: string;
  name: string;
  country: string;
  fields: Field[];
  sync(ctx: SyncContext): Promise<Batch>;
}

/**
 * A failure the app shows by its stable code (bad_credentials,
 * challenge_expired, institution_down, ...), never the institution's text.
 */
export class SyncError extends Error {
  readonly code: string;
  constructor(code: string, detail?: string) {
    super(detail ? `${code}: ${detail}` : code);
    this.code = code;
  }
}
