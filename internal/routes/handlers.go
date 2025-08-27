package routes

import (
	"net/http"
	"time"

	"github.com/go-viper/mapstructure/v2"

	"github.com/cwc1222/rigelledger/internal/auth"
	"github.com/cwc1222/rigelledger/internal/models"
)

type LoginRequest struct {
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

// LoginViewHandler is the handler for the login page
// @Summary		Get home page
// @Description	Returns a simple hello world message
// @Tags		root
// @Accept		json
// @Produce		plain
// @Success		200	{string}	string	"Hello, World!"
// @Router		/ [get]
func (rt *Router) LoginViewHandler(w http.ResponseWriter, r *http.Request) {
	err := rt.te.RenderResponse(w, r, nil, "login")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

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

	http.SetCookie(w, &http.Cookie{
		Name:     auth.AccessTokenCookieName,
		Value:    string(accessToken),
		HttpOnly: true,
		//Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(auth.AccessTokenLifetime.Seconds()),
		Expires:  time.Now().Add(auth.AccessTokenLifetime),
	})

	http.SetCookie(w, &http.Cookie{
		Name:     auth.RefreshTokenCookieName,
		Value:    string(refreshToken),
		HttpOnly: true,
		//Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(auth.RefreshTokenLifetime.Seconds()),
		Expires:  time.Now().Add(auth.RefreshTokenLifetime),
	})

	// http.Redirect(w, r, "/home", http.StatusSeeOther)
}

func (rt *Router) LogoutHandler(w http.ResponseWriter, r *http.Request) {}

// HomeHandler is the handler for the home page
func (rt *Router) HomeHandler(w http.ResponseWriter, r *http.Request) {
	err := rt.te.RenderResponse(w, r, nil, "home")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
