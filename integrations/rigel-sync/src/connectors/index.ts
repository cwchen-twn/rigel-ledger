import { cathaybk } from './tw/cathaybk.ts';
import { fake } from './fake.ts';
import { sinopac } from './tw/sinopac.ts';
import { tdcc } from './tw/tdcc.ts';
import type { Connector } from './types.ts';

/** The connectors this runner offers; the fake one only when asked for. */
export function connectors(opts: { fake: boolean }): Connector[] {
  const all: Connector[] = [cathaybk, sinopac, tdcc];
  if (opts.fake) all.push(fake);
  return all;
}
