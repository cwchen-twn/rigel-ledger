package ledger

import (
	"testing"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

func invoice(id, date, total, seller string, items ...InvoiceItem) ImportRowInput {
	return ImportRowInput{Kind: "invoice", Account: "carrier", ID: id, Date: day(date), Amount: d(total), Counterparty: seller, Items: items}
}

func item(desc, amount string) InvoiceItem { return InvoiceItem{Description: desc, Amount: d(amount)} }

func invoiceBatch(rows ...ImportRowInput) ImportInput {
	return ImportInput{Connector: "tw-einvoice", Accounts: []ImportAccount{{ExternalID: "carrier", Label: "手機條碼", Currency: "TWD"}}, Rows: rows}
}

func TestInvoiceEnrichesWhatPaidForIt(t *testing.T) {
	f := setup(t)
	card := f.keys["credit_card"]
	paid, err := f.svc.CreateTransaction(f.ctx, f.acc, TransactionInput{Date: day("2026-09-03"), Payee: "全聯 PX Mart",
		Lines: []LineInput{{AccountID: f.keys["groceries"], Amount: d("320")}, {AccountID: card, Amount: d("-320")}}})
	if err != nil {
		t.Fatal(err)
	}
	// Toilet paper is housekeeping, not food.
	if _, err := f.svc.CreateRule(f.ctx, f.acc, "衛生紙", f.keys["home_maintenance"], nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Import(f.ctx, f.acc, invoiceBatch(
		invoice("AB-12345678", "2026-09-02", "-320", "全聯福利中心", item("鮮乳 936ml", "90"), item("舒潔衛生紙 12入", "230")),
	)); err != nil {
		t.Fatal(err)
	}
	q := f.queue(t)
	row := q["tw-einvoice:AB-12345678"]
	if row.Proposal != "enrich" || row.MatchTransactionID == nil || *row.MatchTransactionID != paid.ID {
		t.Fatalf("invoice proposal = %s %v, want enrich %d (unmapped carrier or not)", row.Proposal, row.MatchTransactionID, paid.ID)
	}
	items, err := parseItems(row.Items)
	if err != nil || items[0].AccountID != nil || items[1].AccountID == nil || *items[1].AccountID != f.keys["home_maintenance"] {
		t.Fatalf("item categories = %+v %v", items, err)
	}

	txn, err := f.svc.Accept(f.ctx, f.acc, row.ID, AcceptInput{Split: true})
	if err != nil || txn != paid.ID {
		t.Fatalf("accept = %d %v", txn, err)
	}
	v, _ := f.svc.GetTransaction(f.ctx, f.acc, paid.ID)
	got := map[int64]string{}
	for _, p := range v.Postings {
		got[p.AccountID] = p.Amount.String()
	}
	if len(v.Postings) != 3 || got[card] != "-320" || got[f.keys["groceries"]] != "90" || got[f.keys["home_maintenance"]] != "230" {
		t.Fatalf("split = %v", got)
	}
	if len(v.Items) != 2 || v.Items[1].Description != "舒潔衛生紙 12入" || v.Items[1].AccountID == nil || v.Items[0].AccountID != nil {
		t.Fatalf("items = %+v", v.Items)
	}

	// A resend stages nothing; the transaction is taken: a second invoice of
	// the same total, same days, does not claim it.
	if res, err := f.svc.Import(f.ctx, f.acc, invoiceBatch(invoice("AB-12345678", "2026-09-02", "-320", "全聯福利中心", item("x", "320")))); err != nil || res.Duplicates != 1 {
		t.Fatalf("resend = %+v %v", res, err)
	}
	if _, err := f.svc.Import(f.ctx, f.acc, invoiceBatch(invoice("AB-99999999", "2026-09-03", "-320", "全聯福利中心", item("x", "320")))); err != nil {
		t.Fatal(err)
	}
	if p := f.queue(t)["tw-einvoice:AB-99999999"]; p.Proposal != "new" {
		t.Fatalf("a second invoice claimed a transaction that has one: %s", p.Proposal)
	}
}

func TestInvoiceWaitsForItsCardRow(t *testing.T) {
	f := setup(t)
	in := invoiceBatch(invoice("CD-1", "2026-09-10", "-150", "星巴克", item("拿鐵", "150")))
	if _, err := f.svc.Import(f.ctx, f.acc, in); err != nil {
		t.Fatal(err)
	}
	if p := f.queue(t)["tw-einvoice:CD-1"]; p.Proposal != "new" {
		t.Fatalf("nothing paid for it yet: %s", p.Proposal)
	}
	// The card statement arrives later: the invoice now waits for its row.
	if _, err := f.svc.Import(f.ctx, f.acc, bankBatch(
		ImportRowInput{Kind: "transaction", Account: "card", ID: "s1", Date: day("2026-09-11"), Amount: d("-150"), Description: "STARBUCKS 星巴克"},
	)); err != nil {
		t.Fatal(err)
	}
	q := f.queue(t)
	inv, cardRow := q["tw-einvoice:CD-1"], q["testbank:s1"]
	if inv.Proposal != "enrich" || inv.MatchRowID == nil || *inv.MatchRowID != cardRow.ID {
		t.Fatalf("invoice = %s %v, want enrich of the card row", inv.Proposal, inv.MatchRowID)
	}
	_, err := f.svc.Accept(f.ctx, f.acc, inv.ID, AcceptInput{})
	wantCode(t, err, "match_pending")

	f.mapSource(t, "card", f.keys["credit_card"])
	dining := f.keys["dining"]
	txn, err := f.svc.AcceptRow(f.ctx, f.acc, f.queue(t)["testbank:s1"].ID, &dining)
	if err != nil {
		t.Fatal(err)
	}
	inv = f.queue(t)["tw-einvoice:CD-1"]
	if inv.Proposal != "enrich" || inv.MatchTransactionID == nil || *inv.MatchTransactionID != txn {
		t.Fatalf("after the card row: %s %v, want enrich %d", inv.Proposal, inv.MatchTransactionID, txn)
	}
	if _, err := f.svc.Accept(f.ctx, f.acc, inv.ID, AcceptInput{Split: true}); err != nil {
		t.Fatal(err)
	}
	v, _ := f.svc.GetTransaction(f.ctx, f.acc, txn)
	// One category for everything: nothing to split, the items are kept.
	if len(v.Postings) != 2 || len(v.Items) != 1 || v.Items[0].AccountID != nil {
		t.Fatalf("after the invoice: %d postings, items %+v", len(v.Postings), v.Items)
	}
}

func TestInvoicePaidInCash(t *testing.T) {
	f := setup(t)
	if _, err := f.svc.CreateRule(f.ctx, f.acc, "紅茶", f.keys["dining"], nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Import(f.ctx, f.acc, invoiceBatch(invoice("EF-1", "2026-09-12", "-85", "豪大雞排", item("雞排", "75"), item("紅茶", "10")))); err != nil {
		t.Fatal(err)
	}
	row := f.queue(t)["tw-einvoice:EF-1"]
	if row.Proposal != "new" {
		t.Fatalf("proposal = %s", row.Proposal)
	}
	_, err := f.svc.Accept(f.ctx, f.acc, row.ID, AcceptInput{})
	wantCode(t, err, "unmapped")
	f.mapSource(t, "carrier", f.keys["cash"])
	// No rule for the seller, so a category is needed; the 紅茶 rule
	// still splits off its item when asked.
	_, err = f.svc.Accept(f.ctx, f.acc, row.ID, AcceptInput{})
	if le := wantCode(t, err, "invalid_input"); le.Fields["account_id"] != "required" {
		t.Fatalf("fields = %v", le.Fields)
	}
	groceries := f.keys["groceries"]
	txn, err := f.svc.Accept(f.ctx, f.acc, row.ID, AcceptInput{CategoryID: &groceries, Split: true})
	if err != nil {
		t.Fatal(err)
	}
	v, _ := f.svc.GetTransaction(f.ctx, f.acc, txn)
	got := map[int64]string{}
	for _, p := range v.Postings {
		got[p.AccountID] = p.Amount.String()
		if p.AccountID == f.keys["cash"] && p.Status != db.PostingStatusCleared {
			t.Fatal("the cash line should be cleared")
		}
	}
	if v.Payee != "豪大雞排" || got[f.keys["cash"]] != "-85" || got[f.keys["dining"]] != "10" || got[groceries] != "75" || len(v.Items) != 2 {
		t.Fatalf("cash purchase = %s %v, %d items", v.Payee, got, len(v.Items))
	}
}

func TestInvoiceRowsAreChecked(t *testing.T) {
	f := setup(t)
	for _, c := range []struct {
		row   ImportRowInput
		field string
	}{
		{invoice("1", "2026-09-01", "-10", "x"), "rows[0].items"},
		{invoice("1", "2026-09-01", "0", "x", item("a", "1")), "rows[0].amount"},
		{invoice("1", "2026-09-01", "-10", "x", item("a", "1.001")), "rows[0].items"},
		{ImportRowInput{Kind: "transaction", Account: "carrier", ID: "1", Date: day("2026-09-01"), Amount: d("-1"), Items: []InvoiceItem{item("a", "1")}}, "rows[0].items"},
	} {
		_, err := f.svc.Import(f.ctx, f.acc, invoiceBatch(c.row))
		if le := wantCode(t, err, "invalid_input"); le.Fields[c.field] == "" {
			t.Errorf("%+v: fields = %v, want %s", c.row, le.Fields, c.field)
		}
	}
}
