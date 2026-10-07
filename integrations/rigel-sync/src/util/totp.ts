/*
 * A time-based one-time password (RFC 6238: HMAC-SHA1, 30 s, 6 digits) from
 * the base32 secret an authenticator app is set up with, for an institution
 * whose second factor is such an app (Firstrade).
 */
import { createHmac } from 'node:crypto';

const BASE32 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';

/** The bytes of a base32 secret; spaces, dashes and padding ignored. */
export function base32(secret: string): Buffer {
  const clean = secret.toUpperCase().replace(/[\s=-]/g, '');
  let bits = 0;
  let value = 0;
  const out: number[] = [];
  for (const ch of clean) {
    const v = BASE32.indexOf(ch);
    if (v < 0) throw new Error('totp: the secret is not base32');
    value = (value << 5) | v;
    bits += 5;
    if (bits >= 8) {
      out.push((value >>> (bits - 8)) & 0xff);
      bits -= 8;
    }
  }
  return Buffer.from(out);
}

/** The code for a moment (ms since 1970), from the secret's bytes. */
export function totpAt(key: Buffer, at: number, digits = 6, step = 30): string {
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(Math.floor(at / 1000 / step)));
  const h = createHmac('sha1', key).update(counter).digest();
  const o = h[h.length - 1] & 0xf;
  const n = (h.readUInt32BE(o) & 0x7fffffff) % 10 ** digits;
  return String(n).padStart(digits, '0');
}

/** The code now, from a base32 secret. */
export const totp = (secret: string, at = Date.now()) => totpAt(base32(secret), at);
