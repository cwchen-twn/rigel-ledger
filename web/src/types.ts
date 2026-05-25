export interface AppConfig {
  username: string;
  accessTokenLeftTime: number;
  page: string;
}

declare global {
  interface Window {
    __APP_CONFIG__: AppConfig;
  }
}

export interface LedgerType {
  ledgerTypeID: string;
  firstGrade: string;
  secondGrade: string;
  thirdGrade: string;
  typeName: string;
  description: string;
  isActive: boolean;
}

export interface LedgerTypeFirstGrade {
  firstGrade: string;
  typeName: string;
  description: string;
}

export interface Currency {
  alphabeticCode: string;
  numericCode: number;
  minorUnit: number;
  currencyName: string;
}

export interface Posting {
  postingId: number;
  journalId: number;
  ledgerId: number;
  postingType: 'D' | 'C';
  amount: string;
  currencyCode: string;
  description: string;
  createdAt: string;
}

export interface Journal {
  journalId: number;
  userId: string;
  transacId: number;
  transacDate: string;
  postingDate: string;
  photoAddr: string[];
  description: string;
  referenceNo: string;
  isReconciled: boolean;
  exchangeRate: string;
  stockPriceRef: number;
  createdAt: string;
  updatedAt: string;
  postings: Posting[];
}

export interface Ledger {
  ledgerID: number;
  ledgerOwner: string;
  ledgerName: string;
  ledgerTypeID: string;
  ledgerType: LedgerType;
  currency: string;
  balance: string;
  ledgerStatus: number;
  createdAt: string;
  updatedAt: string;
  postingsCount: number;
}
