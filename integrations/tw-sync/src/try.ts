/*
 * `tw-sync try <connector>`: run one connector from a terminal, with no app
 * at all. It asks for the connector's fields here (secrets without echo),
 * answers OTP and CAPTCHA prompts here, and prints the batch it would send
 * as JSON on stdout. Its session state is kept under DATA_DIR/state as
 * try-<connector>, so a second try runs as a trusted device would.
 */
import { createInterface } from 'node:readline/promises';
import { Writable } from 'node:stream';
import { writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { type Connector, SyncError } from './connectors/types.ts';
import { logger } from './log.ts';
import type { Store } from './store.ts';

/**
 * One reader for the whole session, so piped input (printf 'u\np\n' |
 * tw-sync try ...) is not swallowed by the first prompt. A secret is not
 * echoed on a terminal.
 */
function terminal() {
  let muted = false;
  const out = new Writable({
    write(chunk, _enc, cb) {
      if (!muted) process.stderr.write(chunk);
      cb();
    },
  });
  const rl = createInterface({ input: process.stdin, output: out, terminal: process.stdin.isTTY ?? false });
  const lines = rl[Symbol.asyncIterator]();
  return {
    async prompt(question: string, secret = false): Promise<string> {
      process.stderr.write(question);
      muted = secret;
      try {
        const next = await lines.next();
        if (next.done) throw new Error('no more input');
        return next.value;
      } finally {
        muted = false;
        if (secret && process.stdin.isTTY) process.stderr.write('\n');
      }
    },
    close: () => rl.close(),
  };
}

export async function tryConnector(c: Connector, store: Store, signal: AbortSignal): Promise<number> {
  const term = terminal();
  const prompt = term.prompt;
  const credentials: Record<string, string> = {};
  for (const f of c.fields) {
    const v = await prompt(`${f.label}${f.optional ? ' (optional)' : ''}: `, f.kind === 'secret');
    if (v !== '') credentials[f.name] = v;
  }
  const stateName = `try-${c.id}`;
  try {
    const batch = await c.sync({
      credentials,
      state: await store.state(stateName),
      saveState: (s) => store.saveState(stateName, s),
      ask: async (q) => {
        if (q.image) {
          const path = join(store.dir, `challenge-${Date.now()}.png`);
          await writeFile(path, q.image, { mode: 0o600 });
          process.stderr.write(`(the image is at ${path})\n`);
        }
        return prompt(`${q.prompt} [${q.kind}]: `);
      },
      log: logger({ connector: c.id }),
      signal,
    });
    process.stdout.write(JSON.stringify({ connector: c.id, ...batch }, null, 2) + '\n');
    process.stderr.write(`${batch.accounts.length} accounts, ${batch.rows.length} rows\n`);
    return 0;
  } catch (err) {
    if (err instanceof SyncError) {
      process.stderr.write(`failed: ${err.code}\n`);
      return 1;
    }
    throw err;
  } finally {
    term.close();
  }
}
