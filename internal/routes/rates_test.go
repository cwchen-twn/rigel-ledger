package routes

import (
	"context"
	"fmt"
	"testing"
)

func TestBookRatesAndAdminStatus(t *testing.T) {
	f := newAPI(t)
	alice := f.browser("alice")
	var book BookDTO
	alice.json("POST", "/api/books", map[string]string{"name": "B", "base_currency": "TWD"}, 201, &book)
	alice.json("POST", fmt.Sprintf("/api/books/%d/accounts", book.ID),
		map[string]any{"class": "asset", "name": "US bank", "commodity": "USD"}, 201, nil)

	// A scraped snapshot and one manual rate.
	if _, err := f.store.Pool.Exec(context.Background(), `INSERT INTO prices (commodity, quote, date, rate, source)
		VALUES ('USD', 'TWD', current_date - 1, 32.5, 'er-api'), ('USD', 'PYG', current_date - 1, 7300, 'er-api')`); err != nil {
		t.Fatal(err)
	}
	alice.json("POST", fmt.Sprintf("/api/books/%d/prices", book.ID),
		map[string]string{"commodity": "EUR", "quote": "TWD", "date": "2026-01-02", "rate": "35.1"}, 201, nil)

	var manual []PriceDTO
	alice.json("GET", fmt.Sprintf("/api/books/%d/prices?source=manual", book.ID), nil, 200, &manual)
	if len(manual) != 1 || manual[0].Commodity != "EUR" {
		t.Fatalf("manual rates = %+v", manual)
	}

	// The book uses TWD (base) and USD (an account); alice displays in USD too.
	var current []CurrentRateDTO
	alice.json("GET", fmt.Sprintf("/api/books/%d/rates/current", book.ID), nil, 200, &current)
	if len(current) != 1 || current[0].Currency != "USD" || current[0].Rate == nil || *current[0].Rate != "32.5" {
		t.Fatalf("current = %+v", current)
	}

	var status RateStatusDTO
	alice.json("GET", "/api/admin/rates", nil, 200, &status)
	if status.Scheduler {
		t.Fatalf("status without a scheduler = %+v", status)
	}
	res, b := alice.do("POST", "/api/admin/rates/refresh", map[string]any{})
	if res.StatusCode != 409 || errorCode(t, b) != "rates_disabled" {
		t.Fatalf("refresh without a scheduler = %d %s", res.StatusCode, b)
	}
}

func TestReportEndpoints(t *testing.T) {
	f := newAPI(t)
	alice := f.browser("alice")
	var book BookDTO
	alice.json("POST", "/api/books", map[string]string{"name": "B", "base_currency": "TWD"}, 201, &book)
	base := fmt.Sprintf("/api/books/%d/reports", book.ID)

	var bs BalanceSheetDTO
	alice.json("GET", base+"/balance-sheet?as_of=2026-03-31&currency=TWD", nil, 200, &bs)
	if bs.AsOf != "2026-03-31" || bs.Currency != "TWD" || !bs.TotalAssets.Equal(bs.TotalEquity) {
		t.Fatalf("balance sheet = %+v", bs)
	}
	// No currency: the user's display currency (USD); no rate yet, so the
	// report stays in the base currency and says USD is missing.
	alice.json("GET", base+"/balance-sheet", nil, 200, &bs)
	if bs.Currency != "TWD" || len(bs.Missing) != 1 || bs.Missing[0] != "USD" {
		t.Fatalf("untranslatable = %+v", bs)
	}
	var is IncomeStatementDTO
	alice.json("GET", base+"/income-statement?to=2026-06-30", nil, 200, &is)
	if is.From != "2026-01-01" || is.To != "2026-06-30" {
		t.Fatalf("default period = %s..%s", is.From, is.To)
	}
	var cf CashFlowDTO
	alice.json("GET", base+"/cash-flow?from=2026-01-01&to=2026-03-31&currency=TWD", nil, 200, &cf)
	if res, _ := alice.do("GET", base+"/cash-flow?from=2026-04-01&to=2026-03-31", nil); res.StatusCode != 422 {
		t.Fatalf("backwards period = %d", res.StatusCode)
	}
	if res, _ := alice.do("GET", base+"/balance-sheet?currency=NOPE", nil); res.StatusCode != 422 {
		t.Fatalf("unknown currency = %d", res.StatusCode)
	}
	// Members only.
	if res, _ := f.browser("bob").do("GET", base+"/balance-sheet", nil); res.StatusCode != 404 {
		t.Fatalf("non-member = %d", res.StatusCode)
	}
}

