package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

func TestCompilerActivationStatusFollowsJobAndPreservesPendingSuffix(t *testing.T) {
	for _, outcome := range []string{"completed_empty", "retry_wait", "failed", "cancelled"} {
		t.Run(outcome, func(t *testing.T) {
			f := newWorkerFixture(t)
			ctx := context.Background()
			activationStart(t, f)
			root := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer tea."})
			activationAppend(t, f, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "Noted."})
			activationReconcile(t, f, 3)
			before, err := f.store.InspectCompilerActivations(ctx, f.owner)
			if err != nil {
				t.Fatal(err)
			}
			if len(before.Roots) != 1 || before.Roots[0].State != "queued" {
				t.Fatalf("initial roots %+v", before.Roots)
			}
			if outcome == "cancelled" {
				if _, err := f.store.CancelCompilation(ctx, f.owner, before.Roots[0].SelectionID); err != nil {
					t.Fatal(err)
				}
			} else {
				extractor := &workerScript{}
				if outcome == "retry_wait" {
					extractor.run = func(context.Context, memory.CompilerRequest) (CompilerExtraction, error) {
						return CompilerExtraction{ReleaseEvidence: "not_dispatched"}, errors.New("unavailable fixture")
					}
				}
				if outcome == "failed" {
					extractor.run = func(context.Context, memory.CompilerRequest) (CompilerExtraction, error) {
						return CompilerExtraction{ReleaseEvidence: "not_dispatched"}, ErrCompilerConfiguration
					}
				}
				worked, err := f.store.RunCompilerStep(ctx, f.config(extractor))
				if !worked || (outcome == "completed_empty" && err != nil) || (outcome != "completed_empty" && err == nil) {
					t.Fatalf("worker outcome %v %v", worked, err)
				}
			}
			status, err := f.store.InspectCompilerActivations(ctx, f.owner)
			if err != nil {
				t.Fatal(err)
			}
			if len(status.Roots) != 1 || status.Roots[0].State != outcome {
				t.Fatalf("status did not follow %s: %+v", outcome, status.Roots)
			}
			if outcome == "completed_empty" {
				activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, ParentID: root.ID, Content: "A newly selected suffix."})
				if err := f.store.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
					var r memory.CompilerReconciliation
					return discoverCompilerEvidenceInTransaction(ctx, conn, &r)
				}); err != nil {
					t.Fatal(err)
				}
				pending, err := f.store.InspectCompilerActivations(ctx, f.owner)
				if err != nil {
					t.Fatal(err)
				}
				if pending.Roots[0].State != "selected_unmaterialized" || pending.PendingRoots != 1 {
					t.Fatalf("finished prefix hid pending suffix: %+v", pending)
				}
			} else {
				if _, err := f.db.Exec(`UPDATE memory_compiler_jobs SET reason='protected raw diagnostic'`); err != nil {
					t.Fatal(err)
				}
				safe, err := f.store.InspectCompilerActivations(ctx, f.owner)
				if err != nil {
					t.Fatal(err)
				}
				if safe.Roots[0].Reason != "unavailable_detail" {
					t.Fatalf("unsafe reason escaped: %+v", safe.Roots)
				}
			}
		})
	}
}
