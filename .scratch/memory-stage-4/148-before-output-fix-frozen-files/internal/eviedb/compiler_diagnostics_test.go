package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func diagnosticRead(t *testing.T, f *workerFixture, view string) memory.CompilerDiagnostics {
	t.Helper()
	a, err := f.store.LocalOwnerReviewContext(context.Background(), "global")
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.store.InspectOwnerCompilerDiagnostics(context.Background(), a, memory.CompilerDiagnosticsQuery{SessionID: f.owner.SessionID, View: view})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestCompilerDiagnosticsProgressFailureAndRestart(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	first := f.queue(t, "PRIVATE source should never enter diagnostics.")
	before := diagnosticRead(t, f, "jobs")
	if before.Counts["jobs_queued"] != 1 || len(before.Jobs) != 1 || before.Jobs[0].QueuedAtUnixMS == nil || before.Jobs[0].SelectedNewEvents == 0 {
		t.Fatalf("queued %+v", before)
	}
	unavailable := &workerScript{run: func(context.Context, memory.CompilerRequest) (CompilerExtraction, error) {
		return CompilerExtraction{ReleaseEvidence: "not_dispatched"}, errors.Join(ErrCompilerEndpointUnavailable, errors.New("PRIVATE endpoint credential"))
	}}
	if worked, err := f.store.RunCompilerStep(ctx, f.config(unavailable)); !worked || err == nil {
		t.Fatalf("unavailable %v %v", worked, err)
	}
	second := f.queue(t, "A later independent statement.")
	if worked, err := f.store.RunCompilerStep(ctx, f.config(&workerScript{})); !worked || err != nil {
		t.Fatalf("later %v %v", worked, err)
	}
	f.store = f.second(t)
	got := diagnosticRead(t, f, "jobs")
	if got.Counts["jobs_retry_wait"] != 1 || got.Counts["jobs_completed_empty"] != 1 || got.Counts["attempts"] != 2 {
		t.Fatalf("counts %+v", got.Counts)
	}
	for _, job := range got.Jobs {
		if len(job.Measurements) != 1 {
			t.Fatalf("missing attempt %+v", job)
		}
		m := job.Measurements[0]
		if m.InferenceNanos == nil || m.QueueWaitNanos == nil || *m.InferenceNanos < 0 || *m.QueueWaitNanos < 0 {
			t.Fatalf("measurements %+v", m)
		}
		switch job.JobID {
		case first.JobID:
			if job.State != "retry_wait" || job.Reason != "endpoint_unavailable" || job.CompletedNewEvents != 0 || m.ObservedOutcome != "failed" || m.ValidationNanos != nil {
				t.Fatalf("gap %+v", job)
			}
		case second.JobID:
			if job.CompletedNewEvents != job.SelectedNewEvents || job.PublishedAtUnixMS == nil || m.ValidationNanos == nil || m.DatabaseCompletionNanos == nil || m.ObservedOutcome != "completed" {
				t.Fatalf("completed %+v", job)
			}
		}
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "PRIVATE") || strings.Contains(string(raw), "prompt") || strings.Contains(string(raw), "server_identity") {
		t.Fatalf("unsafe diagnostics %s", raw)
	}
	if _, err := f.store.CancelCompilation(ctx, f.owner, first.JobID); err != nil {
		t.Fatal(err)
	}
	after := diagnosticRead(t, f, "jobs")
	if after.Counts["cancellations"] != 1 || after.Counts["jobs_cancelled"] != 1 || after.Counts["jobs_retry_wait"] != 0 {
		t.Fatalf("cancel counters %+v", after.Counts)
	}
}

func TestCompilerDiagnosticsClosedSessionScopeAndCursor(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	f.queue(t, "one")
	f.queue(t, "two")
	a, err := f.store.LocalOwnerReviewContext(ctx, "global")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.store.ReleaseTurnLease(ctx, f.owner.SessionID, f.lease.HolderID, f.lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`UPDATE sessions SET status='closed' WHERE id=?`, f.owner.SessionID); err != nil {
		t.Fatal(err)
	}
	page, err := f.store.InspectOwnerCompilerDiagnostics(ctx, a, memory.CompilerDiagnosticsQuery{SessionID: f.owner.SessionID, Limit: 1})
	if err != nil || len(page.Jobs) != 1 || page.NextCursor == "" {
		t.Fatalf("first %+v %v", page, err)
	}
	next, err := f.store.InspectOwnerCompilerDiagnostics(ctx, a, memory.CompilerDiagnosticsQuery{SessionID: f.owner.SessionID, Limit: 1, Cursor: page.NextCursor})
	if err != nil || len(next.Jobs) != 1 || next.Jobs[0].JobID == page.Jobs[0].JobID {
		t.Fatalf("next %+v %v", next, err)
	}
	for _, q := range []memory.CompilerDiagnosticsQuery{{SessionID: f.owner.SessionID, Limit: 33}, {SessionID: f.owner.SessionID, View: "selection"}, {SessionID: f.owner.SessionID, View: "raw"}} {
		if _, err = f.store.InspectOwnerCompilerDiagnostics(ctx, a, q); !errors.Is(err, ErrReviewInvalidRequest) {
			t.Fatalf("invalid %+v %v", q, err)
		}
	}
	if _, err = f.store.InspectOwnerCompilerDiagnostics(ctx, a, memory.CompilerDiagnosticsQuery{SessionID: f.owner.SessionID, View: "candidates", Cursor: page.NextCursor}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("crossview cursor %v", err)
	}
	scoped, err := f.store.LocalOwnerReviewContext(ctx, "session:"+string(f.owner.SessionID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.InspectOwnerCompilerDiagnostics(ctx, scoped, memory.CompilerDiagnosticsQuery{SessionID: f.owner.SessionID, Cursor: page.NextCursor}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("crossscope cursor %v", err)
	}
	if _, err = f.store.InspectOwnerCompilerDiagnostics(ctx, OwnerReviewContext{}, memory.CompilerDiagnosticsQuery{SessionID: f.owner.SessionID}); !errors.Is(err, ErrOwnerReviewUnauthorized) {
		t.Fatalf("forged authority %v", err)
	}
	sessions, err := f.store.ListOwnerCompilerSessions(ctx, a, memory.CompilerDiagnosticSessionQuery{Limit: 1})
	if err != nil || len(sessions.SessionIDs) != 1 || sessions.SessionIDs[0] != f.owner.SessionID {
		t.Fatalf("closed navigation %+v %v", sessions, err)
	}
}

func TestCompilerDiagnosticsSelectedAndOutsideRemainDistinct(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	before := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "outside"})
	activationStart(t, f)
	selected := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "selected"})
	activationAppend(t, f, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: selected.ID, Content: "done"})
	a, err := f.store.LocalOwnerReviewContext(ctx, "global")
	if err != nil {
		t.Fatal(err)
	}
	q := memory.CompilerDiagnosticsQuery{SessionID: f.owner.SessionID, View: "selection", GenerationID: f.generationID, Limit: 1}
	got, err := f.store.InspectOwnerCompilerDiagnostics(ctx, a, q)
	if err != nil || len(got.Selection) != 1 || got.Selection[0].EventID != before.ID || got.Selection[0].Membership != "outside_selection" {
		t.Fatalf("outside %+v %v", got, err)
	}
	q.Cursor = got.NextCursor
	got, err = f.store.InspectOwnerCompilerDiagnostics(ctx, a, q)
	if err != nil || len(got.Selection) != 1 || got.Selection[0].EventID != selected.ID || got.Selection[0].Membership != "selected_live" {
		t.Fatalf("selected %+v %v", got, err)
	}
	acts := diagnosticRead(t, f, "activations")
	if len(acts.Activations) != 1 || acts.Activations[0].GenerationID != f.generationID {
		t.Fatalf("generation %+v", acts)
	}
}

