package ledger

import "testing"

// #87: a wallet counted by hand drifts from the books by the cash spent that
// no source saw; booking the difference closes it, and the drift points at
// where the next gap opened.
func TestSetBalanceAndBookTheDifference(t *testing.T) {
	f := setup(t)
	cash := f.keys["cash"]
	f.post(t, "2026-08-31",
		LineInput{AccountID: cash, Amount: d("10000")},
		LineInput{AccountID: f.keys[KeyOpeningBalances], Amount: d("-10000")})

	b, err := f.svc.SetBalance(f.ctx, f.acc, cash, day("2026-09-30"), d("9000"))
	if err != nil || !b.Asserted.Equal(d("9000")) || !b.Booked.Equal(d("10000")) {
		t.Fatalf("balance = %+v %v", b, err)
	}
	drifts, _ := f.svc.Drifts(f.ctx, f.acc)
	if len(drifts) != 1 || drifts[0].Since != nil || !drifts[0].First.Equal(day("2026-09-30")) || drifts[0].Source != "manual" {
		t.Fatalf("drift with nothing agreeing before = %+v", drifts)
	}

	// An earlier count that agreed, and a later one that does not.
	if _, err := f.svc.SetBalance(f.ctx, f.acc, cash, day("2026-09-01"), d("10000")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SetBalance(f.ctx, f.acc, cash, day("2026-10-15"), d("8000")); err != nil {
		t.Fatal(err)
	}
	drifts, _ = f.svc.Drifts(f.ctx, f.acc)
	if len(drifts) != 1 || drifts[0].Since == nil || !drifts[0].Since.Equal(day("2026-09-01")) ||
		!drifts[0].First.Equal(day("2026-09-30")) || !drifts[0].Date.Equal(day("2026-10-15")) {
		t.Fatalf("drift = %+v", drifts)
	}

	v, err := f.svc.BookDifference(f.ctx, f.acc, cash, day("2026-09-30"), f.keys["other_expense"])
	if err != nil {
		t.Fatal(err)
	}
	if v.Source != "adjustment" || len(v.Postings) != 2 ||
		v.Postings[0].AccountID != cash || !v.Postings[0].Amount.Equal(d("-1000")) ||
		v.Postings[1].AccountID != f.keys["other_expense"] || !v.Postings[1].Amount.Equal(d("1000")) {
		t.Fatalf("adjustment = %+v %+v", v.Transaction, v.Postings)
	}
	drifts, _ = f.svc.Drifts(f.ctx, f.acc)
	if len(drifts) != 1 || !drifts[0].Since.Equal(day("2026-09-30")) || !drifts[0].First.Equal(day("2026-10-15")) ||
		!drifts[0].Booked.Equal(d("9000")) {
		t.Fatalf("drift after the adjustment = %+v", drifts)
	}

	_, err = f.svc.BookDifference(f.ctx, f.acc, cash, day("2026-09-30"), f.keys["other_expense"])
	wantCode(t, err, "nothing_to_adjust")
	_, err = f.svc.BookDifference(f.ctx, f.acc, cash, day("2026-09-29"), f.keys["other_expense"])
	if e := wantCode(t, err, "invalid_input"); e.Fields["date"] != "no_balance" {
		t.Fatalf("fields = %v", e.Fields)
	}
	_, err = f.svc.BookDifference(f.ctx, f.acc, cash, day("2026-10-15"), cash)
	if e := wantCode(t, err, "invalid_input"); e.Fields["counter_id"] != "invalid" {
		t.Fatalf("fields = %v", e.Fields)
	}
	_, err = f.svc.SetBalance(f.ctx, f.acc, f.keys["other_expense"], day("2026-10-15"), d("1"))
	if e := wantCode(t, err, "invalid_input"); e.Fields["account_id"] != "invalid" {
		t.Fatalf("an expense took a balance: %v", e.Fields)
	}
	if _, err := f.svc.BookDifference(f.ctx, f.acc, cash, day("2026-10-15"), f.keys["other_expense"]); err != nil {
		t.Fatal(err)
	}
	if drifts, _ := f.svc.Drifts(f.ctx, f.acc); len(drifts) != 0 {
		t.Fatalf("every count agrees, drift = %+v", drifts)
	}
}
