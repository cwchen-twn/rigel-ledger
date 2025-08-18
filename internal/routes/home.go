package routes

import (
	"net/http"
)

// LoginHandler is the handler for the login page
// @Summary		Get home page
// @Description	Returns a simple hello world message
// @Tags		root
// @Accept		json
// @Produce		plain
// @Success		200	{string}	string	"Hello, World!"
// @Router		/ [get]
func (rt *Router) LoginHandler(w http.ResponseWriter, r *http.Request) {
	err := rt.te.NamedTemplateWithHeaders(w, http.StatusOK, nil, nil, "login")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// HomeHandler is the handler for the home page
func (rt *Router) HomeHandler(w http.ResponseWriter, r *http.Request) {
	err := rt.te.NamedTemplateWithHeaders(w, http.StatusOK, nil, nil, "home")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