func TestCompilerDiagnosticsLegacyIndexingAndQueryPlans(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	base := f.queue(t, "base")
	// A retained installation is represented by terminal ledger rows; all bounds
	// apply to visited metadata, including jobs with no candidate outputs.
	err := f.store.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		for i := 0; i < 80; i++ {
			id := fmt.Sprintf("retained-%03d", i)
			if _, e := conn.ExecContext(ctx, `INSERT INTO memory_compiler_jobs(job_id,generation_id,destination,session_id,root_id,first_sequence,last_sequence,window_hash,request,state) SELECT ?,generation_id,destination,session_id,root_id,first_sequence,1000+?,window_hash,request,'completed_empty' FROM memory_compiler_jobs WHERE job_id=?`, id, i, base.JobID); e != nil {
				return e
			}
		}
		_, e := conn.ExecContext(ctx, `DELETE FROM memory_compiler_diagnostic_jobs; DELETE FROM memory_compiler_diagnostic_sessions; UPDATE memory_compiler_diagnostic_reconcile SET last_job='',cutoff=(SELECT MAX(job_id) FROM memory_compiler_jobs),complete=0`)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	first := diagnosticRead(t, f, "jobs")
	if !first.Indexing || first.Counts["jobs_completed_empty"] > 16 {
		t.Fatalf("unbounded legacy pass %+v", first)
	}
	for i := 0; i < 6; i++ {
		first = diagnosticRead(t, f, "jobs")
	}
	if first.Indexing || first.Counts["jobs_completed_empty"] != 80 || first.Counts["jobs_queued"] != 1 {
		t.Fatalf("legacy counts %+v", first)
	}
	for _, query := range []string{
		`SELECT job_id FROM memory_compiler_jobs WHERE destination='global' AND session_id='x' AND job_id>'x' ORDER BY job_id LIMIT 32`,
		`SELECT through_position FROM memory_compiler_activations WHERE generation_id='x' AND destination='global' AND source_scope='global' AND source_session='' AND (through_position IS NULL OR through_position>after_position) AND after_position<100 ORDER BY after_position DESC LIMIT 1`,
		`SELECT event_id FROM memory_compiler_history_selection_refs WHERE generation_id='x' AND destination='global' AND session_id='x' AND event_id='x'`,
		`SELECT root_id FROM memory_compiler_diagnostic_foreground WHERE session_id='x' AND root_id>'x' ORDER BY root_id LIMIT 32`,
	} {
		rows, e := f.db.Query("EXPLAIN QUERY PLAN " + query)
		if e != nil {
			t.Fatal(e)
		}
		var plans []string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if e = rows.Scan(&id, &parent, &unused, &detail); e != nil {
				t.Fatal(e)
			}
			plans = append(plans, detail)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			t.Fatal(e)
		}
		joined := strings.Join(plans, " ")
		if !strings.Contains(joined, "SEARCH") || strings.Contains(joined, "SCAN ") || strings.Contains(joined, "TEMP B-TREE") {
			t.Fatalf("unbounded plan %s => %s", query, joined)
		}
	}
}

