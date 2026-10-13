package ledger

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

// A book as one file: everything a person entered or confirmed, so a book
// kept somewhere else (a throwaway server, an older install) can start a new
// one here. These structs ARE the file format, which is why they carry JSON
// tags; change them only with a new BookFileVersion.
//
// Ids in the file are the exporting database's and only link the file
// together; an import never trusts them, it remaps every one. Attachments,
// the import staging rows, members and connections are not in the file.
const (
	BookFileFormat  = "rigelledger-book"
	BookFileVersion = 1
)

type BookFile struct {
	Format     string    `json:"format"`
	Version    int       `json:"version"`
	ExportedAt time.Time `json:"exported_at"`

	Book           FileBook            `json:"book"`
	Commodities    []FileCommodity     `json:"commodities"`
	Accounts       []FileAccount       `json:"accounts"`
	Transactions   []FileTransaction   `json:"transactions"`
	Assertions     []FileAssertion     `json:"balance_assertions"`
	Prices         []FilePrice         `json:"prices"`
	SourceAccounts []FileSourceAccount `json:"source_accounts"`
	Rules          []FileRule          `json:"rules"`
	// What the file leaves out, counted, so a person knows.
	Omitted map[string]int64 `json:"omitted,omitempty"`
}

type FileBook struct {
	Name                    string     `json:"name"`
	BaseCurrency            string     `json:"base_currency"`
	LockDate                *string    `json:"lock_date,omitempty"`
	InterestDividendCfClass db.CfClass `json:"interest_dividend_cf_class"`
}

// FileCommodity is a security or points programme the book uses; ISO
// currencies are seeded everywhere and not listed.
type FileCommodity struct {
	Code          string           `json:"code"`
	Kind          db.CommodityKind `json:"kind"`
	Name          string           `json:"name"`
	Decimals      int16            `json:"decimals"`
	QuoteCurrency *string          `json:"quote_currency,omitempty"`
	ExchangeMIC   *string          `json:"exchange_mic,omitempty"`
	ContractSize  *decimal.Decimal `json:"contract_size,omitempty"`
}

type FileAccount struct {
	ID            int64           `json:"id"`
	ParentID      *int64          `json:"parent_id,omitempty"`
	Class         db.AccountClass `json:"class"`
	Name          *string         `json:"name,omitempty"`
	TemplateKey   *string         `json:"template_key,omitempty"`
	Code          *string         `json:"code,omitempty"`
	Commodity     *string         `json:"commodity,omitempty"`
	IsCurrent     bool            `json:"is_current"`
	IsCash        bool            `json:"is_cash"`
	CfClass       db.CfClass      `json:"cf_class"`
	IsPlaceholder bool            `json:"is_placeholder"`
	Archived      bool            `json:"archived,omitempty"`
}

type FileTransaction struct {
	Date       string     `json:"date"`
	Payee      string     `json:"payee"`
	Memo       string     `json:"memo,omitempty"`
	Source     string     `json:"source"`
	ExternalID *string    `json:"external_id,omitempty"`
	Tags       []string   `json:"tags,omitempty"`
	Lines      []FileLine `json:"lines"`
	Items      []FileItem `json:"items,omitempty"`
}

type FileLine struct {
	AccountID  int64            `json:"account_id"`
	Commodity  string           `json:"commodity"`
	Amount     decimal.Decimal  `json:"amount"`
	BaseAmount decimal.Decimal  `json:"base_amount"`
	UnitCost   *decimal.Decimal `json:"unit_cost,omitempty"`
	Status     db.PostingStatus `json:"status,omitempty"`
	ClearedOn  *string          `json:"cleared_on,omitempty"`
	Memo       string           `json:"memo,omitempty"`
}

type FileItem struct {
	Description string           `json:"description"`
	Quantity    *decimal.Decimal `json:"quantity,omitempty"`
	UnitPrice   *decimal.Decimal `json:"unit_price,omitempty"`
	Amount      decimal.Decimal  `json:"amount"`
	AccountID   *int64           `json:"account_id,omitempty"`
	Source      string           `json:"source,omitempty"`
	Currency    *string          `json:"currency,omitempty"`
}

