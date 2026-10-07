/*
 * KuDE receipts (#44): a Paraguayan e-invoice's QR code, read in the tab
 * from a photo or a PDF. The QR carries the invoice (@sync/statements/kude),
 * so nothing is read by OCR; only the row and the file are sent.
 *
 * jsQR (Apache-2.0, 250 kB) is not in the bundle: like pdf.js, its prebuilt
 * file is added as a script the first time a receipt is read.
 */
import { KUDE_ACCOUNT, kudeRow, readKude } from '@sync/statements/kude.ts';
import jsqrUrl from 'jsqr/dist/jsQR.js?url';
import { script, type Statement, withPdf } from '~/lib/statements';

type JsQR = (data: Uint8ClampedArray, width: number, height: number) => { data: string } | null;

let loading: Promise<JsQR> | undefined;

function jsqr(): Promise<JsQR> {
  loading ??= (async () => {
    await script(jsqrUrl); // UMD: sets self.jsQR
    const f = (globalThis as { jsQR?: JsQR }).jsQR;
    if (!f) throw new Error('jsQR did not load');
    return f;
  })();
  return loading;
}

type Source = CanvasImageSource & { width: number; height: number };

/** The QR's text in an image, tried at a few sizes (a phone photo is 12 MP). */
async function scan(img: Source): Promise<string | null> {
  const decode = await jsqr();
  const canvas = document.createElement('canvas');
  const ctx = canvas.getContext('2d', { willReadFrequently: true });
  if (!ctx) return null;
  const longest = Math.max(img.width, img.height);
  for (const target of [1600, 2400, longest]) {
    const k = Math.min(1, target / longest);
    canvas.width = Math.round(img.width * k);
    canvas.height = Math.round(img.height * k);
    ctx.fillStyle = '#fff';
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    ctx.drawImage(img, 0, 0, canvas.width, canvas.height);
    const code = decode(ctx.getImageData(0, 0, canvas.width, canvas.height).data, canvas.width, canvas.height);
    if (code?.data) return code.data;
    if (k === 1) break;
  }
  return null;
}

/** Each page of a PDF drawn at print resolution, scanned for the QR. */
function scanPdf(data: ArrayBuffer, password: string): Promise<string | null> {
  return withPdf(data, password, async (doc) => {
    for (let n = 1; n <= Math.min(doc.numPages, 3); n++) {
      const page = await doc.getPage(n);
      const viewport = page.getViewport({ scale: 3 });
      const canvas = document.createElement('canvas');
      canvas.width = Math.round(viewport.width);
      canvas.height = Math.round(viewport.height);
      await page.render({ canvas, viewport }).promise;
      const text = await scan(canvas);
      if (text) return text;
    }
    return null;
  });
}

/** The KuDE a photo or PDF is; throws 'unrecognised' when no KuDE QR is found. */
export async function readReceipt(data: ArrayBuffer, type: string, password: string, file: string): Promise<Statement> {
  let text: string | null;
  if (type === 'application/pdf') {
    text = await scanPdf(data, password);
  } else {
    let img: ImageBitmap;
    try {
      img = await createImageBitmap(new Blob([data], { type }));
    } catch {
      throw new Error('unreadable'); // HEIC outside Safari, a broken file
    }
    text = await scan(img);
    img.close();
  }
  const k = text ? readKude(text) : null;
  if (!k) throw new Error('unrecognised');
  return { name: `KuDE ${k.number} · RUC ${k.ruc}`, account: KUDE_ACCOUNT, rows: [kudeRow(k, file)] };
}
