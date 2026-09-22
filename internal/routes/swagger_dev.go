//go:build !prod

package routes

import (
	"fmt"

	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "github.com/cwchen-twn/rigel-ledger/api"
)

// setupSwagger mounts Swagger UI at /swagger/ in non-production builds.
func setupSwagger(r chi.Router, appURL string, appPort int) {
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL(fmt.Sprintf("http://%s:%d/swagger/doc.json", appURL, appPort)),
	))
}
