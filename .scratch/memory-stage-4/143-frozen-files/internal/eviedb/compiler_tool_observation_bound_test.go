package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

func TestCompilerClockMaximumWindowAndAdoptionReadEachEventOnce(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	f.generation.EvidencePolicy = memory.CompilerClockEvidencePolicy
	appendEvent := func(input memory.EventInput) memory.Event { t.Helper(); return activationAppend(t, f, input) }
	overlap := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Earlier owner assertion."})
	for i := 1; i < 16; i++ {
		appendEvent(memory.EventInput{ParentID: overlap.ID, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Earlier owner assertion."})
	}
	root := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Using the checked local date, today I adopted tea."})
	for i := 1; i < 63; i++ {
		appendEvent(memory.EventInput{ParentID: root.ID, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "A concise owner assertion."})
	}
	assistant := appendEvent(memory.EventInput{ParentID: root.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "Checking the clock.", Payload: json.RawMessage(`{"tool_calls":[{"id":"clock-call","name":"get_time","arguments":"{}"}]}`)})
	intent := appendEvent(memory.EventInput{ParentID: assistant.ID, Type: memory.EventToolIntent, ExecutionID: "bound-clock", Payload: json.RawMessage(`{"call":{"id":"clock-call","name":"get_time","arguments":"{}"}}`)})
	outcome := appendEvent(memory.EventInput{ParentID: intent.ID, Type: memory.EventToolSucceeded, Role: memory.RoleTool, ExecutionID: "bound-clock", Content: "2026-09-04 11:42:00", Payload: json.RawMessage(`{"tool_call_id":"clock-call","is_error":false}`)})
	last := outcome
	for i := 0; i < 7; i++ {
		last = appendEvent(memory.EventInput{ParentID: outcome.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "Observed local display."})
	}
	sel := memory.CompilationSelection{SessionID: f.owner.SessionID, RootID: root.ID, Cutoff: last.Sequence, Destination: "global"}
	if err := f.store.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		counted := &compilerCountedSelection{Conn: conn, t: t}
		w, state, reason, err := captureCompilerWindow(ctx, counted, f.owner, sel, root.Sequence, memory.CompilerClockEvidencePolicy)
		if err != nil {
			return err
		}
		if state != "queued" || len(w.Sources) != 88 || counted.inspections != 89 {
			t.Fatalf("capture state=%s reason=%s sources=%d reads=%d", state, reason, len(w.Sources), counted.inspections)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	queued, err := f.store.QueueCandidateUnit(ctx, f.owner, sel, f.generation, &workerScript{})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := f.store.claimCompilerJob(ctx, f.owner, queued.JobID, &workerScript{})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.store.stageCompilerResult(ctx, f.owner, claim.JobID, claim.Holder, claim.Fence, claim.Request, []memory.MemoryCandidate{}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`UPDATE memory_compiler_jobs SET lease_until=unixepoch('now')-1 WHERE job_id=?`, claim.JobID); err != nil {
		t.Fatal(err)
	}
	if err = f.store.RecoverCompilerWork(ctx); err != nil {
		t.Fatal(err)
	}
	if err = f.store.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		counted := &compilerCountingStageTransaction{Conn: conn}
		_, err := adoptCompilerStageInTransaction(ctx, counted, f.owner, queued.JobID)
		if err != nil {
			return err
		}
		// 88 offered fields plus the one control-only intent. Clock, assistant and
		// root are reused from the same cache, even when source order differs.
		if counted.eventReads != 89 || counted.eventReads > 128 {
			t.Fatalf("adoption reads=%d; want89", counted.eventReads)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
