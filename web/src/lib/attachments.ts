/*
 * Files before they are uploaded (#36): a photo straight from a phone camera
 * is 3-12 MB and 4000+ px; a receipt reads fine at 2048 px. Large images are
 * re-encoded here as JPEG, so uploads are small and the books stay small.
 * PDFs, and images the browser cannot decode, go as they are (the server
 * decides what it accepts).
 */

/** The server's cap per file (ledger.MaxAttachmentBytes). */
export const MAX_FILE_BYTES = 10 * 1024 * 1024;

/** What the server keeps: images and PDFs. */
export const ACCEPT = 'image/jpeg,image/png,image/webp,application/pdf,image/*';

const MAX_EDGE = 2048;
const SMALL_BYTES = 1.5 * 1024 * 1024;
const KEPT = ['image/jpeg', 'image/png', 'image/webp'];

export interface PreparedFile {
  blob: Blob;
  name: string;
}

export async function prepareFile(file: File): Promise<PreparedFile> {
  const asIs = { blob: file, name: file.name };
  if (!file.type.startsWith('image/')) return asIs;
  let bitmap: ImageBitmap;
  try {
    bitmap = await createImageBitmap(file, { imageOrientation: 'from-image' });
  } catch {
    return asIs; // HEIC where the browser cannot decode it: the server says no
  }
  const scale = Math.min(1, MAX_EDGE / Math.max(bitmap.width, bitmap.height));
  if (scale === 1 && file.size <= SMALL_BYTES && KEPT.includes(file.type)) {
    bitmap.close();
    return asIs;
  }
  const canvas = document.createElement('canvas');
  canvas.width = Math.round(bitmap.width * scale);
  canvas.height = Math.round(bitmap.height * scale);
  const ctx = canvas.getContext('2d');
  if (!ctx) {
    bitmap.close();
    return asIs;
  }
  // JPEG has no transparency: a transparent screenshot gets white, not black.
  ctx.fillStyle = 'white';
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  ctx.drawImage(bitmap, 0, 0, canvas.width, canvas.height);
  bitmap.close();
  const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, 'image/jpeg', 0.85));
  if (!blob) return asIs;
  return { blob, name: `${file.name.replace(/\.[^.]*$/, '') || 'photo'}.jpg` };
}

/** "1.2 MB" */
export function formatSize(bytes: number, locale: string): string {
  const units = ['B', 'KB', 'MB'];
  let n = bytes;
  let u = 0;
  while (n >= 1024 && u < units.length - 1) {
    n /= 1024;
    u++;
  }
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: u ? 1 : 0 }).format(n)} ${units[u]}`;
}
