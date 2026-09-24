package routes

import (
	"net/http"
	"time"

	"github.com/shopspring/decimal"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
	"github.com/cwchen-twn/rigel-ledger/internal/response"
)

type ReportLineDTO struct {
	AccountID int64           `json:"account_id"`
	Amount    decimal.Decimal `json:"amount" swaggertype:"string"`
	Total     decimal.Decimal `json:"total" swaggertype:"string"`
	// Balance sheet only.
	Historical *decimal.Decimal     `json:"historical,omitempty" swaggertype:"string"`
	Holdings   []CommodityAmountDTO `json:"holdings,omitempty"`
	Revalued   bool                 `json:"revalued,omitempty"`
}

type RateUsedDTO struct {
	From string          `json:"from"`
	To   string          `json:"to"`
	Rate decimal.Decimal `json:"rate" swaggertype:"string"`
	Date string          `json:"date"`
	Path string          `json:"path"`
}

func linesDTO(ls []ledger.ReportLine, withCost bool) []ReportLineDTO {
	out := make([]ReportLineDTO, len(ls))
	for i, l := range ls {
		out[i] = ReportLineDTO{AccountID: l.AccountID, Amount: l.Amount, Total: l.Total, Revalued: l.Revalued}
		if withCost && len(l.Holdings) > 0 {
			h := l.Historical
			out[i].Historical = &h
			for _, c := range l.Holdings {
				out[i].Holdings = append(out[i].Holdings, CommodityAmountDTO{Commodity: c.Commodity, Amount: c.Amount})
			}
		}
	}
	return out
}

func ratesDTO(rs []ledger.RateUsed) []RateUsedDTO {
	out := make([]RateUsedDTO, len(rs))
	for i, r := range rs {
		out[i] = RateUsedDTO{From: r.From, To: r.To, Rate: r.Rate, Date: r.Date.Format(time.DateOnly), Path: r.Path}
	}
	return out
}

type BalanceSheetDTO struct {
	AsOf                  string          `json:"as_of"`
	BaseCurrency          string          `json:"base_currency"`
	Currency              string          `json:"currency"`
	Lines                 []ReportLineDTO `json:"lines"`
	CurrentAssets         decimal.Decimal `json:"current_assets" swaggertype:"string"`
	NonCurrentAssets      decimal.Decimal `json:"non_current_assets" swaggertype:"string"`
	TotalAssets           decimal.Decimal `json:"total_assets" swaggertype:"string"`
	CurrentLiabilities    decimal.Decimal `json:"current_liabilities" swaggertype:"string"`
	NonCurrentLiabilities decimal.Decimal `json:"non_current_liabilities" swaggertype:"string"`
	TotalLiabilities      decimal.Decimal `json:"total_liabilities" swaggertype:"string"`
	EquityAccounts        decimal.Decimal `json:"equity_accounts" swaggertype:"string"`
	AccumulatedResult     decimal.Decimal `json:"accumulated_result" swaggertype:"string"`
	Unrealised            decimal.Decimal `json:"unrealised" swaggertype:"string"`
	TotalEquity           decimal.Decimal `json:"total_equity" swaggertype:"string"`
	RatesUsed             []RateUsedDTO   `json:"rates_used"`
	Missing               []string        `json:"missing"`
}

type IncomeStatementDTO struct {
	From         string          `json:"from"`
	To           string          `json:"to"`
	BaseCurrency string          `json:"base_currency"`
	Currency     string          `json:"currency"`
	Lines        []ReportLineDTO `json:"lines"`
	Income       decimal.Decimal `json:"income" swaggertype:"string"`
	Expenses     decimal.Decimal `json:"expenses" swaggertype:"string"`
	Unrealised   decimal.Decimal `json:"unrealised" swaggertype:"string"`
	NetResult    decimal.Decimal `json:"net_result" swaggertype:"string"`
	RatesUsed    []RateUsedDTO   `json:"rates_used"`
	Missing      []string        `json:"missing"`
}

type CashFlowLineDTO struct {
	Class     db.CfClass      `json:"class"`
	AccountID int64           `json:"account_id"`
	Amount    decimal.Decimal `json:"amount" swaggertype:"string"`
}

type CashFlowDTO struct {
	From         string            `json:"from"`
	To           string            `json:"to"`
	BaseCurrency string            `json:"base_currency"`
	Currency     string            `json:"currency"`
	Opening      decimal.Decimal   `json:"opening" swaggertype:"string"`
	Lines        []CashFlowLineDTO `json:"lines"`
	Operating    decimal.Decimal   `json:"operating" swaggertype:"string"`
	Investing    decimal.Decimal   `json:"investing" swaggertype:"string"`
	Financing    decimal.Decimal   `json:"financing" swaggertype:"string"`
	FXEffect     decimal.Decimal   `json:"fx_effect" swaggertype:"string"`
	Closing      decimal.Decimal   `json:"closing" swaggertype:"string"`
	RatesUsed    []RateUsedDTO     `json:"rates_used"`
	Missing      []string          `json:"missing"`
}

// reportCurrency is ?currency=, else the user's display currency.
func reportCurrency(r *http.Request) string {
	if c := r.URL.Query().Get("currency"); c != "" {
		return c
	}
	id, _ := auth.FromContext(r.Context())
	return id.User.DisplayCurrency
}

