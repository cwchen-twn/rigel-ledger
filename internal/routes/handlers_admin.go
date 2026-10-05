package routes

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/internal/identity"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
	"github.com/cwchen-twn/rigel-ledger/internal/response"
)

func timestamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func timestampPtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := timestamp(*t)
	return &s
}

func queryInt(r *http.Request, name string, def int) int {
	if n, err := strconv.Atoi(r.URL.Query().Get(name)); err == nil {
		return n
	}
	return def
}

func authEventDTO(id int64, username string, userID *int64, event string, failure bool, ip, ua string, detail []byte, at time.Time) AuthEventDTO {
	d := map[string]any{}
	_ = json.Unmarshal(detail, &d)
	return AuthEventDTO{ID: id, Username: username, UserID: userID, Event: event, Failure: failure,
		IP: ip, UserAgent: ua, Detail: d, CreatedAt: timestamp(at)}
}

// AdminSettingsDTO is the whole Administration page. The SMTP password is
// never sent; smtp_password_set says whether one is stored.
type AdminSettingsDTO struct {
	Registration           string   `json:"registration"`
	MfaRequired            bool     `json:"mfa_required"`
	MfaMethods             []string `json:"mfa_methods"`
	DefaultLanguage        string   `json:"default_language"`
	DefaultDisplayCurrency string   `json:"default_display_currency"`
	DefaultTimezone        string   `json:"default_timezone"`
	DefaultDateFormat      string   `json:"default_date_format"`
	DefaultTheme           string   `json:"default_theme"`
	// null: SESSION_TTL from the environment.
	SessionTTLSeconds    *int64 `json:"session_ttl_seconds"`
	InviteTTLSeconds     int64  `json:"invite_ttl_seconds"`
	LoginMaxFailures     int32  `json:"login_max_failures"`
	LoginIPMaxFailures   int32  `json:"login_ip_max_failures"`
	LoginUserMaxFailures int32  `json:"login_user_max_failures"`
	LoginWindowSeconds   int64  `json:"login_window_seconds"`

	MailConfigured  bool   `json:"mail_configured"`
	MailDriver      string `json:"mail_driver"`
	SMTPHost        string `json:"smtp_host"`
	SMTPPort        int32  `json:"smtp_port"`
	SMTPSecurity    string `json:"smtp_security"`
	SMTPUser        string `json:"smtp_user"`
	SMTPPasswordSet bool   `json:"smtp_password_set"`
	MailFrom        string `json:"mail_from"`
	MailFromName    string `json:"mail_from_name"`

	UpdatedAt string `json:"updated_at"`
}

func adminSettingsDTO(s db.SystemSetting) AdminSettingsDTO {
	return AdminSettingsDTO{
		Registration: s.Registration, MfaRequired: s.MfaRequired, MfaMethods: s.MfaMethods,
		DefaultLanguage: s.DefaultLanguage, DefaultDisplayCurrency: s.DefaultDisplayCurrency,
		DefaultTimezone: s.DefaultTimezone, DefaultDateFormat: s.DefaultDateFormat, DefaultTheme: s.DefaultTheme,
		SessionTTLSeconds: s.SessionTtlSeconds, InviteTTLSeconds: s.InviteTtlSeconds,
		LoginMaxFailures: s.LoginMaxFailures, LoginIPMaxFailures: s.LoginIpMaxFailures,
		LoginUserMaxFailures: s.LoginUserMaxFailures, LoginWindowSeconds: s.LoginWindowSeconds,
		MailConfigured: s.MailConfigured, MailDriver: s.MailDriver, SMTPHost: s.SmtpHost, SMTPPort: s.SmtpPort,
		SMTPSecurity: s.SmtpSecurity, SMTPUser: s.SmtpUser, SMTPPasswordSet: len(s.SmtpPassEnc) > 0,
		MailFrom: s.MailFrom, MailFromName: s.MailFromName, UpdatedAt: timestamp(s.UpdatedAt),
	}
}

func admin(r *http.Request) db.User {
	id, _ := auth.FromContext(r.Context())
	return id.User
}

// adminSettings
//
//	@Summary	System settings (admin)
//	@Tags		admin
//	@Produce	json
//	@Success	200	{object}	AdminSettingsDTO
//	@Router		/api/admin/settings [get]
func (h *handlers) adminSettings(w http.ResponseWriter, r *http.Request) {
	s, err := h.identity.Settings(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, adminSettingsDTO(s))
}