func TestCompilerDiagnosticsInterruptedAttemptStaysIncomplete(t *testing.T) {
	f := newWorkerFixture(t)
	job := f.queue(t, "source")
	claim, err := f.store.claimCompilerJob(context.Background(), f.owner, job.JobID, &workerScript{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`UPDATE memory_compiler_jobs SET lease_until=unixepoch('now')-1 WHERE job_id=?`, job.JobID); err != nil {
		t.Fatal(err)
	}
	second := f.second(t)
	if err = second.RecoverCompilerWork(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.store = second
	got := diagnosticRead(t, f, "jobs")
	m := got.Jobs[0].Measurements[0]
	if m.Fence != claim.Fence || m.ObservedOutcome != "incomplete" || m.InferenceNanos != nil || m.DatabaseCompletionNanos != nil {
		t.Fatalf("invented crash durations %+v", m)
	}
}

func TestCompilerDiagnosticsForegroundBoundHistory(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	root := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "private"})
	now := time.Now().UnixMilli()
	commit, done := int64(100), int64(300)
	h := f.store.BindHistory(f.owner.SessionID, f.lease.HolderID)
	m := memory.CompilerForegroundMeasurement{RootID: root.ID, StartedAtUnixMS: now - 1, TerminalCommittedAtUnixMS: &now, TerminalCommitNanos: &commit, ResponseFinalizedAtUnixMS: &now, ResponseFinalizationNanos: &done, Outcome: "success"}
	if err := h.RecordCompilerForeground(ctx, m); err != nil {
		t.Fatal(err)
	}
	got := diagnosticRead(t, f, "foreground")
	if len(got.Foreground) != 1 || *got.Foreground[0].TerminalCommitNanos != 100 || *got.Foreground[0].ResponseFinalizationNanos != 300 {
		t.Fatalf("boundaries %+v", got)
	}
	other, err := f.store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.store.BindHistory(other.ID, "other").RecordCompilerForeground(ctx, m); !errors.Is(err, ErrOwnerReviewUnauthorized) {
		t.Fatalf("crosssession record %v", err)
	}
}

