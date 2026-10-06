/*
 * A pretend 集保 e存摺 API, in this process: the vendored client calls the
 * global fetch on one host, so the test swaps fetch for that host and leaves
 * everything else alone. Just enough of the app's API the connector uses:
 * sign-in with the encrypted fields decrypted as the app encrypts them, the
 * device check by email (and SMS when smsStep is set), holdings, a fund, one
 * settlement account and its movements, and one broker account's movements.
 * Password "wrong" fails; the codes are 246810 (email) and 135790 (SMS).
 */
import { createDecipheriv, randomUUID } from 'node:crypto';

const HOST = 'https://epassbooksys.tdcc.com.tw/MPSBKV2/rest/';
const APP_INFO = 'tw.com.tdcc.epassbook:3.3.8';
export const EMAIL_OTP = '246810';
export const SMS_OTP = '135790';
export const ACCOUNT = '0123456789012';

// The app's key: from the request's time stamp (its sequence) and device type.
function key(ts: string, devType: string): string {
  const chars = btoa(ts + APP_INFO + devType).split('');
  const out: string[] = [];
  for (let i = 0; i < chars.length && out.length < 32; i++) {
    out.push(chars[i % 2 === 0 ? i - Math.floor(i / 2) : chars.length - 1 - Math.floor(i / 2)] ?? '');
  }
  const k = out.join('');
  return k.length < 32 ? '*'.repeat(32 - k.length) + k : k;
}

function decrypt(value: string, ts: string, devType: string): string {
  const d = createDecipheriv('aes-256-cbc', Buffer.from(key(ts, devType)), Buffer.from('0000000000000000'));
  return Buffer.concat([d.update(Buffer.from(value, 'base64')), d.final()]).toString();
}

// One broker account's movements (TR002 rows), oldest last.
const row = (post: string, ser: string, symbol: string, name: string, txn: string, code: string, txnName: string, shares: string, price: string) =>
  [post, ser, symbol, name, '1', '', '', '', '', txn, code, txnName, shares, '', '', '', '', '', price, '', 'TWD'];
const TRADES = [
  row('20260922', '0004', '2330', '台積電', '20260920', 'S1', '賣出', '400', '600'),
  row('20260904', '0003', '2330', '台積電', '20260902', 'B1', '買進', '1,000', '580'),
  row('20260801', '0002', '0050', '元大台灣50', '20260801', 'X9', '神秘異動', '100', '1'),
  row('20260715', '0001', '2330', '台積電', '20260715', 'D1', '配股', '20', '1'),
];

export interface FakeTdcc {
  smsStep: boolean;
  emails: number;
  sms: number;
  logins: number;
  restore(): void;
}

export function installFakeTdcc(): FakeTdcc {
  const real = globalThis.fetch;
  const trusted = new Set<string>(); // device ids
  const tokens = new Map<string, string>(); // token -> device id
  const fake: FakeTdcc = { smsStep: false, emails: 0, sms: 0, logins: 0, restore: () => void (globalThis.fetch = real) };
  let emailVerified = new Set<string>();

  const reply = (body: unknown, code = '0000', token?: string) =>
    new Response(JSON.stringify({ responseHeader: { returnCode: code, returnMsg: code === '0000' ? 'OK' : 'refused', tokenID: token }, responseBody: body }),
      { headers: { 'content-type': 'application/json' } });

  globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input instanceof Request ? input.url : input);
    if (!url.startsWith(HOST)) return real(input, init);
    const endpoint = url.slice(HOST.length).split('?')[0];
    const { requestHeader: h, requestBody: b } = JSON.parse(String(init?.body ?? '{}'));
    const ts = atob(h.sequence);
    const field = (name: string) => decrypt(b[name], ts, h.devType);
    const device = h.devID as string;
    const signedIn = h.tokenID && tokens.get(h.tokenID) === device && trusted.has(device);

    switch (endpoint) {
      case 'CM001':
        return reply({ tokenID: `init-${randomUUID()}` });
      case 'AU001': {
        fake.logins++;
        if (field('loginCode') === 'wrong') return reply({}, 'A0101');
        const token = randomUUID();
        tokens.set(token, device);
        return reply({ isDiffDevice: trusted.has(device) ? 'N' : 'Y', isEmailValid: 'Y', tokenID: token }, '0000', token);
      }
      case 'AU013':
        fake.emails++;
        return reply({});
      case 'AU014':
        fake.sms++;
        return reply({});
      case 'AU015': {
        const sms = b.sendType === 'MOBILE';
        if (field('otp') !== (sms ? SMS_OTP : EMAIL_OTP)) return reply({}, 'V0011');
        if (!sms && fake.smsStep) {
          emailVerified.add(device);
          return reply({ isMobileValid: 'N' });
        }
        if (sms && !emailVerified.has(device)) return reply({}, 'V0011');
        trusted.add(device);
        emailVerified = new Set([...emailVerified].filter((d) => d !== device));
        return reply({ isMobileValid: 'Y' });
      }
    }
    if (!signedIn) return reply({}, 'D0006'); // a session that has gone stale

    switch (endpoint) {
      case 'TR001':
        return reply({
          accounts: [{
            brokerNo: '9800', brokerAccount: '1234567', brokerName: '元大證券',
            items: [['2330', '台積電', '', '', '', '', '', '620', '', '', '', '', '', '', '', '', '', '600', '', 'TWD', '', '20261005']],
          }],
          lastServerTime: '20261005120000',
        });
      case 'TR051V1':
        return reply({ fundDetails: [{ fundNo: 'B00123', fundCHName: '安聯台灣科技基金', fundSHR: '123.4560', currAlias: 'twd', saleOrgCode: 'A01' }], updateTime: '20261005' });
      case 'tsp/TSP006':
        return reply({ tspAccountInfos: [{ bankId: '013', tspAccount: [{ accountNo: ACCOUNT, accountType: '交割戶', currency: 'TWD', balanceAmt: '52,300', isShow: true }] }] });
      case 'tsp/TSP007':
        return reply({
          transactionDetails: [
            { txnDateTime: '20260904093000', transferOutAmount: '580826', summary: '股票交割' },
            { txnDateTime: '20260922093000', transferInAmount: '238500', summary: '股票交割' },
          ],
          pageToken: null, totalCount: 2,
        });
      case 'TR002':
        // Backfill: everything on the first page, then "no more".
        return b.txnSerNo ? reply({}, 'D0002') : reply({ brokerNo: b.brokerNo, brokerAccount: b.brokerAccount, items: TRADES });
    }
    return reply({}, 'X0404');
  }) as typeof fetch;
  return fake;
}
