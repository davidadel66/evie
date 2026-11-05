// The System page (system-page.spec.md): the system explorer snapshot built
// into the binary, served under /explorer/ for Evie's sandboxed frame. It
// sits outside the API guard like Evie's own static files: it serves only
// the public repository's architecture and reads no data.
package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"regexp"
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

// systemMapPage is the System view's map: one self-contained page the vision
// skill builds from .vision/map.json (system-page.decisions.md, 2026-11-04).
const systemMapPage = "system.html"

// systemMapPolicy seals the map page the same way, with what a page built
// from the vision template needs: its inline scripts, allowed by their
// hashes (the page is fixed for the life of the binary); inline styles, since
// it styles elements and SVG it draws as markup; and its web fonts, the Geist
// stylesheet from Google Fonts and the font files it names. Its map data is
// a JSON block, which the browser never runs.
func systemMapPolicy(page []byte) string {
	sources := make([]string, 0, 2)
	for _, script := range inlineScripts(page) {
		sum := sha256.Sum256(script)
		sources = append(sources, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
	}
	scriptSrc := "'none'"
	if len(sources) > 0 {
		scriptSrc = strings.Join(sources, " ")
	}
	return "sandbox allow-scripts allow-popups allow-popups-to-escape-sandbox; " +
		"default-src 'none'; script-src " + scriptSrc + "; " +
		"style-src 'unsafe-inline' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; img-src 'self' data:; " +
		"base-uri 'none'; form-action 'none'; frame-ancestors 'self'"
}

// scriptElement is a script element: its attributes and its text.
var scriptElement = regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script`)

// scriptType and scriptSource read a script element's type and src
// attributes.
var (
	scriptType   = regexp.MustCompile(`(?i)(?:^|\s)type\s*=\s*["']?([^"'\s>]+)`)
	scriptSource = regexp.MustCompile(`(?i)(?:^|\s)src\s*=`)
)

// inlineScripts returns the text of each script element the browser runs
// inline: no src, and no type, or a JavaScript type. A data block such as
// type="application/json" is never run, so it needs no hash.
func inlineScripts(page []byte) [][]byte {
	var scripts [][]byte
	for _, m := range scriptElement.FindAllSubmatch(page, -1) {
		attrs := m[1]
		if scriptSource.Match(attrs) {
			continue
		}
		if t := scriptType.FindSubmatch(attrs); t != nil {
			switch strings.ToLower(string(t[1])) {
			case "text/javascript", "application/javascript", "module":
			default:
				continue
			}
		}
		scripts = append(scripts, m[2])
	}
	return scripts
}

// explorerBuild is what build.js tells the page about the running binary,
// read from Go's build stamp: where its code lives, the commit it was built
// from ("" when unstamped), and whether the checkout had uncommitted
// changes. The page links code to GitHub with it.
type explorerBuild struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Modified   bool   `json:"modified"`
}

// readExplorerBuild reads the build stamp; nil means the binary has none.
func readExplorerBuild(info *debug.BuildInfo) explorerBuild {
	var build explorerBuild
	if info == nil {
		return build
	}
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

// explorerBuildJS is build.js: one call telling the page about the
// build.
func explorerBuildJS(build explorerBuild) []byte {
	// Marshalling two strings and a bool cannot fail.
	data, _ := json.Marshal(build)
	return append(append([]byte("evieExplorer.build("), data...), ");\n"...)
}

func (s *Server) registerExplorerRoutes(mux *http.ServeMux) {
	files := explorer.Files()
	// The build stamp is fixed for the life of the binary, so build.js is
	// written once.
	info, ok := s.buildInfo()
	if !ok {
		info = nil
	}
	buildScript := explorerBuildJS(readExplorerBuild(info))
	// The map page is fixed for the life of the binary too, so the hashes of
	// its scripts are taken once. A snapshot without it still has a policy.
	mapPage, _ := fs.ReadFile(files, systemMapPage)
	mapPolicy := systemMapPolicy(mapPage)
	// The page's files are relative to its directory, so the bare address
	// moves to the directory, keeping the query (the embedded flag).
	mux.HandleFunc("/explorer", func(w http.ResponseWriter, r *http.Request) {
		if !explorerRequest(w, r, explorerPolicy) {
			return
		}
		target := "/explorer/"
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})
	// Files are read by hand rather than through http.FileServerFS, which
	// would redirect index.html to the directory, list directories, and
	// type files by the platform's MIME table.
	mux.HandleFunc("/explorer/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/explorer/")
		policy := explorerPolicy
		if name == systemMapPage {
			policy = mapPolicy
		}
		if !explorerRequest(w, r, policy) {
			return
		}
		if name == "build.js" {
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(buildScript))
			return
		}
		if name == "" {
			name = "index.html"
		}
		// The snapshot is in memory, so a read fails only for a name it does
		// not hold, an invalid path, or a directory.
		data, err := fs.ReadFile(files, name)
		if err != nil {
			http.Error(w, "This file is not part of the explorer.", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", explorerContentType(name))
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
	})
}

// explorerRequest sets the headers every explorer response carries, errors
// included, with the file's policy, and refuses methods other than GET and
// HEAD.
func explorerRequest(w http.ResponseWriter, r *http.Request, policy string) bool {
	header := w.Header()
	header.Set("Content-Security-Policy", policy)
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Referrer-Policy", "no-referrer")
	// The snapshot changes with the binary, so nothing is cached across
	// a rebuild.
	header.Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		header.Set("Allow", "GET, HEAD")
		jsonError(w, http.StatusMethodNotAllowed, "the explorer is GET only")
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
