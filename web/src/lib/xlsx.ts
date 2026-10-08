/*
 * An .xlsx read in the tab (#88): the zip is opened here with the browser's
 * own DecompressionStream, then the runner's sheet reader
 * (@sync/util/sheet.ts) gives the first worksheet's rows of cell text, the
 * same rows the runner reads from a download.
 */
import { sheetRows } from '@sync/util/sheet.ts';

async function inflate(data: Uint8Array): Promise<Uint8Array> {
  const stream = new Blob([data as BlobPart]).stream().pipeThrough(new DecompressionStream('deflate-raw'));
  return new Uint8Array(await new Response(stream).arrayBuffer());
}

/** The workbook's XML parts, name -> text; throws 'unreadable' when it is not a zip. */
async function unzip(buf: ArrayBuffer): Promise<Map<string, string>> {
  const b = new Uint8Array(buf);
  const v = new DataView(buf);
  let eocd = -1;
  for (let i = b.length - 22; i >= Math.max(0, b.length - 65_557); i--) {
    if (v.getUint32(i, true) === 0x06054b50) {
      eocd = i;
      break;
    }
  }
  if (eocd < 0) throw new Error('unreadable');
  const count = v.getUint16(eocd + 10, true);
  let p = v.getUint32(eocd + 16, true);
  const text = new TextDecoder();
  const out = new Map<string, string>();
  for (let n = 0; n < count; n++) {
    if (v.getUint32(p, true) !== 0x02014b50) throw new Error('unreadable');
    const method = v.getUint16(p + 10, true);
    const size = v.getUint32(p + 20, true);
    const nameLen = v.getUint16(p + 28, true);
    const skip = v.getUint16(p + 30, true) + v.getUint16(p + 32, true);
    const local = v.getUint32(p + 42, true);
    const name = text.decode(b.subarray(p + 46, p + 46 + nameLen));
    const start = local + 30 + v.getUint16(local + 26, true) + v.getUint16(local + 28, true);
    const data = b.subarray(start, start + size);
    if (name.endsWith('.xml')) {
      if (method === 0) out.set(name, text.decode(data));
      else if (method === 8) out.set(name, text.decode(await inflate(data)));
      else throw new Error('unreadable');
    }
    p += 46 + nameLen + skip;
  }
  return out;
}

/** The first worksheet's rows of cell text. */
export async function xlsxRows(buf: ArrayBuffer): Promise<string[][]> {
  return sheetRows(await unzip(buf));
}
