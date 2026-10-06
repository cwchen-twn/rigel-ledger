/*
 * What an email says about money, read locally (docs/ARCHITECTURE.md,
 * "Email (Gmail) as a source"): no mail content goes anywhere else.
 *
 * Parsers are tried in order, and the first that recognises an email wins:
 * per-sender ones (card alerts, Taiwan shops, app stores: written from real
 * samples), then schema.org markup, which many senders embed for Gmail
 * (Order, Invoice). An email nothing recognises is left alone.
 */
import { addDecimal, mulDecimal, plainDecimal } from '../connectors/rows.ts';
import type { InvoiceItem } from '../connectors/types.ts';

/** An email, decoded. */
export interface Mail {
  messageId: string;
  from: string; // the address, lower case
  fromName: string;
  subject: string;
  /** The day it was sent, at the sender's own offset (the Date header's). */
  day: string;
  html?: string;
  text?: string;
}

/** What a parser found. Amounts are plain decimal strings, positive for money spent. */
export type Parsed =
  | {
      kind: 'order';
      seller: string;
      number?: string;
      day: string;
      total: string;
      currency: string;
      items: InvoiceItem[];
    }
  | {
      kind: 'card_alert';
      issuer: string; // e.g. tw-cathaybk
      card: string; // the card's last four digits
      day: string;
      amount: string;
      currency: string;
      merchant: string;
    };

export interface Parser {
  name: string;
  parse(mail: Mail): Parsed[] | null;
}

/** The day a Date header names, at its own offset ("Tue, 2 Sep 2026 23:30:00 +0800" -> 2026-09-02). */
export function mailDay(header: string | undefined, fallback: Date): string {
  const t = header ? Date.parse(header) : Number.NaN;
  const m = header ? /([+-])(\d{2})(\d{2})\s*(\([^)]*\))?\s*$/.exec(header.trim()) : null;
  if (Number.isNaN(t)) return fallback.toISOString().slice(0, 10);
  const offset = m ? (m[1] === '-' ? -1 : 1) * (Number(m[2]) * 60 + Number(m[3])) : 0;
  return new Date(t + offset * 60_000).toISOString().slice(0, 10);
}

/** A schema.org date as written ("2026-09-02T10:00:00+08:00" -> 2026-09-02): the sender's own day. */
function schemaDay(value: unknown): string | undefined {
  const m = typeof value === 'string' ? /^(\d{4}-\d{2}-\d{2})/.exec(value.trim()) : null;
  return m?.[1];
}

type Json = Record<string, unknown>;
const isObj = (v: unknown): v is Json => typeof v === 'object' && v !== null && !Array.isArray(v);
const list = (v: unknown): unknown[] => (Array.isArray(v) ? v : v === undefined || v === null ? [] : [v]);
const types = (o: Json) => list(o['@type']).map(String);
const text = (v: unknown): string | undefined => {
  if (typeof v === 'string') return v.trim() || undefined;
  if (isObj(v)) return text(v.name);
  return undefined;
};
const amount = (v: unknown) => (typeof v === 'number' || typeof v === 'string' ? plainDecimal(v) : undefined);

/** Every JSON-LD object in an email's HTML, @graph and arrays flattened. */
export function jsonLd(html: string): Json[] {
  const out: Json[] = [];
  const walk = (v: unknown) => {
    if (Array.isArray(v)) v.forEach(walk);
    else if (isObj(v)) {
      out.push(v);
      if (v['@graph']) walk(v['@graph']);
    }
  };
  for (const m of html.matchAll(/<script[^>]*type\s*=\s*["']?application\/ld\+json["']?[^>]*>([\s\S]*?)<\/script>/gi)) {
    try {
      walk(JSON.parse(m[1].replace(/&quot;/g, '"').trim()));
    } catch {
      // markup that does not parse is not an order
    }
  }
  return out;
}

/** A price: a number, or a PriceSpecification, or the first of a list. */
function price(v: unknown): { value: string; currency?: string } | undefined {
  for (const p of list(v)) {
    if (isObj(p)) {
      const value = amount(p.price ?? p.value);
      if (value) return { value, currency: text(p.priceCurrency) };
    } else {
      const value = amount(p);
      if (value) return { value };
    }
  }
  return undefined;
}

/** schema.org Order and Invoice (https://developers.google.com/gmail/markup/reference/order). */
export const schemaOrg: Parser = {
  name: 'schema.org',
  parse(mail) {
    if (!mail.html) return null;
    const found: Parsed[] = [];
    for (const o of jsonLd(mail.html)) {
      const t = types(o);
      if (!t.includes('Order') && !t.includes('Invoice')) continue;
      const seller = text(o.seller) ?? text(o.merchant) ?? text(o.provider) ?? text(o.broker) ?? mail.fromName ?? mail.from;
      const items: InvoiceItem[] = [];
      let currency: string | undefined;
      for (const offer of list(o.acceptedOffer ?? o.orderedItem ?? (isObj(o.referencesOrder) ? o.referencesOrder.acceptedOffer : undefined))) {
        if (!isObj(offer)) continue;
        const name = text(offer.itemOffered) ?? text(offer.orderedItem) ?? text(offer.name) ?? '';
        const unit = price(offer.price !== undefined ? { price: offer.price, priceCurrency: offer.priceCurrency } : offer.priceSpecification);
        const quantity = amount(isObj(offer.eligibleQuantity) ? offer.eligibleQuantity.value : (offer.orderQuantity ?? offer.eligibleQuantity)) ?? '1';
        if (!unit) continue;
        currency ??= unit.currency;
        items.push({
          description: name,
          quantity,
          unit_price: unit.value,
          amount: quantity === '1' ? unit.value : mulDecimal(unit.value, quantity),
        });
      }
      const total = price(o.totalPaymentDue) ?? price(o.price !== undefined ? { price: o.price, priceCurrency: o.priceCurrency } : o.priceSpecification);
      const sum = items.reduce((acc, it) => addDecimal(acc, it.amount), '0');
      const value = total?.value ?? (items.length ? sum : undefined);
      currency = (total?.currency ?? currency ?? text(o.priceCurrency))?.toUpperCase();
      if (!value || value === '0' || !currency || !/^[A-Z]{3}$/.test(currency)) continue;
      found.push({
        kind: 'order',
        seller,
        number: text(o.orderNumber) ?? text(o.confirmationNumber) ?? text(o.identifier),
        day: schemaDay(o.orderDate) ?? schemaDay(o.paymentDueDate) ?? mail.day,
        total: value,
        currency,
        items: items.length ? items : [{ description: seller, amount: value }],
      });
    }
    return found.length ? found : null;
  },
};

/** The parsers, most specific first. Per-sender ones join here as samples arrive. */
export const parsers: Parser[] = [schemaOrg];

export function parseMail(mail: Mail, list: Parser[] = parsers): Parsed[] {
  for (const p of list) {
    const found = p.parse(mail);
    if (found?.length) return found;
  }
  return [];
}
