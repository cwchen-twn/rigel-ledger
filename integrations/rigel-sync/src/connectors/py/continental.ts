/*
 * Banco Continental (Paraguay), ContiWeb (#67): the month so far.
 *
 * ContiWeb's API answers are encrypted inside the page ("Encrypt-..."), so
 * nothing is read off the wire. The connector does what a person does:
 *
 *   sign in    document number and password typed into the real form; the
 *              second factor is a QR code to scan with Contimóvil, sent to
 *              the person as a `device` challenge (its image), with
 *              "Recordar este dispositivo" ticked. ContiWeb knows a device
 *              by its `_cdip` cookie (a year), kept in the connection's
 *              state, so later runs are not asked again.
 *   accounts   each account's "Descargar extracto: XLS" for this month and
 *              the last, read by statements/continental.ts with the same
 *              row ids as the PDF statements (#43).
 *   cards      each card's page: its movements since the last closing and
 *              "Deuda actual", the balance.
 *
 * The session lives only in the page's memory: moving between sections is
 * done from the side menu, as a click (history.pushState when there is no
 * entry), never by loading a URL.
 */
import { mkdtemp, readdir, readFile, rm, stat } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import type { CookieParam, Page } from 'puppeteer-core';
import cloudflare, { closeSession } from '../../browser/cloudflare.ts';
import { type CardScreen, continentalCurrency, ContinentalUnbalanced, readContinentalCardScreen, readContinentalSheet } from '../../statements/continental.ts';
import { readXlsx } from '../../util/xlsx.ts';
import type { Account, Batch, Connector, Row, SyncContext } from '../types.ts';
import { SyncError } from '../types.ts';

const ORIGIN = 'https://secure.bancontinental.com.py';
const LOGIN = `${ORIGIN}/auth/login/`;
const SECOND_FACTOR_MS = 5 * 60_000;
const DOWNLOAD_MS = 30_000;

// In-page code is text: the runner is compiled without the DOM's types.

/** The account cards on /cuentas: [{number, label}]. */
const ACCOUNTS = `(() => [...document.querySelectorAll('body *')]
  .filter((e) => e.children.length === 0 && /^\\d{8,14}$/.test(e.textContent.trim()))
  .map((e) => {
    let c = e;
    for (let i = 0; i < 5 && c.parentElement && getComputedStyle(c).cursor !== 'pointer'; i++) c = c.parentElement;
    return { number: e.textContent.trim(), label: (c.innerText.split('\\n').map((s) => s.trim()).find((s) => /^Cuenta/i.test(s)) || '') };
  }))()`;

/** Click the card of account number n (on /cuentas). */
const openAccount = (n: string) => `(() => {
  const e = [...document.querySelectorAll('body *')].find((x) => x.children.length === 0 && x.textContent.trim() === ${JSON.stringify(n)});
  let c = e;
  for (let i = 0; c && i < 5 && getComputedStyle(c).cursor !== 'pointer'; i++) c = c.parentElement;
  if (!c) return false;
  c.click();
  return true;
})()`;

const MONTH = '/^(Enero|Febrero|Marzo|Abril|Mayo|Junio|Julio|Agosto|Septiembre|Setiembre|Octubre|Noviembre|Diciembre) \\d{4}$/';
const MONTH_TABS = `[...document.querySelectorAll('button')].filter((b) => ${MONTH}.test(b.innerText.trim()))`;

/** The XLS icon: the second of "Descargar extracto:"'s icons (PDF, XLS). */
const CLICK_XLS = `(() => {
  const b = [...document.querySelectorAll('button')].find((x) => /Descargar extracto/.test(x.innerText));
  const icons = b ? [...b.querySelectorAll('div')].filter((d) => getComputedStyle(d).cursor === 'pointer' && d.querySelector('svg,img')) : [];
  const xls = icons[icons.length - 1];
  if (!xls || icons.length < 2) return false;
  xls.click();
  return true;
})()`;

