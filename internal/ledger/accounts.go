package ledger

import (
	"context"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

func (s *Service) ListAccounts(ctx context.Context, a Access) ([]db.Account, error) {
	return s.store.ListAccounts(ctx, a.Book.ID)
}

// Opening is an account's starting balance, entered when it is created.
// Amount uses the account's natural sign: a positive number is money you
// have for an asset and money you owe for a liability.
type Opening struct {
	Amount     decimal.Decimal
	BaseAmount *decimal.Decimal
	Date       time.Time
}

type AccountInput struct {
	ParentID      *int64
	Class         db.AccountClass
	Name          *string
	Code          *string
	Commodity     *string
	IsCurrent     bool
	IsCash        bool
	CfClass       db.CfClass
	IsPlaceholder bool
	Opening       *Opening
}

func cleanOptional(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}

func (s *Service) CreateAccount(ctx context.Context, a Access, in AccountInput) (db.Account, error) {
	if err := a.require(db.MemberRoleEditor); err != nil {
		return db.Account{}, err
	}
	in.Name = cleanOptional(in.Name)
	in.Code = cleanOptional(in.Code)
	if in.Name == nil {
		return db.Account{}, fieldError("name", "required", "an account needs a name")
	}
	if in.CfClass == "" {
		in.CfClass = db.CfClassOperating
	}
	if !in.CfClass.Valid() {
		return db.Account{}, fieldError("cf_class", "invalid", "must be operating, investing or financing")
	}

	var acct db.Account
	err := s.store.WithTx(ctx, a.UserID, func(q *db.Queries) error {
		if in.ParentID != nil {
			parent, err := q.GetAccount(ctx, db.GetAccountParams{BookID: a.Book.ID, ID: *in.ParentID})
			if err != nil {
				return fieldError("parent_id", "not_found", "parent account %d is not in this book", *in.ParentID)
			}
			if in.Class == "" {
				in.Class = parent.Class
			}
			if in.Class != parent.Class {
				return fieldError("parent_id", "class_mismatch", "a %s account cannot sit under a %s account", in.Class, parent.Class)
			}
		}
		if !in.Class.Valid() {
			return fieldError("class", "invalid", "class must be asset, liability, equity, income or expense")
		}
		if holdsCommodity(in.Class) {
			if in.Commodity == nil || *in.Commodity == "" {
				c := a.Book.BaseCurrency
				in.Commodity = &c
			}
			c := strings.ToUpper(*in.Commodity)
			in.Commodity = &c
			if err := s.validCurrency(ctx, c); err != nil {
				return fieldError("commodity", "unknown", "unknown currency %q", c)
			}
		} else {
			in.Commodity = nil
		}
		if in.IsCash && in.Class != db.AccountClassAsset {
			return fieldError("is_cash", "invalid", "only asset accounts can be cash")
		}
		if in.Opening != nil && in.IsPlaceholder {
			return fieldError("opening_balance", "placeholder", "a placeholder account cannot have a balance")
		}
		if in.Opening != nil && in.Class != db.AccountClassAsset && in.Class != db.AccountClassLiability {
			return fieldError("opening_balance", "invalid", "only asset and liability accounts take an opening balance")
		}

		var err error
		acct, err = q.CreateAccount(ctx, db.CreateAccountParams{
			BookID:        a.Book.ID,
			ParentID:      in.ParentID,
			Class:         in.Class,
			Name:          in.Name,
			Code:          in.Code,
			Commodity:     in.Commodity,
			IsCurrent:     in.IsCurrent,
			IsCash:        in.IsCash,
			CfClass:       in.CfClass,
			IsPlaceholder: in.IsPlaceholder,
		})
		if err != nil {
			return err
		}
		if in.Opening != nil && !in.Opening.Amount.IsZero() {
			return s.postOpening(ctx, q, a, acct, *in.Opening)
		}
		return nil
	})
	return acct, translate(err, "account")
}

// postOpening books an opening balance against the Opening balances equity
// account, as a source='opening' transaction so reports can tell it apart
// from real cash flow.
func (s *Service) postOpening(ctx context.Context, q *db.Queries, a Access, acct db.Account, o Opening) error {
	if o.Date.IsZero() {
		o.Date = time.Now().UTC().Truncate(24 * time.Hour)
	}
	if a.Book.LockDate != nil && !o.Date.After(*a.Book.LockDate) {
		return invalid("book_locked", "the book is locked up to %s", a.Book.LockDate.Format(time.DateOnly))
	}
	equity, err := q.GetAccountByTemplateKey(ctx, db.GetAccountByTemplateKeyParams{BookID: a.Book.ID, TemplateKey: ptr(KeyOpeningBalances)})
	if err != nil {
		return translate(err, "Opening balances account")
	}
	lc, err := s.newLineContext(ctx, q, a.Book, o.Date)
	if err != nil {
		return err
	}

	amount := o.Amount
	var base *decimal.Decimal
	if o.BaseAmount != nil {
		b := *o.BaseAmount
		base = &b
	}
	if acct.Class == db.AccountClassLiability { // credit-normal
		amount = amount.Neg()
		if base != nil {
			nb := base.Neg()
			base = &nb
		}
	}
	line, err := s.resolveLine(ctx, q, lc, 0, LineInput{AccountID: acct.ID, Amount: amount, BaseAmount: base})
	if err != nil {
		return err
	}
	counter := line.BaseAmount.Neg()

	t, err := q.CreateTransaction(ctx, db.CreateTransactionParams{
		BookID: a.Book.ID, Date: o.Date, Source: "opening", UserID: &a.UserID,
	})
	if err != nil {
		return err
	}
	line.TransactionID = t.ID
	if _, err := q.CreatePosting(ctx, line); err != nil {
		return err
	}
	_, err = q.CreatePosting(ctx, db.CreatePostingParams{
		TransactionID: t.ID,
		AccountID:     equity.ID,
		Position:      1,
		Commodity:     *equity.Commodity,
		Amount:        counter,
		BaseAmount:    counter,
		Status:        db.PostingStatusUncleared,
	})
	return err
}

type AccountUpdate struct {
	ParentID      *int64
	Name          *string
	Code          *string
	IsCurrent     bool
	IsCash        bool
	CfClass       db.CfClass
	IsPlaceholder bool
}

// UpdateAccount changes everything except class and commodity, which are
// fixed once an account exists: changing either would reinterpret postings.
func (s *Service) UpdateAccount(ctx context.Context, a Access, id int64, in AccountUpdate) (db.Account, error) {
	if err := a.require(db.MemberRoleEditor); err != nil {
		return db.Account{}, err
	}
	in.Name = cleanOptional(in.Name)
	in.Code = cleanOptional(in.Code)
	if in.CfClass == "" {
		in.CfClass = db.CfClassOperating
	}
	if !in.CfClass.Valid() {
		return db.Account{}, fieldError("cf_class", "invalid", "must be operating, investing or financing")
	}
	var acct db.Account
	err := s.store.WithTx(ctx, a.UserID, func(q *db.Queries) error {
		cur, err := q.GetAccount(ctx, db.GetAccountParams{BookID: a.Book.ID, ID: id})
		if err != nil {
			return translate(err, "account")
		}
		if in.Name == nil && cur.TemplateKey == nil {
			return fieldError("name", "required", "an account needs a name")
		}
		if in.IsCash && cur.Class != db.AccountClassAsset {
			return fieldError("is_cash", "invalid", "only asset accounts can be cash")
		}
		if in.IsPlaceholder && !cur.IsPlaceholder {
			has, err := q.AccountHasPostings(ctx, id)
			if err != nil {
				return err
			}
			if has {
				return fieldError("is_placeholder", "has_postings", "an account with postings cannot become a placeholder")
			}
		}
		acct, err = q.UpdateAccount(ctx, db.UpdateAccountParams{
			BookID:        a.Book.ID,
			ID:            id,
			ParentID:      in.ParentID,
			Name:          in.Name,
			Code:          in.Code,
			IsCurrent:     in.IsCurrent,
			IsCash:        in.IsCash,
			CfClass:       in.CfClass,
			IsPlaceholder: in.IsPlaceholder,
		})
		return err
	})
	return acct, translate(err, "account")
}

func (s *Service) SetAccountArchived(ctx context.Context, a Access, id int64, archived bool) (db.Account, error) {
	if err := a.require(db.MemberRoleEditor); err != nil {
		return db.Account{}, err
	}
	var acct db.Account
	err := s.store.WithTx(ctx, a.UserID, func(q *db.Queries) error {
		cur, err := q.GetAccount(ctx, db.GetAccountParams{BookID: a.Book.ID, ID: id})
		if err != nil {
			return translate(err, "account")
		}
		if archived && isSystemKey(cur.TemplateKey) {
			return conflict("system_account", "this account is required by the book")
		}
		acct, err = q.SetAccountArchived(ctx, db.SetAccountArchivedParams{BookID: a.Book.ID, ID: id, Archived: archived})
		return err
	})
	return acct, translate(err, "account")
}

// DeleteAccount removes an account that never held a posting and has no
// children. Anything with history is archived instead.
func (s *Service) DeleteAccount(ctx context.Context, a Access, id int64) error {
	if err := a.require(db.MemberRoleEditor); err != nil {
		return err
	}
	err := s.store.WithTx(ctx, a.UserID, func(q *db.Queries) error {
		cur, err := q.GetAccount(ctx, db.GetAccountParams{BookID: a.Book.ID, ID: id})
		if err != nil {
			return translate(err, "account")
		}
		if isSystemKey(cur.TemplateKey) {
			return conflict("system_account", "this account is required by the book")
		}
		if has, err := q.AccountHasPostings(ctx, id); err != nil {
			return err
		} else if has {
			return conflict("has_postings", "the account has postings; archive it instead")
		}
		if has, err := q.AccountHasChildren(ctx, &id); err != nil {
			return err
		} else if has {
			return conflict("has_children", "move or delete the child accounts first")
		}
		_, err = q.DeleteAccount(ctx, db.DeleteAccountParams{BookID: a.Book.ID, ID: id})
		return err
	})
	return translate(err, "account")
}
