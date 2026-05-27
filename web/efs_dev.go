//go:build dev

package web

import (
	"io/fs"
	"os"
)

// In dev mode, serve templates and static files directly from disk so that
// template edits are visible on the next request and Vite rebuilds are served
// immediately without recompiling Go. Requires running from the project root.
var TemplateFiles fs.FS = os.DirFS("web")
var StaticFiles fs.FS = os.DirFS("web")
