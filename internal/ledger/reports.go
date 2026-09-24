package ledger

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

// The three statements, computed per request from postings and prices.
// Nothing is stored, so a new display currency or a corrected rate shows on
// the next render (see ARCHITECTURE.md, "IFRS, applied where it fits").
//
// Presented amounts are positive where a reader expects them to be: assets,
// liabilities, equity, income and expenses all read positive; cash inflows
// are positive and outflows negative.

// RateUsed is one rate a report applied, with the date of the price.
type RateUsed struct {
	From, To string
	Rate     decimal.Decimal
	Date     time.Time
	Path     string
}

// reportCtx carries what every statement needs: the book, the commodities,
// the rates looked up so far and the conversion to the report currency.
type reportCtx struct {
	s        *Service
	ctx      context.Context
	book     db.Book
	currency string
	// base -> report currency, at the report date
	toReport  decimal.Decimal
	baseDec   int32
	reportDec int32
	commods   map[string]db.Commodity
	rates     map[string]Rate
	missing   map[string]bool
}

func (s *Service) newReportCtx(ctx context.Context, a Access, currency string, on time.Time) (*reportCtx, error) {
	commods, err := s.commodityMap(ctx)
	if err != nil {
		return nil, err
	}
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		currency = a.Book.BaseCurrency
	}
	if c, ok := commods[currency]; !ok || c.Kind != db.CommodityKindCurrency {
		return nil, fieldError("currency", "unknown", "unknown currency %q", currency)
	}
	rc := &reportCtx{s: s, ctx: ctx, book: a.Book, currency: currency, commods: commods,
		rates: map[string]Rate{}, missing: map[string]bool{},
		baseDec: int32(commods[a.Book.BaseCurrency].Decimals), reportDec: int32(commods[currency].Decimals)}
	r, err := rc.rate(a.Book.BaseCurrency, currency, on)
	if err != nil {
		return nil, err
	}
	if !r.OK {
		// No way to translate: stay in the base currency, and say so.
		rc.currency, rc.reportDec = a.Book.BaseCurrency, rc.baseDec
		rc.toReport = decimal.NewFromInt(1)
		rc.missing[currency] = true
	} else {
		rc.toReport = r.Value
	}
	return rc, nil
}

func (rc *reportCtx) rate(from, to string, on time.Time) (Rate, error) {
	key := from + ">" + to + "@" + on.Format("2006-01-02")
	if r, ok := rc.rates[key]; ok {
		return r, nil
	}
	r, err := RateDetail(rc.ctx, rc.s.store.Queries, from, to, on)
	if err != nil {
		return Rate{}, err
	}
	rc.rates[key] = r
	return r, nil
}

// out converts a base amount to the report currency, rounded.
func (rc *reportCtx) out(base decimal.Decimal) decimal.Decimal {
	return base.Mul(rc.toReport).Round(rc.reportDec)
}

func (rc *reportCtx) used() []RateUsed {
	out := []RateUsed{}
	for _, r := range rc.rates {
		if r.OK && r.From != r.To {
			out = append(out, RateUsed{From: r.From, To: r.To, Rate: r.Value, Date: r.Date, Path: r.Path})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		return out[i].To < out[j].To
	})
	return out
}

