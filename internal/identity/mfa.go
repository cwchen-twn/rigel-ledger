package identity

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
	"github.com/cwchen-twn/rigel-ledger/internal/mail"
)

const (
	loginChallengeTTL  = 5 * time.Minute
	maxLoginAttempts   = 5
	signinCodeTTL      = 10 * time.Minute
	signinCodeResendIn = time.Minute
	recoveryCodeCount  = 10
	totpPeriod         = 30
)

var totpOpts = totp.ValidateOpts{Period: totpPeriod, Skew: 0, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1}

// MFAState is what a user has enrolled, filtered by what the admin allows.
type MFAState struct {
	TOTP         bool
	Email        bool
	Passkeys     int64
	RecoveryLeft int64
	Allowed      []string // system_settings.mfa_methods
	Required     bool
}

// Methods are the second factors this user can use right now.
func (m MFAState) Methods() []string {
	var out []string
	if m.TOTP && slices.Contains(m.Allowed, "totp") {
		out = append(out, "totp")
	}
	if m.Email && slices.Contains(m.Allowed, "email") {
		out = append(out, "email")
	}
	if m.Passkeys > 0 && slices.Contains(m.Allowed, "passkey") {
		out = append(out, "passkey")
	}
	if len(out) > 0 && m.RecoveryLeft > 0 {
		out = append(out, "recovery")
	}
	return out
}

// Enrolled reports at least one allowed factor (recovery codes alone do not count).
func (m MFAState) Enrolled() bool {
	for _, x := range m.Methods() {
		if x != "recovery" {
			return true
		}
	}
	return false
}

func (m MFAState) factorCount() int {
	n := int(m.Passkeys)
	if m.TOTP {
		n++
	}
	if m.Email {
		n++
	}
	return n
}

func (s *Service) MFAState(ctx context.Context, userID int64) (MFAState, error) {
	st, err := s.Settings(ctx)
	if err != nil {
		return MFAState{}, err
	}
	row, err := s.store.GetMFAState(ctx, userID)
	if err != nil {
		return MFAState{}, err
	}
	return MFAState{TOTP: row.Totp, Email: row.Email, Passkeys: row.Passkeys, RecoveryLeft: row.RecoveryLeft,
		Allowed: st.MfaMethods, Required: st.MfaRequired}, nil
}

// LoginResult is either a session (Token) or a second step to take
// (Challenge, Methods).
type LoginResult struct {
	User      db.User
	Token     string
	Challenge string
	Methods   []string
}

// Login checks the password. A user with a second factor gets a challenge
// instead of a session; everyone else a password-level session, which the
// MFA gate confines to enrolment while two-factor sign-in is required.
func (s *Service) Login(ctx context.Context, username, password, client string, c auth.Client) (LoginResult, error) {
	u, err := s.auth.VerifyPassword(ctx, username, password, c)
	if err != nil {
		return LoginResult{}, err
	}
	state, err := s.MFAState(ctx, u.ID)
	if err != nil {
		return LoginResult{}, err
	}
	if state.Enrolled() {
		token, hash, err := auth.NewToken()
		if err != nil {
			return LoginResult{}, err
		}
		if _, err := s.store.CreateChallenge(ctx, db.CreateChallengeParams{
			UserID: &u.ID, Kind: "login", TokenHash: hash, Client: client, Payload: []byte("{}"),
			ExpiresAt: time.Now().Add(loginChallengeTTL),
		}); err != nil {
			return LoginResult{}, err
		}
		return LoginResult{User: u, Challenge: token, Methods: state.Methods()}, nil
	}
	token, err := s.openSession(ctx, u, client, c, auth.AALPassword)
	return LoginResult{User: u, Token: token}, err
}

// openSession opens a session and mails a new-sign-in alert when the
// address has not signed in for this user lately.
func (s *Service) openSession(ctx context.Context, u db.User, client string, c auth.Client, aal int16) (string, error) {
	seen, _ := s.store.SeenFromIP(ctx, db.SeenFromIPParams{UserID: &u.ID, Ip: c.IP})
	first := u.LastLoginAt == nil
	token, err := s.auth.OpenSession(ctx, u, client, c, aal)
	if err != nil {
		return "", err
	}
	if !seen && !first && u.SigninAlerts && u.Email != "" && u.EmailVerifiedAt != nil {
		ip := "?"
		if c.IP != nil {
			ip = c.IP.String()
		}
		// Best effort: a sign-in never fails because the alert did.
		_ = s.send(ctx, u.Email, u.Language, "signin_alert", mail.Data{
			Name: displayName(u), Message: fmt.Sprintf("%s · %s · %s", time.Now().UTC().Format("2006-01-02 15:04 UTC"), ip, c.UserAgent),
			Link: s.link("/settings"),
		})
	}
	return token, nil
}

