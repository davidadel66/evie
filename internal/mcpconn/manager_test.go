package mcpconn

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/mcpconn/mcptest"
)

const testRedirect = "http://127.0.0.1:6687/api/connections/oauth/callback"

type managerFixture struct {
	t       *testing.T
	fake    *mcptest.AuthServer
	records *memoryRecords
	secrets *memorySecrets
	now     time.Time
	nowMu   sync.Mutex
	m       *Manager
}

func newManagerFixture(t *testing.T) *managerFixture {
	f := &managerFixture{t: t, fake: mcptest.NewAuthServer(t), records: &memoryRecords{}, secrets: &memorySecrets{},
		now: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}
	f.m = f.manager()
	return f
}

// manager is a Manager over the fixture's records and secrets, as a
// restarted serve would build it.
func (f *managerFixture) manager() *Manager {
	return NewManager(Config{Records: f.records, Secrets: f.secrets, HTTPClient: f.fake.Server.Client(), RedirectURL: testRedirect, Now: f.clock})
}

func (f *managerFixture) clock() time.Time {
	f.nowMu.Lock()
	defer f.nowMu.Unlock()
	return f.now
}

func (f *managerFixture) advance(d time.Duration) {
	f.nowMu.Lock()
	defer f.nowMu.Unlock()
	f.now = f.now.Add(d)
}

func (f *managerFixture) add(name string) {
	f.t.Helper()
	if _, err := f.m.Add(context.Background(), AddRequest{Name: name, URL: f.fake.Resource()}); err != nil {
		f.t.Fatal(err)
	}
}

func (f *managerFixture) connect(name string) Connection {
	f.t.Helper()
	ctx := context.Background()
	attempt, err := f.m.BeginConnect(ctx, name)
	if err != nil {
		f.t.Fatal(err)
	}
	connection, err := f.m.Callback(ctx, f.fake.Approve(f.t, attempt.AuthorizationURL))
	if err != nil {
		f.t.Fatal(err)
	}
	return connection
}

func (f *managerFixture) status(id string) Status {
	c, _ := f.records.MCPConnection(context.Background(), id)
	return c.Status
}

