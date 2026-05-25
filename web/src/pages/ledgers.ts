import Alpine from 'alpinejs';
import DataTable from 'datatables.net-bs5';
import TomSelect from 'tom-select';
import { sideAlert } from '../notifications';
import type { Ledger, LedgerType, LedgerTypeFirstGrade, Currency } from '../types';
import type { Api as DataTableApi } from 'datatables.net';

// Module-level state shared between Alpine components and DataTable handlers
let table: DataTableApi<Ledger> | null = null;
let ledgerTypeSel: TomSelect | null = null;
let currencySel: TomSelect | null = null;
let ledgerTypesOptions: LedgerType[] = [];
let currenciesOptions: Currency[] = [];

const tomSelectConfig = {
  create: false,
  maxItems: 1,
  loadThrottle: 100,
  preload: 'focus' as const,
};

function buildChildRow(row: Ledger, username: string): string {
  return `
<form
  method="post"
  class="needs-validation d-flex flex-column gap-3 col-12 col-md-5"
  novalidate
  x-init="init()"
  x-data="{
    ledgerID: ${row.ledgerID},
    ledgerName: '${row.ledgerName}',
    ledgerTypeID: '${row.ledgerTypeID}',
    currency: '${row.currency}',
    balance: '${row.balance}',
    ledgerStatus: ${row.ledgerStatus},
    initialized: false,
    validateField(element) {
      const ele = [...element.classList].includes('tomselected') ? element.nextElementSibling : element;
      if (!ele) return;
      if (!element.checkValidity()) { ele.classList.add('is-invalid'); ele.classList.remove('is-valid'); return; }
      ele.classList.add('is-valid'); ele.classList.remove('is-invalid');
    },
    async save(el) {
      const form = el.closest('form');
      if (!form.checkValidity()) { form.querySelectorAll('[required]').forEach(e => this.validateField(e)); return; }
      form.classList.add('was-validated');
      const response = await fetch('/${username}/api/ledgers', {
        method: 'PUT',
        body: JSON.stringify([{ ledgerID: this.ledgerID, ledgerName: this.ledgerName, ledgerTypeID: this.ledgerTypeID, currency: this.currency, balance: parseFloat(this.balance).toFixed(2), ledgerStatus: parseInt(this.ledgerStatus) }]),
      });
      if (!response.ok) { sideAlert.fire({ icon: 'error', title: 'Error', text: await response.text() }); return; }
      sideAlert.fire({ icon: 'success', title: 'Success', text: 'Ledger updated successfully' });
      table?.ajax.reload();
    },
    cancel(el) { const row = el.closest('tr'); if (row && table) { const r = table.row(row); r.child.hide(); } },
    init() {
      if (this.initialized) return;
      this.initialized = true;
      if (${row.postingsCount} > 0) return;
      new TomSelect('#ledgertype-${row.ledgerID}', { ...${JSON.stringify(tomSelectConfig)}, options: ledgerTypesOptions, items: [this.ledgerTypeID], valueField: 'ledgerTypeID', labelField: 'typeName', searchField: ['typeName'] });
      new TomSelect('#currency-${row.ledgerID}', { ...${JSON.stringify(tomSelectConfig)}, options: currenciesOptions, items: [this.currency], valueField: 'alphabeticCode', labelField: 'currencyName', searchField: ['currencyName'] });
    },
  }"
>
  <div class="form-group">
    <label for="ledger-name-${row.ledgerID}">Ledger Name</label>
    <input type="text" class="form-control" id="ledger-name-${row.ledgerID}" x-model="ledgerName" x-on:input="validateField($el)" required>
    <div class="invalid-feedback">Please enter a ledger name</div>
  </div>
  <div class="form-group">
    <label for="ledgertype-${row.ledgerID}">Ledger Type</label>
    <select class="form-control" id="ledgertype-${row.ledgerID}" x-model="ledgerTypeID" autocomplete="off" x-on:change="validateField($el)" ${row.postingsCount === 0 ? '' : 'disabled'} required>
      <template x-if="${row.postingsCount} > 0"><option value="${row.ledgerTypeID}">${ledgerTypesOptions.find(l => l.ledgerTypeID === row.ledgerTypeID)?.typeName ?? ''}</option></template>
    </select>
    <div class="invalid-feedback">Please select a ledger type</div>
  </div>
  <div class="form-group">
    <label for="currency-${row.ledgerID}">Currency</label>
    <select class="form-control" id="currency-${row.ledgerID}" x-model="currency" autocomplete="off" x-on:change="validateField($el)" ${row.postingsCount === 0 ? '' : 'disabled'} required>
      <template x-if="${row.postingsCount} > 0"><option value="${row.currency}">${currenciesOptions.find(c => c.alphabeticCode === row.currency)?.currencyName ?? ''}</option></template>
    </select>
    <div class="invalid-feedback">Please select a currency</div>
  </div>
  <div class="form-group">
    <label for="balance-${row.ledgerID}">Balance</label>
    <input type="number" class="form-control" id="balance-${row.ledgerID}" x-model="balance" autocomplete="off" min="0" step="0.01" ${row.postingsCount === 0 ? '' : 'disabled'} x-on:input="validateField($el)" required>
    <div class="invalid-feedback">Please enter a balance</div>
  </div>
  <div class="form-group">
    <button type="button" class="btn btn-primary" x-on:click="save($el)">Submit</button>
    <button type="button" class="btn btn-secondary" x-on:click="cancel($el)">Cancel</button>
  </div>
</form>`;
}

