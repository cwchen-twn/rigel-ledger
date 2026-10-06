package ledger

import (
	"testing"
	"time"
)

// One purchase, however many sources see it (#53-#55).

func (f *fixture) countTransactions(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.store.Pool.QueryRow(f.ctx, `SELECT count(*) FROM transactions WHERE book_id = $1`, f.acc.Book.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func alertBatch(rows ...ImportRowInput) ImportInput {
	return ImportInput{Connector: "xx-mail", Accounts: []ImportAccount{{ExternalID: "card-tw-cathaybk-4321", Currency: "TWD"}}, Rows: rows}
}

func statementBatch(rows ...ImportRowInput) ImportInput {
	return ImportInput{Connector: "tw-cathaybk", Accounts: []ImportAccount{{ExternalID: "card", Currency: "TWD"}}, Rows: rows}
}

func TestTheSameChargeFromTwoSourcesIsOne(t *testing.T) {
	f := setup(t)
	alert := ImportRowInput{Kind: "transaction", Account: "card-tw-cathaybk-4321", ID: "alert-1", Date: day("2026-09-04"), Amount: d("-350"), Description: "全聯", Pending: true}
	posted := ImportRowInput{Kind: "transaction", Account: "card", ID: "s-1", Date: day("2026-09-05"), Amount: d("-350"), Description: "全聯福利中心"}
	if _, err := f.svc.Import(f.ctx, f.acc, alertBatch(alert)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Import(f.ctx, f.acc, statementBatch(posted)); err != nil {
		t.Fatal(err)
	}
	f.mapSource(t, "card-tw-cathaybk-4321", f.keys["credit_card"])
	f.mapSource(t, "card", f.keys["credit_card"])

	q := f.queue(t)
	a, s := q["xx-mail:alert-1"], q["tw-cathaybk:s-1"]
	// The posted row books; the alert, the estimate, goes with it.
	if a.Proposal != "duplicate" || a.MatchRowID == nil || *a.MatchRowID != s.ID || s.Proposal != "new" {
		t.Fatalf("alert %s %v, statement %s", a.Proposal, a.MatchRowID, s.Proposal)
	}
	_, err := f.svc.AcceptRow(f.ctx, f.acc, a.ID, nil)
	wantCode(t, err, "match_pending")
	groceries := f.keys["groceries"]
	txn, err := f.svc.AcceptRow(f.ctx, f.acc, s.ID, &groceries)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.queue(t)) != 0 || f.countTransactions(t) != 1 {
		t.Fatalf("queue %d rows, %d transactions: want both rows settled as one", len(f.queue(t)), f.countTransactions(t))
	}
	var settled *int64
	_ = f.store.Pool.QueryRow(f.ctx, `SELECT transaction_id FROM import_rows WHERE id = $1`, a.ID).Scan(&settled)
	if settled == nil || *settled != txn {
		t.Fatalf("the alert row was settled with %v, want %d", settled, txn)
	}
}

func TestDuplicateRowsInOtherOrdersAndFromOneSource(t *testing.T) {
	f := setup(t)
	// The statement first, then the alert: the alert is still the one that waits.
	if _, err := f.svc.Import(f.ctx, f.acc, statementBatch(
		ImportRowInput{Kind: "transaction", Account: "card", ID: "s-1", Date: day("2026-09-05"), Amount: d("-350")},
		// one source's two identical coffees are two coffees
		ImportRowInput{Kind: "transaction", Account: "card", ID: "s-2", Date: day("2026-09-06"), Amount: d("-120")},
		ImportRowInput{Kind: "transaction", Account: "card", ID: "s-3", Date: day("2026-09-06"), Amount: d("-120")},
	)); err != nil {
		t.Fatal(err)
	}
	f.mapSource(t, "card", f.keys["credit_card"])
	if _, err := f.svc.Import(f.ctx, f.acc, alertBatch(
		ImportRowInput{Kind: "transaction", Account: "card-tw-cathaybk-4321", ID: "alert-1", Date: day("2026-09-04"), Amount: d("-350"), Pending: true},
	)); err != nil {
		t.Fatal(err)
	}
	f.mapSource(t, "card-tw-cathaybk-4321", f.keys["credit_card"])
	q := f.queue(t)
	if p := q["xx-mail:alert-1"]; p.Proposal != "duplicate" || *p.MatchRowID != q["tw-cathaybk:s-1"].ID {
		t.Fatalf("alert = %s %v", p.Proposal, p.MatchRowID)
	}
	if q["tw-cathaybk:s-2"].Proposal != "new" || q["tw-cathaybk:s-3"].Proposal != "new" {
		t.Fatalf("one source's two coffees: %s, %s", q["tw-cathaybk:s-2"].Proposal, q["tw-cathaybk:s-3"].Proposal)
	}
	// Ignoring the statement row frees the alert: it is matched again.
	if err := f.svc.IgnoreRow(f.ctx, f.acc, q["tw-cathaybk:s-1"].ID); err != nil {
		t.Fatal(err)
	}
	if p := f.queue(t)["xx-mail:alert-1"]; p.Proposal != "new" || p.MatchRowID != nil {
		t.Fatalf("after ignoring its pair: %s %v", p.Proposal, p.MatchRowID)
	}

	// Accepted first, the alert is an uncleared charge the statement then settles.
	g := f.keys["groceries"]
	if _, err := f.svc.AcceptRow(f.ctx, f.acc, f.queue(t)["xx-mail:alert-1"].ID, &g); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Import(f.ctx, f.acc, statementBatch(
		ImportRowInput{Kind: "transaction", Account: "card", ID: "s-4", Date: day("2026-09-05"), Amount: d("-350")},
	)); err != nil {
		t.Fatal(err)
	}
	// The posted figure settles the estimate (the card-settlement flow), once.
	if p := f.queue(t)["tw-cathaybk:s-4"]; p.Proposal != "clears" || p.MatchTransactionID == nil {
		t.Fatalf("the statement after the accepted alert: %s", p.Proposal)
	}
}

// A row is ignored only in its own book.
func TestIgnoreRowStaysInItsBook(t *testing.T) {
	f := setup(t)
	if _, err := f.svc.Import(f.ctx, f.acc, statementBatch(
		ImportRowInput{Kind: "transaction", Account: "card", ID: "s-1", Date: day("2026-09-05"), Amount: d("-350")})); err != nil {
		t.Fatal(err)
	}
	row := f.queue(t)["tw-cathaybk:s-1"].ID
	bob := newUser(t, f, "bob")
	book, err := f.svc.CreateBook(f.ctx, bob.ID, "Bob's", "TWD")
	if err != nil {
		t.Fatal(err)
	}
	bobs, err := f.svc.ResolveAccess(f.ctx, bob.ID, book.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.svc.IgnoreRow(f.ctx, bobs, row), "not_found")
	if len(f.queue(t)) != 1 {
		t.Fatal("another book's editor ignored this book's row")
	}
}

func orderBatch(id, date, total string, file []byte, items ...InvoiceItem) ImportInput {
	return ImportInput{Connector: "xx-mail", Accounts: []ImportAccount{{ExternalID: "orders"}},
		Rows:  []ImportRowInput{{Kind: "invoice", Account: "orders", ID: id, Date: day(date), Amount: d(total), Currency: "TWD", Counterparty: "Books Ltd", Items: items, File: "f"}},
		Files: []ImportFile{{Ref: "f", FileInput: FileInput{Filename: id + ".pdf", Bytes: file}}}}
}

func TestAnOrderEmailAndItsEInvoiceAreOnePurchase(t *testing.T) {
	f := setup(t)
	paid, err := f.svc.CreateTransaction(f.ctx, f.acc, TransactionInput{Date: day("2026-09-02"), Payee: "BOOKS LTD",
		Lines: []LineInput{{AccountID: f.keys["groceries"], Amount: d("598")}, {AccountID: f.keys["credit_card"], Amount: d("-598")}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Import(f.ctx, f.acc, orderBatch("B-1001", "2026-09-02", "-598", pdfFile, item("會計學 x2", "598"))); err != nil {
		t.Fatal(err)
	}
	email := f.queue(t)["xx-mail:B-1001"]
	if email.Proposal != "enrich" {
		t.Fatalf("the email = %s", email.Proposal)
	}
	if _, err := f.svc.Accept(f.ctx, f.acc, email.ID, AcceptInput{}); err != nil {
		t.Fatal(err)
	}
	// The 電子發票 for the same purchase: its file joins, its lines do not.
	in := invoiceBatch(invoice("AB-1", "2026-09-02", "-598", "Books Ltd", item("書籍", "598")))
	in.Rows[0].File = "e"
	in.Files = []ImportFile{{Ref: "e", FileInput: FileInput{Filename: "einvoice.pdf", Bytes: append([]byte("%PDF-1.7 einvoice "), pdfFile...)}}}
	if _, err := f.svc.Import(f.ctx, f.acc, in); err != nil {
		t.Fatal(err)
	}
	e := f.queue(t)["tw-einvoice:AB-1"]
	if e.Proposal != "same_invoice" || e.MatchTransactionID == nil || *e.MatchTransactionID != paid.ID {
		t.Fatalf("the e-invoice = %s %v, want same_invoice of %d", e.Proposal, e.MatchTransactionID, paid.ID)
	}
	if _, err := f.svc.Accept(f.ctx, f.acc, e.ID, AcceptInput{}); err != nil {
		t.Fatal(err)
	}
	v, _ := f.svc.GetTransaction(f.ctx, f.acc, paid.ID)
	if f.countTransactions(t) != 1 || len(v.Items) != 1 || v.Items[0].Description != "會計學 x2" || len(v.Attachments) != 2 {
		t.Fatalf("%d transactions, items %+v, %d files", f.countTransactions(t), v.Items, len(v.Attachments))
	}
}

func TestTwoInvoicesWaitingForOneCardRow(t *testing.T) {
	f := setup(t)
	if _, err := f.svc.Import(f.ctx, f.acc, statementBatch(
		ImportRowInput{Kind: "transaction", Account: "card", ID: "s-1", Date: day("2026-09-03"), Amount: d("-598"), Description: "BOOKS LTD"})); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Import(f.ctx, f.acc, orderBatch("B-1001", "2026-09-02", "-598", pdfFile, item("會計學 x2", "598"))); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Import(f.ctx, f.acc, invoiceBatch(invoice("AB-1", "2026-09-02", "-598", "Books Ltd", item("書籍", "598")))); err != nil {
		t.Fatal(err)
	}
	q := f.queue(t)
	email, einv, card := q["xx-mail:B-1001"], q["tw-einvoice:AB-1"], q["tw-cathaybk:s-1"]
	if email.Proposal != "enrich" || *email.MatchRowID != card.ID || einv.Proposal != "same_invoice" || *einv.MatchRowID != email.ID {
		t.Fatalf("email %s %v, e-invoice %s %v", email.Proposal, email.MatchRowID, einv.Proposal, einv.MatchRowID)
	}
	_, err := f.svc.Accept(f.ctx, f.acc, einv.ID, AcceptInput{})
	wantCode(t, err, "match_pending")

	f.mapSource(t, "card", f.keys["credit_card"])
	g := f.keys["groceries"]
	txn, err := f.svc.AcceptRow(f.ctx, f.acc, f.queue(t)["tw-cathaybk:s-1"].ID, &g)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Accept(f.ctx, f.acc, f.queue(t)["xx-mail:B-1001"].ID, AcceptInput{}); err != nil {
		t.Fatal(err)
	}
	einv = f.queue(t)["tw-einvoice:AB-1"]
	if einv.Proposal != "same_invoice" || einv.MatchTransactionID == nil || *einv.MatchTransactionID != txn {
		t.Fatalf("after the email: %s %v, want same_invoice of %d", einv.Proposal, einv.MatchTransactionID, txn)
	}
	if _, err := f.svc.Accept(f.ctx, f.acc, einv.ID, AcceptInput{}); err != nil {
		t.Fatal(err)
	}
	if f.countTransactions(t) != 1 || len(f.queue(t)) != 0 {
		t.Fatalf("%d transactions, %d waiting", f.countTransactions(t), len(f.queue(t)))
	}
}

func TestInvoiceWaitsAWeekBeforeCash(t *testing.T) {
	f := setup(t)
	defer func(old func() time.Time) { now = old }(now)
	now = func() time.Time { return day("2026-09-14") }
	if _, err := f.svc.Import(f.ctx, f.acc, invoiceBatch(
		invoice("W-1", "2026-09-10", "-85", "豪大雞排", item("雞排", "85")),
		invoice("W-2", "2026-09-01", "-60", "早餐店", item("蛋餅", "60")),
	)); err != nil {
		t.Fatal(err)
	}
	f.mapSource(t, "carrier", f.keys["cash"])
	q := f.queue(t)
	if q["tw-einvoice:W-1"].Proposal != "waiting" || q["tw-einvoice:W-2"].Proposal != "new" {
		t.Fatalf("four days old %s, thirteen days old %s", q["tw-einvoice:W-1"].Proposal, q["tw-einvoice:W-2"].Proposal)
	}
	_, err := f.svc.Accept(f.ctx, f.acc, q["tw-einvoice:W-1"].ID, AcceptInput{})
	if le := wantCode(t, err, "invalid_input"); le.Fields["account_id"] != "required" {
		t.Fatalf("waiting invoice accepted without a choice: %+v", le.Fields)
	}
	// A week on, the queue sees it a cash purchase without anything else happening.
	now = func() time.Time { return day("2026-09-18") }
	if p := f.queue(t)["tw-einvoice:W-1"]; p.Proposal != "new" {
		t.Fatalf("after a week: %s", p.Proposal)
	}
	// Chosen explicitly, it books as cash at once.
	now = func() time.Time { return day("2026-09-14") }
	if _, err := f.svc.Import(f.ctx, f.acc, invoiceBatch(invoice("W-3", "2026-09-13", "-40", "飲料店", item("紅茶", "40")))); err != nil {
		t.Fatal(err)
	}
	dining := f.keys["dining"]
	if _, err := f.svc.Accept(f.ctx, f.acc, f.queue(t)["tw-einvoice:W-3"].ID, AcceptInput{CategoryID: &dining}); err != nil {
		t.Fatal(err)
	}
}
