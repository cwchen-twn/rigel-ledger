package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

type JWT struct {
	issuer   string
	audience []string
	alg      jwa.SignatureAlgorithm
	privKey  any
	verifier jwt.SignEncryptParseOption
}

type TokenData struct {
	JwtID     string
	Issuer    string
	Audience  []string
	Subject   string
	Claims    map[string]any
	Expiry    time.Time
	IssuedAt  time.Time
	NotBefore time.Time
}

func (td *TokenData) ToJSONString() (string, error) {
	json, err := json.Marshal(td)
	if err != nil {
		return "", err
	}
	return string(json), nil
}

type contextKey string

const (
	tokenDataKey contextKey = "tokenData"
)

var (
	AccessTokenLifetime  = 15 * time.Minute
	RefreshTokenLifetime = 24 * time.Hour
)

var (
	ErrJwtIDIsNil         = errors.New("jwt id is nil")
	ErrIssuerIsNil        = errors.New("issuer is nil")
	ErrAudienceIsNil      = errors.New("audience is nil")
	ErrSubjectIsNil       = errors.New("subject is nil")
	ErrExpiryIsNil        = errors.New("expiry is nil")
	ErrIssuedAtIsNil      = errors.New("issued at is nil")
	ErrNotBeforeIsNil     = errors.New("not before is nil")
	ErrInvalidTokenFormat = errors.New("invalid token format")

	ErrExpired      = errors.New("token is expired")
	ErrNBFInvalid   = errors.New("token nbf validation failed")
	ErrIATInvalid   = errors.New("token iat validation failed")
	ErrNoTokenFound = errors.New("no token found")
)

func New(issuer string, audience []string, privKey []byte, options ...jwt.Option) *JWT {
	alg := jwa.HS256()

	return &JWT{
		issuer:   issuer,
		audience: audience,
		alg:      alg,
		privKey:  privKey,
		verifier: jwt.WithKey(alg, privKey, options...),
	}
}

// Sign signs a JWT token with the given subject and claims.
// subject is the subject of the token, i.e., the user ID.
// claims is the claims of the token, any additional data to be added to the token.
// lifetime is the lifetime of the token, i.e. how long the token is valid for.
func (j *JWT) Sign(subject string, claims map[string]any, lifetime time.Duration) ([]byte, error) {
	jwtID := uuid.New().String()

	token, err := jwt.NewBuilder().
		Issuer(j.issuer).
		JwtID(jwtID).
		Subject(subject).
		Audience(j.audience).
		IssuedAt(time.Now()).
		NotBefore(time.Now()).
		Expiration(time.Now().Add(lifetime)).
		Build()
	if err != nil {
		return nil, err
	}

	for k, v := range claims {
		if err := token.Set(k, v); err != nil {
			return nil, err
		}
	}

	signed, err := jwt.Sign(token, j.verifier)
	if err != nil {
		return nil, err
	}
	return signed, nil
}

func (j *JWT) Verify(token []byte) (*TokenData, error) {
	parsed, err := jwt.Parse(token, j.verifier, jwt.WithValidate(true))
	if err != nil {
		return nil, err
	}

	jwtID, ok := parsed.JwtID()
	if !ok {
		return nil, ErrJwtIDIsNil
	}

	issuer, ok := parsed.Issuer()
	if !ok {
		return nil, ErrIssuerIsNil
	}

	audience, ok := parsed.Audience()
	if !ok {
		return nil, ErrAudienceIsNil
	}

	subject, ok := parsed.Subject()
	if !ok {
		return nil, ErrSubjectIsNil
	}

	claims := make(map[string]any)
	for _, k := range parsed.Keys() {
		var v any
		if err := parsed.Get(k, &v); err != nil {
			return nil, err
		}
		claims[k] = v
	}

	expiry, ok := parsed.Expiration()
	if !ok {
		return nil, ErrExpiryIsNil
	}

	issuedAt, ok := parsed.IssuedAt()
	if !ok {
		return nil, ErrIssuedAtIsNil
	}

	notBefore, ok := parsed.NotBefore()
	if !ok {
		return nil, ErrNotBeforeIsNil
	}

	return &TokenData{
		JwtID:     jwtID,
		Issuer:    issuer,
		Audience:  audience,
		Subject:   subject,
		Claims:    claims,
		Expiry:    expiry,
		IssuedAt:  issuedAt,
		NotBefore: notBefore,
	}, nil
}

func (j *JWT) getTokenFromRequest(r *http.Request) (string, error) {
	token := r.Header.Get("Authorization")
	if token == "" {
		return "", ErrNoTokenFound
	}

	parts := strings.Split(token, " ")
	if len(parts) != 2 {
		return "", ErrInvalidTokenFormat
	}

	if strings.ToLower(parts[0]) != "bearer" {
		return "", ErrInvalidTokenFormat
	}

	return parts[1], nil
}

func (j *JWT) getTokenFromCookie(r *http.Request) (string, error) {
	cookie, err := r.Cookie("rigel_jwt_access")
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
}

func (j *JWT) errorMapper(err error) error {
	switch {
	case errors.Is(err, jwt.TokenExpiredError()), err == ErrExpired:
		return ErrExpired
	case errors.Is(err, jwt.InvalidIssuedAtError()), err == ErrIATInvalid:
		return ErrIATInvalid
	case errors.Is(err, jwt.TokenNotYetValidError()), err == ErrNBFInvalid:
		return ErrNBFInvalid
	default:
		return err
	}
}

func JWTMiddleware(j *JWT) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {

		findTokenFns := []func(r *http.Request) (string, error){
			j.getTokenFromRequest,
			j.getTokenFromCookie,
		}

		handler := func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			var token string
			var err error
			for _, fn := range findTokenFns {
				token, err = fn(r)
				if err == nil {
					break
				}
			}

			if err != nil {
				//http.Error(w, err.Error(), http.StatusUnauthorized)
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}

			tokenData, err := j.Verify([]byte(token))
			if err != nil {
				//http.Error(w, j.errorMapper(err).Error(), http.StatusUnauthorized)
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}

			ctx = context.WithValue(ctx, tokenDataKey, tokenData)
			next.ServeHTTP(w, r.WithContext(ctx))
		}

		return http.HandlerFunc(handler)
	}
}
