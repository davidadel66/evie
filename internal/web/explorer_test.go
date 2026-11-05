package web

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The System page serves the system map page built into the binary
// (system-page.spec.md). These tests read the real snapshot and compare it
// with the page in this checkout.

func explorerGet(t *testing.T, h http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, "http://127.0.0.1:6687"+target, nil))
	return rec
}

func explorerFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "architecture", "explorer", filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// wantMapPolicy is the policy every /explorer/ response carries: the map
// page's inline scripts by hash, inline styles, and nothing from any other
// host.
func wantMapPolicy(t *testing.T) string {
	t.Helper()
	var scripts []string
	for _, m := range regexp.MustCompile(`(?s)<script>(.*?)</script>`).FindAllStringSubmatch(explorerFile(t, "system.html"), -1) {
		sum := sha256.Sum256([]byte(m[1]))
		scripts = append(scripts, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
	}
	if len(scripts) == 0 {
		t.Fatal("system.html has no inline script; was it built from the vision template?")
	}
	return "sandbox allow-scripts allow-popups allow-popups-to-escape-sandbox; " +
		"default-src 'none'; script-src " + strings.Join(scripts, " ") + "; style-src 'unsafe-inline'; img-src 'self' data:; " +
		"base-uri 'none'; form-action 'none'; frame-ancestors 'self'"
}

func TestExplorerServesTheMapPageByteForByte(t *testing.T) {
	// The page is the snapshot's only file, and its index.
	h := newTestServer(&fakeClient{})
	for _, target := range []string{"/explorer/", "/explorer/?theme=dark", "/explorer/system.html", "/explorer/system.html?theme=dark"} {
		rec := explorerGet(t, h, http.MethodGet, target)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200\n%s", target, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
			t.Errorf("GET %s: Content-Type = %q, want text/html; charset=utf-8", target, got)
		}
		if rec.Body.String() != explorerFile(t, "system.html") {
			t.Errorf("GET %s did not serve system.html byte for byte", target)
		}
	}
}

func TestExplorerBareAddressRedirectsToTheDirectory(t *testing.T) {
	h := newTestServer(&fakeClient{})
	for _, target := range []string{"/explorer", "/explorer?theme=dark"} {
		rec := explorerGet(t, h, http.MethodGet, target)
		if rec.Code != http.StatusMovedPermanently {
			t.Fatalf("GET %s: status = %d, want 301", target, rec.Code)
		}
		if got, want := rec.Header().Get("Location"), "/explorer/"+strings.TrimPrefix(target, "/explorer"); got != want {
			t.Fatalf("GET %s: Location = %q, want %q", target, got, want)
		}
	}
}

func TestExplorerAnswersGetAndHeadOnly(t *testing.T) {
	h := newTestServer(&fakeClient{})
	for _, target := range []string{"/explorer", "/explorer/", "/explorer/system.html", "/explorer/nope.js"} {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions} {
			rec := explorerGet(t, h, method, target)
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("%s %s: status = %d, want 405", method, target, rec.Code)
			}
			if got := rec.Header().Get("Allow"); got != "GET, HEAD" {
				t.Fatalf("%s %s: Allow = %q, want GET, HEAD", method, target, got)
			}
			// Like Evie's static files, the refusal is a JSON error.
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("%s %s: Content-Type = %q, want application/json", method, target, got)
			}
			if got := rec.Body.String(); got != `{"error":"the explorer is GET only"}`+"\n" {
				t.Fatalf("%s %s: body = %q", method, target, got)
			}
		}
	}
	for _, target := range []string{"/explorer/", "/explorer/system.html"} {
		rec := explorerGet(t, h, http.MethodHead, target)
		if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
			t.Fatalf("HEAD %s: status = %d with %d body bytes, want 200 and none", target, rec.Code, rec.Body.Len())
		}
	}
}

func TestExplorerRefusesFilesOutsideTheSnapshot(t *testing.T) {
	// The retired explorer viewer's files, build.js among them, and the
	// files beside the page are not served.
	h := newTestServer(&fakeClient{})
	policy := wantMapPolicy(t)
	for _, target := range []string{
		"/explorer/index.html",
		"/explorer/explorer.js",
		"/explorer/explorer.css",
		"/explorer/build.js",
		"/explorer/data/facts.js",
		"/explorer/data/manifest.js",
		"/explorer/examples/design-proposal.js",
		"/explorer/README.md",
		"/explorer/snapshot.go",
		"/explorer/data",
		"/explorer/data/",
		"/explorer/system.html/x",
		"/explorer/nope.js",
	} {
		rec := explorerGet(t, h, http.MethodGet, target)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s: status = %d, want 404\n%s", target, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Content-Security-Policy"); got != policy {
			t.Fatalf("GET %s: a refusal carries policy %q, want %q", target, got, policy)
		}
	}
}

