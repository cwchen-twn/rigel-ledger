package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

// Limits are the sign-in throttle's thresholds: failures tolerated inside
// Window per (username, ip), per ip, and per username from anywhere.
type Limits struct {
	UserIP int
	IP     int
	User   int
	Window time.Duration
}

var DefaultLimits = Limits{UserIP: 5, IP: 20, User: 20, Window: 15 * time.Minute}

// Policy supplies settings an admin can change at runtime. The system
// settings service implements it; tests may leave it nil for the defaults.
type Policy interface {
	Limits(ctx context.Context) Limits
	SessionTTL(ctx context.Context) time.Duration
}

// ThrottledError means too many recent failures; retry after RetryAfter.
type ThrottledError struct{ RetryAfter time.Duration }

func (e *ThrottledError) Error() string {
	return fmt.Sprintf("too many attempts; retry in %s", e.RetryAfter.Round(time.Second))
}

func IsThrottled(err error) (*ThrottledError, bool) {
	var te *ThrottledError
	ok := errors.As(err, &te)
	return te, ok
}

func (m *Manager) limits(ctx context.Context) Limits {
	if m.policy != nil {
		return m.policy.Limits(ctx)
	}
	return DefaultLimits
}

// Check refuses with *ThrottledError when username (may be "") or ip has
// used up its failures inside the window. It is called BEFORE the password
// is checked, so a locked-out guesser learns nothing, and an unknown
// username is throttled exactly like a real one.
func (m *Manager) Check(ctx context.Context, username string, ip *netip.Addr) error {
	l := m.limits(ctx)
	row, err := m.store.CountFailures(ctx, db.CountFailuresParams{
		Username: username, Ip: ip, WindowSeconds: int64(l.Window.Seconds()),
	})
	if err != nil {
		return err
	}
	over := (username != "" && ip != nil && row.UserIp >= int64(l.UserIP)) ||
		(ip != nil && row.Ip >= int64(l.IP)) ||
		(username != "" && row.Username >= int64(l.User))
	if !over {
		return nil
	}
	// The oldest counted failure leaving the window is the earliest moment
	// anything can change. Retrying then may still be refused, with a new
	// Retry-After, while newer failures remain.
	wait := time.Until(row.Oldest.Add(l.Window))
	if wait < time.Second {
		wait = time.Second
	}
	return &ThrottledError{RetryAfter: wait}
}

// Event is one line of the security audit. Failure marks it as counting
// against the throttle: bad passwords and codes, and also anonymous actions
// that cost the server (sign-ups, access requests), so one address cannot
// flood them.
type Event struct {
	Username  string
	UserID    *int64
	IP        *netip.Addr
	UserAgent string
	Name      string
	Failure   bool
	Detail    map[string]any
}

// Record writes e. The audit must never break the request it describes, so
// an error is only returned for the caller to log.
func (m *Manager) Record(ctx context.Context, e Event) error {
	detail := []byte("{}")
	if len(e.Detail) > 0 {
		if b, err := json.Marshal(e.Detail); err == nil {
			detail = b
		}
	}
	return m.store.RecordAuthEvent(ctx, db.RecordAuthEventParams{
		Username: e.Username, UserID: e.UserID, Ip: e.IP, UserAgent: e.UserAgent,
		Event: e.Name, Failure: e.Failure, Detail: detail,
	})
}
