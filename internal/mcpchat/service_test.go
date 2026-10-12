package mcpchat

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/mcpconn"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflow"
)

// memoryStore keeps Workspace Access revisions, chat pins and Tool
// Policies in memory.
type memoryStore struct {
	revisions      []workflow.WorkspaceAccess // index = revision - 1
	pins           map[memory.SessionID]int64
	archived       bool
	policyRevision int64
	policies       map[string]map[string]bool
}

func (m *memoryStore) MCPToolPolicies(context.Context, memory.WorkspaceID) (int64, map[string]map[string]bool, error) {
	out := map[string]map[string]bool{}
	for id, selectors := range m.policies {
		out[id] = map[string]bool{}
		for s, v := range selectors {
			out[id][s] = v
		}
	}
	return m.policyRevision, out, nil
}

func (m *memoryStore) SetMCPToolPolicy(_ context.Context, _ memory.WorkspaceID, expected int64, id, selector string, allow bool) (int64, error) {
	if expected != m.policyRevision {
		return 0, eviedb.ErrMCPToolPoliciesChanged
	}
	if m.policies == nil {
		m.policies = map[string]map[string]bool{}
	}
	if allow {
		if m.policies[id] == nil {
			m.policies[id] = map[string]bool{}
		}
		m.policies[id][selector] = true
	} else {
		delete(m.policies[id], selector)
	}
	m.policyRevision++
	return m.policyRevision, nil
}

const ws = memory.WorkspaceID("ws-cairo")

func (m *memoryStore) SessionWorkspaceAccessPin(_ context.Context, s memory.SessionID) (memory.WorkspaceID, int64, error) {
	rev, ok := m.pins[s]
	if !ok {
		return "", 0, eviedb.ErrNoWorkspaceAccessPin
	}
	return ws, rev, nil
}

func (m *memoryStore) WorkspaceAccess(ctx context.Context, id memory.WorkspaceID) (eviedb.WorkspaceAccessRevision, error) {
	return m.WorkspaceAccessAt(ctx, id, int64(len(m.revisions)))
}

func (m *memoryStore) WorkspaceAccessAt(_ context.Context, id memory.WorkspaceID, revision int64) (eviedb.WorkspaceAccessRevision, error) {
	if revision == 0 {
		return eviedb.WorkspaceAccessRevision{WorkspaceID: id, Access: workflow.WorkspaceAccess{Connections: []workflow.ConnectionAccess{}}}, nil
	}
	if revision > int64(len(m.revisions)) {
		return eviedb.WorkspaceAccessRevision{}, eviedb.ErrWorkspaceAccessRevisionNotFound
	}
	return eviedb.WorkspaceAccessRevision{WorkspaceID: id, Revision: revision, Access: m.revisions[revision-1]}, nil
}

func (m *memoryStore) SetWorkspaceAccess(_ context.Context, id memory.WorkspaceID, expected int64, access workflow.WorkspaceAccess) (eviedb.WorkspaceAccessRevision, error) {
	if m.archived {
		return eviedb.WorkspaceAccessRevision{}, eviedb.ErrWorkspaceInactive
	}
	if expected != int64(len(m.revisions)) {
		return eviedb.WorkspaceAccessRevision{}, eviedb.ErrWorkspaceAccessChanged
	}
	normalized, err := workflow.NormalizeAccess(access)
	if err != nil {
		return eviedb.WorkspaceAccessRevision{}, err
	}
	m.revisions = append(m.revisions, normalized)
	return eviedb.WorkspaceAccessRevision{WorkspaceID: id, Revision: int64(len(m.revisions)), Access: normalized}, nil
}

func (m *memoryStore) MCPConnection(_ context.Context, id string) (mcpconn.Connection, error) {
	switch id {
	case "mcp-square":
		return mcpconn.Connection{ID: id, Name: "square", Status: mcpconn.StatusConnected}, nil
	case "mcp-canva":
		return mcpconn.Connection{ID: id, Name: "canva", Status: mcpconn.StatusNeedsSignIn}, nil
	}
	return mcpconn.Connection{}, mcpconn.ErrConnectionNotFound
}

type fakeTools struct {
	tools map[string][]mcpconn.Tool
}

