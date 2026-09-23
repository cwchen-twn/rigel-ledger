package ledger

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

type UserSettings struct {
	DisplayName     string
	Language        string
	DisplayCurrency string
	Timezone        string
	DateFormat      string
	Theme           string
	DefaultBookID   *int64
}

var (
	languages   = map[string]bool{"en": true, "zh": true, "es": true}
	dateFormats = map[string]bool{"YYYY-MM-DD": true, "DD/MM/YYYY": true, "MM/DD/YYYY": true, "YYYY/MM/DD": true}
	themes      = map[string]bool{"system": true, "light": true, "dark": true}
)

// UpdateSettings changes a user's preferences. None of them touches stored
// amounts, so every one can change at any time.
func (s *Service) UpdateSettings(ctx context.Context, userID int64, in UserSettings) (db.User, error) {
	in.DisplayCurrency = strings.ToUpper(strings.TrimSpace(in.DisplayCurrency))
	switch {
	case !languages[in.Language]:
		return db.User{}, fieldError("language", "invalid", "language must be en, zh or es")
	case !dateFormats[in.DateFormat]:
		return db.User{}, fieldError("date_format", "invalid", "unsupported date format")
	case !themes[in.Theme]:
		return db.User{}, fieldError("theme", "invalid", "theme must be system, light or dark")
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil || in.Timezone == "" {
		return db.User{}, fieldError("timezone", "invalid", "unknown time zone %q", in.Timezone)
	}
	if err := s.validCurrency(ctx, in.DisplayCurrency); err != nil {
		return db.User{}, fieldError("display_currency", "unknown", "unknown currency %q", in.DisplayCurrency)
	}
	if in.DefaultBookID != nil {
		if _, err := s.ResolveAccess(ctx, userID, *in.DefaultBookID); err != nil {
			return db.User{}, fieldError("default_book_id", "not_found", "not a member of book %d", *in.DefaultBookID)
		}
	}
	u, err := s.store.UpdateUserSettings(ctx, db.UpdateUserSettingsParams{
		ID:              userID,
		DisplayName:     strings.TrimSpace(in.DisplayName),
		Language:        in.Language,
		DisplayCurrency: in.DisplayCurrency,
		Timezone:        in.Timezone,
		DateFormat:      in.DateFormat,
		Theme:           in.Theme,
		DefaultBookID:   in.DefaultBookID,
	})
	return u, translate(err, "user")
}

const minPasswordLength = 8

// ChangePassword checks the current password, stores the new one and signs
// out every other session of the user.
func (s *Service) ChangePassword(ctx context.Context, user db.User, sessionID int64, current, next string) error {
	if !auth.CheckPassword(user.PasswordHash, current) {
		return fieldError("current_password", "wrong", "the current password is wrong")
	}
	if len(next) < minPasswordLength {
		return fieldError("new_password", "too_short", "use at least %d characters", minPasswordLength)
	}
	hash, err := auth.HashPassword(next)
	if err != nil {
		return err
	}
	return s.store.WithTx(ctx, user.ID, func(q *db.Queries) error {
		if err := q.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{ID: user.ID, PasswordHash: hash}); err != nil {
			return err
		}
		return q.DeleteUserSessionsExcept(ctx, db.DeleteUserSessionsExceptParams{UserID: user.ID, ID: sessionID})
	})
}

type NewUser struct {
	Username        string
	Email           string
	Password        string
	DisplayName     string
	Language        string
	DisplayCurrency string
	Timezone        string
	IsAdmin         bool
}

// CreateUser is used by the CLI; there is no public sign-up.
func (s *Service) CreateUser(ctx context.Context, in NewUser) (db.User, error) {
	in.Username = strings.ToLower(strings.TrimSpace(in.Username))
	in.Email = strings.TrimSpace(in.Email)
	if in.Language == "" {
		in.Language = "en"
	}
	if in.DisplayCurrency == "" {
		in.DisplayCurrency = "USD"
	}
	if in.Timezone == "" {
		in.Timezone = "UTC"
	}
	if in.Username == "" || in.Email == "" {
		return db.User{}, invalid("invalid_input", "username and email are required")
	}
	if len(in.Password) < minPasswordLength {
		return db.User{}, invalid("invalid_input", "the password needs at least %d characters", minPasswordLength)
	}
	if !languages[in.Language] {
		return db.User{}, invalid("invalid_input", "language must be en, zh or es")
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return db.User{}, invalid("invalid_input", "unknown time zone %q", in.Timezone)
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return db.User{}, err
	}
	u, err := s.store.CreateUser(ctx, db.CreateUserParams{
		Username:        in.Username,
		Email:           in.Email,
		PasswordHash:    hash,
		DisplayName:     in.DisplayName,
		Language:        in.Language,
		DisplayCurrency: strings.ToUpper(in.DisplayCurrency),
		Timezone:        in.Timezone,
		IsAdmin:         in.IsAdmin,
	})
	return u, translate(err, "user")
}

