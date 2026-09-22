package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

// LineInput is one posting as a client sends it. Amount is signed (debit > 0)
// and in Commodity. BaseAmount may be given for a foreign line to pin the
// rate the user actually got; otherwise it is derived from prices.
type LineInput struct {
	AccountID  int64
	Commodity  string
	Amount     decimal.Decimal
	BaseAmount *decimal.Decimal
	Status     db.PostingStatus
	ClearedOn  *time.Time
	Memo       string
}

// lineContext is what resolving a line needs to know about its book.
type lineContext struct {
	book     db.Book
	accounts map[int64]db.Account
	decimals map[string]int32
	date     time.Time
}

func (s *Service) newLineContext(ctx context.Context, q *db.Queries, book db.Book, date time.Time) (*lineContext, error) {
	decs, err := s.commodityDecimals(ctx)
	if err != nil {
		return nil, err
	}
	accts, err := q.ListAccounts(ctx, book.ID)
	if err != nil {
		return nil, err
	}
	m := make(map[int64]db.Account, len(accts))
	for _, a := range accts {
		m[a.ID] = a
	}
	return &lineContext{book: book, accounts: m, decimals: decs, date: date}, nil
}

func hasPrecision(d decimal.Decimal, places int32) bool {
	return d.Equal(d.Round(places))
}

// resolveLine validates one line and fills in its commodity and base amount.
func (s *Service) resolveLine(ctx context.Context, q *db.Queries, lc *lineContext, i int, in LineInput) (db.CreatePostingParams, error) {
	f := func(name string) string { return fmt.Sprintf("lines[%d].%s", i, name) }

	a, ok := lc.accounts[in.AccountID]
	if !ok {
		return db.CreatePostingParams{}, fieldError(f("account_id"), "not_found", "account %d is not in this book", in.AccountID)
	}
	if a.IsPlaceholder {
		return db.CreatePostingParams{}, fieldError(f("account_id"), "placeholder", "account %d only groups other accounts", a.ID)
	}
	if a.ArchivedAt != nil {
		return db.CreatePostingParams{}, fieldError(f("account_id"), "archived", "account %d is archived", a.ID)
	}

	commodity := in.Commodity
	switch {
	case a.Commodity != nil && commodity == "":
		commodity = *a.Commodity
	case a.Commodity != nil && commodity != *a.Commodity:
		return db.CreatePostingParams{}, fieldError(f("commodity"), "mismatch", "account %d holds %s, not %s", a.ID, *a.Commodity, commodity)
	case commodity == "":
		commodity = lc.book.BaseCurrency
	}
	places, ok := lc.decimals[commodity]
	if !ok {
		return db.CreatePostingParams{}, fieldError(f("commodity"), "unknown", "unknown commodity %s", commodity)
	}

	if in.Amount.IsZero() {
		return db.CreatePostingParams{}, fieldError(f("amount"), "zero", "amount must not be zero")
	}
	if !hasPrecision(in.Amount, places) {
		return db.CreatePostingParams{}, fieldError(f("amount"), "too_precise", "%s allows %d decimal places", commodity, places)
	}

	base := lc.book.BaseCurrency
	basePlaces := lc.decimals[base]
	var baseAmount decimal.Decimal
	switch {
	case commodity == base:
		baseAmount = in.Amount
	case in.BaseAmount != nil:
		baseAmount = *in.BaseAmount
		if !hasPrecision(baseAmount, basePlaces) {
			return db.CreatePostingParams{}, fieldError(f("base_amount"), "too_precise", "%s allows %d decimal places", base, basePlaces)
		}
		if baseAmount.Sign()*in.Amount.Sign() < 0 {
			return db.CreatePostingParams{}, fieldError(f("base_amount"), "sign", "base_amount must have the same sign as amount")
		}
	default:
		rate, ok, err := RateOn(ctx, q, commodity, base, lc.date)
		if err != nil {
			return db.CreatePostingParams{}, err
		}
		if !ok {
			return db.CreatePostingParams{}, fieldError(f("base_amount"), "rate_missing",
				"no %s/%s rate on or before %s; enter the base amount or add a rate", commodity, base, lc.date.Format(time.DateOnly))
		}
		baseAmount = in.Amount.Mul(rate).Round(basePlaces)
	}

	status := in.Status
	if status == "" {
		status = db.PostingStatusUncleared
	}
	if !status.Valid() {
		return db.CreatePostingParams{}, fieldError(f("status"), "invalid", "unknown status %q", status)
	}

	return db.CreatePostingParams{
		AccountID:  a.ID,
		Position:   int16(i),
		Commodity:  commodity,
		Amount:     in.Amount,
		BaseAmount: baseAmount,
		Status:     status,
		ClearedOn:  in.ClearedOn,
		Memo:       in.Memo,
	}, nil
}

// prepareLines resolves every line and checks that the base amounts balance.
// Converting several foreign lines can leave a residue of one minor unit of
// the base currency; that residue is booked to FX gains/losses rather than
// rejected, because the user has no way to fix it by hand. Anything larger is
// a real imbalance.
func (s *Service) prepareLines(ctx context.Context, q *db.Queries, lc *lineContext, lines []LineInput) ([]db.CreatePostingParams, error) {
	if len(lines) < 2 {
		return nil, fieldError("lines", "too_few", "a transaction needs at least two lines")
	}
	out := make([]db.CreatePostingParams, 0, len(lines)+1)
	total := decimal.Zero
	foreign := false
	for i, in := range lines {
		p, err := s.resolveLine(ctx, q, lc, i, in)
		if err != nil {
			return nil, err
		}
		if p.Commodity != lc.book.BaseCurrency {
			foreign = true
		}
		total = total.Add(p.BaseAmount)
		out = append(out, p)
	}
	if total.IsZero() {
		return out, nil
	}

	base := lc.book.BaseCurrency
	minor := decimal.New(1, -lc.decimals[base])
	if !foreign || total.Abs().GreaterThan(minor) {
		return nil, &Error{Kind: KindInvalid, Code: "unbalanced",
			Message: fmt.Sprintf("lines are unbalanced by %s %s", total.String(), base),
			Fields:  map[string]string{"lines": "unbalanced"}}
	}
	fx, err := q.GetAccountByTemplateKey(ctx, db.GetAccountByTemplateKeyParams{BookID: lc.book.ID, TemplateKey: ptr(KeyFXGainLoss)})
	if err != nil {
		return nil, translate(err, "FX gains/losses account")
	}
	residue := total.Neg()
	out = append(out, db.CreatePostingParams{
		AccountID:  fx.ID,
		Position:   int16(len(out)),
		Commodity:  base,
		Amount:     residue,
		BaseAmount: residue,
		Status:     db.PostingStatusUncleared,
		Memo:       "FX rounding",
	})
	return out, nil
}

func ptr[T any](v T) *T { return &v }
