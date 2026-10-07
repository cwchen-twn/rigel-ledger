/*
 * Shioaji's own server, run for one sync (#83). Shioaji 1.7 is a Rust core;
 * its wheel carries a standalone `shioaji` binary that serves the same API
 * over HTTP on localhost, so the runner needs no Python: the binary is
 * fetched once, started for a run, asked over HTTP, and stopped.
 *
 * The binary is Sinopac's and states no licence, so the runner image does
 * not ship it: the pinned wheel is downloaded from PyPI on first use, its
 * SHA-256 checked, and the binary kept in DATA_DIR/bin.
 *
 * The server is started with an environment of its own (never the
 * runner's), in its home directory under DATA_DIR/shioaji, where it keeps
 * its token pool: a run reuses the last login, well inside Shioaji's 1,000
 * logins a day. Home is given as "." with the directory as the working
 * directory, because the server binds a Unix socket there and a long
 * absolute path would not fit one. It listens on a free localhost port,
 * never a fixed one (8080 is the app's). Its output is dropped: it names
 * the person (ID number and name) when it logs in.
 */
import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { chmod, mkdir, rename, stat, writeFile } from 'node:fs/promises';
import { createServer } from 'node:net';
import { join } from 'node:path';
import { SyncError } from '../connectors/types.ts';
import { parseExact } from './json.ts';
import { unzip } from './zip.ts';

export const SHIOAJI_VERSION = '1.7.7';

const PYPI = 'https://files.pythonhosted.org/packages';
/** The cp37-abi3 wheels of SHIOAJI_VERSION, by Node's platform-arch. */
export const WHEELS: Record<string, { url: string; sha256: string }> = {
  'linux-x64': {
    url: `${PYPI}/07/8c/900eacfb5d248f41868837492c88e5b0bce6946510be92c680ff31931bfe/shioaji-1.7.7-cp37-abi3-manylinux_2_17_x86_64.manylinux2014_x86_64.whl`,
    sha256: '03c613b379cc4c0d767bbde8f97ef73650c991f7c0231224354415deeec48db5',
  },
  'linux-arm64': {
    url: `${PYPI}/62/2d/e9dba547821f2cdc6d926dfcad5af751afc2c769d2198593f14e6f1fd558/shioaji-1.7.7-cp37-abi3-manylinux_2_28_aarch64.whl`,
    sha256: '259bde526a659c8515d00660de19b9b4ed51aefaf29d0252814103bcf4b4fab7',
  },
  'darwin-x64': {
    url: `${PYPI}/80/d0/239d15076129c70f46202882d6720c9feb8b098c583176677c2b9e7814fe/shioaji-1.7.7-cp37-abi3-macosx_10_15_x86_64.whl`,
    sha256: '82deb66a8fcad5127eadf4851982389f2ebe16d3ac2f5f60dabd88ce569d3d6b',
  },
  'darwin-arm64': {
    url: `${PYPI}/a1/10/0b6b860ec01a142e0f2ec7b4b8c7f7e550b3f7bacbc391a6cd63d02f4db6/shioaji-1.7.7-cp37-abi3-macosx_11_0_arm64.whl`,
    sha256: 'a49e30d9e8193d0edd219270ce14165ea00c007bc25b6c9b613ff2b4325a77eb',
  },
};
const IN_WHEEL = `shioaji-${SHIOAJI_VERSION}.data/scripts/shioaji`;

const dataDir = () => process.env.DATA_DIR || '/data';

/**
 * The binary's path: SHIOAJI_BIN when set (a copy already on disk), else
 * DATA_DIR/bin/shioaji-<version>, downloaded the first time.
 */
export async function shioajiBinary(signal: AbortSignal, log: { info(m: string, f?: Record<string, unknown>): void }): Promise<string> {
  if (process.env.SHIOAJI_BIN) return process.env.SHIOAJI_BIN;
  const path = join(dataDir(), 'bin', `shioaji-${SHIOAJI_VERSION}`);
  if (await stat(path).then((s) => s.isFile(), () => false)) return path;
  const wheel = WHEELS[`${process.platform}-${process.arch}`];
  if (!wheel) throw new SyncError('unsupported_platform', `Shioaji has no binary for ${process.platform}-${process.arch}`);
  log.info('shioaji: downloading the binary', { version: SHIOAJI_VERSION });
  let res: Response;
  try {
    res = await fetch(wheel.url, { signal: AbortSignal.any([signal, AbortSignal.timeout(300_000)]) });
  } catch (err) {
    throw new SyncError('institution_down', `shioaji download: ${(err as Error).message}`);
  }
  if (!res.ok) throw new SyncError('institution_down', `shioaji download: HTTP ${res.status}`);
  const buf = Buffer.from(await res.arrayBuffer());
  const sum = createHash('sha256').update(buf).digest('hex');
  if (sum !== wheel.sha256) throw new Error(`shioaji: the wheel's SHA-256 is ${sum}, not the pinned ${wheel.sha256}`);
  const bin = unzip(buf, (name) => name === IN_WHEEL).get(IN_WHEEL);
  if (!bin) throw new Error(`shioaji: ${IN_WHEEL} is not in the wheel`);
  await mkdir(join(dataDir(), 'bin'), { recursive: true });
  const part = `${path}.part`;
  await writeFile(part, bin, { mode: 0o755 });
  await chmod(part, 0o755);
  await rename(part, path);
  return path;
}

