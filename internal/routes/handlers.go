package routes

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/cwc1222/rigelledger/internal/auth"
	"github.com/cwc1222/rigelledger/internal/models"
)

// writeError sends a structured JSON error response: {"error": "<code>"}.
// All user-facing error messages are resolved on the frontend via i18n.
func writeError(w http.ResponseWriter, code string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{code})
}

// SPAHandler serves the SolidJS SPA shell for all HTML routes.
// Auth is handled entirely client-side via /api/me.
func (rt *Router) SPAHandler(w http.ResponseWriter, r *http.Request) {
	if err := rt.te.RenderResponse(w, r, nil, "app"); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type LoginResponse struct {
	Username string `json:"username"`
}

func (lr LoginResponse) ToJSONString() (string, error) {
	b, err := json.Marshal(lr)
	return string(b), err
}

type MeResponse struct {
	Username            string `json:"username"`
	MainLanguage        string `json:"main_language"`
	AccessTokenLeftTime int    `json:"access_token_left_time"`
}

func (mr MeResponse) ToJSONString() (string, error) {
	b, err := json.Marshal(mr)
	return string(b), err
}

// MeHandler returns the authenticated user's info including preferred language.
func (rt *Router) MeHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tokenData, ok := ctx.Value(auth.TokenDataKey).(*auth.TokenData)
	if !ok {
		writeError(w, "UNAUTHORIZED", http.StatusUnauthorized)
		return
	}

	user, err := models.FindByUserName(tokenData.Subject, rt.db)
	if err != nil {
		writeError(w, "INTERNAL_ERROR", http.StatusInternalServerError)
		return
	}

	leftTime := int(time.Until(tokenData.Expiry).Seconds())
	if leftTime < 0 {
		leftTime = 0
	}

	resp := MeResponse{
		Username:            user.Username,
		MainLanguage:        user.MainLanguage,
		AccessTokenLeftTime: leftTime,
	}
	if err := rt.je.RenderResponse(w, r, resp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// LoginHandler authenticates the user and sets JWT cookies.
// @Summary		Post login page
// @Description	Returns a JWT token for the user
// @Tags		root
// @Accept		application/x-www-form-urlencoded
// @Produce		json
// @Success		200	{object}	LoginResponse
// @Failure		400	{object}	object{error=string}
// @Failure		401	{object}	object{error=string}
// @Failure		404	{object}	object{error=string}
// @Failure		500	{object}	object{error=string}
// @Router		/login [post]
func (rt *Router) LoginHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		rt.logger.Error("Failed to parse form", "error", err)
		writeError(w, "PARSE_ERROR", http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	user, err := models.FindByUserName(username, rt.db)
	if errors.Is(err, models.ErrUserNotFound) {
		rt.logger.Error("User not found", "username", username)
		writeError(w, "USER_NOT_FOUND", http.StatusNotFound)
		return
	}
	if err != nil {
		rt.logger.Error("Error occurred while finding user", "error", err)
		writeError(w, "INTERNAL_ERROR", http.StatusInternalServerError)
		return
	}
	if err := user.ValidatePassword(password); err != nil {
		rt.logger.Error("Invalid password", "username", username)
		writeError(w, "INVALID_PASSWORD", http.StatusUnauthorized)
		return
	}

	rt.logger.Info("Form decoded successfully", "username", username)

	accessToken, err := rt.jwt.Sign(username, nil, auth.AccessTokenLifetime)
	if err != nil {
		rt.logger.Error("Failed to generate access token", "error", err)
		writeError(w, "TOKEN_GENERATION_FAILED", http.StatusInternalServerError)
		return
	}

	refreshToken, err := rt.jwt.Sign(username, nil, auth.RefreshTokenLifetime)
	if err != nil {
		rt.logger.Error("Failed to generate refresh token", "error", err)
		writeError(w, "TOKEN_GENERATION_FAILED", http.StatusInternalServerError)
		return
	}

	if err := user.UpdateLastLogin(rt.db); err != nil {
		rt.logger.Error("Error occurred while updating last login", "error", err)
		writeError(w, "INTERNAL_ERROR", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     auth.AccessTokenCookieName,
		Value:    string(accessToken),
		HttpOnly: true,
		Secure:   !rt.IsLocalhost,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(auth.AccessTokenLifetime.Seconds()),
		Expires:  time.Now().Add(auth.AccessTokenLifetime),
	})

	http.SetCookie(w, &http.Cookie{
		Name:     auth.RefreshTokenCookieName,
		Value:    string(refreshToken),
		HttpOnly: true,
		Secure:   !rt.IsLocalhost,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(auth.RefreshTokenLifetime.Seconds()),
		Expires:  time.Now().Add(auth.RefreshTokenLifetime),
	})

	if err := rt.je.RenderResponse(w, r, LoginResponse{Username: username}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (rt *Router) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.AccessTokenCookieName,
		Value:    "",
		HttpOnly: true,
		Secure:   !rt.IsLocalhost,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Now().Add(-1 * time.Hour),
	})

	http.SetCookie(w, &http.Cookie{
		Name:     auth.RefreshTokenCookieName,
		Value:    "",
		HttpOnly: true,
		Secure:   !rt.IsLocalhost,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Now().Add(-1 * time.Hour),
	})

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

type RefreshTokenResponse struct {
	AccessTokenLeftTime int `json:"access_token_left_time"`
}

func (rtr RefreshTokenResponse) ToJSONString() (string, error) {
	json, err := json.Marshal(rtr)
	if err != nil {
		return "", err
	}
	return string(json), nil
}

// RefreshTokenHandler handles token refresh using refresh token
func (rt *Router) RefreshTokenHandler(w http.ResponseWriter, r *http.Request) {
	refreshTokenCookie, err := r.Cookie(auth.RefreshTokenCookieName)
	if err != nil {
		rt.logger.Error("No refresh token found", "error", err)
		writeError(w, "NO_REFRESH_TOKEN", http.StatusUnauthorized)
		return
	}

	tokenData, err := rt.jwt.Verify([]byte(refreshTokenCookie.Value))
	if err != nil {
		rt.logger.Error("Invalid refresh token", "error", err)
		writeError(w, "INVALID_TOKEN", http.StatusUnauthorized)
		return
	}

	newAccessToken, err := rt.jwt.Sign(tokenData.Subject, nil, auth.AccessTokenLifetime)
	if err != nil {
		rt.logger.Error("Failed to generate new access token", "error", err)
		writeError(w, "TOKEN_GENERATION_FAILED", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     auth.AccessTokenCookieName,
		Value:    string(newAccessToken),
		HttpOnly: true,
		Secure:   !rt.IsLocalhost,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(auth.AccessTokenLifetime.Seconds()),
		Expires:  time.Now().Add(auth.AccessTokenLifetime),
	})

	if time.Now().After(tokenData.Expiry.Add(-1 * time.Minute * 10)) {
		newRefreshToken, err := rt.jwt.Sign(tokenData.Subject, nil, auth.RefreshTokenLifetime)
		if err != nil {
			rt.logger.Error("Failed to generate new refresh token", "error", err)
			writeError(w, "TOKEN_GENERATION_FAILED", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     auth.RefreshTokenCookieName,
			Value:    string(newRefreshToken),
			HttpOnly: true,
			Secure:   !rt.IsLocalhost,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   int(auth.RefreshTokenLifetime.Seconds()),
			Expires:  time.Now().Add(auth.RefreshTokenLifetime),
		})
	}

	rtr := RefreshTokenResponse{
		AccessTokenLeftTime: int(time.Until(tokenData.Expiry).Seconds()),
	}

	if err := rt.je.RenderResponse(w, r, rtr); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// ListTransactionsHandler returns paginated journal entries (DataTables format).
// @Summary		Get list transactions page
// @Description	Returns the list transactions page
// @Tags		transactions
// @Accept		html
// @Produce		html
// @Success		200	{string}	string	"List transactions page"
// @Failure		500	{string}	string	"Error occurred while rendering template"
// @Router		/transactions [get]
func (rt *Router) ListTransactionsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tokenData, ok := ctx.Value(auth.TokenDataKey).(*auth.TokenData)
	if !ok {
		writeError(w, "UNAUTHORIZED", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		rt.logger.Error("Failed to parse form", "error", err)
		writeError(w, "PARSE_ERROR", http.StatusBadRequest)
		return
	}

	req := models.DataTableRequest{
		Search: struct {
			Value string
			Regex bool
		}{
			Value: r.FormValue("search[value]"),
			Regex: r.FormValue("search[regex]") == "true",
		},
	}
	if draw, err := strconv.Atoi(r.FormValue("draw")); err != nil {
		rt.logger.Error("Failed to parse draw", "error", err)
		writeError(w, "PARSE_ERROR", http.StatusBadRequest)
		return
	} else {
		req.Draw = draw
	}

	if start, err := strconv.Atoi(r.FormValue("start")); err != nil {
		rt.logger.Error("Failed to parse start", "error", err)
		writeError(w, "PARSE_ERROR", http.StatusBadRequest)
		return
	} else {
		req.Start = start
	}

	if length, err := strconv.Atoi(r.FormValue("length")); err != nil {
		rt.logger.Error("Failed to parse length", "error", err)
		writeError(w, "PARSE_ERROR", http.StatusBadRequest)
		return
	} else {
		req.Length = length
	}

	journals, err := models.FindTransactionsByUserID(tokenData.Subject, req, rt.db)
	if err != nil {
		writeError(w, "DB_ERROR", http.StatusInternalServerError)
		return
	}

	if err := rt.je.RenderResponse(w, r, journals); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (rt *Router) LedgersGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tokenData, ok := ctx.Value(auth.TokenDataKey).(*auth.TokenData)
	if !ok {
		writeError(w, "UNAUTHORIZED", http.StatusUnauthorized)
		return
	}

	ledgers, err := models.FindLedgersByUserID(tokenData.Subject, rt.db)
	if err != nil {
		writeError(w, "DB_ERROR", http.StatusInternalServerError)
		return
	}

	if err := rt.je.RenderResponse(w, r, ledgers); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (rt *Router) LedgerTypesHandler(w http.ResponseWriter, r *http.Request) {
	types, err := models.FindLedgerTypes(rt.db)
	if err != nil {
		writeError(w, "DB_ERROR", http.StatusInternalServerError)
		return
	}

	if err := rt.je.RenderResponse(w, r, types); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (rt *Router) LedgerTypesFirstGradeHandler(w http.ResponseWriter, r *http.Request) {
	types, err := models.FindLedgerTypesFirstGrade(rt.db)
	if err != nil {
		writeError(w, "DB_ERROR", http.StatusInternalServerError)
		return
	}

	if err := rt.je.RenderResponse(w, r, types); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (rt *Router) CurrenciesHandler(w http.ResponseWriter, r *http.Request) {
	currencies, err := models.FindCurrencies(rt.db)
	if err != nil {
		writeError(w, "DB_ERROR", http.StatusInternalServerError)
		return
	}

	if err := rt.je.RenderResponse(w, r, currencies); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (rt *Router) LedgersSaveHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tokenData, ok := ctx.Value(auth.TokenDataKey).(*auth.TokenData)
	if !ok {
		writeError(w, "UNAUTHORIZED", http.StatusUnauthorized)
		return
	}

	defer r.Body.Close()
	var ledgers models.Ledgers
	if err := json.NewDecoder(r.Body).Decode(&ledgers); err != nil {
		rt.logger.Error("Failed to decode request body", "error", err)
		writeError(w, "PARSE_JSON_ERROR", http.StatusBadRequest)
		return
	}

	for i := range ledgers {
		ledgers[i].LedgerOwner = tokenData.Subject
	}

	if err := ledgers.Save(rt.db, tokenData.Subject); err != nil {
		rt.logger.Error("Failed to save ledger", "error", err)
		switch {
		case errors.Is(err, models.ErrLedgerNameRequired):
			writeError(w, "LEDGER_NAME_REQUIRED", http.StatusBadRequest)
		case errors.Is(err, models.ErrBalanceRequired):
			writeError(w, "BALANCE_INVALID", http.StatusBadRequest)
		default:
			writeError(w, "DB_ERROR", http.StatusInternalServerError)
		}
		return
	}
}

func (rt *Router) LedgersEditHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tokenData, ok := ctx.Value(auth.TokenDataKey).(*auth.TokenData)
	if !ok {
		writeError(w, "UNAUTHORIZED", http.StatusUnauthorized)
		return
	}

	defer r.Body.Close()
	var ledgers models.Ledgers
	if err := json.NewDecoder(r.Body).Decode(&ledgers); err != nil {
		rt.logger.Error("Failed to decode request body", "error", err)
		writeError(w, "PARSE_JSON_ERROR", http.StatusBadRequest)
		return
	}

	for i := range ledgers {
		ledgers[i].LedgerOwner = tokenData.Subject
	}

	if err := ledgers.Edit(rt.db, tokenData.Subject); err != nil {
		rt.logger.Error("Failed to edit ledger", "error", err)
		switch {
		case errors.Is(err, models.ErrLedgerNameRequired):
			writeError(w, "LEDGER_NAME_REQUIRED", http.StatusBadRequest)
		case errors.Is(err, models.ErrBalanceRequired):
			writeError(w, "BALANCE_INVALID", http.StatusBadRequest)
		default:
			writeError(w, "DB_ERROR", http.StatusInternalServerError)
		}
		return
	}
}

func (rt *Router) LedgersDeleteHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tokenData, ok := ctx.Value(auth.TokenDataKey).(*auth.TokenData)
	if !ok {
		writeError(w, "UNAUTHORIZED", http.StatusUnauthorized)
		return
	}

	ledgerIDStr := chi.URLParam(r, "ledgerID")
	if ledgerIDStr == "" {
		writeError(w, "LEDGER_ID_REQUIRED", http.StatusBadRequest)
		return
	}

	ledgerID, err := strconv.Atoi(ledgerIDStr)
	if err != nil {
		writeError(w, "INVALID_LEDGER_ID", http.StatusBadRequest)
		return
	}

	if err := models.DeleteLedger(ledgerID, tokenData.Subject, rt.db); err != nil {
		writeError(w, "DB_ERROR", http.StatusInternalServerError)
		return
	}
}
