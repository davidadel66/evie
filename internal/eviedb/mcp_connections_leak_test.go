package eviedb

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/mcpconn"
	"github.com/davidadel66/evie/internal/mcpconn/mcptest"
)

// recordingSecurity stands in for /usr/bin/security: it keeps items in
// memory and records every argv it was given.
type recordingSecurity struct {
	mu    sync.Mutex
	items map[string]string
	argv  []string
}

func (r *recordingSecurity) run(_ context.Context, stdin string, args ...string) ([]byte, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.argv = append(r.argv, strings.Join(args, " "))
	if r.items == nil {
		r.items = map[string]string{}
	}
	flag := func(fields []string, name string) string {
		for i := 0; i+1 < len(fields); i++ {
			if fields[i] == name {
				return fields[i+1]
			}
		}
		return ""
	}
	switch args[0] {
	case "-i":
		fields := strings.Fields(stdin)
		r.items[flag(fields, "-a")] = strings.Trim(flag(fields, "-w"), `"`)
	case "find-generic-password":
		value, ok := r.items[flag(args, "-a")]
		if !ok {
			return nil, 44, nil
		}
		return []byte(value + "\n"), 0, nil
	case "delete-generic-password":
		delete(r.items, flag(args, "-a"))
	}
	return nil, 0, nil
}

// workspace-mcp-connections.spec.md verification: one token through
// connect, refresh and disconnect never appears in the SQLite file (or its
// WAL) or in security(1)'s argv.
func TestNoMCPCredentialReachesTheDatabaseFileOrArgv(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := OpenDBAt(filepath.Join(dir, "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	fake := mcptest.NewAuthServer(t)
	security := &recordingSecurity{}
	now := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	var clockMu sync.Mutex
	clock := func() time.Time { clockMu.Lock(); defer clockMu.Unlock(); return now }
	m := mcpconn.NewManager(mcpconn.Config{Records: store, Secrets: mcpconn.Keychain{Run: security.run},
		HTTPClient: fake.Server.Client(), RedirectURL: "http://127.0.0.1:6687/api/connections/oauth/callback", Now: clock})

	if _, err := m.Add(ctx, mcpconn.AddRequest{Name: "canva", URL: fake.Resource(), ClientID: "portal", ClientSecret: "portal-secret"}); err != nil {
		t.Fatal(err)
	}
	attempt, err := m.BeginConnect(ctx, "canva")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Callback(ctx, fake.Approve(t, attempt.AuthorizationURL)); err != nil {
		t.Fatal(err)
	}
	clockMu.Lock()
	now = now.Add(time.Hour)
	clockMu.Unlock()
	if token, err := m.TokenSource("mcp-canva").Token(ctx); err != nil || token != "access-2" {
		t.Fatalf("Token() = %q, %v", token, err)
	}
	if _, err := m.Disconnect(ctx, "canva"); err != nil {
		t.Fatal(err)
	}

	secrets := []string{"access-1", "access-2", "refresh-1", "refresh-2", "portal-secret"}
	files, err := filepath.Glob(filepath.Join(dir, "evie.db*"))
	if err != nil || len(files) == 0 {
		t.Fatalf("database files = %v, %v", files, err)
	}
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range secrets {
			if bytes.Contains(content, []byte(secret)) {
				t.Fatalf("%s contains %q", filepath.Base(file), secret)
			}
		}
	}
	security.mu.Lock()
	defer security.mu.Unlock()
	for _, argv := range security.argv {
		for _, secret := range secrets {
			if strings.Contains(argv, secret) {
				t.Fatalf("security argv %q contains %q", argv, secret)
			}
		}
	}
	if c, err := store.MCPConnection(ctx, "mcp-canva"); err != nil || c.Status != mcpconn.StatusNotConnected {
		t.Fatalf("record after Disconnect = %+v, %v", c, err)
	}
}
