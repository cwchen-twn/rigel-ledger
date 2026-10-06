import assert from 'node:assert/strict';
import { test } from 'node:test';
import { minorDigits, toDecimal } from '../src/money.ts';

test('currencies have their ISO digits', () => {
  assert.equal(minorDigits('TWD'), 2);
  assert.equal(minorDigits('USD'), 2);
  assert.equal(minorDigits('PYG'), 0);
  assert.equal(minorDigits('JPY'), 0);
});

test('amounts become exact decimal strings', () => {
  assert.equal(toDecimal(-120, 'TWD'), '-120');
  assert.equal(toDecimal(42000, 'TWD'), '42000');
  assert.equal(toDecimal(0.1 + 0.2, 'USD'), '0.3');
  assert.equal(toDecimal(1.01, 'USD'), '1.01');
  assert.equal(toDecimal(15000, 'PYG'), '15000');
  assert.equal(toDecimal(-1234567.89, 'TWD'), '-1234567.89');
  assert.equal(toDecimal(1e21, 'TWD'), '1000000000000000000000');
  assert.equal(toDecimal(-0, 'TWD'), '0');
});

test('more digits than the currency allows is a mistake, not rounding', () => {
  assert.throws(() => toDecimal(10.5, 'PYG'), RangeError);
  assert.throws(() => toDecimal(1.005, 'USD'), RangeError);
  assert.throws(() => toDecimal(Number.NaN, 'TWD'), RangeError);
  assert.throws(() => toDecimal(1, 'XXZ'), RangeError);
});