// loginChallenge resolves a second-step token; too many wrong codes kill it.
func (s *Service) loginChallenge(ctx context.Context, token string) (db.AuthChallenge, db.User, error) {
	ch, err := s.store.GetChallenge(ctx, db.GetChallengeParams{TokenHash: auth.HashToken(token), Kind: "login"})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && ch.UserID == nil) {
		return db.AuthChallenge{}, db.User{}, ledger.NotFound("sign-in step")
	}
	if err != nil {
		return db.AuthChallenge{}, db.User{}, err
	}
	if ch.Attempts >= maxLoginAttempts {
		_, _ = s.store.DeleteChallenge(ctx, ch.ID)
		return db.AuthChallenge{}, db.User{}, ledger.NotFound("sign-in step")
	}
	u, err := s.store.GetUserByID(ctx, *ch.UserID)
	if err != nil || !u.IsActive {
		return db.AuthChallenge{}, db.User{}, ledger.NotFound("sign-in step")
	}
	return ch, u, nil
}

// SendSignInCode mails a code for the second step (the email factor).
func (s *Service) SendSignInCode(ctx context.Context, challenge string, c auth.Client) error {
	ch, u, err := s.loginChallenge(ctx, challenge)
	if err != nil {
		return err
	}
	state, err := s.MFAState(ctx, u.ID)
	if err != nil {
		return err
	}
	if !slices.Contains(state.Methods(), "email") {
		return ledger.FieldError("method", "unavailable", "email codes are not enabled for this account")
	}
	if ch.CodeSentAt != nil && time.Since(*ch.CodeSentAt) < signinCodeResendIn {
		return &auth.ThrottledError{RetryAfter: signinCodeResendIn - time.Since(*ch.CodeSentAt)}
	}
	code, hash, err := newCode()
	if err != nil {
		return err
	}
	if err := s.store.SetChallengeCode(ctx, db.SetChallengeCodeParams{ID: ch.ID, CodeHash: hash}); err != nil {
		return err
	}
	s.record(ctx, auth.Event{Username: u.Username, UserID: &u.ID, IP: c.IP, UserAgent: c.UserAgent, Name: "signin_code_sent"})
	return s.send(ctx, u.Email, u.Language, "signin_code", mail.Data{
		Name: displayName(u), Code: code, Expires: humanDuration(u.Language, signinCodeTTL),
	})
}

// VerifySignIn finishes the second step with a TOTP, emailed or recovery
// code, and opens a two-factor session.
func (s *Service) VerifySignIn(ctx context.Context, challenge, method, code string, c auth.Client) (db.User, string, string, error) {
	ch, u, err := s.loginChallenge(ctx, challenge)
	if err != nil {
		return db.User{}, "", "", err
	}
	if err := s.auth.Check(ctx, u.Username, c.IP); err != nil {
		return db.User{}, "", "", err
	}
	state, err := s.MFAState(ctx, u.ID)
	if err != nil {
		return db.User{}, "", "", err
	}
	if !slices.Contains(state.Methods(), method) || method == "passkey" {
		return db.User{}, "", "", ledger.FieldError("method", "unavailable", "that method is not available")
	}
	code = strings.TrimSpace(code)
	ok := false
	switch method {
	case "totp":
		ok, err = s.checkTOTP(ctx, u.ID, code)
	case "email":
		ok = ch.CodeSentAt != nil && time.Since(*ch.CodeSentAt) < signinCodeTTL && codeMatches(ch.CodeHash, code)
	case "recovery":
		var n int64
		n, err = s.store.UseRecoveryCode(ctx, db.UseRecoveryCodeParams{UserID: u.ID, CodeHash: hashRecovery(code)})
		ok = n == 1
	}
	if err != nil {
		return db.User{}, "", "", err
	}
	if !ok {
		_, _ = s.store.BumpChallengeAttempts(ctx, ch.ID)
		s.record(ctx, auth.Event{Username: u.Username, UserID: &u.ID, IP: c.IP, UserAgent: c.UserAgent,
			Name: "mfa_bad", Failure: true, Detail: map[string]any{"method": method}})
		return db.User{}, "", "", ledger.FieldError("code", "wrong", "the code is wrong")
	}
	if _, err := s.store.DeleteChallenge(ctx, ch.ID); err != nil {
		return db.User{}, "", "", err
	}
	s.record(ctx, auth.Event{Username: u.Username, UserID: &u.ID, IP: c.IP, UserAgent: c.UserAgent,
		Name: "mfa_ok", Detail: map[string]any{"method": method}})
	token, err := s.openSession(ctx, u, ch.Client, c, auth.AALMFA)
	return u, token, ch.Client, err
}

