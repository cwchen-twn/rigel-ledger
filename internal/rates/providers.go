// Package rates keeps exchange rates current: a daily snapshot of USD
// against every ISO currency from free, keyless providers, stored in prices
// with the provider as source. ledger.RateOn then crosses any pair through
// USD, so no per-book or per-user list is needed.
package rates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Base is the currency every snapshot is quoted against.
const Base = "USD"

// Snapshot is one provider's rates for one day: 1 USD = Rates[code] code.
type Snapshot struct {
	Source string
	Date   time.Time
	Rates  map[string]decimal.Decimal
}

type Provider interface {
	Name() string
	// Fetch returns the latest snapshot, or the one for day when it is set.
	Fetch(ctx context.Context, day *time.Time) (Snapshot, error)
}

// ErrNoHistory is returned by a provider that only has the latest rates.
var ErrNoHistory = errors.New("provider has no dated snapshots")

func getJSON(ctx context.Context, c *http.Client, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "rigel-ledger (+https://github.com/cwchen-twn/rigel-ledger)")
	res, err := c.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d", url, res.StatusCode)
	}
	dec := json.NewDecoder(io.LimitReader(res.Body, 4<<20))
	dec.UseNumber() // never float64: rates go straight into decimals
	return dec.Decode(v)
}

func toDecimals(in map[string]json.Number, upper bool) map[string]decimal.Decimal {
	out := make(map[string]decimal.Decimal, len(in))
	for k, v := range in {
		d, err := decimal.NewFromString(v.String())
		if err != nil || !d.IsPositive() {
			continue
		}
		if upper {
			k = strings.ToUpper(k)
		}
		out[k] = d
	}
	return out
}

// ERAPI is open.er-api.com: free, keyless, updated daily, 160+ currencies
// including TWD and PYG. Latest only. Its terms ask for an attribution link
// wherever the rates are shown ("Rates By Exchange Rate API").
type ERAPI struct {
	BaseURL string // https://open.er-api.com
	Client  *http.Client
}

func (p ERAPI) Name() string { return "er-api" }

func (p ERAPI) Fetch(ctx context.Context, day *time.Time) (Snapshot, error) {
	if day != nil {
		return Snapshot{}, ErrNoHistory
	}
	var body struct {
		Result      string                 `json:"result"`
		ErrorType   string                 `json:"error-type"`
		LastUpdate  int64                  `json:"time_last_update_unix"`
		BaseCode    string                 `json:"base_code"`
		ConversionR map[string]json.Number `json:"rates"`
	}
	if err := getJSON(ctx, p.Client, strings.TrimRight(p.BaseURL, "/")+"/v6/latest/"+Base, &body); err != nil {
		return Snapshot{}, err
	}
	if body.Result != "success" || body.BaseCode != Base {
		return Snapshot{}, fmt.Errorf("er-api: result %q %s", body.Result, body.ErrorType)
	}
	return Snapshot{Source: p.Name(), Date: time.Unix(body.LastUpdate, 0).UTC().Truncate(24 * time.Hour),
		Rates: toDecimals(body.ConversionR, false)}, nil
}

// Fawaz is fawazahmed0/exchange-api: free, keyless, 200+ currencies, and
// dated snapshots, which is what the backfill uses. Two mirrors.
type Fawaz struct {
	URLs   []string // "https://cdn.jsdelivr.net/npm/@fawazahmed0/currency-api@{date}/v1", ...
	Client *http.Client
}

func (p Fawaz) Name() string { return "fawaz" }

func (p Fawaz) Fetch(ctx context.Context, day *time.Time) (Snapshot, error) {
	tag := "latest"
	if day != nil {
		tag = day.Format("2006-01-02")
	}
	var last error
	for _, u := range p.URLs {
		var raw map[string]json.RawMessage
		url := strings.ReplaceAll(u, "{date}", tag) + "/currencies/usd.json"
		if err := getJSON(ctx, p.Client, url, &raw); err != nil {
			last = err
			continue
		}
		var date string
		var rates map[string]json.Number
		if err := json.Unmarshal(raw["date"], &date); err != nil {
			last = err
			continue
		}
		dec := json.NewDecoder(strings.NewReader(string(raw["usd"])))
		dec.UseNumber()
		if err := dec.Decode(&rates); err != nil {
			last = err
			continue
		}
		d, err := time.Parse("2006-01-02", date)
		if err != nil {
			last = err
			continue
		}
		return Snapshot{Source: p.Name(), Date: d, Rates: toDecimals(rates, true)}, nil
	}
	return Snapshot{}, fmt.Errorf("fawaz: %w", last)
}

// DefaultProviders are the production endpoints, in the order tried.
func DefaultProviders(c *http.Client) []Provider {
	return []Provider{
		ERAPI{BaseURL: "https://open.er-api.com", Client: c},
		Fawaz{URLs: []string{
			"https://cdn.jsdelivr.net/npm/@fawazahmed0/currency-api@{date}/v1",
			"https://{date}.currency-api.pages.dev/v1",
		}, Client: c},
	}
}
