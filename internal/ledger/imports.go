package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

// The import core (P4a): every source -- a CSV today, the sync runners
// later -- sends the same shape. Rows are staged and matched; nothing
// reaches the books until a person accepts it in the review queue
// (docs/ARCHITECTURE.md, "Data sources and sync").

const (
	duplicateWindow = 3 // days either side
	estimateWindow  = 5
	transferWindow  = 3
)

// estimateTolerance: a posted card charge may differ from its estimate by
// this share (FX, tips, a partial refund) and still settle it.
var estimateTolerance = decimal.RequireFromString("0.10")

var connectorPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)

type ImportAccount struct {
	ExternalID string
	Label      string
	Currency   string
	Kind       string // cash (default) | brokerage: holds securities, settles through a cash account
}

type ImportRowInput struct {
	Kind         string // transaction | balance | holding | trade
	Account      string // ImportAccount.ExternalID
	ID           string // the source's own id; with the connector, the dedupe key
	Date         time.Time
	Amount       decimal.Decimal // debit > 0 on the account (money in, a card payment)
	Currency     string
	Description  string
	Counterparty string
	Pending      bool
	Raw          json.RawMessage
	File         string // ImportFile.Ref: the row's evidence, attached when it is accepted

	// holding and trade rows (#37): the security as NAMESPACE:SYMBOL
	// (XTAI:2330), registered with its name and quote currency the first
	// time; units held (holding) or moved, > 0 in (trade); the price per
	// unit in the quote currency and the cash settled, signed on the
	// settlement account, when the source knows them.
	Security      string
	SecurityName  string
	QuoteCurrency string
	Units         *decimal.Decimal
	Price         *decimal.Decimal
	Cash          *decimal.Decimal

	// invoice rows (#38): the invoice's lines. Amount is its total, signed
	// on the account that paid (a purchase < 0); Counterparty the seller.
	// An amount of 0 with no items is evidence whose amount the source does
	// not state (#57). Reference is an invoice number another source knows.
	Items     []InvoiceItem
	Reference string
}

// ImportFile is evidence a batch carries (an order email, an e-invoice, a
// statement), named by Ref for its rows.
type ImportFile struct {
	Ref string
	FileInput
}

type ImportInput struct {
	Connector string
	Label     string
	Accounts  []ImportAccount
	Rows      []ImportRowInput
	Files     []ImportFile
}

type ImportResult struct {
	BatchID    int64
	Received   int
	Staged     int
	Duplicates int
	Balances   int // assertions recorded at once (the account was mapped)
}