// --- TOTP ---

// TOTPSetup is shown once, to be scanned by an authenticator app.
type TOTPSetup struct {
	Secret string
	URI    string
	QR     string // data:image/png;base64,...
}

func (s *Service) allowed(ctx context.Context, method string) error {
	st, err := s.Settings(ctx)
	if err != nil {
		return err
	}
	if !slices.Contains(st.MfaMethods, method) {
		return ledger.Forbidden("method_disabled", "the administrator has turned this method off")
	}
	return nil
}

func (s *Service) StartTOTP(ctx context.Context, u db.User) (TOTPSetup, error) {
	if err := s.allowed(ctx, "totp"); err != nil {
		return TOTPSetup{}, err
	}
	if f, err := s.store.GetFactor(ctx, db.GetFactorParams{UserID: u.ID, Kind: "totp"}); err == nil && f.ConfirmedAt != nil {
		return TOTPSetup{}, ledger.Conflict("already_enrolled", "an authenticator app is already set up; remove it first")
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: s.appName, AccountName: u.Username, Period: totpPeriod})
	if err != nil {
		return TOTPSetup{}, err
	}
	sealed, err := s.box.Seal([]byte(key.Secret()))
	if err != nil {
		return TOTPSetup{}, err
	}
	if _, err := s.store.UpsertPendingTOTP(ctx, db.UpsertPendingTOTPParams{UserID: u.ID, SecretEnc: sealed}); err != nil {
		return TOTPSetup{}, err
	}
	img, err := key.Image(240, 240)
	if err != nil {
		return TOTPSetup{}, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return TOTPSetup{}, err
	}
	return TOTPSetup{Secret: key.Secret(), URI: key.URL(),
		QR: "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())}, nil
}

// totpStep finds which time step (now, or one either side for clock skew)
// the code belongs to.
func (s *Service) totpStep(sealed []byte, code string) (int64, bool, error) {
	secret, err := s.box.Open(sealed)
	if err != nil {
		return 0, false, err
	}
	now := time.Now().Unix() / totpPeriod
	for _, step := range []int64{now, now - 1, now + 1} {
		want, err := totp.GenerateCodeCustom(string(secret), time.Unix(step*totpPeriod, 0), totpOpts)
		if err != nil {
			return 0, false, err
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return step, true, nil
		}
	}
	return 0, false, nil
}

// checkTOTP accepts a code once: its step must be newer than the last.
func (s *Service) checkTOTP(ctx context.Context, userID int64, code string) (bool, error) {
	f, err := s.store.GetFactor(ctx, db.GetFactorParams{UserID: userID, Kind: "totp"})
	if err != nil || f.ConfirmedAt == nil {
		return false, nil
	}
	step, ok, err := s.totpStep(f.SecretEnc, code)
	if err != nil || !ok {
		return false, err
	}
	n, err := s.store.AdvanceTOTPStep(ctx, db.AdvanceTOTPStepParams{ID: f.ID, Step: step})
	return n == 1, err
}

// ConfirmTOTP finishes enrolment with a first code. The session becomes
// two-factor; the first factor also mints the recovery codes.
func (s *Service) ConfirmTOTP(ctx context.Context, id auth.Identity, code string, c auth.Client) ([]string, error) {
	u := id.User
	f, err := s.store.GetFactor(ctx, db.GetFactorParams{UserID: u.ID, Kind: "totp"})
	if err != nil || f.ConfirmedAt != nil {
		return nil, ledger.Conflict("no_enrolment", "start the authenticator setup first")
	}
	step, ok, err := s.totpStep(f.SecretEnc, strings.TrimSpace(code))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ledger.FieldError("code", "wrong", "the code is wrong")
	}
	if err := s.store.ConfirmFactor(ctx, db.ConfirmFactorParams{ID: f.ID, LastStep: step}); err != nil {
		return nil, err
	}
	return s.enrolled(ctx, id, "totp", c)
}

