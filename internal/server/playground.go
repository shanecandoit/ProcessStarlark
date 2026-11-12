package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

// WithPlayground serves a built playground and API on one origin. The static
// build remains usable by itself, without server drafts or cross-origin access.
func WithPlayground(api http.Handler, dir string) (http.Handler, error) {
	for _, name := range []string{"index.html", "app.js", "draft-client.js", "park.wasm", "worker.js"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.IsDir() {
			return nil, fmt.Errorf("playground build missing %s; run sh web/build.sh", name)
		}
	}
	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	mux.Handle("/healthz", api)
	mux.HandleFunc("GET /draft-config.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte("globalThis.PARK_DRAFT_API = true;\n"))
	})
	mux.Handle("/", http.FileServer(http.Dir(dir)))
	return mux, nil
}
