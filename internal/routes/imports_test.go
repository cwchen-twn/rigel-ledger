package routes

import (
	"fmt"
	"testing"
)

func TestRunnerTokenSendsABatchAndNothingElse(t *testing.T) {
	f := newAPI(t)
	alice := f.browser("alice")
	var book BookDTO
	alice.json("POST", "/api/books", map[string]string{"name": "Family", "base_currency": "TWD"}, 201, &book)
	base := fmt.Sprintf("/api/books/%d", book.ID)
	ids := accountIDs(t, alice, book.ID)

	res, b := alice.do("POST", "/api/me/tokens", map[string]any{"label": ""})
	if res.StatusCode != 422 {
		t.Fatalf("token without a label = %d %s", res.StatusCode, b)
	}
	var made TokenCreatedDTO
	alice.json("POST", "/api/me/tokens", map[string]any{"label": "tw-sync", "days": 30}, 201, &made)
	if made.Token == "" || made.Session.Kind != "token" || made.Session.Label != "tw-sync" {
		t.Fatalf("token = %+v", made)
	}
	runner := &client{f: f, bearer: made.Token}

	// The scope: who am I, which books, send a batch, list the sources.
	runner.json("GET", "/api/me", nil, 200, nil)
	runner.json("GET", "/api/books", nil, 200, nil)
	for _, c := range []struct{ method, path string }{
		{"GET", base + "/accounts"},
		{"GET", base + "/transactions"},
		{"GET", base + "/imports/queue"},
		{"POST", base + "/imports/accept"},
		{"POST", "/api/me/tokens"},
		{"POST", "/api/books"},
		{"GET", "/api/me/sessions"},
	} {
		res, b := runner.do(c.method, c.path, map[string]any{})
		if res.StatusCode != 403 || errorCode(t, b) != "token_scope" {
			t.Errorf("token %s %s = %d %s, want 403 token_scope", c.method, c.path, res.StatusCode, b)
		}
	}

	batch := map[string]any{
		"connector": "testbank", "label": "September",
		"accounts": []map[string]any{{"id": "chk-1234", "label": "Checking ...1234", "currency": "TWD"}},
		"rows": []map[string]any{
			{"kind": "transaction", "account": "chk-1234", "id": "t1", "date": "2026-09-01", "amount": "-320",
				"description": "PX MART", "raw": map[string]any{"memo": "POS 0912"}},
			{"kind": "transaction", "account": "chk-1234", "id": "t2", "date": "2026-09-02", "amount": "-150", "description": "Starbucks"},
			{"kind": "balance", "account": "chk-1234", "id": "b0902", "date": "2026-09-02", "amount": "9530"},
		},
	}
	var got ImportResultDTO
	runner.json("POST", base+"/imports", batch, 201, &got)
	if got.Staged != 3 || got.Duplicates != 0 {
		t.Fatalf("first batch = %+v", got)
	}
	runner.json("POST", base+"/imports", batch, 201, &got)
	if got.Staged != 0 || got.Duplicates != 3 {
		t.Fatalf("resend = %+v", got)
	}
	var sources []SourceAccountDTO
	runner.json("GET", base+"/imports/sources", nil, 200, &sources)
	if len(sources) != 1 || sources[0].AccountID != nil || sources[0].Pending != 3 {
		t.Fatalf("sources = %+v", sources)
	}

	// A person maps the account, adds a rule and accepts.
	alice.json("POST", base+"/imports/rules", map[string]any{"pattern": "PX", "account_id": ids["groceries"]}, 201, nil)
	alice.json("PATCH", fmt.Sprintf("%s/imports/sources/%d", base, sources[0].ID), map[string]any{"account_id": ids["bank_checking"]}, 204, nil)
	var queue []ImportRowDTO
	alice.json("GET", base+"/imports/queue", nil, 200, &queue)
	if len(queue) != 2 {
		t.Fatalf("queue = %+v", queue)
	}
	var accepted AcceptResultDTO
	alice.json("POST", base+"/imports/accept", map[string]any{"row_ids": []int64{queue[0].ID, queue[1].ID}}, 200, &accepted)
	// Starbucks (newest, first) has no category; PX MART goes by the rule.
	if len(accepted.Accepted) != 1 || len(accepted.Failed) != 1 || accepted.Failed[0].Code != "required" {
		t.Fatalf("accept = %+v", accepted)
	}
	alice.json("POST", base+"/imports/accept", map[string]any{"row_ids": []int64{accepted.Failed[0].RowID}, "account_id": ids["dining"]}, 200, &accepted)
	if len(accepted.Accepted) != 1 {
		t.Fatalf("accept with a category = %+v", accepted)
	}

	var drift []DriftDTO
	alice.json("GET", base+"/drift", nil, 200, &drift)
	if len(drift) != 1 || drift[0].Asserted.String() != "9530" || drift[0].Booked.String() != "-470" {
		t.Fatalf("drift = %+v", drift)
	}

	// A viewer sees the queue but cannot change it.
	alice.json("POST", base+"/members", map[string]string{"username": "bob", "role": "viewer"}, 204, nil)
	bob := f.browser("bob")
	bob.json("GET", base+"/imports/queue", nil, 200, nil)
	if res, _ := bob.do("POST", base+"/imports", batch); res.StatusCode != 403 {
		t.Fatalf("viewer import = %d", res.StatusCode)
	}

	// Revoking the token ends it.
	alice.json("DELETE", fmt.Sprintf("/api/me/sessions/%d", made.Session.ID), nil, 204, nil)
	if res, _ := runner.do("GET", "/api/me", nil); res.StatusCode != 401 {
		t.Fatalf("revoked token = %d", res.StatusCode)
	}
}