func TestConnectSignsInWithDynamicRegistrationAndPKCE(t *testing.T) {
	ctx := context.Background()
	f := newManagerFixture(t)
	added, err := f.m.Add(ctx, AddRequest{Name: "square", URL: f.fake.Resource()})
	if err != nil {
		t.Fatal(err)
	}
	if added.ID != "mcp-square" || added.Status != StatusNotConnected {
		t.Fatalf("Add() = %+v", added)
	}
	attempt, err := f.m.BeginConnect(ctx, "square")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(attempt.AuthorizationURL)
	if u.Query().Get("redirect_uri") != testRedirect || u.Query().Get("client_id") != "dcr-1" {
		t.Fatalf("authorization URL = %q", attempt.AuthorizationURL)
	}
	if !attempt.ExpiresAt.Equal(f.clock().Add(10 * time.Minute)) {
		t.Fatalf("attempt expires %v", attempt.ExpiresAt)
	}

	waited := make(chan Connection, 1)
	go func() {
		c, _ := f.m.Wait(ctx, attempt.ID)
		waited <- c
	}()
	connection, err := f.m.Callback(ctx, f.fake.Approve(t, attempt.AuthorizationURL))
	if err != nil {
		t.Fatal(err)
	}
	if connection.Status != StatusConnected || connection.OAuth.TokenEndpoint != f.fake.Server.URL+"/token" ||
		connection.OAuth.Resource != f.fake.Resource() || connection.OAuth.RegisteredRedirect != testRedirect {
		t.Fatalf("Callback() = %+v", connection)
	}
	select {
	case c := <-waited:
		if c.Status != StatusConnected {
			t.Fatalf("Wait() = %+v", c)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Wait() did not return after the callback")
	}
	stored, ok := f.secrets.get("mcp-square")
	if !ok || stored.AccessToken != "access-1" || stored.RefreshToken != "refresh-1" || stored.ClientID != "dcr-1" ||
		!stored.Expiry.Equal(f.clock().Add(time.Hour)) {
		t.Fatalf("Keychain holds %+v", stored)
	}
	if forms := f.fake.Snapshot().TokenForms; forms[0].Get("client_id") != "dcr-1" || forms[0].Get("basic_client_id") != "" || forms[0].Get("client_secret") != "" {
		t.Fatalf("a public client authenticates with client_id only: %v", forms[0])
	}
}

func TestConnectUsesAPreregisteredClientWithBasicAuth(t *testing.T) {
	ctx := context.Background()
	f := newManagerFixture(t)
	f.fake.SetNoRegistration(true)
	if _, err := f.m.Add(ctx, AddRequest{Name: "canva", URL: f.fake.Resource(), ClientID: "portal-client", ClientSecret: "portal-secret"}); err != nil {
		t.Fatal(err)
	}
	if stored, _ := f.secrets.get("mcp-canva"); stored.ClientSecret != "portal-secret" {
		t.Fatal("Add() did not keep the client secret in the Keychain")
	}
	f.connect("canva")
	snapshot := f.fake.Snapshot()
	if snapshot.Registrations != 0 {
		t.Fatal("a pre-registered client was registered again")
	}
	if form := snapshot.TokenForms[0]; form.Get("basic_client_id") != "portal-client" || form.Get("basic_client_secret") != "portal-secret" || form.Get("client_secret") != "" {
		t.Fatalf("token request did not use HTTP Basic: %v", form)
	}
}

func TestClientAuthenticationFollowsTheServersMethods(t *testing.T) {
	cases := []struct {
		methods []string
		basic   bool
	}{
		{nil, true}, // RFC 8414 default
		{[]string{"client_secret_post"}, false},
		{[]string{"client_secret_post", "client_secret_basic"}, true},
		{[]string{"none"}, true},
	}
	for i, c := range cases {
		fake := mcptest.NewAuthServer(t)
		meta := OAuthMetadata{TokenEndpoint: fake.Server.URL + "/token", TokenAuthMethods: c.methods, Resource: fake.Resource()}
		_, _ = requestToken(context.Background(), fake.Server.Client(), meta, Credentials{ClientID: "c", ClientSecret: "s"},
			url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"r"}})
		form := fake.Snapshot().TokenForms[0]
		basic, posted := form.Get("basic_client_secret") == "s", form.Get("client_secret") == "s"
		if basic != c.basic || posted == c.basic {
			t.Fatalf("case %d, methods %v: basic=%v posted=%v", i, c.methods, basic, posted)
		}
	}
}

func TestReconnectAfterDisconnectKeepsAPreregisteredClientSecret(t *testing.T) {
	ctx := context.Background()
	f := newManagerFixture(t)
	f.fake.SetNoRegistration(true)
	if _, err := f.m.Add(ctx, AddRequest{Name: "canva", URL: f.fake.Resource(), ClientID: "portal-client", ClientSecret: "portal-secret"}); err != nil {
		t.Fatal(err)
	}
	f.connect("canva")
	if _, err := f.m.Disconnect(ctx, "canva"); err != nil {
		t.Fatal(err)
	}
	stored, _ := f.secrets.get("mcp-canva")
	if stored.AccessToken != "" || stored.RefreshToken != "" || stored.ClientSecret != "portal-secret" {
		t.Fatalf("after Disconnect the Keychain holds %+v", stored)
	}
	f.connect("canva")
	forms := f.fake.Snapshot().TokenForms
	if last := forms[len(forms)-1]; last.Get("basic_client_secret") != "portal-secret" {
		t.Fatalf("the reconnect did not authenticate the client: %v", last)
	}
	if f.status("mcp-canva") != StatusConnected {
		t.Fatal("the reconnect did not connect")
	}
}

func TestReconnectReusesTheDynamicRegistration(t *testing.T) {
	ctx := context.Background()
	f := newManagerFixture(t)
	f.add("square")
	f.connect("square")
	f.fake.SetRefreshFails(true)
	f.advance(time.Hour)
	if _, err := f.m.TokenSource("mcp-square").Token(ctx); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("Token() error = %v", err)
	}
	f.fake.SetRefreshFails(false)
	f.connect("square")
	if _, err := f.m.Disconnect(ctx, "square"); err != nil {
		t.Fatal(err)
	}
	f.connect("square")
	if n := f.fake.Snapshot().Registrations; n != 1 {
		t.Fatalf("registered %d clients, want the first one reused", n)
	}
	// A changed callback address registers again.
	moved := NewManager(Config{Records: f.records, Secrets: f.secrets, HTTPClient: f.fake.Server.Client(),
		RedirectURL: "http://127.0.0.1:7000/api/connections/oauth/callback", Now: f.clock})
	attempt, err := moved.BeginConnect(ctx, "square")
	if err != nil {
		t.Fatal(err)
	}
	if u, _ := url.Parse(attempt.AuthorizationURL); u.Query().Get("client_id") != "dcr-2" {
		t.Fatalf("a new callback address reused client %q", u.Query().Get("client_id"))
	}
}

