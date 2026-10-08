package routes

import (
	"fmt"
	"testing"
)

// #87 over HTTP: state a wallet's balance, see the drift, book the difference.
func TestSetBalanceAndAdjust(t *testing.T) {
	f := newAPI(t)
	alice := f.browser("alice")
	var book BookDTO
	alice.json("POST", "/api/books", map[string]string{"name": "Family", "base_currency": "TWD"}, 201, &book)
	ids := accountIDs(t, alice, book.ID)
	base := fmt.Sprintf("/api/books/%d", book.ID)
	cash := fmt.Sprintf("%s/accounts/%d", base, ids["cash"])

	var b BalanceDTO
	alice.json("POST", cash+"/balance", map[string]string{"date": "2026-09-30", "amount": "-250"}, 200, &b)
	if b.Asserted.String() != "-250" || !b.Booked.IsZero() {
		t.Fatalf("balance = %+v", b)
	}
	var drift []DriftDTO
	alice.json("GET", base+"/drift", nil, 200, &drift)
	if len(drift) != 1 || drift[0].Source != "manual" || drift[0].Since != nil || drift[0].First.Format("2006-01-02") != "2026-09-30" {
		t.Fatalf("drift = %+v", drift)
	}

	var txn TransactionDTO
	alice.json("POST", cash+"/adjust", map[string]any{"date": "2026-09-30", "counter_id": ids["other_expense"]}, 201, &txn)
	if txn.Source != "adjustment" || len(txn.Postings) != 2 || txn.Postings[0].Amount.String() != "-250" {
		t.Fatalf("adjustment = %+v", txn)
	}
	alice.json("GET", base+"/drift", nil, 200, &drift)
	if len(drift) != 0 {
		t.Fatalf("drift after = %+v", drift)
	}
	if res, body := alice.do("POST", cash+"/adjust", map[string]any{"date": "2026-09-30", "counter_id": ids["other_expense"]}); res.StatusCode != 422 ||
		errorCode(t, body) != "nothing_to_adjust" {
		t.Fatalf("again = %d %s", res.StatusCode, body)
	}

	alice.json("POST", base+"/members", map[string]string{"username": "bob", "role": "viewer"}, 204, nil)
	if res, _ := f.browser("bob").do("POST", cash+"/balance", map[string]string{"date": "2026-10-01", "amount": "0"}); res.StatusCode != 403 {
		t.Fatalf("viewer set a balance: %d", res.StatusCode)
	}
}
