package routes

import (
	"fmt"
	"testing"
)

func TestBrokerageOverTheAPI(t *testing.T) {
	f := newAPI(t)
	alice := f.browser("alice")
	var book BookDTO
	alice.json("POST", "/api/books", map[string]string{"name": "Family", "base_currency": "TWD"}, 201, &book)
	base := fmt.Sprintf("/api/books/%d", book.ID)
	ids := accountIDs(t, alice, book.ID)

	var res ImportResultDTO
	alice.json("POST", base+"/imports", map[string]any{
		"connector": "tw-tdcc", "label": "September",
		"accounts": []map[string]any{{"id": "9800-1234567", "label": "元大", "currency": "TWD", "kind": "brokerage"}},
		"rows": []map[string]any{
			{"kind": "trade", "account": "9800-1234567", "id": "t1", "date": "2026-09-02", "security": "XTAI:0050",
				"security_name": "元大台灣50", "units": "2000", "price": "180.5"},
			{"kind": "holding", "account": "9800-1234567", "id": "h1", "date": "2026-09-30", "security": "XTAI:0050", "units": "2000"},
		},
	}, 201, &res)
	if res.Staged != 2 {
		t.Fatalf("staged = %+v", res)
	}
	var srcs []SourceAccountDTO
	alice.json("GET", base+"/imports/sources", nil, 200, &srcs)
	if len(srcs) != 1 || srcs[0].Kind != "brokerage" {
		t.Fatalf("sources = %+v", srcs)
	}
	alice.json("PATCH", fmt.Sprintf("%s/imports/sources/%d", base, srcs[0].ID),
		map[string]any{"account_id": ids["investments"], "settlement_account_id": ids["bank_checking"]}, 204, nil)

	var queue []ImportRowDTO
	alice.json("GET", base+"/imports/queue", nil, 200, &queue)
	if len(queue) != 1 || queue[0].Kind != "trade" || *queue[0].Security != "XTAI:0050" || *queue[0].SecurityName != "元大台灣50" ||
		queue[0].Units.String() != "2000" || queue[0].Price.String() != "180.5" || queue[0].Cash != nil ||
		queue[0].SettlementAccountID == nil {
		t.Fatalf("queue = %+v", queue)
	}
	row := queue[0].ID

	// Without the cash the row fails, by its field's code; with it, it books.
	var out AcceptResultDTO
	alice.json("POST", base+"/imports/accept", map[string]any{"row_ids": []int64{row}}, 200, &out)
	if len(out.Failed) != 1 || out.Failed[0].Code != "required" {
		t.Fatalf("accept without cash = %+v", out)
	}
	alice.json("POST", base+"/imports/accept", map[string]any{"row_ids": []int64{row}, "cash": map[string]string{fmt.Sprint(row): "-361515"}}, 200, &out)
	if len(out.Accepted) != 1 {
		t.Fatalf("accept = %+v", out)
	}
	var txn TransactionDTO
	alice.json("GET", fmt.Sprintf("%s/transactions/%d", base, out.Transactions[0]), nil, 200, &txn)
	if txn.Postings[0].Commodity != "XTAI:0050" || txn.Postings[0].BaseAmount.String() != "361515" {
		t.Fatalf("buy = %+v", txn.Postings)
	}
}
