package ledger

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

// Securities from a sync (#37, migration 000010). A broker account is a
// source account of kind "brokerage", mapped to a parent account and to the
// bank account its trades settle through. Each security it reports gets its
// own account under the parent the first time it appears (source_securities
// remembers which), and its commodity is registered if the book has never
// seen it.
//
//   holding  units held on a date: an assertion on the security's account,
//            applied as soon as the broker account is mapped
//   trade    units in or out, booked when accepted, at the cash the person
//            confirms: a buy costs that cash (fees included); a sale
//            releases average cost and the difference is a realised gain
//
// Units that move with no cash -- a transfer between brokers, a stock
// dividend (taxed at par in Taiwan) -- are not booked here: they wait in the
// queue to be ignored and entered by hand.

const (
	KindCash      = "cash"
	KindBrokerage = "brokerage"
	// A source's securities are fractional until it says otherwise.
	securityDecimals = 4
	// After a trade is booked, the bank's settlement row (T+2, or later over
	// a weekend) is matched again so it shows as the duplicate it is.
	settlementWindow = 5
)

// securityInput is what a row says about its security.
type securityInput struct {
	Code, Name, Quote string
}

// ensureSecurities registers the securities a batch names that are not yet
// known, before the batch's own transaction, so the commodity cache can see
// them. A failed batch may leave one registered, which is harmless: the list
// is shared, like currencies.
func (s *Service) ensureSecurities(ctx context.Context, in []securityInput) error {
	added := false
	known, err := s.commodityMap(ctx)
	if err != nil {
		return err
	}
	for _, sec := range in {
		if _, ok := known[sec.Code]; ok {
			continue
		}
		name := strings.TrimSpace(sec.Name)
		if name == "" {
			_, name, _ = strings.Cut(sec.Code, ":")
		}
		quote := sec.Quote
		if err := s.store.EnsureSecurity(ctx, db.EnsureSecurityParams{Code: sec.Code, Name: name, Decimals: securityDecimals, QuoteCurrency: &quote}); err != nil {
			return err
		}
		added = true
	}
	if added {
		s.invalidateCommodities()
	}
	return nil
}

// findSecurityAccount is the account already holding a security for a
// broker account: its own (source_securities), or the one another source
// of the same broker made under the same parent. ok is false when there is
// none yet.
func findSecurityAccount(ctx context.Context, q *db.Queries, a Access, sourceID, parentID int64, code string) (id int64, own, ok bool, err error) {
	id, err = q.GetSourceSecurity(ctx, db.GetSourceSecurityParams{SourceAccountID: sourceID, Commodity: code})
	if err == nil {
		return id, true, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, false, err
	}
	id, err = q.FindSecurityChild(ctx, db.FindSecurityChildParams{BookID: a.Book.ID, ParentID: &parentID, Commodity: &code})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, false, nil
	}
	return id, false, err == nil, err
}

// securityAccount is the account holding a security for a broker account,
// made under its parent the first time no source of it has one.
func (s *Service) securityAccount(ctx context.Context, q *db.Queries, a Access, sourceID, parentID int64, code string) (int64, error) {
	id, own, ok, err := findSecurityAccount(ctx, q, a, sourceID, parentID, code)
	if err != nil {
		return 0, err
	}
	if ok {
		if !own {
			err = q.SetSourceSecurity(ctx, db.SetSourceSecurityParams{SourceAccountID: sourceID, Commodity: code, AccountID: id})
		}
		return id, err
	}
	parent, err := q.GetAccount(ctx, db.GetAccountParams{BookID: a.Book.ID, ID: parentID})
	if err != nil {
		return 0, translate(err, "account")
	}
	comms, err := s.commodityMap(ctx)
	if err != nil {
		return 0, err
	}
	c, ok := comms[code]
	if !ok {
		return 0, notFound("security " + code)
	}
	// "2330 台積電": the symbol a person knows, and the name.
	_, symbol, _ := strings.Cut(code, ":")
	name := symbol
	if c.Name != "" && c.Name != symbol {
		name = symbol + " " + c.Name
	}
	acct, err := q.CreateAccount(ctx, db.CreateAccountParams{
		BookID: a.Book.ID, ParentID: &parent.ID, Class: db.AccountClassAsset, Name: &name, Commodity: &code,
		CfClass: db.CfClassInvesting,
	})
	if err != nil {
		return 0, err
	}
	if err := q.SetSourceSecurity(ctx, db.SetSourceSecurityParams{SourceAccountID: sourceID, Commodity: code, AccountID: acct.ID}); err != nil {
		return 0, err
	}
	return acct.ID, nil
}

