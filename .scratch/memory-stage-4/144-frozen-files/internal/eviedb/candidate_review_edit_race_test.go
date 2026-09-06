package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

func TestOwnerReviewEditAcceptCompetingStoresBothWinningOrders(t *testing.T) {
	for _, first := range []string{"edit", "accept"} {
		t.Run(first, func(t *testing.T) {
			ctx := context.Background()
			f, a, refs := batchBoundaryFixture(t, 1)
			item, err := f.store.InspectOwnerCandidate(ctx, a, refs[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			p, err := f.store.PrepareOwnerCandidateReview(ctx, a, item.Ref, "accept")
			if err != nil {
				t.Fatal(err)
			}
			db, err := OpenDBAt(f.path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			other := NewStore(db)
			held, release := make(chan struct{}), make(chan struct{})
			f.store.resolveImmediateTransaction = func(ctx context.Context, c *sql.Conn, statement string) (sql.Result, error) {
				if statement == "COMMIT" {
					close(held)
					<-release
				}
				return executeImmediateTransactionStatement(ctx, c, statement)
			}
			edit := memory.ReviewEditDecision{Candidate: item.Ref, Proposal: item.Candidate.Proposal, Reason: "Review the interpretation again."}
			decision := memory.ReviewDecision{DeliveryKey: "idem:v1:90000000-0000-4000-8000-000000000357", PreviewID: p.ID, PreviewSHA256: p.SHA256, Action: "accept"}
			run := func(s *Store, kind string) error {
				if kind == "edit" {
					_, err := s.EditOwnerCandidate(ctx, a, edit)
					return err
				}
				_, err := s.ResolveOwnerCandidateReview(ctx, a, decision)
				return err
			}
			firstDone, secondDone := make(chan error, 1), make(chan error, 1)
			go func() { firstDone <- run(f.store, first) }()
			<-held
			second := "accept"
			if first == "accept" {
				second = "edit"
			}
			go func() { secondDone <- run(other, second) }()
			close(release)
			if err = <-firstDone; err != nil {
				t.Fatal(err)
			}
			f.store.resolveImmediateTransaction = executeImmediateTransactionStatement
			err = <-secondDone
			want := ErrReviewStale
			if first == "accept" {
				want = ErrReviewResolved
			}
			if !errors.Is(err, want) {
				t.Fatalf("loser %v want%v", err, want)
			}
			var edits, ops int
			if err = f.db.QueryRow(`SELECT (SELECT count(*) FROM memory_review_edit_revisions),(SELECT count(*) FROM semantic_operations WHERE schema_version=6)`).Scan(&edits, &ops); err != nil {
				t.Fatal(err)
			}
			if first == "edit" && (edits != 1 || ops != 0) || first == "accept" && (edits != 0 || ops != 1) {
				t.Fatalf("competing atomic effects %d/%d", edits, ops)
			}
		})
	}
}
func TestOwnerReviewBatchRedactedRejectionAndDeliveryNamespace(t *testing.T) {
	ctx := context.Background()
	f, a, refs := batchBoundaryFixture(t, 2)
	if _, err := f.db.Exec(`UPDATE memory_review_authorization SET source_policy='detector-next'`); err != nil {
		t.Fatal(err)
	}
	p, err := f.store.PrepareOwnerCandidateBatch(ctx, a, boundaryBatchRequest(refs, 2, "reject"))
	if err != nil {
		t.Fatal(err)
	}
	if !p.Groups[0].Preview.Candidates[0].Redacted || len(p.Groups[0].Preview.Candidates[0].Candidate.Support) != 0 {
		t.Fatal("redaction leaked disclosure")
	}
	inspected, err := f.store.InspectOwnerCandidateBatch(ctx, a, p.ID)
	if err != nil || string(compilerJSON(inspected)) != string(compilerJSON(p)) {
		t.Fatalf("safe redacted preview %+v %v", inspected, err)
	}
	d := boundaryBatchDecision(p)
	r, err := f.store.ResolveOwnerCandidateBatch(ctx, a, d)
	if err != nil || r.Groups[0].Outcome != "rejected" || r.Groups[1].Outcome != "rejected" {
		t.Fatalf("safe rejection %+v %v", r, err)
	}
	_, err = f.store.ResolveOwnerCandidateReview(ctx, a, memory.ReviewDecision{DeliveryKey: d.DeliveryKey, PreviewID: p.Groups[0].Preview.ID, PreviewSHA256: p.Groups[0].Preview.SHA256, Action: "reject"})
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("single/batch key collision %v", err)
	}
	var ops int
	if err = f.db.QueryRow(`SELECT count(*) FROM semantic_operations WHERE schema_version=6`).Scan(&ops); err != nil || ops != 0 {
		t.Fatalf("rejection effects%d %v", ops, err)
	}
}

func TestOwnerReviewBatchCompetingTerminalMembersReturnExactPriorResolutions(t *testing.T) {
	ctx := context.Background()
	f, a, all := batchBoundaryFixture(t, 17)
	refs := all[:4]
	p, err := f.store.PrepareOwnerCandidateBatch(ctx, a, memory.ReviewBatchRequest{Groups: []memory.ReviewBatchGroupRequest{{ID: "competing", Action: "accept", Candidates: refs, Dependencies: []memory.ReviewDependency{}}, {ID: "independent", Action: "accept", Candidates: all[16:], Dependencies: []memory.ReviewDependency{}}}})
	if err != nil {
		t.Fatal(err)
	}
	// The first two members share one winning rejection; the third has a distinct
	// winner. The fourth remains unresolved, so this current group cannot commit.
	rejection, err := f.store.PrepareOwnerCandidateBatch(ctx, a, boundaryBatchRequest(refs[:2], 1, "reject"))
	if err != nil {
		t.Fatal(err)
	}
	rejectDecision := boundaryBatchDecision(rejection)
	rejectDecision.DeliveryKey = "idem:v1:90000000-0000-4000-8000-000000000358"
	first, err := f.store.ResolveOwnerCandidateBatch(ctx, a, rejectDecision)
	if err != nil {
		t.Fatal(err)
	}
	single, err := f.store.PrepareOwnerCandidateReview(ctx, a, refs[2], "reject")
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.store.ResolveOwnerCandidateReview(ctx, a, memory.ReviewDecision{DeliveryKey: "idem:v1:90000000-0000-4000-8000-000000000359", PreviewID: single.ID, PreviewSHA256: single.SHA256, Action: "reject"})
	if err != nil {
		t.Fatal(err)
	}
	// Keep vector, policy and exact text unchanged; current metadata makes the
	// losing group's support ineligible. Winners must remain visible nonetheless.
	if _, err = f.db.Exec(`DROP TRIGGER events_append_only_update`); err != nil {
		t.Fatal(err)
	}
	event := p.Groups[0].Preview.Candidates[0].Candidate.Support[0].Locator.EventID
	if _, err = f.db.Exec(`UPDATE events SET role='assistant' WHERE id=?`, event); err != nil {
		t.Fatal(err)
	}
	d := boundaryBatchDecision(p)
	result, err := f.store.ResolveOwnerCandidateBatch(ctx, a, d)
	if err != nil || len(result.Groups) != 2 {
		t.Fatalf("terminal receipt %+v %v", result, err)
	}
	failed := result.Groups[0]
	expected := []memory.ReviewResult{*first.Groups[0].Result, second}
	if failed.Outcome != "failed" || failed.FailureCode != "already_resolved" || failed.Result != nil || string(compilerJSON(failed.PriorResolutions)) != string(compilerJSON(expected)) || result.Groups[1].Outcome != "accepted" {
		t.Fatalf("winning resolutions %+v", result)
	}
	for i, ref := range refs {
		c, err := f.store.InspectOwnerCandidate(ctx, a, ref.ID)
		if err != nil {
			t.Fatal(err)
		}
		if i < 3 && c.Candidate.ReviewState != "rejected" || i == 3 && (c.Candidate.ReviewState != "unresolved" || c.Ref != ref) {
			t.Fatalf("losing member mutated %+v", c)
		}
	}
	var count int
	if err = f.db.QueryRow(`SELECT count(*) FROM memory_review_resolutions WHERE candidate_id=?`, refs[3].ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("unresolved resolution %d %v", count, err)
	}
	if err = f.db.Close(); err != nil {
		t.Fatal(err)
	}
	f.db, err = OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.store = NewStore(f.db)
	again, err := f.store.ResolveOwnerCandidateBatch(ctx, a, d)
	if err != nil || string(compilerJSON(again)) != string(compilerJSON(result)) {
		t.Fatalf("terminal retry %+v %v", again, err)
	}
	if _, err = f.db.Exec(`UPDATE memory_review_authorization SET revision=revision+1`); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ResolveOwnerCandidateBatch(ctx, a, d); !errors.Is(err, ErrOwnerReviewUnauthorized) {
		t.Fatalf("revoked authority obtained recorded result %v", err)
	}
}
