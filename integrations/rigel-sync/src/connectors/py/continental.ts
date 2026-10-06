/*
 * Banco Continental (Paraguay), ContiWeb (#67): step one, a capture run.
 *
 * Nothing is known yet of the JSON ContiWeb's API answers, nor whether its
 * sign-in lets an automated browser through (F5 bot defence and device
 * fingerprinting; docs on #67). This connector signs in the way a person
 * does -- the document number and password typed into the real form --
 * then leaves the second factor (SMS or ContiToken) to the person in the
 * window, opens the accounts and the cards, and records the JSON the page
 * itself fetches from apibanking-gw, to DATA_DIR/captures. The rows come
 * in step two, written from those shapes; until then it sends none.
 *
 * Only with RIGEL_SYNC_EXPERIMENTAL=1, and meant headful
 * (RIGEL_SYNC_HEADFUL=1) on the owner's computer. Sign-in, second-factor
 * and token calls are not recorded, token-like fields are dropped, and
 * every run of ten or more digits is masked to its last four. The file
 * stays on the runner.
 */
import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import type { HTTPResponse } from 'puppeteer-core';
import cloudflare, { closeSession } from '../../browser/cloudflare.ts';
import type { Batch, Connector } from '../types.ts';
import { SyncError } from '../types.ts';

const LOGIN = 'https://secure.bancontinental.com.py/auth/login/';
const GATEWAY = /(^|\.)apibanking-gw\.bancontinental\.com\.py$|(^|\.)secure\.bancontinental\.com\.py$/;
const SKIP_PATH = /auth|login|seguridad|segundo-?factor|token|autoriza|clave|otp|password/i;
const SECRET_KEY = /token|jwt|secret|clave|password|passwd|pin|cvv|otp|cookie|session/i;
const WAIT_SECOND_FACTOR_MS = 5 * 60_000;
const BROWSE_MS = 90_000;

/** A JSON value with secrets dropped and long numbers masked. */
export function scrub(v: unknown): unknown {
  if (Array.isArray(v)) return v.map(scrub);
  if (v && typeof v === 'object') {
    return Object.fromEntries(Object.entries(v).filter(([k]) => !SECRET_KEY.test(k)).map(([k, x]) => [k, scrub(x)]));
  }
  // Ten or more digits, spaced or dashed or not (a card number "4677 9100 ..."): the last four.
  if (typeof v === 'string') return v.replace(/\d(?:[ -]?\d){9,}/g, (m) => `******${m.replace(/\D/g, '').slice(-4)}`);
  if (typeof v === 'number' && Number.isInteger(v) && Math.abs(v) >= 1e9) return `******${String(Math.abs(v)).slice(-4)}`;
  return v;
}

export function makeContinental(opts: { dataDir?: string } = {}): Connector {
  return {
    id: 'py-continental',
    name: 'Banco Continental (Paraguay) -- capture only',
    country: 'PY',
    fields: [
      { name: 'document', label: 'Documento de identidad', kind: 'text' },
      { name: 'password', label: 'Contraseña', kind: 'secret' },
    ],
    async sync(ctx) {
      const { document: documentNumber, password } = ctx.credentials;
      if (!documentNumber || !password) throw new SyncError('bad_credentials');
      const browser = await cloudflare.launch(undefined);
      const captured: Array<{ at: string; method: string; path: string; status: number; body: unknown }> = [];
      try {
        const page = await browser.newPage();
        page.on('response', (r: HTTPResponse) => {
          void (async () => {
            const u = new URL(r.url());
            if (!GATEWAY.test(u.host) || SKIP_PATH.test(u.pathname)) return;
            if (!(r.headers()['content-type'] ?? '').includes('json')) return;
            const body = await r.json().catch(() => undefined);
            if (body === undefined) return;
            captured.push({ at: new Date().toISOString(), method: r.request().method(), path: u.host + u.pathname, status: r.status(), body: scrub(body) });
          })();
        });
        await page.goto(LOGIN, { waitUntil: 'networkidle2', timeout: 60_000 });
        await page.waitForSelector('input[name="document"]', { timeout: 30_000 });
        await page.type('input[name="document"]', documentNumber.trim(), { delay: 60 });
        await page.type('input[name="password"]', password, { delay: 60 });
        // In-page code as text: the runner is compiled without the DOM's types.
        await page.evaluate(`[...document.querySelectorAll('button')].find((b) => b.textContent.trim() === 'Ingresar')?.click()`);
        ctx.log.info('signed in through the form; finish any SMS or ContiToken step in the browser window');
        try {
          await page.waitForFunction(`!location.pathname.startsWith('/auth/')`, { timeout: WAIT_SECOND_FACTOR_MS });
        } catch {
          const text = String(await page.evaluate(`document.body.innerText.replace(/\\s+/g, ' ').slice(0, 300)`));
          throw new SyncError('verification_failed', `still on the sign-in page: ${text}`);
        }
        ctx.log.info('signed in; opening accounts and cards; open each account and the card statement in the window', { seconds: BROWSE_MS / 1000 });
        for (const route of ['/cuentas', '/tarjetas']) {
          await page.goto(new URL(route, LOGIN).href, { waitUntil: 'networkidle2', timeout: 60_000 }).catch(() => {});
        }
        await new Promise((r) => setTimeout(r, BROWSE_MS));
      } finally {
        await browser.close().catch(() => {});
        await closeSession(browser.sessionId()).catch(() => {});
      }
      const dir = join(opts.dataDir ?? process.env.DATA_DIR ?? '/data', 'captures');
      await mkdir(dir, { recursive: true, mode: 0o700 });
      const file = join(dir, `py-continental-${Date.now()}.json`);
      await writeFile(file, JSON.stringify(captured, null, 2), { mode: 0o600 });
      ctx.log.info('captured', { responses: captured.length, file });
      return { label: 'ContiWeb capture', accounts: [], rows: [] } satisfies Batch;
    },
  };
}

export const continental = makeContinental();
