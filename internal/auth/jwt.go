package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type JWT struct {
	issuer   string
	audience []string
	alg      jwt.SigningMethod
	privKey  []byte
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
	TokenDataKey   contextKey = "tokenData"
	AccessTokenKey contextKey = "accessToken"

	AccessTokenCookieName  = "rigel_jwt_access"
	RefreshTokenCookieName = "rigel_jwt_refresh"
	AccessTokenLifetime    = 15 * time.Minute
	RefreshTokenLifetime   = 24 * time.Hour
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

func New(issuer string, audience []string, privKey []byte) *JWT {
	alg := jwt.SigningMethodHS256

	return &JWT{
		issuer:   issuer,
		audience: audience,
		alg:      alg,
		privKey:  privKey,
	}
}

// Sign signs a JWT token with the given subject and claims.
// subject is the subject of the token, i.e., the user ID.
// claims is the claims of the token, any additional data to be added to the token.
// lifetime is the lifetime of the token, i.e. how long the token is valid for.
func (j *JWT) Sign(subject string, claims map[string]any, lifetime time.Duration) ([]byte, error) {

	mc := jwt.MapClaims{
		"jti": uuid.New().String(), // JWT ID
		"iss": j.issuer,
		"aud": j.audience,
		"sub": subject,
		"iat": time.Now().Unix(),               // IssuedAt
		"nbf": time.Now().Unix(),               // NotBefore
		"exp": time.Now().Add(lifetime).Unix(), // ExpirationTime
	}

	maps.Copy(mc, claims)

	token := jwt.NewWithClaims(j.alg, mc)
	signed, err := token.SignedString(j.privKey)

	if err != nil {
		return nil, err
	}

	return []byte(signed), nil
}

func (j *JWT) Verify(token []byte) (*TokenData, error) {
	parsed, err := jwt.Parse(string(token), func(token *jwt.Token) (any, error) {
		return j.privKey, nil
	})
	if err != nil {
		return nil, err
	}

	mc, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidTokenFormat
	}

	jwtID, ok := mc["jti"].(string)
	if !ok {
		return nil, ErrJwtIDIsNil
	}

	issuer, err := mc.GetIssuer()
	if err != nil {
		return nil, ErrIssuerIsNil
	}

	audience, err := mc.GetAudience()
	if err != nil {
		return nil, ErrAudienceIsNil
	}

	subject, err := mc.GetSubject()
	if err != nil {
		return nil, ErrSubjectIsNil
	}

	claims := make(map[string]any)
	for k, v := range mc {
		claims[k] = v
	}

	expiry, err := mc.GetExpirationTime()
	if err != nil {
		return nil, ErrExpiryIsNil
	}

	issuedAt, err := mc.GetIssuedAt()
	if err != nil {
		return nil, ErrIssuedAtIsNil
	}

	notBefore, err := mc.GetNotBefore()
	if err != nil {
		return nil, ErrNotBeforeIsNil
	}

	return &TokenData{
		JwtID:     jwtID,
		Issuer:    issuer,
		Audience:  audience,
		Subject:   subject,
		Claims:    claims,
		Expiry:    expiry.Time,
		IssuedAt:  issuedAt.Time,
		NotBefore: notBefore.Time,
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
	cookie, err := r.Cookie(AccessTokenCookieName)
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
}

func JWTExtractTokenMiddleware(j *JWT) func(http.Handler) http.Handler {
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

			ctx = context.WithValue(ctx, AccessTokenKey, token)
			next.ServeHTTP(w, r.WithContext(ctx))
		}

		return http.HandlerFunc(handler)
	}
}

func JWTValidateTokenMiddleware(j *JWT) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {

		handler := func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			accessToken, ok := ctx.Value(AccessTokenKey).(string)
			if !ok {
				// http.Error(w, "Unauthorized", http.StatusUnauthorized)
				errMessage := "Session expired. Please login again."
				http.Redirect(w, r, fmt.Sprintf("/login?error=%s", errMessage), http.StatusSeeOther)
				return
			}

			tokenData, err := j.Verify([]byte(accessToken))

			if err != nil {
				// http.Error(w, err.Error(), http.StatusUnauthorized)
				errMessage := "Unauthorized. Please login again."
				http.Redirect(w, r, fmt.Sprintf("/login?error=%s", errMessage), http.StatusSeeOther)
				return
			}

			username := chi.URLParam(r, "username")
			if username != tokenData.Subject {
				// http.Error(w, "Unauthorized", http.StatusUnauthorized)
				errMessage := "Unauthorized. Please login again."
				http.Redirect(w, r, fmt.Sprintf("/login?error=%s", errMessage), http.StatusSeeOther)
				return
			}

			ctx = context.WithValue(ctx, TokenDataKey, tokenData)
			next.ServeHTTP(w, r.WithContext(ctx))
		}

		return http.HandlerFunc(handler)
	}
}
