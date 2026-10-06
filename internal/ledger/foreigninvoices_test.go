package ledger

import (
	"testing"
)

// A EUR invoice by email, charged in TWD on the card (#56).
func mailBatch(rows ...ImportRowInput) ImportInput {
	return ImportInput{Connector: "xx-mail", Accounts: []ImportAccount{{ExternalID: "orders", Label: "Orders by email"}}, Rows: rows}
}

func eurInvoice(id, date, total, seller string, items ...InvoiceItem) ImportRowInput {
	r := invoice(id, date, total, seller, items...)
	r.Account, r.Currency = "orders", "EUR"
	return r
}

func TestForeignInvoiceEnrichesTheCardCharge(t *testing.T) {
	f := setup(t)
	f.rate(t, "EUR", "TWD", "2026-09-01", "35")
	card := f.keys["credit_card"]
	pay := func(payee, amount string) int64 {
		t.Helper()
		v, err := f.svc.CreateTransaction(f.ctx, f.acc, TransactionInput{Date: day("2026-09-04"), Payee: payee,
			Lines: []LineInput{{AccountID: f.keys["home_maintenance"], Amount: d(amount).Neg()}, {AccountID: card, Amount: d(amount)}}})
		if err != nil {
			t.Fatal(err)
		}
		return v.ID
	}
	// The same amount at another seller is not it; the seller at 2.5% off
	// the day's rate (the card's rate and its fee) is.
	pay("AMAZON WEB SERVICES", "-1277.5")
	paid := pay("HETZNER ONLINE GMBH GUNZENHAUSEN", "-1310")
	if _, err := f.svc.Import(f.ctx, f.acc, mailBatch(
		eurInvoice("order:hetzner:R001", "2026-09-03", "-36.50", "Hetzner Online GmbH", item("CX22 server", "30.67"), item("VAT 19%", "5.83")),
		eurInvoice("order:hetzner:R002", "2026-09-03", "-99.00", "Hetzner Online GmbH", item("Storage box", "99.00")),
	)); err != nil {
		t.Fatal(err)
	}
	q := f.queue(t)
	row := q["xx-mail:order:hetzner:R001"]
	if row.Proposal != "enrich" || row.MatchTransactionID == nil || *row.MatchTransactionID != paid {
		t.Fatalf("EUR invoice = %s %v, want enrich %d", row.Proposal, row.MatchTransactionID, paid)
	}
	if !row.MatchPostingAmount.Valid || row.MatchPostingAmount.Decimal.String() != "-1310" || *row.MatchPostingCurrency != "TWD" {
		t.Fatalf("queue shows the charge as %v %v", row.MatchPostingAmount, row.MatchPostingCurrency)
	}
	// 99 EUR is ~3465 TWD: nothing near it was charged.
	if p := q["xx-mail:order:hetzner:R002"]; p.Proposal == "enrich" {
		t.Fatalf("a 99 EUR invoice matched %v", p.MatchTransactionID)
	}

	if _, err := f.svc.Accept(f.ctx, f.acc, row.ID, AcceptInput{Split: true}); err != nil {
		t.Fatal(err)
	}
	v, _ := f.svc.GetTransaction(f.ctx, f.acc, paid)
	// The booking stays in TWD; the items keep their EUR.
	if len(v.Postings) != 2 || len(v.Items) != 2 || v.Items[0].Currency == nil || *v.Items[0].Currency != "EUR" || v.Items[1].Amount.String() != "5.83" {
		t.Fatalf("after the invoice: %d postings, items %+v", len(v.Postings), v.Items)
	}
}