func TestCompilerDiagnosticsPendingUnitsAndEmptyCandidatePages(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	activationStart(t, f)
	root := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "still writing"})
	activationReconcile(t, f, 2)
	roots := diagnosticRead(t, f, "live_roots")
	if len(roots.LiveRoots) != 1 || roots.LiveRoots[0].RootID != root.ID || roots.LiveRoots[0].State != "deferred_live" {
		t.Fatalf("pending %+v", roots.LiveRoots)
	}
	activationAppend(t, f, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "done"})
	activationReconcile(t, f, 3)
	units := diagnosticRead(t, f, "selections")
	if len(units.Selections) != 1 || units.Selections[0].State != "queued" || units.Selections[0].SelectedNewEvents == 0 {
		t.Fatalf("units %+v", units.Selections)
	}
	if worked, err := f.store.RunCompilerStep(ctx, f.config(&workerScript{})); !worked || err != nil {
		t.Fatalf("complete %v %v", worked, err)
	}
	a, err := f.store.LocalOwnerReviewContext(ctx, "global")
	if err != nil {
		t.Fatal(err)
	}
	q := memory.CompilerDiagnosticsQuery{SessionID: f.owner.SessionID, View: "candidates", Limit: 1}
	page, err := f.store.InspectOwnerCompilerDiagnostics(ctx, a, q)
	if err != nil || len(page.Candidates) != 0 || page.NextCursor == "" {
		t.Fatalf("empty job %+v %v", page, err)
	}
	q.Cursor = page.NextCursor
	page, err = f.store.InspectOwnerCompilerDiagnostics(ctx, a, q)
	if err != nil || len(page.Candidates) != 0 || page.NextCursor != "" {
		t.Fatalf("empty termination %+v %v", page, err)
	}
}

func TestCompilerDiagnosticsNeverRenderSecretSelectionIdentifier(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	root := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "source"})
	end := activationAppend(t, f, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "done"})
	req := memory.CompilerHistoryRequest{RequestID: "sk-abcdefghijklmnopqrstuvwx", Ranges: []memory.CompilerHistoryRange{historyRange(f, root, end)}}
	if _, err := f.store.SelectCompilerHistory(ctx, []memory.ScopeContext{f.owner}, req, f.generation, &activationScript{}); err != nil {
		t.Fatal(err)
	}
	a, err := f.store.LocalOwnerReviewContext(ctx, "global")
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.store.InspectOwnerCompilerDiagnostics(ctx, a, memory.CompilerDiagnosticsQuery{SessionID: f.owner.SessionID, View: "history"})
	if !errors.Is(err, ErrReviewInvalidSource) || len(out.History) != 0 || out.NextCursor != "" {
		t.Fatalf("secret identifier escaped %+v %v", out, err)
	}
}

