package web

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"
	"testing"
)

// The System page serves the explorer snapshot built into the binary
// (system-page.spec.md). These tests read the real snapshot and compare it
// with the explorer files in this checkout.

const wantExplorerPolicy = "sandbox allow-scripts allow-popups allow-popups-to-escape-sandbox; " +
	"default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'self'"

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

func TestExplorerServesTheSnapshotByteForByte(t *testing.T) {
	h := newTestServer(&fakeClient{})
	cases := []struct{ target, file, contentType string }{
		{"/explorer/", "index.html", "text/html; charset=utf-8"},
		{"/explorer/index.html", "index.html", "text/html; charset=utf-8"},
		{"/explorer/explorer.js", "explorer.js", "text/javascript; charset=utf-8"},
		{"/explorer/explorer.css", "explorer.css", "text/css; charset=utf-8"},
		{"/explorer/data/manifest.js", "data/manifest.js", "text/javascript; charset=utf-8"},
		{"/explorer/data/facts.js", "data/facts.js", "text/javascript; charset=utf-8"},
		{"/explorer/examples/design-proposal.js", "examples/design-proposal.js", "text/javascript; charset=utf-8"},
		{"/explorer/system.html", "system.html", "text/html; charset=utf-8"},
	}
	for _, c := range cases {
		rec := explorerGet(t, h, http.MethodGet, c.target)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200\n%s", c.target, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Content-Type"); got != c.contentType {
			t.Errorf("GET %s: Content-Type = %q, want %q", c.target, got, c.contentType)
		}
		if rec.Body.String() != explorerFile(t, c.file) {
			t.Errorf("GET %s did not serve %s byte for byte", c.target, c.file)
		}
	}
}

func TestExplorerBareAddressRedirectsToTheDirectory(t *testing.T) {
	// The page's files are relative to its directory, so /explorer must
	// become /explorer/ before index.html is served.
	h := newTestServer(&fakeClient{})
	for _, target := range []string{"/explorer", "/explorer?embedded=1"} {
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
	for _, target := range []string{"/explorer", "/explorer/", "/explorer/explorer.js", "/explorer/build.js", "/explorer/system.html", "/explorer/nope.js"} {
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
	for _, target := range []string{"/explorer/", "/explorer/explorer.css", "/explorer/build.js"} {
		rec := explorerGet(t, h, http.MethodHead, target)
		if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
			t.Fatalf("HEAD %s: status = %d with %d body bytes, want 200 and none", target, rec.Code, rec.Body.Len())
		}
	}
}

func TestExplorerRefusesFilesOutsideTheSnapshot(t *testing.T) {
	h := newTestServer(&fakeClient{})
	for _, target := range []string{
		"/explorer/README.md",
		"/explorer/snapshot.go",
		"/explorer/test/model.test.js",
		"/explorer/test/fixture.js",
		"/explorer/data",
		"/explorer/data/",
		"/explorer/examples/",
		"/explorer/nope.js",
		"/explorer/data/nope.js",
	} {
		rec := explorerGet(t, h, http.MethodGet, target)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s: status = %d, want 404\n%s", target, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Content-Security-Policy"); got != wantExplorerPolicy {
			t.Fatalf("GET %s: a refusal carries policy %q, want %q", target, got, wantExplorerPolicy)
		}
	}
}

func TestExplorerHeadersAllowFramingOnlyByEvie(t *testing.T) {
	h := newTestServer(&fakeClient{})
	for _, target := range []string{"/explorer/", "/explorer/explorer.js", "/explorer/build.js"} {
		rec := explorerGet(t, h, http.MethodGet, target)
		want := map[string]string{
			"Content-Security-Policy": wantExplorerPolicy,
			"X-Content-Type-Options":  "nosniff",
			"Referrer-Policy":         "no-referrer",
			"Cache-Control":           "no-store",
		}
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

func explorerBuildScript(t *testing.T, info func() (*debug.BuildInfo, bool)) *httptest.ResponseRecorder {
	t.Helper()
	srv, _ := newTestServerFull(&fakeClient{})
	srv.buildInfo = info
	return explorerGet(t, srv.Handler(), http.MethodGet, "/explorer/build.js")
}

func stampedBuild(settings ...debug.BuildSetting) func() (*debug.BuildInfo, bool) {
	return func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Path: "github.com/davidadel66/evie"}, Settings: settings}, true
	}
}

