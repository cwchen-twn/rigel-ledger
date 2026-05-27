//go:build !dev

package web

import (
	"embed"
	"io/fs"
)

//go:embed "templates"
var templateFilesEmbed embed.FS
var TemplateFiles fs.FS = templateFilesEmbed

//go:embed "static"
var staticFilesEmbed embed.FS
var StaticFiles fs.FS = staticFilesEmbed
