// Static asset serving: the built React app is compiled into the binary, so
// `evie serve` is one file with no asset directory to lose. The dist tree is
// gitignored, so a fresh clone must run `npm --prefix internal/web/ui run
// build` before `go build` — the embed failure names the missing directory.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// all: keeps files Go would otherwise skip (anything starting with _ or .),
// which Vite can emit into assets/.
//
//go:embed all:ui/dist
var distFS embed.FS

// staticHandler serves the embedded build, falling back to index.html for
// unknown paths so a client-side route (none today, one day maybe) resolves.
// The mux routes /api/* before this, so nothing here can shadow the API.
func (s *Server) staticHandler() http.Handler {
	dist, err := fs.Sub(distFS, "ui/dist")
	if err != nil {
		// Only reachable if the embed directive and this path disagree,
		// which is a build-time mistake, not a runtime condition.
		panic("web: embedded ui/dist missing: " + err.Error())
	}
	files := http.FileServerFS(dist)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			jsonError(w, http.StatusMethodNotAllowed, "static assets are GET only")
			return
		}

		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			serveIndex(w, r, dist)
			return
		}
		if _, err := fs.Stat(dist, name); err != nil {
			// Not in the build — hand back the app shell rather than a 404.
			serveIndex(w, r, dist)
			return
		}
		// Vite fingerprints everything under assets/, so those are safe to
		// cache forever; a rebuild changes the name.
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}

// serveIndex hands back the app shell with caching off — index.html names the
// hashed bundles, so a stale copy would point at assets that no longer exist.
// ServeFileFS, not the file server: the latter redirects requests for
// index.html to "./" to keep URLs canonical, which is exactly the URL we're
// answering, so it would 301 in a circle.
func serveIndex(w http.ResponseWriter, r *http.Request, dist fs.FS) {
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFileFS(w, r, dist, "index.html")
}
