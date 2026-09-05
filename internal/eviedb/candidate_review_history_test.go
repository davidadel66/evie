package eviedb_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

func TestOwnerReviewReplayRejectsChangedHistoricalEvidence(t *testing.T) {
	for _, kind := range []string{"support", "context"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			f, compiled, authority := reviewCandidateFixture(t)
			preview, err := f.store.PrepareOwnerCandidateReview(ctx, authority, candidateRef(compiled), "accept")
			if err != nil {
				t.Fatal(err)
			}
			accepted, err := f.store.ResolveOwnerCandidateReview(ctx, authority, decisionFor(preview, "90000000-0000-4000-8000-000000000170"))
			if err != nil {
				t.Fatal(err)
			}
			source := compiled.Candidates[0].Support[0]
			content := "I prefer caff and tea."
			if kind == "context" {
				source = compiled.Candidates[0].Context[0]
				content = "Not recorded."
			}
			// Only the test disables immutability to simulate damaged storage.
			if _, err := f.db.Exec(`DROP TRIGGER events_append_only_update`); err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.Exec(`UPDATE events SET content=? WHERE id=?`, content, source.Locator.EventID); err != nil {
				t.Fatal(err)
			}
			assertReviewHistoryQuarantined(t, f, accepted.Operation.OperationID)
		})
	}
}

func TestOwnerReviewReplayRejectsChangedHistoricalMetadata(t *testing.T) {
	for _, kind := range []string{"support", "context"} {
		for _, mutation := range []struct {
			name, statement string
			value           any
		}{
			{"role", `UPDATE events SET role=? WHERE id=?`, "tool"},
			{"event_type", `UPDATE events SET event_type=? WHERE id=?`, "tool_result"},
			{"format_version", `UPDATE events SET format_version=? WHERE id=?`, 2},
			{"sequence", `UPDATE events SET sequence=? WHERE id=?`, 99},
			{"observed_at", `UPDATE events SET recorded_at=? WHERE id=?`, "2020-01-01T00:00:00.000000000Z"},
			{"invalid_utf8", `UPDATE events SET content=? WHERE id=?`, "I prefer café and tea.\xff"},
		} {
			t.Run(kind+"/"+mutation.name, func(t *testing.T) {
				ctx := context.Background()
				f, compiled, authority := reviewCandidateFixture(t)
				preview, err := f.store.PrepareOwnerCandidateReview(ctx, authority, candidateRef(compiled), "accept")
				if err != nil {
					t.Fatal(err)
				}
				accepted, err := f.store.ResolveOwnerCandidateReview(ctx, authority, decisionFor(preview, "90000000-0000-4000-8000-000000000171"))
				if err != nil {
					t.Fatal(err)
				}
				source := compiled.Candidates[0].Support[0]
				if kind == "context" {
					source = compiled.Candidates[0].Context[0]
				}
				if _, err := f.db.Exec(`DROP TRIGGER events_append_only_update`); err != nil {
					t.Fatal(err)
				}
				if _, err := f.db.Exec(mutation.statement, mutation.value, source.Locator.EventID); err != nil {
					t.Fatal(err)
				}
				assertReviewHistoryQuarantined(t, f, accepted.Operation.OperationID)
			})
		}
	}
}

func TestOwnerReviewReplayRejectsChangedHistoricalLineage(t *testing.T) {
	for _, kind := range []string{"support", "context"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			f, compiled, authority := reviewCandidateFixture(t)
			preview, err := f.store.PrepareOwnerCandidateReview(ctx, authority, candidateRef(compiled), "accept")
			if err != nil {
				t.Fatal(err)
			}
			accepted, err := f.store.ResolveOwnerCandidateReview(ctx, authority, decisionFor(preview, "90000000-0000-4000-8000-000000000172"))
			if err != nil {
				t.Fatal(err)
			}
			project, err := f.store.RegisterProject(ctx, "foreign history", t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			source := compiled.Candidates[0].Support[0]
			if kind == "context" {
				source = compiled.Candidates[0].Context[0]
			}
			if _, err := f.db.Exec(`DROP TRIGGER events_append_only_update`); err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.Exec(`UPDATE events SET project_id=? WHERE id=?`, project.ID, source.Locator.EventID); err != nil {
				t.Fatal(err)
			}
			assertReviewHistoryQuarantined(t, f, accepted.Operation.OperationID)
		})
	}
}