func TestConnectWithoutRegistrationOrClientIDFails(t *testing.T) {
	f := newManagerFixture(t)
	f.fake.SetNoRegistration(true)
	f.add("canva")
	if _, err := f.m.BeginConnect(context.Background(), "canva"); err == nil || !strings.Contains(err.Error(), "client ID") {
		t.Fatalf("BeginConnect() error = %v, want a pre-registered client ID request", err)
	}
}

func TestCallbackRejectsUnknownReplayedAndExpiredState(t *testing.T) {
	ctx := context.Background()
	f := newManagerFixture(t)
	f.add("square")
	attempt, err := f.m.BeginConnect(ctx, "square")
	if err != nil {
		t.Fatal(err)
	}
	query := f.fake.Approve(t, attempt.AuthorizationURL)
	forged := url.Values{"code": query["code"], "state": {"not-the-state"}}
	if _, err := f.m.Callback(ctx, forged); !errors.Is(err, ErrUnknownAttempt) {
		t.Fatalf("forged state: %v", err)
	}
	if _, err := f.m.Callback(ctx, query); err != nil {
		t.Fatal(err)
	}
	if _, err := f.m.Callback(ctx, query); !errors.Is(err, ErrUnknownAttempt) {
		t.Fatalf("replayed state: %v", err)
	}

	late, err := f.m.BeginConnect(ctx, "square")
	if err != nil {
		t.Fatal(err)
	}
	lateQuery := f.fake.Approve(t, late.AuthorizationURL)
	f.advance(10*time.Minute + time.Second)
	if _, err := f.m.Callback(ctx, lateQuery); !errors.Is(err, ErrUnknownAttempt) {
		t.Fatalf("expired state: %v", err)
	}
	if _, err := f.m.Wait(ctx, late.ID); !errors.Is(err, ErrUnknownAttempt) {
		t.Fatalf("Wait() on an expired attempt: %v", err)
	}
}

func TestWaitReportsACompletedSignInEvenAfterTheDeadline(t *testing.T) {
	ctx := context.Background()
	f := newManagerFixture(t)
	f.add("square")
	attempt, err := f.m.BeginConnect(ctx, "square")
	if err != nil {
		t.Fatal(err)
	}
	f.advance(9*time.Minute + 59*time.Second)
	if _, err := f.m.Callback(ctx, f.fake.Approve(t, attempt.AuthorizationURL)); err != nil {
		t.Fatal(err)
	}
	f.advance(time.Minute)
	for range 20 {
		connection, err := f.m.Wait(ctx, attempt.ID)
		if err != nil || connection.Status != StatusConnected {
			t.Fatalf("Wait() after a successful late callback = %+v, %v", connection, err)
		}
	}
}

func TestCallbackWithAnAuthorizationErrorLeavesTheConnectionUnchanged(t *testing.T) {
	ctx := context.Background()
	f := newManagerFixture(t)
	f.add("square")
	attempt, err := f.m.BeginConnect(ctx, "square")
	if err != nil {
		t.Fatal(err)
	}
	state := f.fake.Approve(t, attempt.AuthorizationURL).Get("state")
	_, err = f.m.Callback(ctx, url.Values{"state": {state}, "error": {"access_denied"}})
	if err == nil || !strings.Contains(err.Error(), "access_denied") {
		t.Fatalf("Callback() error = %v", err)
	}
	if _, err := f.m.Wait(ctx, attempt.ID); err == nil || !strings.Contains(err.Error(), "access_denied") {
		t.Fatalf("Wait() error = %v", err)
	}
	if f.status("mcp-square") != StatusNotConnected {
		t.Fatalf("status = %s", f.status("mcp-square"))
	}
}

