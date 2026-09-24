package identity

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
)

const passkeyChallengeTTL = 5 * time.Minute

// webauthnUser adapts a user and their stored credentials to the library.
type webauthnUser struct {
	u     db.User
	creds []webauthn.Credential
}

func (w webauthnUser) WebAuthnID() []byte { return w.u.WebauthnID }
func (w webauthnUser) WebAuthnName() string {
	return w.u.Username
}
func (w webauthnUser) WebAuthnDisplayName() string                { return displayName(w.u) }
func (w webauthnUser) WebAuthnCredentials() []webauthn.Credential { return w.creds }

// relyingParty is built from APP_ORIGIN: the RP ID is its host, and it is the
// only origin accepted. Passkeys are bound to it, so changing the host
// orphans them.
func (s *Service) relyingParty() (*webauthn.WebAuthn, error) {
	o, err := url.Parse(s.origin)
	if err != nil || o.Hostname() == "" {
		return nil, errors.New("APP_ORIGIN is not a URL: " + s.origin)
	}
	return webauthn.New(&webauthn.Config{
		RPID:          o.Hostname(),
		RPDisplayName: s.appName,
		RPOrigins:     []string{strings.TrimRight(s.origin, "/")},
	})
}

func (s *Service) loadWebAuthnUser(ctx context.Context, u db.User) (webauthnUser, error) {
	rows, err := s.store.ListPasskeys(ctx, u.ID)
	if err != nil {
		return webauthnUser{}, err
	}
	w := webauthnUser{u: u}
	for _, r := range rows {
		var c webauthn.Credential
		if err := json.Unmarshal(r.Data, &c); err != nil {
			return webauthnUser{}, err
		}
		w.creds = append(w.creds, c)
	}
	return w, nil
}

func (s *Service) newPasskeyChallenge(ctx context.Context, userID *int64, kind, client string, session *webauthn.SessionData, extra map[string]any) (string, error) {
	payload := map[string]any{"session": session}
	for k, v := range extra {
		payload[k] = v
	}
	token, hash, err := auth.NewToken()
	if err != nil {
		return "", err
	}
	_, err = s.store.CreateChallenge(ctx, db.CreateChallengeParams{
		UserID: userID, Kind: kind, TokenHash: hash, Client: client, Payload: mustMarshal(payload),
		ExpiresAt: time.Now().Add(passkeyChallengeTTL),
	})
	return token, err
}

type passkeyPayload struct {
	Session webauthn.SessionData `json:"session"`
	Name    string               `json:"name,omitempty"`
	Login   string               `json:"login,omitempty"` // the password step it finishes, if any
}

// takeChallenge loads and deletes a one-use WebAuthn challenge.
func (s *Service) takeChallenge(ctx context.Context, token, kind string) (db.AuthChallenge, passkeyPayload, error) {
	ch, err := s.store.GetChallenge(ctx, db.GetChallengeParams{TokenHash: auth.HashToken(token), Kind: kind})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.AuthChallenge{}, passkeyPayload{}, ledger.NotFound("passkey request")
	}
	if err != nil {
		return db.AuthChallenge{}, passkeyPayload{}, err
	}
	_, _ = s.store.DeleteChallenge(ctx, ch.ID)
	var p passkeyPayload
	if err := json.Unmarshal(ch.Payload, &p); err != nil {
		return db.AuthChallenge{}, passkeyPayload{}, err
	}
	return ch, p, nil
}

// BeginPasskeyRegistration returns the browser's creation options. Passkeys
// are discoverable and require user verification, so one alone is a
// two-factor sign-in (something held, plus a PIN or biometric).
func (s *Service) BeginPasskeyRegistration(ctx context.Context, u db.User, name string) (any, string, error) {
	if err := s.allowed(ctx, "passkey"); err != nil {
		return nil, "", err
	}
	if len(u.WebauthnID) == 0 {
		id := make([]byte, 32)
		if _, err := rand.Read(id); err != nil {
			return nil, "", err
		}
		got, err := s.store.SetWebAuthnID(ctx, db.SetWebAuthnIDParams{ID: u.ID, WebauthnID: id})
		if err != nil {
			return nil, "", err
		}
		u.WebauthnID = got
	}
	rp, err := s.relyingParty()
	if err != nil {
		return nil, "", err
	}
	wu, err := s.loadWebAuthnUser(ctx, u)
	if err != nil {
		return nil, "", err
	}
	exclude := make([]protocol.CredentialDescriptor, len(wu.creds))
	for i := range wu.creds {
		exclude[i] = wu.creds[i].Descriptor()
	}
	creation, session, err := rp.BeginRegistration(wu,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey: protocol.ResidentKeyRequirementRequired, UserVerification: protocol.VerificationRequired,
		}),
		webauthn.WithExclusions(exclude),
	)
	if err != nil {
		return nil, "", err
	}
	name = strings.TrimSpace(name)
	if len(name) > 60 {
		name = name[:60]
	}
	token, err := s.newPasskeyChallenge(ctx, &u.ID, "passkey_register", "web", session, map[string]any{"name": name})
	return creation, token, err
}

