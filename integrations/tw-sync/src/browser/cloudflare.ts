/*
 * The stand-in for @cloudflare/puppeteer that the vendored all-set-tw
 * connectors import (scripts/vendor.ts points the import here).
 *
 * On Cloudflare a browser is remote and has a session id: a connector
 * launches one, disconnects while it waits for a one-time code, and
 * connects to it again by id when the code arrives. Here a session is a
 * local Chrome this process launched; disconnecting leaves it running, and
 * closeSession ends it whatever the connector did.
 */
import { randomUUID } from 'node:crypto';
import { existsSync } from 'node:fs';
import puppeteerCore, { type Browser, type ClickOptions, type Page } from 'puppeteer-core';

export type { Browser, CookieParam, Page } from 'puppeteer-core';

type Session = Browser & { sessionId(): string };

const live = new Map<string, { ws: string; launched: Browser }>();

const CANDIDATES = [
  '/usr/bin/chromium',
  '/usr/bin/chromium-browser',
  '/usr/bin/google-chrome',
  '/usr/bin/google-chrome-stable',
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  '/Applications/Chromium.app/Contents/MacOS/Chromium',
];

/** CHROME_PATH, or the first Chrome or Chromium installed in a usual place. */
export function chromePath(): string {
  const env = process.env.CHROME_PATH;
  if (env) return env;
  const found = CANDIDATES.find((p) => existsSync(p));
  if (!found) throw new Error('no Chrome or Chromium found; install one or set CHROME_PATH');
  return found;
}

// Puppeteer 25 dropped click's clickCount (now count), which the connectors
// use to select a field's text before typing over it.
const patched = new WeakSet<object>();
function patchClick(page: Page) {
  const proto = Object.getPrototypeOf(page) as Page;
  if (patched.has(proto)) return;
  patched.add(proto);
  const click = proto.click;
  proto.click = function (this: Page, selector: string, options?: Readonly<ClickOptions> & { clickCount?: number }) {
    if (options?.clickCount && options.count === undefined) options = { ...options, count: options.clickCount };
    return click.call(this, selector, options);
  };
}

/** Pages say Chrome, not HeadlessChrome, the way a person's browser does. */
async function prepare(browser: Browser, id: string): Promise<Session> {
  const ua = (await browser.userAgent()).replace('HeadlessChrome', 'Chrome');
  const fix = async (p: Page | null) => {
    if (!p) return;
    patchClick(p);
    await p.setUserAgent({ userAgent: ua }).catch(() => {});
  };
  for (const p of await browser.pages()) await fix(p);
  browser.on('targetcreated', (t) => void t.page().then(fix, () => {}));
  return Object.assign(browser, { sessionId: () => id });
}

async function launch(_binding: unknown, _options?: { keep_alive?: number }): Promise<Session> {
  const args = ['--lang=zh-TW', '--disable-dev-shm-usage'];
  // A container usually runs as root, where Chrome's sandbox cannot start.
  if (process.getuid?.() === 0) args.push('--no-sandbox');
  // More flags, space-separated, "quoted" when one holds a space:
  // --proxy-server=socks5://... to sign in from a Taiwan address.
  args.push(...(process.env.CHROME_ARGS?.match(/(?:[^\s"]+|"[^"]*")+/g) ?? []).map((a) => a.replaceAll('"', '')));
  const launched = await puppeteerCore.launch({
    executablePath: chromePath(),
    headless: process.env.TW_SYNC_HEADFUL !== '1',
    args,
    defaultViewport: null,
  });
  const id = randomUUID();
  live.set(id, { ws: launched.wsEndpoint(), launched });
  launched.process()?.once('exit', () => live.delete(id));
  return prepare(launched, id);
}

async function connect(_binding: unknown, sessionId: string): Promise<Session> {
  const s = live.get(sessionId);
  if (!s) throw new Error('no such browser session');
  return prepare(await puppeteerCore.connect({ browserWSEndpoint: s.ws, defaultViewport: null }), sessionId);
}

async function sessions(_binding: unknown): Promise<Array<{ sessionId: string }>> {
  return [...live.keys()].map((sessionId) => ({ sessionId }));
}

/** Cloudflare's rate limit on new browsers; a local Chrome has none. */
async function limits(_binding: unknown) {
  return { allowedBrowserAcquisitions: 1, timeUntilNextAllowedBrowserAcquisition: 0 };
}

/** End a session the connector left running; nothing if it has ended. */
export async function closeSession(sessionId: string): Promise<void> {
  const s = live.get(sessionId);
  if (!s) return;
  live.delete(sessionId);
  try {
    await (await puppeteerCore.connect({ browserWSEndpoint: s.ws })).close();
  } catch {
    s.launched.process()?.kill('SIGKILL');
  }
}

/** How many sessions are running; for tests. */
export const liveSessions = () => live.size;

export default { launch, connect, sessions, limits };
