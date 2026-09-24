package identity

import (
	"context"
	"strings"
	"time"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
	"github.com/cwchen-twn/rigel-ledger/internal/mail"
)

const (
	codeTTL         = 10 * time.Minute
	maxCodeAttempts = 5
	// At most maxCodeSends codes per user inside codeSendWindow.
	maxCodeSends   = 3
	codeSendWindow = 15 * time.Minute
)

// StartEmailVerification mails a 6-digit code to address, which is either
// the user's current address (still unverified) or a new one. The address
// changes only when the code comes back (ConfirmEmail), so a typo never
// locks anyone out. After the first-login wizard the current password is
// required, as for a username change.
func (s *Service) StartEmailVerification(ctx context.Context, u db.User, address, currentPassword string, c auth.Client) error {
	address = strings.TrimSpace(address)
	if u.InitializedAt != nil && !auth.CheckPassword(u.PasswordHash, currentPassword) {
		return ledger.FieldError("current_password", "wrong", "the current password is wrong")
	}
	if !ledger.ValidEmail(address) {
		return ledger.FieldError("email", "invalid", "not an email address")
	}
	same := strings.EqualFold(address, u.Email)
	if same && u.EmailVerifiedAt != nil {
		return ledger.Conflict("already_verified", "this address is already verified")
	}
	if !same {
		taken, err := s.store.EmailTaken(ctx, db.EmailTakenParams{Email: address, ExceptID: u.ID})
		if err != nil {
			return err
		}
		if taken {
			return ledger.TakenError("email")
		}
	}
	sent, err := s.store.CountRecentEmailTokens(ctx, db.CountRecentEmailTokensParams{
		UserID: &u.ID, Kind: "verify", WindowSeconds: int64(codeSendWindow / time.Second),
	})
	if err != nil {
		return err
	}
	if sent >= maxCodeSends {
		return &auth.ThrottledError{RetryAfter: codeSendWindow}
	}

	code, hash, err := newCode()
	if err != nil {
		return err
	}
	err = s.store.WithTx(ctx, u.ID, func(q *db.Queries) error {
		if err := q.RevokeEmailTokens(ctx, db.RevokeEmailTokensParams{UserID: &u.ID, Kind: "verify"}); err != nil {
			return err
		}
		_, err := q.CreateEmailToken(ctx, db.CreateEmailTokenParams{
			UserID: &u.ID, Kind: "verify", Email: address, CodeHash: hash, Payload: []byte("{}"),
			ExpiresAt: time.Now().Add(codeTTL), CreatedBy: &u.ID,
		})
		return err
	})
	if err != nil {
		return err
	}
	s.record(ctx, auth.Event{Username: u.Username, UserID: &u.ID, IP: c.IP, UserAgent: c.UserAgent,
		Name: "email_code_sent", Detail: map[string]any{"email": address}})
	return s.send(ctx, address, u.Language, "verify", mail.Data{
		Name: displayName(u), Email: address, Code: code, Expires: humanDuration(u.Language, codeTTL),
	})
}

// ConfirmEmail checks the code. On success the address is verified and, if
// it was a new one, becomes the user's address; the old address is told.
func (s *Service) ConfirmEmail(ctx context.Context, u db.User, code string, c auth.Client) (db.User, error) {
	if err := s.auth.Check(ctx, u.Username, c.IP); err != nil {
		return db.User{}, err
	}
	t, err := s.store.GetOpenEmailToken(ctx, db.GetOpenEmailTokenParams{UserID: &u.ID, Kind: "verify"})
	if err != nil {
		return db.User{}, ledger.Translate(err, "pending verification")
	}
	if t.Attempts >= maxCodeAttempts {
		return db.User{}, ledger.FieldError("code", "expired", "too many wrong codes; request a new one")
	}
	if !codeMatches(t.CodeHash, strings.TrimSpace(code)) {
		_, _ = s.store.BumpEmailTokenAttempts(ctx, t.ID)
		s.record(ctx, auth.Event{Username: u.Username, UserID: &u.ID, IP: c.IP, UserAgent: c.UserAgent,
			Name: "email_code_bad", Failure: true})
		return db.User{}, ledger.FieldError("code", "wrong", "the code is wrong")
	}

	old := u.Email
	// The first address (a bootstrap admin's) is verified, not changed.
	changed := old != "" && !strings.EqualFold(old, t.Email)
	var out db.User
	err = s.store.WithTx(ctx, u.ID, func(q *db.Queries) error {
		n, err := q.UseEmailToken(ctx, t.ID)
		if err != nil {
			return err
		}
		if n == 0 {
			return ledger.NotFound("pending verification")
		}
		now := time.Now()
		out, err = q.SetUserEmail(ctx, db.SetUserEmailParams{ID: u.ID, Email: t.Email, VerifiedAt: &now})
		return err
	})
	if err != nil {
		if le, ok := ledger.Translate(err, "user").(*ledger.Error); ok && le.Code == "duplicate" {
			return db.User{}, ledger.TakenError("email")
		}
		return db.User{}, ledger.Translate(err, "user")
	}
	name := "email_verified"
	if changed {
		name = "email_changed"
	}
	s.record(ctx, auth.Event{Username: u.Username, UserID: &u.ID, IP: c.IP, UserAgent: c.UserAgent,
		Name: name, Detail: map[string]any{"email": t.Email, "old": old}})
	if changed {
		// Best effort: the change is done whether or not the notice goes out.
		_ = s.send(ctx, old, u.Language, "email_changed", mail.Data{Name: displayName(u), Email: t.Email})
	}
	return out, nil
}

// CancelPendingEmail drops a code that was sent and not used.
func (s *Service) CancelPendingEmail(ctx context.Context, u db.User) error {
	return s.store.RevokeEmailTokens(ctx, db.RevokeEmailTokensParams{UserID: &u.ID, Kind: "verify"})
}

// PendingEmail is the address a code was sent to and not yet confirmed, or "".
func (s *Service) PendingEmail(ctx context.Context, u db.User) string {
	t, err := s.store.GetOpenEmailToken(ctx, db.GetOpenEmailTokenParams{UserID: &u.ID, Kind: "verify"})
	if err != nil || t.Attempts >= maxCodeAttempts {
		return ""
	}
	return t.Email
}
