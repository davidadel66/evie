package memorybackground

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

// scriptedModel is the deterministic fake model: each call takes the next
// scripted reply for its purpose ("extract" or "decide") and records the
// exact request, so tests can prove what the pass sent. No paid call is ever
// made in tests.
type scriptedModel struct {
	mu       sync.Mutex
	replies  map[string][]scriptedReply
	requests []ModelRequest
}

type scriptedReply struct {
	text  string
	err   error
	usage [2]int64
	wait  chan struct{}
}

func newScriptedModel() *scriptedModel {
	return &scriptedModel{replies: map[string][]scriptedReply{}}
}

func (m *scriptedModel) Identity() string { return "fake:scripted" }

func (m *scriptedModel) script(purpose string, replies ...scriptedReply) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.replies[purpose] = append(m.replies[purpose], replies...)
}

func (m *scriptedModel) extract(memories ...map[string]any) {
	if memories == nil {
		memories = []map[string]any{}
	}
	encoded, _ := json.Marshal(map[string]any{"memories": memories})
	m.script("extract", scriptedReply{text: string(encoded), usage: [2]int64{900, 120}})
}

func (m *scriptedModel) decide(decisions ...map[string]any) {
	encoded, _ := json.Marshal(map[string]any{"decisions": decisions})
	m.script("decide", scriptedReply{text: string(encoded), usage: [2]int64{600, 80}})
}

func (m *scriptedModel) Complete(ctx context.Context, request ModelRequest) (ModelResponse, error) {
	m.mu.Lock()
	m.requests = append(m.requests, request)
	queue := m.replies[request.Purpose]
	if len(queue) == 0 {
		m.mu.Unlock()
		return ModelResponse{}, fmt.Errorf("no scripted %s reply", request.Purpose)
	}
	reply := queue[0]
	m.replies[request.Purpose] = queue[1:]
	m.mu.Unlock()
	if reply.wait != nil {
		select {
		case <-reply.wait:
		case <-ctx.Done():
			return ModelResponse{}, ctx.Err()
		}
	}
	if reply.err != nil {
		return ModelResponse{}, reply.err
	}
	return ModelResponse{Text: reply.text, InputTokens: reply.usage[0], OutputTokens: reply.usage[1], UsageReported: true}, nil
}

func (m *scriptedModel) calls(purpose string) []ModelRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	var matched []ModelRequest
	for _, request := range m.requests {
		if purpose == "" || request.Purpose == purpose {
			matched = append(matched, request)
		}
	}
	return matched
}

func memoryItem(message int, kind, applies, predicate, label, value string) map[string]any {
	return map[string]any{"message": message, "kind": kind, "applies": applies, "predicate": predicate, "label": label,
		"cardinality": "one", "value_type": "text", "value": value, "polarity": "affirmed"}
}

func decisionItem(candidate int, op, target, mode string) map[string]any {
	return map[string]any{"candidate": candidate, "op": op, "target": target, "mode": mode}
}

type fixture struct {
	t      *testing.T
	ctx    context.Context
	path   string
	db     *sql.DB
	store  *eviedb.Store
	model  *scriptedModel
	runner *Runner
	offset time.Duration
	config Config
	slept  []time.Duration
}

func testConfig() Config {
	c := Defaults()
	c.Enabled, c.Provider, c.Model = true, ProviderOpenRouter, "fake/scripted"
	c.CommitAttempts, c.CommitWait = 2, time.Millisecond
	return c
}

func newFixture(t *testing.T) *fixture {
	return newFixtureWith(t, testConfig())
}

func newFixtureWith(t *testing.T, config Config) *fixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	f := &fixture{t: t, ctx: context.Background(), path: path, db: db, store: eviedb.NewStore(db), model: newScriptedModel(), config: config}
	f.runner = NewRunner(f.store, f.model, config, WithClock(func() time.Time { return time.Now().Add(f.offset) }),
		WithSleep(func(ctx context.Context, d time.Duration) error { f.slept = append(f.slept, d); return ctx.Err() }),
		WithPricing(Pricing{InputPerMillion: 0.3, OutputPerMillion: 1.2, Source: "test"}))
	if _, err := f.store.EnsureMemoryBackgroundActivation(f.ctx); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) globalSession() memory.Session {
	f.t.Helper()
	session, err := f.store.CreateGlobalSession(f.ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	return session
}

func (f *fixture) workspaceSession(name string) memory.Session {
	f.t.Helper()
	workspace, err := f.store.RegisterWorkspace(f.ctx, name)
	if err != nil {
		f.t.Fatal(err)
	}
	session, err := f.store.CreateWorkspaceSessionWithComposition(f.ctx, workspace.ID, workspace.CurrentRevisionID, fixtureReceipt())
	if err != nil {
		f.t.Fatal(err)
	}
	return session
}

func fixtureReceipt() composition.Receipt {
	const hash = "0000000000000000000000000000000000000000000000000000000000000000"
	return composition.Receipt{
		FormatVersion: composition.FormatVersion,
		Preset:        composition.PresetIdentity{ID: "standard", Version: "sha256:" + hash},
		EvieVersion:   "1.0.0",
		Providers:     []composition.Provider{{ID: "fixture", ImplementationVersion: "1.0.0"}},
		Capabilities:  []composition.Capability{{ID: "fixture.echo", ProviderID: "fixture", ContractVersion: "1.0.0", SchemaSHA256: hash}},
		ToolSchemas:   []composition.ToolSchema{{Name: "fixture_echo", SHA256: hash}},
		Instructions:  []composition.InstructionReference{{ID: "fixture-instructions", SHA256: hash}},
	}
}

// turn appends one owner message and, optionally, an assistant reply, as a
// finished turn: the lease is released afterwards.
func (f *fixture) turn(session memory.Session, owner string, assistant ...string) memory.Event {
	f.t.Helper()
	lease, err := f.store.AcquireTurnLease(f.ctx, session.ID, "fixture-turn", time.Minute)
	if err != nil {
		f.t.Fatal(err)
	}
	said, err := f.store.AppendEventWithLease(f.ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: owner})
	if err != nil {
		f.t.Fatal(err)
	}
	for _, reply := range assistant {
		if _, err := f.store.AppendEventWithLease(f.ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
			ParentID: said.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: reply}); err != nil {
			f.t.Fatal(err)
		}
	}
	if err := f.store.ReleaseTurnLease(f.ctx, session.ID, lease.HolderID, lease.FencingToken); err != nil {
		f.t.Fatal(err)
	}
	return said
}

