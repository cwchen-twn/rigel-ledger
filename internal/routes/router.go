// Package routes maps HTTP to the ledger service: the JSON API under /api,
// static files, and the SPA shell for every other GET.
package routes

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httplog/v3"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/identity"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
	"github.com/cwchen-twn/rigel-ledger/internal/rates"
	"github.com/cwchen-twn/rigel-ledger/internal/response"
)

type Deps struct {
	Service  *ledger.Service
	Identity *identity.Service
	Rates    *rates.Service // nil: no scheduler (tests)
	Auth     *auth.Manager
	// TrustedProxies are the hops whose X-Forwarded-For is believed.
	TrustedProxies []netip.Prefix
	Templates      *response.TemplateEngine
	StaticFiles    fs.FS
	Logger         *slog.Logger
	AccessLogger   *slog.Logger // nil disables request logging (tests)
	LogLevel       slog.Level
	AppURL         string
	AppPort        int
	// Ready reports whether the app can serve (the database answers). Nil
	// means always ready -- tests that do not care.
	Ready func(ctx context.Context) error
}

type handlers struct {
	svc       *ledger.Service
	identity  *identity.Service
	rates     *rates.Service
	auth      *auth.Manager
	templates *response.TemplateEngine
	logger    *slog.Logger
}

// New builds the HTTP handler.
//
//	@title		RigelLedger API
//	@version	1.0
//	@description	Session cookie (browser) or Bearer token (scripts, mobile). Cookie-authenticated POST/PUT/PATCH/DELETE must send X-Rigel-Client.
func New(d Deps) http.Handler {
	h := &handlers{svc: d.Service, identity: d.Identity, rates: d.Rates, auth: d.Auth, templates: d.Templates, logger: d.Logger}
	r := chi.NewRouter()

	r.Use(middleware.Compress(6, "text/*", "application/*"))
	if d.AccessLogger != nil {
		r.Use(httplog.RequestLogger(d.AccessLogger, &httplog.Options{
			Level:         d.LogLevel,
			Schema:        httplog.SchemaECS,
			RecoverPanics: true,
		}))
	} else {
		r.Use(middleware.Recoverer)
	}
	r.Use(auth.ResolveClientIP(d.TrustedProxies))
	r.Use(d.Auth.Authenticate)

	// Kubelet probes. Registered after every middleware because chi panics on
	// a route defined before a Use. Authenticate is harmless here: no token,
	// no lookup.
	r.Get("/livez", func(w http.ResponseWriter, _ *http.Request) {
		// Liveness never touches the database: a Postgres outage must make the
		// pod unready, not restart it in a loop.
		response.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if d.Ready != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := d.Ready(ctx); err != nil {
				response.Error(w, http.StatusServiceUnavailable, "not_ready", "database unavailable", nil)
				return
			}
		}
		response.JSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	setupSwagger(r, d.AppURL, d.AppPort)

	if d.StaticFiles != nil {
		static := http.FileServer(http.FS(d.StaticFiles))
		r.Handle("/static/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/"+response.ManifestPath {
				// The one unhashed file in dist: it names the others.
				w.Header().Set("Cache-Control", "no-cache")
			} else if strings.HasPrefix(r.URL.Path, "/static/dist/") {
				// Content-hashed names (see response.Assets): a changed file
				// is a new URL, so a cached one never goes stale.
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				// favicon, robots: unhashed, so a day at most.
				w.Header().Set("Cache-Control", "public, max-age=86400")
			}
			static.ServeHTTP(w, r)
		}))
		r.Get("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
			b, err := fs.ReadFile(d.StaticFiles, "static/robots.txt")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write(b)
		})
	}

	r.Route("/api", func(r chi.Router) {
		// Tell the SPA which frontend build is current, so a tab still running
		// an older bundle can offer a reload instead of misreading new answers.
		if d.Templates != nil {
			r.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if a, err := d.Templates.Assets(); err == nil && a.JS != "" {
						w.Header().Set("X-App-Build", a.BuildID())
					}
					next.ServeHTTP(w, r)
				})
			})
		}
		r.Post("/auth/login", h.login)
		r.Post("/auth/mfa", h.verifyMFA)
		r.Post("/auth/mfa/email", h.sendSignInCode)
		r.Post("/auth/passkey/begin", h.beginPasskeyLogin)
		r.Post("/auth/passkey/finish", h.finishPasskeyLogin)
		r.Get("/auth/config", h.authConfig)
		r.Post("/auth/register", h.register)
		r.Post("/auth/verify-link", h.verifyLink)
		r.Post("/auth/request-access", h.requestAccess)
		r.Get("/auth/invite/{token}", h.invitePreview)
		r.Post("/auth/invite/{token}", h.acceptInvite)
		r.Get("/currencies", h.currencies)
		r.Get("/commodities", h.commodities)

		r.Group(func(r chi.Router) {
			r.Use(auth.RequireUser(writeAuthError))
			r.Use(auth.TokenScope(tokenAllowed, writeAuthError))
			// Reachable before the first-login wizard is finished: it uses them.
			r.Post("/auth/logout", h.logout)
			r.Get("/me", h.me)
			r.Post("/me/email", h.startEmail)
			r.Post("/me/email/confirm", h.confirmEmail)
			r.Delete("/me/email/pending", h.cancelEmail)
			r.Post("/me/onboarding", h.onboarding)

			r.Group(func(r chi.Router) {
				r.Use(auth.RequireInitialized(writeAuthError))
				// Enrolment is open to a password-only session: it is how
				// that session becomes two-factor.
				r.Get("/me/mfa", h.mfaStatus)
				r.Post("/me/mfa/totp", h.startTOTP)
				r.Post("/me/mfa/totp/confirm", h.confirmTOTP)
				r.Post("/me/mfa/email", h.startEmailFactor)
				r.Post("/me/mfa/email/confirm", h.confirmEmailFactor)
				r.Post("/me/mfa/passkeys/begin", h.beginPasskeyRegistration)
				r.Post("/me/mfa/passkeys/finish", h.finishPasskeyRegistration)

				r.Group(func(r chi.Router) {
					r.Use(h.auth.RequireMFA(writeAuthError))
					r.Delete("/me/mfa/totp", h.removeFactor("totp"))
					r.Delete("/me/mfa/email", h.removeFactor("email"))
					r.Delete("/me/mfa/passkeys/{passkeyID}", h.removeFactor("passkey"))
					r.Post("/me/mfa/recovery-codes", h.regenerateRecoveryCodes)
					r.Patch("/me/mfa/alerts", h.setSignInAlerts)
					r.Patch("/me/settings", h.updateSettings)
					r.Post("/me/password", h.changePassword)
					r.Patch("/me/account", h.updateIdentity)
					r.Get("/me/sessions", h.listSessions)
					r.Delete("/me/sessions/{sessionID}", h.revokeSession)
					r.Get("/me/events", h.myEvents)
					r.Post("/me/tokens", h.createToken)

					r.Route("/admin", func(r chi.Router) {
						r.Use(auth.RequireAdmin(writeAuthError))
						r.Get("/settings", h.adminSettings)
						r.Patch("/settings", h.updateAdminSettings)
						r.Patch("/mail", h.updateAdminMail)
						r.Post("/mail/test", h.testMail)
						r.Get("/users", h.adminUsers)
						r.Patch("/users/{userID}", h.adminUpdateUser)
						r.Post("/invitations", h.invite)
						r.Post("/invitations/{userID}/resend", h.resendInvite)
						r.Delete("/invitations/{userID}", h.revokeInvite)
						r.Get("/access-requests", h.accessRequests)
						r.Post("/access-requests/{requestID}/approve", h.approveRequest)
						r.Post("/access-requests/{requestID}/reject", h.rejectRequest)
						r.Get("/events", h.adminEvents)
						r.Post("/users/{userID}/reset-mfa", h.resetMFA)
						r.Get("/rates", h.rateFetches)
						r.Post("/rates/refresh", h.refreshRates)
					})

					r.Get("/books", h.listBooks)
					r.Post("/books", h.createBook)
					r.Route("/books/{bookID}", func(r chi.Router) {
						r.Use(h.bookAccess)
						r.Get("/", h.getBook)
						r.Patch("/", h.updateBook)
						r.Delete("/", h.deleteBook)

						r.Get("/members", h.listMembers)
						r.Post("/members", h.addMember)
						r.Patch("/members/{userID}", h.updateMember)
						r.Delete("/members/{userID}", h.removeMember)

						r.Get("/accounts", h.listAccounts)
						r.Post("/accounts", h.createAccount)
						r.Patch("/accounts/{accountID}", h.updateAccount)
						r.Post("/accounts/{accountID}/archive", h.archiveAccount)
						r.Delete("/accounts/{accountID}", h.deleteAccount)
						r.Get("/accounts/{accountID}/cost", h.costBasis)
						r.Post("/commodities", h.createCommodity)

						r.Get("/transactions", h.listTransactions)
						r.Post("/transactions", h.createTransaction)
						r.Get("/transactions/{transactionID}", h.getTransaction)
						r.Put("/transactions/{transactionID}", h.updateTransaction)
						r.Delete("/transactions/{transactionID}", h.deleteTransaction)
						r.Get("/tags", h.listTags)

						r.Get("/balances", h.balances)

						r.Get("/prices", h.listPrices)
						r.Post("/prices", h.addPrice)
						r.Delete("/prices/{priceID}", h.deletePrice)
						r.Get("/rate", h.rate)
						r.Get("/rates/current", h.currentRates)
						r.Get("/reports/balance-sheet", h.balanceSheet)
						r.Get("/reports/income-statement", h.incomeStatement)
						r.Get("/reports/cash-flow", h.cashFlow)
						r.Get("/reports/tags", h.tagReport)
						r.Get("/reports/tag", h.tagDetail)
						r.Post("/rebase", h.rebaseBook)

						r.Post("/imports", h.importBatch)
						r.Get("/imports/sources", h.listSources)
						r.Patch("/imports/sources/{sourceID}", h.mapSource)
						r.Get("/imports/queue", h.importQueue)
						r.Post("/imports/accept", h.acceptRows)
						r.Post("/imports/ignore", h.ignoreRows)
						r.Get("/imports/rules", h.listRules)
						r.Post("/imports/rules", h.createRule)
						r.Delete("/imports/rules/{ruleID}", h.deleteRule)
						r.Get("/drift", h.drift)
					})
				})
			})
		})

		r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			response.Error(w, http.StatusNotFound, "not_found", "no such endpoint", nil)
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
			response.Error(w, http.StatusMethodNotAllowed, "method_not_allowed", "", nil)
		})
	})

	// Every other GET is a client-side route: serve the shell and let the SPA
	// decide what to show (including its own login redirect).
	r.Get("/*", h.shell)
	return r
}

func (h *handlers) shell(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/static/") {
		http.NotFound(w, r)
		return
	}
	lang, theme := "", ""
	if id, ok := auth.FromContext(r.Context()); ok {
		lang, theme = id.User.Language, id.User.Theme
	}
	if err := h.templates.RenderShell(w, lang, theme); err != nil {
		h.logger.Error("render shell", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
