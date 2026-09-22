package ledger

import (
	"context"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

type CommodityAmount struct {
	Commodity string
	Amount    decimal.Decimal
}

// AccountBalance uses the stored sign convention: debit > 0. The frontend
// flips credit-normal classes (liability, equity, income) for display.
type AccountBalance struct {
	AccountID int64
	// Own postings only.
	Amounts    []CommodityAmount
	BaseAmount decimal.Decimal
	// The account plus all of its descendants.
	TotalAmounts    []CommodityAmount
	TotalBaseAmount decimal.Decimal
}

type Balances struct {
	AsOf         time.Time
	BaseCurrency string
	Accounts     []AccountBalance
	ClassTotals  map[db.AccountClass]decimal.Decimal
	// The sum of every base amount. It is zero by construction; the page
	// shows it anyway, which is the only "trial balance" this system keeps.
	Check decimal.Decimal
}

func sortedAmounts(m map[string]decimal.Decimal) []CommodityAmount {
	out := make([]CommodityAmount, 0, len(m))
	for c, a := range m {
		if !a.IsZero() {
			out = append(out, CommodityAmount{Commodity: c, Amount: a})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Commodity < out[j].Commodity })
	return out
}

// Balances returns every account's balance as of a date, rolled up the tree.
// It is one aggregate query plus an in-memory walk; at household volume that
// is milliseconds, so nothing is cached or stored.
func (s *Service) Balances(ctx context.Context, a Access, asOf time.Time) (Balances, error) {
	accounts, err := s.store.ListAccounts(ctx, a.Book.ID)
	if err != nil {
		return Balances{}, err
	}
	sums, err := s.store.AccountSums(ctx, db.AccountSumsParams{BookID: a.Book.ID, AsOf: asOf})
	if err != nil {
		return Balances{}, err
	}

	parent := make(map[int64]*int64, len(accounts))
	class := make(map[int64]db.AccountClass, len(accounts))
	for _, acc := range accounts {
		parent[acc.ID] = acc.ParentID
		class[acc.ID] = acc.Class
	}

	own := map[int64]map[string]decimal.Decimal{}
	ownBase := map[int64]decimal.Decimal{}
	total := map[int64]map[string]decimal.Decimal{}
	totalBase := map[int64]decimal.Decimal{}
	add := func(m map[int64]map[string]decimal.Decimal, id int64, c string, v decimal.Decimal) {
		if m[id] == nil {
			m[id] = map[string]decimal.Decimal{}
		}
		m[id][c] = m[id][c].Add(v)
	}

	out := Balances{
		AsOf:         asOf,
		BaseCurrency: a.Book.BaseCurrency,
		ClassTotals:  map[db.AccountClass]decimal.Decimal{},
		Check:        decimal.Zero,
	}
	for _, row := range sums {
		add(own, row.AccountID, row.Commodity, row.Amount)
		ownBase[row.AccountID] = ownBase[row.AccountID].Add(row.BaseAmount)
		out.ClassTotals[class[row.AccountID]] = out.ClassTotals[class[row.AccountID]].Add(row.BaseAmount)
		out.Check = out.Check.Add(row.BaseAmount)
		for id := &row.AccountID; id != nil; id = parent[*id] {
			add(total, *id, row.Commodity, row.Amount)
			totalBase[*id] = totalBase[*id].Add(row.BaseAmount)
		}
	}

	out.Accounts = make([]AccountBalance, 0, len(accounts))
	for _, acc := range accounts {
		out.Accounts = append(out.Accounts, AccountBalance{
			AccountID:       acc.ID,
			Amounts:         sortedAmounts(own[acc.ID]),
			BaseAmount:      ownBase[acc.ID],
			TotalAmounts:    sortedAmounts(total[acc.ID]),
			TotalBaseAmount: totalBase[acc.ID],
		})
	}
	return out, nil
}
