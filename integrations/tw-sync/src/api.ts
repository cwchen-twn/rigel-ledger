/*
 * The app's runner API (/api/runner/*, internal/routes/handlers_runner.go),
 * signed in with the person's runner token. Byte fields travel as base64,
 * as Go's encoding/json writes []byte.
 */
import type { Account, Field, Row } from './connectors/types.ts';

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  constructor(status: number, code: string, message: string) {
    super(`${status} ${code} ${message}`.trim());
    this.status = status;
    this.code = code;
  }
}

export interface Job {
  id: number;
  user_id: number;
  book_id: number;
  connector: string;
  key_id: number;
  sealed: string; // base64
}

export interface ChallengeState {
  answered: boolean;
  expired: boolean;
  sealed?: string; // base64; handed out once
}

export class RigelClient {
  readonly url: string;
  private readonly token: string;
  constructor(url: string, token: string) {
    this.url = url.replace(/\/+$/, '');
    this.token = token;
  }

  private async call<T>(method: string, path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
    const res = await fetch(this.url + path, {
      method,
      headers: { Authorization: `Bearer ${this.token}`, 'Content-Type': 'application/json', 'User-Agent': 'rigel-tw-sync' },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: signal ?? AbortSignal.timeout(60_000),
    });
    const text = await res.text();
    if (!res.ok) {
      let code = '';
      let message = '';
      try {
        ({ code, message } = JSON.parse(text).error ?? {});
      } catch {
        message = text.slice(0, 200);
      }
      throw new ApiError(res.status, code ?? '', message ?? '');
    }
    return (text ? JSON.parse(text) : undefined) as T;
  }

  registerKeys(publicKeys: Buffer[]) {
    return this.call<unknown>('POST', '/api/runner/keys', { public_keys: publicKeys.map((k) => k.toString('base64')) });
  }

  publishConnectors(cs: { id: string; name: string; country: string; fields: Field[] }[]) {
    return this.call<void>('PUT', '/api/runner/connectors', { connectors: cs });
  }

  claim(limit: number, signal?: AbortSignal) {
    return this.call<Job[]>('POST', '/api/runner/jobs/claim', { limit }, signal);
  }

  importBatch(connection: number, batch: { connector: string; label: string; accounts: Account[]; rows: Row[] }) {
    return this.call<{ staged: number; duplicates: number }>('POST', `/api/runner/connections/${connection}/imports`, batch);
  }

  raiseChallenge(connection: number, c: { kind: string; prompt: string; image?: string; ttl_seconds: number }) {
    return this.call<{ id: number; expires_at: string }>('POST', `/api/runner/connections/${connection}/challenges`, c);
  }

  challenge(connection: number, challenge: number, signal?: AbortSignal) {
    return this.call<ChallengeState>('GET', `/api/runner/connections/${connection}/challenges/${challenge}`, undefined, signal);
  }

  finish(connection: number, status: 'ok' | 'failed', error = '') {
    return this.call<void>('POST', `/api/runner/connections/${connection}/finish`, { status, error });
  }
}