func TestExplorerHeadersAllowFramingOnlyByEvie(t *testing.T) {
	h := newTestServer(&fakeClient{})
	want := map[string]string{
		"Content-Security-Policy": wantMapPolicy(t),
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "no-referrer",
		"Cache-Control":           "no-store",
	}
	for _, target := range []string{"/explorer", "/explorer/", "/explorer/system.html", "/explorer/nope.js"} {
		rec := explorerGet(t, h, http.MethodGet, target)
		for name, value := range want {
			if got := rec.Header().Get(name); got != value {
				t.Errorf("GET %s: %s = %q, want %q", target, name, got, value)
			}
		}
	}
}

func TestExplorerIsServedOutsideTheAPIGuard(t *testing.T) {
	// Like Evie's own static files: a plain browser GET with no JSON
	// content type, from a page Evie frames, gets the page.
	h := newTestServer(&fakeClient{})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:6687/explorer/", nil)
	req.Header.Set("Sec-Fetch-Dest", "iframe")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("framed GET /explorer/: status = %d, want 200", rec.Code)
	}
}

// policyDirectives splits a content-security policy into its directives by
// name.
func policyDirectives(policy string) map[string]string {
	directives := map[string]string{}
	for _, d := range strings.Split(policy, ";") {
		name, value, _ := strings.Cut(strings.TrimSpace(d), " ")
		directives[name] = value
	}
	return directives
}

func TestSystemMapPageRunsOnlyItsOwnInlineScripts(t *testing.T) {
	// The vision page is one file with inline scripts and styles: its
	// scripts run by hash, never any other inline script, and it reaches no
	// host but Evie.
	h := newTestServer(&fakeClient{})
	rec := explorerGet(t, h, http.MethodGet, "/explorer/?theme=dark")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /explorer/: status = %d, want 200", rec.Code)
	}
	header := rec.Header().Get("Content-Security-Policy")
	if header != wantMapPolicy(t) {
		t.Errorf("policy = %q, want %q", header, wantMapPolicy(t))
	}
	// default-src 'none' covers fonts and connections: the page reaches no
	// host but Evie, so no directive may name one.
	policy := policyDirectives(header)
	for _, name := range []string{"connect-src", "font-src", "frame-src"} {
		if _, ok := policy[name]; ok {
			t.Errorf("the map page may load %s: %q", name, header)
		}
	}
	if strings.Contains(header, "http") {
		t.Errorf("the map page's policy names another host: %q", header)
	}
	// The page Evie embeds asks for no web fonts, so the policy blocks nothing
	// it loads.
	page := explorerFile(t, "system.html")
	for _, host := range []string{"fonts.googleapis.com", "fonts.gstatic.com"} {
		if strings.Contains(page, host) {
			t.Errorf("system.html loads from %s; build it to meta.page so it uses the system fonts", host)
		}
	}
	// It loads no other file of its own: the snapshot holds only the page.
	if regexp.MustCompile(`(?i)<(?:script|link|img|iframe)\b[^>]*\b(?:src|href)\s*=`).MatchString(page) {
		t.Errorf("system.html loads a file; the snapshot holds only the page")
	}
}

func TestSystemMapPolicyHashesOnlyScriptsTheBrowserRuns(t *testing.T) {
	page := []byte("<title>x</title><script>run()</script>\n" +
		`<script type="application/json" id="vision-data">{"a":1}</script>` +
		`<script src="x.js"></script><script type="module">mod()</script><SCRIPT data-type="x">upper()</SCRIPT>`)
	hash := func(s string) string {
		sum := sha256.Sum256([]byte(s))
		return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	}
	want := strings.Join([]string{hash("run()"), hash("mod()"), hash("upper()")}, " ")
	if got := policyDirectives(systemMapPolicy(page))["script-src"]; got != want {
		t.Errorf("script-src = %q, want %q", got, want)
	}
	// The browser runs every JavaScript type, read by its essence (case and
	// parameters aside), and any type it does not know as data: only data
	// blocks go unhashed.
	page = []byte(`<script type="text/ecmascript">ecma()</script>` +
		`<script type="text/javascript;charset=utf-8">params()</script>` +
		`<script type='TEXT/JavaScript; charset="utf-8"'>quoted()</script>` +
		`<script type="">empty()</script>` +
		`<script type="Application/JSON">{"a":1}</script>` +
		`<script type="application/json; charset=utf-8">{"b":2}</script>` +
		`<script type="application/ld+json">{"@type":"x"}</script>`)
	want = strings.Join([]string{hash("ecma()"), hash("params()"), hash("quoted()"), hash("empty()")}, " ")
	if got := policyDirectives(systemMapPolicy(page))["script-src"]; got != want {
		t.Errorf("script-src = %q, want %q", got, want)
	}
	// A quoted attribute value may hold a ">", which does not end the tag:
	// the hash covers exactly the script's text, and an attribute after it
	// still counts.
	page = []byte(`<script data-note="a>b">body()</script>` +
		`<script data-note='x>y' type="application/json">{"c":3}</script>` +
		`<script data-note="p>q" src="x.js"></script>`)
	if got, want := policyDirectives(systemMapPolicy(page))["script-src"], hash("body()"); got != want {
		t.Errorf("script-src = %q, want %q", got, want)
	}
	if got := policyDirectives(systemMapPolicy(nil))["script-src"]; got != "'none'" {
		t.Errorf("with no page, script-src = %q, want 'none'", got)
	}
}
