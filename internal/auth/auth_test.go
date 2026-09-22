package auth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewTokenHashesDeterministically(t *testing.T) {
	token, hash, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 43 { // 32 bytes, unpadded base64url
		t.Fatalf("token length = %d, want 43", len(token))
	}
	if !bytes.Equal(hash, HashToken(token)) {
		t.Fatal("HashToken does not reproduce the stored hash")
	}
	other, _, _ := NewToken()
	if other == token {
		t.Fatal("two tokens are equal")
	}
}

func TestRequireUser(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	onError := func(w http.ResponseWriter, status int, _ string) { w.WriteHeader(status) }
	h := RequireUser(onError)(ok)

	cases := []struct {
		name   string
		id     *Identity
		method string
		header bool
		want   int
	}{
		{"anonymous", nil, http.MethodGet, false, http.StatusUnauthorized},
		{"cookie GET", &Identity{ViaCookie: true}, http.MethodGet, false, http.StatusNoContent},
		{"cookie POST without header", &Identity{ViaCookie: true}, http.MethodPost, false, http.StatusForbidden},
		{"cookie POST with header", &Identity{ViaCookie: true}, http.MethodPost, true, http.StatusNoContent},
		{"bearer POST without header", &Identity{ViaCookie: false}, http.MethodPost, false, http.StatusNoContent},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(c.method, "/api/x", nil)
			if c.header {
				r.Header.Set(ClientHeader, "web")
			}
			if c.id != nil {
				r = r.WithContext(WithIdentity(r.Context(), *c.id))
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != c.want {
				t.Fatalf("status = %d, want %d", w.Code, c.want)
			}
		})
	}
}
