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

	mu          sync.RWMutex
	commodities map[string]db.Commodity // nil until loaded; reset when one is created
}

func NewService(store *db.Store) *Service {
	return &Service{store: store}
}

// commodityMap is every commodity by code. Currencies only change with a
// migration, but securities and points are created at runtime, so the cache
// is dropped whenever CreateCommodity adds one.
func (s *Service) commodityMap(ctx context.Context) (map[string]db.Commodity, error) {
	s.mu.RLock()
	m := s.commodities
	s.mu.RUnlock()
	if m != nil {
		return m, nil
	}
	rows, err := s.store.ListCommodities(ctx)
	if err != nil {
		return nil, err
	}
	m = make(map[string]db.Commodity, len(rows))
	for _, c := range rows {
		m[c.Code] = c
	}
	s.mu.Lock()
	s.commodities = m
	s.mu.Unlock()
	return m, nil
}

func (s *Service) invalidateCommodities() {
	s.mu.Lock()
	s.commodities = nil
	s.mu.Unlock()
}

// commodityDecimals is the minor-unit count per commodity code.
func (s *Service) commodityDecimals(ctx context.Context) (map[string]int32, error) {
	m, err := s.commodityMap(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int32, len(m))
	for code, c := range m {
		out[code] = int32(c.Decimals)
	}
	return out, nil
}

func (s *Service) Currencies(ctx context.Context) ([]db.Commodity, error) {
	return s.store.ListCurrencies(ctx)
}

func (s *Service) Commodities(ctx context.Context) ([]db.Commodity, error) {
	return s.store.ListCommodities(ctx)
}
