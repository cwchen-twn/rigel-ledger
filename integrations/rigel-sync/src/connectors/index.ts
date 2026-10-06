import { cathaybk } from './tw/cathaybk.ts';
import { fake } from './fake.ts';
import { megabank } from './tw/megabank.ts';
import { sinopac } from './tw/sinopac.ts';
import { einvoice } from './tw/einvoice.ts';
import { tdcc } from './tw/tdcc.ts';
import { mail } from './xx/mail.ts';
import type { Connector } from './types.ts';

/** The connectors this runner offers; the fake one only when asked for. */
export function connectors(opts: { fake: boolean }): Connector[] {
  const all: Connector[] = [cathaybk, sinopac, megabank, tdcc, einvoice, mail];
  if (opts.fake) all.push(fake);
  return all;
}