type FileAssertion struct {
	AccountID int64           `json:"account_id"`
	Date      string          `json:"date"`
	Amount    decimal.Decimal `json:"amount"`
	Source    string          `json:"source"`
}

type FilePrice struct {
	Commodity string          `json:"commodity"`
	Quote     string          `json:"quote"`
	Date      string          `json:"date"`
	Rate      decimal.Decimal `json:"rate"`
	Source    string          `json:"source"`
}

type FileSourceAccount struct {
	ID                  int64                `json:"id"`
	Connector           string               `json:"connector"`
	ExternalID          string               `json:"external_id"`
	Label               string               `json:"label,omitempty"`
	Currency            *string              `json:"currency,omitempty"`
	Kind                string               `json:"kind"`
	AccountID           *int64               `json:"account_id,omitempty"`
	SettlementAccountID *int64               `json:"settlement_account_id,omitempty"`
	Ignored             bool                 `json:"ignored,omitempty"`
	Securities          []FileSourceSecurity `json:"securities,omitempty"`
}

type FileSourceSecurity struct {
	Commodity string `json:"commodity"`
	AccountID int64  `json:"account_id"`
}

type FileRule struct {
	Priority        int32  `json:"priority"`
	Pattern         string `json:"pattern"`
	SourceAccountID *int64 `json:"source_account_id,omitempty"`
	AccountID       int64  `json:"account_id"`
}

func fileDate(t time.Time) string { return t.Format(time.DateOnly) }

func fileDatePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := fileDate(*t)
	return &s
}

func decPtr(d decimal.NullDecimal) *decimal.Decimal {
	if !d.Valid {
		return nil
	}
	return &d.Decimal
}

