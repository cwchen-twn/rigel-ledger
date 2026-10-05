import assert from 'node:assert/strict';
import { createServer, type IncomingMessage, type ServerResponse } from 'node:http';
import { mkdtemp, rm, stat } from 'node:fs/promises';
import type { AddressInfo } from 'node:net';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { after, before, test } from 'node:test';
import { RigelClient } from '../src/api.ts';
import { fake } from '../src/connectors/fake.ts';
import { runDaemon } from '../src/runner.ts';
import { aad, publicKeyOf, seal } from '../src/sealing.ts';
import { Store } from '../src/store.ts';

/** Just enough of the app's runner API to drive one runner through its jobs. */
class FakeApp {
  key?: Buffer;
  connectors: unknown[] = [];
  jobs: { id: number; user_id: number; book_id: number; connector: string; key_id: number; sealed: string }[] = [];
  imports = new Map<number, any>();
  finished = new Map<number, { status: string; error: string }>();
  challenges: { id: number; connection: number; kind: string; answer?: string }[] = [];
  answer = '123456';
  auth: string[] = [];

  async handle(req: IncomingMessage, res: ServerResponse) {
    this.auth.push(req.headers.authorization ?? '');
    let body = '';
    for await (const c of req) body += c;
    const json = body ? JSON.parse(body) : undefined;
    const send = (status: number, v?: unknown) => {
      res.writeHead(status, { 'Content-Type': 'application/json' });
      res.end(v === undefined ? '' : JSON.stringify(v));
    };
    const url = req.url ?? '';
    let m: RegExpMatchArray | null;
    if (url === '/api/runner/keys') {
      this.key = Buffer.from(json.public_keys[0], 'base64');
      return send(200, []);
    }
    if (url === '/api/runner/connectors') {
      this.connectors = json.connectors;
      return send(204);
    }
    if (url === '/api/runner/jobs/claim') return send(200, this.jobs.splice(0, json.limit));
    if ((m = url.match(/^\/api\/runner\/connections\/(\d+)\/imports$/))) {
      this.imports.set(Number(m[1]), json);
      return send(201, { staged: json.rows.length, duplicates: 0 });
    }
    if ((m = url.match(/^\/api\/runner\/connections\/(\d+)\/challenges$/))) {
      const ch = { id: this.challenges.length + 100, connection: Number(m[1]), kind: json.kind };
      this.challenges.push(ch);
      return send(201, { id: ch.id, expires_at: new Date(Date.now() + 300_000).toISOString() });
    }
    if ((m = url.match(/^\/api\/runner\/connections\/(\d+)\/challenges\/(\d+)$/))) {
      // The person answers in the browser, which seals to the runner's key.
      const id = Number(m[2]);
      return send(200, { answered: true, expired: false, sealed: seal(this.key!, Buffer.from(this.answer), aad('answer', id)).toString('base64') });
    }
    if ((m = url.match(/^\/api\/runner\/connections\/(\d+)\/finish$/))) {
      this.finished.set(Number(m[1]), json);
      return send(204);
    }
    send(404, { error: { code: 'not_found', message: url } });
  }

  job(id: number, connector: string, creds: unknown, userID = 7) {
    const sealed = seal(this.key!, Buffer.from(JSON.stringify(creds)), aad('credentials', userID, connector)).toString('base64');
    this.jobs.push({ id, user_id: userID, book_id: 1, connector, key_id: 1, sealed });
  }
}

let app: FakeApp;
let url: string;
let dir: string;
const server = createServer((req, res) => void app.handle(req, res));

before(async () => {
  await new Promise<void>((r) => server.listen(0, '127.0.0.1', r));
  url = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
  dir = await mkdtemp(join(tmpdir(), 'tw-sync-'));
});
after(async () => {
  server.close();
  await rm(dir, { recursive: true, force: true });
});

const run = (store: Store) =>
  runDaemon({ client: new RigelClient(url, 'tok'), store, connectors: [fake], pollMs: 10, once: true, signal: new AbortController().signal });

test('a run: key and connectors registered, an OTP answered, a batch sent, a trusted device remembered', async () => {
  app = new FakeApp();
  const store = new Store(dir);
  await run(store); // registers; nothing due yet
  const priv = await store.key();
  assert.deepEqual(app.key, publicKeyOf(priv));
  assert.equal((await stat(join(dir, 'runner.key'))).mode & 0o777, 0o600);
  assert.deepEqual((app.connectors as { id: string }[]).map((c) => c.id), ['fake']);
  assert.ok(app.auth.every((a) => a === 'Bearer tok'));

  app.job(1, 'fake', { username: 'alice', password: 'otp' });
  await run(store);
  assert.deepEqual(app.finished.get(1), { status: 'ok', error: '' });
  assert.equal(app.challenges.length, 1);
  const batch = app.imports.get(1);
  assert.equal(batch.connector, 'fake');
  assert.equal(batch.rows.length, 4);
  assert.ok(batch.rows.every((r: { amount: unknown }) => typeof r.amount === 'string'));
  assert.deepEqual(await store.state('conn-1'), { trusted: true });

  // Trusted now: the next run asks for nothing.
  app.job(1, 'fake', { username: 'alice', password: 'otp' });
  await run(store);
  assert.equal(app.challenges.length, 1);
  assert.deepEqual(app.finished.get(1), { status: 'ok', error: '' });
});

test('failures reach the app as stable codes', async () => {
  app = new FakeApp();
  const store = new Store(dir);
  await run(store);
  app.job(2, 'fake', { username: 'alice', password: 'wrong' });
  app.job(3, 'nope', { username: 'x' });
  // Sealed for another user (or copied onto another row): it does not open.
  app.job(4, 'fake', { username: 'alice', password: 'x' });
  app.jobs[2]!.user_id = 8;
  app.job(5, 'fake', { username: 'bob', password: 'otp' });
  app.answer = '000000';
  for (let i = 0; i < 4; i++) await run(store);
  assert.deepEqual(app.finished.get(2), { status: 'failed', error: 'bad_credentials' });
  assert.deepEqual(app.finished.get(3), { status: 'failed', error: 'connector_unavailable' });
  assert.deepEqual(app.finished.get(4), { status: 'failed', error: 'credentials_unreadable' });
  assert.deepEqual(app.finished.get(5), { status: 'failed', error: 'bad_otp' });
  assert.equal(app.imports.size, 0);
});
