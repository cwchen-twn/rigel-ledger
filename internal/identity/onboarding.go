package identity

import (
	"context"
	"strings"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
)

// Profile is what the first-login wizard and the invitation page collect.
type Profile struct {
	Username    string
	DisplayName string
	ledger.Preferences
}

func (s *Service) validProfile(ctx context.Context, userID int64, p *Profile) error {
	p.Username = ledger.NormalizeUsername(p.Username)
	p.DisplayName = strings.TrimSpace(p.DisplayName)
	p.DisplayCurrency = strings.ToUpper(strings.TrimSpace(p.DisplayCurrency))
	if !ledger.ValidUsername(p.Username) {
		return ledger.FieldError("username", "invalid", "2-50 characters: a-z, 0-9, dot, dash, underscore")
	}
	taken, err := s.store.UsernameTaken(ctx, db.UsernameTakenParams{Username: p.Username, ExceptID: userID})
	if err != nil {
		return err
	}
	if taken {
		return ledger.TakenError("username")
	}
	return s.ledger.ValidatePreferences(ctx, p.Preferences, "")
}

// applyProfile writes the profile inside a transaction.
func applyProfile(ctx context.Context, q *db.Queries, u db.User, p Profile) (db.User, error) {
	if _, err := q.SetUserUsername(ctx, db.SetUserUsernameParams{ID: u.ID, Username: p.Username}); err != nil {
		return db.User{}, err
	}
	return q.UpdateUserSettings(ctx, db.UpdateUserSettingsParams{
		ID: u.ID, DisplayName: p.DisplayName, Language: p.Language, DisplayCurrency: p.DisplayCurrency,
		Timezone: p.Timezone, DateFormat: p.DateFormat, Theme: p.Theme, DefaultBookID: u.DefaultBookID,
	})
}

// CompleteOnboarding finishes the first-login wizard: the profile, a new
// password when the account was bootstrapped with one from the environment,
// and a verified address (confirmed earlier with ConfirmEmail).
func (s *Service) CompleteOnboarding(ctx context.Context, u db.User, p Profile, newPassword string, c auth.Client) (db.User, error) {
	if u.InitializedAt != nil && !u.PasswordMustChange {
		return db.User{}, ledger.Conflict("already_initialized", "the first-login setup is already done")
	}
	if u.Email == "" || u.EmailVerifiedAt == nil {
		return db.User{}, ledger.FieldError("email", "unverified", "verify your email address first")
	}
	var hash string
	if u.PasswordMustChange {
		if len(newPassword) < ledger.MinPasswordLength {
			return db.User{}, ledger.FieldError("new_password", "too_short", "use at least %d characters", ledger.MinPasswordLength)
		}
		if auth.CheckPassword(u.PasswordHash, newPassword) {
			return db.User{}, ledger.FieldError("new_password", "same", "choose a password different from the initial one")
		}
		var err error
		if hash, err = auth.HashPassword(newPassword); err != nil {
			return db.User{}, err
		}
	}
	if err := s.validProfile(ctx, u.ID, &p); err != nil {
		return db.User{}, err
	}
	var out db.User
	err := s.store.WithTx(ctx, u.ID, func(q *db.Queries) error {
		if _, err := applyProfile(ctx, q, u, p); err != nil {
			return err
		}
		if hash != "" {
			if err := q.SetUserPassword(ctx, db.SetUserPasswordParams{ID: u.ID, PasswordHash: hash}); err != nil {
				return err
			}
		}
		var err error
		out, err = q.MarkUserInitialized(ctx, u.ID)
		if err != nil { // already initialized: a bootstrap admin replacing the password again
			out, err = q.GetUserByID(ctx, u.ID)
		}
		return err
	})
	if err != nil {
		return db.User{}, ledger.Translate(err, "user")
	}
	s.record(ctx, auth.Event{Username: out.Username, UserID: &out.ID, IP: c.IP, UserAgent: c.UserAgent, Name: "onboarding_done"})
	return out, nil
}