// ResetPassword is the CLI's way back in when a password is lost.
func (s *Service) ResetPassword(ctx context.Context, username, password string) error {
	if len(password) < minPasswordLength {
		return invalid("invalid_input", "the password needs at least %d characters", minPasswordLength)
	}
	u, err := s.store.GetUserByUsername(ctx, strings.ToLower(username))
	if err != nil {
		return translate(err, "user")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	return s.store.WithTx(ctx, 0, func(q *db.Queries) error {
		if err := q.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{ID: u.ID, PasswordHash: hash}); err != nil {
			return err
		}
		// Sign out everywhere: 0 matches no session id.
		return q.DeleteUserSessionsExcept(ctx, db.DeleteUserSessionsExceptParams{UserID: u.ID, ID: 0})
	})
}

func (s *Service) SetAdmin(ctx context.Context, username string, admin bool) error {
	u, err := s.store.GetUserByUsername(ctx, strings.ToLower(username))
	if err != nil {
		return translate(err, "user")
	}
	return s.store.SetUserAdmin(ctx, db.SetUserAdminParams{ID: u.ID, IsAdmin: admin})
}

// --- prices (manual exchange rates in P1; the scraper arrives in P3) ---

func (s *Service) ListPrices(ctx context.Context, commodity string, limit int) ([]db.Price, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	p := db.ListPricesParams{Lim: int32(limit)}
	if c := strings.ToUpper(strings.TrimSpace(commodity)); c != "" {
		p.Commodity = &c
	}
	return s.store.ListPrices(ctx, p)
}

type PriceInput struct {
	Commodity string
	Quote     string
	Date      time.Time
	Rate      decimal.Decimal
}

// AddPrice records a manual rate or share price. Prices are global facts
// shared by every book, so any editor of any book may add one.
func (s *Service) AddPrice(ctx context.Context, a Access, in PriceInput) (db.Price, error) {
	if err := a.require(db.MemberRoleEditor); err != nil {
		return db.Price{}, err
	}
	in.Commodity = strings.ToUpper(strings.TrimSpace(in.Commodity))
	in.Quote = strings.ToUpper(strings.TrimSpace(in.Quote))
	// A currency or a security (a share price in its quote currency). Points
	// have no market price: they are carried at cost.
	c, err := s.validCommodity(ctx, in.Commodity)
	if err != nil {
		return db.Price{}, err
	}
	if c.Kind == db.CommodityKindPoints {
		return db.Price{}, fieldError("commodity", "unpriced", "%s is carried at cost and has no market price", in.Commodity)
	}
	if err := s.validCurrency(ctx, in.Quote); err != nil {
		return db.Price{}, fieldError("quote", "unknown", "unknown currency %q", in.Quote)
	}
	if in.Commodity == in.Quote {
		return db.Price{}, fieldError("quote", "same", "a rate needs two different currencies")
	}
	if !in.Rate.IsPositive() {
		return db.Price{}, fieldError("rate", "invalid", "the rate must be positive")
	}
	if in.Date.IsZero() {
		return db.Price{}, fieldError("date", "required", "a rate needs a date")
	}
	p, err := s.store.UpsertPrice(ctx, db.UpsertPriceParams{
		Commodity: in.Commodity, Quote: in.Quote, Date: in.Date, Rate: in.Rate,
		Source: "manual", CreatedBy: &a.UserID,
	})
	return p, translate(err, "price")
}

func (s *Service) DeletePrice(ctx context.Context, a Access, id int64) error {
	if err := a.require(db.MemberRoleEditor); err != nil {
		return err
	}
	n, err := s.store.DeletePrice(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return notFound("manual rate")
	}
	return nil
}

// Rate answers the entry form's "what rate applies" question.
func (s *Service) Rate(ctx context.Context, from, to string, on time.Time) (decimal.Decimal, bool, error) {
	return RateOn(ctx, s.store.Queries, strings.ToUpper(from), strings.ToUpper(to), on)
}

// Same rule as the users.username CHECK in the schema.
var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,49}$`)

// UpdateIdentity changes the sign-in name and email. The current password is
// required because the username is what the next login uses. Nothing refers
// to a username (foreign keys use users.id), so a rename is safe.
func (s *Service) UpdateIdentity(ctx context.Context, user db.User, currentPassword, username, email string) (db.User, error) {
	if !auth.CheckPassword(user.PasswordHash, currentPassword) {
		return db.User{}, fieldError("current_password", "wrong", "the current password is wrong")
	}
	username = strings.ToLower(strings.TrimSpace(username))
	email = strings.TrimSpace(email)
	if !usernamePattern.MatchString(username) {
		return db.User{}, fieldError("username", "invalid", "2-50 characters: a-z, 0-9, dot, dash, underscore")
	}
	if !strings.Contains(email, "@") || strings.ContainsAny(email, " \t") {
		return db.User{}, fieldError("email", "invalid", "not an email address")
	}
	u, err := s.store.UpdateUserIdentity(ctx, db.UpdateUserIdentityParams{ID: user.ID, Username: username, Email: email})
	if err != nil {
		err = translate(err, "user")
		var le *Error
		if errors.As(err, &le) && le.Code == "duplicate" {
			field := "username"
			if strings.Contains(le.Message, "email") {
				field = "email"
			}
			return db.User{}, &Error{Kind: KindConflict, Code: "duplicate", Message: le.Message, Fields: map[string]string{field: "taken"}}
		}
		return db.User{}, err
	}
	return u, nil
}
