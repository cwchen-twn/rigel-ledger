/*
 * A pretend www.cathaybk.com.tw, just enough of the pages and calls the
 * vendored connector uses: the login form, the SMS code and trusted device,
 * one deposit account with its transactions (picked on the detail page's
 * account and period comboboxes), one USD account, and one credit card
 * with a statement. Chrome is pointed here with --host-resolver-rules, over HTTPS
 * with a throwaway certificate (openssl). Password "wrong" fails; the code
 * is 123456.
 */
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import type { IncomingMessage, ServerResponse } from 'node:http';
import { createServer } from 'node:https';
import type { AddressInfo } from 'node:net';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

export const ACCOUNT = '012345678901';
export const FOREIGN = '098765432109';
export const OTP = '123456';

const page = (body: string, script = '') =>
  `<!doctype html><html><head><meta charset="utf-8"></head><body>${body}<script>${script}</script></body></html>`;

const LOGIN = page(
  `<form id="f" method="post" action="/MyBank/login">
     <input id="CustID" name="cust"><input id="UserIdKeyin" name="user"><input id="PasswordKeyin" name="pw" type="password">
     <button type="button" class="js-login">登入</button>
   </form>`,
  `window.NormalDataCheck = () => { document.getElementById('f').submit(); return true; };`,
);

const VERIFY = page(
  `<p>Email驗證 簡訊驗證</p>
   <a class="js-otp-change-view">簡訊</a>
   <button id="js-otp-send" type="button">發送簡訊驗證碼</button>
   <button id="js-otp-email-send" type="button" style="display:none">發送Email驗證碼</button>
   <form class="js-otp-view" style="display:none">
     <p id="err"></p>
     <input name="otp" maxlength="6" placeholder="請輸入驗證碼後6位數字">
     <button type="submit">確定</button>
   </form>`,
  // Like the bank: the code is checked in the page, which moves on only once it is right.
  `const form = document.querySelector('.js-otp-view');
   document.getElementById('js-otp-send').onclick = () => {
     form.style.display = 'block';
     fetch('/MyBank/SendOtp', { method: 'POST' });
   };
   form.otp.oninput = () => { document.getElementById('err').textContent = ''; };
   form.onsubmit = (e) => {
     e.preventDefault();
     fetch('/MyBank/otp', { method: 'POST', body: new URLSearchParams(new FormData(form)) }).then((r) => {
       if (r.ok) location.href = '/OnlineBanking/Home';
       else document.getElementById('err').textContent = '驗證碼錯誤';
     });
   };`,
);

const DEPOSITS = page(`<h1>存款總覽</h1><div class="row">臺幣活存 <button type="button">${ACCOUNT}</button> $40,300 $40,000</div>`,
  `document.querySelector('button').onclick = () => { location.href = '/OnlineBanking/AcctInq/B0103'; };`);

// Like the bank's detail page: an account and a period picker (comboboxes
// whose control shows the choice), a query on load for 30 days, and 查詢.
const DETAIL = page(`<h1>交易明細</h1>
  <div class="ctl" data-kind="account"><span class="val">${ACCOUNT}</span><input role="combobox"></div>
  <div class="ctl" data-kind="period"><span class="val">近 30 天</span><input role="combobox"></div>
  <ul id="menu"></ul><button type="button" id="q">查詢</button>`,
  `const OPTIONS = { account: ['${ACCOUNT}'], period: ['近 30 天', '近 90 天', '近 1 年'] };
   const DAYS = { '近 30 天': 30, '近 90 天': 90, '近 1 年': 365 };
   const val = (kind) => document.querySelector('[data-kind="' + kind + '"] .val').textContent;
   const day = (d) => d.toISOString().slice(0, 10);
   const q = () => {
     const end = new Date();
     const start = new Date(end.getTime() - (DAYS[val('period')] - 1) * 86400000);
     return fetch('/OnlineBankingApi/Acct/B_ACCT_Q_TransferDetail', { method: 'POST', headers: { 'content-type': 'application/json' },
       body: JSON.stringify({ content: { queryFilters: [{ accountNumber: '0000' + val('account'), startDate: day(start), endDate: day(end) }] } }) });
   };
   for (const ctl of document.querySelectorAll('.ctl')) {
     ctl.querySelector('input').onkeydown = (e) => {
       if (e.key !== 'ArrowDown') return;
       const menu = document.getElementById('menu');
       menu.innerHTML = '';
       for (const o of OPTIONS[ctl.dataset.kind]) {
         const li = document.createElement('li');
         li.setAttribute('role', 'option');
         li.textContent = o;
         li.onclick = () => { ctl.querySelector('.val').textContent = o; menu.innerHTML = ''; };
         menu.append(li);
       }
     };
   }
   q(); document.getElementById('q').onclick = q;`);

