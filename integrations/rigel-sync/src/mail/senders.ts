/*
 * Per-sender parsers (#41, #57), written from real emails and tested on
 * invented ones (test/senders.test.ts). An email is evidence: each parser
 * reads only what joins it to the payment -- the seller, the day, the total
 * the email states and an invoice number another source knows. The amount
 * in an attached PDF is not read: an email that states none is matched by
 * its seller and day, and its PDF is kept with the payment.
 */
import { plainDecimal } from '../connectors/rows.ts';
import type { Mail, Parsed, Parser } from './parse.ts';
import { plainText } from './text.ts';

/** A Taiwan uniform invoice number (統一發票字軌號碼): two letters, eight digits. */
const GUI = /\b([A-Z]{2}-?\d{8})\b/;

const SYMBOLS: Record<string, string> = { 'NT$': 'TWD', 'US$': 'USD', 'HK$': 'HKD', 'A$': 'AUD', 'C$': 'CAD', '€': 'EUR', '£': 'GBP', '¥': 'JPY', '₲': 'PYG' };

/** "NT$", "€", "USD": an ISO code, or `bare` for a lone "$" (each sender knows its own). */
function currencyOf(symbol: string, bare: string): string | undefined {
  const s = symbol.trim().toUpperCase();
  if (s === '$') return bare;
  if (/^[A-Z]{3}$/.test(s)) return s;
  return SYMBOLS[symbol.trim()] ?? SYMBOLS[s];
}

const from = (mail: Mail, ...domains: string[]) => domains.some((d) => mail.from === d || mail.from.endsWith(`@${d}`) || mail.from.endsWith(`.${d}`));

function order(mail: Mail, p: Omit<Extract<Parsed, { kind: 'order' }>, 'kind' | 'day' | 'items'> & { day?: string; item?: string }): Parsed[] {
  const { item, ...rest } = p;
  return [{
    kind: 'order',
    day: mail.day,
    ...rest,
    items: rest.total ? [{ description: item || p.seller, amount: rest.total }] : [],
  }];
}

/** Hetzner: "The open invoice amount of $ 17.18 will soon be debited from your credit card." */
export const hetzner: Parser = {
  name: 'Hetzner',
  parse(mail) {
    if (!from(mail, 'hetzner.com', 'hetzner.de')) return null;
    const t = plainText(mail);
    const m = /invoice amount of\s*([$€£]|EUR|USD)\s*([\d.,]+)/i.exec(t);
    const total = m && plainDecimal(m[2]);
    const currency = m && currencyOf(m[1], 'USD');
    if (!total || !currency) return null;
    const number = /invoice\s+(\d{6,})/i.exec(mail.subject)?.[1];
    return order(mail, { seller: 'Hetzner Online GmbH', number, total, currency, item: number ? `Invoice ${number}` : undefined });
  },
};

/** DigitalOcean: "Invoice Total: $6.00" (billed in USD). */
export const digitalOcean: Parser = {
  name: 'DigitalOcean',
  parse(mail) {
    if (!from(mail, 'digitalocean.com')) return null;
    const m = /Invoice Total:\s*\$\s*([\d.,]+)/i.exec(plainText(mail));
    const total = m && plainDecimal(m[1]);
    if (!total) return null;
    const month = /(\d{4}-\d{2}) invoice/i.exec(mail.subject)?.[1];
    return order(mail, { seller: 'DigitalOcean', number: month, total, currency: 'USD', item: month ? `Invoice ${month}` : undefined });
  },
};

/**
 * Apple: "ORDER ID MX… INVOICE NUMBER AB12345678 … TOTAL NT$ 190". In
 * Taiwan the invoice number is the 電子發票 (cloud invoice) Apple issues,
 * so the email and the 電子發票 are one invoice.
 */
