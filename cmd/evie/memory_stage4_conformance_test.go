package main

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/davidadel66/evie/internal/web"
)

const stage4ConformanceVersion = "memory-stage-4-conformance-v1"

type stage4ConformanceFixture struct {
	t            *testing.T
	ctx          context.Context
	db           *sql.DB
	path         string
	store        *eviedb.Store
	session      memory.Session
	seed         memory.RememberLiteralProposal
	generation   memory.CompilerGeneration
	generationID string
	modelCalls   atomic.Int64
}

func newStage4ConformanceFixture(t *testing.T) *stage4ConformanceFixture {
	t.Helper()
	f := &stage4ConformanceFixture{t: t, ctx: context.Background(), path: filepath.Join(t.TempDir(), "stage4.db"), generation: reviewCLIGeneration()}
	var err error
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.db.Close() })
	f.store = eviedb.NewStore(f.db)
	f.session, err = f.store.CreateGlobalSession(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	f.generationID, _, err = memory.CompilerGenerationIdentity(f.generation)
	if err != nil {
		t.Fatal(err)
	}
	f.seedLiteral("tea", 1)
	return f
}
func (f *stage4ConformanceFixture) seedLiteral(value string, key int) {
	f.t.Helper()
	lease, err := f.store.AcquireTurnLease(f.ctx, f.session.ID, "stage4-seed", time.Minute)
	if err != nil {
		f.t.Fatal(err)
	}
	event, err := f.store.AppendEventWithLease(f.ctx, f.session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "/remember drink " + value})
	if err != nil {
		f.t.Fatal(err)
	}
	f.seed, err = f.store.PrepareRememberLiteral(f.ctx, f.session.ScopeContext(), memory.RememberLiteralRequest{IdempotencyKey: stage4Key(key), SourceEventID: event.ID, Predicate: "drink", PredicateLabel: "drink", Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: value}})
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err = f.store.ApplyRememberLiteral(f.ctx, lease, f.seed); err != nil {
		f.t.Fatal(err)
	}
	if err = f.store.ReleaseTurnLease(f.ctx, f.session.ID, lease.HolderID, lease.FencingToken); err != nil {
		f.t.Fatal(err)
	}
}
func stage4Key(n int) string { return fmt.Sprintf("idem:v1:14900000-0000-4000-8000-%012d", n) }
func stage4Scope(session memory.Session) string {
	if session.WorkspaceID != "" {
		return "workspace:" + string(session.WorkspaceID)
	}
	if session.ProjectID != "" {
		return "project:" + string(session.ProjectID)
	}
	return "global"
}
func (f *stage4ConformanceFixture) reopen() {
	f.t.Helper()
	if err := f.db.Close(); err != nil {
		f.t.Fatal(err)
	}
	var err error
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		f.t.Fatal(err)
	}
	f.store = eviedb.NewStore(f.db)
}
func (f *stage4ConformanceFixture) closeSource() {
	f.t.Helper()
	if _, err := f.db.Exec(`UPDATE sessions SET status='closed' WHERE id=?`, f.session.ID); err != nil {
		f.t.Fatal(err)
	}
}
func (f *stage4ConformanceFixture) authority(scope string) eviedb.OwnerReviewContext {
	f.t.Helper()
	a, err := f.store.LocalOwnerReviewContext(f.ctx, scope)
	if err != nil {
		f.t.Fatal(err)
	}
	return a
}

type stage4ConversationClient struct{ calls *atomic.Int64 }

