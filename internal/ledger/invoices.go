package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

// Invoices (#38, migration 000011): an invoice enriches; it does not
// create. It matches the card, bank or cash transaction that paid it --
// booked, or still waiting in the queue -- and adds its items, which can
// split the expense by the categories rules give them. Only an invoice
// nothing paid for (cash) proposes a transaction, against the account its
// source (a carrier, 載具) is mapped to.

const (
	invoiceBefore = 2 // days a payment may be dated before its invoice
	invoiceAfter  = 5 // and after (a card posts late)
	maxItems      = 500
	// An invoice nothing paid for is a cash purchase only once a card
	// charge has had time to come (#55): a statement lags the 電子發票.
	invoiceWait = 7
)

// InvoiceItem is one line of an invoice. Amount is what the line cost
// (> 0; a discount line < 0). AccountID is the category a rule gives it.
type InvoiceItem struct {
	Description string           `json:"description"`
	Quantity    *decimal.Decimal `json:"quantity,omitempty"`
	UnitPrice   *decimal.Decimal `json:"unit_price,omitempty"`
	Amount      decimal.Decimal  `json:"amount"`
	AccountID   *int64           `json:"account_id,omitempty"`
}

// TransactionItem is a line of the invoice a transaction carries.
type TransactionItem = db.TransactionItem

func checkItems(items []InvoiceItem, places int32, i int) error {
	if len(items) == 0 || len(items) > maxItems {
		return fieldError(idx("rows", i, "items"), "out_of_range", "an invoice has 1 to %d items", maxItems)
	}
	for _, it := range items {
		if it.Amount.IsZero() && it.Description == "" {
			return fieldError(idx("rows", i, "items"), "invalid", "an item needs a description or an amount")
		}
		if !hasPrecision(it.Amount, places) {
			return fieldError(idx("rows", i, "items"), "too_precise", "an item's amount allows %d decimal places", places)
		}
	}
	return nil
}

func parseItems(raw []byte) ([]InvoiceItem, error) {
	var items []InvoiceItem
	err := json.Unmarshal(raw, &items)
	return items, err
}

// ruleFor is the first rule (in priority order) whose pattern the text
// contains, case-insensitive, among those for this source or every source.
func ruleFor(rules []db.ImportRule, sourceID int64, text string) *db.ImportRule {
	text = strings.ToLower(text)
	for i, rule := range rules {
		if rule.SourceAccountID != nil && *rule.SourceAccountID != sourceID {
			continue
		}
		if strings.Contains(text, strings.ToLower(rule.Pattern)) {
			return &rules[i]
		}
	}
	return nil
}

