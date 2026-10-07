/*
 * JSON whose numbers stay the text they were sent as. An API that sends
 * amounts as JSON numbers (Firstrade, Shioaji's server) would otherwise have
 * them pass through a double on the way in; read with this, every number is
 * a string ("1234.5", "-0.07"), turned into a decimal with plainDecimal.
 */

const NUMBER = /-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?/y;

/** JSON.parse, every number as its literal text. */
export function parseExact(text: string): unknown {
  let out = '';
  let from = 0;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (c === '"') {
      // Skip the string, escapes and all.
      for (i++; i < text.length && text[i] !== '"'; i++) if (text[i] === '\\') i++;
      continue;
    }
    if (c === '-' || (c >= '0' && c <= '9')) {
      NUMBER.lastIndex = i;
      const m = NUMBER.exec(text);
      if (!m) continue; // not a number: JSON.parse says why
      out += `${text.slice(from, i)}"${m[0]}"`;
      i += m[0].length - 1;
      from = i + 1;
    }
  }
  return JSON.parse(out + text.slice(from));
}