type adminSettingsRequest struct {
	Registration           string   `json:"registration"`
	MfaRequired            bool     `json:"mfa_required"`
	MfaMethods             []string `json:"mfa_methods"`
	DefaultLanguage        string   `json:"default_language"`
	DefaultDisplayCurrency string   `json:"default_display_currency"`
	DefaultTimezone        string   `json:"default_timezone"`
	DefaultDateFormat      string   `json:"default_date_format"`
	DefaultTheme           string   `json:"default_theme"`
	SessionTTLSeconds      *int64   `json:"session_ttl_seconds"`
	InviteTTLSeconds       int64    `json:"invite_ttl_seconds"`
	LoginMaxFailures       int      `json:"login_max_failures"`
	LoginIPMaxFailures     int      `json:"login_ip_max_failures"`
	LoginUserMaxFailures   int      `json:"login_user_max_failures"`
	LoginWindowSeconds     int64    `json:"login_window_seconds"`
}

// updateAdminSettings
//
//	@Summary	Replace registration, security and default settings (admin)
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Param		body	body		adminSettingsRequest	true	"every field"
//	@Success	200		{object}	AdminSettingsDTO
//	@Failure	422		{object}	response.ErrorBody
//	@Router		/api/admin/settings [patch]
func (h *handlers) updateAdminSettings(w http.ResponseWriter, r *http.Request) {
	var req adminSettingsRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	var ttl time.Duration
	if req.SessionTTLSeconds != nil {
		ttl = time.Duration(*req.SessionTTLSeconds) * time.Second
	}
	s, err := h.identity.UpdateSettings(r.Context(), admin(r), identity.SettingsInput{
		Registration: req.Registration, MfaRequired: req.MfaRequired, MfaMethods: req.MfaMethods,
		Defaults: ledger.Preferences{
			Language: req.DefaultLanguage, DisplayCurrency: req.DefaultDisplayCurrency,
			Timezone: req.DefaultTimezone, DateFormat: req.DefaultDateFormat, Theme: req.DefaultTheme,
		},
		SessionTTL:           ttl,
		InviteTTL:            time.Duration(req.InviteTTLSeconds) * time.Second,
		LoginMaxFailures:     req.LoginMaxFailures,
		LoginIPMaxFailures:   req.LoginIPMaxFailures,
		LoginUserMaxFailures: req.LoginUserMaxFailures,
		LoginWindow:          time.Duration(req.LoginWindowSeconds) * time.Second,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.auditAdmin(r, "admin_settings_changed")
	response.JSON(w, http.StatusOK, adminSettingsDTO(s))
}

type adminMailRequest struct {
	MailDriver   string `json:"mail_driver"`
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPSecurity string `json:"smtp_security"`
	SMTPUser     string `json:"smtp_user"`
	// Empty or absent keeps the stored password.
	SMTPPassword      *string `json:"smtp_password"`
	ClearSMTPPassword bool    `json:"clear_smtp_password"`
	MailFrom          string  `json:"mail_from"`
	MailFromName      string  `json:"mail_from_name"`
}

// updateAdminMail
//
//	@Summary	Outgoing mail settings (admin); the password is stored sealed and never returned
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Param		body	body		adminMailRequest	true	"mail"
//	@Success	200		{object}	AdminSettingsDTO
//	@Router		/api/admin/mail [patch]
func (h *handlers) updateAdminMail(w http.ResponseWriter, r *http.Request) {
	var req adminMailRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	s, err := h.identity.UpdateMail(r.Context(), admin(r).ID, identity.MailInput{
		Driver: req.MailDriver, Host: req.SMTPHost, Port: req.SMTPPort, Security: req.SMTPSecurity,
		User: req.SMTPUser, Password: req.SMTPPassword, ClearPassword: req.ClearSMTPPassword,
		From: req.MailFrom, FromName: req.MailFromName,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.auditAdmin(r, "admin_mail_changed")
	response.JSON(w, http.StatusOK, adminSettingsDTO(s))
}

// MailTestDTO says where the test went. Driver "log" means it was written
// to the server log and did not leave the server.
type MailTestDTO struct {
	Driver string `json:"driver"`
	To     string `json:"to"`
}

// testMail
//
//	@Summary	Send a test email to yourself with the saved mail settings (admin)
//	@Tags		admin
//	@Produce	json
//	@Success	200	{object}	MailTestDTO
//	@Failure	409	{object}	response.ErrorBody	"mail_failed or mail_disabled"
//	@Router		/api/admin/mail/test [post]
func (h *handlers) testMail(w http.ResponseWriter, r *http.Request) {
	a := admin(r)
	driver, err := h.identity.SendTest(r.Context(), a)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, MailTestDTO{Driver: driver, To: a.Email})
}

func (h *handlers) auditAdmin(r *http.Request, name string) {
	a := admin(r)
	c := clientOf(r)
	if err := h.auth.Record(r.Context(), auth.Event{Username: a.Username, UserID: &a.ID, IP: c.IP, UserAgent: c.UserAgent, Name: name}); err != nil {
		h.logger.Warn("auth event not recorded", "event", name, "error", err)
	}
}

type AdminUserDTO struct {
	ID            int64   `json:"id"`
	Username      string  `json:"username"`
	Email         string  `json:"email"`
	DisplayName   string  `json:"display_name"`
	IsAdmin       bool    `json:"is_admin"`
	IsActive      bool    `json:"is_active"`
	EmailVerified bool    `json:"email_verified"`
	Initialized   bool    `json:"initialized"`
	InvitePending bool    `json:"invite_pending"`
	Invited       bool    `json:"invited"` // has not accepted yet
	LastLoginAt   *string `json:"last_login_at"`
	CreatedAt     string  `json:"created_at"`
}

func adminUserDTO(u db.User, invitePending bool) AdminUserDTO {
	return AdminUserDTO{
		ID: u.ID, Username: u.Username, Email: u.Email, DisplayName: u.DisplayName, IsAdmin: u.IsAdmin,
		IsActive: u.IsActive, EmailVerified: u.EmailVerifiedAt != nil, Initialized: u.InitializedAt != nil,
		InvitePending: invitePending, Invited: u.PasswordHash == "" && u.InitializedAt == nil,
		LastLoginAt: timestampPtr(u.LastLoginAt), CreatedAt: timestamp(u.CreatedAt),
	}
}

// adminUsers
//
//	@Summary	Every user (admin)
//	@Tags		admin
//	@Produce	json
//	@Success	200	{array}	AdminUserDTO
//	@Router		/api/admin/users [get]
func (h *handlers) adminUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.identity.ListUsers(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]AdminUserDTO, len(rows))
	for i, row := range rows {
		out[i] = adminUserDTO(row.User, row.InvitePending)
	}
	response.JSON(w, http.StatusOK, out)
}

type adminUserRequest struct {
	IsActive *bool `json:"is_active"`
	IsAdmin  *bool `json:"is_admin"`
}

// adminUpdateUser
//
//	@Summary	Activate, deactivate, promote or demote another user (admin)
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Param		userID	path		int					true	"user id"
//	@Param		body	body		adminUserRequest	true	"changes"
//	@Success	200		{object}	AdminUserDTO
//	@Failure	409		{object}	response.ErrorBody	"last_admin"
//	@Router		/api/admin/users/{userID} [patch]
func (h *handlers) adminUpdateUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := pathID(r, "userID")
	if !ok {
		badParam(w, "userID", "invalid")
		return
	}
	var req adminUserRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	u, err := h.identity.UpdateUser(r.Context(), admin(r), userID, identity.UserChange{IsActive: req.IsActive, IsAdmin: req.IsAdmin})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, adminUserDTO(u, false))
}

type inviteRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
}

// invite
//
//	@Summary	Invite a user by username and email (admin)
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Param		body	body		inviteRequest	true	"who"
//	@Success	201		{object}	AdminUserDTO
//	@Failure	409		{object}	response.ErrorBody	"taken, or mail_failed (the invitation is saved; resend it)"
//	@Router		/api/admin/invitations [post]
func (h *handlers) invite(w http.ResponseWriter, r *http.Request) {
	var req inviteRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	u, err := h.identity.Invite(r.Context(), admin(r), req.Username, req.Email)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusCreated, adminUserDTO(u, true))
}

// resendInvite
//
//	@Summary	Mail a fresh invitation link; the old one stops working (admin)
//	@Tags		admin
//	@Param		userID	path	int	true	"invited user id"
//	@Success	204
//	@Router		/api/admin/invitations/{userID}/resend [post]
func (h *handlers) resendInvite(w http.ResponseWriter, r *http.Request) {
	userID, ok := pathID(r, "userID")
	if !ok {
		badParam(w, "userID", "invalid")
		return
	}
	if err := h.identity.ResendInvite(r.Context(), admin(r), userID); err != nil {
		h.fail(w, r, err)
		return
	}
	response.NoContent(w)
}

// revokeInvite
//
//	@Summary	Withdraw an invitation that was not accepted; deletes that user (admin)
//	@Tags		admin
//	@Param		userID	path	int	true	"invited user id"
//	@Success	204
//	@Router		/api/admin/invitations/{userID} [delete]
func (h *handlers) revokeInvite(w http.ResponseWriter, r *http.Request) {
	userID, ok := pathID(r, "userID")
	if !ok {
		badParam(w, "userID", "invalid")
		return
	}
	if err := h.identity.RevokeInvite(r.Context(), admin(r), userID); err != nil {
		h.fail(w, r, err)
		return
	}
	response.NoContent(w)
}

