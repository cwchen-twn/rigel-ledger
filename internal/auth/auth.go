// Package auth implements login sessions with opaque tokens.
//
// A token is 32 random bytes, handed to the browser as the rigel_session
// cookie or to scripts and mobile clients as a bearer token. Only its SHA-256
// is stored, so the sessions table cannot be replayed from a backup, and a
// session is revoked by deleting its row -- no signing keys, no refresh dance.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

const (
	CookieName = "rigel_session"
	// ClientHeader must accompany every cookie-authenticated unsafe request.
	// A cross-site page cannot set a custom header without a CORS preflight,
	// and the server answers no preflight, so this is the CSRF defence.
	ClientHeader = "X-Rigel-Client"

	touchEvery = time.Minute
)

var ErrInvalidCredentials = errors.New("invalid username or password")

// A bcrypt hash compared against when the username does not exist, so a
// login for an unknown user costs the same as a wrong password.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("rigel-ledger-timing-guard"), bcrypt.DefaultCost)

func HashPassword(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(h), err
}

// CheckPassword reports whether password matches hash. An empty hash (an
// invited user who has not chosen a password) never matches, and costs the
// same bcrypt time as a real comparison, so it is not a tell.
func CheckPassword(hash, password string) bool {
	if hash == "" {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// NewToken returns a fresh token and the hash to store for it.
func NewToken() (token string, hash []byte, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token), nil
}

func HashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

type Manager struct {
	store        *db.Store
	ttl          time.Duration // SESSION_TTL; the policy may override it
	secureCookie bool
	policy       Policy
}

func NewManager(store *db.Store, ttl time.Duration, secureCookie bool) *Manager {
	return &Manager{store: store, ttl: ttl, secureCookie: secureCookie}
}

// SetPolicy makes the throttle limits and the session lifetime follow the
// system settings instead of the built-in defaults.
func (m *Manager) SetPolicy(p Policy) { m.policy = p }

func (m *Manager) sessionTTL(ctx context.Context) time.Duration {
	if m.policy != nil {
		if d := m.policy.SessionTTL(ctx); d > 0 {
			return d
		}
	}
	return m.ttl
}

// Client describes where a request came from, for sessions and the audit.
type Client struct {
	IP        *netip.Addr
	UserAgent string
}

// VerifyPassword checks the throttle, then the password. Every outcome is
// recorded in auth_events. It opens no session: the caller decides whether a
// second factor is needed first.
func (m *Manager) VerifyPassword(ctx context.Context, username, password string, c Client) (db.User, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if err := m.Check(ctx, username, c.IP); err != nil {
		if _, ok := IsThrottled(err); ok {
			_ = m.Record(ctx, Event{Username: username, IP: c.IP, UserAgent: c.UserAgent, Name: "throttled"})
		}
		return db.User{}, err
	}
	fail := func(uid *int64) (db.User, error) {
		_ = m.Record(ctx, Event{Username: username, UserID: uid, IP: c.IP, UserAgent: c.UserAgent,
			Name: "password_bad", Failure: true})
		return db.User{}, ErrInvalidCredentials
	}
	u, err := m.store.GetUserByUsername(ctx, username)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !u.IsActive) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		if err == nil {
			return fail(&u.ID)
		}
		return fail(nil)
	}
	if err != nil {
		return db.User{}, err
	}
	if !CheckPassword(u.PasswordHash, password) {
		return fail(&u.ID)
	}
	_ = m.Record(ctx, Event{Username: username, UserID: &u.ID, IP: c.IP, UserAgent: c.UserAgent, Name: "password_ok"})
	return u, nil
}

// Session assurance levels.
const (
	AALPassword = 1 // a password or an emailed link
	AALMFA      = 2 // plus a second factor, or a passkey with user verification
)

// OpenSession creates a session at assurance level aal for a user who has
// already proved who they are, records "signed_in" (which also marks the
// address as the owner's, see Check), and returns the token.
func (m *Manager) OpenSession(ctx context.Context, u db.User, kind string, c Client, aal int16) (string, error) {
	token, hash, err := NewToken()
	if err != nil {
		return "", err
	}
	if _, err := m.store.CreateSessionAAL(ctx, db.CreateSessionAALParams{
		UserID:    u.ID,
		TokenHash: hash,
		Kind:      kind,
		UserAgent: c.UserAgent,
		ExpiresAt: time.Now().Add(m.sessionTTL(ctx)),
		Ip:        c.IP,
		Aal:       aal,
	}); err != nil {
		return "", err
	}
	_ = m.store.TouchUserLogin(ctx, u.ID)
	_ = m.Record(ctx, Event{Username: u.Username, UserID: &u.ID, IP: c.IP, UserAgent: c.UserAgent,
		Name: "signed_in", Detail: map[string]any{"kind": kind, "aal": aal}})
	return token, nil
}

func (m *Manager) Logout(ctx context.Context, token string) error {
	return m.store.DeleteSessionByTokenHash(ctx, HashToken(token))
}

func (m *Manager) SetCookie(ctx context.Context, w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(m.sessionTTL(ctx).Seconds()),
		HttpOnly: true,
		Secure:   m.secureCookie,
		SameSite: http.SameSiteLaxMode,
	})
}

func (m *Manager) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   m.secureCookie,
		SameSite: http.SameSiteLaxMode,
	})
}

// Identity is what the middleware puts in the request context.
type Identity struct {
	User      db.User
	SessionID int64
	Token     string
	ViaCookie bool
	AAL       int16
	Kind      string // web (browser), api (a script's login) or token (made in settings)
}

type ctxKey struct{}

func FromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}

func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// tokenFrom prefers the Authorization header, so a script with a bearer token
// is never mistaken for a browser.
func tokenFrom(r *http.Request) (string, bool) {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer ")), false
	}
	if c, err := r.Cookie(CookieName); err == nil && c.Value != "" {
		return c.Value, true
	}
	return "", false
}

// Authenticate resolves the session, if any, and slides its expiry. It never
// rejects a request; RequireUser does that.
func (m *Manager) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, viaCookie := tokenFrom(r)
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}
		row, err := m.store.GetSessionUser(r.Context(), HashToken(token))
		if err != nil {
			if viaCookie {
				m.ClearCookie(w)
			}
			next.ServeHTTP(w, r)
			return
		}
		if time.Since(row.Session.LastUsedAt) > touchEvery {
			expires := time.Now().Add(m.sessionTTL(r.Context()))
			if row.Session.Kind == KindToken {
				expires = row.Session.ExpiresAt // a token lives as long as it was made for, no longer
			}
			_ = m.store.TouchSession(r.Context(), db.TouchSessionParams{ID: row.Session.ID, ExpiresAt: expires})
			if viaCookie {
				m.SetCookie(r.Context(), w, token)
			}
		}
		ctx := WithIdentity(r.Context(), Identity{
			User:      row.User,
			SessionID: row.Session.ID,
			Token:     token,
			ViaCookie: viaCookie,
			AAL:       row.Session.Aal,
			Kind:      row.Session.Kind,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}

// RequireUser rejects anonymous requests with 401, and cookie-authenticated
// unsafe requests that lack ClientHeader with 403. onError writes the body so
// this package stays out of the JSON error format.
func RequireUser(onError func(w http.ResponseWriter, status int, code string)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, ok := FromContext(r.Context())
			if !ok {
				onError(w, http.StatusUnauthorized, "unauthenticated")
				return
			}
			if id.ViaCookie && !isSafeMethod(r.Method) && r.Header.Get(ClientHeader) == "" {
				onError(w, http.StatusForbidden, "csrf")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireInitialized keeps a user who has not finished the first-login
// wizard (or still has to replace a bootstrap password) inside it: every
// route behind this answers 403 onboarding_required until they do.
func RequireInitialized(onError func(w http.ResponseWriter, status int, code string)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, _ := FromContext(r.Context())
			if id.User.InitializedAt == nil || id.User.PasswordMustChange {
				onError(w, http.StatusForbidden, "onboarding_required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAdmin answers 404 to non-admins, the same as a route that does not
// exist, so the admin API does not advertise itself.
func RequireAdmin(onError func(w http.ResponseWriter, status int, code string)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, _ := FromContext(r.Context())
			if !id.User.IsAdmin {
				onError(w, http.StatusNotFound, "not_found")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireMFA keeps a password-only session (aal 1) out while the system
// requires two-factor sign-in: 403 mfa_enrollment_required until a factor is
// enrolled (which raises the session) or the user signs in again with one.
func (m *Manager) RequireMFA(onError func(w http.ResponseWriter, status int, code string)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, _ := FromContext(r.Context())
			if id.AAL < AALMFA && m.policy != nil && m.policy.MFARequired(r.Context()) {
				onError(w, http.StatusForbidden, "mfa_enrollment_required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Session kinds. A web session slides and rides a cookie; an api one is a
// script's full login; a token is made in settings for a runner and is
// confined by TokenScope.
const (
	KindWeb   = "web"
	KindAPI   = "api"
	KindToken = "token"
)

// MaxTokenTTL caps an API token's lifetime: a runner's token is rotated at
// least once every two years.
const MaxTokenTTL = 2 * 365 * 24 * time.Hour

// CreateToken makes an API token for a runner or a script. Only a two-factor
// session may make one, and the token carries that level, so the runner is
// not asked for a second factor it cannot give. It is shown once.
func (m *Manager) CreateToken(ctx context.Context, u db.User, label string, ttl time.Duration, c Client) (string, db.Session, error) {
	if ttl <= 0 || ttl > MaxTokenTTL {
		ttl = MaxTokenTTL / 2
	}
	token, hash, err := NewToken()
	if err != nil {
		return "", db.Session{}, err
	}
	s, err := m.store.CreateSessionAAL(ctx, db.CreateSessionAALParams{
		UserID: u.ID, TokenHash: hash, Kind: KindToken, Label: label, UserAgent: c.UserAgent,
		ExpiresAt: time.Now().Add(ttl), Ip: c.IP, Aal: AALMFA,
	})
	if err != nil {
		return "", db.Session{}, err
	}
	_ = m.Record(ctx, Event{Username: u.Username, UserID: &u.ID, IP: c.IP, UserAgent: c.UserAgent,
		Name: "token_created", Detail: map[string]any{"label": label, "session_id": s.ID}})
	return token, s, nil
}

// TokenScope confines API tokens to what a runner needs: who am I, which
// books, and sending a batch. Everything else -- the review queue, settings,
// making more tokens -- needs a person signed in, so a leaked runner token
// cannot read the books or approve its own rows.
func TokenScope(allowed func(method, path string) bool, onError func(w http.ResponseWriter, status int, code string)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if id, _ := FromContext(r.Context()); id.Kind == KindToken && !allowed(r.Method, r.URL.Path) {
				onError(w, http.StatusForbidden, "token_scope")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RaiseSession marks the current session as having passed a second factor.
func (m *Manager) RaiseSession(ctx context.Context, sessionID int64) error {
	return m.store.RaiseSessionAAL(ctx, sessionID)
}