func TestCompilerDiagnosticsResumedEmptyActivationDoesNotHideNewSegment(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	selector := memory.CompilerLiveSelector{SourceScope: "global", Destination: "global"}
	first, err := f.store.ActivateCompiler(ctx, f.owner, memory.CompilerActivationRequest{RequestID: "empty-first", Selector: selector}, f.generation, &activationScript{})
	if err != nil {
		t.Fatal(err)
	}
	paused, err := f.store.DisableCompilerActivation(ctx, f.owner, memory.CompilerActivationRequest{RequestID: "empty-pause", ActivationID: first.ID, ExpectedRevision: first.Revision, Selector: selector})
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.store.ActivateCompiler(ctx, f.owner, memory.CompilerActivationRequest{RequestID: "empty-second", ExpectedRevision: paused.Revision, Selector: selector}, f.generation, &activationScript{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ResumeCompilerActivation(ctx, f.owner, memory.CompilerActivationRequest{RequestID: "empty-resume-old", ActivationID: first.ID, ExpectedRevision: second.Revision, Selector: selector}, f.generation, &activationScript{}); err != nil {
		t.Fatal(err)
	}
	e := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "selected after empty segment"})
	a, err := f.store.LocalOwnerReviewContext(ctx, "global")
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.store.InspectOwnerCompilerDiagnostics(ctx, a, memory.CompilerDiagnosticsQuery{SessionID: f.owner.SessionID, View: "selection", GenerationID: f.generationID})
	if err != nil || len(got.Selection) != 1 || got.Selection[0].EventID != e.ID || got.Selection[0].Membership != "selected_live" {
		t.Fatalf("empty older segment hid selection %+v %v", got, err)
	}
}

func TestCompilerDiagnosticsScopeNavigationBeforeAnyCandidates(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	project, err := f.store.RegisterProject(ctx, "Compiler only", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := f.store.CreateProjectSession(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	scope := "project:" + string(project.ID)
	act, err := f.store.ActivateCompiler(ctx, session.ScopeContext(), memory.CompilerActivationRequest{RequestID: "compiler-only-project", Selector: memory.CompilerLiveSelector{SourceScope: scope, Destination: scope}}, f.generation, &activationScript{})
	if err != nil {
		t.Fatal(err)
	}
	assertVisible := func() {
		t.Helper()
		listed, e := f.store.ListLocalOwnerCandidateScopes(ctx, memory.OwnerCandidateScopeQuery{})
		if e != nil {
			t.Fatal(e)
		}
		found := false
		for _, s := range listed.Scopes {
			found = found || s.ScopeKey == scope
		}
		if !found {
			t.Fatalf("pre-candidate compiler scope absent %+v", listed)
		}
		a, e := f.store.LocalOwnerReviewContext(ctx, scope)
		if e != nil {
			t.Fatal(e)
		}
		sessions, e := f.store.ListOwnerCompilerSessions(ctx, a, memory.CompilerDiagnosticSessionQuery{})
		if e != nil || len(sessions.SessionIDs) != 1 || sessions.SessionIDs[0] != session.ID {
			t.Fatalf("navigation %+v %v", sessions, e)
		}
	}
	assertVisible()
	// An old selected scope is indexed in a bounded independent transaction.
	if _, err = f.db.Exec(`DELETE FROM memory_review_inbox_revisions WHERE scope_key=?; UPDATE memory_compiler_diagnostic_navigation SET cohort=0,last_id='',last_ordinal=-1,activation_cutoff=?,history_cutoff='',job_cutoff=''`, scope, act.ID); err != nil {
		t.Fatal(err)
	}
	assertVisible()
	a, err := f.store.LocalOwnerReviewContext(ctx, "global")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.InspectOwnerCompilerDiagnostics(ctx, a, memory.CompilerDiagnosticsQuery{SessionID: session.ID}); !errors.Is(err, ErrOwnerReviewUnauthorized) {
		t.Fatalf("cross source lineage escaped %v", err)
	}
}

func TestCompilerDiagnosticsBackwardClockWaitIsUnavailable(t *testing.T) {
	f := newWorkerFixture(t)
	job := f.queue(t, "source")
	if _, err := f.db.Exec(`UPDATE memory_compiler_diagnostic_jobs SET ready_at=unixepoch('now')*1000+60000 WHERE job_id=?`, job.JobID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.claimCompilerJob(context.Background(), f.owner, job.JobID, &workerScript{}); err != nil {
		t.Fatal(err)
	}
	got := diagnosticRead(t, f, "jobs")
	if len(got.Jobs) != 1 || len(got.Jobs[0].Measurements) != 1 || got.Jobs[0].Measurements[0].QueueWaitNanos != nil {
		t.Fatalf("backward clock became zero wait %+v", got.Jobs)
	}
}
