// Package mcptest provides a fake hosted MCP server and OAuth authorization
// server for tests, shaped like Square's (workspace-mcp-connections story
// 0): the MCP endpoint answers 401 with resource_metadata, Dynamic Client
// Registration issues public clients, PKCE is S256, and every refresh
// rotates the refresh token.
package mcptest

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

type AuthServer struct {
	Server *httptest.Server

	mu             sync.Mutex
	registrations  int
	codes          map[string]code
	access         map[string]bool
	refresh        map[string]bool // live refresh tokens; rotation retires the old one
	refreshes      int
	refreshForms   []url.Values
	tokenForms     []url.Values
	revoked        []string
	issued         int
	noRegistration bool
	refreshFails   bool
	refreshStatus  int    // when non-zero, refresh answers this status with refreshBody
	refreshBody    string // raw body for refreshStatus
	afterRotate    func() // runs after a refresh rotated, before the answer is written
	mcpRequests    []string
}

type code struct {
	clientID, redirect, challenge, resource string
}

// Snapshot is what the server has seen.
type Snapshot struct {
	Registrations int
	Refreshes     int
	RefreshForms  []url.Values
	TokenForms    []url.Values
	Revoked       []string
	// MCPRequests are the Authorization headers seen on the MCP endpoint.
	MCPRequests []string
}

func NewAuthServer(t testing.TB) *AuthServer {
	a := &AuthServer{codes: map[string]code{}, access: map[string]bool{}, refresh: map[string]bool{}}
	a.Server = httptest.NewTLSServer(http.HandlerFunc(a.serve))
	t.Cleanup(a.Server.Close)
	return a
}

// Resource is the MCP endpoint URL, the connection's server URL.
func (a *AuthServer) Resource() string { return a.Server.URL + "/mcp" }

func (a *AuthServer) SetNoRegistration(v bool) { a.mu.Lock(); a.noRegistration = v; a.mu.Unlock() }

// SetRefreshFails makes every refresh answer invalid_grant.
func (a *AuthServer) SetRefreshFails(v bool) { a.mu.Lock(); a.refreshFails = v; a.mu.Unlock() }

// SetRefreshAnswer makes every refresh answer status with a raw body, as a
// proxy or misbehaving server might; status 0 restores normal refreshes.
func (a *AuthServer) SetRefreshAnswer(status int, body string) {
	a.mu.Lock()
	a.refreshStatus, a.refreshBody = status, body
	a.mu.Unlock()
}

// SetAfterRotate runs f after a refresh rotated the tokens and before the
// answer is written, outside the server's lock.
func (a *AuthServer) SetAfterRotate(f func()) { a.mu.Lock(); a.afterRotate = f; a.mu.Unlock() }

func (a *AuthServer) Snapshot() Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	return Snapshot{Registrations: a.registrations, Refreshes: a.refreshes,
		RefreshForms: append([]url.Values(nil), a.refreshForms...), TokenForms: append([]url.Values(nil), a.tokenForms...),
		Revoked: append([]string(nil), a.revoked...), MCPRequests: append([]string(nil), a.mcpRequests...)}
}

// Approve plays the owner signing in: it checks the authorization URL and
// returns the query the browser would carry to Evie's callback.
func (a *AuthServer) Approve(t testing.TB, authorizationURL string) url.Values {
	t.Helper()
	u, err := url.Parse(authorizationURL)
	if err != nil || u.Scheme+"://"+u.Host+u.Path != a.Server.URL+"/authorize" {
		t.Fatalf("authorization URL %q is not the fake's", authorizationURL)
	}
	q := u.Query()
	for _, key := range []string{"client_id", "redirect_uri", "state", "code_challenge", "resource"} {
		if q.Get(key) == "" {
			t.Fatalf("authorization URL lacks %s: %q", key, authorizationURL)
		}
	}
	if q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" || q.Get("resource") != a.Resource() {
		t.Fatalf("authorization URL parameters = %v", q)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	value := fmt.Sprintf("code-%d-%d", len(a.codes), a.issued)
	a.codes[value] = code{clientID: q.Get("client_id"), redirect: q.Get("redirect_uri"), challenge: q.Get("code_challenge"), resource: q.Get("resource")}
	return url.Values{"code": {value}, "state": {q.Get("state")}}
}

