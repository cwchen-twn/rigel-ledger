package identity

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
	"github.com/cwchen-twn/rigel-ledger/internal/mail"
)

const registerTTL = 24 * time.Hour

type registerPayload struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	Language     string `json:"language"`
}

// PublicConfig is what the signed-out pages need to know.
type PublicConfig struct {
	Registration string
}

func (s *Service) PublicConfig(ctx context.Context) (PublicConfig, error) {
	st, err := s.Settings(ctx)
	return PublicConfig{Registration: st.Registration}, err
}

// anonymous rate-limits signed-out actions by address and records them so
// they count against it.
func (s *Service) anonymous(ctx context.Context, name string, c auth.Client) error {
	if err := s.auth.Check(ctx, "", c.IP); err != nil {
		return err
	}
	s.record(ctx, auth.Event{IP: c.IP, UserAgent: c.UserAgent, Name: name, Failure: true})
	return nil
}

// Register starts an open sign-up: a link goes to the address, and the user
// exists only once it is clicked, so an unverified sign-up reserves nothing
// for longer than registerTTL and never creates a user.
func (s *Service) Register(ctx context.Context, username, email, password, language string, c auth.Client) error {
	st, err := s.Settings(ctx)
	if err != nil {
		return err
	}
	if st.Registration != "open" {
		return ledger.Forbidden("registration_closed", "sign-up is not open")
	}
	if err := s.anonymous(ctx, "register_requested", c); err != nil {
		return err
	}
	username, email, err = s.checkNewIdentity(ctx, username, email)
	if err != nil {
		return err
	}
	if len(password) < ledger.MinPasswordLength {
		return ledger.FieldError("password", "too_short", "use at least %d characters", ledger.MinPasswordLength)
	}
	if language != "en" && language != "zh" && language != "es" {
		language = st.DefaultLanguage
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	token, tokenHash, err := auth.NewToken()
	if err != nil {
		return err
	}
	if _, err := s.store.CreateEmailToken(ctx, db.CreateEmailTokenParams{
		Kind: "register", Email: email, TokenHash: tokenHash,
		Payload:   mustJSON(registerPayload{Username: username, PasswordHash: hash, Language: language}),
		ExpiresAt: time.Now().Add(registerTTL),
	}); err != nil {
		return err
	}
	return s.send(ctx, email, language, "register", mail.Data{
		Username: username, Link: s.link("/verify?token=" + token), Expires: humanDuration(language, registerTTL),
	})
}

// VerifyLink completes a sign-up: the user is created with the address
// verified, signed in, and sent to the first-login wizard.
func (s *Service) VerifyLink(ctx context.Context, token string, c auth.Client) (db.User, string, error) {
	if err := s.auth.Check(ctx, "", c.IP); err != nil {
		return db.User{}, "", err
	}
	t, err := s.store.GetEmailTokenByHash(ctx, auth.HashToken(token))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && t.Kind != "register") {
		s.record(ctx, auth.Event{IP: c.IP, UserAgent: c.UserAgent, Name: "register_bad_link", Failure: true})
		return db.User{}, "", ledger.NotFound("sign-up link")
	}
	if err != nil {
		return db.User{}, "", err
	}
	var p registerPayload
	if err := json.Unmarshal(t.Payload, &p); err != nil {
		return db.User{}, "", err
	}
	defaults, err := s.Defaults(ctx)
	if err != nil {
		return db.User{}, "", err
	}
	var u db.User
	err = s.store.WithTx(ctx, 0, func(q *db.Queries) error {
		n, err := q.UseEmailToken(ctx, t.ID)
		if err != nil {
			return err
		}
		if n == 0 {
			return ledger.NotFound("sign-up link")
		}
		u, err = q.CreateInitialUser(ctx, db.CreateInitialUserParams{
			Username: p.Username, Email: t.Email, PasswordHash: p.PasswordHash, Language: p.Language,
			DisplayCurrency: defaults.DisplayCurrency, Timezone: defaults.Timezone,
			DateFormat: defaults.DateFormat, Theme: defaults.Theme,
		})
		if err != nil {
			return err
		}
		now := time.Now()
		u, err = q.SetUserEmail(ctx, db.SetUserEmailParams{ID: u.ID, Email: t.Email, VerifiedAt: &now})
		return err
	})
	if err != nil {
		if le, ok := ledger.Translate(err, "user").(*ledger.Error); ok && le.Code == "duplicate" {
			// Someone took the name or address while the link was in the mailbox.
			return db.User{}, "", ledger.Conflict("duplicate", "the username or address was taken meanwhile; sign up again")
		}
		return db.User{}, "", ledger.Translate(err, "user")
	}
	session, err := s.auth.OpenSession(ctx, u, "web", c, auth.AALPassword)
	if err != nil {
		return db.User{}, "", err
	}
	s.record(ctx, auth.Event{Username: u.Username, UserID: &u.ID, IP: c.IP, UserAgent: c.UserAgent, Name: "registered"})
	return u, session, nil
}