func TestOwnerReviewReplayRejectsContextMovedToAnotherSession(t *testing.T) {
	ctx := context.Background()
	f, compiled, authority := reviewCandidateFixture(t)
	preview, err := f.store.PrepareOwnerCandidateReview(ctx, authority, candidateRef(compiled), "accept")
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := f.store.ResolveOwnerCandidateReview(ctx, authority, decisionFor(preview, "90000000-0000-4000-8000-000000000173"))
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := f.store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`DROP TRIGGER events_append_only_update`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE events SET session_id=?,parent_id=NULL WHERE id=?`, foreign.ID, compiled.Candidates[0].Context[0].Locator.EventID); err != nil {
		t.Fatal(err)
	}
	assertReviewHistoryQuarantined(t, f, accepted.Operation.OperationID)
}

func TestOwnerReviewReplayChecksHistoricalRangeBoundaries(t *testing.T) {
	for _, content := range []string{"I prefer caf€ and tea.", "I prefer caf", strings.Repeat("x", 32769)} {
		t.Run(content[:12], func(t *testing.T) {
			ctx := context.Background()
			f, compiled, authority := reviewCandidateFixture(t)
			preview, err := f.store.PrepareOwnerCandidateReview(ctx, authority, candidateRef(compiled), "accept")
			if err != nil {
				t.Fatal(err)
			}
			accepted, err := f.store.ResolveOwnerCandidateReview(ctx, authority, decisionFor(preview, "90000000-0000-4000-8000-000000000174"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.Exec(`DROP TRIGGER events_append_only_update`); err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.Exec(`UPDATE events SET content=? WHERE id=?`, content, compiled.Candidates[0].Support[0].Locator.EventID); err != nil {
				t.Fatal(err)
			}
			assertReviewHistoryQuarantined(t, f, accepted.Operation.OperationID)
		})
	}
}

func TestOwnerReviewReplayIgnoresCurrentAuthorityAndRegistryAvailability(t *testing.T) {
	ctx := context.Background()
	f := newCompilerFixture(t)
	if err := f.store.ReleaseTurnLease(ctx, f.session.ID, f.lease.HolderID, f.lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	workspace, err := f.store.RegisterWorkspace(ctx, "historical review")
	if err != nil {
		t.Fatal(err)
	}
	f.session, err = f.store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, reviewTestReceipt())
	if err != nil {
		t.Fatal(err)
	}
	f.lease, err = f.store.AcquireTurnLease(ctx, f.session.ID, "historical-review", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	selection := f.selection(t, "I prefer tea.", true)
	selection.Destination = "workspace:" + string(workspace.ID)
	extractor := &scriptedCompiler{run: func(_ context.Context, request memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		candidate := f.candidate(request)
		for _, source := range request.Window.Sources {
			if source.Usage == "context" {
				candidate.Context = append(candidate.Context, source.Locator)
			}
		}
		return compilerOutput(request, []memory.ExtractorCandidate{candidate}), nil
	}}
	compiled, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), selection, compilerGeneration(), extractor)
	if err != nil || compiled.State != "completed_candidates" {
		t.Fatalf("compile historical workspace candidate: %+v %v", compiled, err)
	}
	authority, err := f.store.LocalOwnerReviewContext(ctx, selection.Destination)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.store.PrepareOwnerCandidateReview(ctx, authority, candidateRef(compiled), "accept")
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := f.store.ResolveOwnerCandidateReview(ctx, authority, decisionFor(preview, "90000000-0000-4000-8000-000000000175"))
	if err != nil {
		t.Fatal(err)
	}
	var before string
	if err := f.db.QueryRow(`SELECT prepared_proposal_json FROM semantic_operations WHERE operation_id=?`, accepted.Operation.OperationID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := f.store.ReleaseTurnLease(ctx, f.session.ID, f.lease.HolderID, f.lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE sessions SET status='closed' WHERE id=?`, f.session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ArchiveWorkspace(ctx, workspace.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE memory_review_authorization SET source_policy='changed-detector-v2',revision=revision+1,authentication_key=randomblob(32)`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.LocalOwnerReviewContext(ctx, selection.Destination); !errors.Is(err, eviedb.ErrOwnerReviewUnauthorized) {
		t.Fatalf("archived Workspace still grants live review authority: %v", err)
	}
	report, err := f.store.VerifySemanticProjection(ctx)
	if err != nil || !report.Valid {
		t.Fatalf("current authorization or policy changed historical replay: %+v %v", report, err)
	}
	var after, status string
	if err := f.db.QueryRow(`SELECT prepared_proposal_json FROM semantic_operations WHERE operation_id=?`, accepted.Operation.OperationID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`SELECT status FROM sessions WHERE id=?`, f.session.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if before != after || status != "closed" {
		t.Fatal("historical replay changed accepted authority or reopened the source session")
	}
}

func assertReviewHistoryQuarantined(t *testing.T, f *compilerFixture, operationID memory.SemanticID) {
	t.Helper()
	report, err := f.store.VerifySemanticProjection(context.Background())
	var replayErr *eviedb.SemanticReplayError
	if report.Valid || !errors.As(err, &replayErr) || replayErr.SchemaVersion != 6 || replayErr.OperationID != operationID {
		t.Fatalf("corrupt review history was not rejected at its accepted operation: report=%+v err=%v", report, err)
	}
	var quarantined int
	if err := f.db.QueryRow(`SELECT count(*) FROM semantic_projection_quarantine q JOIN semantic_scopes s ON s.scope_id=q.scope_id WHERE s.scope_key='global' AND q.operation_id=?`, operationID).Scan(&quarantined); err != nil {
		t.Fatal(err)
	}
	if quarantined != 1 {
		t.Fatal("corrupt review history did not quarantine the affected scope")
	}
}
