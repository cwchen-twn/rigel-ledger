package ledger

import (
	"context"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

// TagSummary is one tag (a trip, a person, a project) and what it spent.
type TagSummary struct {
	Name         string
	Transactions int64
	First, Last  time.Time
	Expenses     decimal.Decimal // report currency
}

type TagReport struct {
	BaseCurrency string
	Currency     string
	Tags         []TagSummary
	RatesUsed    []RateUsed
	Missing      []string
}

// TagDetail is one tag's spending by expense account.
type TagDetail struct {
	Name         string
	BaseCurrency string
	Currency     string
	Lines        []ReportLine
	Expenses     decimal.Decimal
	RatesUsed    []RateUsed
	Missing      []string
}

// Tags answers "what did each trip cost?": the expense postings of every
// tagged transaction, whatever paid for them -- cash, card, bank or miles (at
// their cost). Stored base amounts are historical, so a trip's cost does not
// move with later rates; it is translated to the report currency at today's
// rate.
func (s *Service) Tags(ctx context.Context, a Access, currency string, on time.Time) (TagReport, error) {
	rc, err := s.newReportCtx(ctx, a, currency, on)
	if err != nil {
		return TagReport{}, err
	}
	rows, err := s.store.TagSummaries(ctx, a.Book.ID)
	if err != nil {
		return TagReport{}, err
	}
	out := TagReport{BaseCurrency: a.Book.BaseCurrency, Currency: rc.currency, Tags: make([]TagSummary, len(rows))}
	for i, r := range rows {
		out.Tags[i] = TagSummary{Name: r.Name, Transactions: r.Transactions, First: r.FirstDate, Last: r.LastDate,
			Expenses: rc.out(r.Expenses)}
	}
	out.RatesUsed, out.Missing = rc.used(), rc.missingList()
	return out, nil
}

func (s *Service) Tag(ctx context.Context, a Access, name, currency string, on time.Time) (TagDetail, error) {
	name = strings.TrimSpace(name)
	rc, err := s.newReportCtx(ctx, a, currency, on)
	if err != nil {
		return TagDetail{}, err
	}
	accs, err := s.accountIndex(ctx, a.Book.ID)
	if err != nil {
		return TagDetail{}, err
	}
	rows, err := s.store.TagExpenses(ctx, db.TagExpensesParams{BookID: a.Book.ID, Name: name})
	if err != nil {
		return TagDetail{}, err
	}
	out := TagDetail{Name: name, BaseCurrency: a.Book.BaseCurrency, Currency: rc.currency}
	lines := map[int64]*ReportLine{}
	for _, r := range rows {
		l := &ReportLine{AccountID: r.AccountID, Amount: rc.out(r.BaseAmount), Historical: r.BaseAmount}
		lines[r.AccountID] = l
		out.Expenses = out.Expenses.Add(l.Amount)
	}
	out.Lines = rollUp(lines, accs)
	out.RatesUsed, out.Missing = rc.used(), rc.missingList()
	return out, nil
}
