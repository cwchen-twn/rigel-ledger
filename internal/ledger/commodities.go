package ledger

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

// Mirrors the CHECK on commodities.code for non-currency kinds.
var commodityCode = regexp.MustCompile(`^[A-Z0-9]+:[A-Z0-9._-]+$`)

type CommodityInput struct {
	Code          string
	Kind          db.CommodityKind
	Name          string
	Decimals      *int16
	QuoteCurrency *string
	ExchangeMIC   *string
	ContractSize  *decimal.Decimal
}

// CreateCommodity adds a security (priced in a quote currency) or a points
// programme (miles, card points: no price, carried at cost). Currencies come
// only from the ISO seed. Commodities are global, like rates, so any editor of
// any book may add one.
func (s *Service) CreateCommodity(ctx context.Context, a Access, in CommodityInput) (db.Commodity, error) {
	if err := a.require(db.MemberRoleEditor); err != nil {
		return db.Commodity{}, err
	}
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	in.Name = strings.TrimSpace(in.Name)
	in.QuoteCurrency = cleanOptional(in.QuoteCurrency)
	in.ExchangeMIC = cleanOptional(in.ExchangeMIC)

	switch in.Kind {
	case db.CommodityKindSecurity, db.CommodityKindPoints:
	default:
		return db.Commodity{}, fieldError("kind", "invalid", "kind must be security or points")
	}
	if !commodityCode.MatchString(in.Code) {
		return db.Commodity{}, fieldError("code", "invalid", `use NAMESPACE:SYMBOL, e.g. "XNAS:AAPL" or "MILES:EVA"`)
	}
	if in.Name == "" {
		return db.Commodity{}, fieldError("name", "required", "a commodity needs a name")
	}
	decimals := int16(0)
	if in.Kind == db.CommodityKindSecurity {
		decimals = 4 // fractional shares
	}
	if in.Decimals != nil {
		decimals = *in.Decimals
	}
	if decimals < 0 || decimals > 8 {
		return db.Commodity{}, fieldError("decimals", "invalid", "between 0 and 8")
	}

	if in.Kind == db.CommodityKindSecurity {
		if in.QuoteCurrency == nil {
			return db.Commodity{}, fieldError("quote_currency", "required", "a security is priced in a currency")
		}
		q := strings.ToUpper(*in.QuoteCurrency)
		in.QuoteCurrency = &q
		if err := s.validCurrency(ctx, q); err != nil {
			return db.Commodity{}, fieldError("quote_currency", "unknown", "unknown currency %q", q)
		}
		if in.ContractSize != nil && !in.ContractSize.IsPositive() {
			return db.Commodity{}, fieldError("contract_size", "invalid", "must be positive")
		}
	} else {
		if in.QuoteCurrency != nil || in.ContractSize != nil {
			return db.Commodity{}, fieldError("kind", "invalid", "points have no quote currency or contract size")
		}
	}

	var cs decimal.NullDecimal
	if in.ContractSize != nil {
		cs = decimal.NullDecimal{Decimal: *in.ContractSize, Valid: true}
	}
	c, err := s.store.CreateCommodity(ctx, db.CreateCommodityParams{
		Code: in.Code, Kind: in.Kind, Name: in.Name, Decimals: decimals,
		QuoteCurrency: in.QuoteCurrency, ExchangeMic: in.ExchangeMIC, ContractSize: cs,
	})
	if err != nil {
		return db.Commodity{}, translate(err, "commodity")
	}
	s.invalidateCommodities()
	return c, nil
}

// CostBasis is what an account holds and what it cost, as of a date.
type CostBasis struct {
	Quantity decimal.Decimal
	Cost     decimal.Decimal
}

// UnitCost is the average cost of one unit in the base currency, or zero
// when nothing is held.
func (c CostBasis) UnitCost() decimal.Decimal {
	if !c.Quantity.IsPositive() {
		return decimal.Zero
	}
	return c.Cost.DivRound(c.Quantity, rateDivisionPlaces)
}

// AccountCostBasis answers "what are my miles (or shares) worth at cost":
// the average-cost input when they are spent or sold. excludeTxn leaves out
// the transaction being edited.
func (s *Service) AccountCostBasis(ctx context.Context, a Access, accountID int64, asOf time.Time, excludeTxn int64) (CostBasis, error) {
	if _, err := s.store.GetAccount(ctx, db.GetAccountParams{BookID: a.Book.ID, ID: accountID}); err != nil {
		return CostBasis{}, translate(err, "account")
	}
	return costBasis(ctx, s.store.Queries, accountID, asOf, excludeTxn)
}

func costBasis(ctx context.Context, q *db.Queries, accountID int64, asOf time.Time, excludeTxn int64) (CostBasis, error) {
	row, err := q.AccountCostBasis(ctx, db.AccountCostBasisParams{AccountID: accountID, AsOf: asOf, ExcludeTransactionID: excludeTxn})
	if err != nil {
		return CostBasis{}, err
	}
	return CostBasis{Quantity: row.Quantity, Cost: row.Cost}, nil
}
