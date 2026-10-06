/*
 * Reads a CAPTCHA in the runner, so a bank's image check does not need the
 * person every time a session expires (永豐: six digits, 兆豐: five; 將來:
 * five letters and digits).
 *
 * The model is ddddocr's common_old.onnx (MIT, github.com/sml2h3/ddddocr),
 * a CNN + CTC recogniser trained on this kind of image. It is downloaded
 * once, from a pinned commit and checked against its sha256, into
 * DATA_DIR/models (RIGEL_SYNC_OCR_MODEL names a copy already on disk), and run
 * by onnxruntime's WebAssembly build: no native code, the same on every
 * architecture. The image is prepared the way ddddocr prepares it (Lanczos
 * to 64 px high, as Pillow computes it, then grey; a PNG first laid on white,
 * as ddddocr's png_fix does), and only blank and the allowed characters
 * (the ten digits, or digits and ASCII letters) may win at each step.
 * Nothing leaves the runner.
 *
 * On 12 real 永豐 images it read 9 right; every miss came out short, so a
 * caller that wants six digits sees the miss before submitting it.
 */
import { createHash } from 'node:crypto';
import { mkdir, readFile, rename, rm, writeFile } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import jpeg from 'jpeg-js';
import { PNG } from 'pngjs';
import * as ort from 'onnxruntime-web';

export const MODEL = {
  url: 'https://raw.githubusercontent.com/sml2h3/ddddocr/f0ee6b122e9f6b4ccc0cd96032f93cd43721543f/ddddocr/common_old.onnx',
  sha256: 'b8f2ad9cbc1f2e3922a6cb9459e30824e7e2467f3fb4fd61420640e34ea0bf68',
  file: 'ddddocr-common_old.onnx',
};

// The characters' places in the model's charset (ddddocr's CHARSET_OLD,
// ddddocr/charsets.py); 0 is blank.
const DIGITS = new Map([
  [6749, '0'], [4410, '1'], [78, '2'], [7721, '3'], [5806, '4'],
  [6977, '5'], [5961, '6'], [409, '7'], [6979, '8'], [2879, '9'],
]);
const LETTERS = new Map([
  [7198, 'a'], [1066, 'b'], [1107, 'c'], [5726, 'd'], [6257, 'e'], [1638, 'f'], [8119, 'g'], [8196, 'h'], [2041, 'i'],
  [6185, 'j'], [3072, 'k'], [2089, 'l'], [4617, 'm'], [4050, 'n'], [6939, 'o'], [5027, 'p'], [7405, 'q'], [806, 'r'],
  [3466, 's'], [7723, 't'], [2663, 'u'], [1769, 'v'], [7136, 'w'], [6736, 'x'], [6612, 'y'], [4730, 'z'],
  [4771, 'A'], [2203, 'B'], [761, 'C'], [687, 'D'], [2525, 'E'], [357, 'F'], [4488, 'G'], [5554, 'H'], [1614, 'I'],
  [1583, 'J'], [4666, 'K'], [6672, 'L'], [747, 'M'], [6216, 'N'], [5418, 'O'], [3930, 'P'], [6601, 'Q'], [7262, 'R'],
  [6386, 'S'], [5046, 'T'], [7284, 'U'], [5734, 'V'], [4810, 'W'], [5225, 'X'], [821, 'Y'], [3963, 'Z'],
]);
const ALNUM = new Map([...DIGITS, ...LETTERS]);

export function modelPath(): string {
  return process.env.RIGEL_SYNC_OCR_MODEL || join(process.env.DATA_DIR || '/data', 'models', MODEL.file);
}

const sha256 = (b: Uint8Array) => createHash('sha256').update(b).digest('hex');

/** The model's bytes: from disk, or downloaded once and kept. */
async function model(): Promise<Uint8Array> {
  const path = modelPath();
  try {
    const have = await readFile(path);
    if (sha256(have) === MODEL.sha256) return have;
    if (process.env.RIGEL_SYNC_OCR_MODEL) throw new Error(`${path} is not ddddocr's common_old.onnx (sha256 differs)`);
  } catch (err) {
    if ((err as NodeJS.ErrnoException).code !== 'ENOENT') throw err;
  }
  const res = await fetch(MODEL.url, { signal: AbortSignal.timeout(120_000) });
  if (!res.ok) throw new Error(`downloading the CAPTCHA model: HTTP ${res.status}`);
  const got = new Uint8Array(await res.arrayBuffer());
  if (sha256(got) !== MODEL.sha256) throw new Error('the downloaded CAPTCHA model is not the pinned one');
  await mkdir(dirname(path), { recursive: true });
  const tmp = `${path}.${process.pid}.tmp`;
  try {
    await writeFile(tmp, got);
    await rename(tmp, path);
  } finally {
    await rm(tmp, { force: true });
  }
  return got;
}

let session: Promise<ort.InferenceSession> | undefined;
function load(): Promise<ort.InferenceSession> {
  if (!session) {
    ort.env.wasm.numThreads = 1; // no worker threads left holding the process open
    ort.env.logLevel = 'error';
    session = model().then((m) => ort.InferenceSession.create(m, { logSeverityLevel: 3 }));
    session.catch(() => (session = undefined)); // try again next time
  }
  return session;
}

