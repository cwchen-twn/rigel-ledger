// Package mail sends the few messages the app needs: invitations, sign-up
// links, verification codes, change notices and access requests.
//
// The SMTP settings live in system_settings and are edited on the
// Administration page, so a Sender is rebuilt whenever they change. In
// development the "log" driver prints each message (and its link or code)
// instead of sending it.
package mail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	gomail "github.com/wneessen/go-mail"
)

type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

type Sender interface {
	Send(ctx context.Context, m Message) error
}

type Config struct {
	Driver   string // smtp, log, off
	Host     string
	Port     int
	Security string // starttls, tls, none
	User     string
	Pass     string
	From     string
	FromName string
}

var ErrDisabled = errors.New("mail is turned off")

// New builds the Sender for cfg. It does not connect; the first Send does.
func New(cfg Config, logger *slog.Logger) (Sender, error) {
	switch cfg.Driver {
	case "off":
		return offSender{}, nil
	case "log", "":
		return logSender{logger: logger}, nil
	case "smtp":
		if cfg.Host == "" || cfg.From == "" {
			return nil, errors.New("smtp needs a host and a from address")
		}
		return &smtpSender{cfg: cfg}, nil
	}
	return nil, fmt.Errorf("unknown mail driver %q", cfg.Driver)
}

type offSender struct{}

func (offSender) Send(context.Context, Message) error { return ErrDisabled }

// logSender is for development: the message, link and code land in the log.
type logSender struct{ logger *slog.Logger }

func (s logSender) Send(_ context.Context, m Message) error {
	s.logger.Info("mail (log driver, not sent)", "to", m.To, "subject", m.Subject, "text", m.Text)
	return nil
}

type smtpSender struct{ cfg Config }

func (s *smtpSender) Send(ctx context.Context, m Message) error {
	msg := gomail.NewMsg()
	if err := msg.FromFormat(s.cfg.FromName, s.cfg.From); err != nil {
		return fmt.Errorf("from address: %w", err)
	}
	if err := msg.To(m.To); err != nil {
		return fmt.Errorf("to address: %w", err)
	}
	msg.Subject(m.Subject)
	msg.SetBodyString(gomail.TypeTextPlain, m.Text)
	if m.HTML != "" {
		msg.AddAlternativeString(gomail.TypeTextHTML, m.HTML)
	}

	opts := []gomail.Option{gomail.WithPort(s.cfg.Port), gomail.WithTimeout(20 * time.Second)}
	switch s.cfg.Security {
	case "tls":
		opts = append(opts, gomail.WithSSL())
	case "none":
		opts = append(opts, gomail.WithTLSPolicy(gomail.NoTLS))
	default:
		opts = append(opts, gomail.WithTLSPolicy(gomail.TLSMandatory))
	}
	if s.cfg.User != "" {
		opts = append(opts, gomail.WithSMTPAuth(gomail.SMTPAuthAutoDiscover),
			gomail.WithUsername(s.cfg.User), gomail.WithPassword(s.cfg.Pass))
	}
	c, err := gomail.NewClient(s.cfg.Host, opts...)
	if err != nil {
		return err
	}
	return c.DialAndSendWithContext(ctx, msg)
}