func (a *AuthServer) serve(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	locked := true
	defer func() {
		if locked {
			a.mu.Unlock()
		}
	}()
	switch {
	case r.URL.Path == "/mcp":
		a.mcpRequests = append(a.mcpRequests, r.Header.Get("Authorization"))
		if !a.access[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")] {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="OAuth", resource_metadata="%s/.well-known/oauth-protected-resource/mcp", error="invalid_token"`, a.Server.URL))
			http.Error(w, `{"error":"invalid_token"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{}}`)
	case r.URL.Path == "/.well-known/oauth-protected-resource/mcp":
		writeJSON(w, map[string]any{"resource": a.Resource(), "authorization_servers": []string{a.Server.URL}})
	case r.URL.Path == "/.well-known/oauth-authorization-server":
		meta := map[string]any{
			"issuer":                                a.Server.URL,
			"authorization_endpoint":                a.Server.URL + "/authorize",
			"token_endpoint":                        a.Server.URL + "/token",
			"revocation_endpoint":                   a.Server.URL + "/revoke",
			"code_challenge_methods_supported":      []string{"plain", "S256"},
			"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post", "none"},
		}
		if !a.noRegistration {
			meta["registration_endpoint"] = a.Server.URL + "/register"
		}
		writeJSON(w, meta)
	case r.URL.Path == "/register" && r.Method == http.MethodPost:
		var body struct {
			RedirectURIs []string `json:"redirect_uris"`
			AuthMethod   string   `json:"token_endpoint_auth_method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body.RedirectURIs) != 1 || body.AuthMethod != "none" {
			http.Error(w, `{"error":"invalid_client_metadata"}`, http.StatusBadRequest)
			return
		}
		a.registrations++
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]any{"client_id": fmt.Sprintf("dcr-%d", a.registrations)})
	case r.URL.Path == "/token" && r.Method == http.MethodPost:
		_ = r.ParseForm()
		form := r.PostForm
		if user, pass, ok := r.BasicAuth(); ok {
			form.Set("basic_client_id", user)
			form.Set("basic_client_secret", pass)
		}
		a.tokenForms = append(a.tokenForms, form)
		switch form.Get("grant_type") {
		case "authorization_code":
			c, ok := a.codes[form.Get("code")]
			delete(a.codes, form.Get("code"))
			sum := sha256.Sum256([]byte(form.Get("code_verifier")))
			client := form.Get("client_id")
			if client == "" {
				client = form.Get("basic_client_id")
			}
			if !ok || c.clientID != client || c.redirect != form.Get("redirect_uri") ||
				c.challenge != base64.RawURLEncoding.EncodeToString(sum[:]) || form.Get("resource") != a.Resource() {
				w.WriteHeader(http.StatusBadRequest)
				writeJSON(w, map[string]string{"error": "invalid_grant"})
				return
			}
			writeJSON(w, a.issueLocked())
		case "refresh_token":
			a.refreshes++
			a.refreshForms = append(a.refreshForms, form)
			if a.refreshStatus != 0 {
				w.WriteHeader(a.refreshStatus)
				fmt.Fprint(w, a.refreshBody)
				return
			}
			if a.refreshFails || !a.refresh[form.Get("refresh_token")] {
				w.WriteHeader(http.StatusBadRequest)
				writeJSON(w, map[string]string{"error": "invalid_grant"})
				return
			}
			delete(a.refresh, form.Get("refresh_token"))
			answer, after := a.issueLocked(), a.afterRotate
			a.mu.Unlock()
			locked = false
			if after != nil {
				after()
			}
			writeJSON(w, answer)
		default:
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(w, map[string]string{"error": "unsupported_grant_type"})
		}
	case r.URL.Path == "/revoke" && r.Method == http.MethodPost:
		_ = r.ParseForm()
		a.revoked = append(a.revoked, r.PostForm.Get("token"))
		delete(a.refresh, r.PostForm.Get("token"))
	default:
		http.NotFound(w, r)
	}
}

func (a *AuthServer) issueLocked() map[string]any {
	a.issued++
	access, refresh := fmt.Sprintf("access-%d", a.issued), fmt.Sprintf("refresh-%d", a.issued)
	a.access[access], a.refresh[refresh] = true, true
	return map[string]any{"access_token": access, "refresh_token": refresh, "token_type": "bearer", "expires_in": 3600}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
