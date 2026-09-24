package ledger

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

// Cross rates go through this currency when neither direct nor inverse is known.
const pivotCurrency = "USD"

// Enough places that a PYG->USD rate (about 0.000137) keeps 14+ significant digits.
const rateDivisionPlaces = 18

// RateOn returns how many units of `to` one unit of `from` was worth on the
// date: the newest price on or before it, tried direct, then inverse, then
// crossed through USD. ok is false when no path has a rate.
func RateOn(ctx context.Context, q *db.Queries, from, to string, on time.Time) (rate decimal.Decimal, ok bool, err error) {
	r, err := RateDetail(ctx, q, from, to, on)
	return r.Value, r.OK, err
}

// Rate is a looked-up rate and where it came from, for reports that show
// their inputs.
type Rate struct {
	From, To string
	Value    decimal.Decimal
	OK       bool
	// The date of the price used: the older of the two legs when crossed.
	Date time.Time
	// "direct", "inverse" or "via USD".
	Path string
}

// RateDetail is RateOn with the date and path of what it found.
func RateDetail(ctx context.Context, q *db.Queries, from, to string, on time.Time) (Rate, error) {
	out := Rate{From: from, To: to}
	if from == to {
		out.Value, out.OK, out.Date, out.Path = decimal.NewFromInt(1), true, on, "same"
		return out, nil
	}
	r, d, path, ok, err := directOrInverse(ctx, q, from, to, on)
	if err != nil || ok {
		out.Value, out.OK, out.Date, out.Path = r, ok, d, path
		return out, err
	}
	if from == pivotCurrency || to == pivotCurrency {
		return out, nil
	}
	r1, d1, _, ok1, err := directOrInverse(ctx, q, from, pivotCurrency, on)
	if err != nil || !ok1 {
		return out, err
	}
	r2, d2, _, ok2, err := directOrInverse(ctx, q, pivotCurrency, to, on)
	if err != nil || !ok2 {
		return out, err
	}
	if d2.Before(d1) {
		d1 = d2
	}
	out.Value, out.OK, out.Date, out.Path = r1.Mul(r2), true, d1, "via "+pivotCurrency
	return out, nil
}

func directOrInverse(ctx context.Context, q *db.Queries, from, to string, on time.Time) (decimal.Decimal, time.Time, string, bool, error) {
	p, err := q.LatestPrice(ctx, db.LatestPriceParams{Commodity: from, Quote: to, OnDate: on})
	if err == nil {
		return p.Rate, p.Date, "direct", true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return decimal.Zero, time.Time{}, "", false, err
	}
	p, err = q.LatestPrice(ctx, db.LatestPriceParams{Commodity: to, Quote: from, OnDate: on})
	if err == nil {
		return decimal.NewFromInt(1).DivRound(p.Rate, rateDivisionPlaces), p.Date, "inverse", true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return decimal.Zero, time.Time{}, "", false, err
	}
	return decimal.Zero, time.Time{}, "", false, nil
}
