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
	if req.Username == "" || req.Password == "" {
		rt.logger.Error("Missing required fields", "username", req.Username, "password", req.Password)
		http.Error(w, "Username and password are required", http.StatusBadRequest)
		return
	}

	rt.logger.Info("Form decoded successfully", "username", req.Username)

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
		//Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(auth.AccessTokenLifetime.Seconds()),
		Expires:  time.Now().Add(auth.AccessTokenLifetime),
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