// proposeInvoice gives each item its category from the rules, then finds
// what paid for the invoice: a booked transaction, a waiting row, or
// nothing (a cash purchase).
func (s *Service) proposeInvoice(ctx context.Context, a Access, r db.GetImportRowRow) error {
	items, err := parseItems(r.Items)
	if err != nil {
		return err
	}
	rules, err := s.store.ListRules(ctx, a.Book.ID)
	if err != nil {
		return err
	}
	for i := range items {
		items[i].AccountID = nil
		if rule := ruleFor(rules, r.SourceAccountID, items[i].Description); rule != nil {
			items[i].AccountID = &rule.AccountID
		}
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return err
	}
	if err := s.store.SetRowItems(ctx, db.SetRowItemsParams{BookID: a.Book.ID, ID: r.ID, Items: raw}); err != nil {
		return err
	}

	from, to := r.Date.AddDate(0, 0, -invoiceBefore), r.Date.AddDate(0, 0, invoiceAfter)
	seller := strings.TrimSpace(r.Counterparty)
	set := func(p db.SetRowProposalParams) error {
		p.ID = r.ID
		return s.store.SetRowProposal(ctx, p)
	}
	if tx, err := s.store.FindInvoiceMatch(ctx, db.FindInvoiceMatchParams{BookID: a.Book.ID, Currency: r.Currency, Amount: r.Amount,
		FromDate: from, ToDate: to, RowID: r.ID, Seller: seller, OnDate: r.Date}); err == nil {
		return set(db.SetRowProposalParams{Proposal: "enrich", MatchTransactionID: &tx})
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if row, err := s.store.FindInvoiceRow(ctx, db.FindInvoiceRowParams{BookID: a.Book.ID, Currency: r.Currency, Amount: r.Amount,
		FromDate: from, ToDate: to, RowID: r.ID, Seller: seller, OnDate: r.Date}); err == nil {
		return set(db.SetRowProposalParams{Proposal: "enrich", MatchRowID: &row})
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	// Another source's invoice for the same purchase (an order email and
	// its 電子發票) already added its lines, or waits to (#54).
	if tx, err := s.store.FindSameInvoiceTx(ctx, db.FindSameInvoiceTxParams{BookID: a.Book.ID, Currency: r.Currency, Amount: r.Amount,
		FromDate: from, ToDate: to, Connector: r.Connector, OnDate: r.Date}); err == nil {
		return set(db.SetRowProposalParams{Proposal: "same_invoice", MatchTransactionID: &tx})
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if other, err := s.store.FindSameInvoiceRow(ctx, db.FindSameInvoiceRowParams{BookID: a.Book.ID, RowID: r.ID, Connector: r.Connector,
		Currency: r.Currency, Amount: r.Amount, FromDate: from, ToDate: to, OnDate: r.Date}); err == nil {
		return set(db.SetRowProposalParams{Proposal: "same_invoice", MatchRowID: &other})
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	// Nothing paid for it: a cash purchase, categorised by its seller --
	// but not before a card charge has had time to come.
	p := db.SetRowProposalParams{Proposal: "new"}
	if rule := ruleFor(rules, r.SourceAccountID, r.Counterparty+" "+r.Description); rule != nil {
		p.ProposedAccountID, p.RuleID = &rule.AccountID, &rule.ID
	}
	if waiting(r.Date, now()) {
		p.Proposal = "waiting"
	}
	return set(p)
}

// now is the clock invoices wait by; a test moves it.
var now = time.Now

// waiting says an invoice of this day may still see its payment come.
func waiting(day, at time.Time) bool {
	return at.Sub(day) < invoiceWait*24*time.Hour
}

// matchAgedInvoices proposes again the invoices that waited long enough to
// be cash purchases; the queue calls it before it is read.
func (s *Service) matchAgedInvoices(ctx context.Context, a Access, rows []db.ListQueueRow) (bool, error) {
	aged := false
	for _, r := range rows {
		if r.Kind != "invoice" || r.Proposal != "waiting" || waiting(r.Date, now()) {
			continue
		}
		row, err := s.store.GetImportRow(ctx, db.GetImportRowParams{BookID: a.Book.ID, ID: r.ID})
		if err != nil {
			return aged, err
		}
		if err := s.proposeInvoice(ctx, a, row); err != nil {
			return aged, err
		}
		aged = true
	}
	return aged, nil
}

// rematchInvoices proposes every waiting invoice again: a card row staged
// or accepted since may be what paid for it.
func (s *Service) rematchInvoices(ctx context.Context, a Access) error {
	ids, err := s.store.PendingInvoiceRows(ctx, a.Book.ID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		r, err := s.store.GetImportRow(ctx, db.GetImportRowParams{BookID: a.Book.ID, ID: id})
		if err != nil {
			return err
		}
		if err := s.proposeInvoice(ctx, a, r); err != nil {
			return err
		}
	}
	return nil
}

// acceptInvoice adds an invoice to the transaction that paid it, or books
// it as a cash purchase, splitting the expense by item category if asked.
func (s *Service) acceptInvoice(ctx context.Context, a Access, r db.GetImportRowRow, in AcceptInput) (int64, error) {
	items, err := parseItems(r.Items)
	if err != nil {
		return 0, err
	}
	proposal := r.Proposal
	if in.CategoryID != nil {
		proposal = "new" // the person chose a category: a cash purchase
	}
	switch {
	case proposal == "waiting":
		return 0, fieldError("account_id", "required", "a card charge may still come for this invoice; choose a category to book it as cash now")
	case proposal == "same_invoice" && r.MatchTransactionID == nil:
		if r.MatchRowID != nil {
			return 0, conflict("match_pending", "accept the other invoice for this purchase first")
		}
		if err := s.proposeInvoice(ctx, a, r); err != nil {
			return 0, err
		}
		return 0, conflict("match_gone", "what this invoice matched is gone; it was matched again")
	case proposal == "same_invoice":
		// The purchase has its lines already: this invoice adds its file.
		txnID := *r.MatchTransactionID
		err := s.store.WithTx(ctx, a.UserID, func(q *db.Queries) error {
			if r.AttachmentID != nil {
				if err := q.LinkAttachment(ctx, db.LinkAttachmentParams{BookID: a.Book.ID, TransactionID: txnID, AttachmentID: *r.AttachmentID, CreatedBy: &a.UserID}); err != nil {
					return err
				}
			}
			n, err := q.DecideRow(ctx, db.DecideRowParams{ID: r.ID, Status: "accepted", TransactionID: &txnID, DecidedBy: &a.UserID})
			if err == nil && n == 0 {
				return conflict("already_decided", "this row was already accepted or ignored")
			}
			return err
		})
		if err != nil {
			return 0, translate(err, "import row")
		}
		return txnID, s.rematchInvoices(ctx, a)
	}
	if proposal == "enrich" && r.MatchTransactionID == nil {
		if r.MatchRowID != nil {
			return 0, conflict("match_pending", "accept the card or bank row that paid for this invoice first")
		}
		if err := s.proposeInvoice(ctx, a, r); err != nil {
			return 0, err
		}
		return 0, conflict("match_gone", "what this invoice matched is gone; it was matched again")
	}

	var txnID int64
	err = s.store.WithTx(ctx, a.UserID, func(q *db.Queries) error {
		var split []InvoiceItem
		if in.Split {
			split = items
		}
		if proposal == "enrich" {
			txnID = *r.MatchTransactionID
			cur, err := s.viewTx(ctx, q, a, txnID)
			if err != nil {
				return err
			}
			accts, err := q.ListAccounts(ctx, a.Book.ID)
			if err != nil {
				return err
			}
			classes := make(map[int64]db.AccountClass, len(accts))
			for _, ac := range accts {
				classes[ac.ID] = ac.Class
			}
			if lines, ok := splitLines(cur, classes, r, split); ok {
				in := TransactionInput{Date: cur.Date, Payee: cur.Payee, Memo: cur.Memo, Tags: cur.Tags, Lines: lines}
				if err := s.updateTx(ctx, q, a, txnID, in); err != nil {
					return err
				}
			} else {
				split = nil
			}
		} else {
			if r.AccountID == nil {
				return invalid("unmapped", "map the invoice's source account (where cash purchases come out of) first")
			}
			cat := in.CategoryID
			if cat == nil {
				cat = r.ProposedAccountID
			}
			total := r.Amount.Neg() // what the purchase cost
			byCat, rest := splitGroups(split, cat, total)
			if cat == nil && !rest.IsZero() {
				return fieldError("account_id", "required", "choose a category for this invoice")
			}
			lines := []LineInput{{AccountID: *r.AccountID, Commodity: r.Currency, Amount: r.Amount, Status: db.PostingStatusCleared, ClearedOn: &r.Date}}
			for _, g := range byCat {
				lines = append(lines, LineInput{AccountID: g.account, Commodity: r.Currency, Amount: g.amount})
			}
			if !rest.IsZero() {
				lines = append(lines, LineInput{AccountID: *cat, Commodity: r.Currency, Amount: rest})
			}
			payee := strings.TrimSpace(r.Counterparty)
			if payee == "" {
				payee = r.Description
			}
			ext := r.ExternalID
			var err error
			if txnID, err = s.createTx(ctx, q, a, TransactionInput{Date: r.Date, Payee: payee, Source: "sync", ExternalID: &ext, Lines: lines}); err != nil {
				return err
			}
		}
		if err := writeItems(ctx, q, a, txnID, items, split != nil, r.ExternalID); err != nil {
			return err
		}
		if r.AttachmentID != nil {
			if err := q.LinkAttachment(ctx, db.LinkAttachmentParams{BookID: a.Book.ID, TransactionID: txnID, AttachmentID: *r.AttachmentID, CreatedBy: &a.UserID}); err != nil {
				return err
			}
		}
		n, err := q.DecideRow(ctx, db.DecideRowParams{ID: r.ID, Status: "accepted", TransactionID: &txnID, DecidedBy: &a.UserID})
		if err == nil && n == 0 {
			return conflict("already_decided", "this row was already accepted or ignored")
		}
		return err
	})
	if err != nil {
		return 0, translate(err, "import row")
	}
	// An invoice for the same purchase waiting for this one can now take its file to it.
	return txnID, s.rematchInvoices(ctx, a)
}

type categoryAmount struct {
	account int64
	amount  decimal.Decimal
}

// splitGroups sums the items by the category a rule gave them, leaving out
// those of the main category (and those without one): the rest stays on
// the main category. A group is in the order its first item came.
func splitGroups(items []InvoiceItem, main *int64, total decimal.Decimal) ([]categoryAmount, decimal.Decimal) {
	var groups []categoryAmount
	rest := total
	for _, it := range items {
		if it.AccountID == nil || main != nil && *it.AccountID == *main {
			continue
		}
		rest = rest.Sub(it.Amount)
		found := false
		for i := range groups {
			if groups[i].account == *it.AccountID {
				groups[i].amount = groups[i].amount.Add(it.Amount)
				found = true
			}
		}
		if !found {
			groups = append(groups, categoryAmount{*it.AccountID, it.Amount})
		}
	}
	return groups, rest
}

// splitLines rewrites a paid transaction's one category line into a line
// per item category, the rest staying where it was. It splits only what is
// plain: one category line, in the invoice's currency, of its whole total,
// and no group that would turn the other way.
func splitLines(cur TransactionView, classes map[int64]db.AccountClass, r db.GetImportRowRow, items []InvoiceItem) ([]LineInput, bool) {
	if len(items) == 0 {
		return nil, false
	}
	var cat *db.Posting // the one income or expense line
	for i, p := range cur.Postings {
		if c := classes[p.AccountID]; c != db.AccountClassExpense && c != db.AccountClassIncome {
			continue
		}
		if cat != nil {
			return nil, false
		}
		cat = &cur.Postings[i]
	}
	total := r.Amount.Neg()
	if cat == nil || cat.Commodity != r.Currency || !cat.Amount.Equal(total) {
		return nil, false
	}
	groups, rest := splitGroups(items, &cat.AccountID, total)
	if len(groups) == 0 || rest.Sign() == -total.Sign() {
		return nil, false
	}
	for _, g := range groups {
		if g.amount.Sign() != total.Sign() {
			return nil, false
		}
	}
	var lines []LineInput
	for _, p := range cur.Postings {
		if p.ID != cat.ID {
			lines = append(lines, LineInput{AccountID: p.AccountID, Commodity: p.Commodity, Amount: p.Amount,
				BaseAmount: &p.BaseAmount, Status: p.Status, ClearedOn: p.ClearedOn, Memo: p.Memo})
			continue
		}
		for _, g := range groups {
			lines = append(lines, LineInput{AccountID: g.account, Commodity: p.Commodity, Amount: g.amount, Memo: p.Memo})
		}
		if !rest.IsZero() {
			lines = append(lines, LineInput{AccountID: p.AccountID, Commodity: p.Commodity, Amount: rest, Memo: p.Memo})
		}
	}
	return lines, true
}

// writeItems keeps an invoice's lines on the transaction it was added to.
func writeItems(ctx context.Context, q *db.Queries, a Access, txnID int64, items []InvoiceItem, split bool, source string) error {
	for i, it := range items {
		var acct *int64
		if split {
			acct = it.AccountID
		}
		if err := q.InsertTransactionItem(ctx, db.InsertTransactionItemParams{
			BookID: a.Book.ID, TransactionID: txnID, Position: int16(i), Description: strings.TrimSpace(it.Description),
			Quantity: nullDecimal(it.Quantity), UnitPrice: nullDecimal(it.UnitPrice), Amount: it.Amount, AccountID: acct, Source: source,
		}); err != nil {
			return err
		}
	}
	return nil
}