func (f *fakeTools) ListTools(_ context.Context, id string) ([]mcpconn.Tool, error) {
	return f.tools[id], nil
}

func (f *fakeTools) CallTool(context.Context, string, string, json.RawMessage) (mcpconn.CallResult, error) {
	return mcpconn.CallResult{}, nil
}

func squareTools() []mcpconn.Tool {
	return []mcpconn.Tool{
		{Name: "make_api_request_read", Hash: "r1"},
		{Name: "make_api_request", Hash: "w1"},
		{Name: "has space", Hash: "s1"},
	}
}

func access(entries map[string][]string) workflow.WorkspaceAccess {
	out := workflow.WorkspaceAccess{}
	for id, resources := range entries {
		out.Connections = append(out.Connections, workflow.ConnectionAccess{ID: id, Resources: resources})
	}
	normalized, _ := workflow.NormalizeAccess(out)
	return normalized
}

func TestEnableAndDisableToolsThroughWorkspaceAccess(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{revisions: []workflow.WorkspaceAccess{access(map[string][]string{"github": {"repo:evie"}})}}
	service := New(store, &fakeTools{tools: map[string][]mcpconn.Tool{"mcp-square": squareTools()}})

	review, err := service.SetToolEnabled(ctx, ws, "square", "make_api_request_read", true)
	if err != nil {
		t.Fatal(err)
	}
	if review.AccessRevision != 2 || stateOf(review, "make_api_request_read") != ToolEnabled || stateOf(review, "make_api_request") != ToolAvailable ||
		stateOf(review, "has space") != ToolUnselectable {
		t.Fatalf("review = %+v", review)
	}
	current, _ := store.WorkspaceAccess(ctx, ws)
	want := access(map[string][]string{"github": {"repo:evie"}, "mcp-square": {"make_api_request_read@r1"}})
	if !reflect.DeepEqual(current.Access, want) {
		t.Fatalf("access = %+v", current.Access)
	}
	if _, err := service.SetToolEnabled(ctx, ws, "square", "has space", true); !errors.Is(err, ErrToolUnselectable) {
		t.Fatalf("enable an unselectable tool: %v", err)
	}
	if _, err := service.SetToolEnabled(ctx, ws, "square", "absent", true); !errors.Is(err, ErrToolNotOffered) {
		t.Fatalf("enable a missing tool: %v", err)
	}

	// The server changes the tool: the review shows it changed, and
	// re-enabling replaces the old selector.
	service.tools.(*fakeTools).tools["mcp-square"][0].Hash = "r2"
	review, err = service.Review(ctx, ws, "square")
	if err != nil {
		t.Fatal(err)
	}
	if stateOf(review, "make_api_request_read") != ToolChanged {
		t.Fatalf("a changed tool's state = %q", stateOf(review, "make_api_request_read"))
	}
	if _, err := service.SetToolEnabled(ctx, ws, "square", "make_api_request_read", true); err != nil {
		t.Fatal(err)
	}
	current, _ = store.WorkspaceAccess(ctx, ws)
	if !reflect.DeepEqual(current.Access, access(map[string][]string{"github": {"repo:evie"}, "mcp-square": {"make_api_request_read@r2"}})) {
		t.Fatalf("access after re-review = %+v", current.Access)
	}

	// Disabling the last tool removes the connection from the Workspace.
	if _, err := service.SetToolEnabled(ctx, ws, "square", "make_api_request_read", false); err != nil {
		t.Fatal(err)
	}
	current, _ = store.WorkspaceAccess(ctx, ws)
	if !reflect.DeepEqual(current.Access, access(map[string][]string{"github": {"repo:evie"}})) {
		t.Fatalf("access after disabling the last tool = %+v", current.Access)
	}
}

func TestReviewListsReviewedToolsTheServerNoLongerOffers(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{revisions: []workflow.WorkspaceAccess{access(map[string][]string{"mcp-square": {"retired@x1", "make_api_request_read@r1"}})}}
	service := New(store, &fakeTools{tools: map[string][]mcpconn.Tool{"mcp-square": squareTools()}})
	review, err := service.Review(ctx, ws, "square")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(review.NoLongerOffered, []string{"retired"}) {
		t.Fatalf("no longer offered = %v", review.NoLongerOffered)
	}
	// Disabling a tool the server dropped still works.
	if _, err := service.SetToolEnabled(ctx, ws, "square", "retired", false); err != nil {
		t.Fatal(err)
	}
}

