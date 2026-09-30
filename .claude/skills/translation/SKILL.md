---
name: translation
description: How to write and check rigel-ledger's UI and mail translations (en, zh = Traditional Chinese as used in Taiwan, es) - the files and key rules, the words Taiwanese users actually say (token, not 權杖), Taiwan vs mainland vocabulary, the fixed glossary for ledger terms, spacing and punctuation, and the check that the three files stay identical. Use when adding or changing any string in web/src/i18n/*.json or internal/mail/templates/.
---

# Translations (rigel-ledger)

Three languages, same keys: `web/src/i18n/{en,zh,es}.json` (UI) and
`internal/mail/templates/{en,zh,es}/*.tmpl` (mail). **zh is Traditional Chinese as
written in Taiwan (zh-TW)**, for Taiwanese users -- not a character conversion of
mainland Chinese. The database stores no translations (CLAUDE.md, Key Conventions).

## The rule that caused a fix: say what users say, not what the dictionary says

A literal dictionary equivalent can be correct and still unreadable. **"token" was
translated 權杖 (a sceptre); no Mandarin speaker reads that as a sign-in or API
token** (fixed in 46ac405). Taiwanese software users say the English word. Before
choosing a Chinese term for anything technical, ask: *would a Taiwanese user say
this word out loud?* If they would say the English word, keep it English.

Keep in English (inside zh strings):

| Keep | Not |
|---|---|
| token, API token | 權杖, 令牌, 代幣 |
| API, CSV, PDF, OTP, TOTP, SMTP, URL, IP | 應用程式介面, 逗號分隔值 ... |
| Docker, App (as in 手機 App) | 應用 |
| passkey is 通行金鑰 (Apple/Google use it in zh-TW); CAPTCHA is 驗證圖片/圖形驗證碼 | |

When unsure, see what Apple, Google, LINE or a Taiwanese bank app calls it in
zh-TW, and prefer that.

## Taiwan, not mainland, vocabulary

| zh-TW (use) | mainland (never) | meaning |
|---|---|---|
| 帳號 | 賬號/賬戶 | a login account |
| 登入 / 登出 | 登錄 / 退出 | sign in / out |
| 預設 | 默認 | default |
| 設定 | 設置 | settings |
| 伺服器 | 服務器 | server |
| 程式 | 程序 | program (同步程式 = the sync runner) |
| 資料 | 數據 | data |
| 網路 | 網絡 | network |
| 軟體 | 軟件 | software |
| 檔案 | 文件 | file |
| 信箱 / 電子郵件 | 郵箱 | email address |
| 訊息 | 信息 | message |
| 影片 | 視頻 | video |

Traditional characters throughout (帳, 據, 裡, 為, 與), never simplified.

## The ledger glossary (keep consistent across the app)

| en | zh | es |
|---|---|---|
| book | 帳本 | libro |
| account (in the chart of accounts) | 科目 | cuenta |
| bank / institution account | 銀行帳戶 / 帳戶 | cuenta bancaria |
| login account (the person's) | 帳號 | cuenta |
| transaction | 交易 | transacción / movimiento (bank rows) |
| review queue / Imports | 待審清單 / 匯入 | cola de revisión / Importaciones |
| Connections | 連結帳戶 | Conexiones |
| sync runner | 同步程式 | sincronizador |
| token | token | token |
| base currency | 本位幣 | moneda base |

**帳戶 vs 帳號 vs 科目 are three different things** -- a bank account, a login account,
a line in the chart of accounts. Pick by meaning, not by the English word "account".

## Form

- **A half-width space between Chinese and Latin letters or digits**: 新增 token,
  匯入 CSV, 6 位數驗證碼, 10 分鐘. No space next to full-width punctuation: 「token」.
  (Every existing zh string follows this.)
- **Full-width punctuation in zh**: ，。：；？！「」（）. Quotes are 「」, not "".
  Ellipsis is …, as in en.
- **Placeholders stay exactly as in en**: `{count}`, `{name}`, `{when}` -- never
  translated, never removed. There are no plural forms (index.tsx only substitutes),
  so phrase counts neutrally: "Accepted: {count}", 已接受：{count} 筆, not "1 rows".
- **es uses tú** (tus, conecta, elige), as the existing strings do, never usted.
- **Spanish variant: not yet decided.** The strings mix Spain words (ordenador, 5
  strings) with neutral ones; the owner lives in Paraguay, where people say
  computadora. Ask the owner before "fixing" it one way or the other, then record the
  decision here.
- Currency and dates are formatted in code (Intl, `formatDate`), not in strings.

## Workflow

1. Add the key to **all three** files with the same path; en first, then zh and es
   written for their readers (not word-for-word from en).
2. Check the three key sets are identical:

   ```bash
   cd web/src/i18n && python3 -c "
   import json
   def keys(n,p=''):
       return {x for k,v in n.items() for x in (keys(v,p+k+'.') if isinstance(v,dict) else {p+k})}
   e,z,s=[keys(json.load(open(f+'.json'))) for f in ('en','zh','es')]
   print(len(e), e==z==s, sorted(e^z)[:5], sorted(e^s)[:5])"
   ```

3. Search zh for the words this file forbids before committing:
   `grep -nE '權杖|令牌|默認|服務器|設置|數據|網絡|軟件|賬' web/src/i18n/zh.json`
   should print nothing.
4. Error codes are translated as `error.<code>`, field codes as `field.<code>`, sign-in
   events as `event.<name>` (an event without a key shows its raw name); add them
   when adding a new code in Go.
5. Look at a new string in the running app in zh as well as en: long zh or es text
   in a narrow button or badge wraps differently.
