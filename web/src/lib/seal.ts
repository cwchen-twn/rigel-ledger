/*
 * Seal a secret to the sync runner's X25519 public key, in the browser, so
 * the app only ever receives ciphertext it cannot open. The format is
 * internal/sealing's (read its package comment); keep the two in step.
 * WebCrypto only: X25519, HKDF-SHA256 and AES-256-GCM.
 */

const VERSION = 1;
const INFO = new TextEncoder().encode('rigel-ledger sealed v1');

const b64 = (bytes: Uint8Array) => btoa(String.fromCharCode(...bytes));
export const fromB64 = (s: string) => Uint8Array.from(atob(s), (c) => c.charCodeAt(0));

/** The additional data binding a blob to its place: aad('credentials', userId, connector). */
export function aad(purpose: string, ...context: (string | number)[]): Uint8Array<ArrayBuffer> {
  return new TextEncoder().encode(`rigel-ledger/${purpose}/v1\0${context.map(String).join('\0')}`);
}

/** Whether this browser can seal (X25519 in WebCrypto: Chrome 133, Firefox 130, Safari 17.4). */
export async function canSeal(): Promise<boolean> {
  try {
    await crypto.subtle.generateKey({ name: 'X25519' }, false, ['deriveBits']);
    return true;
  } catch {
    return false;
  }
}

/** Seal plaintext to runnerPublic (raw 32 bytes, base64); returns the blob as base64. */
export async function seal(runnerPublicB64: string, plaintext: string, additional: Uint8Array<ArrayBuffer>): Promise<string> {
  const runnerRaw = fromB64(runnerPublicB64);
  const runner = await crypto.subtle.importKey('raw', runnerRaw, { name: 'X25519' }, false, []);
  const eph = (await crypto.subtle.generateKey({ name: 'X25519' }, true, ['deriveBits'])) as CryptoKeyPair;
  const ephRaw = new Uint8Array(await crypto.subtle.exportKey('raw', eph.publicKey));
  const shared = await crypto.subtle.deriveBits({ name: 'X25519', public: runner }, eph.privateKey, 256);
  const ikm = await crypto.subtle.importKey('raw', shared, 'HKDF', false, ['deriveKey']);
  const salt = new Uint8Array([...ephRaw, ...runnerRaw]);
  const key = await crypto.subtle.deriveKey(
    { name: 'HKDF', hash: 'SHA-256', salt, info: INFO },
    ikm,
    { name: 'AES-GCM', length: 256 },
    false,
    ['encrypt'],
  );
  const nonce = crypto.getRandomValues(new Uint8Array(12));
  const ct = new Uint8Array(
    await crypto.subtle.encrypt({ name: 'AES-GCM', iv: nonce, additionalData: additional }, key, new TextEncoder().encode(plaintext)),
  );
  return b64(new Uint8Array([VERSION, ...ephRaw, ...nonce, ...ct]));
}
