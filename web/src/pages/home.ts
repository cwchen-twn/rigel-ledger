import DataTable from 'datatables.net-bs5';
import dayjs from 'dayjs';
import type { Journal } from '../types';

export function initHomePage(username: string): void {
  document.addEventListener('DOMContentLoaded', () => {
    const table = new DataTable<Journal>('#transactions-table', {
      pageLength: 10,
      responsive: true,
      ajax: {
        url: `/${username}/api/transactions`,
        dataSrc: (json: { data: Journal[]; draw: number; total: number; filtered: number }) => json.data,
        method: 'POST',
        error: (xhr: XMLHttpRequest, _status: string, err: string) => {
          console.error('DataTable AJAX error', xhr.status, err);
        },
      },
      serverSide: true,
      processing: true,
      columns: [
        {
          defaultContent: '<button class="btn btn-primary" group="viewtx"><i class="bi bi-journal"></i></button>',
          orderable: false,
          searchable: false,
        },
        { title: 'Transaction ID', data: 'transacId', visible: false, searchable: false },
        {
          title: 'Transaction Date',
          data: (row: Journal, type: string) => {
            if (type === 'sort') return dayjs(row.transacDate).unix();
            return row.transacDate;
          },
        },
        { title: 'Description', data: 'description' },
        { title: 'Exchange Rate', data: 'exchangeRate', orderable: false, searchable: false },
      ],
      order: [[1, 'desc']],
    });

    table.on('click', 'button[group="viewtx"]', function (this: HTMLElement) {
      const tr = this.closest('tr')!;
      const targetTr = tr.classList.contains('child') ? (tr.previousElementSibling as HTMLTableRowElement) : tr;
      const row = table.row(targetTr);
      if (row.child.isShown()) { row.child.hide(); return; }
      const rowData = row.data() as Journal;
      const childView = `
        <div class="table-responsive">
          <table class="table table-striped table-bordered">
            <thead><tr><th>Type</th><th>Ledger</th><th>Amount</th><th>Currency</th></tr></thead>
            <tbody>
              ${rowData.postings.map(p => `
                <tr>
                  <td>${p.postingType === 'D' ? 'Debit' : 'Credit'}</td>
                  <td>${p.ledgerId}</td>
                  <td>${p.amount}</td>
                  <td>${p.currencyCode}</td>
                </tr>`).join('')}
            </tbody>
          </table>
        </div>`;
      row.child(childView).show();
    });
  });
}
