package routes

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/identity"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
	"github.com/cwchen-twn/rigel-ledger/internal/response"
)

func clientOf(r *http.Request) auth.Client {
	return auth.Client{IP: auth.IPFrom(r.Context()), UserAgent: r.UserAgent()}
}

type AuthConfigDTO struct {
	// closed: invitations only; request: /request-access; open: /register.
	Registration string `json:"registration"`
}

// authConfig
//
//	@Summary	What the signed-out pages may offer
//	@Tags		auth
//	@Produce	json
//	@Success	200	{object}	AuthConfigDTO
//	@Router		/api/auth/config [get]
func (h *handlers) authConfig(w http.ResponseWriter, r *http.Request) {
	c, err := h.identity.PublicConfig(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, AuthConfigDTO{Registration: c.Registration})
}

type registerRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Language string `json:"language"`
}

// register
//
//	@Summary	Sign up (registration = open); a confirmation link is mailed
//	@Tags		auth
//	@Accept		json
//	@Param		body	body	registerRequest	true	"sign-up"
//	@Success	202
//	@Failure	403	{object}	response.ErrorBody	"registration_closed"
//	@Failure	429	{object}	response.ErrorBody
//	@Router		/api/auth/register [post]
func (h *handlers) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.identity.Register(r.Context(), req.Username, req.Email, req.Password, req.Language, clientOf(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

type tokenRequest struct {
	Token string `json:"token"`
}

type signedInResponse struct {
	User UserDTO `json:"user"`
}

// verifyLink
//
//	@Summary	Confirm a sign-up link: creates the user and signs them in
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		tokenRequest	true	"the token from the link"
//	@Success	200		{object}	signedInResponse
//	@Failure	404		{object}	response.ErrorBody
//	@Router		/api/auth/verify-link [post]
func (h *handlers) verifyLink(w http.ResponseWriter, r *http.Request) {
	var req tokenRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	u, token, err := h.identity.VerifyLink(r.Context(), req.Token, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.auth.SetCookie(r.Context(), w, token)
	response.JSON(w, http.StatusOK, signedInResponse{User: userDTO(u)})
}

type accessRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Message  string `json:"message"`
}

// requestAccess
//
//	@Summary	Ask the administrators for an account (registration = request)
//	@Tags		auth
//	@Accept		json
//	@Param		body	body	accessRequest	true	"request"
//	@Success	202
//	@Router		/api/auth/request-access [post]
func (h *handlers) requestAccess(w http.ResponseWriter, r *http.Request) {
	var req accessRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.identity.RequestAccess(r.Context(), req.Username, req.Email, req.Message, clientOf(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// InvitationDTO is what the invitation page pre-fills.
type InvitationDTO struct {
	Username        string `json:"username"`
	Email           string `json:"email"`
	DisplayName     string `json:"display_name"`
	Language        string `json:"language"`
	DisplayCurrency string `json:"display_currency"`
	Timezone        string `json:"timezone"`
	DateFormat      string `json:"date_format"`
	Theme           string `json:"theme"`
}

// invitePreview
//
//	@Summary	The invitation behind a link
//	@Tags		auth
//	@Produce	json
//	@Param		token	path		string	true	"token from the link"
//	@Success	200		{object}	InvitationDTO
//	@Failure	404		{object}	response.ErrorBody
//	@Router		/api/auth/invite/{token} [get]
func (h *handlers) invitePreview(w http.ResponseWriter, r *http.Request) {
	u, err := h.identity.InvitePreview(r.Context(), chi.URLParam(r, "token"), clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, InvitationDTO{
		Username: u.Username, Email: u.Email, DisplayName: u.DisplayName, Language: u.Language,
		DisplayCurrency: u.DisplayCurrency, Timezone: u.Timezone, DateFormat: u.DateFormat, Theme: u.Theme,
	})
}

type profileRequest struct {
	Username        string `json:"username"`
	DisplayName     string `json:"display_name"`
	Language        string `json:"language"`
	DisplayCurrency string `json:"display_currency"`
	Timezone        string `json:"timezone"`
	DateFormat      string `json:"date_format"`
	Theme           string `json:"theme"`
}

func (p profileRequest) profile() identity.Profile {
	return identity.Profile{Username: p.Username, DisplayName: p.DisplayName, Preferences: ledger.Preferences{
		Language: p.Language, DisplayCurrency: p.DisplayCurrency, Timezone: p.Timezone,
		DateFormat: p.DateFormat, Theme: p.Theme,
	}}
}

type acceptInviteRequest struct {
	profileRequest
	Password string `json:"password"`
}

// acceptInvite
//
//	@Summary	Accept an invitation: settings and password, then signed in
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		token	path		string				true	"token from the link"
//	@Param		body	body		acceptInviteRequest	true	"profile and password"
//	@Success	200		{object}	signedInResponse
//	@Failure	404		{object}	response.ErrorBody
//	@Failure	422		{object}	response.ErrorBody
//	@Router		/api/auth/invite/{token} [post]
func (h *handlers) acceptInvite(w http.ResponseWriter, r *http.Request) {
	var req acceptInviteRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	u, token, err := h.identity.AcceptInvite(r.Context(), chi.URLParam(r, "token"), req.profile(), req.Password, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.auth.SetCookie(r.Context(), w, token)
	response.JSON(w, http.StatusOK, signedInResponse{User: userDTO(u)})
}

type startEmailRequest struct {
	Email           string `json:"email"`
	CurrentPassword string `json:"current_password"`
}

// startEmail
//
//	@Summary	Mail a 6-digit code to verify the current or a new address
//	@Description	After the first-login wizard the current password is required. The address changes only when the code is confirmed.
//	@Tags		me
//	@Accept		json
//	@Param		body	body	startEmailRequest	true	"address"
//	@Success	204
//	@Failure	409	{object}	response.ErrorBody	"email taken, already_verified, mail_failed"
//	@Failure	429	{object}	response.ErrorBody
//	@Router		/api/me/email [post]
func (h *handlers) startEmail(w http.ResponseWriter, r *http.Request) {
	var req startEmailRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	id, _ := auth.FromContext(r.Context())
	if err := h.identity.StartEmailVerification(r.Context(), id.User, req.Email, req.CurrentPassword, clientOf(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	response.NoContent(w)
}

type codeRequest struct {
	Code string `json:"code"`
}

// confirmEmail
//
//	@Summary	Confirm the emailed code; the address becomes verified (and current)
//	@Tags		me
//	@Accept		json
//	@Produce	json
//	@Param		body	body		codeRequest	true	"code"
//	@Success	200		{object}	UserDTO
//	@Failure	422		{object}	response.ErrorBody
//	@Router		/api/me/email/confirm [post]
func (h *handlers) confirmEmail(w http.ResponseWriter, r *http.Request) {
	var req codeRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	id, _ := auth.FromContext(r.Context())
	u, err := h.identity.ConfirmEmail(r.Context(), id.User, req.Code, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, userDTO(u))
}

// cancelEmail
//
//	@Summary	Drop a pending address change
//	@Tags		me
//	@Success	204
//	@Router		/api/me/email/pending [delete]
func (h *handlers) cancelEmail(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	if err := h.identity.CancelPendingEmail(r.Context(), id.User); err != nil {
		h.fail(w, r, err)
		return
	}
	response.NoContent(w)
}

type onboardingRequest struct {
	profileRequest
	// Required when password_must_change (a bootstrap admin).
	NewPassword string `json:"new_password"`
}

// onboarding
//
//	@Summary	Finish the first-login wizard (needs a verified address)
//	@Tags		me
//	@Accept		json
//	@Produce	json
//	@Param		body	body		onboardingRequest	true	"profile"
//	@Success	200		{object}	UserDTO
//	@Failure	422		{object}	response.ErrorBody
//	@Router		/api/me/onboarding [post]
func (h *handlers) onboarding(w http.ResponseWriter, r *http.Request) {
	var req onboardingRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	id, _ := auth.FromContext(r.Context())
	u, err := h.identity.CompleteOnboarding(r.Context(), id.User, req.profile(), req.NewPassword, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, userDTO(u))
}

type SessionDTO struct {
	ID         int64  `json:"id"`
	Kind       string `json:"kind"`
	Label      string `json:"label"`
	UserAgent  string `json:"user_agent"`
	IP         string `json:"ip"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at"`
	ExpiresAt  string `json:"expires_at"`
	Current    bool   `json:"current"`
}

// listSessions
//
//	@Summary	The signed-in user's open sessions
//	@Tags		me
//	@Produce	json
//	@Success	200	{array}	SessionDTO
//	@Router		/api/me/sessions [get]
func (h *handlers) listSessions(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	rows, err := h.identity.Sessions(r.Context(), id.User.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]SessionDTO, len(rows))
	for i, s := range rows {
		out[i] = SessionDTO{ID: s.ID, Kind: s.Kind, Label: s.Label, UserAgent: s.UserAgent, IP: s.Ip,
			CreatedAt: timestamp(s.CreatedAt), LastUsedAt: timestamp(s.LastUsedAt), ExpiresAt: timestamp(s.ExpiresAt),
			Current: s.ID == id.SessionID}
	}
	response.JSON(w, http.StatusOK, out)
}

// revokeSession
//
//	@Summary	Sign out one of your sessions
//	@Tags		me
//	@Param		sessionID	path	int	true	"session id"
//	@Success	204
//	@Router		/api/me/sessions/{sessionID} [delete]
func (h *handlers) revokeSession(w http.ResponseWriter, r *http.Request) {
	sid, ok := pathID(r, "sessionID")
	if !ok {
		badParam(w, "sessionID", "invalid")
		return
	}
	id, _ := auth.FromContext(r.Context())
	if err := h.identity.RevokeSession(r.Context(), id.User, sid, clientOf(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	if sid == id.SessionID && id.ViaCookie {
		h.auth.ClearCookie(w)
	}
	response.NoContent(w)
}

type AuthEventDTO struct {
	ID        int64          `json:"id"`
	Username  string         `json:"username"`
	UserID    *int64         `json:"user_id"`
	Event     string         `json:"event"`
	Failure   bool           `json:"failure"`
	IP        string         `json:"ip"`
	UserAgent string         `json:"user_agent"`
	Detail    map[string]any `json:"detail"`
	CreatedAt string         `json:"created_at"`
}

// myEvents
//
//	@Summary	Recent sign-in and security events of the signed-in user
//	@Tags		me
//	@Produce	json
//	@Success	200	{array}	AuthEventDTO
//	@Router		/api/me/events [get]
func (h *handlers) myEvents(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	rows, err := h.identity.MyEvents(r.Context(), id.User.ID, int32(queryInt(r, "limit", 50)))
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