// failingTools is a server that cannot be listed (signed out or down).
type failingTools struct{ fakeTools }

func (*failingTools) ListTools(context.Context, string) ([]mcpconn.Tool, error) {
	return nil, mcpconn.ErrNotConnected
}

func TestDisablingWorksWhileTheServerCannotBeListed(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{revisions: []workflow.WorkspaceAccess{access(map[string][]string{"mcp-square": {"make_api_request@w1", "make_api_request_read@r1"}})}}
	service := New(store, &failingTools{})
	review, err := service.SetToolEnabled(ctx, ws, "square", "make_api_request", false)
	if err != nil {
		t.Fatal(err)
	}
	if !review.Unlisted || !reflect.DeepEqual(review.Reviewed, []string{"make_api_request_read"}) {
		t.Fatalf("review = %+v", review)
	}
	current, _ := store.WorkspaceAccess(ctx, ws)
	if !reflect.DeepEqual(current.Access, access(map[string][]string{"mcp-square": {"make_api_request_read@r1"}})) {
		t.Fatalf("access = %+v", current.Access)
	}
	if _, err := service.SetToolEnabled(ctx, ws, "square", "make_api_request", true); !errors.Is(err, mcpconn.ErrNotConnected) {
		t.Fatalf("enabling without the server: %v", err)
	}
	if _, err := service.Review(ctx, ws, "square"); !errors.Is(err, mcpconn.ErrNotConnected) {
		t.Fatalf("review without the server: %v", err)
	}
}

func TestToolChangesRefuseAStaleRevisionAndAnArchivedWorkspace(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{}
	service := New(store, &fakeTools{tools: map[string][]mcpconn.Tool{"mcp-square": squareTools()}})
	store.archived = true
	if _, err := service.SetToolEnabled(ctx, ws, "square", "make_api_request_read", true); !errors.Is(err, eviedb.ErrWorkspaceInactive) {
		t.Fatalf("archived Workspace: %v", err)
	}
	store.archived = false
	racing := &racingStore{memoryStore: store}
	service = New(racing, &fakeTools{tools: map[string][]mcpconn.Tool{"mcp-square": squareTools()}})
	if _, err := service.SetToolEnabled(ctx, ws, "square", "make_api_request_read", true); !errors.Is(err, eviedb.ErrWorkspaceAccessChanged) {
		t.Fatalf("stale revision: %v", err)
	}
}

// racingStore lands another access change between the read and the write.
type racingStore struct{ *memoryStore }

func (r *racingStore) SetWorkspaceAccess(ctx context.Context, id memory.WorkspaceID, expected int64, a workflow.WorkspaceAccess) (eviedb.WorkspaceAccessRevision, error) {
	r.revisions = append(r.revisions, access(map[string][]string{"github": {"repo:other"}}))
	return r.memoryStore.SetWorkspaceAccess(ctx, id, expected, a)
}

func TestReachIsThePinnedAccessIntersectedWithTheCurrentAccess(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{
		revisions: []workflow.WorkspaceAccess{
			access(map[string][]string{"mcp-square": {"make_api_request_read@r1", "make_api_request@w1"}, "github": {"repo:evie"}}),
			access(map[string][]string{"mcp-square": {"make_api_request_read@r1", "make_api_request@w1", "get_type_info@t1"}}),
			access(map[string][]string{"mcp-square": {"make_api_request_read@r1", "get_type_info@t1"}}),
		},
		pins: map[memory.SessionID]int64{"early": 1, "fresh": 3, "none": 0},
	}
	service := New(store, &fakeTools{})
	reach, err := service.Reach(ctx, "early", ws)
	if err != nil {
		t.Fatal(err)
	}
	// Pinned before get_type_info was added, and make_api_request was
	// removed since: only the read tool remains. github is not MCP.
	if !reflect.DeepEqual(reach, map[string]map[string]bool{"mcp-square": {"make_api_request_read@r1": true}}) {
		t.Fatalf("early reach = %v", reach)
	}
	if reach, _ := service.Reach(ctx, "fresh", ws); len(reach["mcp-square"]) != 2 {
		t.Fatalf("fresh reach = %v", reach)
	}
	if reach, _ := service.Reach(ctx, "none", ws); len(reach) != 0 {
		t.Fatalf("a chat pinned before any access reaches %v", reach)
	}
	if reach, err := service.Reach(ctx, "unpinned", ws); err != nil || len(reach) != 0 {
		t.Fatalf("an unpinned chat reaches %v, %v", reach, err)
	}
	if _, err := service.Reach(ctx, "early", "another-workspace"); err == nil {
		t.Fatal("a chat reached another Workspace's access")
	}
}

