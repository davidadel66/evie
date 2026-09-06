package main

import (
    "context"
    "encoding/json"
    "testing"
    "time"
    "github.com/davidadel66/evie/internal/memory"
)

func TestPilotHistoryFirstDoesNotManufactureForeignRootGap(t *testing.T) {
    ctx := context.Background()
    _, store, session, config := pilotSelectionFixture(t)
    a, aEnd, err := appendTurn(ctx, store, session, "I prefer tea.")
    if err != nil { t.Fatal(err) }
    for range 2 { if _, err = store.ReconcileCompilerEvidence(ctx, config); err != nil { t.Fatal(err) } }
    b, bEnd, err := appendTurn(ctx, store, session, "I prefer coffee.")
    if err != nil { t.Fatal(err) }
    for range 2 { if _, err = store.ReconcileCompilerEvidence(ctx, config); err != nil { t.Fatal(err) } }
    lease, err := store.AcquireTurnLease(ctx, session.ID, "reviewed-historical-gap", time.Minute)
    if err != nil { t.Fatal(err) }
    late, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, ParentID: a.ID, Content: "Green tea remains my preference."})
    if err != nil { t.Fatal(err) }
    lateEnd, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: a.ID, Content: "Acknowledged.", Payload: json.RawMessage(`{"tool_calls":[]}`)})
    if err != nil { t.Fatal(err) }
    if err = store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken); err != nil { t.Fatal(err) }
    g := generation()
    x := scriptedExtractor{}
    receipt, err := store.SelectCompilerHistory(ctx, []memory.ScopeContext{session.ScopeContext()}, memory.CompilerHistoryRequest{RequestID: "reviewed-history-foreign-gap", Ranges: []memory.CompilerHistoryRange{{SourceScope: "global", Destination: destination(session), SessionID: session.ID, FirstSequence: late.Sequence, LastSequence: lateEnd.Sequence, FirstEventID: late.ID, LastEventID: lateEnd.ID}}}, g, x)
    if err != nil { t.Fatal(err) }
    if receipt.SelectedEvents != 2 { t.Fatalf("history selected %d events", receipt.SelectedEvents) }
    for range 4 { if _, err = store.ReconcileCompilerHistory(ctx, config); err != nil { t.Fatal(err) } }
    owner, err := store.LocalOwnerReviewContext(ctx, destination(session))
    if err != nil { t.Fatal(err) }
    before, err := store.InspectOwnerCompilerDiagnostics(ctx, owner, memory.CompilerDiagnosticsQuery{SessionID: session.ID, View: "jobs", Limit: 32})
    if err != nil { t.Fatal(err) }
    t.Logf("A=%d..%d B=%d..%d historical A=%d..%d; before live revisit: %+v", a.Sequence, aEnd.Sequence, b.Sequence, bEnd.Sequence, late.Sequence, lateEnd.Sequence, before.Jobs)
    if len(before.Jobs) != 3 { t.Fatalf("expected three original owned units, got %+v", before.Jobs) }
    _, err = store.SelectCompilerHistory(ctx, []memory.ScopeContext{session.ScopeContext()}, memory.CompilerHistoryRequest{RequestID: "history-before-live-foreign-gap", Ranges: []memory.CompilerHistoryRange{{SourceScope: "global", Destination: destination(session), SessionID: session.ID, FirstSequence: a.Sequence, LastSequence: lateEnd.Sequence, FirstEventID: a.ID, LastEventID: lateEnd.ID}}}, g, x)
    if err != nil { t.Fatal(err) }
    for range 12 { if _, err = store.ReconcileCompilerHistory(ctx, config); err != nil { t.Fatal(err) } }
    after, err := store.InspectOwnerCompilerDiagnostics(ctx, owner, memory.CompilerDiagnosticsQuery{SessionID: session.ID, View: "jobs", Limit: 32})
    if err != nil { t.Fatal(err) }
    for _, job := range after.Jobs {
        if job.State == "failed" && job.Attempts == 0 && job.SelectedNewEvents == 0 {
            t.Fatalf("historical A suffix manufactured a failed foreign-root gap: interval=%d..%d state=%s reason=%s attempts=%d selected=%d", job.FirstSequence, job.LastSequence, job.State, job.Reason, job.Attempts, job.SelectedNewEvents)
        }
    }
    if len(after.Jobs) != len(before.Jobs) { t.Fatalf("already-owned events acquired new jobs: before=%d after=%+v", len(before.Jobs), after.Jobs) }
}