/** The card tiles on /tarjetas, clicked by index. */
const CARD_TILES = `[...document.querySelectorAll('body *')]
  .filter((e) => e.children.length === 0 && /^Pago M[ií]nimo/i.test(e.textContent.trim()))
  .map((e) => { let c = e; for (let i = 0; c && i < 6 && getComputedStyle(c).cursor !== 'pointer'; i++) c = c.parentElement; return c; })
  .filter(Boolean)`;

/** A card's page: last four, debt, and the movements listed. */
const CARD_SCREEN = `(() => {
  const leaves = [...document.querySelectorAll('body *')].filter((e) => e.children.length === 0);
  const last4 = (leaves.find((e) => /^\\*{4} \\d{4}$/.test(e.textContent.trim()))?.textContent.trim() || '').slice(-4);
  const d = leaves.find((e) => e.textContent.trim() === 'Deuda actual');
  const debt = (d?.previousElementSibling?.innerText || '').trim();
  const items = leaves.filter((e) => /^\\d{1,2} de [a-záéíóú]+ de \\d{4}$/i.test(e.textContent.trim())).map((e) => {
    let c = e;
    for (let i = 0; c && i < 6 && !/(Gs|U\\$|USD)$/.test(c.innerText.trim()); i++) c = c.parentElement;
    const lines = (c?.innerText || '').split('\\n').map((s) => s.trim()).filter(Boolean);
    return { description: lines[0] || '', date: e.textContent.trim(), amount: lines[lines.length - 1] || '' };
  });
  return { last4, debt, items };
})()`;

/** What the sign-in page asks for next: the QR, a code, or an error it shows. */
const SECOND_FACTOR = `(() => {
  const text = document.body.innerText.replace(/\\s+/g, ' ');
  if (/escane[aá] este c[oó]digo QR/i.test(text)) return 'qr';
  if (document.querySelector('input[autocomplete="one-time-code"], input[maxlength="6"]')) return 'code';
  return '';
})()`;

/** Tick "Recordar este dispositivo" (the checkbox before its label). */
const REMEMBER = `(() => {
  const l = [...document.querySelectorAll('body *')].find((e) => e.children.length === 0 && /Recordar este dispositivo/i.test(e.textContent));
  const box = l?.parentElement?.querySelector('input[type=checkbox]');
  if (!box) return false;
  if (!box.checked) box.click();
  return box.checked;
})()`;

/** Signed in: off /auth/, and no "Tu sesión expiró" (15 minutes idle) over the page. */
const signedIn = async (page: Page) =>
  Boolean(await page.evaluate(`!location.pathname.startsWith('/auth/') && !/Tu sesi[oó]n expir[oó]/.test(document.body.innerText)`).catch(() => false));

/**
 * Open a section from the side menu, as a person does (the menu loads its
 * data); history.pushState when there is no such entry.
 */
async function go(page: Page, menu: string, route: string) {
  const clicked = await page.evaluate(`(() => {
    const e = [...document.querySelectorAll('body *')].find((x) => x.children.length === 0 && x.textContent.trim() === ${JSON.stringify(menu)} && x.getBoundingClientRect().left < 300);
    if (e) e.click();
    return !!e;
  })()`);
  if (!clicked) await page.evaluate(`history.pushState({}, '', ${JSON.stringify(route)}); dispatchEvent(new PopStateEvent('popstate'))`);
  await page.waitForFunction(`location.pathname === ${JSON.stringify(route)}`, { timeout: 15_000 }).catch(() => {});
  await settle(page);
  if (!(await signedIn(page))) throw new SyncError('verification_failed', `signed out on ${route}`);
}

const settle = (page: Page) => page.waitForNetworkIdle({ idleTime: 1200, timeout: 30_000 }).catch(() => {});

/** The QR (canvas, svg or img) the second-factor page shows, as a PNG. */
async function qrImage(page: Page): Promise<Buffer | undefined> {
  const handle = await page.evaluateHandle(`(() => {
    const all = [...document.querySelectorAll('canvas, svg, img')].filter((e) => { const r = e.getBoundingClientRect(); return r.width >= 100 && Math.abs(r.width - r.height) < 12; });
    return all.sort((a, b) => b.getBoundingClientRect().width - a.getBoundingClientRect().width)[0] || null;
  })()`);
  const el = handle.asElement();
  if (!el) return undefined;
  return Buffer.from(await el.screenshot({ type: 'png' }));
}

