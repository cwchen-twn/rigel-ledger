package routes

import (
	"log/slog"
	"net/http"
	"time"

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

	// Static files with long-lived cache
	staticHandler := http.FileServer(http.FS(web.StaticFiles))
	r.Handle("/static/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=7884000")
		w.Header().Set("Expires", time.Now().AddDate(0, 3, 0).Format(http.TimeFormat))
		staticHandler.ServeHTTP(w, r)
	}))
	r.Get("/robots.txt", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=7884000")
		w.Header().Set("Expires", time.Now().AddDate(0, 3, 0).Format(http.TimeFormat))
		robotsContent, err := web.StaticFiles.ReadFile("static/robots.txt")
		if err != nil {
			http.Error(w, "robots.txt not found", http.StatusNotFound)
			return
		}
		w.Write(robotsContent)
	}))

	// Auth endpoints
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})
	r.Get("/login", rt.SPAHandler)
	r.Post("/login", rt.LoginHandler)
	r.Get("/logout", rt.LogoutHandler)
	r.Post("/refresh-token", rt.RefreshTokenHandler)

	// Top-level auth check API (no username prefix needed by the SPA)
	r.With(auth.JWTValidateAPIMiddleware(jwt)).Get("/api/me", rt.MeHandler)

	r.Route("/{username}", func(r chi.Router) {
		// HTML routes: serve the SPA shell, no JWT required (SPA handles auth via /api/me)
		r.Get("/", rt.SPAHandler)
		r.Get("/{page}", rt.SPAHandler)

		// API routes: JWT required, returns JSON errors on failure
		r.Route("/api", func(r chi.Router) {
			r.Use(auth.JWTValidateAPIMiddleware(jwt))
			r.Post("/transactions", rt.ListTransactionsHandler)
			r.Get("/ledgers", rt.LedgersGetHandler)
			r.Get("/ledger-types", rt.LedgerTypesHandler)
			r.Get("/ledger-types/firstgrade", rt.LedgerTypesFirstGradeHandler)
			r.Get("/currencies", rt.CurrenciesHandler)
			r.Post("/ledgers", rt.LedgersSaveHandler)
			r.Put("/ledgers", rt.LedgersEditHandler)
			r.Delete("/ledger/{ledgerID}", rt.LedgersDeleteHandler)
		})
	})

	return rt
}
