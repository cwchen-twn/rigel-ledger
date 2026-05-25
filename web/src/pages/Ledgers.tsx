import { createSignal, createResource, For, Show, type JSXElement } from 'solid-js';
import { useParams } from '@solidjs/router';
import { Modal } from 'bootstrap';
import type { Ledger, LedgerType, Currency } from '../types';
import SearchableSelect, { type SelectOption } from '../components/SearchableSelect';
import { sideAlert, centerAlert } from '../notifications';

export default function Ledgers(): JSXElement {
  const params = useParams<{ username: string }>();

  const [ledgers, { refetch: refetchLedgers }] = createResource(
    () => params.username,
    async (u) => fetch(`/${u}/api/ledgers`).then(r => r.json()) as Promise<Ledger[]>,
  );

  const [ledgerTypes] = createResource(
    () => params.username,
    async (u) => fetch(`/${u}/api/ledger-types`).then(r => r.json()) as Promise<LedgerType[]>,
  );

  const [currencies] = createResource(
    () => params.username,
    async (u) => fetch(`/${u}/api/currencies`).then(r => r.json()) as Promise<Currency[]>,
  );

  // New ledger form
  const [newName, setNewName] = createSignal('');
  const [newTypeID, setNewTypeID] = createSignal('');
  const [newCurrency, setNewCurrency] = createSignal('');
  const [newBalance, setNewBalance] = createSignal('0.00');
  const [saving, setSaving] = createSignal(false);

  // Edit state
  const [editingID, setEditingID] = createSignal<number | null>(null);
  const [editName, setEditName] = createSignal('');
  const [editTypeID, setEditTypeID] = createSignal('');
  const [editCurrency, setEditCurrency] = createSignal('');
  const [editBalance, setEditBalance] = createSignal('');
  const [editStatus, setEditStatus] = createSignal(1);
  const [editSaving, setEditSaving] = createSignal(false);

  const typeOptions = (): SelectOption[] =>
    (ledgerTypes() ?? []).map(t => ({ value: t.ledgerTypeID, label: t.typeName }));

  const currencyOptions = (): SelectOption[] =>
    (currencies() ?? []).map(c => ({ value: c.alphabeticCode, label: `${c.alphabeticCode} – ${c.currencyName}` }));

  function startEdit(ledger: Ledger): void {
    if (editingID() === ledger.ledgerID) { setEditingID(null); return; }
    setEditingID(ledger.ledgerID);
    setEditName(ledger.ledgerName);
    setEditTypeID(ledger.ledgerTypeID);
    setEditCurrency(ledger.currency);
    setEditBalance(ledger.balance);
    setEditStatus(ledger.ledgerStatus);
  }

  async function createLedger(): Promise<void> {
    if (!newName().trim() || !newTypeID() || !newCurrency()) {
      await sideAlert.fire({ icon: 'warning', title: 'Please fill all required fields' });
      return;
    }
    setSaving(true);
    try {
      const resp = await fetch(`/${params.username}/api/ledgers`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify([{
          ledgerName: newName().trim(),
          ledgerTypeID: newTypeID(),
          currency: newCurrency(),
          balance: parseFloat(newBalance() || '0').toFixed(2),
        }]),
      });
      if (!resp.ok) {
        void sideAlert.fire({ icon: 'error', title: 'Error', text: await resp.text() });
        return;
      }
      void sideAlert.fire({ icon: 'success', title: 'Ledger created' });
      setNewName(''); setNewTypeID(''); setNewCurrency(''); setNewBalance('0.00');
      const el = document.getElementById('new-ledger-modal');
      if (el) Modal.getInstance(el)?.hide();
      refetchLedgers();
    } finally {
      setSaving(false);
    }
  }

  async function saveLedger(ledger: Ledger): Promise<void> {
    setEditSaving(true);
    try {
      const resp = await fetch(`/${params.username}/api/ledgers`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify([{
          ledgerID: ledger.ledgerID,
          ledgerName: editName().trim(),
          ledgerTypeID: editTypeID(),
          currency: editCurrency(),
          balance: parseFloat(editBalance() || '0').toFixed(2),
          ledgerStatus: editStatus(),
        }]),
      });
      if (!resp.ok) {
        void sideAlert.fire({ icon: 'error', title: 'Error', text: await resp.text() });
        return;
      }
      void sideAlert.fire({ icon: 'success', title: 'Ledger updated' });
      setEditingID(null);
      refetchLedgers();
    } finally {
      setEditSaving(false);
    }
  }

  async function deleteLedger(id: number): Promise<void> {
    const result = await centerAlert.fire({
      title: 'Delete ledger?',
      text: 'This action cannot be undone.',
      icon: 'warning',
      showCancelButton: true,
      confirmButtonColor: '#dc3545',
      confirmButtonText: 'Delete',
    });
    if (!result.isConfirmed) return;

    const resp = await fetch(`/${params.username}/api/ledger/${id}`, { method: 'DELETE' });
    if (!resp.ok) {
      void sideAlert.fire({ icon: 'error', title: 'Error', text: await resp.text() });
      return;
    }
    void sideAlert.fire({ icon: 'success', title: 'Ledger deleted' });
    refetchLedgers();
  }

  return (
    <div class="container-fluid py-4">
      <div class="d-flex justify-content-between align-items-center mb-4">
        <h4 class="mb-0">Ledgers</h4>
        <button
          class="btn btn-primary btn-sm"
          data-bs-toggle="modal"
          data-bs-target="#new-ledger-modal"
        >
          <i class="bi bi-plus-lg me-1"></i>New Ledger
        </button>
      </div>

      <div class="card">
        <div class="card-body p-0">
          <div class="table-responsive">
            <table class="table table-hover mb-0 align-middle">
              <thead class="table-light">
                <tr>
                  <th style="width:80px">Actions</th>
                  <th>Name</th>
                  <th>Type</th>
                  <th>Currency</th>
                  <th>Balance</th>
                  <th>Status</th>
                  <th>Created</th>
                </tr>
              </thead>
              <tbody>
                <Show
                  when={!ledgers.loading}
                  fallback={
                    <tr>
                      <td colspan="7" class="text-center py-4">
                        <span class="spinner-border spinner-border-sm text-primary"></span>
                      </td>
                    </tr>
                  }
                >
                  <For
                    each={ledgers()}
                    fallback={
                      <tr>
                        <td colspan="7" class="text-center text-muted py-4">No ledgers found</td>
                      </tr>
                    }
                  >
                    {(ledger) => (
                      <>
                        <tr>
                          <td>
                            <div class="d-flex gap-1">
                              <Show when={ledger.postingsCount === 0}>
                                <button
                                  class="btn btn-sm btn-danger"
                                  onClick={() => deleteLedger(ledger.ledgerID)}
                                  title="Delete"
                                >
                                  <i class="bi bi-trash"></i>
                                </button>
                              </Show>
                              <button
                                class={`btn btn-sm ${editingID() === ledger.ledgerID ? 'btn-secondary' : 'btn-warning'}`}
                                onClick={() => startEdit(ledger)}
                                title="Edit"
                              >
                                <i class={`bi ${editingID() === ledger.ledgerID ? 'bi-x' : 'bi-pencil-square'}`}></i>
                              </button>
                            </div>
                          </td>
                          <td>{ledger.ledgerName}</td>
                          <td class="small">{ledger.ledgerType.typeName}</td>
                          <td>{ledger.currency}</td>
                          <td class="text-end">{ledger.balance}</td>
                          <td>
                            <span class={`badge ${ledger.ledgerStatus === 1 ? 'bg-success' : 'bg-secondary'}`}>
                              {ledger.ledgerStatus === 1 ? 'Active' : 'Inactive'}
                            </span>
                          </td>
                          <td class="text-muted small">{ledger.createdAt.slice(0, 10)}</td>
                        </tr>

                        <Show when={editingID() === ledger.ledgerID}>
                          <tr class="table-active">
                            <td colspan="7">
                              <div class="p-2 d-flex flex-column gap-2" style="max-width:480px">
                                <div class="row g-2">
                                  <div class="col-12">
                                    <label class="form-label small mb-1">Name</label>
                                    <input
                                      type="text"
                                      class="form-control form-control-sm"
                                      value={editName()}
                                      onInput={e => setEditName(e.currentTarget.value)}
                                    />
                                  </div>
                                  <div class="col-sm-6">
                                    <label class="form-label small mb-1">Type</label>
                                    <SearchableSelect
                                      options={typeOptions()}
                                      value={editTypeID()}
                                      onChange={setEditTypeID}
                                      disabled={ledger.postingsCount > 0}
                                    />
                                  </div>
                                  <div class="col-sm-6">
                                    <label class="form-label small mb-1">Currency</label>
                                    <SearchableSelect
                                      options={currencyOptions()}
                                      value={editCurrency()}
                                      onChange={setEditCurrency}
                                      disabled={ledger.postingsCount > 0}
                                    />
                                  </div>
                                  <div class="col-sm-6">
                                    <label class="form-label small mb-1">Balance</label>
                                    <input
                                      type="number"
                                      class="form-control form-control-sm"
                                      value={editBalance()}
                                      onInput={e => setEditBalance(e.currentTarget.value)}
                                      step="0.01"
                                      min="0"
                                      disabled={ledger.postingsCount > 0}
                                    />
                                  </div>
                                  <div class="col-sm-6">
                                    <label class="form-label small mb-1">Status</label>
                                    <select
                                      class="form-select form-select-sm"
                                      value={editStatus()}
                                      onChange={e => setEditStatus(parseInt(e.currentTarget.value))}
                                    >
                                      <option value="1">Active</option>
                                      <option value="0">Inactive</option>
                                    </select>
                                  </div>
                                </div>
                                <div class="d-flex gap-2">
                                  <button
                                    class="btn btn-primary btn-sm"
                                    onClick={() => saveLedger(ledger)}
                                    disabled={editSaving()}
                                  >
                                    <Show when={editSaving()} fallback={<>Save</>}>
                                      <span class="spinner-border spinner-border-sm me-1"></span>Saving…
                                    </Show>
                                  </button>
                                  <button class="btn btn-secondary btn-sm" onClick={() => setEditingID(null)}>
                                    Cancel
                                  </button>
                                </div>
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
      </div>

      {/* New Ledger Modal */}
      <div class="modal fade" id="new-ledger-modal" tabindex="-1">
        <div class="modal-dialog">
          <div class="modal-content">
            <div class="modal-header">
              <h5 class="modal-title">New Ledger</h5>
              <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
            </div>
            <div class="modal-body">
              <div class="d-flex flex-column gap-3">
                <div>
                  <label class="form-label">Name <span class="text-danger">*</span></label>
                  <input
                    type="text"
                    class="form-control"
                    value={newName()}
                    onInput={e => setNewName(e.currentTarget.value)}
                    placeholder="e.g. Checking Account"
                  />
                </div>
                <div>
                  <label class="form-label">Type <span class="text-danger">*</span></label>
                  <SearchableSelect
                    options={typeOptions()}
                    value={newTypeID()}
                    onChange={setNewTypeID}
                    placeholder="Select ledger type…"
                  />
                </div>
                <div>
                  <label class="form-label">Currency <span class="text-danger">*</span></label>
                  <SearchableSelect
                    options={currencyOptions()}
                    value={newCurrency()}
                    onChange={setNewCurrency}
                    placeholder="Select currency…"
                  />
                </div>
                <div>
                  <label class="form-label">Initial Balance</label>
                  <input
                    type="number"
                    class="form-control"
                    value={newBalance()}
                    onInput={e => setNewBalance(e.currentTarget.value)}
                    step="0.01"
                    min="0"
                  />
                </div>
              </div>
            </div>
            <div class="modal-footer">
              <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Cancel</button>
              <button type="button" class="btn btn-primary" onClick={createLedger} disabled={saving()}>
                <Show when={saving()} fallback={<>Create</>}>
                  <span class="spinner-border spinner-border-sm me-1"></span>Creating…
                </Show>
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
