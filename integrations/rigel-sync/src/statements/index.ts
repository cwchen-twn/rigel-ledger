/*
 * Every statement parser, tried in turn on a PDF's lines (#42). Shared with
 * the web app's PDF import: no imports but parsers, decimal.ts and types.
 */
import { monthRows, readMonth } from './cathayfut.ts';
import { ContinentalUnbalanced, type ParsedStatement, readContinentalAccount, readContinentalCard } from './continental.ts';

export type { ParsedStatement } from './continental.ts';

/** The statement's rows; null when no parser knows it; throws StatementUnbalanced when its totals disagree. */
export function readStatementLines(lines: string[][], file?: string): ParsedStatement | null {
  try {
    const month = readMonth(lines);
    if (month) {
      let parsed;
      try {
        parsed = monthRows(month, file);
      } catch {
        throw new StatementUnbalanced('國泰期貨');
      }
      return { name: `國泰期貨 月對帳單 ${month.to.slice(0, 7)}`, ...parsed };
    }
    const st = readContinentalAccount(lines) ?? readContinentalCard(lines);
    if (st && file) {
      const first = st.rows.find((r) => r.kind === 'transaction');
      if (first) first.file = file;
    }
    return st;
  } catch (err) {
    if (err instanceof ContinentalUnbalanced) throw new StatementUnbalanced(err.message);
    throw err;
  }
}

export class StatementUnbalanced extends Error {}