export const apple: Parser = {
  name: 'Apple',
  parse(mail) {
    if (!from(mail, 'email.apple.com', 'apple.com')) return null;
    const t = plainText(mail);
    const m = /\bTOTAL\s+(NT\$|US\$|HK\$|A\$|C\$|\$|€|£|¥|[A-Z]{3})\s*([\d.,]+)/.exec(t);
    const total = m && plainDecimal(m[2]);
    const currency = m && currencyOf(m[1], 'USD');
    if (!total || !currency) return null;
    const gui = /INVOICE NUMBER\s+([A-Z]{2}\d{8})\b/.exec(t)?.[1];
    const what = /INVOICE NUMBER\s+\S+\s+(?:App Store|Apple Music|iCloud\+?|Apple TV\+?|Apple Arcade|Apple One)?\s*(.{1,80}?)\s+(?:In-App Purchase|Subscription|Renews|Report a Problem)/.exec(t)?.[1];
    return order(mail, { seller: 'Apple', number: /ORDER ID\s+([A-Z0-9]+)/.exec(t)?.[1], total, currency, reference: gui, item: what });
  },
};

/** Gotogate (flights): "Order number 1147-…" in the subject, "Total 1,231.00 USD". */
export const gotogate: Parser = {
  name: 'Gotogate',
  parse(mail) {
    if (!from(mail, 'gotogate.com')) return null;
    const m = /\bTotal\s+([\d.,]+)\s+([A-Z]{3})\b/.exec(plainText(mail));
    const total = m && plainDecimal(m[1]);
    if (!total || !m) return null;
    const number = /Order number\s+([\d-]+)/i.exec(mail.subject)?.[1];
    return order(mail, { seller: 'Gotogate', number, total, currency: m[2], item: number ? `Order ${number}` : 'Flights' });
  },
};

/**
 * 遠傳 (FET) bills: "應繳金額 $596" -- what the card or the bank is charged,
 * last period's unpaid amount included.
 */
export const fetnet: Parser = {
  name: '遠傳電信',
  parse(mail) {
    if (!from(mail, 'fetnet.net')) return null;
    const m = /應繳金額\s*(?:NT)?\$?\s*([\d,]+)/.exec(plainText(mail));
    const total = m && plainDecimal(m[1]);
    if (!total || total === '0') return null;
    const period = /(\d{2,3}\s*年\s*\d{1,2}\s*月)/.exec(mail.subject)?.[1]?.replace(/\s+/g, '');
    return order(mail, { seller: '遠傳電信', number: period, total, currency: 'TWD', item: period ? `${period}帳單` : '電信帳單' });
  },
};

/**
 * 電子發票 notices from a platform (關貿 tradevan: Spotify AB): the invoice
 * number and no amount. The 電子發票 itself (tw-einvoice) brings the amount.
 */
export const eguiNotice: Parser = {
  name: '電子發票開立通知',
  parse(mail) {
    if (!from(mail, 'tradevan.com.tw')) return null;
    const t = plainText(mail);
    const number = /發票號碼\s*(?:Invoice Number)?\s*([A-Z]{2}-?\d{8})/.exec(t)?.[1] ?? GUI.exec(t)?.[1];
    if (!number) return null;
    const seller = (mail.fromName || /電子發票已開立.*?Invoice has been issued\s+(.+?)\s+發票號碼/.exec(t)?.[1] || '').trim();
    return order(mail, { seller: seller || 'tradevan', number: number.replace('-', ''), currency: 'TWD', reference: number.replace('-', '') });
  },
};

/** Trip.com: the order number; the price is only in the attached PDF, so it is evidence. */
export const tripCom: Parser = {
  name: 'Trip.com',
  parse(mail) {
    if (!from(mail, 'trip.com')) return null;
    const t = plainText(mail);
    const number = /(?:訂單編號|Booking No\.?|Order (?:No\.?|number))[：:\s]*(\d{8,})/i.exec(t)?.[1];
    if (!number) return null;
    const m = /(?:總額|總計|總價|Total)[^\d]{0,12}?(NT\$|US\$|HK\$|TWD|USD|HKD|EUR)\s*([\d.,]+)/i.exec(t);
    const total = m ? plainDecimal(m[2]) : undefined;
    const currency = (m && currencyOf(m[1], 'USD')) || 'TWD';
    return order(mail, { seller: 'Trip.com', number, total: total || undefined, currency, item: mail.subject || undefined });
  },
};

export const senders: Parser[] = [hetzner, digitalOcean, apple, gotogate, fetnet, eguiNotice, tripCom];
