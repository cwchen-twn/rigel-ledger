// What src/connectors/tw/einvoice.ts uses from the bundle, as upstream
// declares it (apps/worker/src/sources/einvoice/{protocol,v2-client}.ts);
// keep in step when the pinned commit moves.

/** The session the app hands out at sign-in, kept to sign in no more than needed. */
export interface EInvoiceSessionConfigUpdates {
  sid: string;
  token: string;
  loginAppId: string;
  loginLiat: number;
  loginSsMe: string;
  iv?: string;
  svrCode?: string;
  ltoken?: string;
  hkey?: string;
  serverTimeOffset?: number;
  loginClientCode?: string;
  mobileBarcode?: string;
}

export type InvoiceConfig = Partial<EInvoiceSessionConfigUpdates> & {
  mobile?: string;
  password?: string;
  androidId?: string;
  loginType: number;
};

/** Fills upstream's defaults. */
export function parseInvoiceConfig(config: unknown): InvoiceConfig;

export interface EInvoiceV2Session {
  sid: string;
  token: string;
  carrierCode?: string;
}

export interface EInvoiceInvoiceHeader {
  sourceId: string;
  invNum: string;
  detailInvDate: string;
  invoice: {
    sourceId: string;
    invoiceNumber?: string;
    /** YYYY-MM-DD, or an ISO timestamp in UTC when the API gave a time. */
    invoiceDate: string;
    sellerName?: string;
    amount: number;
  };
}

/** Signs in (or reuses the session in the config) and lists the last two invoice periods. */
export function initializeEInvoiceSync(config: InvoiceConfig): Promise<{
  session: EInvoiceV2Session;
  configUpdates: EInvoiceSessionConfigUpdates;
  headers: EInvoiceInvoiceHeader[];
  detailTasks: EInvoiceInvoiceHeader[];
}>;

/** One invoice's lines, as the API wrote them (strings). */
export function fetchEInvoiceInvoiceDetail(session: EInvoiceV2Session, task: EInvoiceInvoiceHeader): Promise<{
  detailItems: Array<{ id: string; amount: string; description: string; quantity: string; unitPrice: string }>;
}>;