func (rc *reportCtx) missingList() []string {
	out := []string{}
	for c := range rc.missing {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// value is what `amount` units of commodity c, which cost `hist` in the base
// currency, are worth in the base currency on date `on`:
//   - currencies at the latest rate on or before it (IAS 21 closing rate);
//   - listed securities at price x rate (IFRS 9 FVTPL);
//   - points, futures (contract value is exposure, not an asset) and anything
//     without a rate at cost.
func (rc *reportCtx) value(c string, amount, hist decimal.Decimal, on time.Time) (decimal.Decimal, error) {
	base := rc.book.BaseCurrency
	if c == base {
		return amount, nil
	}
	com, ok := rc.commods[c]
	if !ok {
		return hist, nil
	}
	switch com.Kind {
	case db.CommodityKindCurrency:
		r, err := rc.rate(c, base, on)
		if err != nil || !r.OK {
			rc.missing[c] = !r.OK
			return hist, err
		}
		return amount.Mul(r.Value).Round(rc.baseDec), nil
	case db.CommodityKindSecurity:
		if com.ContractSize.Valid || com.QuoteCurrency == nil {
			return hist, nil
		}
		price, err := rc.rate(c, *com.QuoteCurrency, on)
		if err != nil || !price.OK {
			rc.missing[c] = !price.OK
			return hist, err
		}
		fx, err := rc.rate(*com.QuoteCurrency, base, on)
		if err != nil || !fx.OK {
			rc.missing[*com.QuoteCurrency] = !fx.OK
			return hist, err
		}
		return amount.Mul(price.Value).Mul(fx.Value).Round(rc.baseDec), nil
	}
	return hist, nil // points: carried at cost
}

func (s *Service) accountIndex(ctx context.Context, bookID int64) (map[int64]db.Account, error) {
	accs, err := s.store.ListAccounts(ctx, bookID)
	if err != nil {
		return nil, err
	}
	m := make(map[int64]db.Account, len(accs))
	for _, a := range accs {
		m[a.ID] = a
	}
	return m, nil
}

// ReportLine is one account in a statement. Amount is its own postings; Total
// adds its descendants (for the tree). Both in the report currency, presented
// positive for the account's normal side.
type ReportLine struct {
	AccountID int64
	Amount    decimal.Decimal
	Total     decimal.Decimal
	// Balance sheet only: cost in the base currency before revaluation, and
	// what the account holds.
	Historical decimal.Decimal
	Holdings   []CommodityAmount
	Revalued   bool
}

// rollUp fills Total from Amount along parent links, keeping only accounts
// with something in them or under them.
func rollUp(lines map[int64]*ReportLine, accs map[int64]db.Account) []ReportLine {
	for id, l := range lines {
		for p := accs[id].ParentID; p != nil; p = accs[*p].ParentID {
			pl, ok := lines[*p]
			if !ok {
				pl = &ReportLine{AccountID: *p}
				lines[*p] = pl
			}
			pl.Total = pl.Total.Add(l.Amount)
		}
		l.Total = l.Total.Add(l.Amount)
	}
	out := make([]ReportLine, 0, len(lines))
	for _, l := range lines {
		if !l.Total.IsZero() || !l.Amount.IsZero() || len(l.Holdings) > 0 {
			out = append(out, *l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AccountID < out[j].AccountID })
	return out
}

// presented flips credit-normal classes so they read positive.
func presented(class db.AccountClass, debit decimal.Decimal) decimal.Decimal {
	if class == db.AccountClassAsset || class == db.AccountClassExpense {
		return debit
	}
	return debit.Neg()
}

// --- balance sheet ---

type BalanceSheet struct {
	AsOf         time.Time
	BaseCurrency string
	Currency     string
	Lines        []ReportLine // asset, liability and equity accounts

	CurrentAssets, NonCurrentAssets           decimal.Decimal
	CurrentLiabilities, NonCurrentLiabilities decimal.Decimal
	TotalAssets, TotalLiabilities             decimal.Decimal
	// Equity: the equity accounts (opening balances, contributions), the
	// result accumulated in income and expense to date, and what revaluing
	// foreign balances and securities at the date's rates adds (IAS 21,
	// IFRS 9) -- computed, never posted.
	EquityAccounts    decimal.Decimal
	AccumulatedResult decimal.Decimal
	Unrealised        decimal.Decimal
	TotalEquity       decimal.Decimal

	RatesUsed []RateUsed
	Missing   []string // commodities with no rate: shown at cost
}

// unrealisedAt is the revaluation of asset and liability balances on a date,
// in the base currency: sum(value - historical).
func (rc *reportCtx) balances(on time.Time) (map[int64]*ReportLine, decimal.Decimal, map[int64]db.Account, error) {
	accs, err := rc.s.accountIndex(rc.ctx, rc.book.ID)
	if err != nil {
		return nil, decimal.Zero, nil, err
	}
	sums, err := rc.s.store.AccountSums(rc.ctx, db.AccountSumsParams{BookID: rc.book.ID, AsOf: on})
	if err != nil {
		return nil, decimal.Zero, nil, err
	}
	lines := map[int64]*ReportLine{}
	reval := decimal.Zero
	for _, row := range sums {
		a, ok := accs[row.AccountID]
		if !ok {
			continue
		}
		l := lines[row.AccountID]
		if l == nil {
			l = &ReportLine{AccountID: row.AccountID}
			lines[row.AccountID] = l
		}
		v := row.BaseAmount
		if a.Class == db.AccountClassAsset || a.Class == db.AccountClassLiability {
			if v, err = rc.value(row.Commodity, row.Amount, row.BaseAmount, on); err != nil {
				return nil, decimal.Zero, nil, err
			}
			if !row.Amount.IsZero() {
				l.Holdings = append(l.Holdings, CommodityAmount{Commodity: row.Commodity, Amount: row.Amount})
			}
			if !v.Equal(row.BaseAmount) {
				l.Revalued = true
			}
			reval = reval.Add(v.Sub(row.BaseAmount))
		}
		l.Historical = l.Historical.Add(row.BaseAmount)
		l.Amount = l.Amount.Add(v) // debit-positive, base currency, for now
	}
	return lines, reval, accs, nil
}

func (s *Service) BalanceSheet(ctx context.Context, a Access, asOf time.Time, currency string) (BalanceSheet, error) {
	rc, err := s.newReportCtx(ctx, a, currency, asOf)
	if err != nil {
		return BalanceSheet{}, err
	}
	lines, _, accs, err := rc.balances(asOf)
	if err != nil {
		return BalanceSheet{}, err
	}
	bs := BalanceSheet{AsOf: asOf, BaseCurrency: a.Book.BaseCurrency, Currency: rc.currency}
	resultBase := decimal.Zero
	kept := map[int64]*ReportLine{}
	for id, l := range lines {
		acc := accs[id]
		switch acc.Class {
		case db.AccountClassIncome, db.AccountClassExpense:
			resultBase = resultBase.Add(l.Historical)
			continue
		}
		l.Amount = rc.out(presented(acc.Class, l.Amount))
		switch acc.Class {
		case db.AccountClassAsset:
			if acc.IsCurrent {
				bs.CurrentAssets = bs.CurrentAssets.Add(l.Amount)
			} else {
				bs.NonCurrentAssets = bs.NonCurrentAssets.Add(l.Amount)
			}
		case db.AccountClassLiability:
			if acc.IsCurrent {
				bs.CurrentLiabilities = bs.CurrentLiabilities.Add(l.Amount)
			} else {
				bs.NonCurrentLiabilities = bs.NonCurrentLiabilities.Add(l.Amount)
			}
		case db.AccountClassEquity:
			bs.EquityAccounts = bs.EquityAccounts.Add(l.Amount)
		}
		kept[id] = l
	}
	bs.Lines = rollUp(kept, accs)
	bs.TotalAssets = bs.CurrentAssets.Add(bs.NonCurrentAssets)
	bs.TotalLiabilities = bs.CurrentLiabilities.Add(bs.NonCurrentLiabilities)
	bs.AccumulatedResult = rc.out(resultBase.Neg())
	// Whatever keeps the statement balanced is the revaluation: historical
	// base amounts sum to zero, so in the base currency this is exactly
	// sum(value - cost); translated, it also absorbs the per-line rounding.
	bs.TotalEquity = bs.TotalAssets.Sub(bs.TotalLiabilities)
	bs.Unrealised = bs.TotalEquity.Sub(bs.EquityAccounts).Sub(bs.AccumulatedResult)
	bs.RatesUsed, bs.Missing = rc.used(), rc.missingList()
	return bs, nil
}

// --- income statement ---

type IncomeStatement struct {
	From, To     time.Time
	BaseCurrency string
	Currency     string
	Lines        []ReportLine // income and expense accounts
	Income       decimal.Decimal
	Expenses     decimal.Decimal
	// The change in unrealised FX and valuation gains over the period
	// (balance-sheet revaluation at To minus at the day before From).
	Unrealised decimal.Decimal
	NetResult  decimal.Decimal
	RatesUsed  []RateUsed
	Missing    []string
}

func (s *Service) IncomeStatement(ctx context.Context, a Access, from, to time.Time, currency string) (IncomeStatement, error) {
	if to.Before(from) {
		return IncomeStatement{}, fieldError("to", "invalid", "the period ends before it starts")
	}
	rc, err := s.newReportCtx(ctx, a, currency, to)
	if err != nil {
		return IncomeStatement{}, err
	}
	accs, err := s.accountIndex(ctx, a.Book.ID)
	if err != nil {
		return IncomeStatement{}, err
	}
	sums, err := s.store.AccountSumsBetween(ctx, db.AccountSumsBetweenParams{BookID: a.Book.ID, FromDate: from, ToDate: to})
	if err != nil {
		return IncomeStatement{}, err
	}
	is := IncomeStatement{From: from, To: to, BaseCurrency: a.Book.BaseCurrency, Currency: rc.currency}
	lines := map[int64]*ReportLine{}
	for _, row := range sums {
		acc, ok := accs[row.AccountID]
		if !ok || (acc.Class != db.AccountClassIncome && acc.Class != db.AccountClassExpense) {
			continue
		}
		l := lines[row.AccountID]
		if l == nil {
			l = &ReportLine{AccountID: row.AccountID}
			lines[row.AccountID] = l
		}
		l.Historical = l.Historical.Add(row.BaseAmount)
	}
	for id, l := range lines {
		acc := accs[id]
		l.Amount = rc.out(presented(acc.Class, l.Historical))
		if acc.Class == db.AccountClassIncome {
			is.Income = is.Income.Add(l.Amount)
		} else {
			is.Expenses = is.Expenses.Add(l.Amount)
		}
	}
	_, revalEnd, _, err := rc.balances(to)
	if err != nil {
		return IncomeStatement{}, err
	}
	_, revalStart, _, err := rc.balances(from.AddDate(0, 0, -1))
	if err != nil {
		return IncomeStatement{}, err
	}
	is.Lines = rollUp(lines, accs)
	is.Unrealised = rc.out(revalEnd.Sub(revalStart))
	is.NetResult = is.Income.Sub(is.Expenses).Add(is.Unrealised)
	is.RatesUsed, is.Missing = rc.used(), rc.missingList()
	return is, nil
}

// --- cash flow statement (direct method, IAS 7) ---

type CashFlowLine struct {
	Class     db.CfClass
	AccountID int64
	Amount    decimal.Decimal // inflow > 0
}

type CashFlowStatement struct {
	From, To     time.Time
	BaseCurrency string
	Currency     string
	// Cash and equivalents the day before From at that day's rates, plus any
	// opening-balance entries dated inside the period.
	Opening   decimal.Decimal
	Lines     []CashFlowLine
	Operating decimal.Decimal
	Investing decimal.Decimal
	Financing decimal.Decimal
	// Closing - opening - flows: foreign cash revalued over the period.
	FXEffect  decimal.Decimal
	Closing   decimal.Decimal
	RatesUsed []RateUsed
	Missing   []string
}

// cfClass is where a counter-leg's cash goes. Interest and dividends
// received follow the book's IAS 7 choice.
func cfClass(acc db.Account, book db.Book) db.CfClass {
	if acc.TemplateKey != nil && (*acc.TemplateKey == "interest_income" || *acc.TemplateKey == "dividends") {
		return book.InterestDividendCfClass
	}
	return acc.CfClass
}

func (rc *reportCtx) cashAt(on time.Time, accs map[int64]db.Account) (decimal.Decimal, error) {
	sums, err := rc.s.store.AccountSums(rc.ctx, db.AccountSumsParams{BookID: rc.book.ID, AsOf: on})
	if err != nil {
		return decimal.Zero, err
	}
	total := decimal.Zero
	for _, row := range sums {
		if a, ok := accs[row.AccountID]; ok && a.IsCash {
			v, err := rc.value(row.Commodity, row.Amount, row.BaseAmount, on)
			if err != nil {
				return decimal.Zero, err
			}
			total = total.Add(v)
		}
	}
	return total, nil
}

func (s *Service) CashFlow(ctx context.Context, a Access, from, to time.Time, currency string) (CashFlowStatement, error) {
	if to.Before(from) {
		return CashFlowStatement{}, fieldError("to", "invalid", "the period ends before it starts")
	}
	rc, err := s.newReportCtx(ctx, a, currency, to)
	if err != nil {
		return CashFlowStatement{}, err
	}
	accs, err := s.accountIndex(ctx, a.Book.ID)
	if err != nil {
		return CashFlowStatement{}, err
	}
	rows, err := s.store.CashFlowPostings(ctx, db.CashFlowPostingsParams{BookID: a.Book.ID, FromDate: from, ToDate: to})
	if err != nil {
		return CashFlowStatement{}, err
	}
	// A transaction's cash movement is attributed to its other legs: each
	// counter-leg contributes minus its own base amount, which is pro rata by
	// construction (they sum to the cash movement). A pure transfer between
	// cash accounts has no counter-leg and moves nothing.
	flows := map[int64]decimal.Decimal{}
	openingEntries := decimal.Zero
	for _, r := range rows {
		if r.IsCash {
			continue
		}
		// Cash that arrives through the opening-balances account is not a
		// flow: it is money the household already had when it started keeping
		// books, so it belongs to the opening position.
		if a := accs[r.AccountID]; a.TemplateKey != nil && *a.TemplateKey == KeyOpeningBalances {
			openingEntries = openingEntries.Sub(r.BaseAmount)
			continue
		}
		flows[r.AccountID] = flows[r.AccountID].Sub(r.BaseAmount)
	}
	cf := CashFlowStatement{From: from, To: to, BaseCurrency: a.Book.BaseCurrency, Currency: rc.currency}
	netBase := decimal.Zero
	for id, v := range flows {
		if v.IsZero() {
			continue
		}
		netBase = netBase.Add(v)
		line := CashFlowLine{Class: cfClass(accs[id], a.Book), AccountID: id, Amount: rc.out(v)}
		switch line.Class {
		case db.CfClassInvesting:
			cf.Investing = cf.Investing.Add(line.Amount)
		case db.CfClassFinancing:
			cf.Financing = cf.Financing.Add(line.Amount)
		default:
			cf.Operating = cf.Operating.Add(line.Amount)
		}
		cf.Lines = append(cf.Lines, line)
	}
	sort.Slice(cf.Lines, func(i, j int) bool {
		if cf.Lines[i].Class != cf.Lines[j].Class {
			return cf.Lines[i].Class < cf.Lines[j].Class
		}
		return cf.Lines[i].AccountID < cf.Lines[j].AccountID
	})
	opening, err := rc.cashAt(from.AddDate(0, 0, -1), accs)
	if err != nil {
		return CashFlowStatement{}, err
	}
	closing, err := rc.cashAt(to, accs)
	if err != nil {
		return CashFlowStatement{}, err
	}
	cf.Opening, cf.Closing = rc.out(opening.Add(openingEntries)), rc.out(closing)
	cf.FXEffect = cf.Closing.Sub(cf.Opening).Sub(cf.Operating).Sub(cf.Investing).Sub(cf.Financing)
	cf.RatesUsed, cf.Missing = rc.used(), rc.missingList()
	return cf, nil
}
