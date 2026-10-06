# rigel-sync

A person's sync runner (P4c-2): every institution RigelLedger syncs, whatever
the country or kind -- banks, cards, brokers, exchanges. Each person runs their
own and links it to RigelLedger under **Settings -> Sync runner**, the way a
self-hosted CI runner is registered: on a computer they leave on, or (the
owner's) on the server next to the app. It syncs that person's connections and
nobody else's. Design: `docs/ARCHITECTURE.md`, "One runner per person".

What it does, in a loop: register its X25519 public key and the connectors it
offers, claim one due connection, open its credentials (sealed in the browser
to this runner's key -- `src/sealing.ts`, the same format as
`internal/sealing`), run the connector, raise a challenge in the app when the
institution asks for a one-time code or a CAPTCHA, send the rows to the
connection's import queue, and finish the run `ok` or `failed` with a stable
code. Credentials and answers exist in the clear only in this process.

## Running it

With Docker: Settings -> Sync runner -> Link a runner shows the compose file
(image `ghcr.io/cwchen-twn/rigel-ledger-sync`, also on
`git.chenantunez.com/cwchen-twn/rigel-ledger-sync`; linux/amd64, which Apple
silicon runs under Rosetta). The image carries Node, Chromium and the CAPTCHA
model; its key and state live in the `/data` volume. A connector can be tried
in it too:

```bash
docker run --rm -it --platform linux/amd64 -v rigel-sync:/data ghcr.io/cwchen-twn/rigel-ledger-sync try tw-cathaybk
```

`Dockerfile` here builds it (`docker build -t rigel-ledger-sync .`); the `image`
workflows push it on `main` and on tags, with the app's tags.

Without Docker, Node 22.18 or later runs the TypeScript in `src/` directly (type stripping);
there is no build step. The bank connectors drive a real browser, so Chrome or
Chromium must be installed (`puppeteer-core` does not download one).

```bash
bun install --production
RIGEL_URL=https://ledger.chenantunez.com RUNNER_TOKEN=... DATA_DIR=./data node src/main.ts run
```

| Variable | |
|---|---|
| `RIGEL_URL` | the app |
| `RUNNER_TOKEN` | from Settings -> Sync runner -> Link a runner, or `rigel-ledger-cli create-runner-token -u USER` |
| `DATA_DIR` | default `/data`: the runner's private key (`runner.key`) and each connection's session state (`state/conn-<id>.json`: cookies, a trusted device). Both 0600; neither ever reaches the app. Keep this directory: losing it means every connection asks for its credentials again. `models/` holds the CAPTCHA model once downloaded |
| `POLL_SECONDS` | how often to look for due connections, default 30 |
| `CHROME_PATH` | the browser; by default the first Chrome or Chromium found in the usual places |
| `CHROME_ARGS` | more Chrome flags, space-separated, `"quoted"` when one holds a space; e.g. `--proxy-server=socks5://...` to sign in to the banks from a Taiwan address |
| `RIGEL_SYNC_HEADFUL=1` | show the browser window, to watch a connector at work |
| `RIGEL_SYNC_NO_SANDBOX=1` | start Chrome without its sandbox (always as root). The image sets it: a container's default seccomp profile denies the user namespaces the sandbox needs |
| `RIGEL_SYNC_OCR_MODEL` | a copy of the CAPTCHA model already on disk (an offline install); by default it is downloaded once into `DATA_DIR/models` |
| `RIGEL_SYNC_FAKE=1` | also offer the pretend institution `fake` (development only) |

`node src/main.ts run --once` claims and runs once, then exits.

## Institutions

Connector ids are `<country>-<institution>` (ISO country code, lower case), and
each connector lives in `src/connectors/<country>/`.

| id | | fields | notes |
|---|---|---|---|
| `tw-cathaybk` | 國泰世華 | 身分證字號, 用戶代號, password, codes by SMS or email | deposits (90 days) and the credit card (3 statements). The first run asks for a one-time code; the bank then trusts this runner's browser, and later runs ask nothing. Account ids keep the last four digits only |
| `tw-sinopac` | 永豐銀行 | 身分證字號, 使用者代碼, password | deposits in every currency (90 days) and the credit card (posted, and pending authorisations). Sign-in asks for a six-digit image CAPTCHA, which the runner reads itself (below); the session is kept and reused while the bank accepts it. When three images in a row are not read, the image goes to Connections ("Needs you") |
| `tw-tdcc` | 集保 e存摺 | 身分證字號, e存摺 password | every broker account's holdings (shares, ETFs, funds), its movements (paged back over runs), and the settlement (交割) bank accounts with their balances and movements. The app's API, no browser. The first sign-in sends a code by email (sometimes then by SMS); the runner is then a trusted device. A trade's direction comes from its name (買進, 賣出, 配股, ...); a kind not recognised is logged (`集保 movements of a kind not recognised`) and left out. 集保 sends no cash for a trade: the queue asks for it |
| `tw-einvoice` | 電子發票 | mobile number, carrier password (載具驗證碼) | the mobile barcode carrier's invoices for the last two periods (four months) and their lines, as `invoice` rows: each adds its lines to the payment it matches, and only one nothing paid for is booked, as a cash purchase. The e-invoice app's API, no browser and no code; its session is kept and reused. An invoice whose lines cannot be read still comes, as one line of its total |

## Trying a connector without the app

```bash
DATA_DIR=./data node src/main.ts try fake
```

asks for the connector's fields on the terminal (secrets are not echoed),
answers one-time codes and CAPTCHAs there too (a CAPTCHA image is written to
`DATA_DIR`), and prints the batch it would send as JSON. Its state is kept as
`state/try-<connector>.json`, so a second try runs as a trusted device. This is
how a real login is checked from a laptop and from the server before a
connection is made in the app.

## Developing

```bash
bun install
bun run check        # tsc --noEmit, then node --test
```

`make rigel-sync/check` does the same from the repo root; CI runs it as the
`rigel-sync` job on both forges.

A connector (`src/connectors/types.ts`) gets its credentials, its saved
state, `saveState`, `ask` (OTP, CAPTCHA, device check) and an abort signal,
and returns one batch: accounts plus `transaction` and `balance` rows, amounts
as decimal strings (`src/money.ts` turns a JS number into one exactly, or
throws). A failure the person should see is a `SyncError` with a stable code;
anything else is reported as `connector_error` and logged here only.

### all-set-tw, vendored

The bank connectors are [all-set-tw](https://github.com/TedLin1993/all-set-tw)'s
(MIT), copied unedited at the commit `vendor/all-set-tw/UPSTREAM` names. Upstream
is TypeScript for Cloudflare Workers, which Node cannot run as it is, so
`scripts/vendor.ts` bundles each connector (Bun) into `vendor/all-set-tw/<id>.js`,
with `@cloudflare/puppeteer` pointed at `src/browser/cloudflare.ts`: a local Chrome
that keeps running while the connector waits for a one-time code.

```bash
bun scripts/vendor.ts <commit>    # move to a newer upstream; review the diff of vendor/
bun run vendor:check              # the bundles are what the copies give (CI)
```

`src/connectors/<id>.ts` wraps one: our fields, the OTP steps through `ctx.ask`,
the trusted-device state, and the mapping to rows. Its test drives the whole flow
against a pretend bank site in Chrome (`test/fake-cathay.ts`, `test/fake-sinopac.ts`);
CI installs Chrome for it, and elsewhere it is skipped when no Chrome is found.
npm packages the vendored code imports (zod, jpeg-js, node-forge) are left as
imports, pinned in `package.json` at the versions upstream's lockfile resolves.

### CAPTCHAs

`src/ocr/captcha.ts` reads numeric CAPTCHAs in the runner, so a bank's image
check needs the person only when the model fails: ddddocr's `common_old.onnx`
([sml2h3/ddddocr](https://github.com/sml2h3/ddddocr), MIT, 13 MB), downloaded
once from a pinned commit and checked against its sha256, run by
`onnxruntime-web` (WebAssembly, so the same on every architecture). Nothing
leaves the runner. On 11 real 永豐 images in `test/captcha` it reads 9, and both
misses come out short, so they are caught before the bank sees them and a new
image is fetched.

`fake` mirrors `integrations/fake-runner`: password `wrong` fails with
`bad_credentials`, password `otp` asks for a code (123456) and is trusted
afterwards.
