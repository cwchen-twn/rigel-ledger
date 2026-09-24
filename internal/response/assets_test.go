package response

import (
	"testing"
	"testing/fstest"
)

func TestLoadAssets(t *testing.T) {
	fsys := fstest.MapFS{ManifestPath: {Data: []byte(`{
		"index.tsx": {"file": "main-CWqCP0iw.js", "isEntry": true},
		"style.css": {"file": "main-Dt0vvCan.css"},
		"font.woff2": {"file": "inter-Dx4kXJAl.woff2"}
	}`)}}
	a, err := loadAssets(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if a.JS != "/static/dist/main-CWqCP0iw.js" || len(a.CSS) != 1 || a.CSS[0] != "/static/dist/main-Dt0vvCan.css" {
		t.Fatalf("assets = %+v", a)
	}
	if a.BuildID() != "main-CWqCP0iw.js" {
		t.Fatalf("build id = %q", a.BuildID())
	}

	// An entry that names its CSS wins over loose stylesheets.
	a, _ = loadAssets(fstest.MapFS{ManifestPath: {Data: []byte(`{"i": {"file": "m-1.js", "isEntry": true, "css": ["m-2.css"]}}`)}})
	if len(a.CSS) != 1 || a.CSS[0] != "/static/dist/m-2.css" {
		t.Fatalf("entry css = %+v", a)
	}

	// Not built yet: an empty shell, not an error.
	if a, err := loadAssets(fstest.MapFS{}); err != nil || a.JS != "" {
		t.Fatalf("missing manifest: %+v %v", a, err)
	}
	if _, err := loadAssets(fstest.MapFS{ManifestPath: {Data: []byte(`{}`)}}); err == nil {
		t.Fatal("a manifest without an entry was accepted")
	}
}
