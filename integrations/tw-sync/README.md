# tw-sync

A person's sync runner for Taiwan institutions (P4c-2). Each person runs their
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

Node 22.18 or later runs the TypeScript in `src/` directly (type stripping);
there is no build step and no runtime dependency.

```bash
RIGEL_URL=https://ledger.chenantunez.com RUNNER_TOKEN=... DATA_DIR=./data node src/main.ts run
```

| Variable | |
|---|---|
| `RIGEL_URL` | the app |
| `RUNNER_TOKEN` | from Settings -> Sync runner -> Link a runner, or `rigel-ledger-cli create-runner-token -u USER` |
| `DATA_DIR` | default `/data`: the runner's private key (`runner.key`) and each connection's session state (`state/conn-<id>.json`: cookies, a trusted device). Both 0600; neither ever reaches the app. Keep this directory: losing it means every connection asks for its credentials again |
| `POLL_SECONDS` | how often to look for due connections, default 30 |
| `TW_SYNC_FAKE=1` | also offer the pretend institution `fake` (development only) |

`node src/main.ts run --once` claims and runs once, then exits.

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

`make tw-sync/check` does the same from the repo root; CI runs it as the
`tw-sync` job on both forges.

A connector (`src/connectors/types.ts`) gets its credentials, its saved
state, `saveState`, `ask` (OTP, CAPTCHA, device check) and an abort signal,
and returns one batch: accounts plus `transaction` and `balance` rows, amounts
as decimal strings (`src/money.ts` turns a JS number into one exactly, or
throws). A failure the person should see is a `SyncError` with a stable code;
anything else is reported as `connector_error` and logged here only.

`fake` mirrors `integrations/fake-runner`: password `wrong` fails with
`bad_credentials`, password `otp` asks for a code (123456) and is trusted
afterwards.
