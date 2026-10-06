// The CAPTCHA reader on PNGs (將來 sends PNG) and in letters-and-digits mode,
// on the real 永豐 samples in test/captcha.
import assert from 'node:assert/strict';
import { readdirSync, readFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import jpeg from 'jpeg-js';
import { PNG } from 'pngjs';
import { MODEL, readAlphanumeric, readDigits } from '../src/ocr/captcha.ts';

process.env.RIGEL_SYNC_OCR_MODEL ||= join(tmpdir(), 'rigel-sync-test-models', MODEL.file);
const CAPTCHAS = join(import.meta.dirname, 'captcha');

/** The JPEG as a PNG; with `clear`, its near-white pixels transparent (and black beneath). */
function asPng(jpg: Buffer, clear = false): Buffer {
  const src = jpeg.decode(jpg, { useTArray: true, formatAsRGBA: true });
  const png = new PNG({ width: src.width, height: src.height });
  for (let i = 0; i < src.data.length; i += 4) {
    const white = src.data[i] > 245 && src.data[i + 1] > 245 && src.data[i + 2] > 245;
    if (clear && white) png.data.set([0, 0, 0, 0], i);
    else png.data.set([src.data[i], src.data[i + 1], src.data[i + 2], 255], i);
  }
  return PNG.sync.write(png);
}

test('a PNG reads as its JPEG does, a transparent background laid on white', async () => {
  for (const f of readdirSync(CAPTCHAS)) {
    const jpg = readFileSync(join(CAPTCHAS, f));
    const want = await readDigits(jpg);
    assert.equal(await readDigits(asPng(jpg)), want, f);
    assert.equal(await readDigits(asPng(jpg, true)), want, `${f} (transparent)`);
  }
});

test('letters and digits: digit images mostly still read as digits', async () => {
  let same = 0;
  const files = readdirSync(CAPTCHAS);
  for (const f of files) if ((await readAlphanumeric(readFileSync(join(CAPTCHAS, f)))) === f.slice(0, 6)) same++;
  console.log(`alphanumeric mode read ${same} of ${files.length} digit CAPTCHAs exactly`);
  assert.ok(same >= files.length - 4, `only ${same} of ${files.length}`);
});
