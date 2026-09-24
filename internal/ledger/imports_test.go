package ledger

import (
	"testing"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

func (f *fixture) queue(t *testing.T) map[string]db.ListQueueRow {
	t.Helper()
	rows, err := f.svc.Queue(f.ctx, f.acc)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]db.ListQueueRow{}
	for _, r := range rows {
		out[r.ExternalID] = r
	}
	return out
}

func (f *fixture) mapSource(t *testing.T, ext string, account int64) {
	t.Helper()
	srcs, err := f.svc.SourceAccounts(f.ctx, f.acc)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range srcs {
		if s.ExternalID == ext {
			if _, err := f.svc.MapSourceAccount(f.ctx, f.acc, s.ID, &account); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("no source account %q", ext)
}

func bankBatch(rows ...ImportRowInput) ImportInput {
	return ImportInput{Connector: "testbank", Label: "September",
		Accounts: []ImportAccount{{ExternalID: "chk", Label: "Checking", Currency: "TWD"},
			{ExternalID: "sav", Label: "Savings", Currency: "TWD"}, {ExternalID: "card", Label: "Visa", Currency: "TWD"}},
		Rows: rows}
}

func TestImportStagesOnceAndWaitsForMapping(t *testing.T) {
	f := setup(t)
	in := bankBatch(
		ImportRowInput{Kind: "transaction", Account: "chk", ID: "1", Date: day("2026-09-01"), Amount: d("-320"), Description: "PX MART 0912"},
		ImportRowInput{Kind: "transaction", Account: "chk", ID: "2", Date: day("2026-09-02"), Amount: d("-150"), Description: "Starbucks"},
		ImportRowInput{Kind: "balance", Account: "chk", ID: "bal-0902", Date: day("2026-09-02"), Amount: d("9530")},
	)
	res, err := f.svc.Import(f.ctx, f.acc, in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Staged != 3 || res.Duplicates != 0 || res.Balances != 0 {
		t.Fatalf("first = %+v", res)
	}
	// The runner resends the whole statement: nothing is staged twice.
	res, err = f.svc.Import(f.ctx, f.acc, in)
	if err != nil || res.Staged != 0 || res.Duplicates != 3 {
		t.Fatalf("resend = %+v %v", res, err)
	}
	q := f.queue(t)
	if len(q) != 3 || q["testbank:1"].AccountID != nil {
		t.Fatalf("queue = %+v", q)
	}
	_, err = f.svc.AcceptRow(f.ctx, f.acc, q["testbank:1"].ID, nil)
	wantCode(t, err, "unmapped")

	// A rule, then the mapping: rows get proposals, the balance row becomes
	// an assertion (and the books, being empty, drift from it).
	if _, err := f.svc.CreateRule(f.ctx, f.acc, "px mart", f.keys["groceries"], nil); err != nil {
		t.Fatal(err)
	}
	f.mapSource(t, "chk", f.keys["bank_checking"])
	q = f.queue(t)
	if len(q) != 2 {
		t.Fatalf("after mapping %d rows wait, want 2 (the balance was applied)", len(q))
	}
	px, sb := q["testbank:1"], q["testbank:2"]
	if px.Proposal != "new" || px.ProposedAccountID == nil || *px.ProposedAccountID != f.keys["groceries"] {
		t.Fatalf("PX row = %+v", px)
	}
	if sb.ProposedAccountID != nil {
		t.Fatalf("Starbucks row got a category: %+v", sb)
	}
	drift, _ := f.svc.Drifts(f.ctx, f.acc)
	if len(drift) != 1 || !drift[0].Asserted.Equal(d("9530")) || !drift[0].Booked.IsZero() {
		t.Fatalf("drift = %+v", drift)
	}

	_, err = f.svc.AcceptRow(f.ctx, f.acc, sb.ID, nil)
	if e := wantCode(t, err, "invalid_input"); e.Fields["account_id"] != "required" {
		t.Fatalf("fields = %v", e.Fields)
	}
	txID, err := f.svc.AcceptRow(f.ctx, f.acc, px.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	v, err := f.svc.GetTransaction(f.ctx, f.acc, txID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Source != "sync" || v.ExternalID == nil || *v.ExternalID != "testbank:1" || v.Payee != "PX MART 0912" {
		t.Fatalf("transaction = %+v", v.Transaction)
	}
	if v.Postings[0].AccountID != f.keys["bank_checking"] || v.Postings[0].Status != db.PostingStatusCleared ||
		v.Postings[1].AccountID != f.keys["groceries"] || !v.Postings[1].Amount.Equal(d("320")) {
		t.Fatalf("postings = %+v", v.Postings)
	}
	if _, err := f.svc.AcceptRow(f.ctx, f.acc, px.ID, nil); err == nil {
		t.Fatal("a row was accepted twice")
	}
	if _, err := f.svc.AcceptRow(f.ctx, f.acc, sb.ID, ptr(f.keys["dining"])); err != nil {
		t.Fatal(err)
	}
	// With the opening balance the books say 10,000 - 320 - 150 = 9,530, as the bank does.
	f.post(t, "2026-08-31",
		LineInput{AccountID: f.keys["bank_checking"], Amount: d("10000")},
		LineInput{AccountID: f.keys[KeyOpeningBalances], Amount: d("-10000")})
	if drift, _ := f.svc.Drifts(f.ctx, f.acc); len(drift) != 0 {
		t.Fatalf("books agree with the bank, drift = %+v", drift)
	}
}

func TestImportFindsTypedInTransactions(t *testing.T) {
	f := setup(t)
	chk, sav, card := f.keys["bank_checking"], f.keys["bank_savings"], f.keys["credit_card"]
	typed := f.post(t, "2026-09-03",
		LineInput{AccountID: f.keys["dining"], Amount: d("150")},
		LineInput{AccountID: chk, Amount: d("-150")})
	// A card charge entered as an estimate, and a transfer typed in by hand.
	est := f.post(t, "2026-09-04",
		LineInput{AccountID: f.keys["groceries"], Amount: d("1000")},
		LineInput{AccountID: card, Amount: d("-1000")})
	xfer := f.post(t, "2026-09-05",
		LineInput{AccountID: sav, Amount: d("5000")},
		LineInput{AccountID: chk, Amount: d("-5000")})
	f.mapSourceAll(t, map[string]int64{"chk": chk, "sav": sav, "card": card})

	_, err := f.svc.Import(f.ctx, f.acc, bankBatch(
		ImportRowInput{Kind: "transaction", Account: "chk", ID: "a", Date: day("2026-09-04"), Amount: d("-150"), Description: "Starbucks"},
		ImportRowInput{Kind: "transaction", Account: "card", ID: "b", Date: day("2026-09-06"), Amount: d("-1040"), Description: "Costco"},
		ImportRowInput{Kind: "transaction", Account: "chk", ID: "c", Date: day("2026-09-05"), Amount: d("-5000"), Description: "To savings"},
		ImportRowInput{Kind: "transaction", Account: "sav", ID: "d", Date: day("2026-09-05"), Amount: d("5000"), Description: "From checking"},
		// Not typed in: a transfer only the bank knows about.
		ImportRowInput{Kind: "transaction", Account: "chk", ID: "e", Date: day("2026-09-10"), Amount: d("-2000"), Description: "To savings"},
		ImportRowInput{Kind: "transaction", Account: "sav", ID: "f", Date: day("2026-09-11"), Amount: d("2000"), Description: "From checking"},
	))
	if err != nil {
		t.Fatal(err)
	}
	q := f.queue(t)
	check := func(ext, proposal string, tx int64) db.ListQueueRow {
		t.Helper()
		r := q["testbank:"+ext]
		if r.Proposal != proposal || (tx != 0 && (r.MatchTransactionID == nil || *r.MatchTransactionID != tx)) {
			t.Fatalf("row %s = %s %v, want %s %d", ext, r.Proposal, r.MatchTransactionID, proposal, tx)
		}
		return r
	}
	a := check("a", "duplicate", typed.ID)
	b := check("b", "clears", est.ID)
	c := check("c", "duplicate", xfer.ID)
	check("d", "duplicate", xfer.ID) // claimed once from each side
	e := check("e", "transfer", 0)
	fr := check("f", "transfer", 0)
	if e.MatchRowID == nil || *e.MatchRowID != fr.ID || fr.MatchRowID == nil || *fr.MatchRowID != e.ID {
		t.Fatalf("transfer pair = %v / %v", e.MatchRowID, fr.MatchRowID)
	}

	for _, r := range []db.ListQueueRow{a, b, c, e} {
		if _, err := f.svc.AcceptRow(f.ctx, f.acc, r.ID, nil); err != nil {
			t.Fatalf("accept %s: %v", r.ExternalID, err)
		}
	}
	// The duplicate cleared the typed-in posting; the estimate took Costco's
	// figure on both legs; the bank-only transfer is one transaction, and
	// accepting one side took the other off the queue.
	v, _ := f.svc.GetTransaction(f.ctx, f.acc, typed.ID)
	if v.Postings[1].Status != db.PostingStatusCleared || v.Postings[0].Status != db.PostingStatusUncleared {
		t.Fatalf("typed-in = %+v", v.Postings)
	}
	v, _ = f.svc.GetTransaction(f.ctx, f.acc, est.ID)
	if !v.Postings[0].Amount.Equal(d("1040")) || !v.Postings[1].Amount.Equal(d("-1040")) || v.Postings[1].Status != db.PostingStatusCleared {
		t.Fatalf("estimate = %+v", v.Postings)
	}
	q = f.queue(t)
	if len(q) != 1 || q["testbank:d"].ID == 0 {
		t.Fatalf("left in the queue: %v", q)
	}
	b2, _ := f.svc.Balances(f.ctx, f.acc, day("2026-09-30"))
	if got := balanceOf(b2, sav).BaseAmount; !got.Equal(d("7000")) {
		t.Fatalf("savings = %s, want 7000", got)
	}
	if err := f.svc.IgnoreRow(f.ctx, f.acc, q["testbank:d"].ID); err != nil {
		t.Fatal(err)
	}
	if len(f.queue(t)) != 0 {
		t.Fatal("ignored row still waits")
	}
}

func TestImportClearsAForeignEstimateAtThePostedFigure(t *testing.T) {
	f := setup(t)
	card := f.keys["credit_card"]
	f.rate(t, "USD", "TWD", "2026-09-21", "31.50")
	est := f.post(t, "2026-09-22",
		LineInput{AccountID: f.keys["travel"], Commodity: "USD", Amount: d("300")},
		LineInput{AccountID: card, Amount: d("-9450")})
	f.mapSourceAll(t, map[string]int64{"card": card})
	if _, err := f.svc.Import(f.ctx, f.acc, bankBatch(
		ImportRowInput{Kind: "transaction", Account: "card", ID: "x", Date: day("2026-09-24"), Amount: d("-9468"), Description: "OTA"},
	)); err != nil {
		t.Fatal(err)
	}
	r := f.queue(t)["testbank:x"]
	if r.Proposal != "clears" {
		t.Fatalf("row = %+v", r)
	}
	if _, err := f.svc.AcceptRow(f.ctx, f.acc, r.ID, nil); err != nil {
		t.Fatal(err)
	}
	v, _ := f.svc.GetTransaction(f.ctx, f.acc, est.ID)
	if len(v.Postings) != 2 || v.Postings[0].Commodity != "USD" || !v.Postings[0].Amount.Equal(d("300")) ||
		!v.Postings[0].BaseAmount.Equal(d("9468")) || !v.Postings[1].Amount.Equal(d("-9468")) {
		t.Fatalf("settled = %+v", v.Postings)
	}
}

func (f *fixture) mapSourceAll(t *testing.T, m map[string]int64) {
	t.Helper()
	// Register the source accounts with an empty batch, then map them.
	if _, err := f.svc.Import(f.ctx, f.acc, bankBatch()); err != nil {
		t.Fatal(err)
	}
	for ext, id := range m {
		f.mapSource(t, ext, id)
	}
}