func TestTokenRefreshesOnceForConcurrentCallersAndRotates(t *testing.T) {
	ctx := context.Background()
	f := newManagerFixture(t)
	f.add("square")
	f.connect("square")
	tokens := f.m.TokenSource("mcp-square")
	if token, err := tokens.Token(ctx); err != nil || token != "access-1" {
		t.Fatalf("Token() = %q, %v", token, err)
	}
	f.advance(time.Hour)
	var wg sync.WaitGroup
	got := make([]string, 8)
	for i := range got {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got[i], _ = tokens.Token(ctx)
		}()
	}
	wg.Wait()
	for _, token := range got {
		if token != "access-2" {
			t.Fatalf("concurrent Token() = %v", got)
		}
	}
	snapshot := f.fake.Snapshot()
	if snapshot.Refreshes != 1 {
		t.Fatalf("refreshed %d times, want once", snapshot.Refreshes)
	}
	if form := snapshot.RefreshForms[0]; form.Get("resource") != f.fake.Resource() || form.Get("refresh_token") != "refresh-1" {
		t.Fatalf("refresh form = %v", form)
	}
	if stored, _ := f.secrets.get("mcp-square"); stored.RefreshToken != "refresh-2" || stored.AccessToken != "access-2" {
		t.Fatalf("rotated tokens were not saved: %+v", stored)
	}
}

func TestRefreshSurvivesACancelledTurn(t *testing.T) {
	f := newManagerFixture(t)
	f.add("square")
	f.connect("square")
	f.advance(time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	f.fake.SetAfterRotate(cancel) // the turn ends after Square rotated
	_, _ = f.m.TokenSource("mcp-square").Token(ctx)
	if stored, _ := f.secrets.get("mcp-square"); stored.RefreshToken != "refresh-2" {
		t.Fatalf("the rotated refresh token was lost: Keychain holds %+v", stored)
	}
	f.fake.SetAfterRotate(nil)
	f.advance(time.Hour)
	if token, err := f.m.TokenSource("mcp-square").Token(context.Background()); err != nil || token != "access-3" {
		t.Fatalf("the next refresh = %q, %v", token, err)
	}
}

func TestAnUnsavedRotationIsKeptAndSavedLater(t *testing.T) {
	ctx := context.Background()
	f := newManagerFixture(t)
	f.add("square")
	f.connect("square")
	f.advance(time.Hour)
	f.secrets.setFail(ErrKeychainUnavailable)
	if _, err := f.m.TokenSource("mcp-square").Token(ctx); !errors.Is(err, ErrKeychainUnavailable) {
		t.Fatalf("Token() with a refusing Keychain = %v", err)
	}
	if f.status("mcp-square") != StatusKeychainUnavailable {
		t.Fatalf("status = %s", f.status("mcp-square"))
	}
	f.secrets.setFail(nil)
	if token, err := f.m.TokenSource("mcp-square").Token(ctx); err != nil || token != "access-2" {
		t.Fatalf("Token() once the Keychain returns = %q, %v", token, err)
	}
	if n := f.fake.Snapshot().Refreshes; n != 1 {
		t.Fatalf("refreshed %d times; the rotation should have been kept", n)
	}
	if stored, _ := f.secrets.get("mcp-square"); stored.RefreshToken != "refresh-2" {
		t.Fatalf("Keychain holds %+v", stored)
	}
	if f.status("mcp-square") != StatusConnected {
		t.Fatalf("status = %s after the save", f.status("mcp-square"))
	}
}

func TestStatusRecoversWhenTheKeychainReturns(t *testing.T) {
	ctx := context.Background()
	f := newManagerFixture(t)
	f.add("square")
	f.connect("square")
	restarted := f.manager()
	f.secrets.setFail(ErrKeychainUnavailable)
	if _, err := restarted.TokenSource("mcp-square").Token(ctx); !errors.Is(err, ErrKeychainUnavailable) {
		t.Fatalf("Token() = %v", err)
	}
	if f.status("mcp-square") != StatusKeychainUnavailable {
		t.Fatalf("status = %s", f.status("mcp-square"))
	}
	f.secrets.setFail(nil)
	if token, err := restarted.TokenSource("mcp-square").Token(ctx); err != nil || token != "access-1" {
		t.Fatalf("Token() = %q, %v", token, err)
	}
	if f.status("mcp-square") != StatusConnected {
		t.Fatalf("status stayed %s", f.status("mcp-square"))
	}
}

func TestRefreshAfterRejectionSkipsWhenAnotherCallerAlreadyRefreshed(t *testing.T) {
	ctx := context.Background()
	f := newManagerFixture(t)
	f.add("square")
	f.connect("square")
	tokens := f.m.TokenSource("mcp-square")
	first, err := tokens.Refresh(ctx, "access-1")
	if err != nil || first != "access-2" {
		t.Fatalf("Refresh(access-1) = %q, %v", first, err)
	}
	second, err := tokens.Refresh(ctx, "access-1")
	if err != nil || second != "access-2" {
		t.Fatalf("second Refresh(access-1) = %q, %v", second, err)
	}
	if f.fake.Snapshot().Refreshes != 1 {
		t.Fatal("a stale rejection refreshed again")
	}
}

func TestRefreshRefusalNeedsSignIn(t *testing.T) {
	ctx := context.Background()
	f := newManagerFixture(t)
	f.add("square")
	f.connect("square")
	f.fake.SetRefreshFails(true)
	f.advance(time.Hour)
	if _, err := f.m.TokenSource("mcp-square").Token(ctx); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("Token() error = %v, want ErrNotConnected", err)
	}
	if f.status("mcp-square") != StatusNeedsSignIn {
		t.Fatalf("status = %s, want needs_sign_in", f.status("mcp-square"))
	}
	if _, err := f.m.TokenSource("mcp-square").Token(ctx); !errors.Is(err, ErrNotConnected) {
		t.Fatal("a connection that needs sign-in kept refreshing")
	}
	if f.fake.Snapshot().Refreshes != 1 {
		t.Fatal("a refused refresh was retried")
	}
}

