package routes

import (
	"fmt"
	"testing"
)

func TestInvoiceOverTheAPI(t *testing.T) {
	f := newAPI(t)
	alice := f.browser("alice")
	var book BookDTO
	alice.json("POST", "/api/books", map[string]string{"name": "Family", "base_currency": "TWD"}, 201, &book)
	base := fmt.Sprintf("/api/books/%d", book.ID)
	ids := accountIDs(t, alice, book.ID)
	var paid TransactionDTO
	alice.json("POST", base+"/transactions", map[string]any{"date": "2026-09-03", "payee": "全聯",
		"lines": []map[string]any{{"account_id": ids["groceries"], "amount": "320"}, {"account_id": ids["credit_card"], "amount": "-320"}}}, 201, &paid)
	alice.json("POST", base+"/imports/rules", map[string]any{"pattern": "衛生紙", "account_id": ids["home_maintenance"]}, 201, nil)

	alice.json("POST", base+"/imports", map[string]any{
		"connector": "tw-einvoice", "accounts": []map[string]any{{"id": "carrier", "currency": "TWD"}},
		"rows": []map[string]any{{"kind": "invoice", "account": "carrier", "id": "AB-12345678", "date": "2026-09-02",
			"amount": "-320", "counterparty": "全聯福利中心",
			"items": []map[string]any{{"description": "鮮乳", "quantity": "1", "unit_price": "90", "amount": "90"},
				{"description": "衛生紙", "amount": "230"}}}},
	}, 201, nil)
	var queue []ImportRowDTO
	alice.json("GET", base+"/imports/queue", nil, 200, &queue)
	if len(queue) != 1 || queue[0].Proposal != "enrich" || *queue[0].MatchTransactionID != paid.ID || len(queue[0].Items) != 2 ||
		queue[0].Items[1].AccountID == nil || *queue[0].Items[1].AccountID != ids["home_maintenance"] {
		t.Fatalf("queue = %+v", queue)
	}
	var out AcceptResultDTO
	alice.json("POST", base+"/imports/accept", map[string]any{"row_ids": []int64{queue[0].ID}, "split": true}, 200, &out)
	if len(out.Accepted) != 1 || out.Transactions[0] != paid.ID {
		t.Fatalf("accept = %+v", out)
	}
	var txn TransactionDTO
	alice.json("GET", fmt.Sprintf("%s/transactions/%d", base, paid.ID), nil, 200, &txn)
	if len(txn.Postings) != 3 || len(txn.Items) != 2 || txn.Items[0].Quantity.String() != "1" || txn.Items[0].UnitPrice.String() != "90" {
		t.Fatalf("transaction = %+v %+v", txn.Postings, txn.Items)
	}
}