// ExportBook writes the whole book as a BookFile. Owner only: the file is
// everything in the book, members included would be the next step.
func (s *Service) ExportBook(ctx context.Context, a Access) (BookFile, error) {
	if err := a.require(db.MemberRoleOwner); err != nil {
		return BookFile{}, err
	}
	q := s.store.Queries
	id := a.Book.ID
	f := BookFile{
		Format: BookFileFormat, Version: BookFileVersion, ExportedAt: time.Now().UTC().Truncate(time.Second),
		Book: FileBook{Name: a.Book.Name, BaseCurrency: a.Book.BaseCurrency, LockDate: fileDatePtr(a.Book.LockDate),
			InterestDividendCfClass: a.Book.InterestDividendCfClass},
		Commodities: []FileCommodity{}, Accounts: []FileAccount{}, Transactions: []FileTransaction{},
		Assertions: []FileAssertion{}, Prices: []FilePrice{}, SourceAccounts: []FileSourceAccount{}, Rules: []FileRule{},
	}
	used := map[string]bool{a.Book.BaseCurrency: true}

	accts, err := q.ListAccounts(ctx, id)
	if err != nil {
		return BookFile{}, err
	}
	for _, acc := range accts {
		f.Accounts = append(f.Accounts, FileAccount{
			ID: acc.ID, ParentID: acc.ParentID, Class: acc.Class, Name: acc.Name, TemplateKey: acc.TemplateKey,
			Code: acc.Code, Commodity: acc.Commodity, IsCurrent: acc.IsCurrent, IsCash: acc.IsCash,
			CfClass: acc.CfClass, IsPlaceholder: acc.IsPlaceholder, Archived: acc.ArchivedAt != nil,
		})
		if acc.Commodity != nil {
			used[*acc.Commodity] = true
		}
	}

	txns, err := q.ExportTransactions(ctx, id)
	if err != nil {
		return BookFile{}, err
	}
	posts, err := q.ExportPostings(ctx, id)
	if err != nil {
		return BookFile{}, err
	}
	tags, err := q.ExportTransactionTags(ctx, id)
	if err != nil {
		return BookFile{}, err
	}
	items, err := q.ExportItems(ctx, id)
	if err != nil {
		return BookFile{}, err
	}
	lines := map[int64][]FileLine{}
	for _, p := range posts {
		lines[p.TransactionID] = append(lines[p.TransactionID], FileLine{
			AccountID: p.AccountID, Commodity: p.Commodity, Amount: p.Amount, BaseAmount: p.BaseAmount,
			UnitCost: decPtr(p.UnitCost), Status: p.Status, ClearedOn: fileDatePtr(p.ClearedOn), Memo: p.Memo,
		})
		used[p.Commodity] = true
	}
	tagsOf := map[int64][]string{}
	for _, t := range tags {
		tagsOf[t.TransactionID] = append(tagsOf[t.TransactionID], t.Name)
	}
	itemsOf := map[int64][]FileItem{}
	for _, it := range items {
		itemsOf[it.TransactionID] = append(itemsOf[it.TransactionID], FileItem{
			Description: it.Description, Quantity: decPtr(it.Quantity), UnitPrice: decPtr(it.UnitPrice),
			Amount: it.Amount, AccountID: it.AccountID, Source: it.Source, Currency: it.Currency,
		})
		if it.Currency != nil {
			used[*it.Currency] = true
		}
	}
	for _, t := range txns {
		f.Transactions = append(f.Transactions, FileTransaction{
			Date: fileDate(t.Date), Payee: t.Payee, Memo: t.Memo, Source: t.Source, ExternalID: t.ExternalID,
			Tags: tagsOf[t.ID], Lines: lines[t.ID], Items: itemsOf[t.ID],
		})
	}

	asserts, err := q.ExportAssertions(ctx, id)
	if err != nil {
		return BookFile{}, err
	}
	for _, b := range asserts {
		f.Assertions = append(f.Assertions, FileAssertion{AccountID: b.AccountID, Date: fileDate(b.Date), Amount: b.Amount, Source: b.Source})
	}

	sources, err := q.ListSourceAccounts(ctx, id)
	if err != nil {
		return BookFile{}, err
	}
	secs, err := q.ExportSourceSecurities(ctx, id)
	if err != nil {
		return BookFile{}, err
	}
	secsOf := map[int64][]FileSourceSecurity{}
	for _, ss := range secs {
		secsOf[ss.SourceAccountID] = append(secsOf[ss.SourceAccountID], FileSourceSecurity{Commodity: ss.Commodity, AccountID: ss.AccountID})
		used[ss.Commodity] = true
	}
	for _, sa := range sources {
		f.SourceAccounts = append(f.SourceAccounts, FileSourceAccount{
			ID: sa.ID, Connector: sa.Connector, ExternalID: sa.ExternalID, Label: sa.Label, Currency: sa.Currency,
			Kind: sa.Kind, AccountID: sa.AccountID, SettlementAccountID: sa.SettlementAccountID, Ignored: sa.Ignored,
			Securities: secsOf[sa.ID],
		})
		if sa.Currency != nil {
			used[*sa.Currency] = true
		}
	}
	rules, err := q.ListRules(ctx, id)
	if err != nil {
		return BookFile{}, err
	}
	for _, r := range rules {
		f.Rules = append(f.Rules, FileRule{Priority: r.Priority, Pattern: r.Pattern, SourceAccountID: r.SourceAccountID, AccountID: r.AccountID})
	}

	comms, err := s.commodityMap(ctx)
	if err != nil {
		return BookFile{}, err
	}
	codes := make([]string, 0, len(used))
	for c := range used {
		codes = append(codes, c)
		// A security's quote currency is what its price is in.
		if q := comms[c].QuoteCurrency; q != nil && !used[*q] {
			codes = append(codes, *q)
		}
	}
	sort.Strings(codes)
	for _, c := range codes {
		com, ok := comms[c]
		if !ok || com.Kind == db.CommodityKindCurrency {
			continue
		}
		f.Commodities = append(f.Commodities, FileCommodity{
			Code: com.Code, Kind: com.Kind, Name: com.Name, Decimals: com.Decimals, QuoteCurrency: com.QuoteCurrency,
			ExchangeMIC: com.ExchangeMic, ContractSize: decPtr(com.ContractSize),
		})
	}
	prices, err := q.ExportPrices(ctx, codes)
	if err != nil {
		return BookFile{}, err
	}
	for _, p := range prices {
		f.Prices = append(f.Prices, FilePrice{Commodity: p.Commodity, Quote: p.Quote, Date: fileDate(p.Date), Rate: p.Rate, Source: p.Source})
	}

	att, err := s.countBookAttachments(ctx, id)
	if err != nil {
		return BookFile{}, err
	}
	if att > 0 {
		f.Omitted = map[string]int64{"attachments": att}
	}
	return f, nil
}

