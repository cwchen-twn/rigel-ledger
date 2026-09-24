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
