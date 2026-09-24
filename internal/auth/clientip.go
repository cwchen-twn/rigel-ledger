package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ParseCIDRs reads a comma-separated list such as "10.42.0.0/16,127.0.0.1/32".
// A bare address counts as a single-host prefix.
func ParseCIDRs(list string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, s := range strings.Split(list, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !strings.Contains(s, "/") {
			a, err := netip.ParseAddr(s)
			if err != nil {
				return nil, fmt.Errorf("trusted proxy %q: %w", s, err)
			}
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
			continue
		}
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, fmt.Errorf("trusted proxy %q: %w", s, err)
		}
		out = append(out, p.Masked())
	}
	return out, nil
}

// ClientIP finds the address a request came from. The socket peer is used
// unless it is a trusted proxy; then X-Forwarded-For is walked from the
// right, skipping trusted hops, and the first untrusted entry is the client.
//
// Rightmost, not leftmost: anything left of the first hop we trust was
// written by the client and can be forged. (hcloud's vaultwarden learned the
// same lesson with Cloudflare in front: the edge must be listed as trusted,
// or the edge's address is what gets reported.)
func ClientIP(r *http.Request, trusted []netip.Prefix) netip.Addr {
	peer := remoteAddr(r)
	if !peer.IsValid() || !isTrusted(peer, trusted) {
		return peer
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			// A garbled hop: stop trusting the chain here.
			return peer
		}
		a = a.Unmap()
		if !isTrusted(a, trusted) {
			return a
		}
		peer = a
	}
	return peer
}

func remoteAddr(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return a.Unmap()
}

func isTrusted(a netip.Addr, trusted []netip.Prefix) bool {
	for _, p := range trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

type ipKey struct{}

// ResolveClientIP puts the client address in the request context for the
// throttle, the audit and the session list.
func ResolveClientIP(trusted []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			a := ClientIP(r, trusted)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ipKey{}, a)))
		})
	}
}

// IPFrom returns the resolved client address, or nil when unknown.
func IPFrom(ctx context.Context) *netip.Addr {
	a, ok := ctx.Value(ipKey{}).(netip.Addr)
	if !ok || !a.IsValid() {
		return nil
	}
	return &a
}
