package routes

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/web"
)

// In development the asset URL (?version=dev) never changes, so caching it
// kept browsers on a stale bundle; release builds version the URL and may
// cache for months.
func TestStaticCacheHeaders(t *testing.T) {
	for _, dev := range []bool{true, false} {
		h := New(Deps{
			Auth:        auth.NewManager(nil, 0, false),
			StaticFiles: web.StaticFiles,
			Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
			DevAssets:   dev,
		})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/static/robots.txt", nil))
		cc := w.Header().Get("Cache-Control")
		if dev && cc != "no-cache" || !dev && cc != "public, max-age=7884000" {
			t.Errorf("dev=%v: Cache-Control = %q", dev, cc)
		}
	}
}
