package routes

import (
	"net/http"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/response"
)

type mfaVerifyRequest struct {
	Challenge string `json:"challenge"`
	// totp, email or recovery
	Method string `json:"method"`
	Code   string `json:"code"`
}

// verifyMFA
//
//	@Summary	Second sign-in step: a TOTP, emailed or recovery code
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		mfaVerifyRequest	true	"code"
//	@Success	200		{object}	loginResponse
//	@Failure	422		{object}	response.ErrorBody
//	@Failure	429		{object}	response.ErrorBody
//	@Router		/api/auth/mfa [post]
func (h *handlers) verifyMFA(w http.ResponseWriter, r *http.Request) {
	var req mfaVerifyRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	u, token, client, err := h.identity.VerifySignIn(r.Context(), req.Challenge, req.Method, req.Code, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.signedIn(w, r, u, token, client)
}

type challengeRequest struct {
	Challenge string `json:"challenge"`
}

// sendSignInCode
//
//	@Summary	Mail a code for the second sign-in step
//	@Tags		auth
//	@Accept		json
//	@Param		body	body	challengeRequest	true	"the password step's challenge"
//	@Success	204
//	@Router		/api/auth/mfa/email [post]
func (h *handlers) sendSignInCode(w http.ResponseWriter, r *http.Request) {
	var req challengeRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.identity.SendSignInCode(r.Context(), req.Challenge, clientOf(r)); err != nil {
		h.fail(w, r, err)
		return
	}
	response.NoContent(w)
}

type passkeyBeginRequest struct {
	// The password step's challenge, when the passkey is the second step;
	// empty for a passwordless sign-in.
	Challenge string `json:"challenge"`
	Client    string `json:"client,omitempty"`
}

type PasskeyOptionsDTO struct {
	// PublicKeyCredentialCreationOptions / RequestOptions, as JSON for the browser.
	Options any `json:"options"`
	// Send back to the matching finish endpoint.
	Challenge string `json:"challenge"`
}

// beginPasskeyLogin
//
//	@Summary	Start a passkey sign-in (passwordless, or the second step)
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		passkeyBeginRequest	true	"optional password-step challenge"
//	@Success	200		{object}	PasskeyOptionsDTO
//	@Router		/api/auth/passkey/begin [post]
func (h *handlers) beginPasskeyLogin(w http.ResponseWriter, r *http.Request) {
	var req passkeyBeginRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	opts, token, err := h.identity.BeginPasskeyLogin(r.Context(), req.Challenge, req.Client, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, PasskeyOptionsDTO{Options: opts, Challenge: token})
}

// finishPasskeyLogin
//
//	@Summary	Finish a passkey sign-in; the body is the browser's credential JSON
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		challenge	query		string	true	"from begin"
//	@Success	200			{object}	loginResponse
//	@Router		/api/auth/passkey/finish [post]
func (h *handlers) finishPasskeyLogin(w http.ResponseWriter, r *http.Request) {
	u, token, client, err := h.identity.FinishPasskeyLogin(r.Context(), r.URL.Query().Get("challenge"), r, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.signedIn(w, r, u, token, client)
}

type PasskeyDTO struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at"`
}

type MFAStatusDTO struct {
	TOTP         bool         `json:"totp"`
	Email        bool         `json:"email"`
	Passkeys     []PasskeyDTO `json:"passkeys"`
	RecoveryLeft int64        `json:"recovery_left"`
	// What the administrator allows, and whether a second factor is required.
	Allowed      []string `json:"allowed"`
	Required     bool     `json:"required"`
	SignInAlerts bool     `json:"signin_alerts"`
	SessionAAL   int16    `json:"session_aal"`
}

// mfaStatus
//
//	@Summary	The signed-in user's second factors
//	@Tags		mfa
//	@Produce	json
//	@Success	200	{object}	MFAStatusDTO
//	@Router		/api/me/mfa [get]
func (h *handlers) mfaStatus(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	st, err := h.identity.MFAState(r.Context(), id.User.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	keys, err := h.identity.Passkeys(r.Context(), id.User.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := MFAStatusDTO{TOTP: st.TOTP, Email: st.Email, Passkeys: make([]PasskeyDTO, len(keys)),
		RecoveryLeft: st.RecoveryLeft, Allowed: st.Allowed, Required: st.Required,
		SignInAlerts: id.User.SigninAlerts, SessionAAL: id.AAL}
	for i, k := range keys {
		out.Passkeys[i] = PasskeyDTO{ID: k.ID, Name: k.Name, CreatedAt: timestamp(k.CreatedAt), LastUsedAt: timestampPtr(k.LastUsedAt)}
	}
	response.JSON(w, http.StatusOK, out)
}

type TOTPSetupDTO struct {
	Secret string `json:"secret"`
	URI    string `json:"uri"`
	QR     string `json:"qr"` // data: URL of a PNG
}

// startTOTP
//
//	@Summary	Begin authenticator-app setup: the secret and its QR code
//	@Tags		mfa
//	@Produce	json
//	@Success	200	{object}	TOTPSetupDTO
//	@Router		/api/me/mfa/totp [post]
func (h *handlers) startTOTP(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	setup, err := h.identity.StartTOTP(r.Context(), id.User)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, TOTPSetupDTO{Secret: setup.Secret, URI: setup.URI, QR: setup.QR})
}

// EnrolledDTO carries the recovery codes when this was the first factor:
// shown once, never again.
type EnrolledDTO struct {
	RecoveryCodes []string `json:"recovery_codes,omitempty"`
}

// confirmTOTP
//
//	@Summary	Finish authenticator-app setup with a first code
//	@Tags		mfa
//	@Accept		json
//	@Produce	json
//	@Param		body	body		codeRequest	true	"code"
//	@Success	200		{object}	EnrolledDTO
//	@Router		/api/me/mfa/totp/confirm [post]
func (h *handlers) confirmTOTP(w http.ResponseWriter, r *http.Request) {
	var req codeRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	id, _ := auth.FromContext(r.Context())
	codes, err := h.identity.ConfirmTOTP(r.Context(), id, req.Code, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, EnrolledDTO{RecoveryCodes: codes})
}

// startEmailFactor
//
//	@Summary	Begin email-code setup: a code goes to the verified address
//	@Tags		mfa
//	@Produce	json
//	@Success	200	{object}	challengeRequest
//	@Router		/api/me/mfa/email [post]
func (h *handlers) startEmailFactor(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	token, err := h.identity.StartEmailFactor(r.Context(), id.User)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, challengeRequest{Challenge: token})
}

type challengeCodeRequest struct {
	Challenge string `json:"challenge"`
	Code      string `json:"code"`
}

// confirmEmailFactor
//
//	@Summary	Finish email-code setup
//	@Tags		mfa
//	@Accept		json
//	@Produce	json
//	@Param		body	body		challengeCodeRequest	true	"code"
//	@Success	200		{object}	EnrolledDTO
//	@Router		/api/me/mfa/email/confirm [post]
func (h *handlers) confirmEmailFactor(w http.ResponseWriter, r *http.Request) {
	var req challengeCodeRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	id, _ := auth.FromContext(r.Context())
	codes, err := h.identity.ConfirmEmailFactor(r.Context(), id, req.Challenge, req.Code, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, EnrolledDTO{RecoveryCodes: codes})
}

type passkeyRegisterRequest struct {
	Name string `json:"name"`
}

// beginPasskeyRegistration
//
//	@Summary	Begin adding a passkey
//	@Tags		mfa
//	@Accept		json
//	@Produce	json
//	@Param		body	body		passkeyRegisterRequest	true	"a name for it"
//	@Success	200		{object}	PasskeyOptionsDTO
//	@Router		/api/me/mfa/passkeys/begin [post]
func (h *handlers) beginPasskeyRegistration(w http.ResponseWriter, r *http.Request) {
	var req passkeyRegisterRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	id, _ := auth.FromContext(r.Context())
	opts, token, err := h.identity.BeginPasskeyRegistration(r.Context(), id.User, req.Name)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, PasskeyOptionsDTO{Options: opts, Challenge: token})
}