Alpine.data('ledgerData', () => ({
  ledgerName: '' as string,
  ledgerTypeID: '' as string,
  currency: '' as string,
  balance: '0.00' as string,

  cleanInput(): void {
    this.ledgerName = '';
    this.ledgerTypeID = '';
    this.currency = '';
    this.balance = '0.00';
    ledgerTypeSel?.clear();
    currencySel?.clear();
    const form = document.getElementById('new-ledger-form') as HTMLFormElement | null;
    if (!form) return;
    form.classList.remove('was-validated');
    form.querySelectorAll<HTMLElement>('[required]').forEach(el => {
      const target = [...el.classList].includes('tomselected') ? el.nextElementSibling as HTMLElement : el;
      target?.classList.remove('is-invalid', 'is-valid');
    });
  },

  validateField(element: HTMLElement): void {
    const input = element as HTMLInputElement;
    const target = [...element.classList].includes('tomselected') ? element.nextElementSibling as HTMLElement : element;
    if (!target) return;
    if (!input.checkValidity()) {
      target.classList.add('is-invalid');
      target.classList.remove('is-valid');
    } else {
      target.classList.add('is-valid');
      target.classList.remove('is-invalid');
    }
  },

  async save(): Promise<void> {
    const form = document.getElementById('new-ledger-form') as HTMLFormElement;
    if (!form.checkValidity()) {
      form.querySelectorAll<HTMLElement>('[required]').forEach(el => this.validateField(el));
      return;
    }
    form.classList.add('was-validated');
    const username = window.__APP_CONFIG__.username;
    const response = await fetch(`/${username}/api/ledgers`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify([{
        ledgerName: this.ledgerName,
        ledgerTypeID: this.ledgerTypeID,
        currency: this.currency,
        balance: parseFloat(this.balance).toFixed(2),
      }]),
    });
    if (!response.ok) {
      sideAlert.fire({ icon: 'error', title: 'Error', text: await response.text() });
      return;
    }
    sideAlert.fire({ icon: 'success', title: 'Success', text: 'Ledger created successfully' });
    table?.ajax.reload();
    this.cleanInput();
    const modal = document.getElementById('new-ledger-modal');
    if (modal) {
      const bsModal = (window as unknown as { bootstrap: { Modal: { getInstance: (el: Element) => { hide(): void } | null } } }).bootstrap.Modal.getInstance(modal);
      bsModal?.hide();
    }
  },
}));

