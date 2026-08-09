package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// These tests read the real embedded build, so they fail with an empty dist
// until `npm --prefix internal/web/ui run build` has run at least once —
// the same precondition `go build` has.

func TestStaticServesIndex(t *testing.T) {
	h := newTestServer(&fakeClient{})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "http://127.0.0.1:6687/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("root: status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `id="root"`) {
		t.Fatalf("root did not serve the app shell:\n%s", body)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("index Cache-Control = %q, want no-store", cc)
	}
}

func TestStaticUnknownPathFallsBackToIndex(t *testing.T) {
	h := newTestServer(&fakeClient{})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "http://127.0.0.1:6687/nope/deep", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("unknown path: status = %d, want 200 (SPA fallback)", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `id="root"`) {
		t.Fatalf("fallback did not serve the app shell:\n%s", body)
	}
}

func TestStaticAssetIsImmutable(t *testing.T) {
	h := newTestServer(&fakeClient{})

	name := findAsset(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "http://127.0.0.1:6687/"+name, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("%s: status = %d, want 200", name, rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("%s Cache-Control = %q, want immutable", name, cc)
	}
}

func TestStaticRejectsNonGET(t *testing.T) {
	h := newTestServer(&fakeClient{})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("PUT", "http://127.0.0.1:6687/", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT /: status = %d, want 405", rec.Code)
	}
}

// TestStaticDoesNotShadowAPI guards the route order: the static handler owns
// "/", so an /api path must still reach the guard rather than the SPA
// fallback (which would answer 200 with HTML and hide the API entirely).
func TestStaticDoesNotShadowAPI(t *testing.T) {
	h := newTestServer(&fakeClient{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "http://127.0.0.1:6687/api/chat", strings.NewReader(`{"message":"hi"}`))
	req.Header.Set("Content-Type", "text/plain")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("api POST with bad content type: status = %d, want 403 from the guard", rec.Code)
	}
}

// findAsset picks any file under assets/ from the embedded build, so the test
// doesn't hardcode a content hash that changes on every rebuild.
func findAsset(t *testing.T) string {
	t.Helper()
	entries, err := fs.ReadDir(distFS, "ui/dist/assets")
	if err != nil {
		t.Fatalf("read embedded assets (did you run the npm build?): %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			return "assets/" + e.Name()
		}
	}
	t.Fatal("no files under ui/dist/assets")
	return ""
}