func TestExplorerBuildScriptNamesTheRepositoryCommitAndModifiedFlag(t *testing.T) {
	cases := []struct {
		name string
		info func() (*debug.BuildInfo, bool)
		want string
	}{
		{
			name: "a clean stamped build",
			info: stampedBuild(debug.BuildSetting{Key: "vcs", Value: "git"},
				debug.BuildSetting{Key: "vcs.revision", Value: "6c9c8b1a0b5f3e2d4c6a8b0e1f2a3b4c5d6e7f80"},
				debug.BuildSetting{Key: "vcs.modified", Value: "false"}),
			want: `evieExplorer.build({"repository":"https://github.com/davidadel66/evie","commit":"6c9c8b1a0b5f3e2d4c6a8b0e1f2a3b4c5d6e7f80","modified":false});` + "\n",
		},
		{
			name: "a build with uncommitted changes",
			info: stampedBuild(debug.BuildSetting{Key: "vcs.revision", Value: "6c9c8b1a0b5f3e2d4c6a8b0e1f2a3b4c5d6e7f80"},
				debug.BuildSetting{Key: "vcs.modified", Value: "true"}),
			want: `evieExplorer.build({"repository":"https://github.com/davidadel66/evie","commit":"6c9c8b1a0b5f3e2d4c6a8b0e1f2a3b4c5d6e7f80","modified":true});` + "\n",
		},
		{
			name: "an unstamped build",
			info: stampedBuild(),
			want: `evieExplorer.build({"repository":"https://github.com/davidadel66/evie","commit":"","modified":false});` + "\n",
		},
		{
			name: "no build information",
			info: func() (*debug.BuildInfo, bool) { return nil, false },
			want: `evieExplorer.build({"repository":"","commit":"","modified":false});` + "\n",
		},
	}
	for _, c := range cases {
		rec := explorerBuildScript(t, c.info)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", c.name, rec.Code)
		}
		if got := rec.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
			t.Errorf("%s: Content-Type = %q", c.name, got)
		}
		if got := rec.Body.String(); got != c.want {
			t.Errorf("%s: build.js =\n%s\nwant\n%s", c.name, got, c.want)
		}
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
	// The vision page is one file with inline scripts and styles, so it gets
	// its own policy: its scripts by hash, never any other inline script.
	h := newTestServer(&fakeClient{})
	rec := explorerGet(t, h, http.MethodGet, "/explorer/system.html?theme=dark")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /explorer/system.html: status = %d, want 200", rec.Code)
	}
	policy := policyDirectives(rec.Header().Get("Content-Security-Policy"))
	page := explorerFile(t, "system.html")
	var want []string
	for _, m := range regexp.MustCompile(`(?s)<script>(.*?)</script>`).FindAllStringSubmatch(page, -1) {
		sum := sha256.Sum256([]byte(m[1]))
		want = append(want, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
	}
	if len(want) == 0 {
		t.Fatal("system.html has no inline script; was it built from the vision template?")
	}
	if got := policy["script-src"]; got != strings.Join(want, " ") {
		t.Errorf("script-src = %q, want the page's inline scripts by hash, %q", got, strings.Join(want, " "))
	}
	want = []string{
		"sandbox", "allow-scripts allow-popups allow-popups-to-escape-sandbox",
		"default-src", "'none'",
		"style-src", "'unsafe-inline'",
		"img-src", "'self' data:",
		"base-uri", "'none'",
		"form-action", "'none'",
		"frame-ancestors", "'self'",
	}
	for i := 0; i < len(want); i += 2 {
		if got := policy[want[i]]; got != want[i+1] {
			t.Errorf("%s = %q, want %q", want[i], got, want[i+1])
		}
	}
	// default-src 'none' covers fonts and connections: the page reaches no
	// host but Evie, so no directive may name one.
	for _, name := range []string{"connect-src", "font-src", "frame-src"} {
		if _, ok := policy[name]; ok {
			t.Errorf("the map page may load %s: %q", name, rec.Header().Get("Content-Security-Policy"))
		}
	}
	if header := rec.Header().Get("Content-Security-Policy"); strings.Contains(header, "http") {
		t.Errorf("the map page's policy names another host: %q", header)
	}
	// The page Evie embeds asks for no web fonts, so the policy blocks nothing
	// it loads.
	for _, host := range []string{"fonts.googleapis.com", "fonts.gstatic.com"} {
		if strings.Contains(page, host) {
			t.Errorf("system.html loads from %s; build it to meta.page so it uses the system fonts", host)
		}
	}
	for name, value := range map[string]string{"X-Content-Type-Options": "nosniff", "Referrer-Policy": "no-referrer", "Cache-Control": "no-store"} {
		if got := rec.Header().Get(name); got != value {
			t.Errorf("%s = %q, want %q", name, got, value)
		}
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
	if got := policyDirectives(systemMapPolicy(nil))["script-src"]; got != "'none'" {
		t.Errorf("with no page, script-src = %q, want 'none'", got)
	}
}

func TestExplorerFilesOtherThanTheMapKeepTheExplorerPolicy(t *testing.T) {
	h := newTestServer(&fakeClient{})
	for _, target := range []string{"/explorer/", "/explorer/index.html", "/explorer/data/facts.js", "/explorer/system.html/x"} {
		rec := explorerGet(t, h, http.MethodGet, target)
		if got := rec.Header().Get("Content-Security-Policy"); got != wantExplorerPolicy {
			t.Errorf("GET %s: policy %q, want %q", target, got, wantExplorerPolicy)
		}
	}
}
