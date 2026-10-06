/*
 * A pretend m.sinopac.com, just enough of what the vendored connector uses:
 * the mobile login page with its image CAPTCHA, and the app JSON API for
 * deposits (TWD and USD) and the card. Chrome reaches it over HTTPS
 * (--host-resolver-rules, a throwaway openssl certificate); the JSON API is
 * also served over plain HTTP for the fetch the connector makes, which
 * `fetch` below points here.
 *
 * The CAPTCHAs are real images from the bank's login page (test/captcha,
 * named by their digits; `noise` has none), served in the order of
 * `images`, then 008251 for ever. Password "wrong" fails; expireSessions() makes the bank forget
 * every sign-in.
 */
import { execFileSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { createServer as createHttp, type IncomingMessage, type ServerResponse } from 'node:http';
import { createServer as createHttps } from 'node:https';
import type { AddressInfo } from 'node:net';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

export const ACCOUNT = '0012345678909';
const HOST = 'https://m.sinopac.com';
// 'noise' is lines and no digits: an image the runner cannot read.
const image = (digits: string) =>
  readFileSync(join(import.meta.dirname, digits === 'noise' ? 'captcha-noise.jpg' : join('captcha', `${digits}.jpg`)));

const LOGIN = (error = '') => `<!doctype html><html><head><meta charset="utf-8"></head><body>
  <form id="m_login" method="post" action="/m/member/login/m_login.aspx">
    <p class="err">${error}</p>
    <input name="id" placeholder="ID"><input name="user" placeholder="User Code">
    <input name="pw" type="password" placeholder="Password">
    ${error ? '' /* the connector opens the page again for a new image */ : `<img name="imgCode" src="/Share/OnlineService/ValidateNumber.ashx?${Date.now()}" width="114" height="40">`}
    <input id="CheckValidateNumber" name="code" maxlength="6">
    <button id="MMA_Login" type="submit">登入</button>
  </form></body></html>`;

const ok = (body: unknown) => JSON.stringify(body);
const DEPOSITS = [{
  Header: 'SUCCESS',
  SubInfo: [
    { AcctValue: ACCOUNT, AcctText: '活期儲蓄存款', Curr: 'TWD', AvailBalInt: '52,300' },
    { AcctValue: ACCOUNT, AcctText: '外幣活期存款', Curr: 'USD', AvailBalInt: '1,250.50' },
  ],
}];
const TWD_DETAIL = [{
  Header: 'SUCCESS',
  RecordCount: '2',
  SubInfo: [
    { DataText1: '2026/10/01<br>09:15', DataText2: '2026/10/01', DataText3: '薪資', DataText4: '42,000', DataText5: '52,420' },
    { DataText1: '2026/10/02', DataText2: '', DataText3: 'ATM提款', DataText4: '-120', DataText5: '52,300', DataText8: '轉入 0098765432101234' },
    { DataText1: '2026/10/02', DataText2: '', DataText3: '信用卡款', DataText4: '-1,580', DataText5: '50,720' },
  ],
}];
const NOTHING = [{ Header: 'FAIL', Message: '查無資料', SubInfo: [] }];
const SUMMARY = [{
  Header: 'SUCCESS',
  CreditSum: [
    { DataText: '卡號', DataValue: '****4321' },
    { DataText: '永久信用額度', DataValue: '100,000' },
    { DataText: '剩餘可用額度', DataValue: '97,500' },
    { DataText: '本期應繳金額', DataValue: '2,500' },
    { DataText: '繳款截止日', DataValue: '115/10/20' },
    { DataText: '結帳日', DataValue: '115/10/05' },
  ],
}];
const card = (Result: unknown) => ok({ ResultCode: '00', ResultMessage: '成功', Result });
const LATEST = {
  Items: [
    // posted below as well: upstream keeps the posted one
    { AuthDate: '2026/09/28', AuthTime: '12:30', AuthAmt: '1,580', Memo: '台灣大哥大', CardNo: '****4321' },
    // not posted yet: a pending row
    { AuthDate: '2026/10/04', AuthTime: '18:05', AuthAmt: '350', Memo: '全聯福利中心', CardNo: '****4321' },
  ],
};
const OUTSTANDING = {
  Detail: [
    { TXDATE: '2026/09/28', DEDATE: '2026/09/29', AMT: '1,580', MEMO: '台灣大哥大', CurrencyCode: '000', CardNoLast4: '4321' },
    { TXDATE: '2026/09/30', DEDATE: '2026/10/01', AMT: '-200', MEMO: '退貨 全聯', CurrencyCode: '000', CardNoLast4: '4321' },
  ],
  SubTotal: [{ CurrencyCode: '000', SubTotalAmt: '1,380' }],
};
const ACCOUNTING = {
  BaseData: { STMTDATE: '20260905', DUEDATE: '20260920' },
  BillAmounts: [{ CurrencyCode: '000', CURRBAL: '2,500', DUEAMT: '1,000', TotalPaymentAmt: '2,500' }],
};

export interface FakeSinopac {
  chromeArgs: string;
  /** The connector's fetch, pointed at this bank. */
  fetch: typeof fetch;
  /** CAPTCHA images still to serve, by their digits. */
  images: string[];
  served: number;
  logins: number;
  expireSessions(): void;
  close(): Promise<void>;
}

export async function startFakeSinopac(): Promise<FakeSinopac> {
  const dir = mkdtempSync(join(tmpdir(), 'fake-sinopac-'));
  execFileSync('openssl', ['req', '-x509', '-newkey', 'ec', '-pkeyopt', 'ec_paramgen_curve:prime256v1', '-nodes', '-days', '1',
    '-subj', '/CN=m.sinopac.com', '-keyout', join(dir, 'key.pem'), '-out', join(dir, 'cert.pem')], { stdio: 'ignore' });
  const key = readFileSync(join(dir, 'key.pem'));
  const cert = readFileSync(join(dir, 'cert.pem'));
  rmSync(dir, { recursive: true });

  const answers = new Map<string, string>(); // page session -> CAPTCHA digits
  const signedIn = new Set<string>();
  const bank = { images: [] as string[], served: 0, logins: 0 };
  const cookie = (req: IncomingMessage, name: string) => new RegExp(`(?:^|;\\s*)${name}=([^;]+)`).exec(req.headers.cookie ?? '')?.[1];
  const send = (res: ServerResponse, status: number, body: string | Buffer, headers: Record<string, string | string[]> = {}) => {
    const type = Buffer.isBuffer(body) ? 'image/jpeg' : /^[[{]/.test(body) ? 'application/json' : 'text/html; charset=utf-8';
    res.writeHead(status, { 'content-type': type, ...headers });
    res.end(body);
  };
  const read = async (req: IncomingMessage) => {
    let raw = '';
    for await (const chunk of req) raw += chunk;
    return raw;
  };

  const handle = async (req: IncomingMessage, res: ServerResponse) => {
    const url = new URL(req.url ?? '/', HOST);
    const path = url.pathname;
    if (path === '/m/member/login/m_login.aspx') {
      const sid = cookie(req, 'ASP.NET_SessionId') ?? randomUUID();
      const set = { 'set-cookie': `ASP.NET_SessionId=${sid}; Path=/; Secure` };
      if (req.method !== 'POST') return send(res, 200, LOGIN(), set);
      bank.logins++;
      const f = new URLSearchParams(await read(req));
      if (f.get('code') !== answers.get(sid)) return send(res, 200, LOGIN('驗證碼錯誤，請重新輸入'), set);
      if (f.get('pw') === 'wrong') return send(res, 200, LOGIN('密碼錯誤，請重新輸入'), set);
      const token = randomUUID();
      signedIn.add(token);
      return send(res, 302, '', { location: '/m/m_home.aspx', 'set-cookie': [`MBSID=${token}; Path=/; Secure; HttpOnly`] });
    }
    if (path === '/Share/OnlineService/ValidateNumber.ashx') {
      const digits = bank.images.shift() ?? '008251';
      bank.served++;
      answers.set(cookie(req, 'ASP.NET_SessionId') ?? '', digits);
      return send(res, 200, image(digits), { 'cache-control': 'no-store' });
    }
    if (path === '/m/m_home.aspx') return send(res, 200, '<!doctype html><meta charset="utf-8"><h1>永豐行動銀行</h1>');

    // The app JSON API: a forgotten sign-in answers TIMEOUT, as the bank does.
    const token = cookie(req, 'MBSID');
    const body = await read(req);
    if (!token || !signedIn.has(token)) {
      return send(res, 200, path.startsWith('/m/SinoCard/') ? ok({ ResultCode: '99', ResultMessage: '登入逾時' }) : ok([{ Header: 'TIMEOUT', Message: '登入逾時' }]));
    }
    if (path === '/ws/bank/bankbal/ws_bankbal.ashx') return send(res, 200, ok(DEPOSITS));
    if (path === '/ws/bank/transdetail/ws_transdetailMerge.ashx') {
      return send(res, 200, ok(new URLSearchParams(body).get('Curr') === 'TWD' ? TWD_DETAIL : NOTHING));
    }
    if (path === '/ws/card/cardqry/ws_cardsum.ashx') return send(res, 200, ok(SUMMARY));
    if (path === '/ws/card/cardqry/ws_cardbilling_sp.ashx') return send(res, 200, ok([{ Header: 'SUCCESS', CreditDetail: [] }]));
    if (path === '/m/SinoCard/api/security/sso') return send(res, 200, card({ ID: 'A123456789' }));
    if (path === '/m/SinoCard/api/security/auth') return send(res, 200, card({}));
    if (path === '/m/SinoCard/api/Accounting/LatestTx') return send(res, 200, card(LATEST));
    if (path === '/m/SinoCard/api/Accounting/OutstandingDetail') return send(res, 200, card(OUTSTANDING));
    if (path === '/m/SinoCard/api/accounting/accountinginfo') return send(res, 200, card(ACCOUNTING));
    send(res, 404, 'not here');
  };
  const onRequest = (req: IncomingMessage, res: ServerResponse) => void handle(req, res).catch((err) => send(res, 500, String(err)));

  const https = createHttps({ key, cert }, onRequest);
  const http = createHttp(onRequest);
  await Promise.all([https, http].map((s) => new Promise<void>((r) => s.listen(0, '127.0.0.1', r))));
  const httpsPort = (https.address() as AddressInfo).port;
  const httpPort = (http.address() as AddressInfo).port;
  return {
    chromeArgs: `"--host-resolver-rules=MAP m.sinopac.com 127.0.0.1:${httpsPort}" --ignore-certificate-errors`,
    fetch: (input, init) => fetch(String(input instanceof Request ? input.url : input).replace(HOST, `http://127.0.0.1:${httpPort}`), init),
    get images() { return bank.images; },
    set images(v) { bank.images = v; },
    get served() { return bank.served; },
    get logins() { return bank.logins; },
    expireSessions: () => signedIn.clear(),
    close: async () => {
      for (const s of [https, http]) s.closeAllConnections();
      await Promise.all([https, http].map((s) => new Promise<void>((r) => s.close(() => r()))));
    },
  };
}
