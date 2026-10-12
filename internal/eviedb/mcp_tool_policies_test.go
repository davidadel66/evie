package eviedb

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

// workspace-mcp-connections.spec.md story 3: Tool Policies are append-only
// revisions per Workspace; only Allow is stored, Ask is the default.
func TestMCPToolPoliciesAreRevisionedPerWorkspace(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	cairo, err := store.RegisterWorkspace(ctx, "CairosKitchen")
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.RegisterWorkspace(ctx, "Finance")
	if err != nil {
		t.Fatal(err)
	}

	revision, policies, err := store.MCPToolPolicies(ctx, cairo.ID)
	if err != nil || revision != 0 || len(policies) != 0 {
		t.Fatalf("initial policies = %d %v %v", revision, policies, err)
	}
	if revision, err = store.SetMCPToolPolicy(ctx, cairo.ID, 0, "mcp-square", "make_api_request_read@r1", true); err != nil || revision != 1 {
		t.Fatalf("allow = %d, %v", revision, err)
	}
	if _, err := store.SetMCPToolPolicy(ctx, cairo.ID, 0, "mcp-square", "get_type_info@t1", true); !errors.Is(err, ErrMCPToolPoliciesChanged) {
		t.Fatalf("stale revision error = %v", err)
	}
	if revision, err = store.SetMCPToolPolicy(ctx, cairo.ID, 1, "mcp-square", "get_type_info@t1", true); err != nil || revision != 2 {
		t.Fatalf("second allow = %d, %v", revision, err)
	}
	_, policies, _ = store.MCPToolPolicies(ctx, cairo.ID)
	want := map[string]map[string]bool{"mcp-square": {"make_api_request_read@r1": true, "get_type_info@t1": true}}
	if !reflect.DeepEqual(policies, want) {
		t.Fatalf("policies = %v", policies)
	}
	// Back to Ask removes the entry; the last one removes the connection.
	if _, err = store.SetMCPToolPolicy(ctx, cairo.ID, 2, "mcp-square", "get_type_info@t1", false); err != nil {
		t.Fatal(err)
	}
	if revision, err = store.SetMCPToolPolicy(ctx, cairo.ID, 3, "mcp-square", "make_api_request_read@r1", false); err != nil || revision != 4 {
		t.Fatalf("ask = %d, %v", revision, err)
	}
	if _, policies, _ = store.MCPToolPolicies(ctx, cairo.ID); len(policies) != 0 {
		t.Fatalf("policies after asking again = %v", policies)
	}
	if _, policies, _ = store.MCPToolPolicies(ctx, other.ID); len(policies) != 0 {
		t.Fatalf("another Workspace sees %v", policies)
	}
	if _, err := db.ExecContext(ctx, `UPDATE workspace_mcp_tool_policies SET policies='{}'`); err == nil {
		t.Fatal("a policy revision was updated")
	}
	if _, err := store.ArchiveWorkspace(ctx, cairo.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetMCPToolPolicy(ctx, cairo.ID, 4, "mcp-square", "make_api_request_read@r1", true); !errors.Is(err, ErrWorkspaceInactive) {
		t.Fatalf("archived Workspace error = %v", err)
	}
}
