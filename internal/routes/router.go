package routes

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httplog/v3"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	"github.com/cwc1222/rigelledger/internal/response"
	"github.com/cwc1222/rigelledger/web"
)

type RouterConfig struct {
	AppURL       string
	AppPort      int
	AccessLogger *slog.Logger
	LogLevel     slog.Level
}

type Router struct {
	Handler *chi.Mux
	te      *response.TemplateEngine
}

// NewRouter creates a new router with the given configuration
// @title    Rigel Ledger OpenAPI Specification
// @version	 1.0.0.beta
func NewRouter(rc *RouterConfig, te *response.TemplateEngine) *Router {
	r := chi.NewRouter()

	r.Use(httplog.RequestLogger(rc.AccessLogger, &httplog.Options{
		Level:         rc.LogLevel,
		Schema:        httplog.SchemaECS,
		RecoverPanics: true,
	}))

	r.Get("/swagger/*", httpSwagger.Handler(
		//The url pointing to API definition
		httpSwagger.URL(fmt.Sprintf("http://%s:%d/swagger/doc.json", rc.AppURL, rc.AppPort)),
	))

	r.Handle("/static/*", http.FileServer(http.FS(web.StaticFiles)))

	rt := &Router{
		Handler: r,
		te:      te,
	}

	r.Get("/", rt.LoginHandler)
	r.Get("/home", rt.HomeHandler)

	return rt
}
