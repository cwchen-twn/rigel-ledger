//go:build prod

package routes

import "github.com/go-chi/chi/v5"

// setupSwagger is a no-op in production builds: no API explorer is exposed.
func setupSwagger(chi.Router, string, int) {}
