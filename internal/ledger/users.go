package ledger

import (
	"context"
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

// Preferences are the per-user display choices; the admin's defaults for
// new users are the same five.
type Preferences struct {
	Language        string
	DisplayCurrency string
	Timezone        string
	DateFormat      string
	Theme           string
}

// ValidatePreferences checks p, reporting fields as prefix+name (the admin
// form uses "default_"). DisplayCurrency must already be upper case.
func (s *Service) ValidatePreferences(ctx context.Context, p Preferences, prefix string) error {
	switch {
	case !languages[p.Language]:
		return fieldError(prefix+"language", "invalid", "language must be en, zh or es")
	case !dateFormats[p.DateFormat]:
		return fieldError(prefix+"date_format", "invalid", "unsupported date format")
	case !themes[p.Theme]:
		return fieldError(prefix+"theme", "invalid", "theme must be system, light or dark")
	}
	if _, err := time.LoadLocation(p.Timezone); err != nil || p.Timezone == "" {
		return fieldError(prefix+"timezone", "invalid", "unknown time zone %q", p.Timezone)
	}
	if err := s.validCurrency(ctx, p.DisplayCurrency); err != nil {
		return fieldError(prefix+"display_currency", "unknown", "unknown currency %q", p.DisplayCurrency)
	}
	return nil
}

// UpdateSettings changes a user's preferences. None of them touches stored
// amounts, so every one can change at any time: reports are computed per
// request and translated into the display currency when they are drawn, so
// a new display currency shows in every report on its next render.
func (s *Service) UpdateSettings(ctx context.Context, userID int64, in UserSettings) (db.User, error) {
	in.DisplayCurrency = strings.ToUpper(strings.TrimSpace(in.DisplayCurrency))
	if err := s.ValidatePreferences(ctx, Preferences{
		Language: in.Language, DisplayCurrency: in.DisplayCurrency, Timezone: in.Timezone,
		DateFormat: in.DateFormat, Theme: in.Theme,
	}, ""); err != nil {
		return db.User{}, err
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

// CreateUser is the CLI's way to add a user directly. The user still walks
// the first-login wizard (and verifies the address) at the first sign-in.
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

// NormalizeUsername lowercases and trims; ValidUsername applies the schema's rule.
func NormalizeUsername(u string) string { return strings.ToLower(strings.TrimSpace(u)) }
func ValidUsername(u string) bool       { return usernamePattern.MatchString(u) }

// ValidEmail is deliberately loose: the verification mail is the real test.
func ValidEmail(e string) bool {
	at := strings.LastIndex(e, "@")
	return at > 0 && at < len(e)-1 && !strings.ContainsAny(e, " \t\r\n<>,;")
}

const MinPasswordLength = minPasswordLength

// UpdateUsername changes the sign-in name. The current password is required
// because the username is what the next login uses. Nothing refers to a
// username (foreign keys use users.id), so a rename is safe. The email
// address changes through the identity package instead, because a new
// address must be verified before it is used.
func (s *Service) UpdateUsername(ctx context.Context, user db.User, currentPassword, username string) (db.User, error) {
	if !auth.CheckPassword(user.PasswordHash, currentPassword) {
		return db.User{}, fieldError("current_password", "wrong", "the current password is wrong")
	}
	return s.SetUsername(ctx, user.ID, username)
}

// SetUsername validates and stores a username, answering a clash as the
// field error username: taken.
func (s *Service) SetUsername(ctx context.Context, userID int64, username string) (db.User, error) {
	username = NormalizeUsername(username)
	if !ValidUsername(username) {
		return db.User{}, fieldError("username", "invalid", "2-50 characters: a-z, 0-9, dot, dash, underscore")
	}
	taken, err := s.store.UsernameTaken(ctx, db.UsernameTakenParams{Username: username, ExceptID: userID})
	if err != nil {
		return db.User{}, err
	}
	if taken {
		return db.User{}, takenError("username")
	}
	u, err := s.store.SetUserUsername(ctx, db.SetUserUsernameParams{ID: userID, Username: username})
	if err != nil {
		if le, ok := translate(err, "user").(*Error); ok && le.Code == "duplicate" {
			return db.User{}, takenError("username") // lost a race with another rename
		}
		return db.User{}, err
	}
	return u, nil
}

// TakenError is a conflict on a unique field: "username" or "email".
func TakenError(field string) *Error { return takenError(field) }

func takenError(field string) *Error {
	return &Error{Kind: KindConflict, Code: "duplicate", Message: field + " already in use", Fields: map[string]string{field: "taken"}}
}