func TestTagAndRebaseEndpoints(t *testing.T) {
	f := newAPI(t)
	alice := f.browser("alice")
	var book BookDTO
	alice.json("POST", "/api/books", map[string]string{"name": "B", "base_currency": "TWD"}, 201, &book)
	var tags TagReportDTO
	alice.json("GET", fmt.Sprintf("/api/books/%d/reports/tags?currency=TWD", book.ID), nil, 200, &tags)
	if len(tags.Tags) != 0 {
		t.Fatalf("tags = %+v", tags)
	}
	if res, _ := alice.do("GET", fmt.Sprintf("/api/books/%d/reports/tag", book.ID), nil); res.StatusCode != 400 {
		t.Fatalf("tag without name = %d", res.StatusCode)
	}
	// An empty book rebases at once: nothing to re-translate.
	var plan RebasePlanDTO
	alice.json("POST", fmt.Sprintf("/api/books/%d/rebase", book.ID), map[string]any{"base_currency": "USD", "dry_run": true}, 200, &plan)
	if plan.Done || plan.From != "TWD" || plan.To != "USD" {
		t.Fatalf("dry run = %+v", plan)
	}
	alice.json("POST", fmt.Sprintf("/api/books/%d/rebase", book.ID), map[string]any{"base_currency": "USD"}, 200, &plan)
	var got BookDTO
	alice.json("GET", fmt.Sprintf("/api/books/%d", book.ID), nil, 200, &got)
	if !plan.Done || got.BaseCurrency != "USD" {
		t.Fatalf("after rebase: %+v %+v", plan, got)
	}
	// Owners only: bob as an editor is refused.
	alice.json("POST", fmt.Sprintf("/api/books/%d/members", book.ID), map[string]string{"username": "bob", "role": "editor"}, 204, nil)
	if res, _ := f.browser("bob").do("POST", fmt.Sprintf("/api/books/%d/rebase", book.ID), map[string]any{"base_currency": "TWD"}); res.StatusCode != 403 {
		t.Fatalf("editor rebase = %d", res.StatusCode)
	}
}

func TestDeleteBookEndpoint(t *testing.T) {
	f := newAPI(t)
	alice := f.browser("alice")
	var book BookDTO
	alice.json("POST", "/api/books", map[string]string{"name": "Mega Bank 天母", "base_currency": "USD"}, 201, &book)
	alice.json("POST", fmt.Sprintf("/api/books/%d/members", book.ID), map[string]string{"username": "bob", "role": "editor"}, 204, nil)
	if res, _ := f.browser("bob").do("DELETE", fmt.Sprintf("/api/books/%d", book.ID), map[string]string{"confirm": "Mega Bank 天母"}); res.StatusCode != 403 {
		t.Fatalf("editor delete = %d", res.StatusCode)
	}
	if res, _ := alice.do("DELETE", fmt.Sprintf("/api/books/%d", book.ID), map[string]string{"confirm": "Mega"}); res.StatusCode != 422 {
		t.Fatalf("wrong name = %d", res.StatusCode)
	}
	alice.json("DELETE", fmt.Sprintf("/api/books/%d", book.ID), map[string]string{"confirm": "Mega Bank 天母"}, 204, nil)
	if res, _ := alice.do("GET", fmt.Sprintf("/api/books/%d", book.ID), nil); res.StatusCode != 404 {
		t.Fatalf("after delete = %d", res.StatusCode)
	}
}
