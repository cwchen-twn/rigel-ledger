package ledger

import "testing"

// Emails as evidence (#57): one with no amount joins the payment that names
// its seller; one with an invoice number joins that 電子發票.

func evidence(id, date, seller, ref string) ImportRowInput {
	return ImportRowInput{Kind: "invoice", Account: "orders", ID: id, Date: day(date), Amount: d("0"), Currency: "TWD",
		Counterparty: seller, Reference: ref, File: "f-" + id}
}

func withFiles(in ImportInput) ImportInput {
	for _, r := range in.Rows {
		if r.File != "" {
			in.Files = append(in.Files, ImportFile{Ref: r.File, FileInput: FileInput{Filename: r.ID + ".pdf", Bytes: pdfFile}})
		}
	}
	return in
}

func TestAnEmailWithoutAnAmountJoinsThePaymentNamingItsSeller(t *testing.T) {
	f := setup(t)
	if _, err := f.svc.Import(f.ctx, f.acc, withFiles(mailBatch(evidence("esim-1", "2026-08-08", "Trip.com", "")))); err != nil {
		t.Fatal(err)
	}
	row := f.queue(t)["xx-mail:esim-1"]
	if row.Proposal != "waiting" {
		t.Fatalf("nothing paid yet: %s", row.Proposal)
	}
	travel := f.keys["travel"]
	_, err := f.svc.Accept(f.ctx, f.acc, row.ID, AcceptInput{CategoryID: &travel})
	wantCode(t, err, "amount_unknown")

	// The card statement: another shop the same day, and the eSIM a day later.
	if _, err := f.svc.Import(f.ctx, f.acc, bankBatch(
		ImportRowInput{Kind: "transaction", Account: "card", ID: "s1", Date: day("2026-08-08"), Amount: d("-85"), Description: "全家便利商店"},
		ImportRowInput{Kind: "transaction", Account: "card", ID: "s2", Date: day("2026-08-09"), Amount: d("-322"), Description: "TRIP.COM*ESIM HONG KONG"},
	)); err != nil {
		t.Fatal(err)
	}
	q := f.queue(t)
	row, s2 := q["xx-mail:esim-1"], q["testbank:s2"]
	if row.Proposal != "enrich" || row.MatchRowID == nil || *row.MatchRowID != s2.ID {
		t.Fatalf("eSIM email = %s %v, want enrich of s2", row.Proposal, row.MatchRowID)
	}
	f.mapSource(t, "card", f.keys["credit_card"])
	txn, err := f.svc.AcceptRow(f.ctx, f.acc, s2.ID, &travel)
	if err != nil {
		t.Fatal(err)
	}
	// A 電子發票 for the same charge still claims it: evidence claims nothing.
	if _, err := f.svc.Import(f.ctx, f.acc, invoiceBatch(invoice("ZZ-1", "2026-08-09", "-322", "Trip.com", item("eSIM", "322")))); err != nil {
		t.Fatal(err)
	}
	q = f.queue(t)
	if inv := q["tw-einvoice:ZZ-1"]; inv.Proposal != "enrich" || *inv.MatchTransactionID != txn {
		t.Fatalf("the 電子發票 = %s %v, want enrich %d", inv.Proposal, inv.MatchTransactionID, txn)
	}
	row = q["xx-mail:esim-1"]
	if row.Proposal != "enrich" || row.MatchTransactionID == nil || *row.MatchTransactionID != txn {
		t.Fatalf("after the card row: %s %v", row.Proposal, row.MatchTransactionID)
	}
	if _, err := f.svc.Accept(f.ctx, f.acc, row.ID, AcceptInput{}); err != nil {
		t.Fatal(err)
	}
	v, _ := f.svc.GetTransaction(f.ctx, f.acc, txn)
	if len(v.Postings) != 2 || len(v.Items) != 0 || len(v.Attachments) != 1 || v.Postings[0].Amount.Abs().String() != "322" {
		t.Fatalf("the payment after its email: %d postings, %d items, %d files", len(v.Postings), len(v.Items), len(v.Attachments))
	}
}

