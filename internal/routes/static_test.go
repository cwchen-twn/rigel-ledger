package routes

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/response"
	"github.com/cwchen-twn/rigel-ledger/web"
)

// Hashed bundle files are immutable; the manifest and the unhashed files
// are not. The shell links the hashed names, and API answers carry the
// build so an old tab can notice a new one.
func TestStaticCacheAndBuildHeader(t *testing.T) {
	te := response.NewTemplateEngine("test", web.TemplateFiles, web.StaticFiles, false)
	assets, err := te.Assets()
	if err != nil {
		t.Fatal(err)
	}
	if assets.JS == "" {
		t.Skip("frontend not built (cd web && bun run build:prod)")
	}
	h := New(Deps{
		Auth:        auth.NewManager(nil, 0, false),
		Templates:   te,
		StaticFiles: web.StaticFiles,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}
	cases := map[string]string{
		assets.JS:                   "public, max-age=31536000, immutable",
		"/" + response.ManifestPath: "no-cache",
		"/static/robots.txt":        "public, max-age=86400",
	}
	for path, want := range cases {
		if w := get(path); w.Code != 200 || w.Header().Get("Cache-Control") != want {
			t.Errorf("%s: %d, Cache-Control %q, want %q", path, w.Code, w.Header().Get("Cache-Control"), want)
		}
	}
	shell := get("/login")
	body := shell.Body.String()
	if !strings.Contains(body, `src="`+assets.JS+`"`) || !strings.Contains(body, `href="`+assets.CSS[0]+`"`) {
		t.Errorf("shell does not link the hashed bundle %+v", assets)
	}
	if cc := shell.Header().Get("Cache-Control"); !strings.HasPrefix(cc, "no-cache") {
		t.Errorf("shell Cache-Control = %q", cc)
	}
	if b := get("/api/nope").Header().Get("X-App-Build"); b != assets.BuildID() {
		t.Errorf("X-App-Build = %q, want %q", b, assets.BuildID())
	}
}
