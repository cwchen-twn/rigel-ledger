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
}

type ImportRowInput struct {
	Kind         string // transaction | balance
	Account      string // ImportAccount.ExternalID
	ID           string // the source's own id; with the connector, the dedupe key
	Date         time.Time
	Amount       decimal.Decimal // debit > 0 on the account (money in, a card payment)
	Currency     string
	Description  string
	Counterparty string
	Pending      bool
	Raw          json.RawMessage
}

type ImportInput struct {
	Connector string
	Label     string
	Accounts  []ImportAccount
	Rows      []ImportRowInput
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
			sa, err := q.UpsertSourceAccount(ctx, db.UpsertSourceAccountParams{
				BookID: a.Book.ID, Connector: in.Connector, ExternalID: ext, Label: strings.TrimSpace(acc.Label), Currency: cur,
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
		for i, r := range in.Rows {
			sa, ok := sources[r.Account]
			if !ok {
				return fieldError(idx("rows", i, "account"), "unknown", "account %q is not in accounts", r.Account)
			}
			if r.Kind != "transaction" && r.Kind != "balance" {
				return fieldError(idx("rows", i, "kind"), "invalid", "kind is transaction or balance")
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
			if _, ok := commods[cur]; !ok {
				return fieldError(idx("rows", i, "currency"), "unknown", "unknown currency %q", cur)
			}
			raw := []byte(r.Raw)
			if len(raw) == 0 {
				raw = []byte("{}")
			}
			id, err := q.InsertImportRow(ctx, db.InsertImportRowParams{
				BookID: a.Book.ID, BatchID: batch.ID, SourceAccountID: sa.ID, Kind: r.Kind,
				ExternalID: in.Connector + ":" + strings.TrimSpace(r.ID), Date: r.Date, Amount: r.Amount, Currency: cur,
				Description: strings.TrimSpace(r.Description), Counterparty: strings.TrimSpace(r.Counterparty),
				Pending: r.Pending, Raw: raw,
			})
			if errors.Is(err, pgx.ErrNoRows) {
				res.Duplicates++
				continue
			}
			if err != nil {
				return err
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
	return res, nil
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
	if r.Status != "pending" || r.AccountID == nil {
		return false, nil // unmapped: waits for its source account to be mapped
	}
	acct := *r.AccountID
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
	// 3. One side of a transfer between two of the book's accounts.
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
	// 4. New: the first rule whose pattern the text contains.
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

// MapSourceAccount ties a source account to a ledger account and re-runs
// the matcher on its waiting rows.
func (s *Service) MapSourceAccount(ctx context.Context, a Access, sourceID int64, accountID *int64) (db.SourceAccount, error) {
	if err := a.require(db.MemberRoleEditor); err != nil {
		return db.SourceAccount{}, err
	}
	sa, err := s.store.GetSourceAccount(ctx, db.GetSourceAccountParams{BookID: a.Book.ID, ID: sourceID})
	if err != nil {
		return db.SourceAccount{}, translate(err, "source account")
	}
	if accountID != nil {
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
	sa, err = s.store.MapSourceAccount(ctx, db.MapSourceAccountParams{BookID: a.Book.ID, ID: sourceID, AccountID: accountID})
	if err != nil {
		return db.SourceAccount{}, translate(err, "source account")
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
	if r.AccountID == nil {
		return 0, invalid("unmapped", "map the row's source account to an account first")
	}
	if r.Kind != "transaction" {
		return 0, invalid("invalid_input", "balance rows are applied when their account is mapped")
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
	// What it matched was deleted since (the FK set it to NULL): match again.
	if (proposal == "duplicate" || proposal == "clears") && r.MatchTransactionID == nil ||
		proposal == "transfer" && r.MatchRowID == nil {
		if _, err := s.propose(ctx, a, r.ID); err != nil {
			return 0, err
		}
		return 0, conflict("match_gone", "what this row matched is gone; it was matched again")
	}

	var txnID int64
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
			txnID, err = s.createTx(ctx, q, a, TransactionInput{
				Date: minDate(r.Date, other.Date), Payee: payee, Memo: memo, Source: source, ExternalID: &ext,
				Lines: []LineInput{
					{AccountID: acct, Commodity: r.Currency, Amount: r.Amount, Status: status, ClearedOn: clearedOn(status, r.Date)},
					{AccountID: *other.AccountID, Commodity: other.Currency, Amount: other.Amount, Status: db.PostingStatusCleared, ClearedOn: &other.Date},
				},
			})
			if err != nil {
				return err
			}
			if _, err := q.DecideRow(ctx, db.DecideRowParams{ID: other.ID, Status: "accepted", TransactionID: &txnID, DecidedBy: &a.UserID}); err != nil {
				return err
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
		n, err := q.DecideRow(ctx, db.DecideRowParams{ID: r.ID, Status: "accepted", TransactionID: &txnID, DecidedBy: &a.UserID})
		if err == nil && n == 0 {
			return conflict("already_decided", "this row was already accepted or ignored")
		}
		return err
	})
	if err != nil {
		return 0, translate(err, "import row")
	}
	return txnID, nil
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
	n, err := s.store.DecideRow(ctx, db.DecideRowParams{ID: rowID, Status: "ignored", DecidedBy: &a.UserID})
	if err != nil {
		return err
	}
	if n == 0 {
		return conflict("already_decided", "this row was already accepted or ignored")
	}
	return nil
}

func (s *Service) Queue(ctx context.Context, a Access) ([]db.ListQueueRow, error) {
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

// Drift is an account whose institution's latest balance differs from the
// books on that day.
type Drift struct {
	AccountID int64
	Date      time.Time
	Asserted  decimal.Decimal
	Booked    decimal.Decimal
	Source    string
}

// Drifts lists every account's newest assertion that does not agree with
// the books. Nothing is changed: finding the missing or wrong entry is the
// person's job, and the date says where to look.
func (s *Service) Drifts(ctx context.Context, a Access) ([]Drift, error) {
	rows, err := s.store.AccountDrift(ctx, a.Book.ID)
	if err != nil {
		return nil, err
	}
	var out []Drift
	for _, r := range rows {
		if !r.Asserted.Equal(r.Booked) {
			out = append(out, Drift{AccountID: r.AccountID, Date: r.Date, Asserted: r.Asserted, Booked: r.Booked, Source: r.Source})
		}
	}
	return out, nil
}
