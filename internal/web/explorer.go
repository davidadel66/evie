// The System page (system-page.spec.md): the system map page built into
// the binary, served under /explorer/ for Evie's sandboxed frame. It sits
// outside the API guard like Evie's own static files: it serves only the
// public repository's architecture and reads no data.
package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"

	explorer "github.com/davidadel66/evie/docs/architecture/explorer"
)

// systemMapPage is the System view's page: the map, its Architecture and
// its History in one self-contained file the vision skill builds from
// .vision/map.json (system-page.decisions.md, 2026-11-04 and 2026-11-05).
// It is the snapshot's only file, and its index.
const systemMapPage = "system.html"

// systemMapPolicy is the content-security policy every /explorer/ response
// carries, with what a page built from the vision template needs: its
// inline scripts, allowed by their hashes (the page is fixed for the life
// of the binary), and inline styles, since it styles elements and SVG it
// draws as markup. Its map data is a JSON block, which the browser never
// runs. The document is sandboxed with scripts and popups but without
// same-origin, so even opened on its own it has an opaque origin and cannot
// use Evie's routes or storage; popups escape the sandbox so History's
// GitHub links open normally. It loads images only from Evie or data: URLs,
// opens no connections, frames, or forms, and only Evie's own page may
// frame it. Like the rest of Evie, it reaches no other host: a page built
// to meta.page uses the system fonts, not Google's.
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
		"default-src 'none'; script-src " + scriptSrc + "; style-src 'unsafe-inline'; img-src 'self' data:; " +
		"base-uri 'none'; form-action 'none'; frame-ancestors 'self'"
}

// scriptDataTypes are the data blocks a vision page carries, which the
// browser never runs: its map data is application/json.
var scriptDataTypes = map[string]bool{"application/json": true, "application/ld+json": true}

// inlineScripts returns the text of each inline script element the browser
// may run: every one without src whose type, by its essence (lower case,
// without parameters), is not a data block. Hashing a script of a type the
// browser does not run only allows what never runs. The page is read as the
// browser tokenizes it, so a quoted attribute value may hold ">", a script
// in a comment is no script, and the text is what the browser hashes: the
// element's raw text up to its end tag, with line endings as LF.
func inlineScripts(page []byte) [][]byte {
	var scripts [][]byte
	z := html.NewTokenizer(bytes.NewReader(page))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return scripts
		case html.StartTagToken, html.SelfClosingTagToken:
			if name, hasAttr := z.TagName(); string(name) == "script" && runsInline(z, hasAttr) {
				// The tokenizer reads a script's text raw: one text token, or
				// none when the element is empty.
				text := []byte{}
				if z.Next() == html.TextToken {
					text = append(text, z.Text()...)
				}
				scripts = append(scripts, text)
			}
		}
	}
}

// runsInline reads a script start tag's attributes and reports whether the
// browser runs its text: it has no src, and its type, the first one given,
// is not a data block.
func runsInline(z *html.Tokenizer, hasAttr bool) bool {
	runs, typed := true, false
	for hasAttr {
		var key, value []byte
		key, value, hasAttr = z.TagAttr()
		switch string(key) {
		case "src":
			runs = false
		case "type":
			if !typed && scriptDataTypes[mimeEssence(string(value))] {
				runs = false
			}
			typed = true
		}
	}
	return runs
}

// mimeEssence is a MIME type without its parameters, trimmed and lower case.
func mimeEssence(mimeType string) string {
	essence, _, _ := strings.Cut(mimeType, ";")
	return strings.ToLower(strings.TrimSpace(essence))
}

func (s *Server) registerExplorerRoutes(mux *http.ServeMux) {
	// The page is fixed for the life of the binary, so the hashes of its
	// scripts are taken once. The snapshot embeds it by name, so a failed
	// read is a broken build.
	page, err := fs.ReadFile(explorer.Files(), systemMapPage)
	if err != nil {
		panic("web: embedded explorer " + systemMapPage + " missing: " + err.Error())
	}
	policy := systemMapPolicy(page)
	// The page lives at the directory, so the bare address moves there,
	// keeping the query (the theme).
	mux.HandleFunc("/explorer", func(w http.ResponseWriter, r *http.Request) {
		if !explorerRequest(w, r, policy) {
			return
		}
		target := "/explorer/"
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})
	// The directory and the page's own name serve the page; nothing else is
	// in the snapshot.
	mux.HandleFunc("/explorer/", func(w http.ResponseWriter, r *http.Request) {
		if !explorerRequest(w, r, policy) {
			return
		}
		if name := strings.TrimPrefix(r.URL.Path, "/explorer/"); name != "" && name != systemMapPage {
			http.Error(w, "This file is not part of the explorer.", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, systemMapPage, time.Time{}, bytes.NewReader(page))
	})
}

// explorerRequest sets the headers every explorer response carries, errors
// included, and refuses methods other than GET and HEAD.
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
