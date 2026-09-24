package ledger

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

// Access is the caller's membership in a book, resolved by the router once per
// request and passed to every service call on that book.
type Access struct {
	UserID int64
	Book   db.Book
	Role   db.MemberRole
}

func roleRank(r db.MemberRole) int {
	switch r {
	case db.MemberRoleOwner:
		return 3
	case db.MemberRoleEditor:
		return 2
	case db.MemberRoleViewer:
		return 1
	}
	return 0
}

// Can reports whether the caller's role is at least min.
func (a Access) Can(min db.MemberRole) bool { return roleRank(a.Role) >= roleRank(min) }

func (a Access) require(min db.MemberRole) error {
	if !a.Can(min) {
		return forbidden("role", "this needs the "+string(min)+" role in the book")
	}
	return nil
}

// ResolveAccess loads a book for a user. A book the user is not a member of
// is reported as not found, so book ids do not leak.
func (s *Service) ResolveAccess(ctx context.Context, userID, bookID int64) (Access, error) {
	role, err := s.store.GetMemberRole(ctx, db.GetMemberRoleParams{BookID: bookID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Access{}, notFound("book")
	}
	if err != nil {
		return Access{}, err
	}
	b, err := s.store.GetBook(ctx, bookID)
	if err != nil {
		return Access{}, translate(err, "book")
	}
	return Access{UserID: userID, Book: b, Role: role}, nil
}

type BookWithRole struct {
	Book db.Book
	Role db.MemberRole
}

func (s *Service) ListBooks(ctx context.Context, userID int64) ([]BookWithRole, error) {
	rows, err := s.store.ListBooksForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]BookWithRole, len(rows))
	for i, r := range rows {
		out[i] = BookWithRole{Book: r.Book, Role: r.Role}
	}
	return out, nil
}

// validCurrency accepts ISO currencies only: a book's base, a display
// currency and both sides of an exchange rate must be money, not miles.
func (s *Service) validCurrency(ctx context.Context, code string) error {
	m, err := s.commodityMap(ctx)
	if err != nil {
		return err
	}
	if c, ok := m[code]; !ok || c.Kind != db.CommodityKindCurrency {
		return fieldError("base_currency", "unknown", "unknown currency %q", code)
	}
	return nil
}

// validCommodity accepts anything an account can hold.
func (s *Service) validCommodity(ctx context.Context, code string) (db.Commodity, error) {
	m, err := s.commodityMap(ctx)
	if err != nil {
		return db.Commodity{}, err
	}
	c, ok := m[code]
	if !ok {
		return db.Commodity{}, fieldError("commodity", "unknown", "unknown commodity %q", code)
	}
	return c, nil
}

// CreateBook creates a book owned by the user and seeds the personal chart of
// accounts. The first book a user creates becomes their default.
func (s *Service) CreateBook(ctx context.Context, userID int64, name, baseCurrency string) (db.Book, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return db.Book{}, fieldError("name", "required", "a book needs a name")
	}
	baseCurrency = strings.ToUpper(strings.TrimSpace(baseCurrency))
	if err := s.validCurrency(ctx, baseCurrency); err != nil {
		return db.Book{}, err
	}
	var book db.Book
	err := s.store.WithTx(ctx, userID, func(q *db.Queries) error {
		var err error
		book, err = q.CreateBook(ctx, db.CreateBookParams{Name: name, BaseCurrency: baseCurrency, CreatedBy: &userID})
		if err != nil {
			return err
		}
		if err := q.AddMember(ctx, db.AddMemberParams{BookID: book.ID, UserID: userID, Role: db.MemberRoleOwner}); err != nil {
			return err
		}
		if err := seedTemplate(ctx, q, book.ID, baseCurrency); err != nil {
			return err
		}
		return q.SetDefaultBookIfUnset(ctx, db.SetDefaultBookIfUnsetParams{ID: userID, DefaultBookID: &book.ID})
	})
	return book, translate(err, "book")
}

type BookUpdate struct {
	Name                    string
	LockDate                *time.Time
	InterestDividendCfClass db.CfClass
}

func (s *Service) UpdateBook(ctx context.Context, a Access, in BookUpdate) (db.Book, error) {
	if err := a.require(db.MemberRoleOwner); err != nil {
		return db.Book{}, err
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return db.Book{}, fieldError("name", "required", "a book needs a name")
	}
	if !in.InterestDividendCfClass.Valid() {
		return db.Book{}, fieldError("interest_dividend_cf_class", "invalid", "must be operating, investing or financing")
	}
	var book db.Book
	err := s.store.WithTx(ctx, a.UserID, func(q *db.Queries) error {
		var err error
		book, err = q.UpdateBook(ctx, db.UpdateBookParams{
			ID:                      a.Book.ID,
			Name:                    in.Name,
			LockDate:                in.LockDate,
			InterestDividendCfClass: in.InterestDividendCfClass,
		})
		return err
	})
	return book, translate(err, "book")
}

