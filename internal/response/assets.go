package response

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// ManifestPath is where Vite writes its manifest (build.manifest in
// web/vite.config.ts). Not under .vite/: go:embed skips dot directories.
const ManifestPath = "static/dist/manifest.json"

// Assets are the content-hashed files the shell links. Their names change
// whenever their contents do, so /static/dist can be cached forever.
type Assets struct {
	JS  string   // "/static/dist/main-3f9a1c2b.js"
	CSS []string // "/static/dist/main-8b2e4d10.css"
}

// BuildID names this frontend build: the entry file's hashed name. API
// responses carry it (X-App-Build) so an open tab can notice a new build.
func (a Assets) BuildID() string { return path.Base(a.JS) }

type manifestEntry struct {
	File    string   `json:"file"`
	CSS     []string `json:"css"`
	IsEntry bool     `json:"isEntry"`
}

// loadAssets reads the manifest. A missing one (Go tests or tools run
// without `bun run build`) gives empty Assets: the shell still renders, only
// without the SPA. Anything else wrong with it is an error.
func loadAssets(fsys fs.FS) (Assets, error) {
	if fsys == nil {
		return Assets{}, nil
	}
	b, err := fs.ReadFile(fsys, ManifestPath)
	if errors.Is(err, fs.ErrNotExist) {
		return Assets{}, nil
	}
	if err != nil {
		return Assets{}, err
	}
	var m map[string]manifestEntry
	if err := json.Unmarshal(b, &m); err != nil {
		return Assets{}, fmt.Errorf("%s: %w", ManifestPath, err)
	}
	var a Assets
	var loose []string // with cssCodeSplit: false the one stylesheet is its own key ("style.css")
	for _, e := range m {
		switch {
		case e.IsEntry:
			a.JS = "/static/dist/" + e.File
			for _, c := range e.CSS {
				a.CSS = append(a.CSS, "/static/dist/"+c)
			}
		case strings.HasSuffix(e.File, ".css"):
			loose = append(loose, "/static/dist/"+e.File)
		}
	}
	if a.JS == "" {
		return Assets{}, fmt.Errorf("%s: no entry chunk", ManifestPath)
	}
	if len(a.CSS) == 0 {
		sort.Strings(loose)
		a.CSS = loose
	}
	return a, nil
}