type AccessRequestDTO struct {
	ID        int64   `json:"id"`
	Username  string  `json:"username"`
	Email     string  `json:"email"`
	Message   string  `json:"message"`
	IP        string  `json:"ip"`
	Status    string  `json:"status"`
	DecidedAt *string `json:"decided_at"`
	CreatedAt string  `json:"created_at"`
}

// accessRequests
//
//	@Summary	Requests for an account (admin)
//	@Tags		admin
//	@Produce	json
//	@Param		status	query	string	false	"pending, approved or rejected"
//	@Success	200		{array}	AccessRequestDTO
//	@Router		/api/admin/access-requests [get]
func (h *handlers) accessRequests(w http.ResponseWriter, r *http.Request) {
	rows, err := h.identity.ListAccessRequests(r.Context(), r.URL.Query().Get("status"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]AccessRequestDTO, len(rows))
	for i, a := range rows {
		out[i] = AccessRequestDTO{ID: a.ID, Username: a.Username, Email: a.Email, Message: a.Message, IP: a.Ip,
			Status: a.Status, DecidedAt: timestampPtr(a.DecidedAt), CreatedAt: timestamp(a.CreatedAt)}
	}
	response.JSON(w, http.StatusOK, out)
}

func (h *handlers) decideRequest(w http.ResponseWriter, r *http.Request, approve bool) {
	id, ok := pathID(r, "requestID")
	if !ok {
		badParam(w, "requestID", "invalid")
		return
	}
	if err := h.identity.DecideAccessRequest(r.Context(), admin(r), id, approve); err != nil {
		h.fail(w, r, err)
		return
	}
	response.NoContent(w)
}

// approveRequest
//
//	@Summary	Approve a request: the person is invited (admin)
//	@Tags		admin
//	@Param		requestID	path	int	true	"request id"
//	@Success	204
//	@Router		/api/admin/access-requests/{requestID}/approve [post]
func (h *handlers) approveRequest(w http.ResponseWriter, r *http.Request) {
	h.decideRequest(w, r, true)
}

// rejectRequest
//
//	@Summary	Reject a request (admin)
//	@Tags		admin
//	@Param		requestID	path	int	true	"request id"
//	@Success	204
//	@Router		/api/admin/access-requests/{requestID}/reject [post]
func (h *handlers) rejectRequest(w http.ResponseWriter, r *http.Request) {
	h.decideRequest(w, r, false)
}

// adminEvents
//
//	@Summary	Recent sign-in and security events of everyone (admin)
//	@Tags		admin
//	@Produce	json
//	@Param		limit	query	int	false	"at most 500"
//	@Success	200		{array}	AuthEventDTO
//	@Router		/api/admin/events [get]
func (h *handlers) adminEvents(w http.ResponseWriter, r *http.Request) {
	rows, err := h.identity.ListEvents(r.Context(), int32(queryInt(r, "limit", 100)))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]AuthEventDTO, len(rows))
	for i, e := range rows {
		out[i] = authEventDTO(e.ID, e.Username, e.UserID, e.Event, e.Failure, e.Ip, e.UserAgent, e.Detail, e.CreatedAt)
	}
	response.JSON(w, http.StatusOK, out)
}