func TestAnEmailNamingAnEInvoiceJoinsIt(t *testing.T) {
	f := setup(t)
	// The notice comes first, the 電子發票 with its amount later.
	if _, err := f.svc.Import(f.ctx, f.acc, withFiles(mailBatch(evidence("notice-1", "2026-06-12", "Spotify AB", "QQ10000001")))); err != nil {
		t.Fatal(err)
	}
	if p := f.queue(t)["xx-mail:notice-1"]; p.Proposal != "waiting" {
		t.Fatalf("notice alone = %s", p.Proposal)
	}
	if _, err := f.svc.Import(f.ctx, f.acc, invoiceBatch(invoice("QQ10000001:2026-06-12", "2026-06-12", "-149", "Spotify AB", item("Premium", "149")))); err != nil {
		t.Fatal(err)
	}
	q := f.queue(t)
	notice, inv := q["xx-mail:notice-1"], q["tw-einvoice:QQ10000001:2026-06-12"]
	if notice.Proposal != "same_invoice" || notice.MatchRowID == nil || *notice.MatchRowID != inv.ID {
		t.Fatalf("notice = %s %v, want same_invoice of the 電子發票 %d", notice.Proposal, notice.MatchRowID, inv.ID)
	}
	if inv.Proposal == "same_invoice" {
		t.Fatal("the 電子發票 waits on its notice: they wait on each other")
	}
	f.mapSource(t, "carrier", f.keys["cash"])
	music := f.keys["education"]
	txn, err := f.svc.Accept(f.ctx, f.acc, inv.ID, AcceptInput{CategoryID: &music})
	if err != nil {
		t.Fatal(err)
	}
	notice = f.queue(t)["xx-mail:notice-1"]
	if notice.Proposal != "same_invoice" || notice.MatchTransactionID == nil || *notice.MatchTransactionID != txn {
		t.Fatalf("after the 電子發票: %s %v, want same_invoice %d", notice.Proposal, notice.MatchTransactionID, txn)
	}
	if _, err := f.svc.Accept(f.ctx, f.acc, notice.ID, AcceptInput{}); err != nil {
		t.Fatal(err)
	}
	v, _ := f.svc.GetTransaction(f.ctx, f.acc, txn)
	if len(v.Items) != 1 || len(v.Attachments) != 1 {
		t.Fatalf("items %d, files %d", len(v.Items), len(v.Attachments))
	}

	// An Apple email with an amount names a 電子發票 staged before it.
	if _, err := f.svc.Import(f.ctx, f.acc, invoiceBatch(invoice("QQ20000002:2026-07-17", "2026-07-17", "-190", "Apple", item("In-app", "190")))); err != nil {
		t.Fatal(err)
	}
	apple := evidence("apple-1", "2026-07-17", "Apple", "QQ20000002")
	apple.Amount, apple.Items = d("-190"), []InvoiceItem{item("Game pass", "190")}
	if _, err := f.svc.Import(f.ctx, f.acc, withFiles(mailBatch(apple))); err != nil {
		t.Fatal(err)
	}
	q = f.queue(t)
	if a := q["xx-mail:apple-1"]; a.Proposal != "same_invoice" || *a.MatchRowID != q["tw-einvoice:QQ20000002:2026-07-17"].ID {
		t.Fatalf("apple = %s %v", a.Proposal, a.MatchRowID)
	}
	if e := q["tw-einvoice:QQ20000002:2026-07-17"]; e.Proposal == "same_invoice" {
		t.Fatal("the 電子發票 waits on the later Apple email")
	}
}

func TestEvidenceRowsAreChecked(t *testing.T) {
	f := setup(t)
	bad := evidence("x", "2026-08-08", "Shop", "")
	bad.Items = []InvoiceItem{item("x", "1")}
	wantField := func(err error, field, code string) {
		t.Helper()
		if e := wantCode(t, err, "invalid_input"); e.Fields[field] != code {
			t.Fatalf("fields = %v, want %s: %s", e.Fields, field, code)
		}
	}
	_, err := f.svc.Import(f.ctx, f.acc, withFiles(mailBatch(bad)))
	wantField(err, "rows[0].amount", "zero")
	bad = evidence("x", "2026-08-08", "Shop", "AB1")
	_, err = f.svc.Import(f.ctx, f.acc, withFiles(mailBatch(bad)))
	wantField(err, "rows[0].reference", "invalid")
	_, err = f.svc.Import(f.ctx, f.acc, bankBatch(ImportRowInput{Kind: "transaction", Account: "card", ID: "t", Date: day("2026-08-08"), Amount: d("-1"), Reference: "AB12345678"}))
	wantField(err, "rows[0].reference", "invalid")
}