func (s *Service) countBookAttachments(ctx context.Context, bookID int64) (int64, error) {
	var n int64
	err := s.store.Pool.QueryRow(ctx, `SELECT count(*) FROM attachments WHERE book_id = $1`, bookID).Scan(&n)
	return n, err
}

// BookImport is what importing a book file made.
type BookImport struct {
	Book         db.Book
	Accounts     int
	Transactions int
	Assertions   int
	// Prices added: only days with no rate at all for the pair. Others in
	// the file were already known (or frozen by a locked book) and skipped.
	PricesAdded   int64
	PricesSkipped int
}

func parseFileDate(field, s string) (time.Time, error) {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return time.Time{}, fieldError(field, "invalid", "%q is not a YYYY-MM-DD date", s)
	}
	return t, nil
}

func fileError(code, format string, args ...any) error {
	return &Error{Kind: KindInvalid, Code: code, Message: fmt.Sprintf(format, args...), Fields: map[string]string{"file": code}}
}

// ImportBook creates a NEW book, owned by the caller, from a BookFile. The
// book and everything in it are written in one database transaction, so a
// file that fails anywhere leaves nothing behind. Every transaction goes
// through the same line rules as one entered by hand (prepareLines and the
// triggers) with the file's base amounts, so a file that does not balance is
// refused, not repaired. name, when not empty, replaces the file's book name.
func (s *Service) ImportBook(ctx context.Context, userID int64, f BookFile, name string) (BookImport, error) {
	if f.Format != BookFileFormat {
		return BookImport{}, fileError("file_format", "not a RigelLedger book file")
	}
	if f.Version != BookFileVersion {
		return BookImport{}, fileError("file_version", "book file version %d; this app reads version %d", f.Version, BookFileVersion)
	}
	if name = strings.TrimSpace(name); name == "" {
		name = strings.TrimSpace(f.Book.Name)
	}
	if name == "" {
		return BookImport{}, fieldError("name", "required", "a book needs a name")
	}
	base := strings.ToUpper(strings.TrimSpace(f.Book.BaseCurrency))
	if err := s.validCurrency(ctx, base); err != nil {
		return BookImport{}, err
	}
	cf := f.Book.InterestDividendCfClass
	if cf == "" {
		cf = db.CfClassOperating
	}
	if !cf.Valid() {
		return BookImport{}, fieldError("book.interest_dividend_cf_class", "invalid", "unknown cash-flow class %q", cf)
	}
	var lock *time.Time
	if f.Book.LockDate != nil {
		t, err := parseFileDate("book.lock_date", *f.Book.LockDate)
		if err != nil {
			return BookImport{}, err
		}
		lock = &t
	}

	// Commodities first, outside the book's transaction: they are global,
	// any editor may add one, and the line rules read them from the cache,
	// which cannot see an uncommitted row.
	if err := s.importCommodities(ctx, f.Commodities); err != nil {
		return BookImport{}, err
	}

	res := BookImport{}
	err := s.store.WithTx(ctx, userID, func(q *db.Queries) error {
		book, err := q.CreateBook(ctx, db.CreateBookParams{Name: name, BaseCurrency: base, CreatedBy: &userID})
		if err != nil {
			return err
		}
		if err := q.AddMember(ctx, db.AddMemberParams{BookID: book.ID, UserID: userID, Role: db.MemberRoleOwner}); err != nil {
			return err
		}

		ids, archived, err := importAccounts(ctx, q, book.ID, f.Accounts)
		if err != nil {
			return err
		}
		res.Accounts = len(ids)
		acct := func(field string, id int64) (int64, error) {
			n, ok := ids[id]
			if !ok {
				return 0, fieldError(field, "not_found", "account %d is not in the file", id)
			}
			return n, nil
		}

		lc, err := s.newLineContext(ctx, q, book, time.Time{})
		if err != nil {
			return err
		}
		tagIDs := map[string]int64{}
		for i, ft := range f.Transactions {
			field := func(s string) string { return fmt.Sprintf("transactions[%d].%s", i, s) }
			date, err := parseFileDate(field("date"), ft.Date)
			if err != nil {
				return err
			}
			source := ft.Source
			if source == "" {
				source = "manual"
			}
			in := make([]LineInput, len(ft.Lines))
			for j, l := range ft.Lines {
				id, err := acct(field(fmt.Sprintf("lines[%d].account_id", j)), l.AccountID)
				if err != nil {
					return err
				}
				var cleared *time.Time
				if l.ClearedOn != nil {
					t, err := parseFileDate(field(fmt.Sprintf("lines[%d].cleared_on", j)), *l.ClearedOn)
					if err != nil {
						return err
					}
					cleared = &t
				}
				ba := l.BaseAmount
				in[j] = LineInput{AccountID: id, Commodity: l.Commodity, Amount: l.Amount, BaseAmount: &ba,
					UnitCost: l.UnitCost, Status: l.Status, ClearedOn: cleared, Memo: l.Memo}
			}
			lc.date = date
			posts, err := s.prepareLines(ctx, q, lc, in)
			if err != nil {
				return prefixFields(err, fmt.Sprintf("transactions[%d].", i))
			}
			t, err := q.CreateTransaction(ctx, db.CreateTransactionParams{
				BookID: book.ID, Date: date, Payee: strings.TrimSpace(ft.Payee), Memo: strings.TrimSpace(ft.Memo),
				Source: source, ExternalID: ft.ExternalID, UserID: &userID,
			})
			if err != nil {
				return err
			}
			for _, p := range posts {
				p.TransactionID = t.ID
				if _, err := q.CreatePosting(ctx, p); err != nil {
					return err
				}
			}
			for _, name := range normaliseTags(ft.Tags) {
				tid, ok := tagIDs[strings.ToLower(name)]
				if !ok {
					tag, err := q.UpsertTag(ctx, db.UpsertTagParams{BookID: book.ID, Name: name})
					if err != nil {
						return err
					}
					tid = tag.ID
					tagIDs[strings.ToLower(name)] = tid
				}
				if err := q.AddTransactionTag(ctx, db.AddTransactionTagParams{TransactionID: t.ID, TagID: tid}); err != nil {
					return err
				}
			}
			for k, it := range ft.Items {
				var accID *int64
				if it.AccountID != nil {
					n, err := acct(field(fmt.Sprintf("items[%d].account_id", k)), *it.AccountID)
					if err != nil {
						return err
					}
					accID = &n
				}
				if err := q.InsertTransactionItem(ctx, db.InsertTransactionItemParams{
					BookID: book.ID, TransactionID: t.ID, Position: int16(k), Description: it.Description,
					Quantity: nullDecimal(it.Quantity), UnitPrice: nullDecimal(it.UnitPrice), Amount: it.Amount,
					AccountID: accID, Source: it.Source, Currency: it.Currency,
				}); err != nil {
					return err
				}
			}
		}
		res.Transactions = len(f.Transactions)

		for i, b := range f.Assertions {
			id, err := acct(fmt.Sprintf("balance_assertions[%d].account_id", i), b.AccountID)
			if err != nil {
				return err
			}
			date, err := parseFileDate(fmt.Sprintf("balance_assertions[%d].date", i), b.Date)
			if err != nil {
				return err
			}
			if err := q.UpsertAssertion(ctx, db.UpsertAssertionParams{BookID: book.ID, AccountID: id, Date: date, Amount: b.Amount, Source: b.Source}); err != nil {
				return err
			}
		}
		res.Assertions = len(f.Assertions)

		sourceIDs := map[int64]int64{}
		for i, sa := range f.SourceAccounts {
			field := fmt.Sprintf("source_accounts[%d]", i)
			kind := sa.Kind
			if kind == "" {
				kind = "cash"
			}
			row, err := q.UpsertSourceAccount(ctx, db.UpsertSourceAccountParams{BookID: book.ID, Connector: sa.Connector,
				ExternalID: sa.ExternalID, Label: sa.Label, Currency: sa.Currency, Kind: kind})
			if err != nil {
				return err
			}
			sourceIDs[sa.ID] = row.ID
			var accID, settle *int64
			if sa.AccountID != nil {
				n, err := acct(field+".account_id", *sa.AccountID)
				if err != nil {
					return err
				}
				accID = &n
			}
			if sa.SettlementAccountID != nil {
				n, err := acct(field+".settlement_account_id", *sa.SettlementAccountID)
				if err != nil {
					return err
				}
				settle = &n
			}
			if _, err := q.MapSourceAccount(ctx, db.MapSourceAccountParams{BookID: book.ID, ID: row.ID, AccountID: accID,
				SettlementAccountID: settle, Ignored: sa.Ignored}); err != nil {
				return err
			}
			for j, ss := range sa.Securities {
				n, err := acct(fmt.Sprintf("%s.securities[%d].account_id", field, j), ss.AccountID)
				if err != nil {
					return err
				}
				if err := q.SetSourceSecurity(ctx, db.SetSourceSecurityParams{SourceAccountID: row.ID, Commodity: ss.Commodity, AccountID: n}); err != nil {
					return err
				}
			}
		}
		for i, r := range f.Rules {
			id, err := acct(fmt.Sprintf("rules[%d].account_id", i), r.AccountID)
			if err != nil {
				return err
			}
			var src *int64
			if r.SourceAccountID != nil {
				n, ok := sourceIDs[*r.SourceAccountID]
				if !ok {
					return fieldError(fmt.Sprintf("rules[%d].source_account_id", i), "not_found", "source account %d is not in the file", *r.SourceAccountID)
				}
				src = &n
			}
			if _, err := q.CreateRule(ctx, db.CreateRuleParams{BookID: book.ID, Priority: r.Priority, Pattern: r.Pattern,
				SourceAccountID: src, AccountID: id, CreatedBy: &userID}); err != nil {
				return err
			}
		}

		for i, p := range f.Prices {
			date, err := parseFileDate(fmt.Sprintf("prices[%d].date", i), p.Date)
			if err != nil {
				return err
			}
			n, err := q.ImportPrice(ctx, db.ImportPriceParams{Commodity: p.Commodity, Quote: p.Quote, Date: date, Rate: p.Rate, CreatedBy: &userID})
			if err != nil {
				return err
			}
			res.PricesAdded += n
			if n == 0 {
				res.PricesSkipped++
			}
		}

		// Last, so the transactions above could still post to them.
		for _, id := range archived {
			if _, err := q.SetAccountArchived(ctx, db.SetAccountArchivedParams{BookID: book.ID, ID: id, Archived: true}); err != nil {
				return err
			}
		}
		if lock != nil || cf != db.CfClassOperating {
			if book, err = q.UpdateBook(ctx, db.UpdateBookParams{ID: book.ID, Name: name, LockDate: lock, InterestDividendCfClass: cf}); err != nil {
				return err
			}
		}
		res.Book = book
		return q.SetDefaultBookIfUnset(ctx, db.SetDefaultBookIfUnsetParams{ID: userID, DefaultBookID: &book.ID})
	})
	if err != nil {
		return BookImport{}, translate(err, "book")
	}
	return res, nil
}

