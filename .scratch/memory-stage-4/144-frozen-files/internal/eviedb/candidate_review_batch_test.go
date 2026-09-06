package eviedb_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"strings"
	"sync"
	"testing"
)

func batchDecision(p memory.ReviewBatchPreview, key string) memory.ReviewBatchDecision {
	d := memory.ReviewBatchDecision{DeliveryKey: "idem:v1:" + key, PreviewID: p.ID, PreviewSHA256: p.SHA256, Actions: []memory.ReviewBatchAction{}}
	for _, g := range p.Groups {
		d.Actions = append(d.Actions, memory.ReviewBatchAction{GroupID: g.ID, Action: g.Preview.Action})
	}
	return d
}
func compileBatchCandidates(t *testing.T, f *compilerFixture, values ...string) []memory.CandidateRef {
	t.Helper()
	source := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I drink water."})
	seed, err := f.store.PrepareRememberLiteral(context.Background(), f.session.ScopeContext(), memory.RememberLiteralRequest{IdempotencyKey: "idem:v1:90000000-0000-4000-8000-000000009144", SourceEventID: source.ID, Predicate: "batch_drink", PredicateLabel: "batch drink", PredicateCardinality: memory.CardinalityMany, Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "water"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ApplyRememberLiteral(context.Background(), f.lease, seed); err != nil {
		t.Fatal(err)
	}
	f.predicate = seed.Predicate.ID
	sel := f.selection(t, "I drink "+strings.Join(values, " and ")+".", true)
	result, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), sel, compilerGeneration(), &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		cs := []memory.ExtractorCandidate{}
		for _, value := range values {
			c := f.candidate(r)
			c.Proposition.Object.Literal.Value = value
			cs = append(cs, c)
		}
		return compilerOutput(r, cs), nil
	}})
	if err != nil || len(result.Candidates) != len(values) {
		t.Fatalf("compile batch %+v %v", result, err)
	}
	refs := []memory.CandidateRef{}
	for _, c := range result.Candidates {
		refs = append(refs, memory.CandidateRef{ID: c.ID, ReviewRevision: c.ReviewRevision})
	}
	return refs
}
func independentBatch(refs []memory.CandidateRef) memory.ReviewBatchRequest {
	r := memory.ReviewBatchRequest{Groups: []memory.ReviewBatchGroupRequest{}}
	for i, ref := range refs {
		r.Groups = append(r.Groups, memory.ReviewBatchGroupRequest{ID: fmt.Sprintf("group%d", i), Action: "accept", Candidates: []memory.CandidateRef{ref}, Dependencies: []memory.ReviewDependency{}})
	}
	return r
}
func TestOwnerReviewEditFrozenWindowLineageAndReplay(t *testing.T) {
	ctx := context.Background()
	f, compiled, a := reviewCandidateFixture(t)
	original := compiled.Candidates[0]
	ref := candidateRef(compiled)
	old, err := f.store.PrepareOwnerCandidateReview(ctx, a, ref, "accept")
	if err != nil {
		t.Fatal(err)
	}
	proposal := original.Proposal
	proposal.Proposition.Object.Literal = &memory.TypedLiteral{Kind: memory.LiteralText, Value: "tea"}
	// Widen only inside the originally offered field, retaining its authority.
	for _, source := range compiled.Window.Sources {
		if source.Locator.EventID == proposal.Support[0].EventID {
			proposal.Support = []memory.EvidenceLocator{source.Locator}
		}
	}
	edited, err := f.store.EditOwnerCandidate(ctx, a, memory.ReviewEditDecision{Candidate: ref, Proposal: proposal, Reason: "Correct the extracted drink."})
	if err != nil {
		t.Fatal(err)
	}
	if edited.Ref.InterpretationRevision != 1 || edited.Ref.ReviewRevision != 1 || edited.Edit == nil || edited.Original == nil || edited.Edit.Before.Proposal.Proposition.Object.Literal.Value != "café" || edited.Candidate.Proposal.Proposition.Object.Literal.Value != "tea" || edited.Original.Proposal.Proposition.Object.Literal.Value != "café" {
		t.Fatalf("lineage %+v", edited)
	}
	if _, err = f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(old, "90000000-0000-4000-8000-000000000344")); !errors.Is(err, eviedb.ErrReviewStale) {
		t.Fatalf("old preview %v", err)
	}
	var stored []byte
	if err = f.db.QueryRow(`SELECT envelope FROM memory_compiler_candidates WHERE candidate_id=?`, ref.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stored), "café") {
		t.Fatal("extraction overwritten")
	}
	history, err := f.store.InspectOwnerCandidateEditRevision(ctx, a, ref.ID, 1)
	if err != nil || history.After.Support[0].Authority != memory.AuthorityOwnerStatement || history.Before.Support[0].Evidence != "café" {
		t.Fatalf("edit history %+v %v", history, err)
	}
	p, err := f.store.PrepareOwnerCandidateReview(ctx, a, edited.Ref, "accept")
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000345"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Operation == nil {
		t.Fatal("missing accepted operation")
	}
	assertTemporalReplay(t, f)
	if _, err = f.store.EditOwnerCandidate(ctx, a, memory.ReviewEditDecision{Candidate: edited.Ref, Proposal: proposal}); !errors.Is(err, eviedb.ErrReviewResolved) {
		t.Fatalf("terminal edit %v", err)
	}
	if _, err = f.db.Exec(`UPDATE memory_review_edit_revisions SET envelope='{}'`); err == nil {
		t.Fatal("mutable edit")
	}
}
func TestOwnerReviewEditRejectsUnofferedSourceAndConcurrentCAS(t *testing.T) {
	ctx := context.Background()
	f, compiled, a := reviewCandidateFixture(t)
	ref := candidateRef(compiled)
	foreign := f.selection(t, "I drink juice.", true)
	proposal := compiled.Candidates[0].Proposal
	proposal.Support = []memory.EvidenceLocator{{EventID: foreign.RootID, EventPart: memory.EvidenceContent, LocatorKind: memory.LocatorWhole, EvidenceSHA256: memory.CompilerHash([]byte("I drink juice."))}}
	if _, err := f.store.EditOwnerCandidate(ctx, a, memory.ReviewEditDecision{Candidate: ref, Proposal: proposal}); err == nil {
		t.Fatal("unoffered source accepted")
	}
	proposal = compiled.Candidates[0].Proposal
	db, err := eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	other := eviedb.NewStore(db)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, store := range []*eviedb.Store{f.store, other} {
		wg.Add(1)
		go func(s *eviedb.Store) {
			defer wg.Done()
			_, err := s.EditOwnerCandidate(ctx, a, memory.ReviewEditDecision{Candidate: ref, Proposal: proposal})
			errs <- err
		}(store)
	}
	wg.Wait()
	close(errs)
	success, stale := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, eviedb.ErrReviewStale) {
			stale++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || stale != 1 {
		t.Fatalf("competing edit %d/%d", success, stale)
	}
}
func TestOwnerReviewBatchTwoSuccessfulGroupsSameVectorAndReplay(t *testing.T) {
	ctx := context.Background()
	f := newCompilerFixture(t)
	refs := compileBatchCandidates(t, f, "coffee", "juice")
	a := temporalAuthority(t, f)
	p, err := f.store.PrepareOwnerCandidateBatch(ctx, a, independentBatch(refs))
	if err != nil {
		t.Fatal(err)
	}
	if string(mustJSON(t, p.Groups[0].Preview.Effect.PriorRevisions)) != string(mustJSON(t, p.Groups[1].Preview.Effect.PriorRevisions)) {
		t.Fatal("starting vectors differ")
	}
	d := batchDecision(p, "90000000-0000-4000-8000-000000000346")
	result, err := f.store.ResolveOwnerCandidateBatch(ctx, a, d)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range result.Groups {
		if g.Outcome != "accepted" || g.Result == nil || g.Result.Operation == nil {
			t.Fatalf("group %+v", g)
		}
	}
	if result.Groups[1].Result.Operation.ResultingRevisions[0].Revision != result.Groups[0].Result.Operation.ResultingRevisions[0].Revision+1 {
		t.Fatal("own revision advance missing")
	}
	assertTemporalReplay(t, f)
	if err = f.db.Close(); err != nil {
		t.Fatal(err)
	}
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.store = eviedb.NewStore(f.db)
	again, err := f.store.ResolveOwnerCandidateBatch(ctx, a, d)
	if err != nil || string(mustJSON(t, again)) != string(mustJSON(t, result)) {
		t.Fatalf("lost response retry %+v %v", again, err)
	}
	d.Reason = "changed"
	if _, err = f.store.ResolveOwnerCandidateBatch(ctx, a, d); !errors.Is(err, eviedb.ErrIdempotencyConflict) {
		t.Fatalf("changed request %v", err)
	}
}
func TestOwnerReviewBatchSavepointFailureImmutableReceipt(t *testing.T) {
	for _, table := range []string{"semantic_source_links", "semantic_claims"} {
		t.Run(table, func(t *testing.T) {
			ctx := context.Background()
			f := newCompilerFixture(t)
			refs := compileBatchCandidates(t, f, "coffee", "juice")
			a := temporalAuthority(t, f)
			p, err := f.store.PrepareOwnerCandidateBatch(ctx, a, independentBatch(refs))
			if err != nil {
				t.Fatal(err)
			}
			column, id := "claim_id", p.Groups[0].Preview.Effect.Claims[0].Claim.ID
			if table == "semantic_source_links" {
				column = "source_link_id"
				id = p.Groups[0].Preview.Effect.Claims[0].Sources[0].ID
			}
			if _, err = f.db.Exec(fmt.Sprintf(`CREATE TRIGGER batch_fail BEFORE INSERT ON %s WHEN NEW.%s='%s' BEGIN SELECT RAISE(ABORT,'injected safe group failure'); END`, table, column, id)); err != nil {
				t.Fatal(err)
			}
			d := batchDecision(p, "90000000-0000-4000-8000-000000000347")
			result, err := f.store.ResolveOwnerCandidateBatch(ctx, a, d)
			if err != nil {
				t.Fatal(err)
			}
			if result.Groups[0].Outcome != "failed" || result.Groups[1].Outcome != "accepted" {
				t.Fatalf("partial result %+v", result)
			}
			item, err := f.store.InspectOwnerCandidate(ctx, a, refs[0].ID)
			if err != nil || item.Ref != refs[0] || item.Candidate.ReviewState != "unresolved" {
				t.Fatalf("failed member mutated %+v %v", item, err)
			}
			var count int
			if err = f.db.QueryRow(`SELECT count(*) FROM semantic_operations WHERE operation_id=?`, p.Groups[0].Preview.Effect.OperationID).Scan(&count); err != nil || count != 0 {
				t.Fatalf("partial operation %d %v", count, err)
			}
			if _, err = f.db.Exec(`DROP TRIGGER batch_fail`); err != nil {
				t.Fatal(err)
			}
			again, err := f.store.ResolveOwnerCandidateBatch(ctx, a, d)
			if err != nil || string(mustJSON(t, again)) != string(mustJSON(t, result)) {
				t.Fatalf("failed group retried %+v %v", again, err)
			}
			assertTemporalReplay(t, f)
		})
	}
}
func TestOwnerReviewBatchExternalDriftIsWholePreviewStale(t *testing.T) {
	for _, change := range []string{"policy", "global", "source"} {
		t.Run(change, func(t *testing.T) {
			ctx := context.Background()
			f := newCompilerFixture(t)
			refs := compileBatchCandidates(t, f, "coffee", "juice")
			a := temporalAuthority(t, f)
			p, err := f.store.PrepareOwnerCandidateBatch(ctx, a, independentBatch(refs))
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "policy":
				_, err = f.db.Exec(`UPDATE memory_review_authorization SET source_policy='detector-next'`)
			case "global":
				_, err = f.db.Exec(`UPDATE semantic_scopes SET revision=revision+1 WHERE scope_key='global'`)
			case "source":
				if _, err = f.db.Exec(`DROP TRIGGER events_append_only_update`); err == nil {
					_, err = f.db.Exec(`UPDATE events SET content='different' WHERE id=?`, p.Groups[0].Preview.Candidates[0].Candidate.Support[0].Locator.EventID)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.store.ResolveOwnerCandidateBatch(ctx, a, batchDecision(p, "90000000-0000-4000-8000-000000000348")); !errors.Is(err, eviedb.ErrReviewStale) {
				t.Fatalf("external drift %v", err)
			}
			var count int
			if err = f.db.QueryRow(`SELECT count(*) FROM memory_review_batch_deliveries`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("stale receipt %d %v", count, err)
			}
		})
	}
}
func sharedBatchFixture(t *testing.T) (*compilerFixture, eviedb.OwnerReviewContext, memory.ReviewBatchRequest) {
	t.Helper()
	ctx := context.Background()
	f := newCompilerFixture(t)
	sel := f.selection(t, "Maya likes tea and coffee.", true)
	compiled, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), sel, identityGeneration(), &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		cs := []memory.ExtractorCandidate{}
		for _, value := range []string{"tea", "coffee"} {
			c := f.candidate(r)
			c.Proposition.SubjectEntityID = ""
			c.Proposition.PredicateID = ""
			c.Proposition.Object.Literal.Value = value
			c.Identity = &memory.CandidateIdentityProposal{Subject: &memory.EntityMention{Name: "Maya", EntityType: "person", Support: c.Support[0]}, Predicate: &memory.PredicateDefinition{Token: "likes", Label: "likes", ObjectConstraint: memory.PredicateObjectConstraint(memory.LiteralText), Cardinality: memory.CardinalityMany}}
			cs = append(cs, c)
		}
		return compilerOutput(r, cs), nil
	}})
	if err != nil || len(compiled.Candidates) != 2 {
		t.Fatalf("compile shared %+v %v", compiled, err)
	}
	a := temporalAuthority(t, f)
	refs := []memory.CandidateRef{}
	for _, c := range compiled.Candidates {
		ref := memory.CandidateRef{ID: c.ID}
		options, err := f.store.OwnerCandidateIdentityOptions(ctx, a, ref)
		if err != nil {
			t.Fatal(err)
		}
		chosen, err := f.store.ChooseOwnerCandidateIdentity(ctx, a, memory.ReviewIdentityDecision{Candidate: ref, OptionsSHA256: options.SHA256, Choices: memory.ReviewIdentityChoices{Subject: &memory.ReviewEntityChoice{Create: true}, Predicate: &memory.ReviewPredicateChoice{Create: true}}})
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, chosen.Ref)
	}
	deps := []memory.ReviewDependency{{CandidateID: refs[1].ID, Field: "subject", FromCandidateID: refs[0].ID, FromField: "subject"}, {CandidateID: refs[1].ID, Field: "predicate", FromCandidateID: refs[0].ID, FromField: "predicate"}}
	request := memory.ReviewBatchRequest{Groups: []memory.ReviewBatchGroupRequest{{ID: "shared", Action: "accept", Candidates: refs, Dependencies: deps}}}
	return f, a, request
}
func TestOwnerReviewBatchSharedDefinitionsAreAtomic(t *testing.T) {
	ctx := context.Background()
	f, a, request := sharedBatchFixture(t)
	refs, deps := request.Groups[0].Candidates, request.Groups[0].Dependencies
	var err error
	for _, bad := range []memory.ReviewBatchRequest{independentBatch(refs), {Groups: []memory.ReviewBatchGroupRequest{{ID: "missing", Action: "accept", Candidates: refs, Dependencies: deps[:1]}}}} {
		if _, err = f.store.PrepareOwnerCandidateBatch(ctx, a, bad); !errors.Is(err, eviedb.ErrReviewDependencies) {
			t.Fatalf("incomplete closure %v", err)
		}
	}
	p, err := f.store.PrepareOwnerCandidateBatch(ctx, a, request)
	if err != nil {
		t.Fatal(err)
	}
	effect := p.Groups[0].Preview.Effect
	if effect.Claims[0].Subject.ID != effect.Claims[1].Subject.ID || effect.Claims[0].Predicate.ID != effect.Claims[1].Predicate.ID {
		t.Fatal("shared identities duplicated")
	}
	result, err := f.store.ResolveOwnerCandidateBatch(ctx, a, batchDecision(p, "90000000-0000-4000-8000-000000000349"))
	if err != nil || result.Groups[0].Outcome != "accepted" {
		t.Fatalf("shared group %+v %v", result, err)
	}
	var entities, predicates, operations int
	if err = f.db.QueryRow(`SELECT (SELECT count(*) FROM semantic_entities WHERE canonical_name='Maya'),(SELECT count(*) FROM semantic_predicates WHERE token='likes'),(SELECT count(*) FROM semantic_operations WHERE schema_version=6)`).Scan(&entities, &predicates, &operations); err != nil {
		t.Fatal(err)
	}
	if entities != 1 || predicates != 1 || operations != 1 {
		t.Fatalf("compound writes %d/%d/%d", entities, predicates, operations)
	}
	assertTemporalReplay(t, f)
}

