/*
 * A pretend e-invoice app API (uia/upi.einvoice.nat.gov.tw), in this
 * process: the vendored client calls the global fetch, so the test swaps it
 * for those two hosts. Sign-in decrypts the app's ldata as the app encrypts
 * it; every invoice request is a JWT the fake checks against the session it
 * gave out, so expireSessions() makes a kept session fail as a real one
 * would. Password "wrong" fails. Invoice CD00000002's lines fail to load.
 */
import { createDecipheriv, createHash, createHmac, randomBytes } from 'node:crypto';

const MIDDLE = 'https://uia.einvoice.nat.gov.tw';
const BIG = 'https://upi.einvoice.nat.gov.tw';
export const BARCODE = '/ABC1234';

const swapPairs = (v: string) => {
  let out = '';
  for (let i = 0; i < v.length; i += 2) out += (v[i + 1] ?? '') + (v[i] ?? '');
  return out;
};
const reverse = (v: string) => [...v].reverse().join('');

function openLData(value: string): Record<string, unknown> {
  const [a, encoded, b] = value.split('|');
  const key = createHash('sha256').update(`${swapPairs(b)}${swapPairs(a)}${reverse(a)}${reverse(b)}`).digest();
  const bytes = Buffer.from(encoded, 'base64');
  const d = createDecipheriv('aes-256-gcm', key, Buffer.from(`${a.slice(-6)}${b.slice(-6)}`));
  d.setAuthTag(bytes.subarray(-16));
  return JSON.parse(Buffer.concat([d.update(bytes.subarray(0, -16)), d.final()]).toString());
}

// Taiwan midnight of a day, as the API's epoch form gives it.
const taipeiMidnight = (day: string) => Date.parse(`${day}T00:00:00+08:00`);

interface Invoice { invNum: string; day: string; seller: string; amount: string; byEpoch?: boolean; lines: Array<Record<string, string>> }
const INVOICES: Invoice[] = [
  { invNum: 'AB12345678', day: '2026-09-02', seller: '全聯福利中心', amount: '320',
    lines: [{ rowNum: '1', description: '光泉鮮乳 936ml', quantity: '1', unitPrice: '90', amount: '90' },
      { rowNum: '2', description: '舒潔衛生紙 12入', quantity: '1', unitPrice: '230', amount: '230' }] },
  // Dated by epoch: Taiwan midnight, the day before in UTC.
  { invNum: 'CD00000002', day: '2026-09-12', seller: '豪大雞排', amount: '85', byEpoch: true, lines: [] },
  { invNum: 'EF00000003', day: '2026-08-20', seller: '7-ELEVEN', amount: '45',
    lines: [{ rowNum: '1', description: '美式咖啡', quantity: '1', unitPrice: '45.00', amount: '45.00' }] },
];

export interface FakeEInvoice {
  logins: number;
  details: number;
  expireSessions(): void;
  restore(): void;
}

export function installFakeEInvoice(): FakeEInvoice {
  const real = globalThis.fetch;
  let ssme = '';
  const fake: FakeEInvoice = { logins: 0, details: 0, expireSessions: () => void (ssme = ''), restore: () => void (globalThis.fetch = real) };
  const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } });

  globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input instanceof Request ? input.url : input);
    if (url === `${MIDDLE}/mid/v1/login`) {
      fake.logins++;
      const inner = openLData(JSON.parse(String(init?.body)).ldata as string);
      if (inner.password === 'wrong') return json({ result: 401, message: '帳號或密碼錯誤' });
      ssme = randomBytes(16).toString('hex');
      return json({ result: 0, payload: {
        sid: randomBytes(9).toString('hex'), token: randomBytes(24).toString('hex'), appid: 'app-1', liat: 1, ssme,
        carrier_code: BARCODE, now: Math.floor(Date.now() / 1000),
      } });
    }
    if (!url.startsWith(BIG)) return real(input, init);

    // The request is a JWT signed with the session's ssme.
    const jwt = new URLSearchParams(String(init?.body)).get('einvoiceJwt') ?? '';
    const [h, c, sig] = jwt.split('.');
    if (!ssme || createHmac('sha256', ssme).update(`${h}.${c}`).digest('base64url') !== sig) return json({ message: 'unauthorized' }, 401);
    const req = JSON.parse(Buffer.from(c, 'base64url').toString()).reqdata as Record<string, string>;

    if (url.endsWith('/query-invoices-header')) {
      const [from, to] = [req.startDate.replace(/\//g, '-'), req.endDate.replace(/\//g, '-')];
      return json({ details: INVOICES.filter((i) => i.day >= from && i.day <= to).map((i) => ({
        invNum: i.invNum, sellerName: i.seller, amount: i.amount,
        invDate: i.byEpoch ? { time: taipeiMidnight(i.day) } : { year: Number(i.day.slice(0, 4)) - 1911, month: Number(i.day.slice(5, 7)), date: Number(i.day.slice(8, 10)) },
      })) });
    }
    if (url.endsWith('/query-invoices-details')) {
      fake.details++;
      const inv = INVOICES.find((i) => i.invNum === req.invNum);
      if (!inv || !inv.lines.length) return json({ message: 'unavailable' }, 500);
      return json({ details: inv.lines });
    }
    return json({ message: 'no such API' }, 404);
  }) as typeof fetch;
  return fake;
}
