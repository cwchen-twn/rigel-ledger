package routes

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
)

// JSON conventions for the API:
//   - money and rates are decimal strings ("1234.50"), never JSON numbers;
//     shopspring/decimal marshals that way by default and parses either form
//     without going through float64
//   - calendar dates are "YYYY-MM-DD"; instants are RFC 3339
//   - ids are JSON numbers (int64 ids stay far below 2^53)

// Date is a calendar date without a time zone.
type Date struct{ time.Time }

func (d Date) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Format(time.DateOnly))
}

func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("date must be a string: %w", err)
	}
	if s == "" {
		d.Time = time.Time{}
		return nil
	}
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return fmt.Errorf("date must be YYYY-MM-DD: %w", err)
	}
	d.Time = t
	return nil
}

func datePtr(t *time.Time) *Date {
	if t == nil {
		return nil
	}
	return &Date{*t}
}

func (d *Date) timePtr() *time.Time {
	if d == nil || d.IsZero() {
		return nil
	}
	t := d.Time
	return &t
}

type UserDTO struct {
	ID              int64  `json:"id"`
	Username        string `json:"username"`
	Email           string `json:"email"`
	DisplayName     string `json:"display_name"`
	IsAdmin         bool   `json:"is_admin"`
	Language        string `json:"language"`
	DisplayCurrency string `json:"display_currency"`
	Timezone        string `json:"timezone"`
	DateFormat      string `json:"date_format"`
	Theme           string `json:"theme"`
	DefaultBookID   *int64 `json:"default_book_id"`
}

func userDTO(u db.User) UserDTO {
	return UserDTO{
		ID: u.ID, Username: u.Username, Email: u.Email, DisplayName: u.DisplayName, IsAdmin: u.IsAdmin,
		Language: u.Language, DisplayCurrency: u.DisplayCurrency, Timezone: u.Timezone,
		DateFormat: u.DateFormat, Theme: u.Theme, DefaultBookID: u.DefaultBookID,
	}
}

type CurrencyDTO struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Decimals int16  `json:"decimals"`
}

// CommodityDTO is anything an account can hold: a currency, a security or points.
type CommodityDTO struct {
	Code          string           `json:"code"`
	Kind          db.CommodityKind `json:"kind"`
	Name          string           `json:"name"`
	Decimals      int16            `json:"decimals"`
	QuoteCurrency *string          `json:"quote_currency"`
	ExchangeMIC   *string          `json:"exchange_mic"`
	ContractSize  *decimal.Decimal `json:"contract_size" swaggertype:"string"`
}

func commodityDTO(c db.Commodity) CommodityDTO {
	var cs *decimal.Decimal
	if c.ContractSize.Valid {
		v := c.ContractSize.Decimal
		cs = &v
	}
	return CommodityDTO{
		Code: c.Code, Kind: c.Kind, Name: c.Name, Decimals: c.Decimals,
		QuoteCurrency: c.QuoteCurrency, ExchangeMIC: c.ExchangeMic, ContractSize: cs,
	}
}

type CostBasisDTO struct {
	Quantity decimal.Decimal `json:"quantity" swaggertype:"string"`
	Cost     decimal.Decimal `json:"cost" swaggertype:"string"`
	UnitCost decimal.Decimal `json:"unit_cost" swaggertype:"string"`
}

type BookDTO struct {
	ID                      int64         `json:"id"`
	Name                    string        `json:"name"`
	BaseCurrency            string        `json:"base_currency"`
	LockDate                *Date         `json:"lock_date" swaggertype:"string" format:"date"`
	InterestDividendCfClass db.CfClass    `json:"interest_dividend_cf_class"`
	Role                    db.MemberRole `json:"role"`
}

func bookDTO(b db.Book, role db.MemberRole) BookDTO {
	return BookDTO{
		ID: b.ID, Name: b.Name, BaseCurrency: b.BaseCurrency, LockDate: datePtr(b.LockDate),
		InterestDividendCfClass: b.InterestDividendCfClass, Role: role,
	}
}