// applyHolding records a holding row as an assertion on its security's account.
func (s *Service) applyHolding(ctx context.Context, a Access, r db.GetImportRowRow) error {
	return s.store.WithTx(ctx, a.UserID, func(q *db.Queries) error {
		acct, err := s.securityAccount(ctx, q, a, r.SourceAccountID, *r.AccountID, *r.Security)
		if err != nil {
			return err
		}
		if err := q.UpsertAssertion(ctx, db.UpsertAssertionParams{BookID: a.Book.ID, AccountID: acct, Date: r.Date,
			Amount: r.Units.Decimal, Source: r.Connector}); err != nil {
			return err
		}
		_, err = q.DecideRow(ctx, db.DecideRowParams{ID: r.ID, Status: "accepted", DecidedBy: &a.UserID})
		return err
	})
}

// bookTrade books an accepted trade inside the caller's transaction and
// returns the transaction id. cash is signed on the settlement account.
func (s *Service) bookTrade(ctx context.Context, q *db.Queries, a Access, r db.GetImportRowRow, cash decimal.Decimal) (int64, error) {
	if r.SettlementAccountID == nil {
		return 0, invalid("settlement_unmapped", "choose the account this broker account settles through first")
	}
	units := r.Units.Decimal
	switch {
	case cash.IsZero():
		return 0, fieldError("cash", "required", "a trade needs the cash it settled for; units moving without cash are entered by hand")
	case cash.Sign() == units.Sign():
		return 0, fieldError("cash", "sign", "a buy pays cash out (< 0), a sale brings it in (> 0)")
	}
	sec, err := s.securityAccount(ctx, q, a, r.SourceAccountID, *r.AccountID, *r.Security)
	if err != nil {
		return 0, err
	}
	lc, err := s.newLineContext(ctx, q, a.Book, r.Date)
	if err != nil {
		return 0, err
	}
	var price *decimal.Decimal
	if r.Price.Valid {
		price = &r.Price.Decimal
	}
	cleared := db.PostingStatusCleared
	on := r.Date

	var lines []LineInput
	cashLine := LineInput{AccountID: *r.SettlementAccountID, Commodity: r.Currency, Amount: cash, Status: cleared, ClearedOn: &on}
	cp, err := s.resolveLine(ctx, q, lc, 1, cashLine)
	if err != nil {
		return 0, err
	}
	if units.IsPositive() { // a buy: the cash paid, fees included, is the cost
		cost := cp.BaseAmount.Neg()
		lines = []LineInput{{AccountID: sec, Amount: units, BaseAmount: &cost, UnitCost: price}, cashLine}
	} else { // a sale: average cost leaves, the rest is a gain or a loss
		secLine := LineInput{AccountID: sec, Amount: units}
		sp, err := s.resolveLine(ctx, q, lc, 0, secLine)
		if err != nil {
			return 0, err
		}
		lines = []LineInput{secLine, cashLine}
		if gain := cp.BaseAmount.Add(sp.BaseAmount).Neg(); !gain.IsZero() {
			gains, err := q.GetAccountByTemplateKey(ctx, db.GetAccountByTemplateKeyParams{BookID: a.Book.ID, TemplateKey: ptr("realized_gains")})
			if err != nil {
				return 0, translate(err, "realised gains account")
			}
			lines = append(lines, LineInput{AccountID: gains.ID, Commodity: a.Book.BaseCurrency, Amount: gain})
		}
	}
	ext := r.ExternalID
	return s.createTx(ctx, q, a, TransactionInput{
		Date: r.Date, Payee: tradePayee(r), Memo: r.Description, Source: "sync", ExternalID: &ext, Lines: lines,
	})
}

