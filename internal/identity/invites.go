package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
	"github.com/cwchen-twn/rigel-ledger/internal/mail"
)

// checkNewIdentity validates a username and address for a new account and
// refuses one that is taken, or reserved by a sign-up still waiting for its
// link.
func (s *Service) checkNewIdentity(ctx context.Context, username, email string) (string, string, error) {
	username = ledger.NormalizeUsername(username)
	email = strings.TrimSpace(email)
	if !ledger.ValidUsername(username) {
		return "", "", ledger.FieldError("username", "invalid", "2-50 characters: a-z, 0-9, dot, dash, underscore")
	}
	if !ledger.ValidEmail(email) {
		return "", "", ledger.FieldError("email", "invalid", "not an email address")
	}
	if taken, err := s.store.UsernameTaken(ctx, db.UsernameTakenParams{Username: username}); err != nil {
		return "", "", err
	} else if taken {
		return "", "", ledger.TakenError("username")
	}
	if taken, err := s.store.EmailTaken(ctx, db.EmailTakenParams{Email: email}); err != nil {
		return "", "", err
	} else if taken {
		return "", "", ledger.TakenError("email")
	}
	if pending, err := s.store.OpenRegistrationExists(ctx, db.OpenRegistrationExistsParams{Email: email, Username: username}); err != nil {
		return "", "", err
	} else if pending {
		return "", "", ledger.Conflict("pending_registration", "a sign-up with this username or address is waiting for its confirmation")
	}
	return username, email, nil
}

// Invite creates a user with no password and mails them a one-time link to
// choose their settings and password.
func (s *Service) Invite(ctx context.Context, admin db.User, username, email string) (db.User, error) {
	username, email, err := s.checkNewIdentity(ctx, username, email)
	if err != nil {
		return db.User{}, err
	}
	p, err := s.Defaults(ctx)
	if err != nil {
		return db.User{}, err
	}
	var u db.User
	err = s.store.WithTx(ctx, admin.ID, func(q *db.Queries) error {
		var err error
		u, err = q.CreateInitialUser(ctx, db.CreateInitialUserParams{
			Username: username, Email: email, Language: p.Language, DisplayCurrency: p.DisplayCurrency,
			Timezone: p.Timezone, DateFormat: p.DateFormat, Theme: p.Theme, InvitedBy: &admin.ID,
		})
		return err
	})
	if err != nil {
		if le, ok := ledger.Translate(err, "user").(*ledger.Error); ok && le.Code == "duplicate" {
			return db.User{}, ledger.Conflict("duplicate", le.Message)
		}
		return db.User{}, ledger.Translate(err, "user")
	}
	return u, s.sendInvite(ctx, admin, u)
}

func isPendingInvite(u db.User) bool { return u.PasswordHash == "" && u.InitializedAt == nil }

// ResendInvite replaces the link (the old one stops working) and mails it again.
func (s *Service) ResendInvite(ctx context.Context, admin db.User, userID int64) error {
	u, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return ledger.Translate(err, "user")
	}
	if !isPendingInvite(u) {
		return ledger.Conflict("not_invited", "this user has already accepted")
	}
	return s.sendInvite(ctx, admin, u)
}

func (s *Service) sendInvite(ctx context.Context, admin db.User, u db.User) error {
	st, err := s.Settings(ctx)
	if err != nil {
		return err
	}
	ttl := time.Duration(st.InviteTtlSeconds) * time.Second
	token, hash, err := auth.NewToken()
	if err != nil {
		return err
	}
	err = s.store.WithTx(ctx, admin.ID, func(q *db.Queries) error {
		if err := q.RevokeEmailTokens(ctx, db.RevokeEmailTokensParams{UserID: &u.ID, Kind: "invite"}); err != nil {
			return err
		}
		_, err := q.CreateEmailToken(ctx, db.CreateEmailTokenParams{
			UserID: &u.ID, Kind: "invite", Email: u.Email, TokenHash: hash, Payload: []byte("{}"),
			ExpiresAt: time.Now().Add(ttl), CreatedBy: &admin.ID,
		})
		return err
	})
	if err != nil {
		return err
	}
	s.record(ctx, auth.Event{Username: u.Username, UserID: &u.ID, Name: "invite_sent",
		Detail: map[string]any{"by": admin.Username}})
	return s.send(ctx, u.Email, u.Language, "invite", mail.Data{
		Username: u.Username, Inviter: displayName(admin), Link: s.link("/invite/" + token),
		Expires: humanDuration(u.Language, ttl),
	})
}

