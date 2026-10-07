/*
 * A KuDE (#44): the printed side of a Paraguayan e-invoice (SIFEN). Its QR
 * code is a link to ekuatia.set.gov.py whose query carries the invoice
 * itself, so a receipt is read from the QR, never by OCR:
 *
 *   Id           the CDC, 44 digits: type (2), seller RUC (8) and its check
 *                digit, establishment and point (3 + 3), number (7),
 *                taxpayer type, date (YYYYMMDD), emission type, security
 *                code (9), check digit
 *   dFeEmiDE     the emission date-time, hex-encoded ASCII
 *   dTotGralOpe  the total; cItems the number of lines
 *
 * The QR names no currency nor seller: the total is guaraníes (what nearly
 * every KuDE is), the seller its RUC. Some issuers write the total with
 * dots between thousands ("223.542" for ₲223.542), which guaraníes, having
 * no decimals, cannot otherwise mean; any other fraction is refused.
 *
 * Shared with the web app's import (no imports but types).
 */
import type { Account, Row } from '../connectors/types.ts';

export const KUDE_ACCOUNT: Account = { id: 'kude', label: 'KuDE (e-invoices, Paraguay)', currency: 'PYG' };

export interface Kude {
  cdc: string;
  ruc: string; // the seller's, with its check digit: 80107058-9
  number: string; // 001-001-0009852
  date: string; // YYYY-MM-DD
  total: string; // guaraníes, a whole number
  items?: number;
}

/** The KuDE a QR's text is, or null when it is not one. */
export function readKude(qr: string): Kude | null {
  let u: URL;
  try {
    u = new URL(qr.trim());
  } catch {
    return null;
  }
  if (!/(^|\.)set\.gov\.py$/.test(u.hostname)) return null;
  const q = u.searchParams;
  const cdc = q.get('Id') ?? '';
  const m = /^\d{2}(\d{8})(\d)(\d{3})(\d{3})(\d{7})\d(\d{4})(\d{2})(\d{2})\d{11}$/.exec(cdc);
  if (!m) return null;
  const [, ruc, dv, est, point, number, y, mo, d] = m;
  // The emission date, hex ASCII ("2025-12-19T10:20:30"); the CDC's when it is not.
  const hex = q.get('dFeEmiDE') ?? '';
  const stated = /^([0-9a-f]{2})+$/i.test(hex) ? (hex.match(/../g) ?? []).map((h) => String.fromCharCode(parseInt(h, 16))).join('').slice(0, 10) : '';
  const date = /^\d{4}-\d{2}-\d{2}$/.test(stated) ? stated : `${y}-${mo}-${d}`;
  const raw = q.get('dTotGralOpe') ?? '';
  let total: string;
  if (/^\d+$/.test(raw)) total = raw;
  else if (/^\d{1,3}(\.\d{3})+$/.test(raw)) total = raw.replace(/\./g, '');
  else if (/^\d+\.0+$/.test(raw)) total = raw.replace(/\.0+$/, '');
  else return null;
  total = total.replace(/^0+(?=\d)/, '');
  const items = Number(q.get('cItems'));
  return { cdc, ruc: `${ruc.replace(/^0+/, '')}-${dv}`, number: `${est}-${point}-${number}`, date, total, items: Number.isInteger(items) && items > 0 ? items : undefined };
}

/**
 * Its invoice row: a purchase of the total, the CDC both id and reference.
 * The QR holds no lines, so the invoice is one line of its total (the
 * photo, attached, has the rest).
 */
export function kudeRow(k: Kude, file?: string): Row {
  const description = `Factura ${k.number}`;
  return {
    kind: 'invoice', account: KUDE_ACCOUNT.id, id: k.cdc, date: k.date,
    amount: k.total === '0' ? '0' : `-${k.total}`, currency: 'PYG',
    description, counterparty: `RUC ${k.ruc}`, reference: k.cdc,
    items: [{ description: k.items ? `${description} (${k.items} items)` : description, amount: k.total }], file,
  };
}
