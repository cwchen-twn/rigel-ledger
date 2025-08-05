package internal

import (
	"fmt"
	"net/http"

	_ "github.com/cwc1222/rigelledger/docs"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// @Summary		Get home page
// @Description	Returns a simple hello world message
// @Tags		root
// @Accept		json
// @Produce		plain
// @Success		200	{string}	string	"Hello, World!"
// @Router		/ [get]
func homeHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hello, World!"))
}

// @title    Rigel Ledger OpenAPI Specification
// @version	 1.0.0.beta
func NewRouter(appUrl string, appPort int) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL(fmt.Sprintf("%s:%d/swagger/doc.json", appUrl, appPort)), //The url pointing to API definition
	))

	r.Get("/", homeHandler)

	return r
}