func (c stage4ConversationClient) ChatStream(context.Context, openrouter.ChatRequest, openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	c.calls.Add(1)
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", Content: "Recorded in this conversation."}}}}, nil
}
func (f *stage4ConformanceFixture) foreground(content string) memory.CompilationSelection {
	f.t.Helper()
	runtime := agent.NewWithToolset(stage4ConversationClient{&f.modelCalls}, evieTestContextProfile("stage4-scripted"), f.store.BindHistory(f.session.ID, "stage4-foreground"), f.session.ScopeContext(), f.store.BindTurnOwner(f.session.ID, "stage4-foreground"), tools.NewToolset(nil))
	done := make(chan struct{})
	var out bytes.Buffer
	go func() {
		defer close(done)
		runREPLContextIO(f.ctx, runtime, bufio.NewScanner(strings.NewReader(content+"\n")), &out)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		f.t.Fatal("foreground waited for unrelated extraction")
	}
	if !strings.Contains(out.String(), "Recorded in this conversation.") {
		f.t.Fatalf("REPL did not finalize: %s", out.String())
	}
	events, err := f.store.LoadEvents(f.ctx, f.session.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	var root memory.Event
	for _, event := range events {
		if event.Type == memory.EventUserMessage {
			root = event
		}
	}
	if root.Content != content || events[len(events)-1].Type != memory.EventAssistantMessage {
		f.t.Fatal("foreground terminal event did not commit")
	}
	return memory.CompilationSelection{SessionID: f.session.ID, RootID: root.ID, Cutoff: events[len(events)-1].Sequence, Destination: stage4Scope(f.session)}
}

type stage4ScriptedExtractor struct {
	subject, predicate memory.SemanticID
	value              string
	calls              atomic.Int64
	run                func(context.Context, memory.CompilerRequest) (eviedb.CompilerExtraction, error)
}

func (*stage4ScriptedExtractor) ServerIdentity() string { return "scripted:stage4-conformance" }
func (*stage4ScriptedExtractor) VerifyCompilerConfiguration(context.Context, memory.CompilerGeneration) error {
	return nil
}
func (e *stage4ScriptedExtractor) Extract(ctx context.Context, _ memory.CompilerGeneration, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
	e.calls.Add(1)
	if e.run != nil {
		return e.run(ctx, r)
	}
	return stage4CandidateOutput(r, e.subject, e.predicate, e.value), nil
}
func stage4CandidateOutput(r memory.CompilerRequest, subject, predicate memory.SemanticID, value string) eviedb.CompilerExtraction {
	candidates := []memory.ExtractorCandidate{}
	if value != "" {
		var source memory.EvidenceLocator
		for _, s := range r.Window.Sources {
			if s.Usage == "new_support" && s.Authority == memory.AuthorityOwnerStatement {
				source = s.Locator
				break
			}
		}
		candidates = append(candidates, memory.ExtractorCandidate{Proposition: memory.ClaimProposition{SubjectEntityID: subject, PredicateID: predicate, Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: value}}, Polarity: memory.PolarityAffirmed}, Support: []memory.EvidenceLocator{source}, Context: []memory.EvidenceLocator{}})
	}
	raw, _ := json.Marshal(memory.CompilerResponse{RequestID: r.ID, Candidates: candidates})
	return eviedb.CompilerExtraction{Raw: raw, ReleaseEvidence: "completed"}
}
func (f *stage4ConformanceFixture) extractor(value string) *stage4ScriptedExtractor {
	return &stage4ScriptedExtractor{subject: f.seed.Subject.ID, predicate: f.seed.Predicate.ID, value: value}
}
func (f *stage4ConformanceFixture) config(e eviedb.CompilerExtractor) eviedb.CompilerSupervisorConfig {
	return eviedb.CompilerSupervisorConfig{Extractors: map[string]eviedb.CompilerExtractor{f.generationID: e}}
}
func (f *stage4ConformanceFixture) compile(selection memory.CompilationSelection, g memory.CompilerGeneration, e eviedb.CompilerExtractor) memory.Compilation {
	f.t.Helper()
	c, err := f.store.CompileCandidateUnit(f.ctx, f.session.ScopeContext(), selection, g, e)
	if err != nil {
		f.t.Fatalf("compile %s/%s: %v", c.State, c.Reason, err)
	}
	return c
}
func (f *stage4ConformanceFixture) inspectCandidate(c memory.Compilation, scope string) memory.OwnerCandidate {
	f.t.Helper()
	if len(c.Candidates) != 1 {
		f.t.Fatalf("candidate count %d state %s", len(c.Candidates), c.State)
	}
	item, err := f.store.InspectOwnerCandidate(f.ctx, f.authority(scope), c.Candidates[0].ID)
	if err != nil {
		f.t.Fatal(err)
	}
	return item
}
func stage4CLI(t *testing.T, store *eviedb.Store, target any, args ...string) {
	t.Helper()
	var out bytes.Buffer
	handled, err := runOwnerReviewManagement(context.Background(), append([]string{"memory-review"}, args...), &out, store)
	if !handled || err != nil {
		t.Fatalf("CLI %v: %v", args, err)
	}
	if err = json.Unmarshal(out.Bytes(), target); err != nil {
		t.Fatal(err)
	}
}
func stage4HTTP(t *testing.T, server *httptest.Server, route string, input any, status int, target any) []byte {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/memory/candidates/"+route, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", server.URL)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status {
		t.Fatalf("HTTP %s got %d want %d: %s", route, response.StatusCode, status, body)
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("review response could be cached")
	}
	if target != nil {
		if err = json.Unmarshal(body, target); err != nil {
			t.Fatal(err)
		}
	}
	return body
}
func stage4Evidence(t *testing.T, scenario string, details map[string]any) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"version": stage4ConformanceVersion, "scenario": scenario, "details": details})
	if err != nil {
		t.Fatal(err)
	}
	t.Log("STAGE4_EVIDENCE " + string(raw))
}