type MemberDTO struct {
	UserID      int64         `json:"user_id"`
	Username    string        `json:"username"`
	DisplayName string        `json:"display_name"`
	Role        db.MemberRole `json:"role"`
}

type AccountDTO struct {
	ID            int64           `json:"id"`
	ParentID      *int64          `json:"parent_id"`
	Class         db.AccountClass `json:"class"`
	Name          *string         `json:"name"`
	TemplateKey   *string         `json:"template_key"`
	Code          *string         `json:"code"`
	Commodity     *string         `json:"commodity"`
	IsCurrent     bool            `json:"is_current"`
	IsCash        bool            `json:"is_cash"`
	CfClass       db.CfClass      `json:"cf_class"`
	IsPlaceholder bool            `json:"is_placeholder"`
	Archived      bool            `json:"archived"`
}

func accountDTO(a db.Account) AccountDTO {
	return AccountDTO{
		ID: a.ID, ParentID: a.ParentID, Class: a.Class, Name: a.Name, TemplateKey: a.TemplateKey,
		Code: a.Code, Commodity: a.Commodity, IsCurrent: a.IsCurrent, IsCash: a.IsCash,
		CfClass: a.CfClass, IsPlaceholder: a.IsPlaceholder, Archived: a.ArchivedAt != nil,
	}
}

type PostingDTO struct {
	ID         int64            `json:"id"`
	AccountID  int64            `json:"account_id"`
	Commodity  string           `json:"commodity"`
	Amount     decimal.Decimal  `json:"amount" swaggertype:"string"`
	BaseAmount decimal.Decimal  `json:"base_amount" swaggertype:"string"`
	UnitCost   *decimal.Decimal `json:"unit_cost" swaggertype:"string"`
	Status     db.PostingStatus `json:"status"`
	ClearedOn  *Date            `json:"cleared_on" swaggertype:"string" format:"date"`
	Memo       string           `json:"memo"`
}

