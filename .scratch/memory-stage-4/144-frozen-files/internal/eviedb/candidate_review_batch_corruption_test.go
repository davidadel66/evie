package eviedb

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

func TestOwnerReviewBatchMalformedStoredPreviewFailsClosed(t *testing.T) {
	for _, mutation := range []string{"member_claims", "member_sources", "candidate_support", "duplicate_candidate", "nested_member", "missing_records", "unknown_member_version", "forward_dependency"} {
		t.Run(mutation, func(t *testing.T) {
			ctx := context.Background()
			f, a, refs := batchBoundaryFixture(t, 2)
			p, err := f.store.PrepareOwnerCandidateBatch(ctx, a, boundaryBatchRequest(refs, 1, "accept"))
			if err != nil {
				t.Fatal(err)
			}
			g := &p.Groups[0].Preview
			switch mutation {
			case "member_claims":
				g.Effect.Members[0].Claims = nil
				g.Dependencies = []memory.ReviewDependency{{CandidateID: refs[1].ID, Field: "subject", FromCandidateID: refs[0].ID, FromField: "subject"}}
				g.Effect.Dependencies = g.Dependencies
			case "member_sources":
				g.Effect.Members[0].Claims[0].Sources = nil
			case "candidate_support":
				g.Candidates[0].Candidate.Support = nil
			case "duplicate_candidate":
				g.Candidates[1] = g.Candidates[0]
			case "nested_member":
				g.Effect.Members[0].Members = []memory.ReviewEffect{{}}
			case "missing_records":
				g.Effect.Records = nil
			case "unknown_member_version":
				g.Effect.Members[0].Version = "owner-review-effect-v999"
			case "forward_dependency":
				g.Dependencies = []memory.ReviewDependency{{CandidateID: refs[0].ID, Field: "subject", FromCandidateID: refs[1].ID, FromField: "subject"}}
				g.Effect.Dependencies = g.Dependencies
			}
			g.EffectSHA256, _, _ = ownerReviewEffectHash(g.Effect)
			g.SHA256, _, _ = ownerReviewPreviewHash(*g)
			p.SHA256, _, _ = ownerReviewBatchHash(p)
			if err = validateOwnerReviewBatch(p); err == nil {
				t.Fatal("malformed encoding accepted")
			}
			if _, err = f.db.Exec(`DROP TRIGGER memory_review_batch_previews_no_update`); err != nil {
				t.Fatal(err)
			}
			if _, err = f.db.Exec(`UPDATE memory_review_batch_previews SET envelope=?,preview_sha256=? WHERE preview_id=?`, compilerJSON(p), p.SHA256, p.ID); err != nil {
				t.Fatal(err)
			}
			if _, err = f.store.InspectOwnerCandidateBatch(ctx, a, p.ID); err == nil {
				t.Fatal("malformed stored preview disclosed")
			}
			if _, err = f.store.ResolveOwnerCandidateBatch(ctx, a, boundaryBatchDecision(p)); err == nil {
				t.Fatal("malformed stored preview resolved")
			}
			var n int
			if err = f.db.QueryRow(`SELECT count(*) FROM memory_review_batch_deliveries`).Scan(&n); err != nil || n != 0 {
				t.Fatalf("malformed receipt %d %v", n, err)
			}
		})
	}
}
func TestOwnerReviewBatchMalformedAcceptedOperationQuarantinesReplay(t *testing.T) {
	for _, mutation := range []string{"member_claims", "candidate_support", "batch_binding"} {
		t.Run(mutation, func(t *testing.T) {
			ctx := context.Background()
			f, a, refs := batchBoundaryFixture(t, 2)
			p, err := f.store.PrepareOwnerCandidateBatch(ctx, a, boundaryBatchRequest(refs, 1, "accept"))
			if err != nil {
				t.Fatal(err)
			}
			result, err := f.store.ResolveOwnerCandidateBatch(ctx, a, boundaryBatchDecision(p))
			if err != nil {
				t.Fatal(err)
			}
			id := result.Groups[0].Result.Operation.OperationID
			var raw []byte
			var op memory.OwnerReviewOperation
			if err = f.db.QueryRow(`SELECT prepared_proposal_json FROM semantic_operations WHERE operation_id=?`, id).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal(raw, &op); err != nil {
				t.Fatal(err)
			}
			switch mutation {
			case "member_claims":
				op.Preview.Effect.Members[0].Claims = nil
				op.Preview.Dependencies = []memory.ReviewDependency{{CandidateID: refs[1].ID, Field: "subject", FromCandidateID: refs[0].ID, FromField: "subject"}}
				op.Preview.Effect.Dependencies = op.Preview.Dependencies
			case "candidate_support":
				op.Preview.Candidates[0].Candidate.Support = nil
			case "batch_binding":
				op.Batch = nil
			}
			op.Preview.EffectSHA256, _, _ = ownerReviewEffectHash(op.Preview.Effect)
			op.Preview.SHA256, _, _ = ownerReviewPreviewHash(op.Preview)
			if err = validateOwnerReviewOperation(op); err == nil {
				t.Fatal("malformed operation accepted")
			}
			hash, canonical, err := semanticHash(canonicalOwnerReviewOperation(op))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.db.Exec(`DROP TRIGGER semantic_operations_append_only_update`); err != nil {
				t.Fatal(err)
			}
			if _, err = f.db.Exec(`UPDATE semantic_operations SET prepared_proposal_json=?,proposal_json=?,proposal_sha256=?,effect_sha256=? WHERE operation_id=?`, compilerJSON(op), canonical, hash, op.Preview.EffectSHA256, id); err != nil {
				t.Fatal(err)
			}
			_, err = f.store.VerifySemanticProjection(ctx)
			var replay *SemanticReplayError
			if !errors.As(err, &replay) || replay.OperationID != id {
				t.Fatalf("malformed replay failure %#v", err)
			}
			if _, err = f.store.InspectLiteralClaims(ctx, f.owner); !errors.Is(err, ErrSemanticScopeQuarantined) {
				t.Fatalf("malformed replay not quarantined: %v", err)
			}
		})
	}
}