async function signIn(page: Page, ctx: SyncContext, documentNumber: string, password: string) {
  await page.goto(LOGIN, { waitUntil: 'networkidle2', timeout: 60_000 });
  await page.waitForSelector('input[name="document"]', { timeout: 30_000 });
  await page.type('input[name="document"]', documentNumber.trim(), { delay: 60 });
  await page.type('input[name="password"]', password, { delay: 60 });
  await page.evaluate(`[...document.querySelectorAll('button')].find((b) => b.textContent.trim() === 'Ingresar')?.click()`);
  const until = Date.now() + SECOND_FACTOR_MS;
  let asked = false;
  while (Date.now() < until) {
    if (ctx.signal.aborted) throw new SyncError('cancelled');
    if (await signedIn(page)) return;
    const step = String(await page.evaluate(SECOND_FACTOR).catch(() => ''));
    if (step === 'qr' && !asked) {
      asked = true;
      const remembered = await page.evaluate(REMEMBER).catch(() => false);
      ctx.log.info('second factor: QR', { remember: remembered });
      const image = await qrImage(page);
      // The page moves on by itself once the QR is scanned; the answer only
      // says the person is done, so either may come first.
      void ctx.ask({ kind: 'device', prompt: 'Scan this QR code with Contimóvil (Escaneá este código QR con Contimóvil)', image, ttlSeconds: 180 }).catch(() => {});
    } else if (step === 'code' && !asked) {
      asked = true;
      const code = (await ctx.ask({ kind: 'otp', prompt: 'The code ContiWeb sent you (SMS)', ttlSeconds: 300 })).trim();
      await page.type('input[autocomplete="one-time-code"], input[maxlength="6"]', code, { delay: 60 });
      await page.keyboard.press('Enter');
    }
    await new Promise((r) => setTimeout(r, 1000));
  }
  const text = String(await page.evaluate(`document.body.innerText.replace(/\\s+/g, ' ').slice(0, 200)`).catch(() => ''));
  if (/incorrect|inv[aá]lid|bloquead/i.test(text)) throw new SyncError('bad_credentials');
  throw new SyncError(asked ? 'challenge_expired' : 'verification_failed', 'still on the sign-in page');
}

/** The next finished file in dir that was not there before; undefined when none came. */
async function nextFile(dir: string, before: Set<string>): Promise<string | undefined> {
  const until = Date.now() + DOWNLOAD_MS;
  while (Date.now() < until) {
    for (const f of await readdir(dir)) {
      if (before.has(f) || f.endsWith('.crdownload')) continue;
      if ((await stat(join(dir, f))).size > 0) return join(dir, f);
    }
    await new Promise((r) => setTimeout(r, 300));
  }
  return undefined;
}

const today = () => new Intl.DateTimeFormat('en-CA', { timeZone: 'America/Asuncion' }).format(new Date());

