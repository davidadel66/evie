package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

func pilotSelectionFixture(t *testing.T) (*sql.DB, *eviedb.Store, memory.Session, eviedb.CompilerSupervisorConfig) {
	t.Helper()
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "pilot.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := eviedb.NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	x := scriptedExtractor{}
	g := generation()
	id, err := checkedID(g)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ActivateCompiler(ctx, session.ScopeContext(), memory.CompilerActivationRequest{RequestID: "empty-suffix", Selector: memory.CompilerLiveSelector{SourceScope: "global", SessionID: session.ID, Destination: destination(session)}}, g, x)
	if err != nil {
		t.Fatal(err)
	}
	config := eviedb.CompilerSupervisorConfig{Extractors: map[string]eviedb.CompilerExtractor{id: x}}
	return db, store, session, config
}

func TestPilotLaterRootDoesNotCreateEmptyFailedSuffix(t *testing.T) {
	for _, sparse := range []bool{false, true} {
		t.Run(map[bool]string{false: "contiguous", true: "sparse"}[sparse], func(t *testing.T) {
			ctx := context.Background()
			db, store, session, config := pilotSelectionFixture(t)
			first, end, err := appendTurn(ctx, store, session, "I prefer tea.")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.ReconcileCompilerEvidence(ctx, config); err != nil {
				t.Fatal(err)
			}
			later := memory.Event{ID: "sparse-later-root", Sequence: 1000000000000}
			if sparse {
				_, err = db.ExecContext(ctx, `INSERT INTO events(id,session_id,sequence,event_type,role,content,recorded_at) VALUES(?, ?, ?, 'user_message','user','I prefer coffee.','2026-09-05T00:00:00Z')`, later.ID, session.ID, later.Sequence)
			} else {
				later, _, err = appendTurn(ctx, store, session, "I prefer coffee.")
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.ReconcileCompilerEvidence(ctx, config); err != nil {
				t.Fatal(err)
			}
			a, err := store.LocalOwnerReviewContext(ctx, destination(session))
			if err != nil {
				t.Fatal(err)
			}
			d, err := store.InspectOwnerCompilerDiagnostics(ctx, a, memory.CompilerDiagnosticsQuery{SessionID: session.ID, View: "jobs"})
			if err != nil {
				t.Fatal(err)
			}
			for _, job := range d.Jobs {
				if job.State == "failed" && job.Attempts == 0 && job.SelectedNewEvents == 0 {
					var reason, root string
					if err = db.QueryRowContext(ctx, `SELECT reason,root_id FROM memory_compiler_jobs WHERE job_id=?`, job.JobID).Scan(&reason, &root); err != nil {
						t.Fatal(err)
					}
					t.Fatalf("closed root %s [%d,%d] acquired empty failed suffix [%d,%d] at later root %s sequence %d: reason=%s, failed root=%s", first.ID, first.Sequence, end.Sequence, job.FirstSequence, job.LastSequence, later.ID, later.Sequence, reason, root)
				}
			}
			if len(d.Jobs) != 1 || d.Jobs[0].SelectedNewEvents != 2 {
				t.Fatalf("already sealed root changed: %+v", d.Jobs)
			}
		})
	}
}

func TestPilotLaterRootClosesLivePrefixAndPreservesLateMember(t *testing.T) {
	ctx := context.Background()
	_, store, session, config := pilotSelectionFixture(t)
	lease, err := store.AcquireTurnLease(ctx, session.ID, "boundary-fixture", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken)
	appendEvent := func(parent memory.EventID, content string) memory.Event {
		t.Helper()
		e, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, ParentID: parent, Content: content})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	first := appendEvent("", "I prefer tea.")
	if _, err = store.ReconcileCompilerEvidence(ctx, config); err != nil {
		t.Fatal(err)
	}
	_ = appendEvent("", "I prefer coffee.")
	var closed memory.Compilation
	for range 3 {
		r, err := store.ReconcileCompilerEvidence(ctx, config)
		if err != nil {
			t.Fatal(err)
		}
		if r.SelectionID != "" {
			c, err := store.InspectCompilation(ctx, session.ScopeContext(), r.SelectionID)
			if err != nil {
				t.Fatal(err)
			}
			if c.Window.Selection.RootID == first.ID {
				closed = c
			}
		}
	}
	if closed.Window.Closure != "later_root" || closed.Window.Selection.Cutoff != first.Sequence || !slices.Equal(closed.Window.NewEventIDs, []memory.EventID{first.ID}) {
		t.Fatalf("later root did not close exact live prefix: %+v", closed)
	}
	late := appendEvent(first.ID, "Tea remains my preference.")
	found := false
	for range 6 {
		r, err := store.ReconcileCompilerEvidence(ctx, config)
		if err != nil {
			t.Fatal(err)
		}
		if r.SelectionID == "" {
			continue
		}
		c, err := store.InspectCompilation(ctx, session.ScopeContext(), r.SelectionID)
		if err != nil {
			t.Fatal(err)
		}
		if c.State == "failed" && len(c.Window.NewEventIDs) == 0 {
			t.Fatalf("empty failed interval: %+v", c)
		}
		if c.Window.Selection.RootID == first.ID && slices.Contains(c.Window.NewEventIDs, late.ID) {
			found = true
		}
	}
	if !found {
		t.Fatal("late old-root member was omitted")
	}
	retained, err := store.InspectCompilation(ctx, session.ScopeContext(), closed.SelectionID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(retained.Window.NewEventIDs, closed.Window.NewEventIDs) || retained.Window.Selection.Cutoff != closed.Window.Selection.Cutoff {
		t.Fatalf("late member rewrote immutable prefix: %+v", retained)
	}
}

func TestPilotPreFrontierRootLateMembersDoNotOwnNewerRoot(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "pilot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "frontier-boundary-fixture", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken)
	appendEvent := func(input memory.EventInput) memory.Event {
		t.Helper()
		e, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, input)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	oldRoot := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer tea."})
	x := scriptedExtractor{}
	g := generation()
	id, err := checkedID(g)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ActivateCompiler(ctx, session.ScopeContext(), memory.CompilerActivationRequest{RequestID: "late-post-frontier", Selector: memory.CompilerLiveSelector{SourceScope: "global", SessionID: session.ID, Destination: destination(session)}}, g, x); err != nil {
		t.Fatal(err)
	}
	config := eviedb.CompilerSupervisorConfig{Extractors: map[string]eviedb.CompilerExtractor{id: x}}
	appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer coffee."})
	child := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, ParentID: oldRoot.ID, Content: "Green tea is my preference."})
	end := appendEvent(memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: oldRoot.ID, Content: "Acknowledged.", Payload: json.RawMessage(`{"tool_calls":[]}`)})
	// Discover the newer root, then the old root's selected child. The old
	// root's terminal is durable but has not yet advanced ancestry discovery.
	if _, err := store.ReconcileCompilerEvidence(ctx, config); err != nil {
		t.Fatal(err)
	}
	r, err := store.ReconcileCompilerEvidence(ctx, config)
	if err != nil || r.SelectionID == "" {
		t.Fatalf("late selected child was not sealed: %+v %v", r, err)
	}
	sealed, err := store.InspectCompilation(ctx, session.ScopeContext(), r.SelectionID)
	if err != nil {
		t.Fatal(err)
	}
	if sealed.Window.Selection.RootID != oldRoot.ID || !slices.Contains(sealed.Window.NewEventIDs, child.ID) || slices.Contains(sealed.Window.NewEventIDs, oldRoot.ID) {
		t.Fatalf("wrong post-frontier evidence: %+v", sealed)
	}
	before, err := json.Marshal(sealed.Window)
	if err != nil {
		t.Fatal(err)
	}
	newest := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer water."})
	terminalOwned := slices.Contains(sealed.Window.NewEventIDs, end.ID)
	for range 8 {
		r, err := store.ReconcileCompilerEvidence(ctx, config)
		if err != nil {
			t.Fatal(err)
		}
		if r.SelectionID == "" {
			continue
		}
		c, err := store.InspectCompilation(ctx, session.ScopeContext(), r.SelectionID)
		if err != nil {
			t.Fatal(err)
		}
		if c.State == "failed" && len(c.Window.NewEventIDs) == 0 {
			t.Fatalf("post-frontier old root acquired an empty interval at newer root %s: %+v", newest.ID, c)
		}
		if c.Window.Selection.RootID == oldRoot.ID {
			if slices.Contains(c.Window.NewEventIDs, oldRoot.ID) || slices.Contains(c.Window.NewEventIDs, newest.ID) || c.Window.Selection.Cutoff > end.Sequence {
				t.Fatalf("old root acquired evidence beyond its selected suffix: %+v", c)
			}
			terminalOwned = terminalOwned || slices.Contains(c.Window.NewEventIDs, end.ID)
		}
	}
	if !terminalOwned {
		t.Fatal("late terminal was never selected")
	}
	retained, err := store.InspectCompilation(ctx, session.ScopeContext(), sealed.SelectionID)
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(retained.Window)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("later root changed the immutable selected prefix")
	}
}

