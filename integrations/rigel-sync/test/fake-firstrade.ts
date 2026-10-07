/*
 * A pretend Firstrade app API, in this process: the global fetch is swapped
 * for ORIGIN and left alone otherwise. Just what the connector uses: sign-in
 * with each second factor (PIN 2468, the authenticator SECRET, or the email
 * code 135790), the remembered session, one account's list, balance,
 * positions and history. Password "wrong" fails. Every figure is invented;
 * numbers are sent as JSON numbers, as Firstrade does.
 */
import { ACCESS_TOKEN, ORIGIN } from '../src/connectors/us/firstrade.ts';
import { totp } from '../src/util/totp.ts';

export const SECRET = 'JBSWY3DPEHPK3PXP';
export const EMAIL_CODE = '135790';
export const PIN = '2468';
const FTAT = 'ftat-remembered';

export interface FakeFirstrade {
  mode: 'app' | 'email' | 'pin';
  calls: string[];
  /** The history ranges asked for, "from..to". */
  ranges: string[];
  restore(): void;
}

// Raw JSON: the numbers must reach the connector as Firstrade writes them.
const HISTORY = [
  { date: '2026-09-15', json: '{"report_date":"2026-09-15","trans_str":"BUY","quantity":15,"trade_price":39.8,"amount":-597.00,"description":"APPLE INC  UNSOLICITED","symbol":"AAPL","account_type":"Cash"}' },
  { date: '2026-09-22', json: '{"report_date":"09/22/2026","trans_str":"SELL","quantity":-5,"trade_price":41.07,"amount":205.33,"description":"APPLE INC","symbol":"AAPL","account_type":"Cash"}' },
  { date: '2026-09-25', json: '{"report_date":"2026-09-25","trans_str":"Dividend","quantity":0,"trade_price":0,"amount":1.23,"description":"APPLE INC CASH DIV","symbol":"AAPL","account_type":"Cash"}' },
  { date: '2026-09-25', json: '{"report_date":"2026-09-25","trans_str":"Other","quantity":0,"trade_price":0,"amount":-0.37,"description":"NON-RESIDENT ALIEN TAX","symbol":"AAPL","account_type":"Cash"}' },
  { date: '2026-09-01', json: '{"report_date":"2026-09-01","trans_str":"Deposit","quantity":0,"trade_price":0,"amount":1000000.07,"description":"ACH DEPOSIT","symbol":"","account_type":"Cash"}' },
  { date: '2026-09-16', json: '{"report_date":"2026-09-16","trans_str":"BUY","quantity":1,"trade_price":1.05,"amount":-105.65,"description":"CALL ABCD","symbol":"ABCD260116C00003000","account_type":"Cash"}' },
  { date: '2026-09-18', json: '{"report_date":"2026-09-18","trans_str":"Split","quantity":10,"trade_price":0,"amount":0,"description":"STOCK SPLIT","symbol":"VTI","account_type":"Cash"}' },
  { date: '2025-03-03', json: '{"report_date":"2025-03-03","trans_str":"BUY","quantity":0.5,"trade_price":250.1,"amount":-125.05,"description":"VANGUARD TOTAL","symbol":"VTI","account_type":"Cash"}' },
];

export function installFakeFirstrade(mode: FakeFirstrade['mode']): FakeFirstrade {
  const real = globalThis.fetch;
  const fake: FakeFirstrade = { mode, calls: [], ranges: [], restore: () => void (globalThis.fetch = real) };
  const json = (body: string, status = 200) => new Response(body, { status, headers: { 'Content-Type': 'application/json' } });
  globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
    const url = new URL(String(input instanceof Request ? input.url : input));
    if (url.origin !== ORIGIN) return real(input, init);
    const headers = new Headers(init?.headers);
    const form = new URLSearchParams(String(init?.body ?? ''));
    fake.calls.push(url.pathname);
    if (headers.get('access-token') !== ACCESS_TOKEN) return json('{"error":"bad app"}', 403);
    switch (url.pathname) {
      case '/sess/login':
        if (form.get('password') === 'wrong') return json('{"error":"Invalid username or password"}');
        if (headers.get('ftat') === FTAT) return json(`{"ftat":"${FTAT}","sid":"s1","error":""}`);
        if (fake.mode === 'email') return json('{"error":"","t_token":"T","otp":[{"channel":"email","recipientMask":"a****@example.com","recipientId":"R1"}]}');
        return json(`{"error":"","t_token":"T"${fake.mode === 'app' ? ',"mfa":true' : ''}}`);
      case '/sess/request_code':
        return json(form.get('recipientId') === 'R1' && form.get('t_token') === 'T' ? '{"error":"","verificationSid":"V1"}' : '{"error":"no"}');
      case '/sess/verify_pin': {
        const ok = form.get('t_token') === 'T' && form.get('remember_for') === '30' &&
          (form.get('pin') === PIN || form.get('mfaCode') === totp(SECRET) || (form.get('otpCode') === EMAIL_CODE && form.get('verificationSid') === 'V1'));
        return json(ok ? `{"error":"","ftat":"${FTAT}","sid":"s2"}` : '{"error":"Invalid code"}');
      }
    }
    if (headers.get('ftat') !== FTAT) return json('{"error":"Unauthorized"}', 401);
    switch (url.pathname) {
      case '/private/userinfo':
        return json('{"accounts":["90001234"],"authenticated":true}');
      case '/private/acct_list':
        return json('{"statusCode":200,"error":"","items":[{"account":"90001234","alias":"","type":"Individual","total_value":12345.67}]}');
      case '/private/balances':
        return json('{"statusCode":200,"error":"","result":{"account":"90001234","cash_balance":1000478.5,"total_account_value":1001234.56}}');
      case '/private/positions':
        return json('{"statusCode":200,"error":"","items":[' +
          '{"symbol":"AAPL","quantity":10,"last":39.63,"sec_type":1,"company_name":"Apple Inc."},' +
          '{"symbol":"VTI","quantity":0.5,"last":251.2,"sec_type":1,"company_name":"Vanguard Total Stock Market ETF"},' +
          '{"symbol":"ABCD260116C00003000","quantity":1,"last":0.9,"sec_type":2,"company_name":"ABCD Inc."}]}');
      case '/private/account_history': {
        const [from, to] = url.searchParams.getAll('range_arr[]');
        fake.ranges.push(`${from}..${to}`);
        const got = HISTORY.filter((h) => h.date >= from && h.date <= to).map((h) => h.json);
        return json(`{"statusCode":200,"error":"","items":[${got.join(',')}],"per_page":1000,"page":1}`);
      }
    }
    return json('{"error":"not found"}', 404);
  }) as typeof fetch;
  return fake;
}
