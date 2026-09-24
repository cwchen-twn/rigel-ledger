package auth

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	trusted, err := ParseCIDRs("10.42.0.0/16, 127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, remote, xff, want string
	}{
		{"direct, header ignored", "100.99.1.2:5000", "1.2.3.4", "100.99.1.2"},
		{"via traefik", "10.42.0.7:5000", "100.99.1.2", "100.99.1.2"},
		{"forged left entry is skipped", "10.42.0.7:5000", "6.6.6.6, 100.99.1.2", "100.99.1.2"},
		{"trusted hops walked", "10.42.0.7:5000", "100.99.1.2, 10.42.0.9", "100.99.1.2"},
		{"no header from a proxy", "10.42.0.7:5000", "", "10.42.0.7"},
		{"garbage stops the walk", "10.42.0.7:5000", "nonsense", "10.42.0.7"},
		{"ipv6 peer", "[fd7a:115c:a1e0::1]:443", "", "fd7a:115c:a1e0::1"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remote
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := ClientIP(r, trusted).String(); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
	if _, err := ParseCIDRs("10.0.0.0/33"); err == nil {
		t.Error("bad CIDR accepted")
	}
}