type TransactionDTO struct {
	ID        int64        `json:"id"`
	Date      Date         `json:"date" swaggertype:"string" format:"date"`
	Payee     string       `json:"payee"`
	Memo      string       `json:"memo"`
	Source    string       `json:"source"`
	Postings  []PostingDTO `json:"postings"`
	Tags      []string     `json:"tags"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

func transactionDTO(v ledger.TransactionView) TransactionDTO {
	ps := make([]PostingDTO, len(v.Postings))
	for i, p := range v.Postings {
		ps[i] = PostingDTO{
			ID: p.ID, AccountID: p.AccountID, Commodity: p.Commodity, Amount: p.Amount,
			BaseAmount: p.BaseAmount, Status: p.Status, ClearedOn: datePtr(p.ClearedOn), Memo: p.Memo,
		}
		if p.UnitCost.Valid {
			v := p.UnitCost.Decimal
			ps[i].UnitCost = &v
		}
	}
	return TransactionDTO{
		ID: v.ID, Date: Date{v.Date}, Payee: v.Payee, Memo: v.Memo, Source: v.Source,
		Postings: ps, Tags: v.Tags, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
}

type LineInputDTO struct {
	AccountID  int64            `json:"account_id"`
	Commodity  string           `json:"commodity,omitempty"`
	Amount     decimal.Decimal  `json:"amount" swaggertype:"string"`
	BaseAmount *decimal.Decimal `json:"base_amount,omitempty" swaggertype:"string"`
	UnitCost   *decimal.Decimal `json:"unit_cost,omitempty" swaggertype:"string"`
	Status     db.PostingStatus `json:"status,omitempty"`
	ClearedOn  *Date            `json:"cleared_on,omitempty" swaggertype:"string" format:"date"`
	Memo       string           `json:"memo,omitempty"`
}

type TransactionInputDTO struct {
	Date  Date           `json:"date" swaggertype:"string" format:"date"`
	Payee string         `json:"payee"`
	Memo  string         `json:"memo"`
	Tags  []string       `json:"tags"`
	Lines []LineInputDTO `json:"lines"`
}

func (in TransactionInputDTO) toLedger() ledger.TransactionInput {
	lines := make([]ledger.LineInput, len(in.Lines))
	for i, l := range in.Lines {
		lines[i] = ledger.LineInput{
			AccountID: l.AccountID, Commodity: l.Commodity, Amount: l.Amount, BaseAmount: l.BaseAmount,
			UnitCost: l.UnitCost, Status: l.Status, ClearedOn: l.ClearedOn.timePtr(), Memo: l.Memo,
		}
	}
	return ledger.TransactionInput{Date: in.Date.Time, Payee: in.Payee, Memo: in.Memo, Tags: in.Tags, Lines: lines}
}

type TransactionPageDTO struct {
	Transactions []TransactionDTO `json:"transactions"`
	NextCursor   string           `json:"next_cursor"`
}

type CommodityAmountDTO struct {
	Commodity string          `json:"commodity"`
	Amount    decimal.Decimal `json:"amount" swaggertype:"string"`
}

func amountsDTO(in []ledger.CommodityAmount) []CommodityAmountDTO {
	out := make([]CommodityAmountDTO, len(in))
	for i, a := range in {
		out[i] = CommodityAmountDTO{Commodity: a.Commodity, Amount: a.Amount}
	}
	return out
}

type AccountBalanceDTO struct {
	AccountID       int64                `json:"account_id"`
	Amounts         []CommodityAmountDTO `json:"amounts"`
	BaseAmount      decimal.Decimal      `json:"base_amount" swaggertype:"string"`
	TotalAmounts    []CommodityAmountDTO `json:"total_amounts"`
	TotalBaseAmount decimal.Decimal      `json:"total_base_amount" swaggertype:"string"`
}

type BalancesDTO struct {
	AsOf         Date                       `json:"as_of" swaggertype:"string" format:"date"`
	BaseCurrency string                     `json:"base_currency"`
	Check        decimal.Decimal            `json:"check" swaggertype:"string"`
	ClassTotals  map[string]decimal.Decimal `json:"class_totals" swaggertype:"object,string"`
	Accounts     []AccountBalanceDTO        `json:"accounts"`
}

func balancesDTO(b ledger.Balances) BalancesDTO {
	totals := map[string]decimal.Decimal{}
	for _, c := range []db.AccountClass{db.AccountClassAsset, db.AccountClassLiability, db.AccountClassEquity, db.AccountClassIncome, db.AccountClassExpense} {
		totals[string(c)] = b.ClassTotals[c]
	}
	accts := make([]AccountBalanceDTO, len(b.Accounts))
	for i, a := range b.Accounts {
		accts[i] = AccountBalanceDTO{
			AccountID: a.AccountID, Amounts: amountsDTO(a.Amounts), BaseAmount: a.BaseAmount,
			TotalAmounts: amountsDTO(a.TotalAmounts), TotalBaseAmount: a.TotalBaseAmount,
		}
	}
	return BalancesDTO{AsOf: Date{b.AsOf}, BaseCurrency: b.BaseCurrency, Check: b.Check, ClassTotals: totals, Accounts: accts}
}

type PriceDTO struct {
	ID        int64           `json:"id"`
	Commodity string          `json:"commodity"`
	Quote     string          `json:"quote"`
	Date      Date            `json:"date" swaggertype:"string" format:"date"`
	Rate      decimal.Decimal `json:"rate" swaggertype:"string"`
	Source    string          `json:"source"`
}

func priceDTO(p db.Price) PriceDTO {
	return PriceDTO{ID: p.ID, Commodity: p.Commodity, Quote: p.Quote, Date: Date{p.Date}, Rate: p.Rate, Source: p.Source}
}

type RateDTO struct {
	From string           `json:"from"`
	To   string           `json:"to"`
	Date Date             `json:"date" swaggertype:"string" format:"date"`
	Rate *decimal.Decimal `json:"rate" swaggertype:"string"`
}
