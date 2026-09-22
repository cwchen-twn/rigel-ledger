package routes

import (
	"errors"
	"net/http"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
	"github.com/cwchen-twn/rigel-ledger/internal/response"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	// "api" returns a bearer token in the body instead of setting a cookie.
	Client string `json:"client,omitempty"`
}

type loginResponse struct {
	User  UserDTO `json:"user"`
	Token string  `json:"token,omitempty"`
}

// login
//
//	@Summary	Sign in
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Param		body	body		loginRequest	true	"credentials"
//	@Success	200		{object}	loginResponse
//	@Failure	401		{object}	response.ErrorBody
//	@Router		/api/auth/login [post]
func (h *handlers) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	kind := "web"
	if req.Client == "api" {
		kind = "api"
	}
	u, token, err := h.auth.Login(r.Context(), req.Username, req.Password, kind, r.UserAgent())
	if errors.Is(err, auth.ErrInvalidCredentials) {
		response.Error(w, http.StatusUnauthorized, "invalid_credentials", err.Error(), nil)
		return
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	resp := loginResponse{User: userDTO(u)}
	if kind == "api" {
		resp.Token = token
	} else {
		h.auth.SetCookie(w, token)
	}
	response.JSON(w, http.StatusOK, resp)
}

// logout
//
//	@Summary	Sign out this session
//	@Tags		auth
//	@Success	204
//	@Router		/api/auth/logout [post]
func (h *handlers) logout(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	if err := h.auth.Logout(r.Context(), id.Token); err != nil {
		h.fail(w, r, err)
		return
	}
	h.auth.ClearCookie(w)
	response.NoContent(w)
}

// me
//
//	@Summary	The signed-in user and their settings
//	@Tags		me
//	@Produce	json
//	@Success	200	{object}	UserDTO
//	@Router		/api/me [get]
func (h *handlers) me(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	response.JSON(w, http.StatusOK, userDTO(id.User))
}

type settingsRequest struct {
	DisplayName     string `json:"display_name"`
	Language        string `json:"language"`
	DisplayCurrency string `json:"display_currency"`
	Timezone        string `json:"timezone"`
	DateFormat      string `json:"date_format"`
	Theme           string `json:"theme"`
	DefaultBookID   *int64 `json:"default_book_id"`
}

// updateSettings
//
//	@Summary	Replace the user's preferences
//	@Tags		me
//	@Accept		json
//	@Produce	json
//	@Param		body	body		settingsRequest	true	"all preference fields"
//	@Success	200		{object}	UserDTO
//	@Failure	422		{object}	response.ErrorBody
//	@Router		/api/me/settings [patch]
func (h *handlers) updateSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	id, _ := auth.FromContext(r.Context())
	u, err := h.svc.UpdateSettings(r.Context(), id.User.ID, ledger.UserSettings(req))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, userDTO(u))
}

type passwordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// changePassword
//
//	@Summary	Change password and sign out every other session
//	@Tags		me
//	@Accept		json
//	@Param		body	body	passwordRequest	true	"passwords"
//	@Success	204
//	@Failure	422	{object}	response.ErrorBody
//	@Router		/api/me/password [post]
func (h *handlers) changePassword(w http.ResponseWriter, r *http.Request) {
	var req passwordRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	id, _ := auth.FromContext(r.Context())
	if err := h.svc.ChangePassword(r.Context(), id.User, id.SessionID, req.CurrentPassword, req.NewPassword); err != nil {
		h.fail(w, r, err)
		return
	}
	response.NoContent(w)
}

// currencies
//
//	@Summary	Every ISO 4217 currency with its minor units
//	@Tags		reference
//	@Produce	json
//	@Success	200	{array}	CurrencyDTO
//	@Router		/api/currencies [get]
func (h *handlers) currencies(w http.ResponseWriter, r *http.Request) {
	cs, err := h.svc.Currencies(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]CurrencyDTO, len(cs))
	for i, c := range cs {
		out[i] = CurrencyDTO{Code: c.Code, Name: c.Name, Decimals: c.Decimals}
	}
	response.JSON(w, http.StatusOK, out)
}