// importAccounts creates the file's accounts parents first and returns the
// file id -> new id map, and the new ids to archive once everything is in.
func importAccounts(ctx context.Context, q *db.Queries, bookID int64, accts []FileAccount) (map[int64]int64, []int64, error) {
	ids := make(map[int64]int64, len(accts))
	byID := make(map[int64]FileAccount, len(accts))
	for i, a := range accts {
		if _, dup := byID[a.ID]; dup {
			return nil, nil, fieldError(fmt.Sprintf("accounts[%d].id", i), "duplicate", "account id %d appears twice", a.ID)
		}
		byID[a.ID] = a
	}
	var archived []int64
	var create func(a FileAccount, depth int) error
	create = func(a FileAccount, depth int) error {
		if _, done := ids[a.ID]; done {
			return nil
		}
		if depth > len(accts) {
			return fieldError("accounts", "cycle", "account %d is its own ancestor", a.ID)
		}
		var parent *int64
		if a.ParentID != nil {
			p, ok := byID[*a.ParentID]
			if !ok {
				return fieldError("accounts", "not_found", "account %d has parent %d, which is not in the file", a.ID, *a.ParentID)
			}
			if err := create(p, depth+1); err != nil {
				return err
			}
			n := ids[p.ID]
			parent = &n
		}
		if !a.Class.Valid() || !a.CfClass.Valid() {
			return fieldError("accounts", "invalid", "account %d has class %q, cash-flow class %q", a.ID, a.Class, a.CfClass)
		}
		row, err := q.CreateAccount(ctx, db.CreateAccountParams{
			BookID: bookID, ParentID: parent, Class: a.Class, Name: a.Name, TemplateKey: a.TemplateKey, Code: a.Code,
			Commodity: a.Commodity, IsCurrent: a.IsCurrent, IsCash: a.IsCash, CfClass: a.CfClass, IsPlaceholder: a.IsPlaceholder,
		})
		if err != nil {
			return err
		}
		ids[a.ID] = row.ID
		if a.Archived {
			archived = append(archived, row.ID)
		}
		return nil
	}
	for _, a := range accts {
		if err := create(a, 0); err != nil {
			return nil, nil, err
		}
	}
	return ids, archived, nil
}

