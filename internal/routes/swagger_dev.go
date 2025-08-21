//go:build !prod
// +build !prod

package routes

import (
	"fmt"

	_ "github.com/cwc1222/rigelledger/api"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

// setupSwagger sets up Swagger documentation endpoints for development builds
func (rt *Router) setupSwagger(appURL string, appPort int) {
	rt.Handler.Get("/swagger/*", httpSwagger.Handler(
		// The url pointing to API definition
		httpSwagger.URL(fmt.Sprintf("http://%s:%d/swagger/doc.json", appURL, appPort)),
	))
}
