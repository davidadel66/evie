package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func historyScopedFixture(t *testing.T, f *workerFixture, kind string) (*workerFixture, func(bool)) {
	t.Helper()
	ctx := context.Background()
	scoped := *f
	var session memory.Session
	var err error
	if kind == "workspace" {
		workspace, e := f.store.RegisterWorkspace(ctx, "Temporarily unavailable history")
		if e != nil {
			t.Fatal(e)
		}
		session, err = f.store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, standardReceipt(t))
	} else {
		project, e := f.store.RegisterProject(ctx, "Temporarily unavailable history", t.TempDir())
		if e != nil {
			t.Fatal(e)
		}
		session, err = f.store.CreateProjectSession(ctx, project.ID)
	}
	if err != nil {
		t.Fatal(err)
	}
	scoped.owner = session.ScopeContext()
	scoped.lease, err = f.store.AcquireTurnLease(ctx, session.ID, "history-unavailable", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	toggle := func(unavailable bool) {
		t.Helper()
		var err error
		switch kind {
		case "workspace":
			state := "active"
			if unavailable {
				state = "archived"
			}
			_, err = f.db.Exec(`UPDATE workspaces SET lifecycle_state=? WHERE id=?`, state, session.WorkspaceID)
		case "project":
			_, err = f.db.Exec(`UPDATE projects SET archived=? WHERE id=?`, unavailable, session.ProjectID)
		case "quarantined":
			scope := scopeKeyForContext(scoped.owner)
			if unavailable {
				if _, err = f.db.Exec(`INSERT OR IGNORE INTO semantic_scopes(scope_id,scope_key,scope_kind,registry_id,revision) VALUES('history-quarantine',?,'project',?,0)`, scope, session.ProjectID); err != nil {
					t.Fatal(err)
				}
				_, err = f.db.Exec(`INSERT INTO semantic_projection_quarantine(scope_id,reason,verified_at) SELECT scope_id,'fixture quarantine',strftime('%Y-%m-%dT%H:%M:%fZ','now') FROM semantic_scopes WHERE scope_key=?`, scope)
			} else {
				_, err = f.db.Exec(`DELETE FROM semantic_projection_quarantine WHERE scope_id IN (SELECT scope_id FROM semantic_scopes WHERE scope_key=?)`, scope)
			}
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	return &scoped, toggle
}

func scopedHistoryRange(f *workerFixture, first, last memory.Event) memory.CompilerHistoryRange {
	r := historyRange(f, first, last)
	r.SourceScope = scopeKeyForContext(f.owner)
	r.Destination = r.SourceScope
	return r
}

func TestCompilerHistoryUnavailableScopeRotatesWithoutStarvingSelectionOrResume(t *testing.T) {
	for _, kind := range []string{"project", "workspace", "quarantined"} {
		t.Run(kind, func(t *testing.T) {
			f := newWorkerFixture(t)
			ctx := context.Background()
			// Prepare a separate global explicit-resume obligation before the older
			// scoped root becomes unavailable. Root failure must not skip this lane.
			resumeRoot, resumeEnd := historyRoot(t, f, "Global work selected for later resume.")
			historySelect(t, f, "global-resume", historyRange(f, resumeRoot, resumeEnd))
			historyReconcile(t, f, 4)
			if _, err := f.store.CancelCompilerHistory(ctx, []memory.ScopeContext{f.owner}, memory.CompilerHistoryChange{RequestID: "global-resume", OperationID: "cancel-resume", ExpectedRevision: 1}); err != nil {
				t.Fatal(err)
			}
			if _, err := f.store.ResumeCompilerHistory(ctx, []memory.ScopeContext{f.owner}, memory.CompilerHistoryChange{RequestID: "global-resume", OperationID: "resume-global", ExpectedRevision: 2}, f.generation, &activationScript{}); err != nil {
				t.Fatal(err)
			}
			scoped, toggle := historyScopedFixture(t, f, kind)
			root, end := historyRoot(t, scoped, "Original captured scoped assertion.")
			original := historySelect(t, scoped, "unavailable", scopedHistoryRange(scoped, root, end))
			global, globalEnd := historyRoot(t, f, "Independent later global assertion.")
			historySelect(t, f, "later-global", historyRange(f, global, globalEnd))
			toggle(true)
			paused := false
			for range 16 {
				r, err := f.store.ReconcileCompilerHistory(ctx, f.config(&activationScript{}))
				if err != nil {
					t.Fatal("known unavailable scope became infrastructure failure", err)
				}
				paused = paused || r.State == "authorization_paused"
			}
			if !paused {
				t.Fatal("unavailable authority had no safe disposition")
			}
			if n := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_jobs WHERE session_id=?`, scoped.owner.SessionID); n != 0 {
				t.Fatal("unavailable source was materialized", n)
			}
			if n := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_jobs WHERE session_id=? AND state='queued'`, f.owner.SessionID); n != 2 {
				t.Fatal("independent selection/resume starved", n)
			}
			if n := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_history_roots WHERE request_id='unavailable' AND reason='source_scope_unavailable' AND state='selected_unmaterialized'`); n != 1 {
				t.Fatal("original obligation lost", n)
			}
			if n := activationCount(t, f.db, `SELECT pending_roots FROM memory_compiler_history_requests WHERE request_id='unavailable'`); n != 1 {
				t.Fatal("unavailable work removed from revisit index")
			}
			if _, err := f.store.InspectCompilerHistory(ctx, []memory.ScopeContext{scoped.owner}, "unavailable", 0, 0, 64); err == nil {
				t.Fatal("progress inspection bypassed unavailable scope authority")
			}
			script := &workerScript{}
			for range 2 {
				if worked, err := f.store.RunCompilerStep(ctx, f.config(script)); !worked || err != nil {
					t.Fatal("independent worker failed", worked, err)
				}
			}
			toggle(false)
			outside := activationAppend(t, scoped, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Later unselected scoped assertion."})
			if err := f.store.ReleaseTurnLease(ctx, scoped.owner.SessionID, scoped.lease.HolderID, scoped.lease.FencingToken); err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.Exec(`UPDATE sessions SET status='closed' WHERE id=?`, scoped.owner.SessionID); err != nil {
				t.Fatal(err)
			}
			f.store = f.second(t)
			scoped.store = f.store
			historyReconcile(t, f, 4)
			var raw []byte
			var attempts int
			var cutoff int64
			if err := f.db.QueryRow(`SELECT request,attempts,last_sequence FROM memory_compiler_jobs WHERE session_id=?`, scoped.owner.SessionID).Scan(&raw, &attempts, &cutoff); err != nil {
				t.Fatal(err)
			}
			if attempts != 0 || cutoff != end.Sequence || strings.Contains(string(raw), string(outside.ID)) {
				t.Fatal("restoration widened cutoff or consumed an attempt", attempts, cutoff)
			}
			if repeated := historySelect(t, scoped, "unavailable", scopedHistoryRange(scoped, root, end)); string(compilerJSON(repeated)) != string(compilerJSON(original)) {
				t.Fatal("restoration/reopen changed receipt")
			}
			if worked, err := f.store.RunCompilerStep(ctx, f.config(script)); !worked || err != nil {
				t.Fatal("restored closed source did not progress", worked, err)
			}
			progress, err := f.store.InspectCompilerHistory(ctx, []memory.ScopeContext{scoped.owner}, "unavailable", 0, 0, 64)
			if err != nil {
				t.Fatal(err)
			}
			if progress.ContiguousFrontier != end.Sequence || progress.OutsideSelectionEvents != 1 || progress.Intervals[0].State != "completed_empty" || progress.Intervals[0].Attempts != 1 {
				t.Fatalf("restored exact progress %+v", progress)
			}
		})
	}
}

func TestCompilerHistoryUnavailableDispositionDoesNotSwallowDatabaseOrCancellationErrors(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	scoped, toggle := historyScopedFixture(t, f, "project")
	root, end := historyRoot(t, scoped, "An assertion.")
	historySelect(t, scoped, "unavailable", scopedHistoryRange(scoped, root, end))
	// Discover without materializing so the archived root is the next decision.
	for range 2 {
		if err := f.store.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
			var r memory.CompilerReconciliation
			return discoverCompilerHistory(ctx, conn, &r)
		}); err != nil {
			t.Fatal(err)
		}
	}
	toggle(true)
	if _, err := f.db.Exec(`CREATE TRIGGER fail_history_unavailable BEFORE UPDATE OF reason ON memory_compiler_history_roots WHEN NEW.reason='source_scope_unavailable' BEGIN SELECT RAISE(ABORT,'fixture disposition write failed'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ReconcileCompilerHistory(ctx, f.config(&activationScript{})); err == nil {
		t.Fatal("failed pause write reported success")
	}
	if n := activationCount(t, f.db, `SELECT root_checked FROM memory_compiler_history_requests WHERE request_id='unavailable'`); n != 0 {
		t.Fatal("failed transaction committed root rotation")
	}
	if n := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_history_roots WHERE reason=''`); n != 1 {
		t.Fatal("failed transaction committed a disposition")
	}
	if _, err := f.db.Exec(`DROP TRIGGER fail_history_unavailable`); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := f.store.ReconcileCompilerHistory(cancelled, f.config(&activationScript{})); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation converted to pause", err)
	}
	if n := activationCount(t, f.db, `SELECT root_checked FROM memory_compiler_history_requests WHERE request_id='unavailable'`); n != 0 {
		t.Fatal("cancelled decision mutated scheduling")
	}
	result, err := f.store.ReconcileCompilerHistory(ctx, f.config(&activationScript{}))
	if err != nil || result.State != "authorization_paused" {
		t.Fatal("safe disposition failed", result, err)
	}
	// An actual closed connection must stay an error, even when scope metadata
	// would otherwise prove an unavailable source.
	conn, err := f.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if available, err := compilerHistorySourceAvailable(ctx, conn, scoped.owner, scopeKeyForContext(scoped.owner)); available || !errors.Is(err, sql.ErrConnDone) {
		t.Fatal(fmt.Sprintf("connection error converted to availability: %v %v", available, err))
	}
}
