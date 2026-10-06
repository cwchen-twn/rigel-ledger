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
}

/** A row of the import queue; amounts are decimal strings (see money.ts). */
export interface Row {
  kind: 'transaction' | 'balance';
  account: string; // Account.id
  id: string; // stable across runs: with the connector, the duplicate key
  date: string; // YYYY-MM-DD
  amount: string; // debit > 0 on the account: money in, a card payment
  currency?: string;
  description?: string;
  counterparty?: string;
  pending?: boolean;
}

export interface Batch {
  label: string;
  accounts: Account[];
  rows: Row[];
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
