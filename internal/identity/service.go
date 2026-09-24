// Package identity owns everything about who may use the app: the system
// settings an admin edits, outgoing mail, the bootstrap admin, invitations,
// registration and access requests, email verification, the first-login
// wizard, and the admin's user list.
//
// It shares ledger.Error with the bookkeeping rules, so handlers map every
// failure the same way; throttling surfaces as *auth.ThrottledError.
package identity

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
	"github.com/cwchen-twn/rigel-ledger/internal/mail"
	"github.com/cwchen-twn/rigel-ledger/internal/secretbox"
)

type Options struct {
	Store   *db.Store
	Ledger  *ledger.Service
	Auth    *auth.Manager
	Box     *secretbox.Box
	Logger  *slog.Logger
	AppName string
	// Origin is the public base URL ("https://ledger.example.com") that
	// links in mail point at.
	Origin string
	// Sender, when set, replaces the one built from the mail settings.
	// Tests use it to read the codes and links that were sent.
	Sender mail.Sender
}

type Service struct {
	store   *db.Store
	ledger  *ledger.Service
	auth    *auth.Manager
	box     *secretbox.Box
	logger  *slog.Logger
	appName string
	origin  string

	fixedSender mail.Sender

	mu        sync.Mutex
	settings  *db.SystemSetting
	loadedAt  time.Time
	sender    mail.Sender
	senderFor time.Time // settings.UpdatedAt the sender was built from
}

func New(o Options) *Service {
	if o.AppName == "" {
		o.AppName = "RigelLedger"
	}
	return &Service{
		store: o.Store, ledger: o.Ledger, auth: o.Auth, box: o.Box, logger: o.Logger,
		appName: o.AppName, origin: strings.TrimRight(o.Origin, "/"), fixedSender: o.Sender,
	}
}

func (s *Service) link(path string) string { return s.origin + path }

func (s *Service) record(ctx context.Context, e auth.Event) {
	if err := s.auth.Record(ctx, e); err != nil {
		s.logger.Warn("auth event not recorded", "event", e.Name, "error", err)
	}
}

// humanDuration is the "expires in" phrase of a mail, in its language.
func humanDuration(lang string, d time.Duration) string {
	type unit struct {
		size       time.Duration
		en, zh, es string
		enPl, esPl string
	}
	units := []unit{
		{24 * time.Hour, "day", "天", "día", "days", "días"},
		{time.Hour, "hour", "小時", "hora", "hours", "horas"},
		{time.Minute, "minute", "分鐘", "minuto", "minutes", "minutos"},
	}
	for _, u := range units {
		if d >= u.size && d%u.size == 0 || u.size == time.Minute {
			n := int(d / u.size)
			if n < 1 {
				n = 1
			}
			switch lang {
			case "zh":
				return itoa(n) + " " + u.zh
			case "es":
				if n == 1 {
					return "1 " + u.es
				}
				return itoa(n) + " " + u.esPl
			default:
				if n == 1 {
					return "1 " + u.en
				}
				return itoa(n) + " " + u.enPl
			}
		}
	}
	return d.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
