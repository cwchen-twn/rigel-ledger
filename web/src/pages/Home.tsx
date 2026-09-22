import { createSignal, createResource, createMemo, For, Show, type JSXElement } from 'solid-js';
import { useParams } from '@solidjs/router';
import dayjs from 'dayjs';
import type { Journal, DataTableResponse, Ledger, Currency } from '../types';
import { useI18n } from '../i18n';

const PAGE_SIZE = 10;
type BalanceView = 'assets' | 'liabilities' | 'networth';

function fmt(n: number): string {
  return new Intl.NumberFormat('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(n);
}

export default function Home(): JSXElement {
  const params = useParams<{ username: string }>();
  const { t } = useI18n();

  // ── Ledger summary ──────────────────────────────────────────────────────────
  const [ledgers] = createResource(
    () => params.username,
    (u) => fetch(`/${u}/api/ledgers`).then(r => r.json()) as Promise<Ledger[]>,
  );

  const [balanceView, setBalanceView] = createSignal<BalanceView>('assets');

  const totals = createMemo(() => {
    const data = ledgers() ?? [];
    const active = data.filter(l => l.ledgerStatus === 1);
    const assets = active
      .filter(l => l.ledgerType.firstGrade === '1')
      .reduce((s, l) => s + parseFloat(l.balance || '0'), 0);
    const liabilities = active
      .filter(l => l.ledgerType.firstGrade === '2')
      .reduce((s, l) => s + parseFloat(l.balance || '0'), 0);
    return { assets, liabilities, networth: assets - liabilities };
  });

  const displayValue = createMemo(() => totals()[balanceView()]);
  const displayLabel = createMemo(() => ({
    assets: t('home.total_assets'),
    liabilities: t('home.total_liabilities'),
    networth: t('home.net_worth'),
  })[balanceView()]);

  const primaryCurrency = createMemo(() => {
    const data = ledgers() ?? [];
    const freq: Record<string, number> = {};
    data.filter(l => l.ledgerType.firstGrade === '1' && l.ledgerStatus === 1)
      .forEach(l => { freq[l.currency] = (freq[l.currency] ?? 0) + 1; });
    return Object.entries(freq).sort((a, b) => b[1] - a[1])[0]?.[0] ?? '';
  });

  // ── Transactions table ───────────────────────────────────────────────────────
  const [page, setPage] = createSignal(0);
  const [search, setSearch] = createSignal('');
  const [expandedRow, setExpandedRow] = createSignal<number | null>(null);

  const [transactions] = createResource(
    () => ({ page: page(), search: search(), username: params.username }),
    async ({ page: pg, search: q, username }) => {
      const body = new URLSearchParams({
        draw: '1',
        start: String(pg * PAGE_SIZE),
        length: String(PAGE_SIZE),
        'search[value]': q,
        'search[regex]': 'false',
      });
      const resp = await fetch(`/${username}/api/transactions`, { method: 'POST', body });
      if (!resp.ok) throw new Error('Failed to fetch transactions');
      return resp.json() as Promise<DataTableResponse<Journal>>;
    },
  );

  const totalPages = () => Math.ceil((transactions()?.recordsFiltered ?? 0) / PAGE_SIZE);

  return (
    <div class="container-fluid py-4">
      <h4 class="mb-4 fw-semibold">{t('home.title')}</h4>

      {/* ── Row 1: Balance summary ─────────────────────────────────────────── */}
      <div class="row g-3 mb-3">

        {/* Balance card */}
        <div class="col-12 col-md-6 col-lg-3">
          <div class="card h-100 border shadow-none">
            <div class="card-body">
              <div class="btn-group btn-group-sm mb-3 w-100" role="group">
                <For each={(['assets', 'liabilities', 'networth'] as BalanceView[])}>
                  {(v) => (
                    <>
                      <input
                        type="radio"
                        class="btn-check"
                        name="balancetype"
                        id={`bv-${v}`}
                        checked={balanceView() === v}
                        onChange={() => setBalanceView(v)}
                      />
                      <label class="btn btn-outline-secondary" style="font-size:0.7rem" for={`bv-${v}`}>
                        {v === 'assets' ? t('home.btn_assets') : v === 'liabilities' ? t('home.btn_liabilities') : t('home.btn_networth')}
                      </label>
                    </>
                  )}
                </For>
              </div>
              <p class="text-muted small mb-1">{displayLabel()}</p>
              <Show
                when={!ledgers.loading}
                fallback={
                  <div class="placeholder-glow">
                    <span class="placeholder col-8" style="height:2.5rem;border-radius:4px">&nbsp;</span>
                  </div>
                }
              >
                <p class="fs-2 fw-bold mb-0 lh-1">{fmt(displayValue())}</p>
                <p class="text-muted small mt-1 mb-0">{primaryCurrency()}</p>
              </Show>
            </div>
          </div>
        </div>

        {/* Assets distribution placeholder */}
        <div class="col-12 col-md-6 col-lg-3">
          <div class="card h-100 border shadow-none">
            <div class="card-body d-flex flex-column">
              <p class="text-muted small mb-3">{t('home.assets_distribution')}</p>
              <div class="flex-grow-1 d-flex align-items-center justify-content-center" style="min-height:120px">
                <i class="bi bi-pie-chart text-muted" style="font-size:2.5rem;opacity:0.2"></i>
              </div>
            </div>
          </div>
        </div>

        {/* Balance over time placeholder */}
        <div class="col-12 col-lg-6">
          <div class="card h-100 border shadow-none">
            <div class="card-body d-flex flex-column">
              <p class="text-muted small mb-3">{t('home.balance_over_time')}</p>
              <div class="flex-grow-1 d-flex align-items-center justify-content-center" style="min-height:120px">
                <i class="bi bi-graph-up text-muted" style="font-size:2.5rem;opacity:0.2"></i>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* ── Row 2: Income / Expense summary ───────────────────────────────── */}
      <div class="row g-3 mb-3">

        {/* Income / Expense / Net card */}
        <div class="col-12 col-md-6 col-lg-3">
          <div class="card h-100 border shadow-none">
            <div class="card-body">
              <p class="text-muted small mb-1">{t('home.income')}</p>
              <p class="fs-4 fw-bold mb-0">—</p>
              <p class="text-muted small mb-3">{t('common.no_data')}</p>
              <hr class="my-2" />
              <p class="text-muted small mb-1">{t('home.expenses')}</p>
              <p class="fs-4 fw-bold mb-0">—</p>
              <p class="text-muted small mb-3">{t('common.no_data')}</p>
              <hr class="my-2" />
              <p class="text-muted small mb-1">{t('home.net_income')}</p>
              <p class="fs-4 fw-bold mb-0">—</p>
              <p class="text-muted small mb-0">{t('common.no_data')}</p>
            </div>
          </div>
        </div>

        {/* Top income / expenses placeholder */}
        <div class="col-12 col-md-6 col-lg-3">
          <div class="card h-100 border shadow-none">
            <div class="card-body">
              <p class="text-muted small mb-2">{t('home.top_income')}</p>
              <p class="text-muted small py-2 mb-0">{t('common.no_data')}</p>
              <hr class="my-2" />
              <p class="text-muted small mb-2">{t('home.top_expenses')}</p>
              <p class="text-muted small py-2 mb-0">{t('common.no_data')}</p>
            </div>
          </div>
        </div>

        {/* Income / expense over time placeholder */}
        <div class="col-12 col-lg-6">
          <div class="card h-100 border shadow-none">
            <div class="card-body d-flex flex-column">
              <p class="text-muted small mb-3">{t('home.income_expense_over_time')}</p>
              <div class="flex-grow-1 d-flex align-items-center justify-content-center" style="min-height:120px">
                <i class="bi bi-bar-chart text-muted" style="font-size:2.5rem;opacity:0.2"></i>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* ── Row 3: Recent transactions ────────────────────────────────────── */}
      <div class="card border shadow-none">
        <div class="card-header bg-transparent d-flex flex-wrap gap-2 justify-content-between align-items-center">
          <span class="fw-medium">{t('home.recent_transactions')}</span>
          <input
            type="search"
            class="form-control form-control-sm"
            style="width:200px"
            placeholder={t('home.search')}
            onInput={e => { setSearch(e.currentTarget.value); setPage(0); }}
          />
        </div>

        <div class="card-body p-0">
          <div class="table-responsive">
            <table class="table table-hover mb-0 align-middle">
              <thead class="table-light">
                <tr>
                  <th style="width:48px"></th>
                  <th>{t('home.col_date')}</th>
                  <th>{t('home.col_description')}</th>
                  <th>{t('home.col_exchange_rate')}</th>
                </tr>
              </thead>
              <tbody>
                <Show
                  when={!transactions.loading}
                  fallback={
                    <tr>
                      <td colspan="4" class="text-center py-4">
                        <span class="spinner-border spinner-border-sm text-primary"></span>
                      </td>
                    </tr>
                  }
                >
                  <For
                    each={transactions()?.data}
                    fallback={
                      <tr>
                        <td colspan="4" class="text-center text-muted py-4">{t('home.no_transactions')}</td>
                      </tr>
                    }
                  >
                    {(row) => (
                      <>
                        <tr>
                          <td>
                            <button
                              class="btn btn-sm btn-outline-primary"
                              onClick={() => setExpandedRow(cur => (cur === row.transacId ? null : row.transacId))}
                              title={t('home.view_postings')}
                            >
                              <i class={`bi ${expandedRow() === row.transacId ? 'bi-chevron-up' : 'bi-journal'}`}></i>
                            </button>
                          </td>
                          <td class="text-nowrap">{dayjs(row.transacDate).format('YYYY-MM-DD')}</td>
                          <td>{row.description}</td>
                          <td>{row.exchangeRate}</td>
                        </tr>
                        <Show when={expandedRow() === row.transacId}>
                          <tr class="table-active">
                            <td colspan="4" class="p-0">
                              <div class="p-3">
                                <table class="table table-sm table-bordered mb-0">
                                  <thead>
                                    <tr>
                                      <th>{t('home.col_type')}</th>
                                      <th>{t('home.col_ledger_id')}</th>
                                      <th>{t('home.col_amount')}</th>
                                      <th>{t('home.col_currency')}</th>
                                    </tr>
                                  </thead>
                                  <tbody>
                                    <For each={row.postings}>
                                      {(p) => (
                                        <tr>
                                          <td>{p.postingType === 'D' ? t('home.debit') : t('home.credit')}</td>
                                          <td>{p.ledgerId}</td>
                                          <td>{p.amount}</td>
                                          <td>{p.currencyCode}</td>
                                        </tr>
                                      )}
                                    </For>
                                  </tbody>
                                </table>
                              </div>
                            </td>
                          </tr>
                        </Show>
                      </>
                    )}
                  </For>
                </Show>
              </tbody>
            </table>
          </div>
        </div>

        <div class="card-footer bg-transparent d-flex justify-content-between align-items-center flex-wrap gap-2">
          <small class="text-muted">
            {t('common.records', { count: transactions()?.recordsFiltered ?? 0 })}
          </small>
          <div class="d-flex align-items-center gap-2">
            <button
              class="btn btn-sm btn-outline-secondary"
              onClick={() => setPage(p => Math.max(0, p - 1))}
              disabled={page() === 0 || transactions.loading}
            >
              <i class="bi bi-chevron-left"></i>
            </button>
            <span class="small">
              {t('home.pagination', { page: page() + 1, total: Math.max(1, totalPages()) })}
            </span>
            <button
              class="btn btn-sm btn-outline-secondary"
              onClick={() => setPage(p => p + 1)}
              disabled={page() >= totalPages() - 1 || transactions.loading}
            >
              <i class="bi bi-chevron-right"></i>
            </button>
          </div>
        </div>
      </div>

      {/* ── New Transaction FAB ────────────────────────────────────────────── */}
      <div class="position-fixed bottom-0 end-0 p-4" style="z-index:100">
        <button
          type="button"
          class="btn btn-primary shadow"
          data-bs-toggle="modal"
          data-bs-target="#new-transaction-modal"
        >
          <i class="bi bi-clipboard2-plus me-1"></i>{t('home.new_transaction')}
        </button>
      </div>

      {/* ── New Transaction modal ──────────────────────────────────────────── */}
      <NewTransactionModal username={params.username} />
    </div>
  );
}