/** A localhost port nothing listens on. */
function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const s = createServer();
    s.unref();
    s.on('error', reject);
    s.listen(0, '127.0.0.1', () => {
      const { port } = s.address() as { port: number };
      s.close(() => resolve(port));
    });
  });
}

export interface ShioajiServer {
  /** POST a JSON body to /api/v1/<path>; numbers come back as their text. */
  post(path: string, body: Record<string, unknown>): Promise<unknown>;
  get(path: string): Promise<unknown>;
  stop(): Promise<void>;
}

export interface ServerOptions {
  bin: string;
  /** Its own directory: the token pool lives there. */
  home: string;
  apiKey: string;
  secretKey: string;
  production: boolean;
  signal: AbortSignal;
  /** Seconds to wait for it to sign in and listen. */
  startSeconds?: number;
  /** Least time between two calls: Shioaji allows 25 portfolio queries in 5 s. */
  gapMs?: number;
}

/** Starts the server, signed in, and waits until it answers. */
export async function startShioaji(o: ServerOptions): Promise<ShioajiServer> {
  await mkdir(o.home, { recursive: true, mode: 0o700 });
  const port = await freePort();
  const env: Record<string, string> = {
    PATH: '/usr/bin:/bin',
    HOME: o.home,
    SJ_API_KEY: o.apiKey,
    SJ_SEC_KEY: o.secretKey,
    SJ_HOME_PATH: '.',
    SJ_HTTP_ADDR: `127.0.0.1:${port}`,
    SJ_HTTP_LOG: 'false',
    SJ_HTTP_CORS: 'false',
  };
  if (o.production) env.SJ_PRODUCTION = 'true';
  const child = spawn(o.bin, ['server', 'start', '--no-open'], { cwd: o.home, env, stdio: ['ignore', 'pipe', 'pipe'] });
  // Kept only to tell a refused key from anything else; never logged.
  let output = '';
  const keep = (b: Buffer) => void (output = (output + b.toString()).slice(-4000));
  child.stdout.on('data', keep);
  child.stderr.on('data', keep);
  let exited = false;
  const gone = new Promise<void>((resolve) => child.on('exit', () => void ((exited = true), resolve())));
  child.on('error', () => void (exited = true));

  const stop = async () => {
    if (exited) return;
    child.kill('SIGTERM');
    const timer = setTimeout(() => child.kill('SIGKILL'), 5000);
    await gone;
    clearTimeout(timer);
  };
  const onAbort = () => void stop();
  o.signal.addEventListener('abort', onAbort, { once: true });

  const base = `http://127.0.0.1:${port}/api/v1/`;
  const deadline = Date.now() + (o.startSeconds ?? 60) * 1000;
  for (;;) {
    if (exited) {
      // Shioaji says why in its own words; only whether the key was refused matters here.
      throw /Authentication failed/.test(output)
        ? new SyncError('bad_credentials')
        : new SyncError('institution_down', 'the Shioaji server stopped while starting');
    }
    if (o.signal.aborted) {
      await stop();
      throw new SyncError('cancelled');
    }
    const ok = await fetch(`${base}health`, { signal: AbortSignal.timeout(2000) }).then((r) => r.ok, () => false);
    if (ok) break;
    if (Date.now() > deadline) {
      await stop();
      throw new SyncError('institution_down', 'the Shioaji server did not start');
    }
    await new Promise((r) => setTimeout(r, 500));
  }

  let last = 0;
  const call = async (method: 'GET' | 'POST', path: string, body?: Record<string, unknown>) => {
    const wait = last + (o.gapMs ?? 250) - Date.now();
    if (wait > 0) await new Promise((r) => setTimeout(r, wait));
    last = Date.now();
    let res: Response;
    try {
      res = await fetch(base + path, {
        method,
        headers: body ? { 'Content-Type': 'application/json' } : undefined,
        body: body ? JSON.stringify(body) : undefined,
        signal: AbortSignal.any([o.signal, AbortSignal.timeout(60_000)]),
      });
    } catch (err) {
      if (o.signal.aborted) throw new SyncError('cancelled');
      throw new SyncError('institution_down', `shioaji ${path}: ${(err as Error).message}`);
    }
    const text = await res.text();
    if (res.status === 401 || res.status === 403) throw new SyncError('bad_credentials');
    if (!res.ok) throw new SyncError('institution_down', `shioaji ${path}: HTTP ${res.status}`);
    return parseExact(text);
  };
  return {
    post: (path, body) => call('POST', path, body),
    get: (path) => call('GET', path),
    stop: async () => {
      o.signal.removeEventListener('abort', onAbort);
      await stop();
    },
  };
}