func TestPilotSealedRootDoesNotOwnLateMembersOfEarlierRoot(t *testing.T) {
	ctx := context.Background()
	_, store, session, config := pilotSelectionFixture(t)
	prior, _, err := appendTurn(ctx, store, session, "I prefer tea.")
	if err != nil {
		t.Fatal(err)
	}
	current, end, err := appendTurn(ctx, store, session, "I prefer coffee.")
	if err != nil {
		t.Fatal(err)
	}
	// Discover both earlier-root members, then seal the current root while
	// its final assistant event remains undiscovered by the ancestry cursor.
	for range 3 {
		if _, err := store.ReconcileCompilerEvidence(ctx, config); err != nil {
			t.Fatal(err)
		}
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "interleaved-boundary-fixture", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken)
	for _, input := range []memory.EventInput{
		{Type: memory.EventUserMessage, Role: memory.RoleUser, ParentID: prior.ID, Content: "Green tea remains my preference."},
		{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: prior.ID, Content: "Acknowledged.", Payload: json.RawMessage(`{"tool_calls":[]}`)},
		{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer water."},
	} {
		if _, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, input); err != nil {
			t.Fatal(err)
		}
	}
	r, err := store.ReconcileCompilerEvidence(ctx, config)
	if err != nil || r.SelectionID == "" {
		t.Fatalf("current root was not reconsidered: %+v %v", r, err)
	}
	c, err := store.InspectCompilation(ctx, session.ScopeContext(), r.SelectionID)
	if err != nil {
		t.Fatal(err)
	}
	if c.State == "failed" && len(c.Window.NewEventIDs) == 0 {
		t.Fatalf("sealed root %s ending at %d acquired empty interval %d..%d containing earlier-root late members: state=%s reason=%s", current.ID, end.Sequence, c.Window.FirstSequence, c.Window.Selection.Cutoff, c.State, c.Reason)
	}
	if c.Window.Selection.RootID == current.ID && c.Window.Selection.Cutoff > end.Sequence {
		t.Fatalf("current root acquired an interval beyond its own members: %+v", c)
	}
}

