package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-viper/mapstructure/v2"

	"github.com/cwc1222/rigelledger/internal/auth"
	"github.com/cwc1222/rigelledger/internal/models"
)

type LoginRequest struct {
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

func (rt *Router) LoginViewHandler(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()
	accessToken, ok := ctx.Value(auth.AccessTokenKey).(string)
	if ok {
		tokenData, err := rt.jwt.Verify([]byte(accessToken))
		if err == nil {
			rt.logger.Info("Access token verified")
			http.Redirect(w, r, fmt.Sprintf("/%s", tokenData.Subject), http.StatusSeeOther)
			return
		}
	}

	errorMessage := r.URL.Query().Get("error")
	err := rt.te.RenderResponse(w, r, map[string]any{"Error": errorMessage}, "login")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// LoginHandler is the handler for the login page
// @Summary		Post login page
// @Description	Returns a JWT token for the user
// @Tags		root
// @Accept		application/x-www-form-urlencoded
// @Produce		plain
// @Success		200	{string}	string	"JWT token"
// @Failure		400	{string}	string	"Failed to parse form"
// @Failure		401	{string}	string	"Invalid password"
// @Failure		404	{string}	string	"User not found"
// @Failure		500	{string}	string	"Error occurred while finding user"
// @Failure		500	{string}	string	"Error occurred while updating last login"
// @Failure		500	{string}	string	"Failed to generate access token"
// @Router		/login [post]
func (rt *Router) LoginHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		rt.logger.Error("Failed to parse form", "error", err)
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	formData := make(map[string]any)
	for key, values := range r.PostForm {
		if len(values) > 0 {
			formData[key] = values[0] // Take the first value for each key
		}
	}

	var req LoginRequest
	if err := mapstructure.Decode(formData, &req); err != nil {
		rt.logger.Error("Failed to decode form", "error", err)
		http.Error(w, "Failed to decode form", http.StatusBadRequest)
		return
	}

	// Validate that we got the required fields
	user, err := models.FindByUserName(req.Username, rt.db)
	if err == models.ErrUserNotFound {
		rt.logger.Error("User not found", "username", req.Username)
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}
	if err != nil {
		rt.logger.Error("Error occurred while finding user", "error", err)
		http.Error(w, "Error occurred while finding user", http.StatusInternalServerError)
		return
	}
	if err := user.ValidatePassword(req.Password); err != nil {
		rt.logger.Error("Invalid password", "username", req.Username)
		http.Error(w, "Invalid password", http.StatusUnauthorized)
		return
	}

	rt.logger.Info("Form decoded successfully", "username", req.Username)

	accessToken, err := rt.jwt.Sign(req.Username, nil, auth.AccessTokenLifetime)
	if err != nil {
		rt.logger.Error("Failed to generate access token", "error", err)
		http.Error(w, "Failed to generate access token", http.StatusInternalServerError)
		return
	}

	refreshToken, err := rt.jwt.Sign(req.Username, nil, auth.RefreshTokenLifetime)
	if err != nil {
		rt.logger.Error("Failed to generate refresh token", "error", err)
		http.Error(w, "Failed to generate refresh token", http.StatusInternalServerError)
		return
	}

	if err := user.UpdateLastLogin(rt.db); err != nil {
		rt.logger.Error("Error occurred while updating last login", "error", err)
		http.Error(w, "Error occurred while updating last login", http.StatusInternalServerError)
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

	// http.Redirect(w, r, "/home", http.StatusSeeOther)
}

func (rt *Router) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	// Clear both access and refresh token cookies
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
	// Get refresh token from cookie
	refreshTokenCookie, err := r.Cookie(auth.RefreshTokenCookieName)
	if err != nil {
		rt.logger.Error("No refresh token found", "error", err)
		http.Error(w, "No refresh token", http.StatusUnauthorized)
		return
	}

	// Verify refresh token
	tokenData, err := rt.jwt.Verify([]byte(refreshTokenCookie.Value))
	if err != nil {
		rt.logger.Error("Invalid refresh token", "error", err)
		http.Error(w, "Invalid refresh token", http.StatusUnauthorized)
		return
	}

	// Generate new access token
	newAccessToken, err := rt.jwt.Sign(tokenData.Subject, nil, auth.AccessTokenLifetime)
	if err != nil {
		rt.logger.Error("Failed to generate new access token", "error", err)
		http.Error(w, "Failed to generate new access token", http.StatusInternalServerError)
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

	// If the refresh token is expiring in less than 10 minutes, generate a new one
	if time.Now().After(tokenData.Expiry.Add(-1 * time.Minute * 10)) {
		newRefreshToken, err := rt.jwt.Sign(tokenData.Subject, nil, auth.RefreshTokenLifetime)
		if err != nil {
			rt.logger.Error("Failed to generate new refresh token", "error", err)
			http.Error(w, "Failed to generate new refresh token", http.StatusInternalServerError)
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

	accessTokenLeftTime := time.Until(tokenData.Expiry).Seconds()
	rtr := RefreshTokenResponse{
		AccessTokenLeftTime: int(accessTokenLeftTime),
	}

	if err := rt.je.RenderResponse(w, r, rtr); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

}

// HomeHandler is the handler for the home page
func (rt *Router) HomeHandler(w http.ResponseWriter, r *http.Request) {
	err := rt.te.RenderResponse(w, r, nil, "home")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// ListTransactionsHandler is the handler for the list transactions page
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
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		rt.logger.Error("Failed to parse form", "error", err)
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
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
		http.Error(w, "Failed to parse draw", http.StatusBadRequest)
		return
	} else {
		req.Draw = draw
	}

	if start, err := strconv.Atoi(r.FormValue("start")); err != nil {
		rt.logger.Error("Failed to parse start", "error", err)
		http.Error(w, "Failed to parse start", http.StatusBadRequest)
		return
	} else {
		req.Start = start
	}

	if length, err := strconv.Atoi(r.FormValue("length")); err != nil {
		rt.logger.Error("Failed to parse length", "error", err)
		http.Error(w, "Failed to parse length", http.StatusBadRequest)
		return
	} else {
		req.Length = length
	}

	journals, err := models.FindTransactionsByUserID(tokenData.Subject, req, rt.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := rt.je.RenderResponse(w, r, journals); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (rt *Router) LedgersHandler(w http.ResponseWriter, r *http.Request) {
	err := rt.te.RenderResponse(w, r, nil, "ledgers")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (rt *Router) LedgersGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tokenData, ok := ctx.Value(auth.TokenDataKey).(*auth.TokenData)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ledgers, err := models.FindLedgersByUserID(tokenData.Subject, rt.db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := rt.je.RenderResponse(w, r, ledgers); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (rt *Router) LedgersSaveHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tokenData, ok := ctx.Value(auth.TokenDataKey).(*auth.TokenData)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	defer r.Body.Close()
	var ledgers models.Ledgers
	if err := json.NewDecoder(r.Body).Decode(&ledgers); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	for i := range ledgers {
		ledgers[i].LedgerOwner = tokenData.Subject
	}

	if err := ledgers.Save(rt.db); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (rt *Router) ReportsHandler(w http.ResponseWriter, r *http.Request) {
	err := rt.te.RenderResponse(w, r, nil, "reports")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
