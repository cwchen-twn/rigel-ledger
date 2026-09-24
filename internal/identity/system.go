package identity

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
	"github.com/cwchen-twn/rigel-ledger/internal/mail"
)

// settingsCacheTTL bounds how stale a read may be. Writes through this
// service drop the cache at once; the TTL only matters for a second replica
// during a rollout, or a change made with psql.
const settingsCacheTTL = 30 * time.Second

// Settings returns the system settings row, cached.
func (s *Service) Settings(ctx context.Context) (db.SystemSetting, error) {
	s.mu.Lock()
	if s.settings != nil && time.Since(s.loadedAt) < settingsCacheTTL {
		out := *s.settings
		s.mu.Unlock()
		return out, nil
	}
	s.mu.Unlock()
	row, err := s.store.GetSystemSettings(ctx)
	if err != nil {
		return db.SystemSetting{}, err
	}
	s.mu.Lock()
	s.settings, s.loadedAt = &row, time.Now()
	s.mu.Unlock()
	return row, nil
}

func (s *Service) cacheSettings(row db.SystemSetting) {
	s.mu.Lock()
	s.settings, s.loadedAt = &row, time.Now()
	s.mu.Unlock()
}

// Limits implements auth.Policy.
func (s *Service) Limits(ctx context.Context) auth.Limits {
	st, err := s.Settings(ctx)
	if err != nil {
		s.logger.Warn("system settings unavailable; default sign-in limits", "error", err)
		return auth.DefaultLimits
	}
	return auth.Limits{
		UserIP: int(st.LoginMaxFailures), IP: int(st.LoginIpMaxFailures), User: int(st.LoginUserMaxFailures),
		Window: time.Duration(st.LoginWindowSeconds) * time.Second,
	}
}

// MFARequired implements auth.Policy.
func (s *Service) MFARequired(ctx context.Context) bool {
	st, err := s.Settings(ctx)
	if err != nil {
		// Fail closed: without the settings, assume the strict default.
		s.logger.Warn("system settings unavailable; two-factor assumed required", "error", err)
		return true
	}
	return st.MfaRequired
}

// SessionTTL implements auth.Policy; 0 means SESSION_TTL from the environment.
func (s *Service) SessionTTL(ctx context.Context) time.Duration {
	st, err := s.Settings(ctx)
	if err != nil || st.SessionTtlSeconds == nil {
		return 0
	}
	return time.Duration(*st.SessionTtlSeconds) * time.Second
}

// Defaults are the preferences a new user starts with.
func (s *Service) Defaults(ctx context.Context) (ledger.Preferences, error) {
	st, err := s.Settings(ctx)
	if err != nil {
		return ledger.Preferences{}, err
	}
	return ledger.Preferences{
		Language: st.DefaultLanguage, DisplayCurrency: st.DefaultDisplayCurrency, Timezone: st.DefaultTimezone,
		DateFormat: st.DefaultDateFormat, Theme: st.DefaultTheme,
	}, nil
}

type SettingsInput struct {
	Registration         string
	MfaRequired          bool
	MfaMethods           []string
	Defaults             ledger.Preferences
	SessionTTL           time.Duration // 0: SESSION_TTL from the environment
	InviteTTL            time.Duration
	LoginMaxFailures     int
	LoginIPMaxFailures   int
	LoginUserMaxFailures int
	LoginWindow          time.Duration
}

var (
	registrationModes = []string{"closed", "request", "open"}
	mfaMethods        = []string{"email", "totp", "passkey"}
)

