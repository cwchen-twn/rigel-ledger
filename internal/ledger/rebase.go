package ledger

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

// RebaseGap is a rate the rebase needs and does not have.
type RebaseGap struct {
	From, To string
	Date     time.Time
}

// RebasePlan is what a change of base currency would do.
type RebasePlan struct {
	From, To     string
	Transactions int
	Postings     int
	// Transactions whose re-translated legs did not sum to zero: the
	// difference goes to FX gain/loss, as IAS 21 books it.
	Adjusted int
	// Sum of those differences, in the new base.
	Residue decimal.Decimal
	Gaps    []RebaseGap
}

type rebaseLine struct {
	id, txn   int64
	commodity string
	amount    decimal.Decimal
	newBase   decimal.Decimal
	date      time.Time
}

// Rebase changes a book's base (functional) currency: every posting's base
// amount is recomputed at its transaction date's rate (IAS 21 on a change of
// functional currency). Money in another currency is re-translated from its
// own amount; what is carried at cost (securities, points, and amounts in the
// old base) has that cost converted. Rounding and rate differences inside a
// transaction go to the FX gain/loss account, so each still balances.
//
// It refuses, and lists the gaps, if any rate is missing; and while the book
// has a lock date, which a rebase would otherwise rewrite. dryRun reports the
// plan and changes nothing. The whole change is one transaction, audited
// posting by posting.
func (s *Service) Rebase(ctx context.Context, a Access, to string, dryRun bool) (RebasePlan, error) {
	if err := a.require(db.MemberRoleOwner); err != nil {
		return RebasePlan{}, err
	}
	to = strings.ToUpper(strings.TrimSpace(to))
	from := a.Book.BaseCurrency
	if err := s.validCurrency(ctx, to); err != nil {
		return RebasePlan{}, fieldError("base_currency", "unknown", "unknown currency %q", to)
	}
	if to == from {
		return RebasePlan{}, fieldError("base_currency", "same", "the book is already in %s", to)
	}
	if a.Book.LockDate != nil {
		return RebasePlan{}, conflict("book_locked", "clear the lock date first: a rebase rewrites every period")
	}
	commods, err := s.commodityMap(ctx)
	if err != nil {
		return RebasePlan{}, err
	}
	dec := int32(commods[to].Decimals)
	rows, err := s.store.RebasePostings(ctx, a.Book.ID)
	if err != nil {
		return RebasePlan{}, err
	}

	plan := RebasePlan{From: from, To: to, Residue: decimal.Zero}
	type key struct {
		c string
		d string
	}
	cache := map[key]Rate{}
	gaps := map[key]bool{}
	rate := func(c string, on time.Time) (decimal.Decimal, bool, error) {
		k := key{c, on.Format(time.DateOnly)}
		r, ok := cache[k]
		if !ok {
			var err error
			if r, err = RateDetail(ctx, s.store.Queries, c, to, on); err != nil {
				return decimal.Zero, false, err
			}
			cache[k] = r
		}
		if !r.OK {
			gaps[k] = true
		}
		return r.Value, r.OK, nil
	}

	var lines []rebaseLine
	txns := map[int64][]int{}
	for _, p := range rows {
		l := rebaseLine{id: p.ID, txn: p.TransactionID, commodity: p.Commodity, amount: p.Amount, date: p.Date}
		com := commods[p.Commodity]
		switch {
		case p.Commodity == to:
			l.newBase = p.Amount // a new-base posting's base IS its amount
		case com.Kind == db.CommodityKindCurrency && p.Commodity != from:
			r, ok, err := rate(p.Commodity, p.Date)
			if err != nil {
				return RebasePlan{}, err
			}
			if ok {
				l.newBase = p.Amount.Mul(r).Round(dec)
			}
		default: // the old base, securities, points: convert the cost
			r, ok, err := rate(from, p.Date)
			if err != nil {
				return RebasePlan{}, err
			}
			if ok {
				l.newBase = p.BaseAmount.Mul(r).Round(dec)
			}
		}
		txns[p.TransactionID] = append(txns[p.TransactionID], len(lines))
		lines = append(lines, l)
	}
	plan.Transactions, plan.Postings = len(txns), len(lines)
	for k := range gaps {
		d, _ := time.Parse(time.DateOnly, k.d)
		plan.Gaps = append(plan.Gaps, RebaseGap{From: k.c, To: to, Date: d})
	}
	sort.Slice(plan.Gaps, func(i, j int) bool {
		if !plan.Gaps[i].Date.Equal(plan.Gaps[j].Date) {
			return plan.Gaps[i].Date.Before(plan.Gaps[j].Date)
		}
		return plan.Gaps[i].From < plan.Gaps[j].From
	})

	residues := map[int64]decimal.Decimal{}
	for txn, idx := range txns {
		sum := decimal.Zero
		for _, i := range idx {
			sum = sum.Add(lines[i].newBase)
		}
		if !sum.IsZero() {
			residues[txn] = sum.Neg()
			plan.Adjusted++
			plan.Residue = plan.Residue.Add(sum.Neg())
		}
	}
	if len(plan.Gaps) > 0 {
		if dryRun {
			return plan, nil
		}
		first := plan.Gaps[0]
		return plan, &Error{Kind: KindInvalid, Code: "rebase_rates_missing",
			Message: fmt.Sprintf("%d rates are missing, the first %s->%s on %s", len(plan.Gaps), first.From, first.To, first.Date.Format(time.DateOnly))}
	}
	if dryRun {
		return plan, nil
	}

	err = s.store.WithTx(ctx, a.UserID, func(q *db.Queries) error {
		if _, err := q.SetBookBaseCurrency(ctx, db.SetBookBaseCurrencyParams{ID: a.Book.ID, BaseCurrency: to}); err != nil {
			return err
		}
		for _, l := range lines {
			if err := q.SetPostingBase(ctx, db.SetPostingBaseParams{ID: l.id, BaseAmount: l.newBase}); err != nil {
				return err
			}
		}
		if len(residues) == 0 {
			return nil
		}
		fx, err := q.GetAccountByTemplateKey(ctx, db.GetAccountByTemplateKeyParams{BookID: a.Book.ID, TemplateKey: ptr(KeyFXGainLoss)})
		if err != nil {
			return err
		}
		for txn, r := range residues {
			pos, err := q.MaxPostingPosition(ctx, txn)
			if err != nil {
				return err
			}
			if _, err := q.CreatePosting(ctx, db.CreatePostingParams{
				TransactionID: txn, AccountID: fx.ID, Position: int16(pos + 1), Commodity: to,
				Amount: r, BaseAmount: r, Status: db.PostingStatusCleared, Memo: "rebase " + from + " -> " + to,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return plan, translate(err, "book")
	}
	return plan, nil
}
