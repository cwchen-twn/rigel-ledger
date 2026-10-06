import assert from 'node:assert/strict';
import { test } from 'node:test';
import { aad, newKey, open, publicKeyOf, seal, SealError } from '../src/sealing.ts';

// The browser-made blob internal/sealing's TestOpensABrowserBlob opens: the
// three implementations agree. A throwaway key pair; it protects nothing.
const browserPrivate = 'mW4ImU8Om/3tQlDhvBD9CbEOCjsEM/ZT7MeKah2G6wk='; // gitleaks:allow
const browserBlob =
  'AbIl0v0XsqLPx5K+WSo7T3NUeZm4chBdsPjxF4GYxM8iVv0EFV6ZZyXhRievVh0DktfwuQsTLVPMECdWRl/vv4+y+bWq7LbSN1SNksediZa7yY682ns0OChMlgBuUBQOCPQ=';

test('opens a blob the browser sealed', () => {
  const got = open(Buffer.from(browserPrivate, 'base64'), Buffer.from(browserBlob, 'base64'), aad('credentials', 7, 'fake'));
  assert.equal(got.toString(), '{"username":"alice","password":"otp"}');
});

test('the additional data binds a blob to its place', () => {
  assert.throws(
    () => open(Buffer.from(browserPrivate, 'base64'), Buffer.from(browserBlob, 'base64'), aad('credentials', 8, 'fake')),
    SealError,
  );
});

test('seal and open round trip, and only the right key opens', () => {
  const k = newKey();
  assert.deepEqual(publicKeyOf(k.privateKey), k.publicKey);
  const blob = seal(k.publicKey, Buffer.from('123456'), aad('answer', 42));
  assert.equal(open(k.privateKey, blob, aad('answer', 42)).toString(), '123456');
  assert.throws(() => open(newKey().privateKey, blob, aad('answer', 42)), SealError);
  assert.throws(() => open(k.privateKey, blob.subarray(0, 20), aad('answer', 42)), SealError);
});