func (s *Service) UpdateSettings(ctx context.Context, admin db.User, in SettingsInput) (db.SystemSetting, error) {
	in.Defaults.DisplayCurrency = strings.ToUpper(strings.TrimSpace(in.Defaults.DisplayCurrency))
	if !slices.Contains(registrationModes, in.Registration) {
		return db.SystemSetting{}, ledger.FieldError("registration", "invalid", "registration must be closed, request or open")
	}
	methods := make([]string, 0, len(in.MfaMethods))
	for _, m := range mfaMethods { // canonical order, no duplicates
		if slices.Contains(in.MfaMethods, m) {
			methods = append(methods, m)
		}
	}
	for _, m := range in.MfaMethods {
		if !slices.Contains(mfaMethods, m) {
			return db.SystemSetting{}, ledger.FieldError("mfa_methods", "invalid", "unknown method %q", m)
		}
	}
	if in.MfaRequired && len(methods) == 0 {
		return db.SystemSetting{}, ledger.FieldError("mfa_methods", "required", "enable at least one method while two-factor sign-in is required")
	}
	if err := s.ledger.ValidatePreferences(ctx, in.Defaults, "default_"); err != nil {
		return db.SystemSetting{}, err
	}
	switch {
	case in.SessionTTL != 0 && in.SessionTTL < 5*time.Minute:
		return db.SystemSetting{}, ledger.FieldError("session_ttl_seconds", "too_small", "at least 5 minutes")
	case in.InviteTTL < time.Hour:
		return db.SystemSetting{}, ledger.FieldError("invite_ttl_seconds", "too_small", "at least 1 hour")
	case in.LoginWindow < time.Minute:
		return db.SystemSetting{}, ledger.FieldError("login_window_seconds", "too_small", "at least 1 minute")
	case in.LoginMaxFailures < 1:
		return db.SystemSetting{}, ledger.FieldError("login_max_failures", "too_small", "at least 1")
	case in.LoginIPMaxFailures < 1:
		return db.SystemSetting{}, ledger.FieldError("login_ip_max_failures", "too_small", "at least 1")
	case in.LoginUserMaxFailures < 1:
		return db.SystemSetting{}, ledger.FieldError("login_user_max_failures", "too_small", "at least 1")
	}
	var ttl *int64
	if in.SessionTTL != 0 {
		v := int64(in.SessionTTL / time.Second)
		ttl = &v
	}
	var row db.SystemSetting
	err := s.store.WithTx(ctx, admin.ID, func(q *db.Queries) error {
		var err error
		row, err = q.UpdateSystemSettings(ctx, db.UpdateSystemSettingsParams{
			Registration: in.Registration, MfaRequired: in.MfaRequired, MfaMethods: methods,
			DefaultLanguage: in.Defaults.Language, DefaultDisplayCurrency: in.Defaults.DisplayCurrency,
			DefaultTimezone: in.Defaults.Timezone, DefaultDateFormat: in.Defaults.DateFormat,
			DefaultTheme: in.Defaults.Theme, SessionTtlSeconds: ttl,
			InviteTtlSeconds:     int64(in.InviteTTL / time.Second),
			LoginMaxFailures:     int32(in.LoginMaxFailures),
			LoginIpMaxFailures:   int32(in.LoginIPMaxFailures),
			LoginUserMaxFailures: int32(in.LoginUserMaxFailures),
			LoginWindowSeconds:   int64(in.LoginWindow / time.Second),
			UpdatedBy:            &admin.ID,
		})
		return err
	})
	if err != nil {
		return db.SystemSetting{}, ledger.Translate(err, "settings")
	}
	s.cacheSettings(row)
	return row, nil
}

// MailInput is the Mail tab. Password nil or "" keeps the stored one (the
// API never sends it back, so an untouched form has none); ClearPassword
// removes it.
type MailInput struct {
	Driver        string
	Host          string
	Port          int
	Security      string
	User          string
	Password      *string
	ClearPassword bool
	From          string
	FromName      string
}

func (s *Service) UpdateMail(ctx context.Context, adminID int64, in MailInput) (db.SystemSetting, error) {
	in.Host, in.From, in.User = strings.TrimSpace(in.Host), strings.TrimSpace(in.From), strings.TrimSpace(in.User)
	in.FromName = strings.TrimSpace(in.FromName)
	if in.FromName == "" {
		in.FromName = s.appName
	}
	if in.Security == "" {
		in.Security = "starttls"
	}
	if in.Port == 0 {
		in.Port = 587
	}
	switch {
	case !slices.Contains([]string{"smtp", "log", "off"}, in.Driver):
		return db.SystemSetting{}, ledger.FieldError("mail_driver", "invalid", "driver must be smtp, log or off")
	case !slices.Contains([]string{"starttls", "tls", "none"}, in.Security):
		return db.SystemSetting{}, ledger.FieldError("smtp_security", "invalid", "security must be starttls, tls or none")
	case in.Port < 1 || in.Port > 65535:
		return db.SystemSetting{}, ledger.FieldError("smtp_port", "invalid", "port out of range")
	case in.Driver == "smtp" && in.Host == "":
		return db.SystemSetting{}, ledger.FieldError("smtp_host", "required", "an SMTP host is required")
	case in.Driver == "smtp" && !ledger.ValidEmail(in.From):
		return db.SystemSetting{}, ledger.FieldError("mail_from", "invalid", "a sender address is required")
	case in.From != "" && !ledger.ValidEmail(in.From):
		return db.SystemSetting{}, ledger.FieldError("mail_from", "invalid", "not an email address")
	}
	var sealed []byte
	if in.Password != nil && *in.Password != "" {
		var err error
		if sealed, err = s.box.Seal([]byte(*in.Password)); err != nil {
			return db.SystemSetting{}, err
		}
	}
	var uid *int64
	if adminID != 0 {
		uid = &adminID
	}
	var row db.SystemSetting
	err := s.store.WithTx(ctx, adminID, func(q *db.Queries) error {
		if in.ClearPassword {
			if err := q.ClearSMTPPassword(ctx); err != nil {
				return err
			}
		}
		var err error
		row, err = q.UpdateMailSettings(ctx, db.UpdateMailSettingsParams{
			MailDriver: in.Driver, SmtpHost: in.Host, SmtpPort: int32(in.Port), SmtpSecurity: in.Security,
			SmtpUser: in.User, SmtpPassEnc: sealed, MailFrom: in.From, MailFromName: in.FromName, UpdatedBy: uid,
		})
		return err
	})
	if err != nil {
		return db.SystemSetting{}, ledger.Translate(err, "settings")
	}
	s.cacheSettings(row)
	return row, nil
}