func TestStage4ForegroundRestartClosedReviewReplay(t *testing.T) {
	f := newStage4ConformanceFixture(t)
	extractor := f.extractor("café")
	started := make(chan struct{})
	extractor.run = func(ctx context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		close(started)
		<-ctx.Done()
		return eviedb.CompilerExtraction{ReleaseEvidence: "completed"}, ctx.Err()
	}
	activation, err := f.store.ActivateCompiler(f.ctx, f.session.ScopeContext(), memory.CompilerActivationRequest{RequestID: "stage4-live", Selector: memory.CompilerLiveSelector{SourceScope: "global", Destination: "global", SessionID: f.session.ID}}, f.generation, extractor)
	if err != nil {
		t.Fatal(err)
	}
	selected := f.foreground("I prefer café as my standing drink.")
	var queued memory.Compilation
	for range 12 {
		if _, err = f.store.ReconcileCompilerEvidence(f.ctx, f.config(extractor)); err != nil {
			t.Fatal(err)
		}
		status, err := f.store.InspectCompilerActivations(f.ctx, f.session.ScopeContext())
		if err != nil {
			t.Fatal(err)
		}
		if len(status.Roots) > 0 && status.Roots[0].SelectionID != "" {
			queued, err = f.store.InspectCompilation(f.ctx, f.session.ScopeContext(), status.Roots[0].SelectionID)
			if err != nil {
				t.Fatal(err)
			}
			if queued.State == "queued" {
				break
			}
		}
	}
	if queued.JobID == "" || queued.Window.Selection.RootID != selected.RootID {
		t.Fatalf("activation did not select foreground root %+v", queued)
	}
	workCtx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	workerDone := make(chan error, 1)
	go func() { _, err := f.store.RunCompilerStep(workCtx, f.config(extractor)); workerDone <- err }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("selected worker never entered extractor")
	}
	later := f.foreground("The foreground conversation remains available while extraction waits.")
	if extractor.calls.Load() != 1 || f.modelCalls.Load() != 2 {
		t.Fatal("background stall altered foreground dispatch")
	}
	foreground, err := f.store.InspectOwnerCompilerDiagnostics(f.ctx, f.authority("global"), memory.CompilerDiagnosticsQuery{SessionID: f.session.ID, View: "foreground"})
	if err != nil {
		t.Fatal(err)
	}
	if len(foreground.Foreground) != 2 {
		t.Fatalf("missing actual host boundaries %+v", foreground)
	}
	for _, measurement := range foreground.Foreground {
		if measurement.TerminalCommittedAtUnixMS == nil || measurement.ResponseFinalizedAtUnixMS == nil || measurement.TerminalCommitNanos == nil || measurement.ResponseFinalizationNanos == nil || *measurement.ResponseFinalizationNanos < *measurement.TerminalCommitNanos {
			t.Fatalf("missing/distorted foreground observation %+v", measurement)
		}
	}
	cancel()
	select {
	case err = <-workerDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("shutdown %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("worker shutdown did not release local client")
	}
	before, err := f.store.InspectCompilation(f.ctx, f.session.ScopeContext(), queued.JobID)
	if err != nil || before.State != "retry_wait" || before.Attempts != 1 {
		t.Fatalf("durable shutdown %+v %v", before, err)
	}
	frozenWindow := before.Window
	f.reopen()
	// Control only the already documented retry clock; the sealed work, attempt,
	// capacity disposition and selection remain durable across the public reopen.
	if _, err = f.db.Exec(`UPDATE memory_compiler_jobs SET retry_at=0 WHERE job_id=?`, queued.JobID); err != nil {
		t.Fatal(err)
	}
	recoveredExtractor := f.extractor("café")
	if worked, err := f.store.RunCompilerStep(f.ctx, f.config(recoveredExtractor)); !worked || err != nil {
		t.Fatalf("recovery %v %v", worked, err)
	}
	compiled, err := f.store.InspectCompilation(f.ctx, f.session.ScopeContext(), queued.JobID)
	if err != nil || compiled.State != "completed_candidates" || compiled.Attempts != 2 || !reflect.DeepEqual(compiled.Window, frozenWindow) {
		t.Fatalf("recovery changed sealed work %+v %v", compiled, err)
	}
	acceptedBefore, err := f.store.InspectLiteralClaims(f.ctx, f.session.ScopeContext())
	if err != nil || len(acceptedBefore.Claims) != 1 || acceptedBefore.Claims[0].Literal.Value != "tea" {
		t.Fatalf("candidate escaped isolation %+v %v", acceptedBefore, err)
	}
	f.closeSource()
	item := f.inspectCandidate(compiled, "global")
	var cliItem, httpItem memory.OwnerCandidate
	stage4CLI(t, f.store, &cliItem, "inspect", "--scope", "global", "--id", item.Ref.ID)
	server := httptest.NewServer(web.WithCandidateReview(web.NewServer(nil), f.store).Handler())
	defer server.Close()
	stage4HTTP(t, server, "inspect", map[string]any{"scope_key": "global", "id": item.Ref.ID}, 200, &httpItem)
	if !reflect.DeepEqual(item, cliItem) || !reflect.DeepEqual(item, httpItem) {
		t.Fatal("Kernel/CLI/HTTP candidate evidence mismatch")
	}
	var preview memory.ReviewPreview
	stage4HTTP(t, server, "prepare", map[string]any{"scope_key": "global", "candidate": item.Ref, "action": "accept"}, 200, &preview)
	if preview.ScopeKey != "global" || preview.Effect == nil || len(preview.Effect.Claims) != 1 || len(preview.Effect.Claims[0].Sources) != 1 {
		t.Fatalf("wrong full effect %+v", preview)
	}
	source := preview.Effect.Claims[0].Sources[0]
	if source.EventID != selected.RootID || source.Evidence != "I prefer café as my standing drink." || source.Authority != memory.AuthorityOwnerStatement || source.Actor != "owner" || source.SessionID != f.session.ID {
		t.Fatalf("original source authority %+v", source)
	}
	decision := memory.ReviewDecision{DeliveryKey: stage4Key(2), PreviewID: preview.ID, PreviewSHA256: preview.SHA256, Action: "accept", Reason: "Scripted conformance approval; not an owner pilot observation."}
	var resolution memory.ReviewResult
	stage4CLI(t, f.store, &resolution, "resolve", "--scope", "global", "--preview", decision.PreviewID, "--digest", decision.PreviewSHA256, "--delivery", decision.DeliveryKey, "--action", "accept", "--reason", decision.Reason)
	if resolution.Operation == nil || len(resolution.Operation.ClaimIDs) != 1 {
		t.Fatalf("missing atomic result %+v", resolution)
	}
	var repeated memory.ReviewResult
	stage4HTTP(t, server, "resolve", map[string]any{"scope_key": "global", "decision": decision}, 200, &repeated)
	if !reflect.DeepEqual(resolution, repeated) {
		t.Fatal("HTTP duplicate delivery differs from CLI exact result")
	}
	operation, err := f.store.InspectOwnerReviewOperation(f.ctx, f.authority("global"), resolution.Operation.OperationID)
	if err != nil || !reflect.DeepEqual(operation.Preview, preview) {
		t.Fatalf("canonical accepted preview changed %v", err)
	}
	observer, err := f.store.CreateGlobalSession(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	readTime := resolution.Operation.TransactionTime
	readQuery := memory.ClaimQuery{ValidAt: &readTime, AsKnownAt: &readTime}
	claim, err := f.store.InspectSemanticObjectAt(f.ctx, observer.ScopeContext(), memory.SemanticObjectClaim, resolution.Operation.ClaimIDs[0], readQuery)
	if err != nil {
		t.Fatal(err)
	}
	beforeReplayCalls := f.modelCalls.Load() + extractor.calls.Load() + recoveredExtractor.calls.Load()
	verified, err := f.store.VerifySemanticProjection(f.ctx)
	if err != nil || !verified.Valid {
		t.Fatalf("canonical replay %v %+v", err, verified)
	}
	// Inject projection-only corruption to exercise quarantine and public owner
	// rebuild against the actual just-accepted operation stream, not a fixture op.
	if _, err = f.db.Exec(`DROP TRIGGER semantic_claims_append_only_update`); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`UPDATE semantic_claims SET literal_value='corrupted projection' WHERE claim_id=?`, resolution.Operation.ClaimIDs[0]); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`CREATE TRIGGER semantic_claims_append_only_update BEFORE UPDATE ON semantic_claims BEGIN SELECT RAISE(ABORT,'semantic claims are append-only'); END`); err != nil {
		t.Fatal(err)
	}
	divergent, err := f.store.VerifySemanticProjection(f.ctx)
	if err != nil || divergent.Valid {
		t.Fatalf("projection divergence hidden %+v %v", divergent, err)
	}
	if _, err = f.store.InspectLiteralClaims(f.ctx, observer.ScopeContext()); !errors.Is(err, eviedb.ErrSemanticScopeQuarantined) {
		t.Fatalf("quarantine bypass %v", err)
	}
	rebuilt, err := f.store.OwnerRebuildSemanticProjection(f.ctx, "stage4-conformance-rebuild")
	if err != nil || !rebuilt.Valid {
		t.Fatalf("owner rebuild %+v %v", rebuilt, err)
	}
	restored, err := f.store.InspectSemanticObjectAt(f.ctx, observer.ScopeContext(), memory.SemanticObjectClaim, resolution.Operation.ClaimIDs[0], readQuery)
	if err != nil || !reflect.DeepEqual(restored, claim) {
		t.Fatalf("canonical accepted read changed after rebuild %v", err)
	}
	if beforeReplayCalls != f.modelCalls.Load()+extractor.calls.Load()+recoveredExtractor.calls.Load() {
		t.Fatal("replay dispatched model/extractor")
	}
	var toolEvents, operations int
	if err = f.db.QueryRow(`SELECT COUNT(*) FROM events WHERE event_type IN ('tool_intent','tool_succeeded')`).Scan(&toolEvents); err != nil || toolEvents != 0 {
		t.Fatal("replay external effect", toolEvents, err)
	}
	if err = f.db.QueryRow(`SELECT COUNT(*) FROM semantic_operations`).Scan(&operations); err != nil || operations != 2 {
		t.Fatalf("duplicate accepted operation %d %v", operations, err)
	}
	if source.EventID == later.RootID || activation.AfterPosition >= selected.Cutoff {
		t.Fatal("outside source replaced frozen support")
	}
	stage4Evidence(t, "foreground_restart_closed_review_replay", map[string]any{"foreground_turns": 2, "stalled_inferences": 1, "recovered_attempts": compiled.Attempts, "source_session_closed": true, "candidate_id": item.Ref.ID, "preview_sha256": preview.SHA256, "effect_sha256": preview.EffectSHA256, "source_sha256": source.EvidenceSHA256, "accepted_operation": resolution.Operation.OperationID, "adapter_results_equal": true, "canonical_replay": true, "quarantine_rebuild": true, "replay_model_calls": 0, "replay_external_effects": 0})
}
