package ledger

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

type TransactionInput struct {
	Date  time.Time
	Payee string
	Memo  string
	Lines []LineInput
	Tags  []string
	// Set by imports, never by the API: "import" or "sync", and the
	// source's id, which the (book, source, external_id) key keeps unique.
	Source     string
	ExternalID *string
}

// TransactionView is a transaction with its postings and tag names.
type TransactionView struct {
	db.Transaction
	Postings []db.Posting
	Tags     []string
}

func checkLock(book db.Book, dates ...time.Time) error {
	if book.LockDate == nil {
		return nil
	}
	for _, d := range dates {
		if !d.After(*book.LockDate) {
			return invalid("book_locked", "the book is locked up to %s", book.LockDate.Format(time.DateOnly))
		}
	}
	return nil
}

func normaliseTags(tags []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" || seen[strings.ToLower(t)] {
			continue
		}
		seen[strings.ToLower(t)] = true
		out = append(out, t)
	}
	return out
}

func (s *Service) writeLinesAndTags(ctx context.Context, q *db.Queries, a Access, txnID int64, in TransactionInput) error {
	lc, err := s.newLineContext(ctx, q, a.Book, in.Date)
	if err != nil {
		return err
	}
	lc.excludeTxn = txnID
	lines, err := s.prepareLines(ctx, q, lc, in.Lines)
	if err != nil {
		return err
	}
	for _, p := range lines {
		p.TransactionID = txnID
		if _, err := q.CreatePosting(ctx, p); err != nil {
			return err
		}
	}
	if err := q.ClearTransactionTags(ctx, txnID); err != nil {
		return err
	}
	for _, name := range normaliseTags(in.Tags) {
		tag, err := q.UpsertTag(ctx, db.UpsertTagParams{BookID: a.Book.ID, Name: name})
		if err != nil {
			return err
		}
		if err := q.AddTransactionTag(ctx, db.AddTransactionTagParams{TransactionID: txnID, TagID: tag.ID}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) CreateTransaction(ctx context.Context, a Access, in TransactionInput) (TransactionView, error) {
	if err := a.require(db.MemberRoleEditor); err != nil {
		return TransactionView{}, err
	}
	if in.Date.IsZero() {
		return TransactionView{}, fieldError("date", "required", "a transaction needs a date")
	}
	if err := checkLock(a.Book, in.Date); err != nil {
		return TransactionView{}, err
	}
	var id int64
	err := s.store.WithTx(ctx, a.UserID, func(q *db.Queries) error {
		var err error
		id, err = s.createTx(ctx, q, a, in)
		return err
	})
	if err != nil {
		return TransactionView{}, translate(err, "transaction")
	}
	return s.GetTransaction(ctx, a, id)
}

// createTx writes a transaction inside the caller's database transaction,
// so an import can create it and mark its row accepted atomically.
func (s *Service) createTx(ctx context.Context, q *db.Queries, a Access, in TransactionInput) (int64, error) {
	source := in.Source
	if source == "" {
		source = "manual"
	}
	t, err := q.CreateTransaction(ctx, db.CreateTransactionParams{
		BookID:     a.Book.ID,
		Date:       in.Date,
		Payee:      strings.TrimSpace(in.Payee),
		Memo:       strings.TrimSpace(in.Memo),
		Source:     source,
		ExternalID: in.ExternalID,
		UserID:     &a.UserID,
	})
	if err != nil {
		return 0, err
	}
	return t.ID, s.writeLinesAndTags(ctx, q, a, t.ID, in)
}

// UpdateTransaction replaces the header, every posting and the tags.
func (s *Service) UpdateTransaction(ctx context.Context, a Access, id int64, in TransactionInput) (TransactionView, error) {
	if err := a.require(db.MemberRoleEditor); err != nil {
		return TransactionView{}, err
	}
	if in.Date.IsZero() {
		return TransactionView{}, fieldError("date", "required", "a transaction needs a date")
	}
	err := s.store.WithTx(ctx, a.UserID, func(q *db.Queries) error {
		return s.updateTx(ctx, q, a, id, in)
	})
	if err != nil {
		return TransactionView{}, translate(err, "transaction")
	}
	return s.GetTransaction(ctx, a, id)
}

// updateTx replaces a transaction inside the caller's database transaction.
func (s *Service) updateTx(ctx context.Context, q *db.Queries, a Access, id int64, in TransactionInput) error {
	cur, err := q.GetTransaction(ctx, db.GetTransactionParams{BookID: a.Book.ID, ID: id})
	if err != nil {
		return translate(err, "transaction")
	}
	if err := checkLock(a.Book, cur.Date, in.Date); err != nil {
		return err
	}
	if _, err := q.UpdateTransaction(ctx, db.UpdateTransactionParams{
		BookID: a.Book.ID, ID: id, Date: in.Date,
		Payee: strings.TrimSpace(in.Payee), Memo: strings.TrimSpace(in.Memo), UserID: &a.UserID,
	}); err != nil {
		return err
	}
	if err := q.DeletePostings(ctx, id); err != nil {
		return err
	}
	return s.writeLinesAndTags(ctx, q, a, id, in)
}

func (s *Service) DeleteTransaction(ctx context.Context, a Access, id int64) error {
	if err := a.require(db.MemberRoleEditor); err != nil {
		return err
	}
	err := s.store.WithTx(ctx, a.UserID, func(q *db.Queries) error {
		cur, err := q.GetTransaction(ctx, db.GetTransactionParams{BookID: a.Book.ID, ID: id})
		if err != nil {
			return translate(err, "transaction")
		}
		if err := checkLock(a.Book, cur.Date); err != nil {
			return err
		}
		_, err = q.DeleteTransaction(ctx, db.DeleteTransactionParams{BookID: a.Book.ID, ID: id})
		return err
	})
	return translate(err, "transaction")
}

func (s *Service) GetTransaction(ctx context.Context, a Access, id int64) (TransactionView, error) {
	t, err := s.store.GetTransaction(ctx, db.GetTransactionParams{BookID: a.Book.ID, ID: id})
	if err != nil {
		return TransactionView{}, translate(err, "transaction")
	}
	views, err := s.attach(ctx, []db.Transaction{t})
	if err != nil {
		return TransactionView{}, err
	}
	return views[0], nil
}

func (s *Service) attach(ctx context.Context, ts []db.Transaction) ([]TransactionView, error) {
	ids := make([]int64, len(ts))
	for i, t := range ts {
		ids[i] = t.ID
	}
	postings, err := s.store.ListPostings(ctx, ids)
	if err != nil {
		return nil, err
	}
	tags, err := s.store.ListTransactionTags(ctx, ids)
	if err != nil {
		return nil, err
	}
	byTxn := map[int64][]db.Posting{}
	for _, p := range postings {
		byTxn[p.TransactionID] = append(byTxn[p.TransactionID], p)
	}
	tagsByTxn := map[int64][]string{}
	for _, t := range tags {
		tagsByTxn[t.TransactionID] = append(tagsByTxn[t.TransactionID], t.Name)
	}
	out := make([]TransactionView, len(ts))
	for i, t := range ts {
		out[i] = TransactionView{Transaction: t, Postings: byTxn[t.ID], Tags: tagsByTxn[t.ID]}
		if out[i].Postings == nil {
			out[i].Postings = []db.Posting{}
		}
		if out[i].Tags == nil {
			out[i].Tags = []string{}
		}
	}
	return out, nil
}

type ListFilter struct {
	From      *time.Time
	To        *time.Time
	AccountID *int64 // includes the account's descendants
	Query     string
	Tag       string
	Cursor    string
	Limit     int
}

const (
	defaultPageSize = 50
	maxPageSize     = 200
)

// Cursor is "<date>_<id>" of the last row of the previous page.
func encodeCursor(t db.Transaction) string {
	return t.Date.Format(time.DateOnly) + "_" + strconv.FormatInt(t.ID, 10)
}

func decodeCursor(c string) (*time.Time, *int64, error) {
	if c == "" {
		return nil, nil, nil
	}
	d, idStr, ok := strings.Cut(c, "_")
	if !ok {
		return nil, nil, fieldError("cursor", "invalid", "malformed cursor")
	}
	date, err := time.Parse(time.DateOnly, d)
	if err != nil {
		return nil, nil, fieldError("cursor", "invalid", "malformed cursor")
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return nil, nil, fieldError("cursor", "invalid", "malformed cursor")
	}
	return &date, &id, nil
}

func descendants(accounts []db.Account, root int64) []int64 {
	children := map[int64][]int64{}
	for _, a := range accounts {
		if a.ParentID != nil {
			children[*a.ParentID] = append(children[*a.ParentID], a.ID)
		}
	}
	out := []int64{root}
	for i := 0; i < len(out); i++ {
		out = append(out, children[out[i]]...)
	}
	return out
}

// ListTransactions returns one page, newest first, and the cursor for the
// next page ("" when this is the last).
func (s *Service) ListTransactions(ctx context.Context, a Access, f ListFilter) ([]TransactionView, string, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	cursorDate, cursorID, err := decodeCursor(f.Cursor)
	if err != nil {
		return nil, "", err
	}
	p := db.ListTransactionsParams{
		BookID:     a.Book.ID,
		FromDate:   f.From,
		ToDate:     f.To,
		AccountIds: []int64{},
		CursorDate: cursorDate,
		CursorID:   cursorID,
		Lim:        int32(limit + 1),
	}
	if f.AccountID != nil {
		accts, err := s.store.ListAccounts(ctx, a.Book.ID)
		if err != nil {
			return nil, "", err
		}
		p.AccountIds = descendants(accts, *f.AccountID)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		p.Q = &q
	}
	if tag := strings.TrimSpace(f.Tag); tag != "" {
		tags, err := s.store.ListTags(ctx, a.Book.ID)
		if err != nil {
			return nil, "", err
		}
		var id int64 = -1 // an unknown tag matches nothing
		for _, t := range tags {
			if strings.EqualFold(t.Name, tag) {
				id = t.ID
			}
		}
		p.TagID = &id
	}

	rows, err := s.store.ListTransactions(ctx, p)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = encodeCursor(rows[len(rows)-1])
	}
	views, err := s.attach(ctx, rows)
	if err != nil {
		return nil, "", err
	}
	return views, next, nil
}

func (s *Service) ListTags(ctx context.Context, a Access) ([]db.Tag, error) {
	return s.store.ListTags(ctx, a.Book.ID)
}

// String helps error messages and logs.
func (v TransactionView) String() string {
	return fmt.Sprintf("transaction %d (%s)", v.ID, v.Date.Format(time.DateOnly))
}