const FOREIGN_PAGE = page('<h1>外幣存款總覽</h1>',
  `fetch('/OnlineBankingApi/FAcct/R_ACCT_Q_OverView', { method: 'POST' });`);

const CARD = page(`<h1>信用卡總覽</h1><script>fetch('/OnlineBankingApi/Com/C_COM_Q_CardStatus', { method: 'POST' });</script><p>國泰世華 CUBE VISA 卡</p><p>卡片末四碼：4321</p><p>永久信用額度 TWD 50,000</p>
  <p>剩餘可用額度 TWD 48,420</p><p>繳款截止日 2026/10/20</p><p>應繳金額 TWD 1,380</p>`);

const TRANSFERS = {
  returnCode: '0000',
  content: {
    datas: [{
      accountNumber: `0000${ACCOUNT}`,
      queryStatus: 'Success',
      details: [
        { txnDateTime: '2026/09/30 09:15:00', description: '薪資', incomeAmt: 42000, expendAmt: 0 },
        { txnDateTime: '2026/09/30 12:01:00', description: '7-ELEVEN', incomeAmt: 0, expendAmt: 120 },
        { txnDateTime: '2026/09/30 18:40:00', description: '7-ELEVEN', incomeAmt: 0, expendAmt: 120 },
        { accountDate: '2026/10/01', description: '信用卡款', incomeAmt: 0, expendAmt: 1580 },
      ],
    }],
  },
};

const BILL = {
  content: {
    twdBillDetailInfo: [
      { detailType: 'LastBillAmount', tradeData: [{ consumeDate: null, transDesc: '上期帳單', amount: 999, currency: 'TWD' }] },
      { detailType: 'Consume', tradeData: [
        { consumeDate: '2026/09/02', transDesc: '全聯福利中心', amount: 1580, currency: 'TWD' },
        { consumeDate: '2026/09/05', transDesc: '退貨 全聯', amount: -200, currency: 'TWD' },
      ] },
    ],
  },
};

export interface FakeCathay {
  port: number;
  chromeArgs: string;
  logins: number;
  codesSent: number;
  loggedOut: number;
  close(): Promise<void>;
}