// ── New Transaction modal ──────────────────────────────────────────────────────

interface PostingRow {
  id: number;
  ledgerId: string;
  type: 'D' | 'C';
  amount: string;
  currency: string;
}

interface NewTransactionModalProps {
  username: string;
}

function NewTransactionModal(props: NewTransactionModalProps): JSXElement {
  const { t } = useI18n();

  const [ledgers] = createResource(
    () => props.username,
    (u) => fetch(`/${u}/api/ledgers`).then(r => r.json()) as Promise<Ledger[]>,
  );

  const [currencies] = createResource(
    () => props.username,
    (u) => fetch(`/${u}/api/currencies`).then(r => r.json()) as Promise<Currency[]>,
  );

  const [date, setDate] = createSignal(dayjs().format('YYYY-MM-DD'));
  const [description, setDescription] = createSignal('');
  const [nextId, setNextId] = createSignal(3);
  const [postings, setPostings] = createSignal<PostingRow[]>([
    { id: 1, ledgerId: '', type: 'D', amount: '', currency: '' },
    { id: 2, ledgerId: '', type: 'C', amount: '', currency: '' },
  ]);

  const activeLedgers = createMemo(() =>
    (ledgers() ?? []).filter(l => l.ledgerStatus === 1),
  );

  const ledgerGroups = createMemo(() => {
    const map = new Map<string, Ledger[]>();
    for (const l of activeLedgers()) {
      const key = t(`ledger_grades.types.${l.ledgerTypeID}`, {}, l.ledgerType.typeName);
      if (!map.has(key)) map.set(key, []);
      map.get(key)!.push(l);
    }
    return [...map.entries()];
  });

  function updatePosting(id: number, patch: Partial<PostingRow>): void {
    setPostings(rows => rows.map(r => r.id === id ? { ...r, ...patch } : r));
  }

  function addPosting(): void {
    const id = nextId();
    setNextId(id + 1);
    setPostings(rows => [...rows, { id, ledgerId: '', type: 'D', amount: '', currency: '' }]);
  }

  function removePosting(id: number): void {
    setPostings(rows => rows.filter(r => r.id !== id));
  }

  function onLedgerChange(id: number, ledgerId: string): void {
    const ledger = activeLedgers().find(l => String(l.ledgerID) === ledgerId);
    updatePosting(id, { ledgerId, currency: ledger?.currency ?? '' });
  }

  const isBalanced = createMemo(() => {
    const rows = postings();
    const debitSum = rows.filter(r => r.type === 'D').reduce((s, r) => s + (parseFloat(r.amount) || 0), 0);
    const creditSum = rows.filter(r => r.type === 'C').reduce((s, r) => s + (parseFloat(r.amount) || 0), 0);
    return debitSum > 0 && Math.abs(debitSum - creditSum) < 0.001;
  });

  return (
    <div class="modal fade" id="new-transaction-modal" tabindex="-1">
      <div class="modal-dialog modal-dialog-centered modal-dialog-scrollable modal-lg">
        <div class="modal-content">
          <div class="modal-header">
            <h5 class="modal-title fw-semibold">{t('home.new_transaction')}</h5>
            <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
          </div>

          <div class="modal-body">
            <div class="row g-3 mb-4">
              <div class="col-sm-4">
                <label class="form-label small fw-medium">
                  {t('home.date_label')} <span class="text-danger">*</span>
                </label>
                <input
                  type="date"
                  class="form-control"
                  value={date()}
                  onInput={e => setDate(e.currentTarget.value)}
                />
              </div>
              <div class="col-sm-8">
                <label class="form-label small fw-medium">
                  {t('home.description_label')} <span class="text-danger">*</span>
                </label>
                <input
                  type="text"
                  class="form-control"
                  placeholder={t('home.description_placeholder')}
                  value={description()}
                  onInput={e => setDescription(e.currentTarget.value)}
                />
              </div>
            </div>

            <div class="d-flex justify-content-between align-items-center mb-2">
              <span class="small fw-medium">{t('home.journal_entries')}</span>
              <Show when={!isBalanced() && postings().some(r => r.amount !== '')}>
                <span class="badge bg-warning text-dark small">
                  <i class="bi bi-exclamation-triangle me-1"></i>{t('home.unbalanced')}
                </span>
              </Show>
              <Show when={isBalanced()}>
                <span class="badge bg-success small">
                  <i class="bi bi-check-circle me-1"></i>{t('home.balanced')}
                </span>
              </Show>
            </div>

            <div class="table-responsive">
              <table class="table table-sm align-middle mb-2">
                <thead class="table-light">
                  <tr>
                    <th>{t('home.col_ledger_id')}</th>
                    <th style="width:90px">{t('home.col_type')}</th>
                    <th style="width:130px">{t('home.col_amount')}</th>
                    <th style="width:110px">{t('home.col_currency')}</th>
                    <th style="width:40px"></th>
                  </tr>
                </thead>
                <tbody>
                  <For each={postings()}>
                    {(row) => (
                      <tr>
                        <td>
                          <select
                            class="form-select form-select-sm"
                            value={row.ledgerId}
                            onChange={e => onLedgerChange(row.id, e.currentTarget.value)}
                          >
                            <option value="">{t('home.select_ledger')}</option>
                            <For each={ledgerGroups()}>
                              {([typeName, group]) => (
                                <optgroup label={typeName}>
                                  <For each={group}>
                                    {(l) => <option value={String(l.ledgerID)}>{l.ledgerName}</option>}
                                  </For>
                                </optgroup>
                              )}
                            </For>
                          </select>
                        </td>
                        <td>
                          <select
                            class="form-select form-select-sm"
                            value={row.type}
                            onChange={e => updatePosting(row.id, { type: e.currentTarget.value as 'D' | 'C' })}
                          >
                            <option value="D">{t('home.debit')}</option>
                            <option value="C">{t('home.credit')}</option>
                          </select>
                        </td>
                        <td>
                          <input
                            type="number"
                            class="form-control form-control-sm"
                            placeholder="0.00"
                            value={row.amount}
                            step="0.01"
                            min="0"
                            onInput={e => updatePosting(row.id, { amount: e.currentTarget.value })}
                          />
                        </td>
                        <td>
                          <select
                            class="form-select form-select-sm"
                            value={row.currency}
                            onChange={e => updatePosting(row.id, { currency: e.currentTarget.value })}
                          >
                            <option value="">—</option>
                            <For each={currencies() ?? []}>
                              {(c) => (
                                <option value={c.alphabeticCode}>
                                  {c.alphabeticCode}
                                </option>
                              )}
                            </For>
                          </select>
                        </td>
                        <td>
                          <Show when={postings().length > 2}>
                            <button
                              class="btn btn-sm btn-outline-danger"
                              onClick={() => removePosting(row.id)}
                              title="Remove"
                            >
                              <i class="bi bi-x"></i>
                            </button>
                          </Show>
                        </td>
                      </tr>
                    )}
                  </For>
                </tbody>
              </table>
            </div>

            <button class="btn btn-sm btn-outline-secondary" onClick={addPosting}>
              <i class="bi bi-plus me-1"></i>{t('home.add_line')}
            </button>
          </div>

          <div class="modal-footer">
            <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">
              {t('common.cancel')}
            </button>
            <button type="button" class="btn btn-primary" disabled title="Transaction creation API coming soon">
              {t('home.save_transaction')}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
