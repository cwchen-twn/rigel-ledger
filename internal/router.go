package internal

import (
	"fmt"
	"log/slog"
	"net/http"

	_ "github.com/cwc1222/rigelledger/docs"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httplog/v3"
	httpSwagger "github.com/swaggo/http-swagger/v2"
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

type RouterConfig struct {
	AppURL       string
	AppPort      int
	AccessLogger *slog.Logger
	LogLevel     slog.Level
}

// NewRouter creates a new router with the given configuration
// @title    Rigel Ledger OpenAPI Specification
// @version	 1.0.0.beta
func NewRouter(config RouterConfig) http.Handler {
	r := chi.NewRouter()

	r.Use(httplog.RequestLogger(config.AccessLogger, &httplog.Options{
		Level:         config.LogLevel,
		Schema:        httplog.SchemaECS,
		RecoverPanics: true,
	}))

	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL(fmt.Sprintf("http://%s:%d/swagger/doc.json", config.AppURL, config.AppPort)), //The url pointing to API definition
	))

	r.Get("/", homeHandler)

	return r
}
