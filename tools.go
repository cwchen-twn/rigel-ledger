//go:build tools

package tools

import (
	_ "github.com/swaggo/swag"
)

// This keeps swag (the Swagger doc generator) tracked as a dependency
// so you can run go run github.com/swaggo/swag/cmd/swag@latest
// reliably with a pinned version.