// Import stages a batch. A row whose external id is already staged (from
// any earlier batch) is counted as a duplicate and dropped, so a runner can
// resend a whole statement safely.
func (s *Service) Import(ctx context.Context, a Access, in ImportInput) (ImportResult, error) {
	if err := a.require(db.MemberRoleEditor); err != nil {
		return ImportResult{}, err
	}
	in.Connector = strings.ToLower(strings.TrimSpace(in.Connector))
	if !connectorPattern.MatchString(in.Connector) {
		return ImportResult{}, fieldError("connector", "invalid", "a connector is a-z, 0-9, dash, underscore")
	}
	if len(in.Rows) > 5000 {
		return ImportResult{}, fieldError("rows", "too_many", "at most 5000 rows per batch")
	}
	// Securities first, so the commodity map below knows them.
	kinds := map[string]string{}
	for _, acc := range in.Accounts {
		kinds[strings.TrimSpace(acc.ExternalID)] = acc.Kind
	}
	var secs []securityInput
	for i, r := range in.Rows {
		if r.Kind != "holding" && r.Kind != "trade" {
			continue
		}
		code := strings.ToUpper(strings.TrimSpace(r.Security))
		if !commodityCode.MatchString(code) {
			return ImportResult{}, fieldError(idx("rows", i, "security"), "invalid", `a security is NAMESPACE:SYMBOL, e.g. "XTAI:2330"`)
		}
		if kinds[r.Account] != KindBrokerage {
			return ImportResult{}, fieldError(idx("rows", i, "account"), "invalid", "holdings and trades belong to a brokerage account")
		}
		quote := strings.ToUpper(strings.TrimSpace(r.QuoteCurrency))
		if quote == "" {
			quote = a.Book.BaseCurrency
		}
		if err := s.validCurrency(ctx, quote); err != nil {
			return ImportResult{}, fieldError(idx("rows", i, "quote_currency"), "unknown", "unknown currency %q", quote)
		}
		in.Rows[i].Security = code
		secs = append(secs, securityInput{Code: code, Name: r.SecurityName, Quote: quote})
	}
	if err := s.ensureSecurities(ctx, secs); err != nil {
		return ImportResult{}, err
	}
	commods, err := s.commodityMap(ctx)
	if err != nil {
		return ImportResult{}, err
	}

	res := ImportResult{Received: len(in.Rows)}
	var staged []int64
	err = s.store.WithTx(ctx, a.UserID, func(q *db.Queries) error {
		sources := map[string]db.SourceAccount{}
		for i, acc := range in.Accounts {
			ext := strings.TrimSpace(acc.ExternalID)
			if ext == "" {
				return fieldError(idx("accounts", i, "external_id"), "required", "an account needs an id")
			}
			var cur *string
			if c := strings.ToUpper(strings.TrimSpace(acc.Currency)); c != "" {
				if _, ok := commods[c]; !ok {
					return fieldError(idx("accounts", i, "currency"), "unknown", "unknown currency %q", c)
				}
				cur = &c
			}
			kind := acc.Kind
			if kind == "" {
				kind = KindCash
			}
			if kind != KindCash && kind != KindBrokerage {
				return fieldError(idx("accounts", i, "kind"), "invalid", "kind is cash or brokerage")
			}
			sa, err := q.UpsertSourceAccount(ctx, db.UpsertSourceAccountParams{
				BookID: a.Book.ID, Connector: in.Connector, ExternalID: ext, Label: strings.TrimSpace(acc.Label), Currency: cur, Kind: kind,
			})
			if err != nil {
				return err
			}
			sources[ext] = sa
		}
		batch, err := q.CreateImportBatch(ctx, db.CreateImportBatchParams{
			BookID: a.Book.ID, Connector: in.Connector, Label: strings.TrimSpace(in.Label), CreatedBy: &a.UserID,
		})
		if err != nil {
			return err
		}
		res.BatchID = batch.ID
		files := map[string]int{} // ref -> index in in.Files
		for i, f := range in.Files {
			ref := strings.TrimSpace(f.Ref)
			if _, dup := files[ref]; ref == "" || dup {
				return fieldError(idx("files", i, "ref"), "invalid", "every file needs its own ref")
			}
			files[ref] = i
		}
		stored := map[string]int64{} // ref -> attachment, kept only for a row that is new
		for i, r := range in.Rows {
			fi, hasFile := files[r.File]
			if r.File != "" && !hasFile {
				return fieldError(idx("rows", i, "file"), "unknown", "file %q is not in files", r.File)
			}
			sa, ok := sources[r.Account]
			if !ok {
				return fieldError(idx("rows", i, "account"), "unknown", "account %q is not in accounts", r.Account)
			}
			switch r.Kind {
			case "transaction", "balance", "holding", "trade", "invoice":
			default:
				return fieldError(idx("rows", i, "kind"), "invalid", "kind is transaction, balance, holding, trade or invoice")
			}
			if strings.TrimSpace(r.ID) == "" {
				return fieldError(idx("rows", i, "id"), "required", "every row needs the source's id")
			}
			if r.Date.IsZero() {
				return fieldError(idx("rows", i, "date"), "required", "every row needs a date")
			}
			cur := strings.ToUpper(strings.TrimSpace(r.Currency))
			if cur == "" && sa.Currency != nil {
				cur = *sa.Currency
			}
			if cur == "" && (r.Kind == "holding" || r.Kind == "trade") {
				cur = a.Book.BaseCurrency // what a trade settles in, unless the source says
			}
			if _, ok := commods[cur]; !ok {
				return fieldError(idx("rows", i, "currency"), "unknown", "unknown currency %q", cur)
			}
			raw := []byte(r.Raw)
			if len(raw) == 0 {
				raw = []byte("{}")
			}
			params := db.InsertImportRowParams{
				BookID: a.Book.ID, BatchID: batch.ID, SourceAccountID: sa.ID, Kind: r.Kind,
				ExternalID: in.Connector + ":" + strings.TrimSpace(r.ID), Date: r.Date, Amount: r.Amount, Currency: cur,
				Description: strings.TrimSpace(r.Description), Counterparty: strings.TrimSpace(r.Counterparty),
				Pending: r.Pending, Raw: raw,
			}
			if r.Kind == "holding" || r.Kind == "trade" {
				if err := securityRow(&params, r, commods[r.Security], cur, i); err != nil {
					return err
				}
			}
			if r.Kind == "invoice" {
				if r.Amount.IsZero() && len(r.Items) > 0 {
					return fieldError(idx("rows", i, "amount"), "zero", "an invoice with items has a total")
				}
				if err := checkItems(r.Items, r.Amount.IsZero(), int32(commods[cur].Decimals), i); err != nil {
					return err
				}
				if ref := strings.TrimSpace(r.Reference); ref != "" {
					if len(ref) < 6 || len(ref) > 64 {
						return fieldError(idx("rows", i, "reference"), "invalid", "an invoice number has 6 to 64 characters")
					}
					params.Reference = &ref
				}
				if r.Items == nil {
					r.Items = []InvoiceItem{} // [] for the row's check, not null
				}
				if params.Items, err = json.Marshal(r.Items); err != nil {
					return err
				}
			} else if len(r.Items) > 0 {
				return fieldError(idx("rows", i, "items"), "invalid", "only an invoice has items")
			} else if ref := strings.TrimSpace(r.Reference); ref != "" {
				// A transaction's reference is the bank's movement number, which
				// pairs the two sides of an exchange between currencies.
				if r.Kind != "transaction" || len(ref) > 64 {
					return fieldError(idx("rows", i, "reference"), "invalid", "only an invoice or a transaction has a reference")
				}
				params.Reference = &ref
			}
			id, err := q.InsertImportRow(ctx, params)
			if errors.Is(err, pgx.ErrNoRows) {
				res.Duplicates++
				continue
			}
			if err != nil {
				return err
			}
			if hasFile {
				at, ok := stored[r.File]
				if !ok {
					if at, err = s.storeFile(ctx, q, a, in.Files[fi].FileInput, idx("files", fi, "data")); err != nil {
						return err
					}
					stored[r.File] = at
				}
				if err := q.SetRowAttachment(ctx, db.SetRowAttachmentParams{BookID: a.Book.ID, ID: id, AttachmentID: &at}); err != nil {
					return err
				}
			}
			staged = append(staged, id)
		}
		return q.SetBatchCounts(ctx, db.SetBatchCountsParams{ID: batch.ID, Received: int32(res.Received), Duplicates: int32(res.Duplicates)})
	})
	if err != nil {
		return ImportResult{}, translate(err, "import")
	}
	res.Staged = len(staged)
	for _, id := range staged {
		applied, err := s.propose(ctx, a, id)
		if err != nil {
			return res, err
		}
		if applied {
			res.Balances++
		}
	}
	// A card row just staged may be what a waiting invoice was paid with.
	return res, s.rematchInvoices(ctx, a)
}