func TestForeignInvoiceWaitsForTheCardRow(t *testing.T) {
	f := setup(t)
	f.rate(t, "USD", "TWD", "2026-09-01", "32")
	if _, err := f.svc.Import(f.ctx, f.acc, bankBatch(
		ImportRowInput{Kind: "transaction", Account: "card", ID: "s1", Date: day("2026-09-06"), Amount: d("-660"), Description: "GITHUB, INC. SAN FRANCISCO"},
	)); err != nil {
		t.Fatal(err)
	}
	inv := eurInvoice("order:github:G1", "2026-09-05", "-20", "GitHub, Inc.", item("Copilot", "20"))
	inv.Currency = "USD"
	if _, err := f.svc.Import(f.ctx, f.acc, mailBatch(inv)); err != nil {
		t.Fatal(err)
	}
	q := f.queue(t)
	row, cardRow := q["xx-mail:order:github:G1"], q["testbank:s1"]
	if row.Proposal != "enrich" || row.MatchRowID == nil || *row.MatchRowID != cardRow.ID {
		t.Fatalf("USD invoice = %s %v, want enrich of the card row %d", row.Proposal, row.MatchRowID, cardRow.ID)
	}
	if !row.MatchRowAmount.Valid || row.MatchRowAmount.Decimal.String() != "-660" {
		t.Fatalf("queue shows the row as %v", row.MatchRowAmount)
	}
	_, err := f.svc.Accept(f.ctx, f.acc, row.ID, AcceptInput{})
	wantCode(t, err, "match_pending")

	f.mapSource(t, "card", f.keys["credit_card"])
	cat := f.keys["education"]
	txn, err := f.svc.AcceptRow(f.ctx, f.acc, cardRow.ID, &cat)
	if err != nil {
		t.Fatal(err)
	}
	row = f.queue(t)["xx-mail:order:github:G1"]
	if row.Proposal != "enrich" || row.MatchTransactionID == nil || *row.MatchTransactionID != txn {
		t.Fatalf("after the card row: %s %v, want enrich %d", row.Proposal, row.MatchTransactionID, txn)
	}
}

func TestForeignInvoiceNeedsARateAndANameToMatch(t *testing.T) {
	f := setup(t)
	if _, err := f.svc.Import(f.ctx, f.acc, bankBatch(
		ImportRowInput{Kind: "transaction", Account: "card", ID: "s1", Date: day("2026-09-06"), Amount: d("-1300"), Description: "HETZNER ONLINE"},
		ImportRowInput{Kind: "transaction", Account: "card", ID: "s2", Date: day("2026-09-06"), Amount: d("-1400"), Description: "PAYPAL *XYZ"},
	)); err != nil {
		t.Fatal(err)
	}
	// No EUR rate yet: no way to tell.
	if _, err := f.svc.Import(f.ctx, f.acc, mailBatch(eurInvoice("a", "2026-09-05", "-37", "Hetzner Online GmbH", item("x", "37")))); err != nil {
		t.Fatal(err)
	}
	if p := f.queue(t)["xx-mail:a"]; p.Proposal == "enrich" {
		t.Fatalf("matched without a rate: %v", p.MatchRowID)
	}
	f.rate(t, "EUR", "TWD", "2026-09-01", "35.5")
	// A seller named only by noise words matches nothing, even at the right amount.
	if _, err := f.svc.Import(f.ctx, f.acc, mailBatch(
		eurInvoice("b", "2026-09-05", "-37", "Online Services GmbH", item("x", "37")),
		eurInvoice("c", "2026-09-05", "-37", "Hetzner Online GmbH", item("x", "37")),
	)); err != nil {
		t.Fatal(err)
	}
	q := f.queue(t)
	if p := q["xx-mail:b"]; p.Proposal == "enrich" {
		t.Fatalf("matched on noise words: %v", p.MatchRowID)
	}
	// 37 EUR at 35.5 is 1313.50: the Hetzner charge, 1% off.
	if c := q["xx-mail:c"]; c.Proposal != "enrich" || c.MatchRowID == nil || *c.MatchRowID != q["testbank:s1"].ID {
		t.Fatalf("c = %s %v, want enrich of s1", c.Proposal, c.MatchRowID)
	}
}

func TestSellerTokens(t *testing.T) {
	for in, want := range map[string]string{
		"Hetzner Online GmbH": "hetzner", "GitHub, Inc.": "github", "Online Services GmbH": "", "Uber B.V.": "uber",
	} {
		got := ""
		for i, w := range sellerTokens(in) {
			if i > 0 {
				got += " "
			}
			got += w
		}
		if got != want {
			t.Errorf("sellerTokens(%q) = %q, want %q", in, got, want)
		}
	}
}
