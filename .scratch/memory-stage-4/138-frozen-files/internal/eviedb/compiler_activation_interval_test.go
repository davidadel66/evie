package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/davidadel66/evie/internal/memory"
	"testing"
)

func TestCompilerActivationResumedEarlierSegmentRetainsItsOwnInterval(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	a := activationStart(t, f)
	root := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer tea."})
	req := activationRequest(f, "disable-original", 1)
	req.ActivationID = a.ID
	if _, err := f.store.DisableCompilerActivation(ctx, f.owner, req); err != nil {
		t.Fatal(err)
	}
	gap := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, ParentID: root.ID, Content: "Disabled interval remains historical."})
	b, err := f.store.ActivateCompiler(ctx, f.owner, activationRequest(f, "activate-next", 2), f.generation, &activationScript{})
	if err != nil {
		t.Fatal(err)
	}
	end := activationAppend(t, f, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "Noted."})
	activationReconcile(t, f, 4)
	var laterWindow []byte
	if err := f.db.QueryRow(`SELECT window FROM memory_compiler_selections`).Scan(&laterWindow); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE session_turn_leases SET expires_at='2000-01-01T00:00:00Z' WHERE session_id=?`, f.owner.SessionID); err != nil {
		t.Fatal(err)
	}
	req = activationRequest(f, "resume-original", 3)
	req.ActivationID = a.ID
	if _, err := f.store.ResumeCompilerActivation(ctx, f.owner, req, f.generation, &activationScript{}); err != nil {
		t.Fatal(err)
	}
	activationReconcile(t, f, 6)
	var oldSel, newSel, oldState, newState string
	if err := f.db.QueryRow(`SELECT selection_id,state FROM memory_compiler_activation_roots WHERE activation_id=?`, a.ID).Scan(&oldSel, &oldState); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`SELECT selection_id,state FROM memory_compiler_activation_roots WHERE activation_id=?`, b.ID).Scan(&newSel, &newState); err != nil {
		t.Fatal(err)
	}
	var first, last int64
	if err := f.db.QueryRow(`SELECT first_sequence,last_sequence FROM memory_compiler_selections WHERE selection_id=?`, oldSel).Scan(&first, &last); err != nil {
		t.Fatal(err)
	}
	t.Logf("original root=%d disabled gap=%d new final=%d; A state=%s selection=%s; B state=%s selection=%s; adopted interval=%d..%d", root.Sequence, gap.Sequence, end.Sequence, oldState, oldSel, newState, newSel, first, last)
	if oldSel == newSel || first != root.Sequence || last != root.Sequence {
		t.Fatalf("old selected prefix was replaced by later suffix; root assertion remains unowned")
	}
	var sameWindow []byte
	if err := f.db.QueryRow(`SELECT window FROM memory_compiler_selections WHERE selection_id=?`, newSel).Scan(&sameWindow); err != nil {
		t.Fatal(err)
	}
	if string(sameWindow) != string(laterWindow) {
		t.Fatal("resuming earlier work changed the immutable later window")
	}
	if got := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_selections WHERE first_sequence<=? AND last_sequence>=?`, gap.Sequence, gap.Sequence); got != 0 {
		t.Fatal("disabled gap acquired ownership")
	}
	if got := activationCount(t, f.db, `SELECT COUNT(DISTINCT activation_id) FROM memory_compiler_activation_jobs`); got != 2 {
		t.Fatalf("ownership was attached to another activation: %d", got)
	}
	status, err := f.store.InspectCompilerActivations(ctx, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	if status.SelectedEvents != 2 || status.OutsideSelectionEvents != 1 {
		t.Fatalf("disabled gap no longer outside selection: %+v", status)
	}
	if worked, err := f.store.RunCompilerStep(ctx, f.config(&workerScript{})); !worked || err != nil {
		t.Fatalf("old selected assertion cannot compile: %v %v", worked, err)
	}
}

func TestCompilerActivationRevisitsRemainderAroundExistingHistoricalIntervals(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	a := activationStart(t, f)
	root := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "An owner assertion."})
	for n := 2; n <= 7; n++ {
		activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, ParentID: root.ID, Content: "Additional support."})
	}
	if _, err := f.db.Exec(`UPDATE session_turn_leases SET expires_at='2000-01-01T00:00:00Z' WHERE session_id=?`, f.owner.SessionID); err != nil {
		t.Fatal(err)
	}
	generation, manifest, err := memory.CompilerGenerationIdentity(f.generation)
	if err != nil {
		t.Fatal(err)
	}
	var pinned memory.CompilerGeneration
	if err := json.Unmarshal(manifest, &pinned); err != nil {
		t.Fatal(err)
	}
	if err := f.store.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		for _, point := range []int64{3, 5} {
			_, _, err := selectCompilerUnitInTransaction(ctx, conn, f.owner, memory.CompilationSelection{SessionID: f.owner.SessionID, RootID: root.ID, Cutoff: point, Destination: "global"}, generation, manifest, pinned, compilerScheduling{FirstSequence: point, Lane: "historical"})
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	activationReconcile(t, f, 12)
	if got := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_jobs`); got != 5 {
		t.Fatalf("remaining intervals were lost or duplicated: %d jobs", got)
	}
	if got := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_jobs a JOIN memory_compiler_jobs b ON a.job_id<b.job_id AND a.first_sequence<=b.last_sequence AND b.first_sequence<=a.last_sequence`); got != 0 {
		t.Fatal("activation overlapped an existing owner")
	}
	if got := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_activation_jobs`); got != 3 {
		t.Fatalf("historical jobs acquired activation authority: %d", got)
	}
	if got := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_activation_roots WHERE state IN ('selected_unmaterialized','deferred_live')`); got != 0 {
		t.Fatal("remainder not revisited")
	}
	req := activationRequest(f, "disable-new-work", 1)
	req.ActivationID = a.ID
	if _, err := f.store.DisableCompilerActivation(ctx, f.owner, req); err != nil {
		t.Fatal(err)
	}
	if worked, err := f.store.RunCompilerStep(ctx, f.config(&workerScript{})); !worked || err != nil {
		t.Fatalf("activation pause stopped independently selected history: %v %v", worked, err)
	}
	if got := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_jobs j JOIN memory_compiler_job_schedule q USING(job_id) WHERE j.state='completed_empty' AND q.lane='historical'`); got != 1 {
		t.Fatal("wrong authority/lane resumed")
	}
}
