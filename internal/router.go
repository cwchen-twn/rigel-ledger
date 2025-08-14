package internal

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httplog/v3"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	"github.com/cwc1222/rigelledger/internal/routes"
)

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
		//The url pointing to API definition
		httpSwagger.URL(fmt.Sprintf("http://%s:%d/swagger/doc.json", config.AppURL, config.AppPort)),
	))

	r.Get("/", routes.LoginHandler)
	r.Get("/home", routes.HomeHandler)

	return r
}
