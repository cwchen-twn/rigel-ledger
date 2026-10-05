/*
 * `tw-sync run`: the daemon. It registers its key and connectors, then
 * claims its person's due connections one at a time, opens their sealed
 * credentials, runs the connector, and reports back: rows, challenges, the
 * end of the run. Credentials and answers exist in the clear only in this
 * process's memory.
 */
import { setTimeout as sleep } from 'node:timers/promises';
import { ApiError, RigelClient, type Job } from './api.ts';
import { type Ask, type Connector, SyncError } from './connectors/types.ts';
import { logger } from './log.ts';
import { aad, open, publicKeyOf, SealError } from './sealing.ts';
import type { Store } from './store.ts';

const RUN_TIMEOUT_MS = 25 * 60_000; // inside the app's 30-minute claim
const ANSWER_POLL_MS = 2_000;

export interface RunOptions {
  client: RigelClient;
  store: Store;
  connectors: Connector[];
  pollMs: number;
  once?: boolean;
  signal: AbortSignal;
}

export async function runDaemon(o: RunOptions): Promise<void> {
  const log = logger();
  const priv = await o.store.key();
  await withRetry(o.signal, log, 'register', async () => {
    await o.client.registerKeys([publicKeyOf(priv)]);
    await o.client.publishConnectors(o.connectors.map(({ id, name, country, fields }) => ({ id, name, country, fields })));
  });
  log.info('tw-sync ready', { url: o.client.url, connectors: o.connectors.map((c) => c.id) });

  let failures = 0;
  while (!o.signal.aborted) {
    let jobs: Job[] = [];
    try {
      jobs = await o.client.claim(1, o.signal);
      failures = 0;
    } catch (err) {
      if (o.signal.aborted) break;
      if (err instanceof ApiError && err.status === 401) {
        // Unlinked, or linked again with a new token: this one is done.
        log.error('the runner token was refused; link the runner again (Settings -> Sync runner)');
        throw err;
      }
      failures++;
      log.warn('claim failed', { error: String(err), retry_in_s: backoff(failures) / 1000 });
      await sleep(backoff(failures), undefined, { signal: o.signal }).catch(() => {});
      continue;
    }
    for (const j of jobs) await runJob(o, priv, j);
    if (o.once) return;
    if (jobs.length === 0) await sleep(o.pollMs, undefined, { signal: o.signal }).catch(() => {});
  }
}

async function runJob(o: RunOptions, priv: Buffer, job: Job): Promise<void> {
  const log = logger({ connection: job.id, connector: job.connector });
  const finish = async (status: 'ok' | 'failed', code = '') => {
    try {
      await o.client.finish(job.id, status, code);
    } catch (err) {
      log.error('finish failed', { error: String(err) });
    }
  };
  const connector = o.connectors.find((c) => c.id === job.connector);
  if (!connector) return finish('failed', 'connector_unavailable');

  let credentials: Record<string, string>;
  try {
    const plain = open(priv, Buffer.from(job.sealed, 'base64'), aad('credentials', job.user_id, job.connector));
    credentials = JSON.parse(plain.toString('utf8'));
    if (typeof credentials !== 'object' || credentials === null) throw new SealError('not an object');
  } catch (err) {
    log.warn('credentials do not open', { error: err instanceof SealError ? err.message : 'not JSON' });
    return finish('failed', 'credentials_unreadable');
  }

  const stateName = `conn-${job.id}`;
  const signal = AbortSignal.any([o.signal, AbortSignal.timeout(RUN_TIMEOUT_MS)]);
  const started = Date.now();
  try {
    const batch = await connector.sync({
      credentials,
      state: await o.store.state(stateName),
      saveState: (s) => o.store.saveState(stateName, s),
      ask: (q) => askThroughApp(o.client, priv, job.id, q, signal),
      log,
      signal,
    });
    let res;
    try {
      res = await o.client.importBatch(job.id, { connector: job.connector, ...batch });
    } catch (err) {
      log.error('the app rejected the batch', { error: String(err) });
      return finish('failed', 'import_rejected');
    }
    log.info('synced', { staged: res.staged, duplicates: res.duplicates, rows: batch.rows.length, ms: Date.now() - started });
    return finish('ok');
  } catch (err) {
    if (err instanceof SyncError) {
      log.warn('sync failed', { code: err.code });
      return finish('failed', err.code);
    }
    if (o.signal.aborted) return finish('failed', 'interrupted');
    if (signal.aborted) return finish('failed', 'timeout');
    // The detail stays in this log; the app only ever gets a stable code.
    log.error('connector error', { error: err instanceof Error ? (err.stack ?? err.message) : String(err) });
    return finish('failed', 'connector_error');
  }
}

/** Raise a challenge in the app and wait for the person's sealed answer. */
async function askThroughApp(client: RigelClient, priv: Buffer, connection: number, q: Ask, signal: AbortSignal): Promise<string> {
  const ch = await client.raiseChallenge(connection, {
    kind: q.kind,
    prompt: q.prompt,
    image: q.image?.toString('base64'),
    ttl_seconds: q.ttlSeconds ?? 300,
  });
  for (;;) {
    const a = await client.challenge(connection, ch.id, signal);
    if (a.expired) throw new SyncError('challenge_expired');
    if (a.answered && a.sealed) return open(priv, Buffer.from(a.sealed, 'base64'), aad('answer', ch.id)).toString('utf8');
    await sleep(ANSWER_POLL_MS, undefined, { signal });
  }
}

const backoff = (n: number) => Math.min(5 * 60_000, 2_000 * 2 ** Math.min(n, 8));

async function withRetry(signal: AbortSignal, log: ReturnType<typeof logger>, what: string, fn: () => Promise<void>) {
  for (let n = 1; ; n++) {
    try {
      return await fn();
    } catch (err) {
      if (signal.aborted || (err instanceof ApiError && err.status < 500 && err.status !== 429)) throw err;
      log.warn(`${what} failed`, { error: String(err), retry_in_s: backoff(n) / 1000 });
      await sleep(backoff(n), undefined, { signal });
    }
  }
}
