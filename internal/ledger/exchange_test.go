package ledger

import "testing"

// An exchange between two of the book's accounts in two currencies (a bank
// selling USD for PYG): one movement number on both statements.
func TestAnExchangeBetweenCurrenciesIsOneTransfer(t *testing.T) {
	f := setup(t)
	usd, pyg := f.holding(t, "Continental USD", "USD"), f.holding(t, "Continental PYG", "PYG")
	f.rate(t, "USD", "TWD", "2026-09-01", "32")
	batch := ImportInput{Connector: "statement", Accounts: []ImportAccount{
		{ExternalID: "usd", Label: "USD", Currency: "USD"}, {ExternalID: "pyg", Label: "PYG", Currency: "PYG"}}, Rows: []ImportRowInput{
		{Kind: "transaction", Account: "usd", ID: "u1", Date: day("2026-09-04"), Amount: d("-1708"), Currency: "USD", Description: "DEBITO X OPERAC.CAMBIOS", Reference: "2145246"},
		{Kind: "transaction", Account: "usd", ID: "u2", Date: day("2026-09-04"), Amount: d("-100"), Currency: "USD", Description: "DEBITO X OPERAC.CAMBIOS", Reference: "2145247"},
		{Kind: "transaction", Account: "pyg", ID: "p1", Date: day("2026-09-04"), Amount: d("10008880"), Currency: "PYG", Description: "CREDITO X OPERAC.CAMBIOS", Reference: "2145246"},
		{Kind: "transaction", Account: "pyg", ID: "p2", Date: day("2026-09-04"), Amount: d("586000"), Currency: "PYG", Description: "CREDITO X OPERAC.CAMBIOS", Reference: "2145247"},
	}}
	if _, err := f.svc.Import(f.ctx, f.acc, batch); err != nil {
		t.Fatal(err)
	}
	f.mapSource(t, "usd", usd)
	f.mapSource(t, "pyg", pyg)
	q := f.queue(t)
	u1, p1, u2 := q["statement:u1"], q["statement:p1"], q["statement:u2"]
	if u1.Proposal != "transfer" || u1.MatchRowID == nil || *u1.MatchRowID != p1.ID || *u2.MatchRowID != q["statement:p2"].ID {
		t.Fatalf("u1 = %s %v, want transfer with p1 %d (and u2 with p2)", u1.Proposal, u1.MatchRowID, p1.ID)
	}
	txn, err := f.svc.AcceptRow(f.ctx, f.acc, u1.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := f.svc.GetTransaction(f.ctx, f.acc, txn)
	got := map[string]string{}
	for _, p := range v.Postings {
		got[p.Commodity] = p.Amount.String() + " / " + p.BaseAmount.String()
	}
	// 1,708 USD at 32 is NT$54,656; the PYG side is worth what was sold.
	if len(v.Postings) != 2 || got["USD"] != "-1708 / -54656" || got["PYG"] != "10008880 / 54656" {
		t.Fatalf("exchange = %v", got)
	}
	if p := f.queue(t)["statement:p1"]; p.ID != 0 {
		t.Fatal("the PYG side is still waiting")
	}

	// A reference on a row that is neither a transaction nor an invoice is refused.
	bad := ImportInput{Connector: "statement", Accounts: batch.Accounts, Rows: []ImportRowInput{
		{Kind: "balance", Account: "usd", ID: "b", Date: day("2026-09-30"), Amount: d("2500"), Currency: "USD", Reference: "x"}}}
	_, err = f.svc.Import(f.ctx, f.acc, bad)
	if e := wantCode(t, err, "invalid_input"); e.Fields["rows[0].reference"] != "invalid" {
		t.Fatalf("fields = %v", e.Fields)
	}
}