func TestPilotReviewedHistoricalSuffixDoesNotManufactureForeignRootGap(t *testing.T) {
	for _, historyFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "live-first", true: "history-first"}[historyFirst], func(t *testing.T) {
			ctx := context.Background()
			db, store, session, config := pilotSelectionFixture(t)
			a, aEnd, err := appendTurn(ctx, store, session, "I prefer tea.")
			if err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if _, err = store.ReconcileCompilerEvidence(ctx, config); err != nil {
					t.Fatal(err)
				}
			}
			b, bEnd, err := appendTurn(ctx, store, session, "I prefer coffee.")
			if err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if _, err = store.ReconcileCompilerEvidence(ctx, config); err != nil {
					t.Fatal(err)
				}
			}
			lease, err := store.AcquireTurnLease(ctx, session.ID, "reviewed-historical-gap", time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			late, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, ParentID: a.ID, Content: "Green tea remains my preference."})
			if err != nil {
				t.Fatal(err)
			}
			lateEnd, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: a.ID, Content: "Acknowledged.", Payload: json.RawMessage(`{"tool_calls":[]}`)})
			if err != nil {
				t.Fatal(err)
			}
			if err = store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken); err != nil {
				t.Fatal(err)
			}
			g := generation()
			x := scriptedExtractor{}
			receipt, err := store.SelectCompilerHistory(ctx, []memory.ScopeContext{session.ScopeContext()}, memory.CompilerHistoryRequest{RequestID: "reviewed-history-foreign-gap", Ranges: []memory.CompilerHistoryRange{{SourceScope: "global", Destination: destination(session), SessionID: session.ID, FirstSequence: late.Sequence, LastSequence: lateEnd.Sequence, FirstEventID: late.ID, LastEventID: lateEnd.ID}}}, g, x)
			if err != nil {
				t.Fatal(err)
			}
			if receipt.SelectedEvents != 2 {
				t.Fatalf("history selected %d events", receipt.SelectedEvents)
			}
			for range 4 {
				if _, err = store.ReconcileCompilerHistory(ctx, config); err != nil {
					t.Fatal(err)
				}
			}
			owner, err := store.LocalOwnerReviewContext(ctx, destination(session))
			if err != nil {
				t.Fatal(err)
			}
			before, err := store.InspectOwnerCompilerDiagnostics(ctx, owner, memory.CompilerDiagnosticsQuery{SessionID: session.ID, View: "jobs", Limit: 32})
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("A=%d..%d B=%d..%d historical A=%d..%d; before live revisit: %+v", a.Sequence, aEnd.Sequence, b.Sequence, bEnd.Sequence, late.Sequence, lateEnd.Sequence, before.Jobs)
			if len(before.Jobs) != 3 {
				t.Fatalf("expected three original owned units, got %+v", before.Jobs)
			}
			reconcileBroadHistory := func() {
				_, err = store.SelectCompilerHistory(ctx, []memory.ScopeContext{session.ScopeContext()}, memory.CompilerHistoryRequest{RequestID: "history-across-coordinate-gap", Ranges: []memory.CompilerHistoryRange{{SourceScope: "global", Destination: destination(session), SessionID: session.ID, FirstSequence: a.Sequence, LastSequence: lateEnd.Sequence, FirstEventID: a.ID, LastEventID: lateEnd.ID}}}, g, x)
				if err != nil {
					t.Fatal(err)
				}
				for range 12 {
					if _, err = store.ReconcileCompilerHistory(ctx, config); err != nil {
						t.Fatal(err)
					}
				}
			}
			if historyFirst {
				reconcileBroadHistory()
			}
			for range 4 {
				if _, err = store.ReconcileCompilerEvidence(ctx, config); err != nil {
					t.Fatal(err)
				}
			}
			after, err := store.InspectOwnerCompilerDiagnostics(ctx, owner, memory.CompilerDiagnosticsQuery{SessionID: session.ID, View: "jobs", Limit: 32})
			if err != nil {
				t.Fatal(err)
			}
			for _, job := range after.Jobs {
				if job.State == "failed" && job.Attempts == 0 && job.SelectedNewEvents == 0 {
					t.Fatalf("historical A suffix manufactured a failed foreign-root gap: interval=%d..%d state=%s reason=%s attempts=%d selected=%d", job.FirstSequence, job.LastSequence, job.State, job.Reason, job.Attempts, job.SelectedNewEvents)
				}
			}
			if len(after.Jobs) != len(before.Jobs) {
				t.Fatalf("already-owned events acquired new jobs: before=%d after=%+v", len(before.Jobs), after.Jobs)
			}
			units, err := store.InspectOwnerCompilerDiagnostics(ctx, owner, memory.CompilerDiagnosticsQuery{SessionID: session.ID, View: "selections", Limit: 32})
			if err != nil {
				t.Fatal(err)
			}
			var gap memory.CompilerDiagnosticUnit
			for _, unit := range units.Selections {
				if unit.Reason == "no_root_members" {
					gap = unit
				}
			}
			if gap.SelectionID == "" || gap.JobID != "" || gap.State != "excluded" || gap.SelectedNewEvents != 0 || gap.FirstSequence != b.Sequence || gap.LastSequence != bEnd.Sequence {
				t.Fatalf("incorrect coordinate-only exclusion: %+v", units.Selections)
			}
			var coverageRows int
			if err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_compiler_coverage`).Scan(&coverageRows); err != nil {
				t.Fatal(err)
			}
			if coverageRows != 0 {
				t.Fatalf("coordinate-only exclusion invented %d coverage rows", coverageRows)
			}
			var file string
			var sequence int
			var name string
			if err = db.QueryRowContext(ctx, `PRAGMA database_list`).Scan(&sequence, &name, &file); err != nil {
				t.Fatal(err)
			}
			for range 3 {
				if err = db.Close(); err != nil {
					t.Fatal(err)
				}
				db, err = eviedb.OpenDBAt(file)
				if err != nil {
					t.Fatal(err)
				}
				reopened := db
				t.Cleanup(func() { reopened.Close() })
				store = eviedb.NewStore(db)
				for range 4 {
					if _, err = store.ReconcileCompilerEvidence(ctx, config); err != nil {
						t.Fatal(err)
					}
				}
			}
			defer db.Close()
			owner, err = store.LocalOwnerReviewContext(ctx, destination(session))
			if err != nil {
				t.Fatal(err)
			}
			restarted, err := store.InspectOwnerCompilerDiagnostics(ctx, owner, memory.CompilerDiagnosticsQuery{SessionID: session.ID, View: "jobs", Limit: 32})
			if err != nil {
				t.Fatal(err)
			}
			if len(restarted.Jobs) != 3 {
				t.Fatalf("restart replay created a job: %+v", restarted.Jobs)
			}
			for i, job := range restarted.Jobs {
				if job.JobID != before.Jobs[i].JobID || job.SelectedNewEvents != 2 || job.CompletedNewEvents != 0 || job.Lane != before.Jobs[i].Lane {
					t.Fatalf("gap rewrote jobs, lane, or coverage: %+v", restarted.Jobs)
				}
			}
			// A broad historical receipt maps each selected event to its actual root.
			// The A gap must never make the pending B events appear excluded/covered.
			reconcileBroadHistory()
			progress, err := store.InspectCompilerHistory(ctx, []memory.ScopeContext{session.ScopeContext()}, "history-across-coordinate-gap", 0, 0, 32)
			if err != nil {
				t.Fatal(err)
			}
			if progress.ContiguousFrontier != 0 {
				t.Fatalf("coordinate exclusion advanced real-event coverage: %+v", progress)
			}
			for _, interval := range progress.Intervals {
				if interval.State != "queued" {
					t.Fatalf("real event acquired false coverage/exclusion: %+v", progress)
				}
			}
			lease, err = store.AcquireTurnLease(ctx, session.ID, "later-real-member", time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			next, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, ParentID: a.ID, Content: "My actual later tea preference."})
			if err != nil {
				t.Fatal(err)
			}
			if err = store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken); err != nil {
				t.Fatal(err)
			}
			for range 6 {
				if _, err = store.ReconcileCompilerEvidence(ctx, config); err != nil {
					t.Fatal(err)
				}
			}
			final, err := store.InspectOwnerCompilerDiagnostics(ctx, owner, memory.CompilerDiagnosticsQuery{SessionID: session.ID, View: "jobs", Limit: 32})
			if err != nil {
				t.Fatal(err)
			}
			if len(final.Jobs) != 4 {
				t.Fatalf("actual later member did not progress: %+v", final.Jobs)
			}
			found := false
			for _, job := range final.Jobs {
				c, err := store.InspectCompilation(ctx, session.ScopeContext(), job.JobID)
				if err != nil {
					t.Fatal(err)
				}
				if slices.Contains(c.Window.NewEventIDs, next.ID) {
					found = true
					if c.State != "queued" {
						t.Fatalf("later member not queued: %+v", c)
					}
				}
			}
			if !found {
				t.Fatal("actual later root member was skipped")
			}
		})
	}
}

func TestPilotExplicitEmptySelectionKeepsFailedOutcome(t *testing.T) {
	ctx := context.Background()
	_, store, session, _ := pilotSelectionFixture(t)
	a, aEnd, err := appendTurn(ctx, store, session, "I prefer tea.")
	if err != nil {
		t.Fatal(err)
	}
	_, bEnd, err := appendTurn(ctx, store, session, "I prefer coffee.")
	if err != nil {
		t.Fatal(err)
	}
	sel := memory.CompilationSelection{SessionID: session.ID, RootID: a.ID, Cutoff: aEnd.Sequence, Destination: destination(session)}
	if _, err = store.QueueCandidateUnit(ctx, session.ScopeContext(), sel, generation(), scriptedExtractor{}); err != nil {
		t.Fatal(err)
	}
	sel.Cutoff = bEnd.Sequence
	empty, err := store.QueueCandidateUnit(ctx, session.ScopeContext(), sel, generation(), scriptedExtractor{})
	if err != nil {
		t.Fatal(err)
	}
	if empty.State != "failed" || empty.Reason != "empty_selection" || empty.JobID == "" || len(empty.Window.NewEventIDs) != 0 {
		t.Fatalf("explicit selection error was converted: %+v", empty)
	}
}
