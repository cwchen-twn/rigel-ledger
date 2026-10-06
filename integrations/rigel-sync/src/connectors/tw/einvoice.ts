/*
 * 電子發票 (Taiwan e-invoices) on a mobile barcode carrier (手機條碼,
 * 載具): the last two invoice periods (four months) and each invoice's
 * lines, from all-set-tw's connector (vendor/all-set-tw, a pinned commit,
 * unedited), which speaks the government e-invoice app's API -- no browser,
 * no code to answer: the carrier's mobile number and password.
 *
 * The app's session is kept in the connection's state and tried first; when
 * the API refuses it, the runner signs in again.
 *
 * Each invoice is an `invoice` row (the app's import core, #38): it adds its
 * lines to the card, bank or cash payment it matches, and only an invoice
 * nothing paid for is booked, as a cash purchase from the account the
 * carrier is mapped to. An invoice's id is its number and its day in
 * Taiwan: upstream's id embeds a date computed in the runner's own time
 * zone, which would make a laptop's and a server's ids differ.
 */
import { randomBytes } from 'node:crypto';
import {
  type EInvoiceInvoiceHeader,
  type EInvoiceSessionConfigUpdates,
  fetchEInvoiceInvoiceDetail,
  initializeEInvoiceSync,
  parseInvoiceConfig,
} from '../../../vendor/all-set-tw/einvoice.js';
import { toDecimal } from '../../money.ts';
import { plainDecimal, taipeiDay } from '../rows.ts';
import type { Batch, Connector, InvoiceItem, Logger, Row } from '../types.ts';
import { SyncError } from '../types.ts';

interface EInvoiceState {
  androidId?: string; // this runner, as the app's device
  session?: EInvoiceSessionConfigUpdates;
}

export const einvoice: Connector = {
  id: 'tw-einvoice',
  name: '電子發票 Taiwan e-invoices',
  country: 'TW',
  fields: [
    { name: 'mobile', label: 'Mobile number (手機號碼)', kind: 'text' },
    { name: 'password', label: 'Carrier password (載具驗證碼)', kind: 'secret' },
  ],
  async sync(ctx) {
    const mobile = (ctx.credentials.mobile ?? '').replace(/[\s-]/g, '');
    const password = ctx.credentials.password;
    if (!mobile || !password) throw new SyncError('bad_credentials');
    const state = ctx.state as EInvoiceState;
    const androidId = state.androidId ?? randomBytes(8).toString('hex');
    const base = { mobile, password, androidId };

    let synced: Awaited<ReturnType<typeof initializeEInvoiceSync>>;
    try {
      synced = await initializeEInvoiceSync(parseInvoiceConfig({ ...base, ...state.session }));
    } catch (err) {
      if (!state.session) throw failure(err);
      // The kept session was refused: sign in again, once.
      ctx.log.info('the saved e-invoice session was refused; signing in again');
      try {
        synced = await initializeEInvoiceSync(parseInvoiceConfig(base));
      } catch (again) {
        throw failure(again);
      }
    }
    await ctx.saveState({ androidId, session: synced.configUpdates });

    const items = new Map<string, InvoiceItem[]>();
    for (const task of synced.detailTasks) {
      try {
        const detail = await fetchEInvoiceInvoiceDetail(synced.session, task);
        items.set(task.sourceId, detail.detailItems.flatMap((it) => {
          const amount = plainDecimal(it.amount);
          if (amount === undefined) return [];
          return [{ description: it.description.trim(), quantity: plainDecimal(it.quantity), unit_price: plainDecimal(it.unitPrice), amount }];
        }));
      } catch (err) {
        // One invoice's lines missing does not lose the others: it still matches by its total.
        ctx.log.warn('an invoice\'s lines could not be read', { invoice: task.invNum, error: err instanceof Error ? err.message : String(err) });
      }
    }
    return toBatch(synced.configUpdates.mobileBarcode ?? '', synced.headers, items, taipeiDay(new Date()), ctx.log);
  },
};

/** A stable code for what the person can act on; anything else stays an error for the log. */
function failure(err: unknown): unknown {
  const message = err instanceof Error ? err.message : String(err);
  if (/HTTP 5\d\d|逾時|fetch failed/.test(message)) return new SyncError('institution_down', message);
  // Upstream wraps every refused sign-in this way; the API names no single code for a wrong password.
  if (message.startsWith('電子發票登入失敗')) return new SyncError('bad_credentials', message);
  return err;
}

/** An invoice's day in Taiwan: upstream gives YYYY-MM-DD, or a UTC timestamp when the API had a time. */
export function invoiceDay(value: string): string | undefined {
  if (/^\d{4}-\d{2}-\d{2}$/.test(value)) return value;
  const t = Date.parse(value);
  return Number.isNaN(t) ? undefined : taipeiDay(new Date(t));
}

/**
 * The carrier's invoices as one batch: one account, `carrier`, labelled with
 * the barcode, and an invoice row per invoice with its lines (or, when they
 * could not be read, one line of its total).
 */
export function toBatch(
  barcode: string,
  headers: EInvoiceInvoiceHeader[],
  items: Map<string, InvoiceItem[]>,
  day: string,
  log: Pick<Logger, 'warn'>,
): Batch {
  const rows: Row[] = [];
  const seen = new Set<string>();
  for (const h of headers) {
    const date = invoiceDay(h.invoice.invoiceDate);
    const number = h.invoice.invoiceNumber ?? h.invNum;
    if (!date || !number || h.invoice.amount === 0) {
      if (!date || !number) log.warn('an invoice without a number or a date was left out', { seller: h.invoice.sellerName });
      continue;
    }
    const id = `${number}:${date}`;
    if (seen.has(id)) continue;
    seen.add(id);
    const total = toDecimal(h.invoice.amount, 'TWD');
    const seller = h.invoice.sellerName ?? '';
    const lines = items.get(h.sourceId);
    rows.push({
      kind: 'invoice', account: 'carrier', id, date, currency: 'TWD',
      amount: toDecimal(-h.invoice.amount, 'TWD'), // a purchase: out of whatever paid
      counterparty: seller || undefined,
      description: `發票 ${number}`,
      items: lines?.length ? lines : [{ description: seller || number, amount: total }],
    });
  }
  const label = barcode ? `手機條碼 ${barcode}` : '手機條碼';
  return { label: `電子發票 ${day}`, accounts: [{ id: 'carrier', label, currency: 'TWD' }], rows };
}
