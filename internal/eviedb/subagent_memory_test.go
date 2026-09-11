package eviedb_test

import (
	"context"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
)

func TestDelegatedAssignmentCannotCompileOrSupportOwnerMemory(t *testing.T) {
	f := newCompilerFixture(t)
	ctx := context.Background()
	manager, err := plugins.NewManager(tools.NewToolset(nil), plugins.NewWeb())
	if err != nil {
		t.Fatal(err)
	}
	if err = manager.SetEnabled(plugins.WebPluginID, true); err != nil {
		t.Fatal(err)
	}
	resolved, err := manager.ResolvePreset(plugins.ResearchPresetID)
	if err != nil {
		t.Fatal(err)
	}
	child, err := f.store.CreateDelegatedSessionWithComposition(ctx, f.session.ID, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := f.store.AcquireTurnLease(ctx, child.ID, "worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	live := &historyPublicScript{scriptedCompiler: scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		if r.Window.Selection.SessionID == child.ID {
			t.Error("live extractor received delegated assignment")
		}
		return compilerOutput(r, []memory.ExtractorCandidate{}), nil
	}}}
	generation := compilerGeneration()
	if _, err = f.store.ActivateCompiler(ctx, f.session.ScopeContext(), memory.CompilerActivationRequest{RequestID: "live-owner", Selector: memory.CompilerLiveSelector{SourceScope: "global", Destination: "global"}}, generation, live); err != nil {
		t.Fatal(err)
	}
	root, err := f.store.AppendEventWithLease(ctx, child.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "David prefers tea"})
	if err != nil {
		t.Fatal(err)
	}
	extractor := &scriptedCompiler{}
	_, err = f.store.CompileCandidateUnit(ctx, child.ScopeContext(), memory.CompilationSelection{SessionID: child.ID, RootID: root.ID, Cutoff: root.Sequence, Destination: "global"}, compilerGeneration(), extractor)
	if err == nil || extractor.calls.Load() != 0 {
		t.Fatalf("delegated source compiled: %v calls=%d", err, extractor.calls.Load())
	}
	_, err = f.store.PrepareRememberLiteral(ctx, child.ScopeContext(), memory.RememberLiteralRequest{IdempotencyKey: "idem:v1:91000000-0000-4000-8000-000000000001", SourceEventID: root.ID, Predicate: "drink", PredicateLabel: "drink", Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "tea"}})
	if err == nil {
		t.Fatal("delegated assignment became owner statement")
	}
	final, err := f.store.AppendEventWithLease(ctx, child.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "child finding"})
	if err != nil {
		t.Fatal(err)
	}
	historical := &historyPublicScript{}
	_, err = f.store.SelectCompilerHistory(ctx, []memory.ScopeContext{child.ScopeContext()}, memory.CompilerHistoryRequest{RequestID: "delegated-history", Ranges: []memory.CompilerHistoryRange{{SourceScope: "global", Destination: "global", SessionID: child.ID, FirstSequence: root.Sequence, LastSequence: final.Sequence, FirstEventID: root.ID, LastEventID: final.ID}}}, generation, historical)
	if err == nil || historical.calls.Load() != 0 {
		t.Fatalf("delegated historical source admitted: %v", err)
	}
	f.selection(t, "I prefer green tea.", true)
	id, _, err := memory.CompilerGenerationIdentity(generation)
	if err != nil {
		t.Fatal(err)
	}
	config := eviedb.CompilerSupervisorConfig{Extractors: map[string]eviedb.CompilerExtractor{id: live}}
	for i := 0; i < 12; i++ {
		if _, err = f.store.ReconcileCompilerEvidence(ctx, config); err != nil {
			t.Fatal(err)
		}
	}
	if worked, err := f.store.RunCompilerStep(ctx, config); err != nil || !worked {
		t.Fatalf("owner compilation stalled: %t %v", worked, err)
	}
	if live.calls.Load() != 1 {
		t.Fatalf("unexpected live extraction count %d", live.calls.Load())
	}

}
