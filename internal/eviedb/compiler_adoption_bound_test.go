package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

type compilerCountingStageTransaction struct {
	*sql.Conn
	eventReads int
}

func (q *compilerCountingStageTransaction) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if strings.Contains(query, "FROM events WHERE id=?") {
		q.eventReads++
	}
	return q.Conn.QueryRowContext(ctx, query, args...)
}

func TestCompilerWorkerAdoptionReadsMaximumWindowOnce(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	appendSource := func(kind memory.EventType, role memory.EventRole, parent memory.EventID) memory.Event {
		event, err := f.store.AppendEventWithLease(ctx, f.owner.SessionID, f.lease.HolderID, f.lease.FencingToken, memory.EventInput{Type: kind, Role: role, ParentID: parent, Content: "A concise fact."})
		if err != nil {
			t.Fatal(err)
		}
		return event
	}
	overlapRoot := appendSource(memory.EventUserMessage, memory.RoleUser, "")
	for i := 1; i < 16; i++ {
		appendSource(memory.EventUserMessage, memory.RoleUser, overlapRoot.ID)
	}
	root := appendSource(memory.EventUserMessage, memory.RoleUser, "")
	for i := 1; i < 64; i++ {
		appendSource(memory.EventUserMessage, memory.RoleUser, root.ID)
	}
	var end memory.Event
	for i := 0; i < 8; i++ {
		end = appendSource(memory.EventAssistantMessage, memory.RoleAssistant, root.ID)
	}
	selection := memory.CompilationSelection{SessionID: f.owner.SessionID, RootID: root.ID, Cutoff: end.Sequence, Destination: "global"}
	queued, err := f.store.QueueCandidateUnit(ctx, f.owner, selection, f.generation, &workerScript{})
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, source := range queued.Window.Sources {
		counts[source.Usage]++
	}
	if counts["new_support"] != 64 || counts["overlap"] != 16 || counts["context"] != 8 {
		t.Fatalf("not a maximum window: %v", counts)
	}
	claim, err := f.store.claimCompilerJob(ctx, f.owner, queued.JobID, &workerScript{})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.stageCompilerResult(ctx, f.owner, claim.JobID, claim.Holder, claim.Fence, claim.Request, []memory.MemoryCandidate{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE memory_compiler_jobs SET lease_until=unixepoch('now')-1 WHERE job_id=?`, claim.JobID); err != nil {
		t.Fatal(err)
	}
	if err := f.store.RecoverCompilerWork(ctx); err != nil {
		t.Fatal(err)
	}
	var adopted compilerClaim
	err = f.store.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		counted := &compilerCountingStageTransaction{Conn: conn}
		var err error
		adopted, err = adoptCompilerStageInTransaction(ctx, counted, f.owner, queued.JobID)
		if err != nil {
			return err
		}
		if counted.eventReads != 88 || counted.eventReads > 128 {
			t.Errorf("adoption inspected %d event rows; maximum window must be checked once (88), bounded by128", counted.eventReads)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Eligibility can change between the adoption and publication transactions.
	// A completed group still requires a fresh pass over all currently stored
	// source metadata; saved validation cannot authorize changed source bytes.
	// Inject corruption below the append-only API to exercise fresh validation.
	if _, err := f.db.Exec(`DROP TRIGGER events_append_only_update`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE events SET content='Changed after adoption.' WHERE id=?`, root.ID); err != nil {
		t.Fatal(err)
	}
	err = f.store.publishCompilerResult(ctx, f.owner, adopted.JobID, adopted.Holder, adopted.Fence, adopted.Request)
	if err == nil || errors.Is(err, ErrCompilerFence) {
		t.Fatalf("publication did not revalidate changed source: %v", err)
	}
	for _, table := range []string{"memory_compiler_candidate_groups", "memory_compiler_coverage"} {
		var count int
		if err := f.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("changed source published %s rows=%d err=%v", table, count, err)
		}
	}
}
