package subagents_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/subagents"
	"github.com/davidadel66/evie/internal/task"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

type provider struct {
	mu       sync.Mutex
	calls    int
	requests []openrouter.ChatRequest
}

func (p *provider) ChatStream(_ context.Context, r openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.requests = append(p.requests, r)
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", Content: "Research finding. Source: https://example.com/evidence"}}}}, nil
}

type fixture struct {
	db         *sql.DB
	store      *eviedb.Store
	supervisor *subagents.Supervisor
	manager    *plugins.Manager
	parent     delegation.Parent
	client     *provider
	profile    openrouter.ContextProfile
}

func setup(t *testing.T, policy delegation.Policy) *fixture {
	t.Helper()
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	f := &fixture{db: db, store: eviedb.NewStore(db), client: &provider{}}
	f.profile, err = openrouter.NewExplicitContextProfile("test-model", 131072, 65536, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f.supervisor, err = subagents.New(f.store, policy)
	if err != nil {
		t.Fatal(err)
	}
	f.manager, err = plugins.NewManager(tools.KernelToolset(), researchWeb{}, plugins.NewFinance(), plugins.NewYouTube(), plugins.NewTodo(f.store), plugins.NewSubagents(f.supervisor))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []plugins.PluginID{plugins.WebPluginID, plugins.FinancePluginID, plugins.YouTubePluginID, plugins.TodoPluginID, plugins.SubagentsPluginID} {
		if err = f.manager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	f.supervisor.Configure(f.client, f.profile, func(ctx context.Context, receipt *composition.Receipt) (subagents.Composition, error) {
		var r plugins.ResolvedComposition
		var err error
		if receipt == nil {
			r, err = f.manager.ResolvePresetContext(ctx, plugins.ResearchPresetID)
		} else {
			r, err = f.manager.ResumeCompositionContext(ctx, *receipt)
		}
		return subagents.Composition{Receipt: r.Receipt, Toolset: r.Toolset, Instructions: plugins.ResearchInstructions}, err
	})
	resolved, err := f.manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := f.store.CreateGlobalSessionWithComposition(ctx, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := f.store.AcquireTurnLease(ctx, stored.ID, "orchestrator", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	root, err := f.store.AppendEventWithLease(ctx, stored.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "parent private history"})
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(memory.ToolIntentPayload{Call: memory.ToolCall{ID: "delegation", Name: delegation.ToolName, Arguments: `{}`}})
	intent, err := f.store.AppendEventWithLease(ctx, stored.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventToolIntent, ParentID: root.ID, ExecutionID: "invocation", Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	f.parent = delegation.Parent{Scope: stored.ScopeContext(), Lease: lease, SourceEventID: root.ID, IntentEventID: intent.ID}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		f.supervisor.Stop(ctx)
	})
	return f
}

func TestForegroundAssignmentRetainsResultAcrossDuplicateRequests(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	ctx := context.Background()
	requests := []delegation.Assignment{{Key: "evidence", Objective: "Research source", Context: "selected fact"}}
	first, err := f.delegate(t, ctx, f.parent, requests)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].Status != "succeeded" || first[0].Findings == "" || len(first[0].Sources) != 1 || first[0].Usage != nil {
		t.Fatalf("outcome: %+v", first)
	}
	second, err := f.delegate(t, ctx, f.parent, requests)
	if err != nil {
		t.Fatal(err)
	}
	if first[0].ExecutionID != second[0].ExecutionID || f.client.calls != 1 {
		t.Fatalf("retry reran child: %+v", second)
	}
	events, err := f.store.LoadEvents(ctx, first[0].ChildSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) < 3 {
		t.Fatalf("child history missing: %+v", events)
	}
	_ = agent.DefaultModel
}

type gatedProvider struct {
	mu                  sync.Mutex
	active, peak, calls int
	entered             chan struct{}
	release             chan struct{}
}

// Gate arrival is a liveness check, not a performance target. Allow race builds
// and concurrent repository checks time to schedule independent SQLite owners.
const gateTimeout = 5 * time.Second

func (p *gatedProvider) ChatStream(ctx context.Context, _ openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	p.mu.Lock()
	p.active++
	p.calls++
	if p.active > p.peak {
		p.peak = p.active
	}
	p.mu.Unlock()
	defer func() { p.mu.Lock(); p.active--; p.mu.Unlock() }()
	p.entered <- struct{}{}
	select {
	case <-ctx.Done():
		return openrouter.ChatResponse{}, ctx.Err()
	case <-p.release:
	}
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", Content: "independent evidence"}}}}, nil
}
func configure(t *testing.T, f *fixture, client agent.Client) {
	t.Helper()
	f.supervisor.Configure(client, f.profile, func(ctx context.Context, receipt *composition.Receipt) (subagents.Composition, error) {
		var r plugins.ResolvedComposition
		var err error
		if receipt == nil {
			r, err = f.manager.ResolvePresetContext(ctx, plugins.ResearchPresetID)
		} else {
			r, err = f.manager.ResumeCompositionContext(ctx, *receipt)
		}
		return subagents.Composition{Receipt: r.Receipt, Toolset: r.Toolset, Instructions: plugins.ResearchInstructions}, err
	})
}
func TestForegroundBatchActuallyOverlapsWithinConfiguredCapacity(t *testing.T) {
	for _, limits := range []struct{ parent, runtime, want int }{{1, 3, 1}, {2, 3, 2}, {3, 2, 2}} {
		t.Run(fmt.Sprintf("parent%d-runtime%d", limits.parent, limits.runtime), func(t *testing.T) {
			policy := delegation.DefaultPolicy()
			policy.PerParent = limits.parent
			policy.Runtime = limits.runtime
			f := setup(t, policy)
			g := &gatedProvider{entered: make(chan struct{}, 4), release: make(chan struct{}, 4)}
			configure(t, f, g)
			done := make(chan error, 1)
			go func() {
				r, err := f.delegate(t, context.Background(), f.parent, []delegation.Assignment{{Key: "a", Objective: "a"}, {Key: "b", Objective: "b"}, {Key: "c", Objective: "c"}})
				if err == nil && len(r) != 3 {
					err = fmt.Errorf("results=%d", len(r))
				}
				done <- err
			}()
			for i := 0; i < limits.want; i++ {
				select {
				case <-g.entered:
				case <-time.After(gateTimeout):
					t.Fatal("independent children did not overlap")
				}
			}
			select {
			case <-g.entered:
				t.Fatal("capacity exceeded while providers are held open")
			case <-time.After(40 * time.Millisecond):
			}
			g.release <- struct{}{}
			select {
			case <-g.entered:
			case <-time.After(gateTimeout):
				t.Fatal("capacity was not released for queued child")
			}
			for i := 0; i < 3; i++ {
				g.release <- struct{}{}
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(gateTimeout):
				t.Fatal("batch did not finish")
			}
			g.mu.Lock()
			peak, calls := g.peak, g.calls
			g.mu.Unlock()
			if peak != limits.want || calls != 3 {
				t.Fatalf("measured peak=%d calls=%d", peak, calls)
			}
		})
	}
}

type clientFunc func(context.Context, openrouter.ChatRequest, openrouter.StreamHandlers) (openrouter.ChatResponse, error)

func (f clientFunc) ChatStream(c context.Context, r openrouter.ChatRequest, h openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	return f(c, r, h)
}
func response(content string) openrouter.ChatResponse {
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", Content: content}}}}
}
func TestBatchRetainsPartialFailureAndAcceptedResultsAfterCancellation(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	entered := make(chan struct{}, 1)
	accepted := make(chan struct{}, 2)
	var calls atomic.Int32
	configure(t, f, clientFunc(func(ctx context.Context, r openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		calls.Add(1)
		text := r.Messages[len(r.Messages)-1].Content
		if strings.Contains(text, "fail-source") {
			accepted <- struct{}{}
			return openrouter.ChatResponse{}, errors.New("provider unavailable")
		}
		if strings.Contains(text, "held-source") {
			entered <- struct{}{}
			<-ctx.Done()
			return openrouter.ChatResponse{}, ctx.Err()
		}
		accepted <- struct{}{}
		return response("accepted evidence"), nil
	}))
	requests := []delegation.Assignment{{Key: "success", Objective: "success-source"}, {Key: "fail", Objective: "fail-source"}, {Key: "held", Objective: "held-source"}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan []delegation.Result, 1)
	errs := make(chan error, 1)
	go func() { r, err := f.delegate(t, ctx, f.parent, requests); done <- r; errs <- err }()
	<-entered
	<-accepted
	<-accepted
	// Wait through the public seam until both completed siblings are durable.
	// The held child ensures cancellation still has unfinished work to stop.
	var successID string
	deadline := time.After(time.Second)
	for successID == "" {
		r, err := f.delegate(t, context.Background(), f.parent, requests[:2])
		if err != nil {
			t.Fatal(err)
		}
		if len(r) == 2 && r[0].Status == "succeeded" && r[1].Status == "failed" {
			successID = r[0].ExecutionID
			break
		}
		select {
		case <-deadline:
			t.Fatal("success not retained")
		default:
		}
	}
	cancel()
	var first []delegation.Result
	select {
	case first = <-done:
	case <-time.After(time.Second):
		t.Fatal("cancellation did not join workers")
	}
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	if first[0].Status != "succeeded" || first[1].Status != "failed" || first[2].Status != "cancelled" {
		t.Fatalf("partial outcomes: %+v", first)
	}
	retried, err := f.delegate(t, context.Background(), f.parent, requests)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 || retried[0].ExecutionID != successID || retried[1].Status != "failed" || retried[2].Status != "cancelled" {
		t.Fatalf("retry changed terminal outcomes: %+v calls=%d", retried, calls.Load())
	}
}

func TestChildModelCallAllowanceStopsRepeatedToolRequests(t *testing.T) {
	policy := delegation.DefaultPolicy()
	policy.ModelCalls = 2
	f := setup(t, policy)
	var calls atomic.Int32
	configure(t, f, clientFunc(func(_ context.Context, _ openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		n := calls.Add(1)
		return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", ToolCalls: []openrouter.ToolCall{{ID: fmt.Sprint(n), Type: "function", Function: openrouter.FunctionCall{Name: "read_file", Arguments: `{"path":"/etc/passwd"}`}}}}}}}, nil
	}))
	r, err := f.delegate(t, context.Background(), f.parent, []delegation.Assignment{{Key: "limit", Objective: "keep researching"}})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || r[0].Status != "failed" || r[0].Reason != "policy_limit" {
		t.Fatalf("model allowance: %+v calls=%d", r, calls.Load())
	}
}

func TestDuplicateWaiterCancellationDoesNotCancelOriginalChild(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	g := &gatedProvider{entered: make(chan struct{}, 2), release: make(chan struct{}, 2)}
	configure(t, f, g)
	requests := []delegation.Assignment{{Key: "shared", Objective: "research"}}
	done := make(chan error, 1)
	go func() { _, err := f.delegate(t, context.Background(), f.parent, requests); done <- err }()
	<-g.entered
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := f.delegate(t, ctx, f.parent, requests); err == nil {
		t.Fatal("timed out waiter succeeded")
	}
	g.release <- struct{}{}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	r, err := f.delegate(t, context.Background(), f.parent, requests)
	if err != nil {
		t.Fatal(err)
	}
	if r[0].Status != "succeeded" || g.calls != 1 {
		t.Fatalf("waiter cancelled healthy child: %+v", r)
	}
}

// Each public-supervisor request is backed by an actual fenced, committed intent.
func (f *fixture) delegate(t *testing.T, ctx context.Context, p delegation.Parent, requests []delegation.Assignment) ([]delegation.Result, error) {
	t.Helper()
	b, _ := json.Marshal(struct {
		Assignments []delegation.Assignment `json:"assignments"`
	}{requests})
	payload, _ := json.Marshal(memory.ToolIntentPayload{Call: memory.ToolCall{ID: "call-" + uuid.NewString(), Name: delegation.ToolName, Arguments: string(b)}})
	intent, err := f.store.AppendEventWithLease(ctx, p.Scope.SessionID, p.Lease.HolderID, p.Lease.FencingToken, memory.EventInput{Type: memory.EventToolIntent, ParentID: p.SourceEventID, ExecutionID: memory.ExecutionID(uuid.NewString()), Payload: payload})
	if err != nil {
		return nil, err
	}
	p.IntentEventID = intent.ID
	return f.supervisor.Delegate(ctx, p, requests)
}

type eventSink struct{ content []string }

func (e *eventSink) Delta(s string)                                { e.content = append(e.content, s) }
func (e *eventSink) Reasoning(string)                              {}
func (e *eventSink) ReasoningDone()                                {}
func (e *eventSink) AssistantDone(s string)                        { e.content = append(e.content, s) }
func (e *eventSink) ToolCall(string, string, string)               {}
func (e *eventSink) ToolResult(string, string, bool)               {}
func (e *eventSink) ResponseDiscarded(agent.DiscardReason, string) {}

func TestComposedParentDelegatesThroughPluginAndReceivesIsolatedEvidence(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	ctx := context.Background()
	resolved, err := f.manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := f.store.CreateGlobalSessionWithComposition(ctx, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	var parentCalls, childCalls atomic.Int32
	client := clientFunc(func(_ context.Context, r openrouter.ChatRequest, h openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		isParent := false
		for _, tool := range r.Tools {
			if tool.Function.Name == delegation.ToolName {
				isParent = true
			}
		}
		if isParent {
			if parentCalls.Add(1) == 1 {
				return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", ToolCalls: []openrouter.ToolCall{{ID: "research", Type: "function", Function: openrouter.FunctionCall{Name: delegation.ToolName, Arguments: `{"assignments":[{"idempotency_key":"source","objective":"research source","context":"selected-sentinel"}]}`}}}}}}}, nil
			}
			var results []delegation.Result
			for _, m := range r.Messages {
				if m.Role == "tool" && m.ToolCallID == "research" {
					if err := json.Unmarshal([]byte(m.Content), &results); err != nil {
						return openrouter.ChatResponse{}, err
					}
				}
			}
			if len(results) != 1 || results[0].Status != "succeeded" || !strings.Contains(results[0].Findings, "child evidence") {
				return openrouter.ChatResponse{}, fmt.Errorf("parent received no evidence: %+v", results)
			}
			return response("Owner-visible researched answer"), nil
		}
		childCall := childCalls.Add(1)
		encoded, _ := json.Marshal(r.Messages)
		if strings.Contains(string(encoded), "parent-private-sentinel") || !strings.Contains(string(encoded), "selected-sentinel") || len(r.Tools) != 2 || !strings.Contains(r.Messages[0].Content, plugins.ResearchInstructions) {
			return openrouter.ChatResponse{}, errors.New("child context or composition leaked")
		}
		if childCall == 1 {
			return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", ToolCalls: []openrouter.ToolCall{{ID: "search", Type: "function", Function: openrouter.FunctionCall{Name: "web_search", Arguments: `{"query":"independent source"}`}}}}}}}, nil
		}
		if !strings.Contains(string(encoded), "fake-web-evidence") {
			return openrouter.ChatResponse{}, errors.New("child did not receive selected Web evidence")
		}
		if h.OnContent != nil {
			h.OnContent("CHILD_STREAM_SENTINEL")
		}
		return response("child evidence https://example.com"), nil
	})
	configure(t, f, client)
	sink := &eventSink{}
	parent := agent.NewWithToolset(client, f.profile, f.store.BindHistory(stored.ID, "composed"), stored.ScopeContext(), f.store.BindTurnOwner(stored.ID, "composed"), resolved.Toolset)
	if err = parent.Send(ctx, "parent-private-sentinel", sink, nil); err != nil {
		t.Fatal(err)
	}
	if parentCalls.Load() != 2 || childCalls.Load() != 2 || strings.Contains(strings.Join(sink.content, " "), "CHILD_STREAM_SENTINEL") {
		t.Fatalf("conversation activity leaked: %+v", sink.content)
	}
	tasks, err := f.store.ListOpenGlobalTasks(ctx)
	if err != nil || len(tasks) != 0 {
		t.Fatalf("incidental delegation created Tasks: %+v %v", tasks, err)
	}
}

func taskContext(p delegation.Parent) context.Context {
	return task.WithMutationAttribution(context.Background(), task.MutationAttribution{ActorID: string(memory.LocalOwnerID), SessionID: string(p.Scope.SessionID), RunID: "orchestrator", LeaseHolderID: string(p.Lease.HolderID), LeaseToken: uint64(p.Lease.FencingToken), LeaseGeneration: uint64(p.Lease.Generation)})
}
func TestTaskAssociationLeavesClaimsAndCompletionWithOrchestrator(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	ctx := taskContext(f.parent)
	root, err := f.store.CreateGlobalTask(ctx, task.CreateInput{Title: "Compare independent sources", IdempotencyKey: "root"})
	if err != nil {
		t.Fatal(err)
	}
	tree, err := f.store.DecomposeGlobalTask(ctx, root.ID, task.DecomposeInput{ExpectedRevision: root.Revision, Children: []task.ChildInput{{Title: "source-a"}, {Title: "source-b"}}, IdempotencyKey: "children"})
	if err != nil {
		t.Fatal(err)
	}
	requests := []delegation.Assignment{{Key: "a", Objective: "source-a", TaskID: string(tree.Children[0].ID)}, {Key: "b", Objective: "source-b", TaskID: string(tree.Children[1].ID)}}
	results, err := f.delegate(t, context.Background(), f.parent, requests)
	if err != nil {
		t.Fatal(err)
	}
	for i, child := range tree.Children {
		current, err := f.store.GetGlobalTask(ctx, child.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Status != child.Status || current.Revision != child.Revision {
			t.Fatal("association mutated Task")
		}
		if _, found, err := f.store.GetGlobalTaskClaim(ctx, child.ID); err != nil || found {
			t.Fatalf("association claimed Task: %t %v", found, err)
		}
		childScope, err := f.store.GetSession(ctx, results[i].ChildSessionID)
		if err != nil {
			t.Fatal(err)
		}
		childCtx := task.WithMutationAttribution(context.Background(), task.MutationAttribution{ActorID: string(memory.LocalOwnerID), SessionID: string(childScope.ID), ParentSessionID: string(childScope.ParentSessionID), RunID: "child"})
		if _, err = f.store.GetGlobalTask(childCtx, child.ID); err == nil {
			t.Fatal("association granted child Task access")
		}
	}
	completed := task.StatusCompleted
	if _, err = f.store.UpdateGlobalTask(ctx, tree.Children[0].ID, task.UpdateInput{ExpectedRevision: tree.Children[0].Revision, Status: &completed, IdempotencyKey: "unclaimed"}); !errors.Is(err, task.ErrClaimRequired) {
		t.Fatalf("unclaimed update: %v", err)
	}
	for _, node := range append([]task.Task{tree.Parent}, tree.Children...) {
		if _, err = f.store.ClaimGlobalTask(ctx, node.ID, task.ClaimInput{IdempotencyKey: task.IdempotencyKey("claim-" + string(node.ID))}); err != nil {
			t.Fatal(err)
		}
	}
	other := f.newParent(t)
	if _, err = f.store.ClaimGlobalTask(taskContext(other), tree.Children[0].ID, task.ClaimInput{IdempotencyKey: "competing-claim"}); !errors.Is(err, task.ErrClaimHeld) {
		t.Fatalf("competing claim: %v", err)
	}
	if _, err = f.store.UpdateGlobalTask(ctx, root.ID, task.UpdateInput{ExpectedRevision: tree.Parent.Revision, Status: &completed, IdempotencyKey: "premature"}); !errors.Is(err, task.ErrActiveDescendants) {
		t.Fatalf("premature parent completion: %v", err)
	}
	for i, child := range tree.Children {
		summary := results[i].Findings
		if _, err = f.store.UpdateGlobalTask(ctx, child.ID, task.UpdateInput{ExpectedRevision: child.Revision, Status: &completed, ResultSummary: &summary, IdempotencyKey: task.IdempotencyKey("complete-" + string(child.ID))}); err != nil {
			t.Fatal(err)
		}
	}
	current, err := f.store.GetGlobalTask(ctx, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.UpdateGlobalTask(ctx, root.ID, task.UpdateInput{ExpectedRevision: current.Revision, Status: &completed, IdempotencyKey: "finish-root"}); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) newParent(t *testing.T) delegation.Parent {
	t.Helper()
	ctx := context.Background()
	resolved, err := f.manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := f.store.CreateGlobalSessionWithComposition(ctx, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := f.store.AcquireTurnLease(ctx, stored.ID, memory.LeaseHolderID(uuid.NewString()), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	root, err := f.store.AppendEventWithLease(ctx, stored.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "independent parent"})
	if err != nil {
		t.Fatal(err)
	}
	return delegation.Parent{Scope: stored.ScopeContext(), Lease: lease, SourceEventID: root.ID}
}
func TestRuntimeCapacityIsSharedAcrossParents(t *testing.T) {
	p := delegation.DefaultPolicy()
	p.PerParent = 2
	p.Runtime = 3
	f := setup(t, p)
	other := f.newParent(t)
	g := &gatedProvider{entered: make(chan struct{}, 8), release: make(chan struct{}, 8)}
	configure(t, f, g)
	done := make(chan error, 2)
	for _, parent := range []delegation.Parent{f.parent, other} {
		go func(parent delegation.Parent) {
			_, err := f.delegate(t, context.Background(), parent, []delegation.Assignment{{Key: "a", Objective: "a"}, {Key: "b", Objective: "b"}})
			done <- err
		}(parent)
	}
	for i := 0; i < 3; i++ {
		select {
		case <-g.entered:
		case <-time.After(gateTimeout):
			t.Fatal("runtime did not reach configured overlap")
		}
	}
	select {
	case <-g.entered:
		t.Fatal("runtime capacity exceeded across parents")
	case <-time.After(40 * time.Millisecond):
	}
	g.release <- struct{}{}
	select {
	case <-g.entered:
	case <-time.After(gateTimeout):
		t.Fatal("runtime slot not reused")
	}
	for i := 0; i < 3; i++ {
		g.release <- struct{}{}
	}
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	g.mu.Lock()
	peak := g.peak
	g.mu.Unlock()
	if peak != 3 {
		t.Fatalf("measured shared capacity=%d", peak)
	}
}
func TestAuthorityLossAndShutdownJoinForegroundChildren(t *testing.T) {
	for _, action := range []string{"parent_lease", "disable", "shutdown"} {
		t.Run(action, func(t *testing.T) {
			f := setup(t, delegation.DefaultPolicy())
			g := &gatedProvider{entered: make(chan struct{}, 4), release: make(chan struct{}, 4)}
			configure(t, f, g)
			done := make(chan []delegation.Result, 1)
			go func() {
				r, _ := f.delegate(t, context.Background(), f.parent, []delegation.Assignment{{Key: "a", Objective: "a"}, {Key: "b", Objective: "b"}})
				done <- r
			}()
			<-g.entered
			<-g.entered
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			switch action {
			case "parent_lease":
				if err := f.store.ReleaseTurnLease(ctx, f.parent.Scope.SessionID, f.parent.Lease.HolderID, f.parent.Lease.FencingToken); err != nil {
					t.Fatal(err)
				}
			case "disable":
				if err := f.manager.Disable(ctx, plugins.SubagentsPluginID); err != nil {
					t.Fatal(err)
				}
			case "shutdown":
				if err := f.supervisor.Stop(ctx); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case results := <-done:
				if len(results) != 2 {
					t.Fatalf("missing retained outcomes after %s: %+v", action, results)
				}
				for _, r := range results {
					if r.Status == "succeeded" {
						t.Fatalf("stale success after %s", action)
					}
					if action == "parent_lease" && (r.Status != "interrupted" || r.Reason != "authority_ended") {
						t.Fatalf("ownership loss reported as user cancellation: %+v", r)
					}
				}
			case <-ctx.Done():
				t.Fatal("children did not stop")
			}
			g.mu.Lock()
			active := g.active
			g.mu.Unlock()
			if active != 0 {
				t.Fatal("foreground child leaked")
			}
		})
	}
}

func TestConfiguredDeadlineContextAndResultLimits(t *testing.T) {
	t.Run("deadline", func(t *testing.T) {
		p := delegation.DefaultPolicy()
		p.Deadline = 80 * time.Millisecond
		f := setup(t, p)
		configure(t, f, clientFunc(func(ctx context.Context, _ openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
			<-ctx.Done()
			return openrouter.ChatResponse{}, ctx.Err()
		}))
		r, err := f.delegate(t, context.Background(), f.parent, []delegation.Assignment{{Key: "deadline", Objective: "research"}})
		if err != nil {
			t.Fatal(err)
		}
		if r[0].Status != "failed" || r[0].Reason != "deadline_limit" {
			t.Fatalf("deadline outcome: %+v", r)
		}
	})
	t.Run("request_context", func(t *testing.T) {
		p := delegation.DefaultPolicy()
		p.RequestBytes = 100
		f := setup(t, p)
		r, err := f.delegate(t, context.Background(), f.parent, []delegation.Assignment{{Key: "context", Objective: "research"}})
		if err != nil {
			t.Fatal(err)
		}
		if r[0].Reason != "policy_limit" || f.client.calls != 0 {
			t.Fatalf("oversized context reached provider: %+v", r)
		}
	})
	t.Run("result", func(t *testing.T) {
		p := delegation.DefaultPolicy()
		p.ResultBytes = 512
		f := setup(t, p)
		configure(t, f, clientFunc(func(context.Context, openrouter.ChatRequest, openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
			return response(strings.Repeat("évidence ", 800)), nil
		}))
		r, err := f.delegate(t, context.Background(), f.parent, []delegation.Assignment{{Key: "result", Objective: "research"}})
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(r[0])
		if len(b) > 512 || !utf8.ValidString(r[0].Findings) || len(r[0].Limitations) == 0 {
			t.Fatalf("unbounded result %d bytes %+v", len(b), r[0])
		}
	})
	t.Run("invalid_policy", func(t *testing.T) {
		p := delegation.DefaultPolicy()
		p.PerParent = 0
		f := setup(t, delegation.DefaultPolicy())
		if _, err := subagents.New(f.store, p); err == nil {
			t.Fatal("unbounded policy accepted")
		}
	})
}

func TestWorkspaceAdmissionExplainsMissingReviewedPresetAllowance(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	ctx := context.Background()
	resolved, err := f.manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := f.store.RegisterWorkspace(ctx, "Research workspace")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := f.store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := f.store.AcquireTurnLease(ctx, stored.ID, "workspace-parent", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	root, err := f.store.AppendEventWithLease(ctx, stored.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "research"})
	if err != nil {
		t.Fatal(err)
	}
	parent := delegation.Parent{Scope: stored.ScopeContext(), Lease: lease, SourceEventID: root.ID}
	_, err = f.delegate(t, ctx, parent, []delegation.Assignment{{Key: "workspace", Objective: "research"}})
	if err == nil || !strings.Contains(err.Error(), "reviewed") || f.client.calls != 0 {
		t.Fatalf("Workspace admission: %v", err)
	}
}

// Replace only the Web execution boundary; use the actual manifest and schemas.
type researchWeb struct{ plugins.Web }

func (researchWeb) ToolCapabilities() []plugins.ToolCapability {
	capabilities := plugins.NewWeb().ToolCapabilities()
	for i := range capabilities {
		capabilities[i].Tool.Execute = func(context.Context, string) (string, error) { return "fake-web-evidence https://example.com", nil }
	}
	return capabilities
}

func TestOldAndNewParentReceiptsReopenWithoutChangingDelegation(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	ctx := context.Background()
	current, err := f.manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := f.manager.ResumeComposition(current.Receipt)
	if err != nil {
		t.Fatalf("new parent cannot reopen: %v", err)
	}
	hasDelegate := func(r plugins.ResolvedComposition) bool {
		for _, schema := range r.Toolset.Schemas() {
			if schema.Function.Name == delegation.ToolName {
				return true
			}
		}
		return false
	}
	if !hasDelegate(reopened) {
		t.Fatal("new receipt lost delegation")
	}
	if err = f.manager.Disable(ctx, plugins.SubagentsPluginID); err != nil {
		t.Fatal(err)
	}
	before, err := f.manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	before.Receipt.Preset.Version = "sha256:3c812f0838e55608076db195ca47ae01bc434896fefb190b98e7ff17eb0c8e87"
	warnings := before.Receipt.Warnings[:0]
	for _, w := range before.Receipt.Warnings {
		if w.CapabilityID != delegation.CapabilityID {
			warnings = append(warnings, w)
		}
	}
	before.Receipt.Warnings = warnings
	if err = f.manager.Enable(ctx, plugins.SubagentsPluginID); err != nil {
		t.Fatal(err)
	}
	old, err := f.manager.ResumeComposition(before.Receipt)
	if err != nil {
		t.Fatalf("old parent cannot reopen: %v", err)
	}
	if hasDelegate(old) {
		t.Fatal("old receipt gained delegation")
	}
}

func TestChildOutputLimitIsAppliedBeforeResponsesSerialization(t *testing.T) {
	p := delegation.DefaultPolicy()
	p.OutputTokens = 128
	f := setup(t, p)
	profile, err := openrouter.NewExplicitContextProfile(openrouter.AstraModel, 131072, 65536, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f.profile = profile
	var serializedLimit int64
	configure(t, f, clientFunc(func(_ context.Context, r openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		b, err := openrouter.RequestBytes(r)
		if err != nil {
			return openrouter.ChatResponse{}, err
		}
		var body struct {
			MaxOutputTokens int64 `json:"max_output_tokens"`
		}
		if err = json.Unmarshal(b, &body); err != nil {
			return openrouter.ChatResponse{}, err
		}
		serializedLimit = body.MaxOutputTokens
		return response("bounded evidence"), nil
	}))
	r, err := f.delegate(t, context.Background(), f.parent, []delegation.Assignment{{Key: "astra-limit", Objective: "research"}})
	if err != nil {
		t.Fatal(err)
	}
	if r[0].Status != "succeeded" || serializedLimit != 128 {
		t.Fatalf("serialized output limit=%d outcome=%+v", serializedLimit, r)
	}
}
func TestFailedProviderCallMakesIncompleteUsageUnknown(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	var calls int
	configure(t, f, clientFunc(func(context.Context, openrouter.ChatRequest, openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		calls++
		if calls == 2 {
			return openrouter.ChatResponse{}, errors.New("provider failure")
		}
		measured := int64(40)
		return openrouter.ChatResponse{Usage: &openrouter.TokenUsage{TotalTokens: &measured}, Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", ToolCalls: []openrouter.ToolCall{{ID: "search", Type: "function", Function: openrouter.FunctionCall{Name: "web_search", Arguments: `{"query":"source"}`}}}}}}}, nil
	}))
	r, err := f.delegate(t, context.Background(), f.parent, []delegation.Assignment{{Key: "usage", Objective: "research"}})
	if err != nil {
		t.Fatal(err)
	}
	if r[0].Status != "failed" || r[0].Usage != nil {
		t.Fatalf("incomplete usage reported as complete: %+v", r)
	}
}

func TestProviderFailuresRemainDistinctAndSafeAcrossBatchRetry(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	var calls atomic.Int32
	configure(t, f, clientFunc(func(_ context.Context, r openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		calls.Add(1)
		assignment := r.Messages[len(r.Messages)-1].Content
		if strings.Contains(assignment, "transport-failure") {
			return openrouter.ChatResponse{}, errors.New("upstream rejected secret-sentinel")
		}
		if strings.Contains(assignment, "invalid-response") {
			return openrouter.ChatResponse{}, nil
		}
		return response("accepted independent evidence"), nil
	}))
	requests := []delegation.Assignment{{Key: "transport", Objective: "transport-failure"}, {Key: "invalid", Objective: "invalid-response"}, {Key: "success", Objective: "research"}}
	first, err := f.delegate(t, context.Background(), f.parent, requests)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 3 || first[0].Reason != "provider_failure" || first[1].Reason != "provider_response_invalid" || first[2].Status != "succeeded" {
		t.Fatalf("failure classification or sibling result: %+v", first)
	}
	retained, err := f.delegate(t, context.Background(), f.parent, requests)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(first)
	after, _ := json.Marshal(retained)
	if string(before) != string(after) || strings.Contains(string(after), "secret-sentinel") || calls.Load() != 3 {
		t.Fatalf("retry changed or exposed failure: %s calls=%d", after, calls.Load())
	}
}

func TestChildComposedContextOverflowIsAPolicyFailure(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	var err error
	f.profile, err = openrouter.NewExplicitContextProfile("test-model", 8192, 4300, 128)
	if err != nil {
		t.Fatal(err)
	}
	configure(t, f, f.client)
	r, err := f.delegate(t, context.Background(), f.parent, []delegation.Assignment{{Key: "overflow", Objective: "research"}})
	if err != nil {
		t.Fatal(err)
	}
	if r[0].Status != "failed" || r[0].Reason != "policy_limit" || f.client.calls != 0 {
		t.Fatalf("context overflow classification: %+v calls=%d", r, f.client.calls)
	}
}

func TestChildPersistenceFailureIsRetainedAsInfrastructure(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	_, err := f.db.Exec(`CREATE TRIGGER fail_child_context BEFORE INSERT ON events
 WHEN NEW.event_type='context_snapshot'
 AND (SELECT parent_session_id FROM sessions WHERE id=NEW.session_id) IS NOT NULL
 BEGIN SELECT RAISE(ABORT, 'injected storage failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	requests := []delegation.Assignment{{Key: "storage", Objective: "research"}}
	first, err := f.delegate(t, context.Background(), f.parent, requests)
	if err != nil {
		t.Fatal(err)
	}
	if first[0].Status != "failed" || first[0].Reason != "infrastructure_failure" || f.client.calls != 0 {
		t.Fatalf("infrastructure outcome: %+v calls=%d", first, f.client.calls)
	}
	if _, err = f.db.Exec(`DROP TRIGGER fail_child_context`); err != nil {
		t.Fatal(err)
	}
	retained, err := f.delegate(t, context.Background(), f.parent, requests)
	if err != nil {
		t.Fatal(err)
	}
	if retained[0].ExecutionID != first[0].ExecutionID || retained[0].Reason != "infrastructure_failure" || f.client.calls != 0 {
		t.Fatalf("retry reran infrastructure failure: %+v", retained)
	}
}