// run runs one pass over the conversation as an idle trigger would.
func (f *fixture) run(session memory.Session) (memory.BackgroundRun, error) {
	f.t.Helper()
	return f.runner.runSession(f.ctx, session.ID, memory.BackgroundTriggerIdle, 0)
}

func (f *fixture) mustRun(session memory.Session) memory.BackgroundRun {
	f.t.Helper()
	run, err := f.run(session)
	if err != nil {
		f.t.Fatalf("pass failed: %v (run %+v)", err, run)
	}
	if run.Status != memory.BackgroundRunSucceeded {
		f.t.Fatalf("pass did not succeed: %+v", run)
	}
	return run
}

func (f *fixture) inbox() memory.MemoryProposalPage {
	f.t.Helper()
	page, err := f.store.ListMemoryProposals(f.ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	return page
}

func (f *fixture) approve(item memory.MemoryProposalItem, alwaysApplies bool) memory.MemoryProposalResult {
	f.t.Helper()
	result, err := f.store.ApproveMemoryProposal(f.ctx, memory.MemoryProposalApproval{ProposalID: item.ProposalID, PreparedSHA256: item.PreparedSHA256, AlwaysApplies: alwaysApplies})
	if err != nil {
		f.t.Fatalf("approve %s: %v", item.ToolName, err)
	}
	return result
}

// approveEverywhere approves a proposal's Global version, as "Save
// everywhere instead" does, with its default "Always applies".
func (f *fixture) approveEverywhere(item memory.MemoryProposalItem) memory.MemoryProposalResult {
	f.t.Helper()
	if item.Everywhere == nil {
		f.t.Fatalf("proposal %s has no Global version", item.ProposalID)
	}
	result, err := f.store.ApproveMemoryProposal(f.ctx, memory.MemoryProposalApproval{ProposalID: item.ProposalID,
		PreparedSHA256: item.Everywhere.PreparedSHA256, AlwaysApplies: true, Everywhere: true})
	if err != nil {
		f.t.Fatalf("approve everywhere: %v", err)
	}
	return result
}

// remember saves one owner memory through the background pass and an inbox
// approval, the way David would.
func (f *fixture) remember(session memory.Session, message, predicate, label, value string, kind memory.ClaimKind) memory.MemoryProposalResult {
	f.t.Helper()
	f.turn(session, message)
	applies := "profile"
	if kind == memory.ClaimKindWorkspaceBrief {
		applies = "workspace"
	}
	f.model.extract(memoryItem(0, string(kind), applies, predicate, label, value))
	f.mustRun(session)
	page := f.inbox()
	for _, item := range page.Items {
		if item.Reason == memory.MemoryProposalBackground && strings.Contains(item.Proposal, `"value":"`+value+`"`) {
			return f.approve(item, memory.DefaultStanding(kind))
		}
	}
	f.t.Fatalf("no proposal for %s in %+v", value, page.Items)
	return memory.MemoryProposalResult{}
}

func (f *fixture) cursor(session memory.Session) int64 {
	f.t.Helper()
	batch, err := f.store.MemoryBackgroundBatch(f.ctx, session.ID, memory.BackgroundBatchLimits{MaxMessages: 1, MaxBytes: 1})
	if err != nil {
		f.t.Fatal(err)
	}
	return batch.FromSequence
}

func (f *fixture) claimTexts(session memory.Session) []string {
	f.t.Helper()
	pool, err := f.store.MemoryBackgroundPool(f.ctx, session.ScopeContext())
	if err != nil {
		f.t.Fatal(err)
	}
	var texts []string
	for _, item := range pool.Memories {
		if !item.Pending {
			texts = append(texts, item.ScopeKey+" "+item.Text)
		}
	}
	return texts
}

func backgroundItems(page memory.MemoryProposalPage) []memory.MemoryProposalItem {
	var items []memory.MemoryProposalItem
	for _, item := range page.Items {
		if item.Reason == memory.MemoryProposalBackground {
			items = append(items, item)
		}
	}
	return items
}

var errScripted = errors.New("scripted provider failure")

// resetActivationForTest models a database that predates the pass: no
// activation row and no cursors.
func resetActivationForTest(f *fixture) error {
	_, err := f.db.ExecContext(f.ctx, `DELETE FROM memory_background_cursors; DELETE FROM memory_background_state;`)
	return err
}
