package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// delegatedWorkerContext carries the harness-owned invocation context the turn
// loop gives a delegated child session: its scope names a parent session.
func delegatedWorkerContext() context.Context {
	return WithInvocationContext(context.Background(), InvocationContext{Scope: memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "child-session", ParentSessionID: "parent-session",
	}})
}

func webFetchArgs(t *testing.T, raw string) string {
	t.Helper()
	args, err := json.Marshal(map[string]string{"url": raw})
	if err != nil {
		t.Fatal(err)
	}
	return string(args)
}

func countingServer(t *testing.T, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// T2: a delegated research worker's fetch is refused at dial time for any
// non-public destination — literal IPs, names that resolve there, and both
// the legacy and excerpt fetch contracts — while the main chat keeps its
// local and private access.
func TestWebFetchFromDelegatedWorkerRefusesNonPublicAddresses(t *testing.T) {
	saved := fetchTimeout
	fetchTimeout = 3 * time.Second
	t.Cleanup(func() { fetchTimeout = saved })

	srv, hits := countingServer(t, "private dev server")
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	tools := []struct {
		name    string
		execute func(context.Context, string) (string, error)
	}{
		{"legacy web_fetch", webFetch},
		{"excerpt web_fetch", webFetchExcerpt},
	}
	// Loopback first: before the fence existed these reach the local server
	// and stop the test, so no LAN address is ever contacted by a failing run.
	targets := []string{
		srv.URL,
		"http://localhost:" + port + "/",
		"http://[::1]:" + port + "/",
		"http://0.0.0.0:" + port + "/",
		"http://192.168.1.1/",
		"http://10.0.0.1/",
		"http://172.16.0.1/",
		"http://169.254.169.254/latest/meta-data/",
		"http://[fd00::1]/",
		"http://100.64.0.1/",
	}
	for _, target := range targets {
		for _, tool := range tools {
			out, err := tool.execute(delegatedWorkerContext(), webFetchArgs(t, target))
			if err == nil {
				t.Fatalf("%s from a delegated worker fetched %s: %q", tool.name, target, out)
			}
			if !strings.Contains(err.Error(), "public") {
				t.Fatalf("%s to %s failed for the wrong reason: %v", tool.name, target, err)
			}
		}
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("delegated worker reached the loopback server %d times", n)
	}

	// The main chat — with or without a harness invocation context — keeps
	// its local access.
	mainChat := WithInvocationContext(context.Background(), InvocationContext{Scope: memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "main-session",
	}})
	for _, ctx := range []context.Context{context.Background(), mainChat} {
		for _, tool := range tools {
			out, err := tool.execute(ctx, webFetchArgs(t, srv.URL))
			if err != nil || !strings.Contains(out, "private dev server") {
				t.Fatalf("main chat %s lost local access: %q, %v", tool.name, out, err)
			}
		}
	}
}

// The fence is in the dialer, so a redirect hop is checked like the first
// request: a same-host redirect is followed automatically, and a cross-host
// one is returned to the model, which fetches it as a new request.
func TestWebFetchFromDelegatedWorkerChecksEveryRedirectHop(t *testing.T) {
	private, privateHits := countingServer(t, "metadata secrets")
	_, privatePort, err := net.SplitHostPort(private.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	entry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Same hostname, different port: sameHost follows it.
		http.Redirect(w, r, "http://127.0.0.1:"+privatePort+"/latest/meta-data/", http.StatusFound)
	}))
	t.Cleanup(entry.Close)

	// The entry server stands in for a public host; every other address,
	// including the redirect target, is judged by the real policy.
	t.Cleanup(PermitWorkerFetchesForTest(netip.MustParseAddrPort(entry.Listener.Addr().String())))

	for _, execute := range []func(context.Context, string) (string, error){webFetch, webFetchExcerpt} {
		out, err := execute(delegatedWorkerContext(), webFetchArgs(t, entry.URL))
		if err == nil {
			t.Fatalf("delegated worker followed a redirect into a private address: %q", out)
		}
		if !strings.Contains(err.Error(), "public") {
			t.Fatalf("redirect hop failed for the wrong reason: %v", err)
		}
	}
	if n := privateHits.Load(); n != 0 {
		t.Fatalf("redirect reached the private server %d times", n)
	}
}

func TestIsPublicAddress(t *testing.T) {
	for _, tc := range []struct {
		addr   string
		public bool
	}{
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"2606:4700:4700::1111", true},
		{"127.0.0.1", false},
		{"127.8.9.10", false},
		{"::1", false},
		{"0.0.0.0", false},
		{"0.1.2.3", false},
		{"::", false},
		{"10.1.2.3", false},
		{"172.16.0.1", false},
		{"172.31.255.255", false},
		{"192.168.0.10", false},
		{"169.254.169.254", false},
		{"fe80::1", false},
		{"fc00::1", false},
		{"fd12:3456::1", false},
		{"100.64.0.1", false},
		{"224.0.0.1", false},
		{"ff02::1", false},
		{"255.255.255.255", false},
		{"::ffff:127.0.0.1", false},
		{"::ffff:192.168.1.1", false},
	} {
		if got := isPublicAddress(netip.MustParseAddr(tc.addr)); got != tc.public {
			t.Errorf("isPublicAddress(%s) = %t, want %t", tc.addr, got, tc.public)
		}
	}
}
