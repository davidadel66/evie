package eviedb

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/mcpconn"
)

func TestMCPConnectionRecordsRoundTrip(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	created := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	square := mcpconn.Connection{ID: "mcp-square", Name: "square", URL: "https://mcp.squareup.com/mcp",
		Status: mcpconn.StatusNotConnected, CreatedAt: created, UpdatedAt: created}
	if err := store.CreateMCPConnection(ctx, square); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateMCPConnection(ctx, square); !errors.Is(err, mcpconn.ErrConnectionExists) {
		t.Fatalf("duplicate create error = %v", err)
	}
	canva := mcpconn.Connection{ID: "mcp-canva", Name: "canva", URL: "https://mcp.canva.com/mcp", ClientID: "portal",
		Status: mcpconn.StatusNotConnected, CreatedAt: created, UpdatedAt: created}
	if err := store.CreateMCPConnection(ctx, canva); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MCPConnection(ctx, "mcp-absent"); !errors.Is(err, mcpconn.ErrConnectionNotFound) {
		t.Fatalf("missing connection error = %v", err)
	}

	meta := mcpconn.OAuthMetadata{Resource: square.URL, Issuer: "https://mcp.squareup.com", TokenEndpoint: "https://mcp.squareup.com/token",
		TokenAuthMethods: []string{"none"}}
	later := created.Add(time.Minute)
	if err := store.SetMCPConnectionStatus(ctx, "mcp-square", mcpconn.StatusConnected, "", &meta, later); err != nil {
		t.Fatal(err)
	}
	if err := store.SetMCPConnectionStatus(ctx, "mcp-square", mcpconn.StatusNeedsSignIn, "refresh refused", nil, later.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, err := store.MCPConnection(ctx, "mcp-square")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != mcpconn.StatusNeedsSignIn || got.StatusDetail != "refresh refused" || got.OAuth.TokenEndpoint != meta.TokenEndpoint ||
		len(got.OAuth.TokenAuthMethods) != 1 || !got.CreatedAt.Equal(created) || !got.UpdatedAt.Equal(later.Add(time.Minute)) {
		t.Fatalf("MCPConnection() = %+v", got)
	}
	if err := store.SetMCPConnectionStatus(ctx, "mcp-absent", mcpconn.StatusConnected, "", nil, later); !errors.Is(err, mcpconn.ErrConnectionNotFound) {
		t.Fatalf("status of a missing connection error = %v", err)
	}
	if err := store.SetMCPConnectionStatus(ctx, "mcp-square", mcpconn.Status("bogus"), "", nil, later); err == nil {
		t.Fatal("an unknown status was stored")
	}

	all, err := store.MCPConnections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].Name != "canva" || all[1].Name != "square" || all[0].ClientID != "portal" {
		t.Fatalf("MCPConnections() = %+v", all)
	}
}