func tradePayee(r db.GetImportRowRow) string {
	if p := strings.TrimSpace(r.Counterparty); p != "" {
		return p
	}
	_, symbol, _ := strings.Cut(*r.Security, ":")
	if r.Units.Decimal.IsPositive() {
		return "Buy " + symbol
	}
	return "Sell " + symbol
}

// rematchSettlement proposes again the waiting rows on a settlement account
// near a trade, so the bank's own line for it shows as a duplicate.
func (s *Service) rematchSettlement(ctx context.Context, a Access, account int64, on time.Time) error {
	ids, err := s.store.PendingRowsOfAccount(ctx, db.PendingRowsOfAccountParams{BookID: a.Book.ID, AccountID: &account,
		FromDate: on.AddDate(0, 0, -settlementWindow), ToDate: on.AddDate(0, 0, settlementWindow)})
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.propose(ctx, a, id); err != nil {
			return err
		}
	}
	return nil
}

// securityRow checks a holding or trade row and fills its columns. Their
// amount is 0: units, price and cash are columns of their own.
func securityRow(p *db.InsertImportRowParams, r ImportRowInput, c db.Commodity, cashCurrency string, i int) error {
	if r.Units == nil {
		return fieldError(idx("rows", i, "units"), "required", "a holding or trade needs its units")
	}
	units := *r.Units
	switch {
	case r.Kind == "holding" && units.IsNegative():
		return fieldError(idx("rows", i, "units"), "sign", "a holding is not negative")
	case r.Kind == "trade" && units.IsZero():
		return fieldError(idx("rows", i, "units"), "zero", "a trade moves units")
	case !hasPrecision(units, int32(c.Decimals)):
		return fieldError(idx("rows", i, "units"), "too_precise", "%s allows %d decimal places", c.Code, c.Decimals)
	case r.Price != nil && !r.Price.IsPositive():
		return fieldError(idx("rows", i, "price"), "invalid", "a price is positive")
	case r.Kind == "holding" && r.Cash != nil:
		return fieldError(idx("rows", i, "cash"), "invalid", "a holding settles no cash")
	case r.Cash != nil && !r.Cash.IsZero() && r.Cash.Sign() == units.Sign():
		return fieldError(idx("rows", i, "cash"), "sign", "a buy pays cash out (< 0), a sale brings it in (> 0)")
	}
	p.Amount = decimal.Zero
	p.Security = &c.Code
	p.Units = decimal.NullDecimal{Decimal: units, Valid: true}
	p.Price = nullDecimal(r.Price)
	p.Cash = nullDecimal(r.Cash)
	if r.Kind == "holding" {
		p.Currency = c.Code // a holding is counted in the security itself
	} else {
		p.Currency = cashCurrency
	}
	return nil
}

// checkBrokerageMapping: the parent is an asset account (a group is fine:
// only the securities' own accounts take postings); the settlement account
// takes postings in the cash the trades settle in.
func (s *Service) checkBrokerageMapping(ctx context.Context, a Access, sa db.SourceAccount, m SourceMapping) error {
	if m.AccountID != nil {
		acc, err := s.store.GetAccount(ctx, db.GetAccountParams{BookID: a.Book.ID, ID: *m.AccountID})
		if err != nil {
			return fieldError("account_id", "not_found", "no such account in this book")
		}
		if acc.Class != db.AccountClassAsset {
			return fieldError("account_id", "invalid", "the securities go under an asset account")
		}
	}
	if m.SettlementAccountID != nil {
		acc, err := s.store.GetAccount(ctx, db.GetAccountParams{BookID: a.Book.ID, ID: *m.SettlementAccountID})
		if err != nil {
			return fieldError("settlement_account_id", "not_found", "no such account in this book")
		}
		if acc.IsPlaceholder || acc.Class != db.AccountClassAsset {
			return fieldError("settlement_account_id", "invalid", "trades settle through an asset account that takes postings")
		}
		cur := a.Book.BaseCurrency
		if sa.Currency != nil {
			cur = *sa.Currency
		}
		if acc.Commodity != nil && *acc.Commodity != cur {
			return fieldError("settlement_account_id", "mismatch", "the account holds %s, the trades settle in %s", *acc.Commodity, cur)
		}
	}
	return nil
}

