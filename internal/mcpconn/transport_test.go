package mcpconn

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type scriptedTokens struct {
	mu        sync.Mutex
	token     string
	next      string
	refreshes int
	err       error
}

func (s *scriptedTokens) Token(context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token, s.err
}

func (s *scriptedTokens) Refresh(_ context.Context, rejected string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshes++
	if s.next == "" {
		return "", ErrNotConnected
	}
	if rejected == s.token {
		s.token = s.next
	}
	return s.token, nil
}

type seenRequest struct {
	auth, body string
}

type recorder struct {
	mu   sync.Mutex
	seen []seenRequest
}

func (r *recorder) requests() []seenRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]seenRequest(nil), r.seen...)
}

// tlsServer answers with respond(authorization) and records each request.
func tlsServer(t *testing.T, respond func(w http.ResponseWriter, auth string)) (*httptest.Server, *recorder) {
	rec := &recorder{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rec.mu.Lock()
		rec.seen = append(rec.seen, seenRequest{auth: r.Header.Get("Authorization"), body: string(body)})
		rec.mu.Unlock()
		respond(w, r.Header.Get("Authorization"))
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func status(code int) func(http.ResponseWriter, string) {
	return func(w http.ResponseWriter, _ string) { w.WriteHeader(code) }
}

func hostOf(rawURL string) string {
	return strings.TrimPrefix(strings.TrimPrefix(rawURL, "https://"), "http://")
}

func connectionClient(srv *httptest.Server, tokens TokenSource) *http.Client {
	return NewHTTPClient(srv.Client().Transport, hostOf(srv.URL), tokens)
}

func post(t *testing.T, client *http.Client, ctx context.Context, target, body string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if resp != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	return resp, err
}

func TestTransportReSendsOnceAfterA401AndARefresh(t *testing.T) {
	srv, rec := tlsServer(t, func(w http.ResponseWriter, auth string) {
		if auth == "Bearer new" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	})
	tokens := &scriptedTokens{token: "old", next: "new"}
	resp, err := post(t, connectionClient(srv, tokens), context.Background(), srv.URL+"/mcp", `{"method":"tools/call"}`)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("response = %v, %v", resp, err)
	}
	seen := rec.requests()
	if len(seen) != 2 || seen[0].auth != "Bearer old" || seen[1].auth != "Bearer new" ||
		seen[0].body != `{"method":"tools/call"}` || seen[1].body != seen[0].body {
		t.Fatalf("server saw %+v", seen)
	}
	if tokens.refreshes != 1 {
		t.Fatalf("refreshed %d times", tokens.refreshes)
	}
}

func TestTransportSendsAtMostTwiceWhenTheRefreshedTokenIsAlsoRejected(t *testing.T) {
	srv, rec := tlsServer(t, status(http.StatusUnauthorized))
	resp, err := post(t, connectionClient(srv, &scriptedTokens{token: "old", next: "new"}), context.Background(), srv.URL+"/mcp", "{}")
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("response = %v, %v", resp, err)
	}
	if n := len(rec.requests()); n != 2 {
		t.Fatalf("server saw %d requests, want 2", n)
	}
}

func TestTransportDoesNotReSendWhenRefreshFails(t *testing.T) {
	srv, rec := tlsServer(t, status(http.StatusUnauthorized))
	resp, err := post(t, connectionClient(srv, &scriptedTokens{token: "old"}), context.Background(), srv.URL+"/mcp", "{}")
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("response = %v, %v", resp, err)
	}
	if n := len(rec.requests()); n != 1 {
		t.Fatalf("server saw %d requests, want 1", n)
	}
}

func TestTransportNeverReSendsA403(t *testing.T) {
	srv, rec := tlsServer(t, status(http.StatusForbidden))
	tokens := &scriptedTokens{token: "old", next: "new"}
	resp, err := post(t, connectionClient(srv, tokens), context.Background(), srv.URL+"/mcp", "{}")
	if err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("response = %v, %v", resp, err)
	}
	if n := len(rec.requests()); n != 1 || tokens.refreshes != 0 {
		t.Fatalf("server saw %d requests after %d refreshes", n, tokens.refreshes)
	}
}

func TestTransportNeverFollowsARedirect(t *testing.T) {
	var target atomic.Int32
	other, _ := tlsServer(t, func(w http.ResponseWriter, _ string) { target.Add(1); w.WriteHeader(http.StatusOK) })
	for _, code := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		srv, rec := tlsServer(t, func(w http.ResponseWriter, _ string) {
			w.Header().Set("Location", other.URL+"/moved")
			w.WriteHeader(code)
		})
		resp, err := post(t, connectionClient(srv, &scriptedTokens{token: "t"}), context.Background(), srv.URL+"/mcp", `{"method":"tools/call"}`)
		if err != nil || resp.StatusCode != code {
			t.Fatalf("%d: response = %v, %v", code, resp, err)
		}
		if n := len(rec.requests()); n != 1 {
			t.Fatalf("%d: server saw %d requests", code, n)
		}
	}
	if target.Load() != 0 {
		t.Fatal("a redirect was followed")
	}
	// A client that does follow redirects still cannot re-send through
	// the transport.
	srv, _ := tlsServer(t, func(w http.ResponseWriter, _ string) {
		w.Header().Set("Location", other.URL+"/moved")
		w.WriteHeader(http.StatusTemporaryRedirect)
	})
	following := &http.Client{Transport: NewTransport(srv.Client().Transport, hostOf(srv.URL), &scriptedTokens{token: "t"})}
	if _, err := post(t, following, context.Background(), srv.URL+"/mcp", "{}"); !errors.Is(err, ErrRedirect) {
		t.Fatalf("followed redirect error = %v, want ErrRedirect", err)
	}
	if target.Load() != 0 {
		t.Fatal("a redirect was re-sent through the transport")
	}
}

