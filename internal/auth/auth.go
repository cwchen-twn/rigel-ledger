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

func CheckPassword(hash, password string) bool {
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
	ttl          time.Duration
	secureCookie bool
}

func NewManager(store *db.Store, ttl time.Duration, secureCookie bool) *Manager {
	return &Manager{store: store, ttl: ttl, secureCookie: secureCookie}
}

// Login checks the password and opens a session of the given kind ("web" or "api").
func (m *Manager) Login(ctx context.Context, username, password, kind, userAgent string) (db.User, string, error) {
	u, err := m.store.GetUserByUsername(ctx, strings.ToLower(strings.TrimSpace(username)))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !u.IsActive) {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return db.User{}, "", ErrInvalidCredentials
	}
	if err != nil {
		return db.User{}, "", err
	}
	if !CheckPassword(u.PasswordHash, password) {
		return db.User{}, "", ErrInvalidCredentials
	}

	token, hash, err := NewToken()
	if err != nil {
		return db.User{}, "", err
	}
	if _, err := m.store.CreateSession(ctx, db.CreateSessionParams{
		UserID:    u.ID,
		TokenHash: hash,
		Kind:      kind,
		UserAgent: userAgent,
		ExpiresAt: time.Now().Add(m.ttl),
	}); err != nil {
		return db.User{}, "", err
	}
	_ = m.store.TouchUserLogin(ctx, u.ID)
	return u, token, nil
}

func (m *Manager) Logout(ctx context.Context, token string) error {
	return m.store.DeleteSessionByTokenHash(ctx, HashToken(token))
}

func (m *Manager) SetCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(m.ttl.Seconds()),
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
				ExpiresAt: time.Now().Add(m.ttl),
			})
			if viaCookie {
				m.SetCookie(w, token)
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