// proposeTrade matches a trade against what other sources of the same
// broker account brought (#81): 集保 sends a trade's units with no cash,
// Shioaji the same trade with its cash.
//
//  1. Already in the books: the same units on the security's account, a
//     few days apart, is a duplicate of that transaction.
//  2. Waiting from another source: the one with cash books (between two
//     alike, the first staged), and the other waits to go with it.
//
// Otherwise it is new, booked at the cash the person confirms.
func (s *Service) proposeTrade(ctx context.Context, a Access, r db.GetImportRowRow) error {
	set := func(id int64, p db.SetRowProposalParams) error {
		p.ID = id
		return s.store.SetRowProposal(ctx, p)
	}
	from, to := r.Date.AddDate(0, 0, -duplicateWindow), r.Date.AddDate(0, 0, duplicateWindow)
	sec, _, ok, err := findSecurityAccount(ctx, s.store.Queries, a, r.SourceAccountID, *r.AccountID, *r.Security)
	if err != nil {
		return err
	}
	if ok {
		tx, err := s.store.FindTradeDuplicate(ctx, db.FindTradeDuplicateParams{BookID: a.Book.ID, AccountID: sec, Units: r.Units.Decimal,
			FromDate: from, ToDate: to, RowID: r.ID, OnDate: r.Date})
		if err == nil {
			return set(r.ID, db.SetRowProposalParams{Proposal: "duplicate", MatchTransactionID: &tx})
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	other, err := s.store.FindTradeRow(ctx, db.FindTradeRowParams{BookID: a.Book.ID, RowID: r.ID, AccountID: r.AccountID,
		SourceAccountID: r.SourceAccountID, Security: r.Security, Units: r.Units, FromDate: from, ToDate: to, OnDate: r.Date})
	if err == nil {
		mine := !r.Cash.Valid && other.HasCash || r.Cash.Valid == other.HasCash && r.ID > other.ID
		if mine {
			return set(r.ID, db.SetRowProposalParams{Proposal: "duplicate", MatchRowID: &other.ID})
		}
		if err := set(other.ID, db.SetRowProposalParams{Proposal: "duplicate", MatchRowID: &r.ID}); err != nil {
			return err
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return set(r.ID, db.SetRowProposalParams{Proposal: "new"})
}

// acceptTradeDuplicate settles a trade the books already have: the row
// becomes that transaction's, and nothing is booked twice.
func (s *Service) acceptTradeDuplicate(ctx context.Context, a Access, r db.GetImportRowRow) (int64, error) {
	if r.MatchTransactionID == nil {
		if r.MatchRowID != nil {
			return 0, conflict("match_pending", "this is the same trade as another waiting row: accept that one, and this one goes with it")
		}
		if err := s.proposeTrade(ctx, a, r); err != nil {
			return 0, err
		}
		return 0, conflict("match_gone", "what this row matched is gone; it was matched again")
	}
	txnID := *r.MatchTransactionID
	n, err := s.store.DecideRow(ctx, db.DecideRowParams{ID: r.ID, Status: "accepted", TransactionID: &txnID, DecidedBy: &a.UserID})
	if err == nil && n == 0 {
		return 0, conflict("already_decided", "this row was already accepted or ignored")
	}
	return txnID, err
}

// acceptTrade books a trade at the cash the source sent or the person
// confirmed, then matches the settlement account's waiting rows again.
func (s *Service) acceptTrade(ctx context.Context, a Access, r db.GetImportRowRow, cash *decimal.Decimal) (int64, error) {
	if cash == nil && r.Cash.Valid {
		cash = &r.Cash.Decimal
	}
	if cash == nil {
		return 0, fieldError("cash", "required", "enter the cash this trade settled for")
	}
	var txnID int64
	err := s.store.WithTx(ctx, a.UserID, func(q *db.Queries) error {
		var err error
		if txnID, err = s.bookTrade(ctx, q, a, r, *cash); err != nil {
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
		if err != nil {
			return err
		}
		// The same trade from another source (集保's, without cash) goes with it.
		return settleDuplicates(ctx, q, a, r.ID, txnID)
	})
	if err != nil {
		return 0, translate(err, "import row")
	}
	return txnID, s.rematchSettlement(ctx, a, *r.SettlementAccountID, r.Date)
}
