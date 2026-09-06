package web_test

import (
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

func TestCandidateAdvancedHTTPInputRefusalIsDefinitiveWithoutPersistence(t *testing.T) {
	for _, route := range []string{"resolve", "batch/resolve", "edit"} {
		for _, reason := range []struct{ name, value string }{
			{"secret", "password=synthetic-fixture-value"},
			{"oversized", strings.Repeat("x", 4097)},
		} {
			t.Run(route+"/"+reason.name, func(t *testing.T) {
				f := newWebReviewFixture(t)
				single := webPrepare(t, f, "reject")
				batch := reviewResponse[memory.ReviewBatchPreview](t, advancedPost(t, f, "batch/prepare", memory.ReviewBatchRequest{Groups: []memory.ReviewBatchGroupRequest{{ID: "group", Action: "reject", Candidates: []memory.CandidateRef{f.candidate.Ref}}}}), 200)
				decision := webDecision(single)
				decision.Reason = reason.value
				batchDecision := memory.ReviewBatchDecision{DeliveryKey: decision.DeliveryKey, PreviewID: batch.ID, PreviewSHA256: batch.SHA256, Actions: []memory.ReviewBatchAction{{GroupID: "group", Action: "reject"}}, Reason: reason.value}
				edit := memory.ReviewEditDecision{Candidate: f.candidate.Ref, Proposal: f.candidate.Candidate.Proposal, Reason: reason.value}
				switch route {
				case "resolve":
					advancedError(t, reviewPost(t, f.handler, route, map[string]any{"scope_key": "global", "decision": decision}), 400, "invalid_review_request")
				case "batch/resolve":
					advancedError(t, advancedPost(t, f, route, batchDecision), 400, "invalid_review_request")
				case "edit":
					advancedError(t, advancedPost(t, f, route, edit), 400, "invalid_review_request")
				}
				var deliveries, edits, resolutions, operations int
				err := f.db.QueryRow(`SELECT
      (SELECT count(*) FROM memory_review_deliveries)+(SELECT count(*) FROM memory_review_batch_deliveries),
      (SELECT count(*) FROM memory_review_edit_revisions),
      (SELECT count(*) FROM memory_review_resolutions),
      (SELECT count(*) FROM semantic_operations WHERE operation_kind='owner_candidate_review')`).Scan(&deliveries, &edits, &resolutions, &operations)
				if err != nil || deliveries+edits+resolutions+operations != 0 {
					t.Fatalf("refused request changed durable state: %d/%d/%d/%d %v", deliveries, edits, resolutions, operations, err)
				}
				// The definitive refusal leaves the same preview/ref available for a new
				// explicit decision with a corrected reason; no unknown delivery is stored.
				switch route {
				case "resolve":
					decision.Reason = "Reviewed"
					reviewResponse[memory.ReviewResult](t, reviewPost(t, f.handler, route, map[string]any{"scope_key": "global", "decision": decision}), 200)
				case "batch/resolve":
					batchDecision.Reason = "Reviewed"
					reviewResponse[memory.ReviewBatchResult](t, advancedPost(t, f, route, batchDecision), 200)
				case "edit":
					edit.Reason = "Reviewed"
					reviewResponse[memory.OwnerCandidate](t, advancedPost(t, f, route, edit), 200)
				}
			})
		}
	}
}
