//go:build prod
// +build prod

package routes

// setupSwagger is a no-op function for production builds
// Swagger is completely disabled in production for security and performance
func (rt *Router) setupSwagger(appURL string, appPort int) {
	// No-op: Swagger is disabled in production
}
