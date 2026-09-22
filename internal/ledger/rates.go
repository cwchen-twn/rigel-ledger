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
	if from == to {
		return decimal.NewFromInt(1), true, nil
	}
	if r, ok, err := directOrInverse(ctx, q, from, to, on); err != nil || ok {
		return r, ok, err
	}
	if from == pivotCurrency || to == pivotCurrency {
		return decimal.Zero, false, nil
	}
	r1, ok1, err := directOrInverse(ctx, q, from, pivotCurrency, on)
	if err != nil || !ok1 {
		return decimal.Zero, false, err
	}
	r2, ok2, err := directOrInverse(ctx, q, pivotCurrency, to, on)
	if err != nil || !ok2 {
		return decimal.Zero, false, err
	}
	return r1.Mul(r2), true, nil
}

func directOrInverse(ctx context.Context, q *db.Queries, from, to string, on time.Time) (decimal.Decimal, bool, error) {
	p, err := q.LatestPrice(ctx, db.LatestPriceParams{Commodity: from, Quote: to, OnDate: on})
	if err == nil {
		return p.Rate, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return decimal.Zero, false, err
	}
	p, err = q.LatestPrice(ctx, db.LatestPriceParams{Commodity: to, Quote: from, OnDate: on})
	if err == nil {
		return decimal.NewFromInt(1).DivRound(p.Rate, rateDivisionPlaces), true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return decimal.Zero, false, err
	}
	return decimal.Zero, false, nil
}