func idx(list string, i int, field string) string {
	return list + "[" + itoa(i) + "]." + field
}

func itoa(i int) string {
	return decimal.NewFromInt(int64(i)).String()
}

// propose runs the matcher on one pending row. A balance row on a mapped
// account becomes an assertion at once (reported as applied); a transaction
// row gets a proposal for the review queue.
func (s *Service) propose(ctx context.Context, a Access, rowID int64) (applied bool, err error) {
	r, err := s.store.GetImportRow(ctx, db.GetImportRowParams{BookID: a.Book.ID, ID: rowID})
	if err != nil {
		return false, err
	}
	if r.Status == "pending" && r.Kind == "invoice" {
		// Matched whether or not its source is mapped: only a cash purchase needs that.
		return false, s.proposeInvoice(ctx, a, r)
	}
	if r.Status == "pending" && r.SourceIgnored {
		// Set aside: another source brings this account (#81).
		_, err := s.store.DecideRow(ctx, db.DecideRowParams{ID: r.ID, Status: "ignored"})
		return false, err
	}
	if r.Status != "pending" || r.AccountID == nil {
		return false, nil // unmapped: waits for its source account to be mapped
	}
	acct := *r.AccountID
	switch r.Kind {
	case "holding":
		return true, s.applyHolding(ctx, a, r)
	case "trade":
		return false, s.proposeTrade(ctx, a, r)
	}
	if r.Kind == "balance" {
		err := s.store.WithTx(ctx, a.UserID, func(q *db.Queries) error {
			if err := q.UpsertAssertion(ctx, db.UpsertAssertionParams{BookID: a.Book.ID, AccountID: acct, Date: r.Date,
				Amount: r.Amount, Source: r.Connector}); err != nil {
				return err
			}
			_, err := q.DecideRow(ctx, db.DecideRowParams{ID: r.ID, Status: "accepted", DecidedBy: &a.UserID})
			return err
		})
		return err == nil, err
	}

	window := func(days int) (time.Time, time.Time) {
		return r.Date.AddDate(0, 0, -days), r.Date.AddDate(0, 0, days)
	}
	set := func(p db.SetRowProposalParams) error {
		p.ID = r.ID
		return s.store.SetRowProposal(ctx, p)
	}

	// 1. Already in the books (typed in by hand, or an earlier import).
	from, to := window(duplicateWindow)
	if tx, err := s.store.FindDuplicate(ctx, db.FindDuplicateParams{BookID: a.Book.ID, AccountID: acct, Amount: r.Amount,
		FromDate: from, ToDate: to, OnDate: r.Date}); err == nil {
		return false, set(db.SetRowProposalParams{Proposal: "duplicate", MatchTransactionID: &tx})
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	// 2. The posted form of an uncleared estimate (a card charge entered by
	// hand, or accepted while pending): same sign, within the tolerance.
	if !r.Pending && !r.Amount.IsZero() {
		lo, hi := r.Amount.Mul(decimal.NewFromInt(1).Sub(estimateTolerance)), r.Amount.Mul(decimal.NewFromInt(1).Add(estimateTolerance))
		if lo.GreaterThan(hi) {
			lo, hi = hi, lo
		}
		from, to := window(estimateWindow)
		if tx, err := s.store.FindEstimate(ctx, db.FindEstimateParams{BookID: a.Book.ID, AccountID: acct, Lo: lo, Hi: hi,
			FromDate: from, ToDate: to, Amount: r.Amount, OnDate: r.Date}); err == nil {
			return false, set(db.SetRowProposalParams{Proposal: "clears", MatchTransactionID: &tx})
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return false, err
		}
	}
	// 3. The same charge waiting from another source (a card alert and the
	// statement's row; a 交割 movement from 集保 and from the bank): the
	// posted one books, between two alike the first staged, and the other
	// waits to be settled with it.
	if other, err := s.store.FindDuplicateRow(ctx, db.FindDuplicateRowParams{BookID: a.Book.ID, RowID: r.ID, AccountID: &acct,
		SourceAccountID: r.SourceAccountID, Currency: r.Currency, Amount: r.Amount, FromDate: from, ToDate: to, OnDate: r.Date}); err == nil {
		mine := r.Pending && !other.Pending || r.Pending == other.Pending && r.ID > other.ID
		if mine {
			return false, set(db.SetRowProposalParams{Proposal: "duplicate", MatchRowID: &other.ID})
		}
		if err := s.store.SetRowProposal(ctx, db.SetRowProposalParams{ID: other.ID, Proposal: "duplicate", MatchRowID: &r.ID}); err != nil {
			return false, err
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	// 4. One side of a transfer between two of the book's accounts.
	from, to = window(transferWindow)
	if other, err := s.store.FindTransferPartner(ctx, db.FindTransferPartnerParams{BookID: a.Book.ID, RowID: r.ID,
		AccountID: &acct, Currency: r.Currency, Amount: r.Amount, FromDate: from, ToDate: to, OnDate: r.Date}); err == nil {
		if err := set(db.SetRowProposalParams{Proposal: "transfer", MatchRowID: &other}); err != nil {
			return false, err
		}
		return false, s.store.SetRowProposal(ctx, db.SetRowProposalParams{ID: other, Proposal: "transfer", MatchRowID: &r.ID})
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	// An exchange between currencies: the bank's movement number pairs the sides.
	if r.Reference != nil {
		if other, err := s.store.FindTransferByReference(ctx, db.FindTransferByReferenceParams{BookID: a.Book.ID, RowID: r.ID,
			AccountID: &acct, Reference: *r.Reference, Currency: r.Currency, Amount: r.Amount, FromDate: from, ToDate: to, OnDate: r.Date}); err == nil {
			if err := set(db.SetRowProposalParams{Proposal: "transfer", MatchRowID: &other}); err != nil {
				return false, err
			}
			return false, s.store.SetRowProposal(ctx, db.SetRowProposalParams{ID: other, Proposal: "transfer", MatchRowID: &r.ID})
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return false, err
		}
	}
	// 5. New: the first rule whose pattern the text contains.
	rules, err := s.store.ListRules(ctx, a.Book.ID)
	if err != nil {
		return false, err
	}
	text := strings.ToLower(r.Description + " " + r.Counterparty)
	for _, rule := range rules {
		if rule.SourceAccountID != nil && *rule.SourceAccountID != r.SourceAccountID {
			continue
		}
		if strings.Contains(text, strings.ToLower(rule.Pattern)) {
			return false, set(db.SetRowProposalParams{Proposal: "new", ProposedAccountID: &rule.AccountID, RuleID: &rule.ID})
		}
	}
	return false, set(db.SetRowProposalParams{Proposal: "new"})
}

// SourceMapping is where a source account's rows go: one ledger account; for
// a brokerage, the parent its securities' accounts are made under, and the
// account its trades settle through. Ignored sets the source account aside
// (#81): its waiting rows and every row it sends later are ignored, because
// another source brings the same account.
type SourceMapping struct {
	AccountID           *int64
	SettlementAccountID *int64
	Ignored             bool
}

// MapSourceAccount ties a source account to a ledger account and re-runs
// the matcher on its waiting rows.
func (s *Service) MapSourceAccount(ctx context.Context, a Access, sourceID int64, m SourceMapping) (db.SourceAccount, error) {
	if err := a.require(db.MemberRoleEditor); err != nil {
		return db.SourceAccount{}, err
	}
	sa, err := s.store.GetSourceAccount(ctx, db.GetSourceAccountParams{BookID: a.Book.ID, ID: sourceID})
	if err != nil {
		return db.SourceAccount{}, translate(err, "source account")
	}
	accountID := m.AccountID
	if sa.Kind == KindBrokerage {
		if err := s.checkBrokerageMapping(ctx, a, sa, m); err != nil {
			return db.SourceAccount{}, err
		}
	} else if m.SettlementAccountID != nil {
		return db.SourceAccount{}, fieldError("settlement_account_id", "invalid", "only a brokerage account settles through another")
	}
	if accountID != nil && sa.Kind != KindBrokerage {
		acc, err := s.store.GetAccount(ctx, db.GetAccountParams{BookID: a.Book.ID, ID: *accountID})
		if err != nil {
			return db.SourceAccount{}, fieldError("account_id", "not_found", "no such account in this book")
		}
		if acc.IsPlaceholder || (acc.Class != db.AccountClassAsset && acc.Class != db.AccountClassLiability) {
			return db.SourceAccount{}, fieldError("account_id", "invalid", "map to an asset or liability account")
		}
		if sa.Currency != nil && acc.Commodity != nil && *acc.Commodity != *sa.Currency {
			return db.SourceAccount{}, fieldError("account_id", "mismatch", "the account holds %s, the source sends %s", *acc.Commodity, *sa.Currency)
		}
	}
	sa, err = s.store.MapSourceAccount(ctx, db.MapSourceAccountParams{BookID: a.Book.ID, ID: sourceID, AccountID: accountID,
		SettlementAccountID: m.SettlementAccountID, Ignored: m.Ignored})
	if err != nil {
		return db.SourceAccount{}, translate(err, "source account")
	}
	if m.Ignored {
		return sa, s.ignoreSource(ctx, a, sa.ID)
	}
	if accountID != nil {
		ids, err := s.store.PendingRowsOfSource(ctx, sa.ID)
		if err != nil {
			return sa, err
		}
		for _, id := range ids {
			if _, err := s.propose(ctx, a, id); err != nil {
				return sa, err
			}
		}
	}
	return sa, nil
}

// AcceptRow books a row as the queue proposes, or against categoryID when
// given (a "new" row without a rule needs one).
func (s *Service) AcceptRow(ctx context.Context, a Access, rowID int64, categoryID *int64) (int64, error) {
	return s.Accept(ctx, a, rowID, AcceptInput{CategoryID: categoryID})
}

// AcceptInput is what the person adds when accepting a row: a category for
// a new transaction row, the cash a trade settled for (signed on the
// settlement account) when the source did not send it.
type AcceptInput struct {
	CategoryID *int64
	Cash       *decimal.Decimal
	// An invoice: split the expense by the categories its items' rules give.
	Split bool
}

// Accept books a row; see AcceptRow.
func (s *Service) Accept(ctx context.Context, a Access, rowID int64, in AcceptInput) (int64, error) {
	categoryID := in.CategoryID
	if err := a.require(db.MemberRoleEditor); err != nil {
		return 0, err
	}
	r, err := s.store.GetImportRow(ctx, db.GetImportRowParams{BookID: a.Book.ID, ID: rowID})
	if err != nil {
		return 0, translate(err, "import row")
	}
	if r.Status != "pending" {
		return 0, conflict("already_decided", "this row was already accepted or ignored")
	}
	if r.Kind == "invoice" {
		return s.acceptInvoice(ctx, a, r, in)
	}
	if r.AccountID == nil {
		return 0, invalid("unmapped", "map the row's source account to an account first")
	}
	if r.Kind == "trade" {
		if r.Proposal == "duplicate" {
			return s.acceptTradeDuplicate(ctx, a, r)
		}
		return s.acceptTrade(ctx, a, r, in.Cash)
	}
	if r.Kind != "transaction" {
		return 0, invalid("invalid_input", "balance and holding rows are applied when their account is mapped")
	}
	acct := *r.AccountID
	source := "sync"
	if r.Connector == "csv" {
		source = "import"
	}
	payee, memo := r.Counterparty, r.Description
	if payee == "" {
		payee, memo = r.Description, ""
	}
	status := db.PostingStatusCleared
	if r.Pending {
		status = db.PostingStatusUncleared
	}
	proposal := r.Proposal
	if categoryID != nil {
		proposal = "new" // the person chose a category: book it as new
	}
	if proposal == "duplicate" && r.MatchTransactionID == nil && r.MatchRowID != nil {
		return 0, conflict("match_pending", "this is the same charge as another waiting row: accept that one, and this one goes with it")
	}
	// What it matched was deleted since (the FK set it to NULL): match again.
	if (proposal == "duplicate" || proposal == "clears") && r.MatchTransactionID == nil ||
		proposal == "transfer" && r.MatchRowID == nil {
		if _, err := s.propose(ctx, a, r.ID); err != nil {
			return 0, err
		}
		return 0, conflict("match_gone", "what this row matched is gone; it was matched again")
	}

	var txnID int64
	var evidence []int64
	if r.AttachmentID != nil {
		evidence = append(evidence, *r.AttachmentID)
	}
	err = s.store.WithTx(ctx, a.UserID, func(q *db.Queries) error {
		switch proposal {
		case "duplicate":
			txnID = *r.MatchTransactionID
			if err := q.SetPostingStatus(ctx, db.SetPostingStatusParams{TransactionID: txnID, AccountID: acct,
				Status: status, ClearedOn: clearedOn(status, r.Date)}); err != nil {
				return err
			}
		case "clears":
			txnID = *r.MatchTransactionID
			cur, err := s.viewTx(ctx, q, a, txnID)
			if err != nil {
				return err
			}
			in := TransactionInput{Date: cur.Date, Payee: cur.Payee, Memo: cur.Memo, Tags: cur.Tags}
			for _, p := range cur.Postings {
				l := LineInput{AccountID: p.AccountID, Commodity: p.Commodity, Memo: p.Memo, Status: p.Status, ClearedOn: p.ClearedOn}
				if p.AccountID == acct {
					l.Amount, l.Status, l.ClearedOn = r.Amount, status, clearedOn(status, r.Date)
				} else {
					// The other leg follows. A foreign one (300 USD of travel on
					// a TWD card) keeps its own amount and takes the posted
					// figure as its base, as a settled card charge does.
					switch {
					case p.Commodity == r.Currency:
						l.Amount = r.Amount.Neg()
					case r.Currency == a.Book.BaseCurrency:
						base := r.Amount.Neg()
						l.Amount, l.BaseAmount = p.Amount, &base
					default:
						l.Amount = p.Amount
					}
				}
				in.Lines = append(in.Lines, l)
			}
			if err := s.updateTx(ctx, q, a, txnID, in); err != nil {
				return err
			}
		case "transfer":
			other, err := q.GetImportRow(ctx, db.GetImportRowParams{BookID: a.Book.ID, ID: *r.MatchRowID})
			if err != nil || other.Status != "pending" || other.AccountID == nil {
				return conflict("transfer_gone", "the other side of the transfer is no longer waiting")
			}
			ext := r.ExternalID
			lines := []LineInput{
				{AccountID: acct, Commodity: r.Currency, Amount: r.Amount, Status: status, ClearedOn: clearedOn(status, r.Date)},
				{AccountID: *other.AccountID, Commodity: other.Currency, Amount: other.Amount, Status: db.PostingStatusCleared, ClearedOn: &other.Date},
			}
			if other.Currency != r.Currency {
				// An exchange: the bank's own rate is the two amounts, so one
				// side's value in the base is the other's, not a second rate.
				base, err := s.baseValue(ctx, q, a, r.Currency, r.Amount, r.Date, other.Currency, other.Amount)
				if err != nil {
					return err
				}
				neg := base.Neg()
				lines[0].BaseAmount, lines[1].BaseAmount = &base, &neg
			}
			txnID, err = s.createTx(ctx, q, a, TransactionInput{
				Date: minDate(r.Date, other.Date), Payee: payee, Memo: memo, Source: source, ExternalID: &ext, Lines: lines,
			})
			if err != nil {
				return err
			}
			if _, err := q.DecideRow(ctx, db.DecideRowParams{ID: other.ID, Status: "accepted", TransactionID: &txnID, DecidedBy: &a.UserID}); err != nil {
				return err
			}
			if other.AttachmentID != nil {
				evidence = append(evidence, *other.AttachmentID)
			}
		default: // new
			cat := categoryID
			if cat == nil {
				cat = r.ProposedAccountID
			}
			if cat == nil {
				return fieldError("account_id", "required", "choose a category for this row")
			}
			ext := r.ExternalID
			var err error
			txnID, err = s.createTx(ctx, q, a, TransactionInput{
				Date: r.Date, Payee: payee, Memo: memo, Source: source, ExternalID: &ext,
				Lines: []LineInput{
					{AccountID: acct, Commodity: r.Currency, Amount: r.Amount, Status: status, ClearedOn: clearedOn(status, r.Date)},
					{AccountID: *cat, Commodity: r.Currency, Amount: r.Amount.Neg()},
				},
			})
			if err != nil {
				return err
			}
		}
		// The row's evidence (and a transfer's other side's) follows it.
		for _, at := range evidence {
			if err := q.LinkAttachment(ctx, db.LinkAttachmentParams{BookID: a.Book.ID, TransactionID: txnID, AttachmentID: at, CreatedBy: &a.UserID}); err != nil {
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
		return settleDuplicates(ctx, q, a, r.ID, txnID)
	})
	if err != nil {
		return 0, translate(err, "import row")
	}
	// An invoice waiting for this row can now be added to its transaction.
	return txnID, s.rematchInvoices(ctx, a)
}

func clearedOn(st db.PostingStatus, d time.Time) *time.Time {
	if st == db.PostingStatusUncleared {
		return nil
	}
	return &d
}

func minDate(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

// viewTx reads a transaction and its postings inside a database transaction.
func (s *Service) viewTx(ctx context.Context, q *db.Queries, a Access, id int64) (TransactionView, error) {
	t, err := q.GetTransaction(ctx, db.GetTransactionParams{BookID: a.Book.ID, ID: id})
	if err != nil {
		return TransactionView{}, translate(err, "transaction")
	}
	ps, err := q.ListPostings(ctx, []int64{id})
	if err != nil {
		return TransactionView{}, err
	}
	tags, err := q.ListTransactionTags(ctx, []int64{id})
	if err != nil {
		return TransactionView{}, err
	}
	v := TransactionView{Transaction: t, Postings: ps}
	for _, tg := range tags {
		v.Tags = append(v.Tags, tg.Name)
	}
	return v, nil
}

// IgnoreRow drops a row from the queue; it stays on record, so the same
// source id is never staged again.
func (s *Service) IgnoreRow(ctx context.Context, a Access, rowID int64) error {
	if err := a.require(db.MemberRoleEditor); err != nil {
		return err
	}
	// The row is looked up in this book first: DecideRow knows only ids.
	if _, err := s.store.GetImportRow(ctx, db.GetImportRowParams{BookID: a.Book.ID, ID: rowID}); err != nil {
		return translate(err, "import row")
	}
	n, err := s.store.DecideRow(ctx, db.DecideRowParams{ID: rowID, Status: "ignored", DecidedBy: &a.UserID})
	if err != nil {
		return err
	}
	if n == 0 {
		return conflict("already_decided", "this row was already accepted or ignored")
	}
	// What waited for it (its duplicate, an invoice) is matched again.
	ids, err := s.store.RowsMatchingRow(ctx, db.RowsMatchingRowParams{BookID: a.Book.ID, RowID: &rowID})
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

// ignoreSource ignores a set-aside source account's waiting rows, as the
// person, and matches again what waited for them.
func (s *Service) ignoreSource(ctx context.Context, a Access, sourceID int64) error {
	ids, err := s.store.PendingRowsOfSource(ctx, sourceID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		n, err := s.store.DecideRow(ctx, db.DecideRowParams{ID: id, Status: "ignored", DecidedBy: &a.UserID})
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}
		waiting, err := s.store.RowsMatchingRow(ctx, db.RowsMatchingRowParams{BookID: a.Book.ID, RowID: &id})
		if err != nil {
			return err
		}
		for _, w := range waiting {
			if _, err := s.propose(ctx, a, w); err != nil {
				return err
			}
		}
	}
	return nil
}

// settleDuplicates decides the waiting rows that were the same charge as an
// accepted row: they are that row's transaction.
func settleDuplicates(ctx context.Context, q *db.Queries, a Access, rowID, txnID int64) error {
	ids, err := q.RowsMatchingRow(ctx, db.RowsMatchingRowParams{BookID: a.Book.ID, RowID: &rowID})
	if err != nil {
		return err
	}
	for _, id := range ids {
		o, err := q.GetImportRow(ctx, db.GetImportRowParams{BookID: a.Book.ID, ID: id})
		if err != nil {
			return err
		}
		if o.Kind != "transaction" && o.Kind != "trade" || o.Proposal != "duplicate" {
			continue // an invoice waiting for it is matched again afterwards
		}
		if _, err := q.DecideRow(ctx, db.DecideRowParams{ID: id, Status: "accepted", TransactionID: &txnID, DecidedBy: &a.UserID}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Queue(ctx context.Context, a Access) ([]db.ListQueueRow, error) {
	rows, err := s.store.ListQueue(ctx, db.ListQueueParams{BookID: a.Book.ID, Lim: 500})
	if err != nil {
		return nil, err
	}
	// Time alone changes a waiting invoice: a week on, it is a cash purchase.
	if aged, err := s.matchAgedInvoices(ctx, a, rows); err != nil || !aged {
		return rows, err
	}
	return s.store.ListQueue(ctx, db.ListQueueParams{BookID: a.Book.ID, Lim: 500})
}

func (s *Service) SourceAccounts(ctx context.Context, a Access) ([]db.ListSourceAccountsRow, error) {
	return s.store.ListSourceAccounts(ctx, a.Book.ID)
}

// CreateRule adds "text containing pattern goes to account", then re-runs
// the matcher on waiting rows that have no category yet.
func (s *Service) CreateRule(ctx context.Context, a Access, pattern string, accountID int64, sourceID *int64) (db.ImportRule, error) {
	if err := a.require(db.MemberRoleEditor); err != nil {
		return db.ImportRule{}, err
	}
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return db.ImportRule{}, fieldError("pattern", "required", "a rule needs some text to match")
	}
	acc, err := s.store.GetAccount(ctx, db.GetAccountParams{BookID: a.Book.ID, ID: accountID})
	if err != nil || acc.IsPlaceholder {
		return db.ImportRule{}, fieldError("account_id", "invalid", "choose an account that takes postings")
	}
	rule, err := s.store.CreateRule(ctx, db.CreateRuleParams{BookID: a.Book.ID, Priority: 100, Pattern: pattern,
		SourceAccountID: sourceID, AccountID: accountID, CreatedBy: &a.UserID})
	if err != nil {
		return db.ImportRule{}, translate(err, "rule")
	}
	queue, err := s.Queue(ctx, a)
	if err != nil {
		return rule, err
	}
	for _, r := range queue {
		if r.Proposal == "new" && r.ProposedAccountID == nil {
			if _, err := s.propose(ctx, a, r.ID); err != nil {
				return rule, err
			}
		}
	}
	return rule, nil
}

func (s *Service) Rules(ctx context.Context, a Access) ([]db.ImportRule, error) {
	return s.store.ListRules(ctx, a.Book.ID)
}

func (s *Service) DeleteRule(ctx context.Context, a Access, id int64) error {
	if err := a.require(db.MemberRoleEditor); err != nil {
		return err
	}
	n, err := s.store.DeleteRule(ctx, db.DeleteRuleParams{BookID: a.Book.ID, ID: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return notFound("rule")
	}
	return nil
}

// Drift is an account whose latest stated balance differs from the books on
// that day. Since is the newest earlier assertion that still agreed (nil
// when none did) and First the one right after it: the gap opened between
// the two, which is where a statement may be missing.
type Drift struct {
	AccountID int64
	Date      time.Time
	Asserted  decimal.Decimal
	Booked    decimal.Decimal
	Source    string
	Since     *time.Time
	First     time.Time
}

// Drifts lists every account whose newest assertion does not agree with the
// books. Nothing is changed: finding the missing or wrong entry is the
// person's job (or booking the difference, BookDifference), and the dates
// say where to look.
func (s *Service) Drifts(ctx context.Context, a Access) ([]Drift, error) {
	rows, err := s.store.AccountAssertions(ctx, a.Book.ID)
	if err != nil {
		return nil, err
	}
	var out []Drift
	for i := 0; i < len(rows); {
		j := i
		for j < len(rows) && rows[j].AccountID == rows[i].AccountID {
			j++
		}
		if d, ok := drift(rows[i:j]); ok {
			out = append(out, d)
		}
		i = j
	}
	return out, nil
}

// drift reads one account's assertions, oldest first.
func drift(rows []db.AccountAssertionsRow) (Drift, bool) {
	last := rows[len(rows)-1]
	if last.Asserted.Equal(last.Booked) {
		return Drift{}, false
	}
	d := Drift{AccountID: last.AccountID, Date: last.Date, Asserted: last.Asserted, Booked: last.Booked, Source: last.Source, First: rows[0].Date}
	for k := len(rows) - 2; k >= 0; k-- {
		if rows[k].Asserted.Equal(rows[k].Booked) {
			since := rows[k].Date
			d.Since, d.First = &since, rows[k+1].Date
			break
		}
	}
	return d, true
}

// Balance is an account's stated balance on a day beside the books'.
type Balance struct {
	Asserted decimal.Decimal
	Booked   decimal.Decimal
}

// SetBalance records what an account really held at the end of a day (a
// count of the cash in a wallet, a balance read off a statement), as an
// assertion of its own source, next to any institution's.
func (s *Service) SetBalance(ctx context.Context, a Access, accountID int64, on time.Time, amount decimal.Decimal) (Balance, error) {
	if err := a.require(db.MemberRoleEditor); err != nil {
		return Balance{}, err
	}
	acc, err := s.balanceAccount(ctx, a, accountID)
	if err != nil {
		return Balance{}, err
	}
	if on.IsZero() {
		return Balance{}, fieldError("date", "required", "a balance needs a date")
	}
	err = s.store.WithTx(ctx, a.UserID, func(q *db.Queries) error {
		return q.UpsertAssertion(ctx, db.UpsertAssertionParams{BookID: a.Book.ID, AccountID: acc.ID, Date: on, Amount: amount, Source: "manual"})
	})
	if err != nil {
		return Balance{}, translate(err, "balance")
	}
	booked, err := s.store.BookedOn(ctx, db.BookedOnParams{BookID: a.Book.ID, AccountID: acc.ID, Date: on})
	return Balance{Asserted: amount, Booked: booked}, err
}

// BookDifference books what separates the books from an account's stated
// balance on a day as one adjustment against counter: cash spent that no
// source saw ("other expenses"), or history before the books began
// (opening balances).
func (s *Service) BookDifference(ctx context.Context, a Access, accountID int64, on time.Time, counterID int64) (TransactionView, error) {
	if err := a.require(db.MemberRoleEditor); err != nil {
		return TransactionView{}, err
	}
	acc, err := s.balanceAccount(ctx, a, accountID)
	if err != nil {
		return TransactionView{}, err
	}
	if acc.Commodity == nil || s.validCurrency(ctx, *acc.Commodity) != nil {
		return TransactionView{}, fieldError("account_id", "invalid", "only money is adjusted; shares and points need their cost")
	}
	counter, err := s.store.GetAccount(ctx, db.GetAccountParams{BookID: a.Book.ID, ID: counterID})
	if err != nil || counter.IsPlaceholder || counter.ArchivedAt != nil || counter.ID == acc.ID {
		return TransactionView{}, fieldError("counter_id", "invalid", "choose another account that takes postings")
	}
	asserted, err := s.store.AssertionOn(ctx, db.AssertionOnParams{BookID: a.Book.ID, AccountID: acc.ID, Date: on})
	if errors.Is(err, pgx.ErrNoRows) {
		return TransactionView{}, fieldError("date", "no_balance", "no balance is known for that day")
	}
	if err != nil {
		return TransactionView{}, err
	}
	booked, err := s.store.BookedOn(ctx, db.BookedOnParams{BookID: a.Book.ID, AccountID: acc.ID, Date: on})
	if err != nil {
		return TransactionView{}, err
	}
	diff := asserted.Sub(booked)
	if diff.IsZero() {
		return TransactionView{}, invalid("nothing_to_adjust", "the books already agree on that day")
	}
	cur := *acc.Commodity
	return s.CreateTransaction(ctx, a, TransactionInput{
		Date:   on,
		Source: "adjustment",
		Lines: []LineInput{
			{AccountID: acc.ID, Commodity: cur, Amount: diff},
			{AccountID: counter.ID, Commodity: cur, Amount: diff.Neg()},
		},
	})
}

// balanceAccount is an account of the book that can hold a balance.
func (s *Service) balanceAccount(ctx context.Context, a Access, id int64) (db.Account, error) {
	acc, err := s.store.GetAccount(ctx, db.GetAccountParams{BookID: a.Book.ID, ID: id})
	if err != nil || acc.IsPlaceholder || acc.Commodity == nil {
		return db.Account{}, fieldError("account_id", "invalid", "choose an account that holds a balance")
	}
	return acc, nil
}

// baseValue is an amount's value in the book's base on a day: itself when
// it is the base, the other side of an exchange when that one is, else at
// the day's rate.
func (s *Service) baseValue(ctx context.Context, q *db.Queries, a Access, cur string, amount decimal.Decimal, on time.Time,
	otherCur string, otherAmount decimal.Decimal) (decimal.Decimal, error) {
	base := a.Book.BaseCurrency
	switch base {
	case cur:
		return amount, nil
	case otherCur:
		return otherAmount.Neg(), nil
	}
	rate, ok, err := RateOn(ctx, q, cur, base, on)
	if err != nil {
		return decimal.Zero, err
	}
	if !ok {
		return decimal.Zero, fieldError("base_amount", "rate_missing", "no exchange rate from %s to %s on or before %s", cur, base, on.Format(time.DateOnly))
	}
	decs, err := s.commodityDecimals(ctx)
	if err != nil {
		return decimal.Zero, err
	}
	return amount.Mul(rate).Round(decs[base]), nil
}
