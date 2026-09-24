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

// Login checks the throttle, then the password, and opens a session of the
// given kind ("web" or "api"). Every outcome is recorded in auth_events.
func (m *Manager) Login(ctx context.Context, username, password, kind string, c Client) (db.User, string, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if err := m.Check(ctx, username, c.IP); err != nil {
		if _, ok := IsThrottled(err); ok {
			_ = m.Record(ctx, Event{Username: username, IP: c.IP, UserAgent: c.UserAgent, Name: "throttled"})
		}
		return db.User{}, "", err
	}
	fail := func(uid *int64) (db.User, string, error) {
		_ = m.Record(ctx, Event{Username: username, UserID: uid, IP: c.IP, UserAgent: c.UserAgent,
			Name: "password_bad", Failure: true})
		return db.User{}, "", ErrInvalidCredentials
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
		return db.User{}, "", err
	}
	if !CheckPassword(u.PasswordHash, password) {
		return fail(&u.ID)
	}

	token, err := m.OpenSession(ctx, u.ID, kind, c)
	if err != nil {
		return db.User{}, "", err
	}
	_ = m.store.TouchUserLogin(ctx, u.ID)
	_ = m.Record(ctx, Event{Username: username, UserID: &u.ID, IP: c.IP, UserAgent: c.UserAgent,
		Name: "password_ok", Detail: map[string]any{"kind": kind}})
	return u, token, nil
}

// OpenSession creates a session for a user who has already proved who they
// are (a password, an invitation or a sign-up link) and returns its token.
func (m *Manager) OpenSession(ctx context.Context, userID int64, kind string, c Client) (string, error) {
	token, hash, err := NewToken()
	if err != nil {
		return "", err
	}
	if _, err := m.store.CreateSessionWithIP(ctx, db.CreateSessionWithIPParams{
		UserID:    userID,
		TokenHash: hash,
		Kind:      kind,
		UserAgent: c.UserAgent,
		ExpiresAt: time.Now().Add(m.sessionTTL(ctx)),
		Ip:        c.IP,
	}); err != nil {
		return "", err
	}
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
			_ = m.store.TouchSession(r.Context(), db.TouchSessionParams{
				ID:        row.Session.ID,
				ExpiresAt: time.Now().Add(m.sessionTTL(r.Context())),
			})
			if viaCookie {
				m.SetCookie(r.Context(), w, token)
			}
		}
		ctx := WithIdentity(r.Context(), Identity{
			User:      row.User,
			SessionID: row.Session.ID,
			Token:     token,
			ViaCookie: viaCookie,
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
