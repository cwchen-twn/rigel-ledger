package identity

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
)

// BootstrapAdmin is ADMIN_USERNAME / ADMIN_INITIAL_PASSWORD / ADMIN_EMAIL.
type BootstrapAdmin struct {
	Username string
	Password string
	Email    string
}

// EnsureAdmin creates the first administrator from the environment when the
// instance has none. It never touches an existing user: once any admin
// exists the variables are ignored (and the password can be removed from
// the secret). The new admin must replace the password and walk the
// first-login wizard before anything else.
func (s *Service) EnsureAdmin(ctx context.Context, b BootstrapAdmin) (bool, error) {
	b.Username = ledger.NormalizeUsername(b.Username)
	b.Email = strings.TrimSpace(b.Email)
	if b.Username == "" {
		return false, nil
	}
	n, err := s.store.CountAdmins(ctx)
	if err != nil {
		return false, err
	}
	if n > 0 {
		if b.Password != "" {
			s.logger.Info("An administrator exists; ADMIN_* variables are ignored and ADMIN_INITIAL_PASSWORD can be removed")
		}
		return false, nil
	}
	if _, err := s.store.GetUserByUsername(ctx, b.Username); err == nil {
		return false, errors.New("ADMIN_USERNAME " + b.Username + " already exists but is not an admin; promote it with: rigel-ledger-cli set-admin -u " + b.Username)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	switch {
	case !ledger.ValidUsername(b.Username):
		return false, errors.New("ADMIN_USERNAME: 2-50 characters: a-z, 0-9, dot, dash, underscore")
	case len(b.Password) < ledger.MinPasswordLength:
		return false, errors.New("ADMIN_INITIAL_PASSWORD must have at least 8 characters")
	case b.Email != "" && !ledger.ValidEmail(b.Email):
		return false, errors.New("ADMIN_EMAIL is not an email address")
	}
	hash, err := auth.HashPassword(b.Password)
	if err != nil {
		return false, err
	}
	p, err := s.Defaults(ctx)
	if err != nil {
		return false, err
	}
	u, err := s.store.CreateInitialUser(ctx, db.CreateInitialUserParams{
		Username: b.Username, Email: b.Email, PasswordHash: hash, Language: p.Language,
		DisplayCurrency: p.DisplayCurrency, Timezone: p.Timezone, DateFormat: p.DateFormat, Theme: p.Theme,
		IsAdmin: true, PasswordMustChange: true,
	})
	if err != nil {
		return false, ledger.Translate(err, "user")
	}
	s.record(ctx, auth.Event{Username: u.Username, UserID: &u.ID, Name: "admin_bootstrapped"})
	s.logger.Info("Created the bootstrap administrator; the first sign-in asks for a new password", "username", u.Username)
	return true, nil
}
