package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/web"
)

func TestStage4AtomicBatchPolicyConformance(t *testing.T) {
	f := newStage4ConformanceFixture(t)
	// Two claims explicitly share newly chosen definitions, forming one real
	// dependency component. A third candidate remains an independent group.
	generation := f.generation
	generation.EntityPolicy = memory.CompilerIdentityPolicyV2
	generation.PredicatePolicy = generation.EntityPolicy
	generation.ValidationPolicy = generation.EntityPolicy
	generation.EquivalencePolicy = generation.EntityPolicy
	generation.EffectPolicy = generation.EntityPolicy
	extractor := f.extractor("")
	extractor.run = func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		candidates := []memory.ExtractorCandidate{}
		for _, value := range []string{"coffee", "espresso"} {
			output := stage4CandidateOutput(r, f.seed.Subject.ID, f.seed.Predicate.ID, value)
			var parsed memory.CompilerResponse
			if err := json.Unmarshal(output.Raw, &parsed); err != nil {
				return eviedb.CompilerExtraction{}, err
			}
			c := parsed.Candidates[0]
			c.Proposition.SubjectEntityID = ""
			c.Proposition.PredicateID = ""
			c.Identity = &memory.CandidateIdentityProposal{Subject: &memory.EntityMention{Name: "Maya", EntityType: "person", Support: c.Support[0]}, Predicate: &memory.PredicateDefinition{Token: "likes", Label: "likes", ObjectConstraint: memory.PredicateObjectConstraint(memory.LiteralText), Cardinality: memory.CardinalityMany}}
			candidates = append(candidates, c)
		}
		raw, err := json.Marshal(memory.CompilerResponse{RequestID: r.ID, Candidates: candidates})
		return eviedb.CompilerExtraction{Raw: raw, ReleaseEvidence: "completed"}, err
	}
	selected := f.foreground("Maya likes coffee and espresso.")
	compiled := f.compile(selected, generation, extractor)
	if len(compiled.Candidates) != 2 {
		t.Fatal("missing dependent candidates")
	}
	refs := []memory.CandidateRef{}
	for _, candidate := range compiled.Candidates {
		ref := memory.CandidateRef{ID: candidate.ID, ReviewRevision: candidate.ReviewRevision}
		a := f.authority("global")
		options, err := f.store.OwnerCandidateIdentityOptions(f.ctx, a, ref)
		if err != nil {
			t.Fatal(err)
		}
		chosen, err := f.store.ChooseOwnerCandidateIdentity(f.ctx, a, memory.ReviewIdentityDecision{Candidate: ref, OptionsSHA256: options.SHA256, Choices: memory.ReviewIdentityChoices{Subject: &memory.ReviewEntityChoice{Create: true}, Predicate: &memory.ReviewPredicateChoice{Create: true}}})
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, chosen.Ref)
	}
	independent := f.foreground("I drink cocoa.")
	item := f.inspectCandidate(f.compile(independent, f.generation, f.extractor("cocoa")), "global")
	refs = append(refs, item.Ref)
	f.closeSource()
	sourceSession := f.session.ID
	a := f.authority("global")
	server := httptest.NewServer(web.WithCandidateReview(web.NewServer(nil), f.store).Handler())
	defer server.Close()
	request := memory.ReviewBatchRequest{Groups: []memory.ReviewBatchGroupRequest{
		{ID: "atomic", Action: "accept", Candidates: refs[:2], Dependencies: []memory.ReviewDependency{{CandidateID: refs[1].ID, Field: "subject", FromCandidateID: refs[0].ID, FromField: "subject"}, {CandidateID: refs[1].ID, Field: "predicate", FromCandidateID: refs[0].ID, FromField: "predicate"}}},
		{ID: "independent", Action: "accept", Candidates: refs[2:], Dependencies: []memory.ReviewDependency{}},
	}}
	var preview memory.ReviewBatchPreview
	stage4HTTP(t, server, "batch/prepare", map[string]any{"scope_key": "global", "input": request}, 200, &preview)
	if len(preview.Groups) != 2 || len(preview.Groups[0].Preview.Effect.Claims) != 2 {
		t.Fatalf("incomplete atomic preview %+v", preview)
	}
	var cliPreview memory.ReviewBatchPreview
	stage4CLI(t, f.store, &cliPreview, "batch-inspect", "--scope", "global", "--id", preview.ID)
	if !reflect.DeepEqual(preview, cliPreview) {
		t.Fatal("batch adapter preview differs")
	}
	// A competing explicit rejection is group-local drift, not semantic-vector
	// drift. It must survive a later unrelated outer transaction rollback.
	reject, err := f.store.PrepareOwnerCandidateReview(f.ctx, a, refs[2], "reject")
	if err != nil {
		t.Fatal(err)
	}
	rejected, err := f.store.ResolveOwnerCandidateReview(f.ctx, a, memory.ReviewDecision{DeliveryKey: stage4Key(300), PreviewID: reject.ID, PreviewSHA256: reject.SHA256, Action: "reject"})
	if err != nil {
		t.Fatal(err)
	}
	decision := memory.ReviewBatchDecision{DeliveryKey: stage4Key(301), PreviewID: preview.ID, PreviewSHA256: preview.SHA256, Actions: []memory.ReviewBatchAction{{GroupID: "atomic", Action: "accept"}, {GroupID: "independent", Action: "accept"}}}
	// Fail the outer durable-result write after successful groups. This is a
	// controlled SQLite fault, exercising the actual HTTP -> Kernel transaction.
	if _, err = f.db.Exec(`CREATE TRIGGER stage4_fail_batch_delivery BEFORE INSERT ON memory_review_batch_deliveries BEGIN SELECT RAISE(ABORT,'scripted outer persistence failure'); END`); err != nil {
		t.Fatal(err)
	}
	var failure map[string]any
	stage4HTTP(t, server, "batch/resolve", map[string]any{"scope_key": "global", "input": decision}, 503, &failure)
	if failure["code"] != "review_retryable" {
		t.Fatalf("incorrect retry boundary %+v", failure)
	}
	var operations, deliveries, resolutions int
	if err = f.db.QueryRow(`SELECT (SELECT count(*) FROM semantic_operations),(SELECT count(*) FROM memory_review_batch_deliveries),(SELECT count(*) FROM memory_review_resolutions)`).Scan(&operations, &deliveries, &resolutions); err != nil {
		t.Fatal(err)
	}
	if operations != 1 || deliveries != 0 || resolutions != 1 {
		t.Fatalf("outer rollback leaked %d/%d/%d", operations, deliveries, resolutions)
	}
	for _, ref := range refs[:2] {
		item, err := f.store.InspectOwnerCandidate(f.ctx, a, ref.ID)
		if err != nil || item.Ref != ref || item.Candidate.ReviewState != "unresolved" {
			t.Fatalf("rolled-back candidate changed %+v %v", item, err)
		}
	}
	if _, err = f.db.Exec(`DROP TRIGGER stage4_fail_batch_delivery`); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	var result memory.ReviewBatchResult
	stage4CLI(t, f.store, &result, "batch-resolve", "--scope", "global", "--decision", string(raw))
	if len(result.Groups) != 2 || result.Groups[0].Outcome != "accepted" || result.Groups[0].Result == nil || result.Groups[0].Result.Operation == nil || len(result.Groups[0].Result.Operation.ClaimIDs) != 2 || result.Groups[1].Outcome != "failed" || result.Groups[1].FailureCode != "already_resolved" || len(result.Groups[1].PriorResolutions) != 1 || result.Groups[1].PriorResolutions[0].AuditID != rejected.AuditID {
		t.Fatalf("atomic partial result %+v", result)
	}
	var duplicate memory.ReviewBatchResult
	stage4HTTP(t, server, "batch/resolve", map[string]any{"scope_key": "global", "input": decision}, 200, &duplicate)
	if !reflect.DeepEqual(result, duplicate) {
		t.Fatal("HTTP retried a permanent group failure")
	}
	f.reopen()
	a = f.authority("global")
	repeated, err := f.store.ResolveOwnerCandidateBatch(f.ctx, a, decision)
	if err != nil || !reflect.DeepEqual(result, repeated) {
		t.Fatalf("reopened batch changed %+v %v", repeated, err)
	}
	observer, err := f.store.CreateGlobalSession(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := f.store.InspectLiteralClaims(f.ctx, observer.ScopeContext())
	if err != nil || len(claims.Claims) != 3 {
		t.Fatalf("batch accepted read %+v %v", claims, err)
	}
	operation := result.Groups[0].Result.Operation
	var stored string
	if err = f.db.QueryRow(`SELECT prepared_proposal_json FROM semantic_operations WHERE operation_id=?`, operation.OperationID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	// A current source-policy revision hides disclosure, without rewriting the
	// historical acceptance authority or erasing already accepted truth.
	if _, err = f.db.Exec(`UPDATE memory_review_authorization SET source_policy='stage4-changed-detector-v2'`); err != nil {
		t.Fatal(err)
	}
	hidden, err := f.store.InspectOwnerReviewOperation(f.ctx, a, operation.OperationID)
	if !errors.Is(err, eviedb.ErrReviewInvalidSource) || hidden.OperationID != "" {
		t.Fatalf("historical source disclosure bypass %+v %v", hidden, err)
	}
	source, err := f.store.InspectSemanticObject(f.ctx, observer.ScopeContext(), memory.SemanticObjectSourceLink, operation.SourceLinkIDs[0])
	if err != nil || source.Source == nil || source.Source.Evidence != "" || len(source.Operations) != 1 || source.Operations[0].PreparedJSON != "" || source.Operations[0].ProposalJSON != "" {
		t.Fatalf("redaction bypass %+v %v", source, err)
	}
	claims, err = f.store.InspectLiteralClaims(f.ctx, observer.ScopeContext())
	if err != nil || len(claims.Claims) != 3 {
		t.Fatalf("source policy erased truth %+v %v", claims, err)
	}
	replay, err := f.store.VerifySemanticProjection(f.ctx)
	if err != nil || !replay.Valid {
		t.Fatalf("policy-dependent replay %+v %v", replay, err)
	}
	var after, status string
	if err = f.db.QueryRow(`SELECT prepared_proposal_json FROM semantic_operations WHERE operation_id=?`, operation.OperationID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if err = f.db.QueryRow(`SELECT status FROM sessions WHERE id=?`, sourceSession).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if after != stored || status != "closed" {
		t.Fatal("accepted history or original source session changed")
	}
	repeated, err = f.store.ResolveOwnerCandidateBatch(f.ctx, a, decision)
	if err != nil || !reflect.DeepEqual(result, repeated) {
		t.Fatalf("source policy changed duplicate receipt %+v %v", repeated, err)
	}
	stage4Evidence(t, "atomic_batch_source_policy", map[string]any{"preview_sha256": preview.SHA256, "atomic_effect_sha256": preview.Groups[0].Preview.EffectSHA256, "outer_rollback": true, "atomic_claims": 2, "permanent_partial_result": true, "duplicate_adapter_results_equal": true, "original_source_closed": true, "source_policy_redacts": true, "accepted_truth_preserved": true, "historical_acceptance_sha256": memory.CompilerHash([]byte(stored)), "canonical_replay": true})
}