// RequestAccess files a request for the admins (registration = request).
// A taken username or address is accepted silently and filed nowhere, so the
// form does not reveal who has an account.
func (s *Service) RequestAccess(ctx context.Context, username, email, message string, c auth.Client) error {
	st, err := s.Settings(ctx)
	if err != nil {
		return err
	}
	if st.Registration != "request" {
		return ledger.Forbidden("registration_closed", "access requests are not accepted")
	}
	if err := s.anonymous(ctx, "access_requested", c); err != nil {
		return err
	}
	message = strings.TrimSpace(message)
	if len(message) > 1000 {
		message = message[:1000]
	}
	username, email, err = s.checkNewIdentity(ctx, username, email)
	if err != nil {
		var le *ledger.Error
		if errors.As(err, &le) && le.Code == "invalid_input" {
			return err
		}
		return nil
	}
	if pending, err := s.store.PendingAccessRequestExists(ctx, db.PendingAccessRequestExistsParams{Email: email, Username: username}); err != nil || pending {
		return err
	}
	if _, err := s.store.CreateAccessRequest(ctx, db.CreateAccessRequestParams{
		Username: username, Email: email, Message: message, Ip: c.IP,
	}); err != nil {
		return err
	}
	admins, err := s.store.ListAdminEmails(ctx)
	if err != nil {
		return err
	}
	for _, to := range admins {
		_ = s.send(ctx, to, st.DefaultLanguage, "access_request", mail.Data{
			Username: username, Email: email, Message: message, Link: s.link("/admin/requests"),
		})
	}
	return nil
}

func (s *Service) ListAccessRequests(ctx context.Context, status string) ([]db.ListAccessRequestsRow, error) {
	var st *string
	if status != "" {
		st = &status
	}
	return s.store.ListAccessRequests(ctx, st)
}

// DecideAccessRequest approves (which sends an invitation) or rejects.
func (s *Service) DecideAccessRequest(ctx context.Context, admin db.User, id int64, approve bool) error {
	r, err := s.store.GetAccessRequest(ctx, id)
	if err != nil {
		return ledger.Translate(err, "access request")
	}
	if r.Status != "pending" {
		return ledger.Conflict("already_decided", "this request was already decided")
	}
	status := "rejected"
	if approve {
		status = "approved"
		if _, err := s.Invite(ctx, admin, r.Username, r.Email); err != nil {
			return err
		}
	}
	if _, err := s.store.DecideAccessRequest(ctx, db.DecideAccessRequestParams{ID: id, Status: status, DecidedBy: &admin.ID}); err != nil {
		return err
	}
	s.record(ctx, auth.Event{Username: r.Username, Name: "access_" + status, Detail: map[string]any{"by": admin.Username}})
	return nil
}