func TestAllowIsBoundToTheEnabledVersion(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{}
	tools := &fakeTools{tools: map[string][]mcpconn.Tool{"mcp-square": squareTools()}}
	service := New(store, tools)
	if _, err := service.SetToolPolicy(ctx, ws, "square", "make_api_request_read", true); !errors.Is(err, ErrToolNotEnabled) {
		t.Fatalf("allowing a tool that is not enabled: %v", err)
	}
	if _, err := service.SetToolEnabled(ctx, ws, "square", "make_api_request_read", true); err != nil {
		t.Fatal(err)
	}
	if allowed, _ := service.Allowed(ctx, ws, "mcp-square", "make_api_request_read@r1"); allowed {
		t.Fatal("a newly enabled tool is allowed; the default is ask")
	}
	review, err := service.SetToolPolicy(ctx, ws, "square", "make_api_request_read", true)
	if err != nil {
		t.Fatal(err)
	}
	if policyOf(review, "make_api_request_read") != PolicyAllow || policyOf(review, "make_api_request") != "" {
		t.Fatalf("review = %+v", review.Tools)
	}
	if allowed, _ := service.Allowed(ctx, ws, "mcp-square", "make_api_request_read@r1"); !allowed {
		t.Fatal("the allowed version asks")
	}
	// The server changes the tool; David re-enables the new version, which
	// starts at ask.
	tools.tools["mcp-square"][0].Hash = "r2"
	if review, err = service.SetToolEnabled(ctx, ws, "square", "make_api_request_read", true); err != nil {
		t.Fatal(err)
	}
	if policyOf(review, "make_api_request_read") != PolicyAsk {
		t.Fatalf("a re-reviewed tool's policy = %q", policyOf(review, "make_api_request_read"))
	}
	if allowed, _ := service.Allowed(ctx, ws, "mcp-square", "make_api_request_read@r2"); allowed {
		t.Fatal("the new version inherited Allow")
	}
	if _, err := service.SetToolPolicy(ctx, ws, "square", "make_api_request_read", true); err != nil {
		t.Fatal(err)
	}
	if review, err = service.SetToolPolicy(ctx, ws, "square", "make_api_request_read", false); err != nil || policyOf(review, "make_api_request_read") != PolicyAsk {
		t.Fatalf("back to ask = %+v, %v", review.Tools, err)
	}
	// Disabling forgets Allow: enabling the same version again asks.
	if _, err := service.SetToolPolicy(ctx, ws, "square", "make_api_request_read", true); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetToolEnabled(ctx, ws, "square", "make_api_request_read", false); err != nil {
		t.Fatal(err)
	}
	if review, err = service.SetToolEnabled(ctx, ws, "square", "make_api_request_read", true); err != nil || policyOf(review, "make_api_request_read") != PolicyAsk {
		t.Fatalf("re-enabled after disabling = %+v, %v", review.Tools, err)
	}
	if allowed, _ := service.Allowed(ctx, ws, "mcp-square", "make_api_request_read@r2"); allowed {
		t.Fatal("a disabled and re-enabled tool kept Allow")
	}
}

func policyOf(review Review, name string) ToolPolicy {
	for _, tool := range review.Tools {
		if tool.Name == name {
			return tool.Policy
		}
	}
	return "missing"
}

func stateOf(review Review, name string) ToolState {
	for _, tool := range review.Tools {
		if tool.Name == name {
			return tool.State
		}
	}
	return ""
}