func TestANonOAuthTokenErrorKeepsTheRefreshToken(t *testing.T) {
	ctx := context.Background()
	f := newManagerFixture(t)
	f.add("square")
	f.connect("square")
	f.advance(time.Hour)
	for _, answer := range []struct {
		status int
		body   string
	}{{400, "<html>blocked by the firewall</html>"}, {400, `{"error":"invalid_request"}`}, {503, "busy"}} {
		f.fake.SetRefreshAnswer(answer.status, answer.body)
		if _, err := f.m.TokenSource("mcp-square").Token(ctx); err == nil || errors.Is(err, ErrNotConnected) {
			t.Fatalf("%d %q: Token() error = %v", answer.status, answer.body, err)
		}
		if f.status("mcp-square") != StatusUnreachable {
			t.Fatalf("%d %q: status = %s", answer.status, answer.body, f.status("mcp-square"))
		}
		if stored, _ := f.secrets.get("mcp-square"); stored.RefreshToken != "refresh-1" {
			t.Fatalf("%d %q: the refresh token was dropped", answer.status, answer.body)
		}
	}
	f.fake.SetRefreshAnswer(0, "")
	if token, err := f.m.TokenSource("mcp-square").Token(ctx); err != nil || token != "access-2" {
		t.Fatalf("Token() once the server recovers = %q, %v", token, err)
	}
	if f.status("mcp-square") != StatusConnected {
		t.Fatalf("status = %s", f.status("mcp-square"))
	}
}

func TestDisconnectRevokesAndForgetsTheTokens(t *testing.T) {
	ctx := context.Background()
	f := newManagerFixture(t)
	f.add("square")
	f.connect("square")
	connection, err := f.m.Disconnect(ctx, "square")
	if err != nil {
		t.Fatal(err)
	}
	if connection.Status != StatusNotConnected {
		t.Fatalf("status = %s", connection.Status)
	}
	if stored, _ := f.secrets.get("mcp-square"); stored.AccessToken != "" || stored.RefreshToken != "" || stored.ClientID != "dcr-1" {
		t.Fatalf("after Disconnect the Keychain holds %+v", stored)
	}
	if revoked := f.fake.Snapshot().Revoked; len(revoked) != 1 || revoked[0] != "refresh-1" {
		t.Fatalf("revoked = %v", revoked)
	}
	if _, err := f.m.TokenSource("mcp-square").Token(ctx); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("Token() after Disconnect = %v", err)
	}
}