export function initLedgersPage(username: string): void {
  document.addEventListener('DOMContentLoaded', async () => {
    const [firstGradeOptions, fetchedLedgerTypes, fetchedCurrencies] = await Promise.all([
      fetch(`/${username}/api/ledger-types/firstgrade`).then(r => r.json()) as Promise<LedgerTypeFirstGrade[]>,
      fetch(`/${username}/api/ledger-types`).then(r => r.json()) as Promise<LedgerType[]>,
      fetch(`/${username}/api/currencies`).then(r => r.json()) as Promise<Currency[]>,
    ]);
    ledgerTypesOptions = fetchedLedgerTypes;
    currenciesOptions = fetchedCurrencies;

    table = new DataTable<Ledger>('#ledgers-table', {
      pageLength: 10,
      responsive: true,
      ajax: {
        dataSrc: '',
        url: `/${username}/api/ledgers`,
        method: 'GET',
        error: (xhr: XMLHttpRequest, _status: string, err: string) => {
          console.error('DataTable AJAX error', xhr.status, err);
        },
      },
      columns: [
        { data: 'ledgerID', visible: false, searchable: false, orderable: false },
        {
          data: (row: Ledger) => {
            const del = row.postingsCount === 0
              ? `<button type="button" title="Delete" class="btn btn-sm btn-danger" name="deleteledger"><i class="bi bi-trash"></i></button>`
              : '';
            const edit = `<button type="button" title="Edit" class="btn btn-sm btn-warning" name="editledger"><i class="bi bi-pencil-square"></i></button>`;
            return `<div class="d-flex gap-1">${del} ${edit}</div>`;
          },
          orderable: false,
          searchable: false,
        },
        { title: 'Ledger Name', data: 'ledgerName' },
        {
          title: 'Ledger Type',
          data: (row: Ledger, type: string) => {
            if (type === 'display' || type === 'filter') return row.ledgerType.typeName;
            return row.ledgerType as unknown as string;
          },
        },
        { title: 'Currency', data: 'currency' },
        { title: 'Balance', data: 'balance' },
        {
          title: 'Status',
          data: (row: Ledger, type: string) => {
            const label = row.ledgerStatus === 1 ? 'Active' : 'Inactive';
            if (type === 'display' || type === 'filter') return label;
            return row.ledgerStatus as unknown as string;
          },
        },
        { title: 'Created At', data: 'createdAt' },
        { title: 'Updated At', data: 'updatedAt' },
      ],
      order: [[0, 'asc']],
    });

    table.on('click', 'button[name="deleteledger"]', async function (this: HTMLElement) {
      const tr = this.closest('tr')!;
      const targetTr = tr.classList.contains('child') ? (tr.previousElementSibling as HTMLTableRowElement) : tr;
      const rowData = (table as DataTableApi<Ledger>).row(targetTr).data() as Ledger;
      try {
        const resp = await fetch(`/${username}/api/ledger/${rowData.ledgerID}`, { method: 'DELETE' });
        if (!resp.ok) { sideAlert.fire({ icon: 'error', title: 'Error', text: resp.statusText }); return; }
        sideAlert.fire({ icon: 'success', title: 'Success', text: 'Ledger deleted successfully' });
        (table as DataTableApi<Ledger>).ajax.reload();
      } catch (err) {
        sideAlert.fire({ icon: 'error', title: 'Error', text: String(err) });
      }
    });

    table.on('click', 'button[name="editledger"]', function (this: HTMLElement) {
      const tr = this.closest('tr')!;
      const targetTr = tr.classList.contains('child') ? (tr.previousElementSibling as HTMLTableRowElement) : tr;
      const rowApi = (table as DataTableApi<Ledger>).row(targetTr);
      if (rowApi.child.isShown()) { rowApi.child.hide(); return; }
      rowApi.child(buildChildRow(rowApi.data() as Ledger, username)).show();
    });

    const modal = document.getElementById('new-ledger-modal');
    if (modal) {
      modal.addEventListener('hide.bs.modal', () => {
        const focused = document.activeElement as HTMLElement | null;
        if (focused && modal.contains(focused)) focused.blur();
      });
    }

    ledgerTypeSel = new TomSelect('#ledgertype', {
      ...tomSelectConfig,
      options: ledgerTypesOptions,
      optgroups: firstGradeOptions,
      optgroupField: 'firstGrade',
      optgroupLabelField: 'typeName',
      optgroupValueField: 'firstGrade',
      valueField: 'ledgerTypeID',
      labelField: 'typeName',
      searchField: ['typeName'],
      render: {
        optgroup: (data: { options: DocumentFragment }) => {
          const div = document.createElement('div');
          div.className = 'optgroup';
          div.appendChild(data.options);
          return div;
        },
        optgroup_header: (data: { typeName: string }, escape: (s: string) => string) =>
          `<div class="optgroup-header text-muted fst-italic">${escape(data.typeName)}</div>`,
      },
    });

    currencySel = new TomSelect('#currency', {
      ...tomSelectConfig,
      options: currenciesOptions,
      valueField: 'alphabeticCode',
      labelField: 'currencyName',
      searchField: ['currencyName'],
    });
  });
}
