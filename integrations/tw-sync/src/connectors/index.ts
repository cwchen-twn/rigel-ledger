import { fake } from './fake.ts';
import type { Connector } from './types.ts';

/** The connectors this runner offers; the fake one only when asked for. */
export function connectors(opts: { fake: boolean }): Connector[] {
  const all: Connector[] = [];
  if (opts.fake) all.push(fake);
  return all;
}