// enrolled runs after any factor is added.
func (s *Service) enrolled(ctx context.Context, id auth.Identity, method string, c auth.Client) ([]string, error) {
	u := id.User
	if err := s.auth.RaiseSession(ctx, id.SessionID); err != nil {
		return nil, err
	}
	s.record(ctx, auth.Event{Username: u.Username, UserID: &u.ID, IP: c.IP, UserAgent: c.UserAgent,
		Name: "mfa_enrolled", Detail: map[string]any{"method": method}})
	state, err := s.store.GetMFAState(ctx, u.ID)
	if err != nil || state.RecoveryLeft > 0 {
		return nil, err
	}
	return s.newRecoveryCodes(ctx, u.ID)
}

// --- email factor ---

// StartEmailFactor mails a code to the (verified) address; confirming it
// proves the mailbox is the user's before it becomes a factor.
func (s *Service) StartEmailFactor(ctx context.Context, u db.User) (string, error) {
	if err := s.allowed(ctx, "email"); err != nil {
		return "", err
	}
	if u.Email == "" || u.EmailVerifiedAt == nil {
		return "", ledger.FieldError("email", "unverified", "verify your email address first")
	}
	code, codeHash, err := newCode()
	if err != nil {
		return "", err
	}
	token, hash, err := auth.NewToken()
	if err != nil {
		return "", err
	}
	if _, err := s.store.CreateChallenge(ctx, db.CreateChallengeParams{
		UserID: &u.ID, Kind: "email_enroll", TokenHash: hash, Client: "web", Payload: []byte("{}"),
		CodeHash: codeHash, ExpiresAt: time.Now().Add(signinCodeTTL),
	}); err != nil {
		return "", err
	}
	return token, s.send(ctx, u.Email, u.Language, "signin_code", mail.Data{
		Name: displayName(u), Code: code, Expires: humanDuration(u.Language, signinCodeTTL),
	})
}

func (s *Service) ConfirmEmailFactor(ctx context.Context, id auth.Identity, challenge, code string, c auth.Client) ([]string, error) {
	u := id.User
	ch, err := s.store.GetChallenge(ctx, db.GetChallengeParams{TokenHash: auth.HashToken(challenge), Kind: "email_enroll"})
	if err != nil || ch.UserID == nil || *ch.UserID != u.ID || ch.Attempts >= maxLoginAttempts {
		return nil, ledger.NotFound("email setup")
	}
	if !codeMatches(ch.CodeHash, strings.TrimSpace(code)) {
		_, _ = s.store.BumpChallengeAttempts(ctx, ch.ID)
		return nil, ledger.FieldError("code", "wrong", "the code is wrong")
	}
	_, _ = s.store.DeleteChallenge(ctx, ch.ID)
	if err := s.store.EnableEmailFactor(ctx, u.ID); err != nil {
		return nil, err
	}
	return s.enrolled(ctx, id, "email", c)
}

// --- recovery codes ---

var recoveryAlphabet = base32.NewEncoding("abcdefghijkmnpqrstuvwxyz23456789").WithPadding(base32.NoPadding)

func normalizeRecovery(code string) string {
	return strings.ToLower(strings.NewReplacer("-", "", " ", "").Replace(code))
}

func hashRecovery(code string) []byte {
	h := sha256.Sum256([]byte("rigel-recovery:" + normalizeRecovery(code)))
	return h[:]
}

// newRecoveryCodes replaces any old ones: ten "xxxxx-xxxxx", about 50 bits each.
func (s *Service) newRecoveryCodes(ctx context.Context, userID int64) ([]string, error) {
	codes := make([]string, recoveryCodeCount)
	err := s.store.WithTx(ctx, userID, func(q *db.Queries) error {
		if err := q.DeleteRecoveryCodes(ctx, userID); err != nil {
			return err
		}
		for i := range codes {
			b := make([]byte, 7)
			if _, err := rand.Read(b); err != nil {
				return err
			}
			raw := recoveryAlphabet.EncodeToString(b)[:10]
			codes[i] = raw[:5] + "-" + raw[5:]
			if err := q.CreateRecoveryCode(ctx, db.CreateRecoveryCodeParams{UserID: userID, CodeHash: hashRecovery(raw)}); err != nil {
				return err
			}
		}
		return nil
	})
	return codes, err
}

