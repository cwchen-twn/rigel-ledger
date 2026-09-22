package response

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"sync"
)

// Shell is the one HTML page the server renders: <div id="app"> plus the
// bundle. Everything else is the SolidJS app. It carries the signed-in user's
// language and theme so the first paint is already right.
type Shell struct {
	Version string
	Nonce   string
	Lang    string
	// "light", "dark" or "system"; the inline script resolves "system".
	Theme string
}

type TemplateEngine struct {
	fsys     fs.FS
	version  string
	reparse  bool // development: pick up template edits without a restart
	isSecure bool

	once sync.Once
	tmpl *template.Template
	err  error
}

func NewTemplateEngine(version string, fsys fs.FS, development bool) *TemplateEngine {
	return &TemplateEngine{fsys: fsys, version: version, reparse: development, isSecure: !development}
}

func (te *TemplateEngine) parse() (*template.Template, error) {
	return template.ParseFS(te.fsys, "templates/partials/*.tmpl", "templates/app.tmpl")
}

func (te *TemplateEngine) template() (*template.Template, error) {
	if te.reparse {
		return te.parse()
	}
	te.once.Do(func() { te.tmpl, te.err = te.parse() })
	return te.tmpl, te.err
}

func newNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func (te *TemplateEngine) setHeaders(w http.ResponseWriter, nonce string) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", fmt.Sprintf(
		"default-src 'self'; script-src 'self' 'nonce-%s' 'strict-dynamic'; style-src 'self' 'unsafe-inline'; "+
			"img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'",
		nonce))
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
	h.Set("Cache-Control", "no-cache, must-revalidate")
	h.Set("X-Frame-Options", "DENY")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Robots-Tag", "noindex, nofollow")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	if te.isSecure {
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	}
}

// RenderShell writes the SPA shell.
func (te *TemplateEngine) RenderShell(w http.ResponseWriter, lang, theme string) error {
	t, err := te.template()
	if err != nil {
		return err
	}
	if lang == "" {
		lang = "en"
	}
	if theme == "" {
		theme = "system"
	}
	nonce := newNonce()
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "app", Shell{Version: te.version, Nonce: nonce, Lang: lang, Theme: theme}); err != nil {
		return err
	}
	te.setHeaders(w, nonce)
	_, err = buf.WriteTo(w)
	return err
}