export async function startFakeCathay(): Promise<FakeCathay> {
  const dir = mkdtempSync(join(tmpdir(), 'fake-cathay-'));
  execFileSync('openssl', ['req', '-x509', '-newkey', 'ec', '-pkeyopt', 'ec_paramgen_curve:prime256v1', '-nodes', '-days', '1',
    '-subj', '/CN=www.cathaybk.com.tw', '-keyout', join(dir, 'key.pem'), '-out', join(dir, 'cert.pem')], { stdio: 'ignore' });
  const key = readFileSync(join(dir, 'key.pem'));
  const cert = readFileSync(join(dir, 'cert.pem'));
  rmSync(dir, { recursive: true });

  const state = { logins: 0, codesSent: 0, loggedOut: 0 };
  const send = (res: ServerResponse, status: number, body: string, headers: Record<string, string | string[]> = {}) => {
    res.writeHead(status, { 'content-type': body.startsWith('{') ? 'application/json' : 'text/html; charset=utf-8', ...headers });
    res.end(body);
  };
  const form = async (req: IncomingMessage) => {
    let raw = '';
    for await (const chunk of req) raw += chunk;
    return new URLSearchParams(raw);
  };
  const trusted = (req: IncomingMessage) => /CUB\.eBank\.DeviceId=trusted/.test(req.headers.cookie ?? '');
  const signedIn = (req: IncomingMessage) => /session=in/.test(req.headers.cookie ?? '');
  const home = (res: ServerResponse) => send(res, 303, '', { location: '/OnlineBanking/Home', 'set-cookie': 'session=in; Path=/; Secure' });

  const server = createServer({ key, cert }, async (req, res) => {
    const path = new URL(req.url ?? '/', 'https://www.cathaybk.com.tw').pathname;
    if (path === '/MyBank/' && req.method === 'GET') return send(res, 200, LOGIN);
    if (path === '/MyBank/login') {
      state.logins++;
      const f = await form(req);
      if (f.get('pw') === 'wrong') return send(res, 200, page('<p>登入失敗，請重新輸入</p>'));
      return trusted(req) ? home(res) : send(res, 200, VERIFY);
    }
    if (path === '/MyBank/SendOtp') {
      state.codesSent++;
      return send(res, 200, '{}');
    }
    if (path === '/MyBank/otp') {
      const f = await form(req);
      if (f.get('otp') !== OTP) return send(res, 400, '{}');
      return send(res, 200, '{}', {
        // Not HttpOnly: the connector looks for it in document.cookie.
        'set-cookie': ['CUB.eBank.DeviceId=trusted; Domain=.cathaybk.com.tw; Path=/; Secure; Max-Age=31536000', 'session=in; Path=/; Secure'],
      });
    }
    if (path === '/OnlineBanking/Logout/Index') {
      state.loggedOut++;
      return send(res, 200, page('<p>已登出</p>'), { 'set-cookie': 'session=; Path=/; Max-Age=0' });
    }
    if (!signedIn(req)) return send(res, 302, '', { location: '/MyBank/' });
    if (path === '/OnlineBanking/Home') return send(res, 200, page('<h1>網路銀行</h1>'));
    if (path === '/OnlineBanking/AcctInq/B0101_DepInq') return send(res, 200, DEPOSITS);
    if (path === '/OnlineBanking/AcctInq/B0103') return send(res, 200, DETAIL);
    if (path.endsWith('/B_ACCT_Q_TransferDetail')) return send(res, 200, JSON.stringify(TRANSFERS));
    if (path === '/OnlineBanking/FAcctInq/R0101_FDepInq') return send(res, 200, FOREIGN_PAGE);
    if (path.endsWith('/R_ACCT_Q_OverView')) {
      return send(res, 200, JSON.stringify({ returnCode: '0000', content: { isGetDemandAccountSuccess: true,
        demandAccounts: [{ account: FOREIGN, details: [{ currencyCode: 'USD', balance: '1234.5' }] }] } }));
    }
    if (path.endsWith('/C_COM_Q_CardStatus')) return send(res, 200, JSON.stringify({ returnCode: '0000', content: { cardStatus: 'Valid' } }));
    if (path === '/OnlineBanking/CQuery/C0101_BillOverview') return send(res, 200, CARD);
    if (path === '/OnlineBanking/CQuery/C0102_BillInq') return send(res, 200, page('<h1>帳單查詢</h1>'));
    if (path === '/MyBank/Customized/GetJWT') return send(res, 200, JSON.stringify({ Data: { JwtToken: 'jwt', CustomerId: 'c1' } }));
    if (path.endsWith('/C_BILL_Q_HistoryBillList')) {
      return send(res, 200, JSON.stringify({ content: { historyBillInfoList: [{ billDate: '2026-09-15', twdAmount: 1380, usdAmount: null, billStatus: 'N' }] } }));
    }
    if (path.endsWith('/C_BILL_Q_RecentBillDetail')) return send(res, 200, JSON.stringify(BILL));
    send(res, 404, page('not here'));
  });
  await new Promise<void>((r) => server.listen(0, '127.0.0.1', r));
  const { port } = server.address() as AddressInfo;
  return {
    port,
    chromeArgs: `"--host-resolver-rules=MAP www.cathaybk.com.tw 127.0.0.1:${port}" --ignore-certificate-errors`,
    get logins() { return state.logins; },
    get codesSent() { return state.codesSent; },
    get loggedOut() { return state.loggedOut; },
    close: () => new Promise<void>((r) => { server.closeAllConnections(); server.close(() => r()); }),
  };
}