func TestKeychainFailureMarksTheConnection(t *testing.T) {
	ctx := context.Background()
	f := newManagerFixture(t)
	f.add("square")
	attempt, err := f.m.BeginConnect(ctx, "square")
	if err != nil {
		t.Fatal(err)
	}
	f.secrets.setFail(ErrKeychainUnavailable)
	if _, err := f.m.Callback(ctx, f.fake.Approve(t, attempt.AuthorizationURL)); !errors.Is(err, ErrKeychainUnavailable) {
		t.Fatalf("Callback() error = %v", err)
	}
	if f.status("mcp-square") != StatusKeychainUnavailable {
		t.Fatalf("status = %s", f.status("mcp-square"))
	}
}

// leakCheckingRecords serializes every write the Manager sends toward
// SQLite, so a test can prove no credential ever reaches the database.
type leakCheckingRecords struct {
	*memoryRecords
	mu     sync.Mutex
	writes []string
}

func (l *leakCheckingRecords) CreateMCPConnection(ctx context.Context, c Connection) error {
	l.record(c)
	return l.memoryRecords.CreateMCPConnection(ctx, c)
}

func (l *leakCheckingRecords) SetMCPConnectionStatus(ctx context.Context, id string, status Status, detail string, oauth *OAuthMetadata, at time.Time) error {
	l.record(id, status, detail, oauth)
	return l.memoryRecords.SetMCPConnectionStatus(ctx, id, status, detail, oauth, at)
}

func (l *leakCheckingRecords) record(values ...any) {
	encoded, _ := json.Marshal(values)
	l.mu.Lock()
	l.writes = append(l.writes, string(encoded))
	l.mu.Unlock()
}

func TestNoCredentialReachesTheRecords(t *testing.T) {
	ctx := context.Background()
	f := newManagerFixture(t)
	records := &leakCheckingRecords{memoryRecords: f.records}
	m := NewManager(Config{Records: records, Secrets: f.secrets, HTTPClient: f.fake.Server.Client(), RedirectURL: testRedirect, Now: f.clock})
	if _, err := m.Add(ctx, AddRequest{Name: "canva", URL: f.fake.Resource(), ClientID: "portal", ClientSecret: "portal-secret"}); err != nil {
		t.Fatal(err)
	}
	attempt, err := m.BeginConnect(ctx, "canva")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Callback(ctx, f.fake.Approve(t, attempt.AuthorizationURL)); err != nil {
		t.Fatal(err)
	}
	f.advance(time.Hour)
	if _, err := m.TokenSource("mcp-canva").Token(ctx); err != nil {
		t.Fatal(err)
	}
	f.fake.SetRefreshFails(true)
	if _, err := m.TokenSource("mcp-canva").Refresh(ctx, "access-2"); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("Refresh() error = %v", err)
	}
	if _, err := m.Disconnect(ctx, "canva"); err != nil {
		t.Fatal(err)
	}
	records.mu.Lock()
	defer records.mu.Unlock()
	if len(records.writes) < 4 {
		t.Fatalf("only %d writes recorded", len(records.writes))
	}
	for _, write := range records.writes {
		for _, secret := range []string{"access-1", "access-2", "refresh-1", "refresh-2", "portal-secret", attempt.ID} {
			if strings.Contains(write, secret) {
				t.Fatalf("a record write carries %q: %s", secret, write)
			}
		}
	}
}

func TestAddValidatesNamesURLsAndDuplicates(t *testing.T) {
	ctx := context.Background()
	f := newManagerFixture(t)
	for _, req := range []AddRequest{
		{Name: "Square", URL: "https://mcp.squareup.com/mcp"},
		{Name: "square", URL: "http://mcp.squareup.com/mcp"},
		{Name: "square", URL: "https://user:pw@mcp.squareup.com/mcp"},
		{Name: "square", URL: "https://mcp.squareup.com/mcp", ClientSecret: "secret-without-id"},
	} {
		if _, err := f.m.Add(ctx, req); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Add(%+v) error = %v, want ErrInvalid", req, err)
		}
	}
	if _, err := f.m.Add(ctx, AddRequest{Name: "square", URL: "https://mcp.squareup.com/mcp"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.m.Add(ctx, AddRequest{Name: "square", URL: "https://mcp.squareup.com/mcp"}); !errors.Is(err, ErrConnectionExists) {
		t.Fatalf("duplicate Add() error = %v", err)
	}
}