// FinishPasskeyRegistration verifies the browser's response (the request
// body) and stores the credential.
func (s *Service) FinishPasskeyRegistration(ctx context.Context, id auth.Identity, challenge string, r *http.Request, c auth.Client) ([]string, error) {
	ch, p, err := s.takeChallenge(ctx, challenge, "passkey_register")
	if err != nil {
		return nil, err
	}
	if ch.UserID == nil || *ch.UserID != id.User.ID {
		return nil, ledger.NotFound("passkey request")
	}
	u, err := s.store.GetUserByID(ctx, id.User.ID) // with the webauthn_id just set
	if err != nil {
		return nil, err
	}
	rp, err := s.relyingParty()
	if err != nil {
		return nil, err
	}
	wu, err := s.loadWebAuthnUser(ctx, u)
	if err != nil {
		return nil, err
	}
	cred, err := rp.FinishRegistration(wu, p.Session, r)
	if err != nil {
		return nil, ledger.Invalid("passkey_rejected", "the passkey could not be verified: %v", err)
	}
	name := p.Name
	if name == "" {
		name = "Passkey " + time.Now().Format("2006-01-02")
	}
	if _, err := s.store.CreatePasskey(ctx, db.CreatePasskeyParams{
		UserID: u.ID, CredentialID: cred.ID, Data: mustMarshal(cred), Name: name,
	}); err != nil {
		return nil, ledger.Translate(err, "passkey")
	}
	return s.enrolled(ctx, id, "passkey", c)
}

// BeginPasskeyLogin starts a sign-in with a passkey. Without a login
// challenge it is passwordless (any of this site's passkeys); with one it is
// the second step after the password, and must be that user's passkey.
func (s *Service) BeginPasskeyLogin(ctx context.Context, login, client string, c auth.Client) (any, string, error) {
	if err := s.auth.Check(ctx, "", c.IP); err != nil {
		return nil, "", err
	}
	if err := s.allowed(ctx, "passkey"); err != nil {
		return nil, "", err
	}
	var uid *int64
	if login != "" {
		ch, u, err := s.loginChallenge(ctx, login)
		if err != nil {
			return nil, "", err
		}
		uid, client = &u.ID, ch.Client
	}
	rp, err := s.relyingParty()
	if err != nil {
		return nil, "", err
	}
	assertion, session, err := rp.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return nil, "", err
	}
	if client != "api" {
		client = "web"
	}
	token, err := s.newPasskeyChallenge(ctx, uid, "passkey_login", client, session, map[string]any{"login": login})
	return assertion, token, err
}

// FinishPasskeyLogin verifies the assertion and opens a two-factor session.
func (s *Service) FinishPasskeyLogin(ctx context.Context, challenge string, r *http.Request, c auth.Client) (db.User, string, string, error) {
	ch, p, err := s.takeChallenge(ctx, challenge, "passkey_login")
	if err != nil {
		return db.User{}, "", "", err
	}
	rp, err := s.relyingParty()
	if err != nil {
		return db.User{}, "", "", err
	}
	var owner db.User
	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		u, err := s.store.GetUserByWebAuthnID(ctx, userHandle)
		if err != nil {
			return nil, err
		}
		owner = u
		return s.loadWebAuthnUser(ctx, u)
	}
	_, cred, err := rp.FinishPasskeyLogin(handler, p.Session, r)
	fail := func() (db.User, string, string, error) {
		s.record(ctx, auth.Event{Username: owner.Username, IP: c.IP, UserAgent: c.UserAgent, Name: "passkey_bad", Failure: true})
		return db.User{}, "", "", ledger.Invalid("passkey_rejected", "the passkey was not accepted")
	}
	if err != nil || !owner.IsActive {
		return fail()
	}
	// The second step after a password must be that user's own passkey.
	if ch.UserID != nil && *ch.UserID != owner.ID {
		return fail()
	}
	if row, err := s.store.GetPasskeyByCredentialID(ctx, cred.ID); err == nil {
		_ = s.store.TouchPasskey(ctx, db.TouchPasskeyParams{ID: row.ID, Data: mustMarshal(cred)})
	}
	if p.Login != "" {
		if lc, _, err := s.loginChallenge(ctx, p.Login); err == nil {
			_, _ = s.store.DeleteChallenge(ctx, lc.ID)
		}
	}
	s.record(ctx, auth.Event{Username: owner.Username, UserID: &owner.ID, IP: c.IP, UserAgent: c.UserAgent, Name: "passkey_ok"})
	token, err := s.openSession(ctx, owner, ch.Client, c, auth.AALMFA)
	return owner, token, ch.Client, err
}
