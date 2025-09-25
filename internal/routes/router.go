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
	// Create a file server with cache headers for static resources
	staticHandler := http.FileServer(http.FS(web.StaticFiles))
	r.Handle("/static/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set cache headers for static resources
		w.Header().Set("Cache-Control", "public, max-age=7884000") // 1 year / 4 = 3 months
		w.Header().Set("Expires", time.Now().AddDate(0, 3, 0).Format(http.TimeFormat))
		staticHandler.ServeHTTP(w, r)
	}))
	r.Get("/robots.txt", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=7884000") // 1 year / 4 = 3 months
		w.Header().Set("Expires", time.Now().AddDate(0, 3, 0).Format(http.TimeFormat))
		// Read robots.txt from embedded static files
		robotsContent, err := web.StaticFiles.ReadFile("static/robots.txt")
		if err != nil {
			http.Error(w, "robots.txt not found", http.StatusNotFound)
			return
		}
		w.Write(robotsContent)
	}))

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
