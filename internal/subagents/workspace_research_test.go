package subagents_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

func (f *fixture) workspaceParent(t *testing.T, workspace memory.Workspace) delegation.Parent {
	t.Helper()
	ctx := context.Background()
	resolved, err := f.manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	session, err := f.store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := f.store.AcquireTurnLease(ctx, session.ID, memory.LeaseHolderID(uuid.NewString()), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	root, err := f.store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "private parent conversation"})
	if err != nil {
		t.Fatal(err)
	}
	return delegation.Parent{Scope: session.ScopeContext(), Lease: lease, SourceEventID: root.ID}
}

type legacyResearchWeb struct{ plugins.Web }

func (legacyResearchWeb) Manifest() plugins.Manifest {
	m := plugins.NewWeb().Manifest()
	m.ImplementationVersion = "1.0.0"
	m.Capabilities[0].Version = "1.0.0"
	m.ResumableFrom = nil
	return m
}
func (legacyResearchWeb) ToolCapabilities() []plugins.ToolCapability {
	return plugins.NewWeb().ResumableToolCapabilities("1.0.0")
}

func TestLegacyParentDelegatesWithoutUpgradingWebAuthority(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	ctx := context.Background()
	current := f.manager
	legacy, err := plugins.NewManager(tools.KernelToolset(), legacyResearchWeb{}, plugins.NewFinance(), plugins.NewYouTube(), plugins.NewTodo(f.store), plugins.NewSubagents(f.supervisor))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []plugins.PluginID{plugins.WebPluginID, plugins.FinancePluginID, plugins.YouTubePluginID, plugins.TodoPluginID, plugins.SubagentsPluginID} {
		if err := legacy.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	f.manager = legacy
	w, err := f.store.RegisterWorkspaceWithOptions(ctx, eviedb.WorkspaceRegistration{DisplayName: "Legacy research", AllowResearchDelegation: true})
	if err != nil {
		t.Fatal(err)
	}
	parent := f.workspaceParent(t, w)
	original, err := f.store.GetCompositionReceipt(ctx, parent.Scope.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	f.manager = current
	configure(t, f, clientFunc(func(ctx context.Context, request openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		for _, tool := range request.Tools {
			if tool.Function.Name == "web_fetch" {
				b, _ := json.Marshal(tool)
				if strings.Contains(string(b), "max_bytes") {
					return openrouter.ChatResponse{}, errors.New("legacy worker upgraded Web contract")
				}
			}
		}
		var count int
		if err := f.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM session_compatibility_resolutions WHERE session_id IN (SELECT id FROM sessions WHERE parent_session_id = ?)`, parent.Scope.SessionID).Scan(&count); err != nil {
			return openrouter.ChatResponse{}, err
		}
		if count != 1 {
			return openrouter.ChatResponse{}, errors.New("worker executed without compatibility evidence")
		}
		return response("Legacy research finding. Source: https://example.com/evidence"), nil
	}))
	results, err := f.delegate(t, ctx, parent, []delegation.Assignment{{Key: "legacy", Objective: "Research source"}})
	if err != nil || len(results) != 1 || results[0].Status != "succeeded" {
		t.Fatalf("legacy delegation: %+v %v", results, err)
	}
	after, err := f.store.GetCompositionReceipt(ctx, parent.Scope.SessionID)
	if err != nil || !reflect.DeepEqual(original, after) {
		t.Fatalf("parent receipt mutated: %v", err)
	}
	child, err := f.store.GetCompositionReceipt(ctx, results[0].ChildSessionID)
	if err != nil {
		t.Fatal(err)
	}
	for _, capability := range child.Capabilities {
		if capability.ContractVersion != "1.0.0" {
			t.Fatalf("child authority upgraded: %+v", child)
		}
	}
}

func TestWorkspaceResearchPermissionAdditionRevocationAndReenable(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	ctx := context.Background()
	w, err := f.store.RegisterWorkspace(ctx, "Research")
	if err != nil {
		t.Fatal(err)
	}
	old := f.workspaceParent(t, w)
	w, err = f.store.SetWorkspaceResearch(ctx, w.ID, w.CurrentRevisionID, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.delegate(t, ctx, old, []delegation.Assignment{{Key: "old", Objective: "research"}}); !errors.Is(err, delegation.ErrAuthority) || f.client.calls != 0 {
		t.Fatalf("old parent acquired permission: %v calls=%d", err, f.client.calls)
	}
	parent := f.workspaceParent(t, w)
	results, err := f.delegate(t, ctx, parent, []delegation.Assignment{{Key: "allowed", Objective: "research", Context: "selected fact"}})
	if err != nil || len(results) != 1 || results[0].Status != "succeeded" {
		t.Fatalf("allowed delegation=%+v error=%v", results, err)
	}
	child, err := f.store.GetSession(ctx, results[0].ChildSessionID)
	if err != nil || child.WorkspaceID != w.ID || child.WorkspaceRevisionSnapshot != parent.Scope.WorkspaceRevision {
		t.Fatalf("child scope=%+v error=%v", child, err)
	}
	w, err = f.store.SetWorkspaceResearch(ctx, w.ID, w.CurrentRevisionID, false)
	if err != nil {
		t.Fatal(err)
	}
	retained, err := f.store.InspectSubagent(ctx, parent, results[0].ExecutionID)
	if err != nil || retained.Result == nil || retained.Result.Status != "succeeded" {
		t.Fatalf("revocation lost accepted result: %+v %v", retained, err)
	}
	w, err = f.store.SetWorkspaceResearch(ctx, w.ID, w.CurrentRevisionID, true)
	if err != nil {
		t.Fatal(err)
	}
	calls := f.client.calls
	if _, err := f.delegate(t, ctx, parent, []delegation.Assignment{{Key: "revoked", Objective: "research"}}); !errors.Is(err, delegation.ErrAuthority) || f.client.calls != calls {
		t.Fatalf("revoked pin resurrected: %v", err)
	}
	fresh := f.workspaceParent(t, w)
	results, err = f.delegate(t, ctx, fresh, []delegation.Assignment{{Key: "fresh", Objective: "research"}})
	if err != nil || results[0].Status != "succeeded" {
		t.Fatalf("new grant=%+v %v", results, err)
	}
	if _, err := f.store.ArchiveWorkspace(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.InspectSubagent(ctx, fresh, results[0].ExecutionID); !errors.Is(err, delegation.ErrAuthority) {
		t.Fatalf("archived result access=%v", err)
	}
}

func TestWorkspaceResearchRevocationStopsActiveWorker(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	ctx := context.Background()
	w, err := f.store.RegisterWorkspaceWithOptions(ctx, eviedb.WorkspaceRegistration{DisplayName: "Research", AllowResearchDelegation: true})
	if err != nil {
		t.Fatal(err)
	}
	parent := f.workspaceParent(t, w)
	gate := &gatedProvider{entered: make(chan struct{}, 1), release: make(chan struct{}, 1)}
	configure(t, f, gate)
	done := make(chan []delegation.Result, 1)
	go func() {
		results, _ := f.delegate(t, ctx, parent, []delegation.Assignment{{Key: "active", Objective: "research"}})
		done <- results
	}()
	select {
	case <-gate.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not start")
	}
	if _, err := f.store.SetWorkspaceResearch(ctx, w.ID, w.CurrentRevisionID, false); err != nil {
		t.Fatal(err)
	}
	select {
	case results := <-done:
		if len(results) != 1 || results[0].Status == "succeeded" {
			t.Fatalf("revoked worker=%+v", results)
		}
		stored, err := f.store.InspectSubagent(ctx, parent, results[0].ExecutionID)
		if err != nil || stored.FinalEventID != "" {
			t.Fatalf("revoked final accepted: %+v %v", stored, err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("revoked worker kept running")
	}
}

func TestWorkerUsesInvokingModelAndOneMiBDefaultWithSmallParentWindow(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	profile, err := openrouter.NewExplicitContextProfile("selected/model", 2_000_000, 8192, 1024)
	if err != nil {
		t.Fatal(err)
	}
	parent := f.parent
	parent.Profile = &profile
	configure(t, f, clientFunc(func(_ context.Context, request openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		if request.Model != "selected/model" {
			t.Fatalf("worker model=%s", request.Model)
		}
		return response("Concise finding. Source: https://example.com/evidence"), nil
	}))
	results, err := f.delegate(t, context.Background(), parent, []delegation.Assignment{{Key: "selected", Objective: "research", Context: strings.Repeat("x", 7500)}})
	if err != nil || results[0].Status != "succeeded" {
		t.Fatalf("worker=%+v %v", results, err)
	}
	events, err := f.store.BindHistory(results[0].ChildSessionID, "inspect").Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var snapshot memory.ContextSnapshotPayload
	for _, event := range events {
		if event.Type == memory.EventContextSnapshot {
			if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
				t.Fatal(err)
			}
		}
	}
	if snapshot.UsableInputBytes != 1_048_576 || snapshot.ConfiguredModel != "selected/model" || snapshot.SerializedBytes <= 8192 {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	// The worker cites a URL without fetching it, so it is unverified.
	if strings.Contains(results[0].Summary, "private parent") || len(results[0].Sources) != 0 || len(results[0].UnverifiedURLs) != 1 {
		t.Fatalf("result=%+v", results[0])
	}
}

func TestWorkspaceWorkerReadsExcerptsBeyondLegacyBudgetAndReturnsOnlyFindings(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	ctx := context.Background()
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, strings.Repeat("irrelevant navigation\n", 6000)+"Relevant section\n"+strings.Repeat("evidence-sentinel ", 7000))
	}))
	defer page.Close()
	// Delegated workers may fetch only public addresses; the page stands in for one.
	t.Cleanup(tools.PermitWorkerFetchesForTest(netip.MustParseAddrPort(page.Listener.Addr().String())))
	manager, err := plugins.NewManager(tools.KernelToolset(), plugins.NewWeb(), plugins.NewFinance(), plugins.NewYouTube(), plugins.NewTodo(f.store), plugins.NewSubagents(f.supervisor))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []plugins.PluginID{plugins.WebPluginID, plugins.FinancePluginID, plugins.YouTubePluginID, plugins.TodoPluginID, plugins.SubagentsPluginID} {
		if err := manager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	f.manager = manager
	w, err := f.store.RegisterWorkspaceWithOptions(ctx, eviedb.WorkspaceRegistration{DisplayName: "Excerpts", AllowResearchDelegation: true})
	if err != nil {
		t.Fatal(err)
	}
	parent := f.workspaceParent(t, w)
	profile, err := openrouter.NewExplicitContextProfile("selected/large", 2_000_000, 8192, 1024)
	if err != nil {
		t.Fatal(err)
	}
	parent.Profile = &profile
	calls, largest := 0, 0
	configure(t, f, clientFunc(func(_ context.Context, request openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		calls++
		encoded, _ := json.Marshal(request)
		largest = max(largest, len(encoded))
		if request.Model != profile.Model() || len(request.Tools) != 2 || strings.Contains(string(encoded), "private parent conversation") || strings.Contains(string(encoded), "irrelevant navigation") {
			return openrouter.ChatResponse{}, errors.New("worker model, capability or excerpt isolation failed")
		}
		if calls == 4 {
			return response("The relevant section supports the finding. Source: " + page.URL), nil
		}
		args := map[string]any{"url": page.URL, "max_bytes": 32768}
		if calls == 1 {
			args["query"] = "Relevant section"
		} else {
			var last struct {
				SHA256     string `json:"sha256"`
				NextOffset *int   `json:"next_offset"`
				Content    string `json:"content"`
			}
			for _, message := range request.Messages {
				if message.Role == "tool" {
					if err := json.Unmarshal([]byte(message.Content), &last); err != nil {
						return openrouter.ChatResponse{}, err
					}
				}
			}
			if last.NextOffset == nil || !strings.Contains(last.Content, "evidence-sentinel") {
				return openrouter.ChatResponse{}, errors.New("missing bounded excerpt continuation")
			}
			args["offset"], args["expected_sha256"] = *last.NextOffset, last.SHA256
		}
		encodedArgs, _ := json.Marshal(args)
		return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", ToolCalls: []openrouter.ToolCall{{ID: fmt.Sprintf("excerpt-%d", calls), Type: "function", Function: openrouter.FunctionCall{Name: "web_fetch", Arguments: string(encodedArgs)}}}}}}}, nil
	}))
	results, err := f.delegate(t, ctx, parent, []delegation.Assignment{{Key: "excerpts", Objective: "Read relevant excerpts and return concise sourced findings"}})
	if err != nil || len(results) != 1 || results[0].Status != "succeeded" {
		t.Fatalf("excerpt research: %+v %v", results, err)
	}
	if calls != 4 || largest <= 65536 || largest >= 1<<20 {
		t.Fatalf("worker requests calls=%d largest=%d", calls, largest)
	}
	b, _ := json.Marshal(results[0])
	if len(b) > delegation.DefaultPolicy().ResultBytes || strings.Contains(string(b), "evidence-sentinel") || len(results[0].Sources) != 1 ||
		!results[0].Sources[0].Fetched || !results[0].Sources[0].Cited || len(results[0].UnverifiedURLs) != 0 {
		t.Fatalf("parent result contains worker pages: %s", b)
	}
}
