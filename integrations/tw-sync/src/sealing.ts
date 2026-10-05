/*
 * The sealing format of internal/sealing (read its package comment; keep the
 * three copies -- Go, web/src/lib/seal.ts, this one -- in step), with
 * node:crypto only. The runner opens what browsers sealed to its key: the
 * credentials of a connection and the answers to its challenges.
 *
 *   0x01 | ephemeral X25519 public key (32) | AES-GCM nonce (12) | ciphertext+tag
 */
import { createCipheriv, createDecipheriv, createPrivateKey, createPublicKey, diffieHellman, generateKeyPairSync, hkdfSync, randomBytes, type KeyObject } from 'node:crypto';

const VERSION = 1;
const INFO = 'rigel-ledger sealed v1';
const KEY = 32;
const NONCE = 12;
const TAG = 16;

// DER prefixes that wrap a raw 32-byte X25519 key (RFC 8410).
const PKCS8 = Buffer.from('302e020100300506032b656e04220420', 'hex');
const SPKI = Buffer.from('302a300506032b656e032100', 'hex');

export class SealError extends Error {}

/** The additional data binding a blob to its place: aad('credentials', userId, connector). */
export function aad(purpose: string, ...context: (string | number)[]): Buffer {
  return Buffer.from(`rigel-ledger/${purpose}/v1\0${context.map(String).join('\0')}`);
}

/** A runner key pair as raw 32-byte keys. */
export function newKey(): { privateKey: Buffer; publicKey: Buffer } {
  const { privateKey } = generateKeyPairSync('x25519');
  const raw = Buffer.from(privateKey.export({ format: 'jwk' }).d!, 'base64url');
  return { privateKey: raw, publicKey: publicKeyOf(raw) };
}

export function publicKeyOf(rawPrivate: Buffer): Buffer {
  return rawPublic(createPublicKey(privateKeyObject(rawPrivate)));
}

function privateKeyObject(raw: Buffer): KeyObject {
  if (raw.length !== KEY) throw new SealError('not an X25519 private key');
  return createPrivateKey({ key: Buffer.concat([PKCS8, raw]), format: 'der', type: 'pkcs8' });
}

function publicKeyObject(raw: Buffer): KeyObject {
  if (raw.length !== KEY) throw new SealError('not an X25519 public key');
  return createPublicKey({ key: Buffer.concat([SPKI, raw]), format: 'der', type: 'spki' });
}

function rawPublic(k: KeyObject): Buffer {
  return Buffer.from(k.export({ format: 'jwk' }).x!, 'base64url');
}

function keyFor(shared: Buffer, eph: Buffer, runner: Buffer): Buffer {
  return Buffer.from(hkdfSync('sha256', shared, Buffer.concat([eph, runner]), INFO, KEY));
}

/** Seal plaintext to a runner's public key (what the browser does; here for tests and tools). */
export function seal(runnerPublic: Buffer, plaintext: Buffer, additional: Buffer): Buffer {
  const { privateKey } = generateKeyPairSync('x25519');
  const eph = rawPublic(createPublicKey(privateKey));
  const shared = diffieHellman({ privateKey, publicKey: publicKeyObject(runnerPublic) });
  const nonce = randomBytes(NONCE);
  const c = createCipheriv('aes-256-gcm', keyFor(shared, eph, runnerPublic), nonce);
  c.setAAD(additional);
  const ct = Buffer.concat([c.update(plaintext), c.final(), c.getAuthTag()]);
  return Buffer.concat([Buffer.from([VERSION]), eph, nonce, ct]);
}

/** Open a blob sealed to this runner. Throws SealError on any mismatch. */
export function open(runnerPrivate: Buffer, blob: Buffer, additional: Buffer): Buffer {
  if (blob.length < 1 + KEY + NONCE + TAG || blob[0] !== VERSION) throw new SealError('not a sealed blob');
  const eph = blob.subarray(1, 1 + KEY);
  const nonce = blob.subarray(1 + KEY, 1 + KEY + NONCE);
  const body = blob.subarray(1 + KEY + NONCE, blob.length - TAG);
  const tag = blob.subarray(blob.length - TAG);
  try {
    const priv = privateKeyObject(runnerPrivate);
    const shared = diffieHellman({ privateKey: priv, publicKey: publicKeyObject(Buffer.from(eph)) });
    const d = createDecipheriv('aes-256-gcm', keyFor(shared, Buffer.from(eph), rawPublic(createPublicKey(priv))), nonce);
    d.setAAD(additional);
    d.setAuthTag(tag);
    return Buffer.concat([d.update(body), d.final()]);
  } catch {
    throw new SealError('the blob does not open with this key and additional data');
  }
}
