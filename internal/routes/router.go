package routes

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httplog/v3"

	"github.com/cwc1222/rigelledger/internal/auth"
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
	jwt     *auth.JWT
	logger  *slog.Logger
}

// NewRouter creates a new router with the given configuration
// @title    Rigel Ledger OpenAPI Specification
// @version	 1.0.0.beta
func NewRouter(rc *RouterConfig, te *response.TemplateEngine, jwt *auth.JWT, logger *slog.Logger) *Router {
	r := chi.NewRouter()

	r.Use(middleware.Compress(6, "text/*", "application/json"))
	r.Use(httplog.RequestLogger(rc.AccessLogger, &httplog.Options{
		Level:         rc.LogLevel,
		Schema:        httplog.SchemaECS,
		RecoverPanics: true,
	}))

	rt := &Router{
		Handler: r,
		te:      te,
		jwt:     jwt,
		logger:  logger,
	}

	// Setup Swagger conditionally based on build tags
	rt.setupSwagger(rc.AppURL, rc.AppPort)
	r.Handle("/static/*", http.FileServer(http.FS(web.StaticFiles)))
	r.Get("/", rt.LoginViewHandler)
	r.Post("/login", rt.LoginHandler)
	r.Get("/logout", rt.LogoutHandler)

	r.Group(func(r chi.Router) {
		r.Use(auth.JWTMiddleware(jwt))
		r.Get("/home", rt.HomeHandler)
	})

	return rt
}