func (s *Service) ListMembers(ctx context.Context, a Access) ([]db.ListMembersRow, error) {
	return s.store.ListMembers(ctx, a.Book.ID)
}

func (s *Service) AddMember(ctx context.Context, a Access, username string, role db.MemberRole) error {
	if err := a.require(db.MemberRoleOwner); err != nil {
		return err
	}
	if !role.Valid() {
		return fieldError("role", "invalid", "role must be owner, editor or viewer")
	}
	u, err := s.store.GetUserByUsername(ctx, strings.ToLower(strings.TrimSpace(username)))
	if err != nil {
		return translate(err, "user")
	}
	err = s.store.WithTx(ctx, a.UserID, func(q *db.Queries) error {
		return q.AddMember(ctx, db.AddMemberParams{BookID: a.Book.ID, UserID: u.ID, Role: role})
	})
	return translate(err, "member")
}

// ensureOwnerRemains refuses a change that would leave the book with no owner.
func ensureOwnerRemains(ctx context.Context, q *db.Queries, bookID, userID int64) error {
	current, err := q.GetMemberRole(ctx, db.GetMemberRoleParams{BookID: bookID, UserID: userID})
	if err != nil {
		return translate(err, "member")
	}
	if current != db.MemberRoleOwner {
		return nil
	}
	n, err := q.CountOwners(ctx, bookID)
	if err != nil {
		return err
	}
	if n <= 1 {
		return conflict("last_owner", "a book must keep at least one owner")
	}
	return nil
}

func (s *Service) UpdateMemberRole(ctx context.Context, a Access, userID int64, role db.MemberRole) error {
	if err := a.require(db.MemberRoleOwner); err != nil {
		return err
	}
	if !role.Valid() {
		return fieldError("role", "invalid", "role must be owner, editor or viewer")
	}
	err := s.store.WithTx(ctx, a.UserID, func(q *db.Queries) error {
		if role != db.MemberRoleOwner {
			if err := ensureOwnerRemains(ctx, q, a.Book.ID, userID); err != nil {
				return err
			}
		}
		n, err := q.UpdateMemberRole(ctx, db.UpdateMemberRoleParams{BookID: a.Book.ID, UserID: userID, Role: role})
		if err == nil && n == 0 {
			return notFound("member")
		}
		return err
	})
	return translate(err, "member")
}

func (s *Service) RemoveMember(ctx context.Context, a Access, userID int64) error {
	// Anyone may leave a book; only owners may remove someone else.
	if userID != a.UserID {
		if err := a.require(db.MemberRoleOwner); err != nil {
			return err
		}
	}
	err := s.store.WithTx(ctx, a.UserID, func(q *db.Queries) error {
		if err := ensureOwnerRemains(ctx, q, a.Book.ID, userID); err != nil {
			return err
		}
		n, err := q.RemoveMember(ctx, db.RemoveMemberParams{BookID: a.Book.ID, UserID: userID})
		if err == nil && n == 0 {
			return notFound("member")
		}
		return err
	})
	return translate(err, "member")
}

// DeleteBook removes a book and everything in it (owners only). confirm must
// be the book's name, typed again: there is no undo short of a restore. A
// locked book is refused -- its lock protects that history -- until the
// lock date is cleared.
func (s *Service) DeleteBook(ctx context.Context, a Access, confirm string) error {
	if err := a.require(db.MemberRoleOwner); err != nil {
		return err
	}
	if strings.TrimSpace(confirm) != a.Book.Name {
		return fieldError("confirm", "mismatch", "type the book's name to confirm")
	}
	if a.Book.LockDate != nil {
		return conflict("book_locked", "clear the lock date first")
	}
	return translate(s.store.WithTx(ctx, a.UserID, func(q *db.Queries) error {
		id := a.Book.ID
		if err := q.DeleteBookTransactions(ctx, id); err != nil {
			return err
		}
		if err := q.DetachBookAccounts(ctx, id); err != nil {
			return err
		}
		if err := q.DeleteBookAccounts(ctx, id); err != nil {
			return err
		}
		if err := q.DeleteBookTags(ctx, id); err != nil {
			return err
		}
		n, err := q.DeleteBook(ctx, id)
		if err == nil && n == 0 {
			return notFound("book")
		}
		return err
	}), "book")
}