func TestTransportSendsTheTokenOnlyOverHTTPSToTheConnectionsHost(t *testing.T) {
	srv, rec := tlsServer(t, status(http.StatusOK))
	client := NewHTTPClient(srv.Client().Transport, "mcp.example.com", &scriptedTokens{token: "secret"})
	if _, err := post(t, client, context.Background(), srv.URL+"/elsewhere", "{}"); err != nil {
		t.Fatal(err)
	}
	if auth := rec.requests()[0].auth; auth != "" {
		t.Fatalf("another host received %q", auth)
	}
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("plain http received %q", r.Header.Get("Authorization"))
		}
	}))
	defer plain.Close()
	client = NewHTTPClient(http.DefaultTransport, hostOf(plain.URL), &scriptedTokens{token: "secret"})
	if _, err := post(t, client, context.Background(), plain.URL+"/mcp", "{}"); err != nil {
		t.Fatal(err)
	}
}

// recordingBase captures the request the transport hands to net/http.
type recordingBase struct{ got *http.Request }

func (r *recordingBase) RoundTrip(req *http.Request) (*http.Response, error) {
	r.got = req
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
}

func TestTransportLeavesNetHTTPNoWayToReplayABody(t *testing.T) {
	base := &recordingBase{}
	client := NewHTTPClient(base, "mcp.example.com", &scriptedTokens{token: "t"})
	if _, err := post(t, client, context.Background(), "https://mcp.example.com/mcp", "{}"); err != nil {
		t.Fatal(err)
	}
	if base.got.GetBody != nil {
		t.Fatal("the request keeps GetBody, so net/http could replay the POST")
	}
	if base.got.Header.Get("Authorization") != "Bearer t" {
		t.Fatal("the connection's own https host got no token")
	}
}

func TestSendTraceReportsADialFailureAsNotSent(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // nothing listens here now
	client := NewHTTPClient(http.DefaultTransport, addr, &scriptedTokens{token: "t"})
	ctx, trace := WithSendTrace(context.Background())
	_, err = post(t, client, ctx, "https://"+addr+"/mcp", "{}")
	if err == nil {
		t.Fatal("expected a dial error")
	}
	if outcome := trace.Outcome(err); outcome != OutcomeNotSent {
		t.Fatalf("Outcome() = %v, want not sent", outcome)
	}
}

func TestSendTraceReportsADroppedConnectionAfterWritingAsUnknown(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		requests.Add(1) // the server acted on the request
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			conn.Close()
		}
	}))
	defer srv.Close()
	ctx, trace := WithSendTrace(context.Background())
	_, err := post(t, connectionClient(srv, &scriptedTokens{token: "t"}), ctx, srv.URL+"/mcp", `{"method":"tools/call"}`)
	if err == nil {
		t.Fatal("expected a dropped-connection error")
	}
	if outcome := trace.Outcome(err); outcome != OutcomeUnknown {
		t.Fatalf("Outcome() = %v, want unknown", outcome)
	}
	if n := requests.Load(); n != 1 {
		t.Fatalf("the server received %d requests; an unknown outcome must not be retried", n)
	}
}

func TestSendTraceReportsATimeoutAfterWritingAsUnknown(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		<-release
	}))
	defer srv.Close()
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	ctx, trace := WithSendTrace(ctx)
	_, err := post(t, connectionClient(srv, &scriptedTokens{token: "t"}), ctx, srv.URL+"/mcp", "{}")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
	if outcome := trace.Outcome(err); outcome != OutcomeUnknown {
		t.Fatalf("Outcome() = %v, want unknown", outcome)
	}
}

func TestSendTraceReportsACompleteRefusalAsRejected(t *testing.T) {
	for _, code := range []int{http.StatusForbidden, http.StatusBadRequest} {
		srv, _ := tlsServer(t, status(code))
		ctx, trace := WithSendTrace(context.Background())
		if _, err := post(t, connectionClient(srv, &scriptedTokens{token: "t"}), ctx, srv.URL+"/mcp", "{}"); err != nil {
			t.Fatal(err)
		}
		// The SDK turns a non-2xx answer into an error.
		if outcome := trace.Outcome(errors.New("rejected by transport")); outcome != OutcomeRejected {
			t.Fatalf("%d: Outcome() = %v, want rejected", code, outcome)
		}
	}
	srv, _ := tlsServer(t, status(http.StatusBadGateway))
	ctx, trace := WithSendTrace(context.Background())
	if _, err := post(t, connectionClient(srv, &scriptedTokens{token: "t"}), ctx, srv.URL+"/mcp", "{}"); err != nil {
		t.Fatal(err)
	}
	if outcome := trace.Outcome(errors.New("rejected by transport")); outcome != OutcomeUnknown {
		t.Fatalf("502: Outcome() = %v, want unknown", outcome)
	}
}

func TestSendTraceWithoutAnErrorIsSent(t *testing.T) {
	srv, _ := tlsServer(t, status(http.StatusOK))
	ctx, trace := WithSendTrace(context.Background())
	if _, err := post(t, connectionClient(srv, &scriptedTokens{token: "t"}), ctx, srv.URL+"/mcp", "{}"); err != nil {
		t.Fatal(err)
	}
	if outcome := trace.Outcome(nil); outcome != OutcomeSent {
		t.Fatalf("Outcome() = %v, want sent", outcome)
	}
}
