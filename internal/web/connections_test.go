package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/mcpconn"
)

// fakeConnections records route calls and answers from scripted values.
type fakeConnections struct {
	added       []mcpconn.AddRequest
	callbacks   []url.Values
	callbackErr error
	connectErr  error
	waitErr     error
}

func (f *fakeConnections) List(context.Context) ([]mcpconn.Connection, error) {
	return []mcpconn.Connection{{ID: "mcp-square", Name: "square", URL: "https://mcp.squareup.com/mcp", Status: mcpconn.StatusConnected}}, nil
}

func (f *fakeConnections) Add(_ context.Context, req mcpconn.AddRequest) (mcpconn.Connection, error) {
	if req.Name == "taken" {
		return mcpconn.Connection{}, mcpconn.ErrConnectionExists
	}
	if req.Name == "Bad" {
		return mcpconn.Connection{}, fmt.Errorf("%w: bad name", mcpconn.ErrInvalid)
	}
	f.added = append(f.added, req)
	return mcpconn.Connection{ID: "mcp-" + req.Name, Name: req.Name, URL: req.URL, Status: mcpconn.StatusNotConnected}, nil
}

func (f *fakeConnections) BeginConnect(_ context.Context, name string) (mcpconn.Attempt, error) {
	if f.connectErr != nil {
		return mcpconn.Attempt{}, f.connectErr
	}
	return mcpconn.Attempt{ID: "state-1", ConnectionID: "mcp-" + name, AuthorizationURL: "https://mcp.squareup.com/authorize?state=state-1"}, nil
}

func (f *fakeConnections) Wait(_ context.Context, id string) (mcpconn.Connection, error) {
	if f.waitErr != nil {
		return mcpconn.Connection{}, f.waitErr
	}
	return mcpconn.Connection{ID: "mcp-square", Name: "square", Status: mcpconn.StatusConnected}, nil
}

func (f *fakeConnections) Callback(_ context.Context, query url.Values) (mcpconn.Connection, error) {
	f.callbacks = append(f.callbacks, query)
	if f.callbackErr != nil {
		return mcpconn.Connection{}, f.callbackErr
	}
	return mcpconn.Connection{ID: "mcp-square", Name: "square", Status: mcpconn.StatusConnected}, nil
}

func (f *fakeConnections) Disconnect(_ context.Context, name string) (mcpconn.Connection, error) {
	if name != "square" {
		return mcpconn.Connection{}, mcpconn.ErrConnectionNotFound
	}
	return mcpconn.Connection{ID: "mcp-square", Name: "square", Status: mcpconn.StatusNotConnected}, nil
}

func TestConnectionManagementRoutes(t *testing.T) {
	fake := &fakeConnections{}
	handler := WithMCPConnections(NewServer(nil), fake).Handler()
	call := func(route, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, managementRequest(route, body))
		return rec
	}
	if rec := call("/api/connections/list", `{}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"square"`) {
		t.Fatalf("list = %d %s", rec.Code, rec.Body)
	}
	rec := call("/api/connections/add", `{"name":"canva","url":"https://mcp.canva.com/mcp","client_id":"portal","client_secret":"shh"}`)
	if rec.Code != http.StatusOK || len(fake.added) != 1 || fake.added[0].ClientSecret != "shh" || strings.Contains(rec.Body.String(), "shh") {
		t.Fatalf("add = %d %s, added %+v", rec.Code, rec.Body, fake.added)
	}
	if rec := call("/api/connections/add", `{"name":"taken","url":"https://x.example/mcp"}`); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate add = %d %s", rec.Code, rec.Body)
	}
	if rec := call("/api/connections/add", `{"name":"Bad","url":"https://x.example/mcp"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid add = %d %s", rec.Code, rec.Body)
	}
	rec = call("/api/connections/connect", `{"name":"square"}`)
	var attempt mcpconn.Attempt
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &attempt) != nil || attempt.ID != "state-1" || attempt.AuthorizationURL == "" {
		t.Fatalf("connect = %d %s", rec.Code, rec.Body)
	}
	if rec := call("/api/connections/wait", `{"attempt_id":"state-1"}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"connected"`) {
		t.Fatalf("wait = %d %s", rec.Code, rec.Body)
	}
	fake.waitErr = mcpconn.ErrUnknownAttempt
	if rec := call("/api/connections/wait", `{"attempt_id":"gone"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("wait on an unknown attempt = %d %s", rec.Code, rec.Body)
	}
	fake.connectErr = fmt.Errorf("the server does not offer client registration")
	if rec := call("/api/connections/connect", `{"name":"square"}`); rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "client registration") {
		t.Fatalf("failed connect = %d %s", rec.Code, rec.Body)
	}
	if rec := call("/api/connections/disconnect", `{"name":"square"}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "not_connected") {
		t.Fatalf("disconnect = %d %s", rec.Code, rec.Body)
	}
	if rec := call("/api/connections/disconnect", `{"name":"absent"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("disconnect of a missing connection = %d %s", rec.Code, rec.Body)
	}
	// The management routes keep the cross-origin guard.
	cross := managementRequest("/api/connections/connect", `{"name":"square"}`)
	cross.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, cross)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-origin connect = %d", rec.Code)
	}
}

func TestOAuthCallbackAcceptsTheBrowserRedirectAndEscapesErrors(t *testing.T) {
	fake := &fakeConnections{}
	handler := WithMCPConnections(NewServer(nil), fake).Handler()
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:6687/api/connections/oauth/callback?code=c1&state=s1", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Connected to square") || len(fake.callbacks) != 1 || fake.callbacks[0].Get("state") != "s1" {
		t.Fatalf("callback = %d %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("Content-Type") != "text/html; charset=utf-8" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("callback headers = %v", rec.Header())
	}

	fake.callbackErr = fmt.Errorf("sign-in was not completed: <script>alert(1)</script>")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:6687/api/connections/oauth/callback?error=x&state=s2", nil))
	if rec.Code != http.StatusBadRequest || strings.Contains(rec.Body.String(), "<script>") || !strings.Contains(rec.Body.String(), "&lt;script&gt;") {
		t.Fatalf("failed callback = %d %s", rec.Code, rec.Body)
	}

	post := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:6687/api/connections/oauth/callback", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, post)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST callback = %d", rec.Code)
	}
}