// finishPasskeyRegistration
//
//	@Summary	Finish adding a passkey; the body is the browser's credential JSON
//	@Tags		mfa
//	@Accept		json
//	@Produce	json
//	@Param		challenge	query		string	true	"from begin"
//	@Success	200			{object}	EnrolledDTO
//	@Router		/api/me/mfa/passkeys/finish [post]
func (h *handlers) finishPasskeyRegistration(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	codes, err := h.identity.FinishPasskeyRegistration(r.Context(), id, r.URL.Query().Get("challenge"), r, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, EnrolledDTO{RecoveryCodes: codes})
}

type stepUpRequest struct {
	CurrentPassword string `json:"current_password"`
}

func (h *handlers) removeFactor(factor string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req stepUpRequest
		if err := response.Decode(w, r, &req); err != nil {
			h.fail(w, r, err)
			return
		}
		var passkeyID int64
		if factor == "passkey" {
			var ok bool
			if passkeyID, ok = pathID(r, "passkeyID"); !ok {
				badParam(w, "passkeyID", "invalid")
				return
			}
		}
		id, _ := auth.FromContext(r.Context())
		if err := h.identity.RemoveFactor(r.Context(), id, factor, passkeyID, req.CurrentPassword, clientOf(r)); err != nil {
			h.fail(w, r, err)
			return
		}
		response.NoContent(w)
	}
}

// regenerateRecoveryCodes
//
//	@Summary	Replace the recovery codes (needs the password and a two-factor session)
//	@Tags		mfa
//	@Accept		json
//	@Produce	json
//	@Param		body	body		stepUpRequest	true	"password"
//	@Success	200		{object}	EnrolledDTO
//	@Router		/api/me/mfa/recovery-codes [post]
func (h *handlers) regenerateRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	var req stepUpRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	id, _ := auth.FromContext(r.Context())
	codes, err := h.identity.RegenerateRecoveryCodes(r.Context(), id, req.CurrentPassword, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, EnrolledDTO{RecoveryCodes: codes})
}

type alertsRequest struct {
	Enabled bool `json:"enabled"`
}

// setSignInAlerts
//
//	@Summary	Mail me when a sign-in comes from a new address
//	@Tags		mfa
//	@Accept		json
//	@Param		body	body	alertsRequest	true	"on or off"
//	@Success	204
//	@Router		/api/me/mfa/alerts [patch]
func (h *handlers) setSignInAlerts(w http.ResponseWriter, r *http.Request) {
	var req alertsRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	id, _ := auth.FromContext(r.Context())
	if err := h.identity.SetSignInAlerts(r.Context(), id.User, req.Enabled); err != nil {
		h.fail(w, r, err)
		return
	}
	response.NoContent(w)
}

// resetMFA
//
//	@Summary	Remove every second factor and session of a user (admin)
//	@Tags		admin
//	@Param		userID	path	int	true	"user id"
//	@Success	204
//	@Router		/api/admin/users/{userID}/reset-mfa [post]
func (h *handlers) resetMFA(w http.ResponseWriter, r *http.Request) {
	userID, ok := pathID(r, "userID")
	if !ok {
		badParam(w, "userID", "invalid")
		return
	}
	a := admin(r)
	if err := h.identity.ResetMFA(r.Context(), a.ID, a.Username, userID); err != nil {
		h.fail(w, r, err)
		return
	}
	response.NoContent(w)
}
