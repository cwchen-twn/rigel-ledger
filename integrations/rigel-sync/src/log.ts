import type { Logger } from './connectors/types.ts';

/** One JSON line per event on stderr, like the app's slog output. Never log credentials. */
export function logger(base: Record<string, unknown> = {}): Logger & { with(f: Record<string, unknown>): ReturnType<typeof logger>; error(msg: string, f?: Record<string, unknown>): void } {
  const out = (level: string, msg: string, f?: Record<string, unknown>) =>
    process.stderr.write(JSON.stringify({ time: new Date().toISOString(), level, msg, ...base, ...f }) + '\n');
  return {
    info: (m, f) => out('INFO', m, f),
    warn: (m, f) => out('WARN', m, f),
    error: (m, f) => out('ERROR', m, f),
    with: (f) => logger({ ...base, ...f }),
  };
}
