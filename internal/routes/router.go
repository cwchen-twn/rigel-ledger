package routes

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httplog/v3"
	"github.com/jmoiron/sqlx"

	"github.com/cwc1222/rigelledger/internal/auth"
	"github.com/cwc1222/rigelledger/internal/response"
	"github.com/cwc1222/rigelledger/web"
)

type RouterConfig struct {
	AppURL       string
	AppPort      int
	AccessLogger *slog.Logger
	LogLevel     slog.Level
	IsLocalhost  bool
}

type Router struct {
	Handler     *chi.Mux
	te          *response.TemplateEngine
	je          *response.JSONEngine
	jwt         *auth.JWT
	logger      *slog.Logger
	db          *sqlx.DB
	IsLocalhost bool
}

// NewRouter creates a new router with the given configuration
// @title    Rigel Ledger OpenAPI Specification
// @version	 1.0.0.beta
func NewRouter(rc *RouterConfig, te *response.TemplateEngine, je *response.JSONEngine, jwt *auth.JWT, logger *slog.Logger, db *sqlx.DB) *Router {
	r := chi.NewRouter()

	r.Use(middleware.Compress(6, "text/*", "application/*"))
	r.Use(httplog.RequestLogger(rc.AccessLogger, &httplog.Options{
		Level:         rc.LogLevel,
		Schema:        httplog.SchemaECS,
		RecoverPanics: true,
	}))
	r.Use(auth.JWTExtractTokenMiddleware(jwt))

	rt := &Router{
		Handler:     r,
		te:          te,
		je:          je,
		jwt:         jwt,
		logger:      logger,
		db:          db,
		IsLocalhost: rc.IsLocalhost,
	}

	// Setup Swagger conditionally based on build tags
	rt.setupSwagger(rc.AppURL, rc.AppPort)
	r.Handle("/static/*", http.FileServer(http.FS(web.StaticFiles)))
	r.Get("/", rt.LoginViewHandler)
	r.Get("/login", rt.LoginViewHandler)
	r.Post("/login", rt.LoginHandler)
	r.Get("/logout", rt.LogoutHandler)
	r.Post("/refresh-token", rt.RefreshTokenHandler)

	r.Route("/{username}", func(r chi.Router) {
		r.Use(auth.JWTValidateTokenMiddleware(jwt))

		r.Get("/", rt.HomeHandler)
		r.Get("/ledgers", rt.LedgersHandler)
		r.Get("/reports", rt.ReportsHandler)

		r.Route("/api", func(r chi.Router) {
			r.Post("/transactions", rt.ListTransactionsHandler)
		})
	})

	return rt
}