/** The digits read from a JPEG or PNG; shorter than asked when the model was unsure. */
export async function readDigits(image: Uint8Array): Promise<string> {
  return read(image, DIGITS);
}

/** Letters and digits (as the model sees their case), from a JPEG or PNG. */
export async function readAlphanumeric(image: Uint8Array): Promise<string> {
  return read(image, ALNUM);
}

async function read(image: Uint8Array, allowed: Map<number, string>): Promise<string> {
  const s = await load();
  const { grey, width, height } = prepare(image);
  const input = new ort.Tensor('float32', Float32Array.from(grey, (v) => v / 255), [1, 1, height, width]);
  const out = (await s.run({ [s.inputNames[0]]: input }))[s.outputNames[0]];
  const [steps, , classes] = out.dims as number[];
  const scores = out.data as Float32Array;
  let text = '';
  let prev = -1;
  for (let t = 0; t < steps; t++) {
    let best = 0; // blank
    for (const c of allowed.keys()) if (scores[t * classes + c] > scores[t * classes + best]) best = c;
    if (best !== prev && best !== 0) text += allowed.get(best);
    prev = best;
  }
  return text;
}

// Pillow's Lanczos resampling (support 3), in its fixed point, so the
// model sees what ddddocr gives it.
const BITS = 22;
const lanczos = (x: number) => {
  if (x === 0) return 1;
  if (x <= -3 || x >= 3) return 0;
  const a = Math.PI * x;
  return (Math.sin(a) / a) * (Math.sin(a / 3) / (a / 3));
};
function weights(from: number, to: number) {
  const scale = from / to;
  const fs = Math.max(scale, 1);
  const support = 3 * fs;
  return Array.from({ length: to }, (_, x) => {
    const center = (x + 0.5) * scale;
    const min = Math.max(Math.trunc(center - support + 0.5), 0);
    const n = Math.min(Math.trunc(center + support + 0.5), from) - min;
    const k = Array.from({ length: n }, (_, j) => lanczos((j + min - center + 0.5) / fs));
    const total = k.reduce((a, b) => a + b, 0);
    return { min, k: k.map((w) => Math.trunc((total ? w / total : 0) * (1 << BITS) + (w < 0 ? -0.5 : 0.5))) };
  });
}
const clip8 = (v: number) => Math.min(255, Math.max(0, Math.floor(v / (1 << BITS))));

const isPng = (b: Uint8Array) => b[0] === 0x89 && b[1] === 0x50 && b[2] === 0x4e && b[3] === 0x47;

/** RGBA pixels of a JPEG, or of a PNG laid on white (ddddocr's png_fix). */
function decode(image: Uint8Array): { width: number; height: number; data: Uint8Array } {
  if (!isPng(image)) return jpeg.decode(image, { useTArray: true, formatAsRGBA: true });
  const png = PNG.sync.read(Buffer.from(image));
  const data = new Uint8Array(png.data.length);
  for (let i = 0; i < png.data.length; i += 4) {
    const a = png.data[i + 3];
    for (let c = 0; c < 3; c++) data[i + c] = Math.round((png.data[i + c] * a + 255 * (255 - a)) / 255);
    data[i + 3] = 255;
  }
  return { width: png.width, height: png.height, data };
}

export function prepare(image: Uint8Array): { grey: Uint8Array; width: number; height: number } {
  const src = decode(image);
  const height = 64;
  const width = Math.trunc(src.width * (height / src.height));
  const across = weights(src.width, width);
  const down = weights(src.height, height);
  const mid = new Uint8Array(width * src.height * 3);
  for (let y = 0; y < src.height; y++) {
    for (let x = 0; x < width; x++) {
      const { min, k } = across[x];
      for (let c = 0; c < 3; c++) {
        let s = 1 << (BITS - 1);
        for (let j = 0; j < k.length; j++) s += src.data[(y * src.width + min + j) * 4 + c] * k[j];
        mid[(y * width + x) * 3 + c] = clip8(s);
      }
    }
  }
  const grey = new Uint8Array(width * height);
  for (let y = 0; y < height; y++) {
    const { min, k } = down[y];
    for (let x = 0; x < width; x++) {
      const rgb = [0, 1, 2].map((c) => {
        let s = 1 << (BITS - 1);
        for (let j = 0; j < k.length; j++) s += mid[((min + j) * width + x) * 3 + c] * k[j];
        return clip8(s);
      });
      grey[y * width + x] = (rgb[0] * 19595 + rgb[1] * 38470 + rgb[2] * 7471 + 0x8000) >> 16;
    }
  }
  return { grey, width, height };
}

/**
 * With RIGEL_SYNC_KEEP_CAPTCHAS set to a directory, keeps each CAPTCHA a
 * connector read, named by what was read and whether the bank took it,
 * so the reader can be measured on real images. Off by default; the files
 * stay on the runner.
 */
export async function keepCaptcha(bank: string, image: Uint8Array, read: string, outcome: 'accepted' | 'rejected' | 'unsure'): Promise<void> {
  const dir = process.env.RIGEL_SYNC_KEEP_CAPTCHAS;
  if (!dir) return;
  const ext = isPng(image) ? 'png' : 'jpg';
  await mkdir(dir, { recursive: true });
  await writeFile(join(dir, `${bank}-${Date.now()}-${outcome}-${read.replace(/[^A-Za-z0-9]/g, '') || 'none'}.${ext}`), image);
}