// SeedMailFromEnv copies SMTP_* / MAIL_* from the environment into the
// settings the first time only: once the mail settings were saved, the
// Administration page owns them and the environment is ignored.
func (s *Service) SeedMailFromEnv(ctx context.Context, cfg mail.Config) error {
	if cfg.Driver == "" && cfg.Host == "" {
		return nil
	}
	st, err := s.Settings(ctx)
	if err != nil {
		return err
	}
	if st.MailConfigured {
		s.logger.Info("Mail settings already saved; SMTP_* and MAIL_* environment variables are ignored")
		return nil
	}
	if cfg.Driver == "" {
		cfg.Driver = "smtp"
	}
	in := MailInput{Driver: cfg.Driver, Host: cfg.Host, Port: cfg.Port, Security: cfg.Security,
		User: cfg.User, From: cfg.From, FromName: cfg.FromName}
	if cfg.Pass != "" {
		in.Password = &cfg.Pass
	}
	if _, err := s.UpdateMail(ctx, 0, in); err != nil {
		return err
	}
	s.logger.Info("Mail settings seeded from the environment", "driver", cfg.Driver, "host", cfg.Host)
	return nil
}

// mailer returns a Sender for the current settings, rebuilt when they change.
func (s *Service) mailer(ctx context.Context) (mail.Sender, error) {
	if s.fixedSender != nil {
		return s.fixedSender, nil
	}
	st, err := s.Settings(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sender != nil && s.senderFor.Equal(st.UpdatedAt) {
		return s.sender, nil
	}
	cfg := mail.Config{
		Driver: st.MailDriver, Host: st.SmtpHost, Port: int(st.SmtpPort), Security: st.SmtpSecurity,
		User: st.SmtpUser, From: st.MailFrom, FromName: st.MailFromName,
	}
	if len(st.SmtpPassEnc) > 0 {
		pass, err := s.box.Open(st.SmtpPassEnc)
		if err != nil {
			return nil, err
		}
		cfg.Pass = string(pass)
	}
	sender, err := mail.New(cfg, s.logger)
	if err != nil {
		return nil, err
	}
	s.sender, s.senderFor = sender, st.UpdatedAt
	return sender, nil
}

// ErrMail wraps a failure to send, so callers can report "the invitation is
// saved but the mail did not go out" rather than a bare 500.
var ErrMail = errors.New("mail not sent")

func (s *Service) send(ctx context.Context, to, lang, tmpl string, d mail.Data) error {
	d.AppName = s.appName
	m, err := mail.Render(lang, tmpl, d)
	if err != nil {
		return err
	}
	m.To = to
	sender, err := s.mailer(ctx)
	if err != nil {
		s.logger.Error("mailer unavailable", "error", err)
		return mailFailed(err)
	}
	if err := sender.Send(ctx, m); err != nil {
		s.logger.Error("mail not sent", "template", tmpl, "error", err)
		return mailFailed(err)
	}
	return nil
}

func mailFailed(err error) error {
	if errors.Is(err, mail.ErrDisabled) {
		return &ledger.Error{Kind: ledger.KindConflict, Code: "mail_disabled", Message: "outgoing mail is turned off"}
	}
	return &ledger.Error{Kind: ledger.KindConflict, Code: "mail_failed", Message: err.Error()}
}

// SendTest mails the admin who asked, in their language, with the SAVED
// mail settings, and says which driver handled it. Under "log" nothing
// leaves the server, and the page must say so: an earlier version reported
// "sent" while the unsaved form still meant the default log driver.
func (s *Service) SendTest(ctx context.Context, admin db.User) (driver string, err error) {
	if admin.Email == "" {
		return "", ledger.FieldError("email", "required", "your account has no email address")
	}
	st, err := s.Settings(ctx)
	if err != nil {
		return "", err
	}
	return st.MailDriver, s.send(ctx, admin.Email, admin.Language, "test", mail.Data{Name: displayName(admin)})
}

func displayName(u db.User) string {
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Username
}

// InvalidateSettings drops the cached row (after a change made outside this
// service: tests, or a manual fix in psql followed by a restart-free reload).
func (s *Service) InvalidateSettings() {
	s.mu.Lock()
	s.settings = nil
	s.mu.Unlock()
}
