import { createSignal, createResource, For, Show, type JSXElement } from 'solid-js';
import { useParams } from '@solidjs/router';
import dayjs from 'dayjs';
import type { Journal, DataTableResponse } from '../types';

const PAGE_SIZE = 10;

export default function Home(): JSXElement {
  const params = useParams<{ username: string }>();
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

  function toggleRow(id: number): void {
    setExpandedRow(cur => (cur === id ? null : id));
  }

  return (
    <div class="container-fluid py-4">
      <h4 class="mb-4">Dashboard</h4>

      <div class="card">
        <div class="card-header d-flex flex-wrap gap-2 justify-content-between align-items-center">
          <h6 class="mb-0">Recent Transactions</h6>
          <input
            type="search"
            class="form-control form-control-sm"
            style="width:200px"
            placeholder="Search…"
            onInput={e => { setSearch(e.currentTarget.value); setPage(0); }}
          />
        </div>

        <div class="card-body p-0">
          <div class="table-responsive">
            <table class="table table-hover mb-0 align-middle">
              <thead class="table-light">
                <tr>
                  <th style="width:48px"></th>
                  <th>Date</th>
                  <th>Description</th>
                  <th>Exchange Rate</th>
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
                        <td colspan="4" class="text-center text-muted py-4">No transactions found</td>
                      </tr>
                    }
                  >
                    {(row) => (
                      <>
                        <tr>
                          <td>
                            <button
                              class="btn btn-sm btn-outline-primary"
                              onClick={() => toggleRow(row.transacId)}
                              title="View postings"
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
                                      <th>Type</th>
                                      <th>Ledger ID</th>
                                      <th>Amount</th>
                                      <th>Currency</th>
                                    </tr>
                                  </thead>
                                  <tbody>
                                    <For each={row.postings}>
                                      {(p) => (
                                        <tr>
                                          <td>{p.postingType === 'D' ? 'Debit' : 'Credit'}</td>
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

        <div class="card-footer d-flex justify-content-between align-items-center flex-wrap gap-2">
          <small class="text-muted">
            {transactions()?.recordsFiltered ?? 0} record(s)
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
              Page {page() + 1} / {Math.max(1, totalPages())}
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
    </div>
  );
}