// importCommodities adds the file's securities and points programmes that
// this install does not have, and refuses one it has with a different
// meaning (another kind, quote currency or contract size), since every
// posting in that commodity would then mean something else.
func (s *Service) importCommodities(ctx context.Context, cs []FileCommodity) error {
	have, err := s.commodityMap(ctx)
	if err != nil {
		return err
	}
	added := false
	for i, c := range cs {
		field := fmt.Sprintf("commodities[%d]", i)
		if h, ok := have[c.Code]; ok {
			same := h.Kind == c.Kind && strEq(h.QuoteCurrency, c.QuoteCurrency) &&
				decEq(decPtr(h.ContractSize), c.ContractSize) && h.Decimals >= c.Decimals
			if !same {
				return &Error{Kind: KindInvalid, Code: "commodity_conflict", Fields: map[string]string{field: "commodity_conflict"},
					Message: fmt.Sprintf("%s exists here with a different kind, quote currency, contract size or fewer decimals", c.Code)}
			}
			continue
		}
		if c.Kind == db.CommodityKindCurrency {
			return fieldError(field, "unknown", "unknown currency %q", c.Code)
		}
		if !commodityCode.MatchString(c.Code) || strings.TrimSpace(c.Name) == "" || c.Decimals < 0 || c.Decimals > 8 {
			return fieldError(field, "invalid", "commodity %q is not valid", c.Code)
		}
		if c.QuoteCurrency != nil {
			if err := s.validCurrency(ctx, *c.QuoteCurrency); err != nil {
				return fieldError(field+".quote_currency", "unknown", "unknown currency %q", *c.QuoteCurrency)
			}
		}
		var size decimal.NullDecimal
		if c.ContractSize != nil {
			size = decimal.NullDecimal{Decimal: *c.ContractSize, Valid: true}
		}
		if _, err := s.store.CreateCommodity(ctx, db.CreateCommodityParams{Code: c.Code, Kind: c.Kind, Name: strings.TrimSpace(c.Name),
			Decimals: c.Decimals, QuoteCurrency: c.QuoteCurrency, ExchangeMic: c.ExchangeMIC, ContractSize: size}); err != nil {
			return translate(err, "commodity")
		}
		added = true
	}
	if added {
		s.invalidateCommodities()
	}
	return nil
}

func strEq(a, b *string) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }

func decEq(a, b *decimal.Decimal) bool { return (a == nil) == (b == nil) && (a == nil || a.Equal(*b)) }

// prefixFields puts the transaction's place in the file in front of a line
// error's field names, so "lines[1].amount" says which transaction.
func prefixFields(err error, prefix string) error {
	var le *Error
	if !errors.As(err, &le) || len(le.Fields) == 0 {
		return err
	}
	out := *le
	out.Fields = make(map[string]string, len(le.Fields))
	for k, v := range le.Fields {
		out.Fields[prefix+k] = v
	}
	return &out
}
