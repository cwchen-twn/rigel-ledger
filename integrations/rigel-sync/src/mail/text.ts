import type { Mail } from './parse.ts';

const strip = (s: string) =>
  s
    .replace(/<(style|script|head)\b[\s\S]*?<\/\1>/gi, ' ')
    .replace(/<[^>]+>/g, ' ')
    .replace(/&nbsp;/g, ' ')
    .replace(/&amp;/g, '&')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&#(\d+);/g, (_, n) => String.fromCodePoint(Number(n)));

/**
 * An email's words, without markup: its text part, or its HTML. Tags are
 * stripped from both: some senders (Gotogate) put HTML in the text part.
 */
export function plainText(mail: Mail): string {
  const body = mail.text?.trim() ? mail.text : (mail.html ?? '');
  return strip(body).replace(/\s+/g, ' ').trim();
}