// period reads ?from= and ?to=: by default the year to date.
func period(w http.ResponseWriter, r *http.Request) (time.Time, time.Time, bool) {
	to, ok := queryDate(r, "to")
	if !ok {
		badParam(w, "to", "invalid")
		return time.Time{}, time.Time{}, false
	}
	if to == nil {
		t := todayUTC()
		to = &t
	}
	from, ok := queryDate(r, "from")
	if !ok {
		badParam(w, "from", "invalid")
		return time.Time{}, time.Time{}, false
	}
	if from == nil {
		f := time.Date(to.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		from = &f
	}
	return *from, *to, true
}

// balanceSheet
//
//	@Summary	Balance sheet at the date's rates (IAS 1, IAS 21, IFRS 9 FVTPL)
//	@Tags		reports
//	@Produce	json
//	@Param		bookID		path		int		true	"book id"
//	@Param		as_of		query		string	false	"YYYY-MM-DD, default today"
//	@Param		currency	query		string	false	"report currency, default the user's display currency"
//	@Success	200			{object}	BalanceSheetDTO
//	@Router		/api/books/{bookID}/reports/balance-sheet [get]
func (h *handlers) balanceSheet(w http.ResponseWriter, r *http.Request) {
	asOf, ok := queryDate(r, "as_of")
	if !ok {
		badParam(w, "as_of", "invalid")
		return
	}
	if asOf == nil {
		t := todayUTC()
		asOf = &t
	}
	bs, err := h.svc.BalanceSheet(r.Context(), access(r), *asOf, reportCurrency(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, BalanceSheetDTO{
		AsOf: bs.AsOf.Format(time.DateOnly), BaseCurrency: bs.BaseCurrency, Currency: bs.Currency,
		Lines: linesDTO(bs.Lines, true), CurrentAssets: bs.CurrentAssets, NonCurrentAssets: bs.NonCurrentAssets,
		TotalAssets: bs.TotalAssets, CurrentLiabilities: bs.CurrentLiabilities, NonCurrentLiabilities: bs.NonCurrentLiabilities,
		TotalLiabilities: bs.TotalLiabilities, EquityAccounts: bs.EquityAccounts, AccumulatedResult: bs.AccumulatedResult,
		Unrealised: bs.Unrealised, TotalEquity: bs.TotalEquity, RatesUsed: ratesDTO(bs.RatesUsed), Missing: bs.Missing,
	})
}

// incomeStatement
//
//	@Summary	Income statement for a period, with the change in unrealised gains
//	@Tags		reports
//	@Produce	json
//	@Param		bookID		path		int		true	"book id"
//	@Param		from		query		string	false	"YYYY-MM-DD, default 1 January"
//	@Param		to			query		string	false	"YYYY-MM-DD, default today"
//	@Param		currency	query		string	false	"report currency"
//	@Success	200			{object}	IncomeStatementDTO
//	@Router		/api/books/{bookID}/reports/income-statement [get]
func (h *handlers) incomeStatement(w http.ResponseWriter, r *http.Request) {
	from, to, ok := period(w, r)
	if !ok {
		return
	}
	is, err := h.svc.IncomeStatement(r.Context(), access(r), from, to, reportCurrency(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, IncomeStatementDTO{
		From: is.From.Format(time.DateOnly), To: is.To.Format(time.DateOnly), BaseCurrency: is.BaseCurrency,
		Currency: is.Currency, Lines: linesDTO(is.Lines, false), Income: is.Income, Expenses: is.Expenses,
		Unrealised: is.Unrealised, NetResult: is.NetResult, RatesUsed: ratesDTO(is.RatesUsed), Missing: is.Missing,
	})
}

// cashFlow
//
//	@Summary	Cash flow statement for a period, direct method (IAS 7)
//	@Tags		reports
//	@Produce	json
//	@Param		bookID		path		int		true	"book id"
//	@Param		from		query		string	false	"YYYY-MM-DD, default 1 January"
//	@Param		to			query		string	false	"YYYY-MM-DD, default today"
//	@Param		currency	query		string	false	"report currency"
//	@Success	200			{object}	CashFlowDTO
//	@Router		/api/books/{bookID}/reports/cash-flow [get]
func (h *handlers) cashFlow(w http.ResponseWriter, r *http.Request) {
	from, to, ok := period(w, r)
	if !ok {
		return
	}
	cf, err := h.svc.CashFlow(r.Context(), access(r), from, to, reportCurrency(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	lines := make([]CashFlowLineDTO, len(cf.Lines))
	for i, l := range cf.Lines {
		lines[i] = CashFlowLineDTO{Class: l.Class, AccountID: l.AccountID, Amount: l.Amount}
	}
	response.JSON(w, http.StatusOK, CashFlowDTO{
		From: cf.From.Format(time.DateOnly), To: cf.To.Format(time.DateOnly), BaseCurrency: cf.BaseCurrency,
		Currency: cf.Currency, Opening: cf.Opening, Lines: lines, Operating: cf.Operating, Investing: cf.Investing,
		Financing: cf.Financing, FXEffect: cf.FXEffect, Closing: cf.Closing, RatesUsed: ratesDTO(cf.RatesUsed), Missing: cf.Missing,
	})
}