func TestOwnerReviewBatchSharedGroupFailureKeepsIndependentProgress(t *testing.T) {
	ctx := context.Background()
	f, a, request := sharedBatchFixture(t)
	refs := compileBatchCandidates(t, f, "juice")
	request.Groups = append(request.Groups, memory.ReviewBatchGroupRequest{ID: "independent", Action: "accept", Candidates: refs, Dependencies: []memory.ReviewDependency{}})
	// The independent seed legitimately advanced the registry. Refresh explicit
	// owner identity choices against that final starting vector before preview.
	for i, ref := range request.Groups[0].Candidates {
		o, err := f.store.OwnerCandidateIdentityOptions(ctx, a, ref)
		if err != nil {
			t.Fatal(err)
		}
		chosen, err := f.store.ChooseOwnerCandidateIdentity(ctx, a, memory.ReviewIdentityDecision{Candidate: ref, OptionsSHA256: o.SHA256, Choices: memory.ReviewIdentityChoices{Subject: &memory.ReviewEntityChoice{Create: true}, Predicate: &memory.ReviewPredicateChoice{Create: true}}})
		if err != nil {
			t.Fatal(err)
		}
		request.Groups[0].Candidates[i] = chosen.Ref
	}
	p, err := f.store.PrepareOwnerCandidateBatch(ctx, a, request)
	if err != nil {
		t.Fatal(err)
	}
	shared := p.Groups[0].Preview.Effect
	// Fail after shared Entity/Predicate and the first dependent Claim were written.
	if _, err = f.db.Exec(fmt.Sprintf(`CREATE TRIGGER fail_second_shared_source BEFORE INSERT ON semantic_source_links WHEN NEW.source_link_id='%s' BEGIN SELECT RAISE(ABORT,'safe group failure');END`, shared.Claims[1].Sources[0].ID)); err != nil {
		t.Fatal(err)
	}
	d := batchDecision(p, "90000000-0000-4000-8000-000000000350")
	r, err := f.store.ResolveOwnerCandidateBatch(ctx, a, d)
	if err != nil || len(r.Groups) != 2 || r.Groups[0].Outcome != "failed" || r.Groups[1].Outcome != "accepted" {
		t.Fatalf("dependent partial %+v %v", r, err)
	}
	var entities, predicates, claims, resolutions int
	if err = f.db.QueryRow(`SELECT (SELECT count(*) FROM semantic_entities WHERE canonical_name='Maya'),(SELECT count(*) FROM semantic_predicates WHERE token='likes'),(SELECT count(*) FROM semantic_claims WHERE created_operation_id=?),(SELECT count(*) FROM memory_review_resolutions WHERE candidate_id IN (?,?))`, shared.OperationID, request.Groups[0].Candidates[0].ID, request.Groups[0].Candidates[1].ID).Scan(&entities, &predicates, &claims, &resolutions); err != nil {
		t.Fatal(err)
	}
	if entities+predicates+claims+resolutions != 0 {
		t.Fatalf("dependent partial leak %d/%d/%d/%d", entities, predicates, claims, resolutions)
	}
	if _, err = f.db.Exec(`DROP TRIGGER fail_second_shared_source`); err != nil {
		t.Fatal(err)
	}
	retry, err := f.store.ResolveOwnerCandidateBatch(ctx, a, d)
	if err != nil || string(mustJSON(t, retry)) != string(mustJSON(t, r)) {
		t.Fatalf("immutable failure %+v %v", retry, err)
	}
	for i, ref := range request.Groups[0].Candidates {
		o, err := f.store.OwnerCandidateIdentityOptions(ctx, a, ref)
		if err != nil {
			t.Fatal(err)
		}
		chosen, err := f.store.ChooseOwnerCandidateIdentity(ctx, a, memory.ReviewIdentityDecision{Candidate: ref, OptionsSHA256: o.SHA256, Choices: memory.ReviewIdentityChoices{Subject: &memory.ReviewEntityChoice{Create: true}, Predicate: &memory.ReviewPredicateChoice{Create: true}}})
		if err != nil {
			t.Fatal(err)
		}
		request.Groups[0].Candidates[i] = chosen.Ref
	}
	fresh, err := f.store.PrepareOwnerCandidateBatch(ctx, a, memory.ReviewBatchRequest{Groups: request.Groups[:1]})
	if err != nil {
		t.Fatal(err)
	}
	retry, err = f.store.ResolveOwnerCandidateBatch(ctx, a, batchDecision(fresh, "90000000-0000-4000-8000-000000000351"))
	if err != nil || retry.Groups[0].Outcome != "accepted" {
		t.Fatalf("fresh group %+v %v", retry, err)
	}
	assertTemporalReplay(t, f)
}
func TestOwnerReviewBatchTwoCorrectionsShareOneCanonicalOperation(t *testing.T) {
	ctx := context.Background()
	f := newCompilerFixture(t)
	a := temporalAuthority(t, f)
	first := compileTemporal(t, f, "I was mistaken about tea. I have always drunk coffee.", func(c *memory.ExtractorCandidate) {
		c.Proposition.Object.Literal.Value = "coffee"
		c.Temporal.Correction = &memory.CandidateCorrectionProposal{Modes: []memory.CorrectionMode{memory.CorrectionError}}
	})
	event := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I eat rice."})
	seed, err := f.store.PrepareRememberLiteral(ctx, f.session.ScopeContext(), memory.RememberLiteralRequest{IdempotencyKey: "idem:v1:90000000-0000-4000-8000-000000000352", SourceEventID: event.ID, Predicate: "eats", PredicateLabel: "eats", PredicateCardinality: memory.CardinalityOne, Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "rice"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ApplyRememberLiteral(ctx, f.lease, seed); err != nil {
		t.Fatal(err)
	}
	f.predicate = seed.Predicate.ID
	second := compileTemporal(t, f, "I was mistaken about rice. I eat pasta.", func(c *memory.ExtractorCandidate) {
		c.Proposition.Object.Literal.Value = "pasta"
		c.Temporal.Correction = &memory.CandidateCorrectionProposal{Modes: []memory.CorrectionMode{memory.CorrectionError}}
	})
	refs := []memory.CandidateRef{chooseTemporal(t, f, a, candidateRef(first), memory.CorrectionError).Ref, chooseTemporal(t, f, a, candidateRef(second), memory.CorrectionError).Ref}
	p, err := f.store.PrepareOwnerCandidateBatch(ctx, a, memory.ReviewBatchRequest{Groups: []memory.ReviewBatchGroupRequest{{ID: "two_corrections", Action: "accept", Candidates: refs, Dependencies: []memory.ReviewDependency{}}}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := f.store.ResolveOwnerCandidateBatch(ctx, a, batchDecision(p, "90000000-0000-4000-8000-000000000353"))
	if err != nil || r.Groups[0].Outcome != "accepted" {
		t.Fatalf("two corrections %+v %v", r, err)
	}
	var rows int
	if err = f.db.QueryRow(`SELECT count(*) FROM semantic_claim_corrections WHERE operation_id=?`, r.Groups[0].Result.Operation.OperationID).Scan(&rows); err != nil || rows != 2 {
		t.Fatalf("compound correction rows%d %v", rows, err)
	}
	for _, member := range p.Groups[0].Preview.Effect.Members {
		old, err := f.store.InspectSemanticObject(ctx, f.session.ScopeContext(), memory.SemanticObjectClaim, member.Correction.OldClaim.ID)
		if err != nil || len(old.Lifecycle) != 2 || old.Lifecycle[1].State != memory.SemanticStateSuperseded {
			t.Fatalf("old lineage %+v %v", old, err)
		}
	}
	assertTemporalReplay(t, f)
	if err = f.db.Close(); err != nil {
		t.Fatal(err)
	}
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.store = eviedb.NewStore(f.db)
	assertTemporalReplay(t, f)
}