// RegenerateRecoveryCodes needs a two-factor session and the password.
func (s *Service) RegenerateRecoveryCodes(ctx context.Context, id auth.Identity, password string, c auth.Client) ([]string, error) {
	if err := s.stepUp(ctx, id, password); err != nil {
		return nil, err
	}
	codes, err := s.newRecoveryCodes(ctx, id.User.ID)
	if err == nil {
		s.record(ctx, auth.Event{Username: id.User.Username, UserID: &id.User.ID, IP: c.IP, UserAgent: c.UserAgent, Name: "recovery_regenerated"})
	}
	return codes, err
}

// stepUp: changing factors needs the password again, in a session that
// already passed a second factor (when the user has one).
func (s *Service) stepUp(ctx context.Context, id auth.Identity, password string) error {
	state, err := s.MFAState(ctx, id.User.ID)
	if err != nil {
		return err
	}
	if state.factorCount() > 0 && id.AAL < auth.AALMFA {
		return ledger.Forbidden("mfa_step_up", "sign in again with your second factor first")
	}
	if !auth.CheckPassword(id.User.PasswordHash, password) {
		return ledger.FieldError("current_password", "wrong", "the current password is wrong")
	}
	return nil
}

// RemoveFactor drops "totp", "email" or passkey:<id>. The last factor stays
// while two-factor sign-in is required.
func (s *Service) RemoveFactor(ctx context.Context, id auth.Identity, factor string, passkeyID int64, password string, c auth.Client) error {
	if err := s.stepUp(ctx, id, password); err != nil {
		return err
	}
	u := id.User
	state, err := s.MFAState(ctx, u.ID)
	if err != nil {
		return err
	}
	if state.Required && state.factorCount() <= 1 {
		return ledger.Conflict("last_factor", "add another method before removing this one")
	}
	var n int64
	if factor == "passkey" {
		n, err = s.store.DeletePasskey(ctx, db.DeletePasskeyParams{ID: passkeyID, UserID: u.ID})
	} else {
		n, err = s.store.DeleteFactor(ctx, db.DeleteFactorParams{UserID: u.ID, Kind: factor})
	}
	if err != nil {
		return err
	}
	if n == 0 {
		return ledger.NotFound("method")
	}
	if state.factorCount() == 1 { // that was the last one: the codes go too
		_ = s.store.DeleteRecoveryCodes(ctx, u.ID)
	}
	s.record(ctx, auth.Event{Username: u.Username, UserID: &u.ID, IP: c.IP, UserAgent: c.UserAgent,
		Name: "mfa_removed", Detail: map[string]any{"method": factor}})
	return nil
}

func (s *Service) SetSignInAlerts(ctx context.Context, u db.User, on bool) error {
	return s.store.SetSignInAlerts(ctx, db.SetSignInAlertsParams{ID: u.ID, SigninAlerts: on})
}

// ResetMFA is the admin's (and the CLI's) way back in for a user who lost
// every factor: all factors, passkeys, codes and sessions go.
func (s *Service) ResetMFA(ctx context.Context, actorID int64, actor string, userID int64) error {
	u, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return ledger.Translate(err, "user")
	}
	err = s.store.WithTx(ctx, actorID, func(q *db.Queries) error {
		if err := q.ResetMFA(ctx, u.ID); err != nil {
			return err
		}
		return q.DeleteUserSessions(ctx, u.ID)
	})
	if err != nil {
		return err
	}
	s.record(ctx, auth.Event{Username: u.Username, UserID: &u.ID, Name: "mfa_reset", Detail: map[string]any{"by": actor}})
	return nil
}

// --- passkey list ---

type Passkey struct {
	ID         int64
	Name       string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

func (s *Service) Passkeys(ctx context.Context, userID int64) ([]Passkey, error) {
	rows, err := s.store.ListPasskeys(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Passkey, len(rows))
	for i, r := range rows {
		out[i] = Passkey{ID: r.ID, Name: r.Name, CreatedAt: r.CreatedAt, LastUsedAt: r.LastUsedAt}
	}
	return out, nil
}

func mustMarshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
