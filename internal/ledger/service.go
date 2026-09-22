// Package ledger holds the bookkeeping rules: what a valid transaction is, how
// foreign amounts become base amounts, and how balances roll up. Handlers call
// it; it calls internal/db. Every write goes through db.Store.WithTx so the
// audit trigger knows who made it.
package ledger

import (
	"context"
	"sync"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

type Service struct {
	store *db.Store

	decimalsOnce sync.Once
	decimals     map[string]int32
	decimalsErr  error
}

func NewService(store *db.Store) *Service {
	return &Service{store: store}
}

// commodityDecimals is the minor-unit count per commodity code, loaded once:
// the ISO list only changes with a migration.
func (s *Service) commodityDecimals(ctx context.Context) (map[string]int32, error) {
	s.decimalsOnce.Do(func() {
		rows, err := s.store.ListCurrencies(ctx)
		if err != nil {
			s.decimalsErr = err
			return
		}
		m := make(map[string]int32, len(rows))
		for _, c := range rows {
			m[c.Code] = int32(c.Decimals)
		}
		s.decimals = m
	})
	return s.decimals, s.decimalsErr
}

func (s *Service) Currencies(ctx context.Context) ([]db.Commodity, error) {
	return s.store.ListCurrencies(ctx)
}