// RevokeInvite deletes an invited user who never accepted.
func (s *Service) RevokeInvite(ctx context.Context, admin db.User, userID int64) error {
	u, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return ledger.Translate(err, "user")
	}
	if !isPendingInvite(u) {
		return ledger.Conflict("not_invited", "this user has already accepted")
	}
	if err := s.store.WithTx(ctx, admin.ID, func(q *db.Queries) error { return q.DeleteUser(ctx, u.ID) }); err != nil {
		return err
	}
	s.record(ctx, auth.Event{Username: u.Username, Name: "invite_revoked", Detail: map[string]any{"by": admin.Username}})
	return nil
}

// invitation resolves a link token to its invited user.
func (s *Service) invitation(ctx context.Context, token string) (db.EmailToken, db.User, error) {
	t, err := s.store.GetEmailTokenByHash(ctx, auth.HashToken(token))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && t.Kind != "invite") || (err == nil && t.UserID == nil) {
		return db.EmailToken{}, db.User{}, ledger.NotFound("invitation")
	}
	if err != nil {
		return db.EmailToken{}, db.User{}, err
	}
	u, err := s.store.GetUserByID(ctx, *t.UserID)
	if err != nil {
		return db.EmailToken{}, db.User{}, ledger.Translate(err, "invitation")
	}
	return t, u, nil
}

// InvitePreview is what the invitation page shows before the user accepts.
func (s *Service) InvitePreview(ctx context.Context, token string, c auth.Client) (db.User, error) {
	if err := s.auth.Check(ctx, "", c.IP); err != nil {
		return db.User{}, err
	}
	_, u, err := s.invitation(ctx, token)
	if err != nil {
		s.record(ctx, auth.Event{IP: c.IP, UserAgent: c.UserAgent, Name: "invite_bad_link", Failure: true})
	}
	return u, err
}

// AcceptInvite sets the profile and password, marks the address verified
// (the link reached that mailbox) and the wizard done, and signs the user in.
func (s *Service) AcceptInvite(ctx context.Context, token string, p Profile, password string, c auth.Client) (db.User, string, error) {
	if err := s.auth.Check(ctx, "", c.IP); err != nil {
		return db.User{}, "", err
	}
	t, u, err := s.invitation(ctx, token)
	if err != nil {
		s.record(ctx, auth.Event{IP: c.IP, UserAgent: c.UserAgent, Name: "invite_bad_link", Failure: true})
		return db.User{}, "", err
	}
	if len(password) < ledger.MinPasswordLength {
		return db.User{}, "", ledger.FieldError("password", "too_short", "use at least %d characters", ledger.MinPasswordLength)
	}
	if err := s.validProfile(ctx, u.ID, &p); err != nil {
		return db.User{}, "", err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return db.User{}, "", err
	}
	var out db.User
	err = s.store.WithTx(ctx, u.ID, func(q *db.Queries) error {
		n, err := q.UseEmailToken(ctx, t.ID)
		if err != nil {
			return err
		}
		if n == 0 {
			return ledger.NotFound("invitation")
		}
		if _, err := applyProfile(ctx, q, u, p); err != nil {
			return err
		}
		if err := q.SetUserPassword(ctx, db.SetUserPasswordParams{ID: u.ID, PasswordHash: hash}); err != nil {
			return err
		}
		now := time.Now()
		if _, err := q.SetUserEmail(ctx, db.SetUserEmailParams{ID: u.ID, Email: u.Email, VerifiedAt: &now}); err != nil {
			return err
		}
		out, err = q.MarkUserInitialized(ctx, u.ID)
		return err
	})
	if err != nil {
		return db.User{}, "", ledger.Translate(err, "user")
	}
	session, err := s.auth.OpenSession(ctx, out.ID, "web", c)
	if err != nil {
		return db.User{}, "", err
	}
	s.record(ctx, auth.Event{Username: out.Username, UserID: &out.ID, IP: c.IP, UserAgent: c.UserAgent, Name: "invite_accepted"})
	return out, session, nil
}
