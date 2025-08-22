package routes

import (
	"net/http"
	"time"

	"github.com/go-viper/mapstructure/v2"

	"github.com/cwc1222/rigelledger/internal/auth"
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
	err := rt.te.NamedTemplateWithHeaders(w, http.StatusOK, nil, nil, "login")
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

	var req LoginRequest
	if err := mapstructure.Decode(r.PostForm, &req); err != nil {
		rt.logger.Error("Failed to decode form", "error", err)
		http.Error(w, "Failed to decode form", http.StatusBadRequest)
		return
	}

	token, err := rt.jwt.Sign(req.Username, nil, auth.AccessTokenLifetime)
	if err != nil {
		rt.logger.Error("Failed to generate token", "error", err)
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "rigel_jwt_access",
		Value:    string(token),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(auth.AccessTokenLifetime.Seconds()),
		Expires:  time.Now().Add(auth.AccessTokenLifetime),
	})

	http.Redirect(w, r, "/home", http.StatusSeeOther)
}

func (rt *Router) LogoutHandler(w http.ResponseWriter, r *http.Request) {}

// HomeHandler is the handler for the home page
func (rt *Router) HomeHandler(w http.ResponseWriter, r *http.Request) {
	err := rt.te.NamedTemplateWithHeaders(w, http.StatusOK, nil, nil, "home")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
