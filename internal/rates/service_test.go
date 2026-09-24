package rates

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cwchen-twn/rigel-ledger/internal/dbtest"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
)

// fakeProviders serves er-api's and fawaz's formats. erDown makes er-api
// answer 503; fawaz serves any date as that date.
func fakeProviders(t *testing.T, erDown *atomic.Bool) (ERAPI, Fawaz, *atomic.Int32) {
	t.Helper()
	fawazHits := &atomic.Int32{}
	mux := http.NewServeMux()
	mux.HandleFunc("/v6/latest/USD", func(w http.ResponseWriter, r *http.Request) {
		if erDown.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		// 2026-09-24 00:02:32 UTC
		fmt.Fprint(w, `{"result":"success","time_last_update_unix":1790208152,"base_code":"USD",
			"rates":{"USD":1,"TWD":32.4567,"PYG":7300.12,"EUR":0.921234567890123456,"XXX":1}}`)
	})
	mux.HandleFunc("/fawaz/", func(w http.ResponseWriter, r *http.Request) {
		fawazHits.Add(1)
		// /fawaz/<tag>/currencies/usd.json
		tag := strings.Split(strings.TrimPrefix(r.URL.Path, "/fawaz/"), "/")[0]
		date := tag
		if tag == "latest" {
			date = "2026-09-23"
		}
		fmt.Fprintf(w, `{"date":"%s","usd":{"twd":32.5,"pyg":7299.5,"eur":0.92,"1inch":9.5}}`, date)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return ERAPI{BaseURL: srv.URL, Client: srv.Client()},
		Fawaz{URLs: []string{srv.URL + "/fawaz/{date}"}, Client: srv.Client()}, fawazHits
}

func TestRefreshStoresUSDRatesAndFallsBack(t *testing.T) {
	store := dbtest.New(t)
	ctx := context.Background()
	down := &atomic.Bool{}
	er, fz, _ := fakeProviders(t, down)
	s := New(store, []Provider{er, fz}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	r, err := s.Refresh(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Source != "er-api" || r.RateDate.Format("2006-01-02") != "2026-09-24" || r.Rates != 3 {
		t.Fatalf("result = %+v", r)
	}
	// Exact decimals, not floats; any pair crosses through USD.
	day := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	eur, ok, err := ledger.RateOn(ctx, store.Queries, "USD", "EUR", day)
	if err != nil || !ok || eur.String() != "0.921234567890123456" {
		t.Fatalf("USD->EUR = %s %v %v", eur, ok, err)
	}
	pygTwd, ok, _ := ledger.RateOn(ctx, store.Queries, "PYG", "TWD", day)
	if !ok || pygTwd.StringFixed(8) != "0.00444605" { // 32.4567 / 7300.12 = 0.0044460502...
		t.Fatalf("PYG->TWD = %s", pygTwd.StringFixed(8))
	}

	// er-api down: fawaz takes over, and the failure is on record.
	down.Store(true)
	r, err = s.Refresh(ctx, nil)
	if err != nil || r.Source != "fawaz" {
		t.Fatalf("fallback = %+v %v", r, err)
	}
	recent, _ := s.Recent(ctx, 10)
	if len(recent) != 3 || recent[1].Error == "" {
		t.Fatalf("recorded = %+v", recent)
	}
}

func TestLockedBookRatesAreSkipped(t *testing.T) {
	store := dbtest.New(t)
	ctx := context.Background()
	svc := ledger.NewService(store)
	u, err := svc.CreateUser(ctx, ledger.NewUser{Username: "alice", Email: "a@example.com", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.CreateBook(ctx, u.ID, "Home", "TWD")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool.Exec(ctx, "UPDATE books SET lock_date = '2026-09-30' WHERE id = $1", b.ID); err != nil {
		t.Fatal(err)
	}
	down := &atomic.Bool{}
	er, fz, _ := fakeProviders(t, down)
	s := New(store, []Provider{er, fz}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r, err := s.Refresh(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	// TWD is the locked book's base: its USD->TWD rate for a locked day is
	// refused (USD is not a currency of that book, TWD is); the others go in.
	if r.Skipped != 1 || r.Rates != 2 {
		t.Fatalf("result = %+v", r)
	}
}

func TestTickBackfillsMissingDaysOnce(t *testing.T) {
	store := dbtest.New(t)
	ctx := context.Background()
	down := &atomic.Bool{}
	er, fz, hits := fakeProviders(t, down)
	s := New(store, []Provider{er, fz}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.now = func() time.Time { return time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC) }

	s.Tick(ctx)
	// er-api gave the 24th; fawaz filled the 14 days before it.
	if got := hits.Load(); got != backfillDays {
		t.Fatalf("fawaz calls = %d, want %d", got, backfillDays)
	}
	if missing := s.missingDays(ctx); len(missing) != 0 {
		t.Fatalf("still missing: %v", missing)
	}
	// A second tick within the refresh window does nothing at all.
	s.Tick(ctx)
	if got := hits.Load(); got != backfillDays {
		t.Fatalf("second tick fetched again: %d", got)
	}
}
