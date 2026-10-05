#!/usr/bin/env -S node --disable-warning=ExperimentalWarning
/*
 * tw-sync: a person's sync runner for Taiwan institutions (P4c-2). Linked
 * in the app under Settings -> Sync runner; see integrations/tw-sync/README.md.
 *
 *   tw-sync run                 the daemon (RIGEL_URL, RUNNER_TOKEN, DATA_DIR)
 *   tw-sync try <connector>     one connector from the terminal, no app
 *   tw-sync connectors          what this build offers
 */
import { RigelClient } from './api.ts';
import { connectors } from './connectors/index.ts';
import { runDaemon } from './runner.ts';
import { Store } from './store.ts';
import { tryConnector } from './try.ts';

const env = (name: string, fallback?: string) => process.env[name] || fallback;

async function main(argv: string[]): Promise<number> {
  const [cmd = 'run', ...rest] = argv;
  const store = new Store(env('DATA_DIR', '/data')!);
  const offered = connectors({ fake: env('TW_SYNC_FAKE') === '1' || cmd === 'try' });
  const ac = new AbortController();
  for (const s of ['SIGINT', 'SIGTERM'] as const) process.once(s, () => ac.abort());

  switch (cmd) {
    case 'run': {
      const url = env('RIGEL_URL');
      const token = env('RUNNER_TOKEN');
      if (!url || !token) {
        process.stderr.write('RIGEL_URL and RUNNER_TOKEN are required (Settings -> Sync runner -> Link a runner)\n');
        return 2;
      }
      await runDaemon({
        client: new RigelClient(url, token),
        store,
        connectors: offered,
        pollMs: Number(env('POLL_SECONDS', '30')) * 1000,
        once: rest.includes('--once'),
        signal: ac.signal,
      });
      return 0;
    }
    case 'try': {
      const c = offered.find((x) => x.id === rest[0]);
      if (!c) {
        process.stderr.write(`usage: tw-sync try <${offered.map((x) => x.id).join('|')}>\n`);
        return 2;
      }
      return tryConnector(c, store, ac.signal);
    }
    case 'connectors':
      for (const c of offered) process.stdout.write(`${c.id}\t${c.country}\t${c.name}\n`);
      return 0;
    default:
      process.stderr.write('usage: tw-sync run [--once] | try <connector> | connectors\n');
      return 2;
  }
}

main(process.argv.slice(2)).then(
  (code) => process.exit(code),
  (err) => {
    process.stderr.write(`${err instanceof Error ? (err.stack ?? err.message) : String(err)}\n`);
    process.exit(1);
  },
);
