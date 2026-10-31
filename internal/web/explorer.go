// The System page (system-page.spec.md): the system explorer snapshot built
// into the binary, served under /explorer/ for Evie's sandboxed frame. It
// sits outside the API guard like Evie's own static files: it serves only
// the public repository's architecture and reads no data.
package web

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"runtime/debug"
	"strings"
	"time"

	explorer "github.com/davidadel66/evie/docs/architecture/explorer"
)

// explorerPolicy seals every explorer response. The document is sandboxed
// with scripts and popups but without same-origin, so even opened on its own
// it has an opaque origin and cannot use Evie's routes or storage; popups
// escape the sandbox so GitHub links open normally. It loads scripts and
// styles only from Evie (the explorer is classic scripts and one stylesheet,
// with no inline code or style attributes) and images only from Evie or
// data: URLs (the search icon), opens no connections, frames, or forms, and
// only Evie's own page may frame it.
const explorerPolicy = "sandbox allow-scripts allow-popups allow-popups-to-escape-sandbox; " +
	"default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'self'"

// explorerBuild is what build.js tells the page about the running binary,
// read from Go's build stamp: where its code lives, the commit it was built
// from ("" when unstamped), and whether the checkout had uncommitted
// changes. The page links code to GitHub with it.
type explorerBuild struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Modified   bool   `json:"modified"`
}

func readExplorerBuild(read func() (*debug.BuildInfo, bool)) explorerBuild {
	info, ok := read()
	if !ok || info == nil {
		return explorerBuild{}
	}
	var build explorerBuild
	if info.Main.Path != "" {
		build.Repository = "https://" + info.Main.Path
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			build.Commit = setting.Value
		case "vcs.modified":
			build.Modified = setting.Value == "true"
		}
	}
	return build
}

func (s *Server) registerExplorerRoutes(mux *http.ServeMux) {
	files := explorer.Files()
	// The page's files are relative to its directory, so the bare address
	// moves to the directory, keeping the query (the embedded flag).
	mux.HandleFunc("/explorer", func(w http.ResponseWriter, r *http.Request) {
		if !explorerRequest(w, r) {
			return
		}
		target := "/explorer/"
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})
	mux.HandleFunc("/explorer/", func(w http.ResponseWriter, r *http.Request) {
		if !explorerRequest(w, r) {
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/explorer/")
		if name == "build.js" {
			script, err := json.Marshal(readExplorerBuild(s.buildInfo))
			if err != nil {
				http.Error(w, "The build information could not be written.", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			body := append(append([]byte("evieExplorer.build("), script...), ");\n"...)
			http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(body))
			return
		}
		if name == "" {
			name = "index.html"
		}
		if !fs.ValidPath(name) {
			http.Error(w, "This file is not part of the explorer.", http.StatusNotFound)
			return
		}
		if info, err := fs.Stat(files, name); err != nil || info.IsDir() {
			http.Error(w, "This file is not part of the explorer.", http.StatusNotFound)
			return
		}
		data, err := fs.ReadFile(files, name)
		if err != nil {
			http.Error(w, "The explorer could not be read.", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", explorerContentType(name))
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
	})
}

// explorerRequest sets the headers every explorer response carries, errors
// included, and refuses methods other than GET and HEAD.
func explorerRequest(w http.ResponseWriter, r *http.Request) bool {
	header := w.Header()
	header.Set("Content-Security-Policy", explorerPolicy)
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Referrer-Policy", "no-referrer")
	// The snapshot changes with the binary, so nothing is cached across
	// a rebuild.
	header.Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		header.Set("Allow", "GET, HEAD")
		http.Error(w, "The explorer is GET only.", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// explorerContentType is the type an explorer file is served as; the
// snapshot holds only the page, scripts, and a stylesheet.
func explorerContentType(name string) string {
	switch path.Ext(name) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	}
	return "application/octet-stream"
}