export function makeContinental(): Connector {
  return {
    id: 'py-continental',
    name: 'Banco Continental (Paraguay)',
    country: 'PY',
    fields: [
      { name: 'document', label: 'Documento de identidad', kind: 'text' },
      { name: 'password', label: 'Contraseña', kind: 'secret' },
    ],
    async sync(ctx) {
      const { document: documentNumber, password } = ctx.credentials;
      if (!documentNumber || !password) throw new SyncError('bad_credentials');
      const downloads = await mkdtemp(join(tmpdir(), 'rigel-conti-'));
      const browser = await cloudflare.launch(undefined);
      const accounts: Account[] = [];
      const rows: Row[] = [];
      try {
        const page = await browser.newPage();
        // The remembered device: ContiWeb's cookies from the last run.
        const saved = (ctx.state.cookies ?? []) as CookieParam[];
        if (saved.length) await page.setCookie(...saved);
        await signIn(page, ctx, documentNumber, password);
        const cookies = (await page.cookies(ORIGIN)).map(({ name, value, domain, path, expires, secure, httpOnly, sameSite }) => ({ name, value, domain, path, expires, secure, httpOnly, sameSite }));
        await ctx.saveState({ ...ctx.state, cookies });
        ctx.log.info('signed in', { remembered: saved.some((c) => c.name === '_cdip') });

        const cdp = await page.createCDPSession();
        await cdp.send('Browser.setDownloadBehavior', { behavior: 'allow', downloadPath: downloads, eventsEnabled: true });

        await go(page, 'Cuentas', '/cuentas');
        await page.waitForFunction(`[...document.querySelectorAll('body *')].some((e) => e.children.length === 0 && /^\\d{8,14}$/.test(e.textContent.trim()))`, { timeout: 20_000 }).catch(() => {});
        const list = (await page.evaluate(ACCOUNTS)) as Array<{ number: string; label: string }>;
        for (const a of list) {
          const currency = continentalCurrency(a.label);
          if (!currency) {
            ctx.log.warn('account in a currency not handled; skipped', { label: a.label });
            continue;
          }
          const ref = { number: a.number, label: a.label, currency };
          await go(page, 'Cuentas', '/cuentas');
          await page.waitForFunction(`[...document.querySelectorAll('body *')].some((e) => e.children.length === 0 && e.textContent.trim() === ${JSON.stringify(a.number)})`, { timeout: 20_000 }).catch(() => {});
          if (!(await page.evaluate(openAccount(a.number)))) throw new SyncError('institution_down', 'account card not found');
          await page.waitForFunction(`location.pathname === '/extracto-cuenta' && ${MONTH_TABS}.length > 0`, { timeout: 30_000 });
          await settle(page);
          let n = 0;
          // This month and the last: the days before a statement PDF comes.
          for (const tab of [0, 1]) {
            if (tab > 0 && !(await page.evaluate(`(() => { const t = ${MONTH_TABS}[${tab}]; if (t) t.click(); return !!t; })()`))) break;
            await settle(page);
            const before = new Set(await readdir(downloads));
            if (!(await page.evaluate(CLICK_XLS))) throw new SyncError('institution_down', 'no XLS download on the account page');
            const file = await nextFile(downloads, before);
            if (!file) {
              // A month with no movements may give no file at all.
              ctx.log.warn('no XLS came for a month', { account: `***${a.number.slice(-4)}`, tab });
              continue;
            }
            const st = readContinentalSheet(readXlsx(await readFile(file)), ref);
            await rm(file, { force: true });
            if (!accounts.some((x) => x.id === st.account.id)) accounts.push(st.account);
            for (const r of st.rows) if (!rows.some((x) => x.id === r.id)) rows.push(r);
            n += st.rows.length;
          }
          ctx.log.info('account read', { account: `***${a.number.slice(-4)}`, currency, rows: n });
        }

        await go(page, 'Tarjetas', '/tarjetas');
        await page.waitForFunction(`${CARD_TILES}.length > 0`, { timeout: 20_000 }).catch(() => {});
        const tiles = Number(await page.evaluate(`${CARD_TILES}.length`));
        for (let i = 0; i < tiles; i++) {
          await go(page, 'Tarjetas', '/tarjetas');
          await page.waitForFunction(`${CARD_TILES}.length > ${i}`, { timeout: 20_000 }).catch(() => {});
          await page.evaluate(`${CARD_TILES}[${i}]?.click()`);
          await page.waitForFunction(`location.pathname === '/extracto-tarjeta' && document.body.innerText.includes('Deuda actual')`, { timeout: 30_000 });
          await settle(page);
          const screen = (await page.evaluate(CARD_SCREEN)) as CardScreen;
          const st = readContinentalCardScreen(screen, today());
          accounts.push(st.account);
          rows.push(...st.rows);
          ctx.log.info('card read', { card: `***${screen.last4}`, rows: st.rows.length });
        }
      } catch (err) {
        if (err instanceof ContinentalUnbalanced) throw new SyncError('unexpected_page', err.message);
        throw err;
      } finally {
        await browser.close().catch(() => {});
        await closeSession(browser.sessionId()).catch(() => {});
        await rm(downloads, { recursive: true, force: true });
      }
      return { label: `ContiWeb ${today()}`, accounts, rows } satisfies Batch;
    },
  };
}

export const continental = makeContinental();
