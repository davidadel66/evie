package eviedb_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

func identityGeneration() memory.CompilerGeneration {
	g := compilerGeneration()
	g.EntityPolicy = memory.CompilerIdentityPolicyV2
	g.PredicatePolicy = g.EntityPolicy
	g.ValidationPolicy = g.EntityPolicy
	g.EquivalencePolicy = g.EntityPolicy
	g.EffectPolicy = g.EntityPolicy
	return g
}

func compileIdentity(t *testing.T, f *compilerFixture, content string, adapt func(*memory.ExtractorCandidate, memory.CompilerRequest)) memory.Compilation {
	t.Helper()
	sel := f.selection(t, content, true)
	result, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), sel, identityGeneration(), &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		if r.IdentityPolicy != memory.CompilerIdentityPolicyV2 {
			t.Fatal("unversioned identity request")
		}
		c := f.candidate(r)
		adapt(&c, r)
		return compilerOutput(r, []memory.ExtractorCandidate{c}), nil
	}})
	if err != nil || result.State != "completed_candidates" {
		t.Fatalf("compile identities %+v: %v", result, err)
	}
	return result
}

func identityProposal(c *memory.ExtractorCandidate) {
	c.Proposition.PredicateID = ""
	c.Proposition.Object = memory.ClaimObject{}
	confidence := 0.35
	c.Identity = &memory.CandidateIdentityProposal{Object: &memory.EntityMention{Name: "Maya", EntityType: "person", Support: c.Support[0]}, Predicate: &memory.PredicateDefinition{Token: "works_with", Label: "works with", ObjectConstraint: memory.ConstraintEntity, Cardinality: memory.CardinalityMany}, Uncertainty: "Maya may refer to a different person; owner must resolve.", Confidence: &confidence}
}

func TestOwnerReviewIdentityCreatesDependentEffectsAndReplays(t *testing.T) {
	ctx := context.Background()
	f := newCompilerFixture(t)
	compiled := compileIdentity(t, f, "I work with Maya.", func(c *memory.ExtractorCandidate, _ memory.CompilerRequest) { identityProposal(c) })
	a, err := f.store.LocalOwnerReviewContext(ctx, "global")
	if err != nil {
		t.Fatal(err)
	}
	ref := candidateRef(compiled)
	if _, err = f.store.PrepareOwnerCandidateReview(ctx, a, ref, "accept"); err == nil || !strings.Contains(err.Error(), "needs_choice") {
		t.Fatalf("unresolved reference accepted: %v", err)
	}
	options, err := f.store.OwnerCandidateIdentityOptions(ctx, a, ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(options.Object) != 0 || len(options.Predicates) != 0 {
		t.Fatalf("invented known alternatives %+v", options)
	}
	choices := memory.ReviewIdentityChoices{Object: &memory.ReviewEntityChoice{Create: true}, Predicate: &memory.ReviewPredicateChoice{Create: true}}
	chosen, err := f.store.ChooseOwnerCandidateIdentity(ctx, a, memory.ReviewIdentityDecision{Candidate: ref, OptionsSHA256: options.SHA256, Choices: choices})
	if err != nil {
		t.Fatal(err)
	}
	if chosen.Ref.InterpretationRevision != 1 || chosen.Ref.ReviewRevision != 1 || chosen.Candidate.Proposal.Proposition.Object.EntityID != "" || chosen.Identity == nil {
		t.Fatalf("original meaning/revision changed %+v", chosen)
	}
	if _, err = f.store.PrepareOwnerCandidateReview(ctx, a, ref, "accept"); !errors.Is(err, eviedb.ErrReviewStale) {
		t.Fatalf("old ref: %v", err)
	}
	p, err := f.store.PrepareOwnerCandidateReview(ctx, a, chosen.Ref, "accept")
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != "owner-review-preview-v2" || p.Effect.Identity == nil || !p.Effect.Claims[0].Predicate.Create || !p.Effect.Claims[0].ObjectEntity.Create || len(p.Effect.Identity.Aliases) != 1 {
		t.Fatalf("incomplete effect %+v", p.Effect)
	}
	result, err := f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000141"))
	if err != nil {
		t.Fatal(err)
	}
	claim, err := f.store.InspectSemanticObject(ctx, f.session.ScopeContext(), memory.SemanticObjectClaim, result.Operation.ClaimIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if claim.Claim.Object.EntityID != p.Effect.Claims[0].ObjectEntity.ID {
		t.Fatalf("wrong identity %+v", claim)
	}
	replay, err := f.store.VerifySemanticProjection(ctx)
	if err != nil || !replay.Valid {
		b, _ := json.MarshalIndent(replay, "", "  ")
		t.Fatalf("identity replay: %v %s", err, b)
	}
}

func seedReviewPerson(t *testing.T, f *compilerFixture, name, alias, key string) memory.SemanticID {
	t.Helper()
	source := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: name + " works with me."})
	p, err := f.store.PrepareRememberEntity(context.Background(), f.session.ScopeContext(), memory.RememberEntityRequest{IdempotencyKey: "idem:v1:" + key, SourceEventID: source.ID, Predicate: "works_with", PredicateLabel: "works with", Subject: memory.EntitySelector{EntityID: f.subject}, Object: memory.EntitySelector{Create: true, CanonicalName: name, EntityType: "person", Alias: alias}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ApplyRememberEntity(context.Background(), f.lease, p); err != nil {
		t.Fatal(err)
	}
	return p.Claim.ObjectEntityID
}

func TestOwnerReviewIdentityLexicalAlternativesReuseAndStale(t *testing.T) {
	ctx := context.Background()
	f := newCompilerFixture(t)
	first := seedReviewPerson(t, f, "Maya North", "Maya", "90000000-0000-4000-8000-000000000142")
	second := seedReviewPerson(t, f, "Maya South", "Maya", "90000000-0000-4000-8000-000000000143")
	compiled := compileIdentity(t, f, "I work with Maya.", func(c *memory.ExtractorCandidate, r memory.CompilerRequest) {
		identityProposal(c)
		if len(r.Aliases) < 2 {
			t.Fatal("extractor omitted accepted Aliases")
		}
	})
	a, err := f.store.LocalOwnerReviewContext(ctx, "global")
	if err != nil {
		t.Fatal(err)
	}
	ref := candidateRef(compiled)
	options, err := f.store.OwnerCandidateIdentityOptions(ctx, a, ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(options.Object) != 2 || options.Object[0].Entity.ID == options.Object[1].Entity.ID || len(options.Predicates) != 1 {
		t.Fatalf("same-name alternatives were merged %+v", options)
	}
	for _, alt := range options.Object {
		if len(alt.Aliases) != 1 || alt.Aliases[0].SourceEventID == "" || len(alt.Context) == 0 {
			t.Fatalf("missing supporting identity context %+v", alt)
		}
	}
	choices := memory.ReviewIdentityChoices{Object: &memory.ReviewEntityChoice{EntityID: first}, Predicate: &memory.ReviewPredicateChoice{PredicateID: options.Predicates[0].ID}}
	chosen, err := f.store.ChooseOwnerCandidateIdentity(ctx, a, memory.ReviewIdentityDecision{Candidate: ref, OptionsSHA256: options.SHA256, Choices: choices})
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.store.PrepareOwnerCandidateReview(ctx, a, chosen.Ref, "accept")
	if err != nil {
		t.Fatal(err)
	}
	if p.Effect.Claims[0].Create || p.Effect.Claims[0].ObjectEntity.Create || len(p.Effect.Identity.Aliases) != 0 {
		t.Fatalf("equal existing proposition duplicated %+v", p.Effect)
	}
	current, err := f.store.OwnerCandidateIdentityOptions(ctx, a, chosen.Ref)
	if err != nil {
		t.Fatal(err)
	}
	choices.Object.EntityID = second
	changed, err := f.store.ChooseOwnerCandidateIdentity(ctx, a, memory.ReviewIdentityDecision{Candidate: chosen.Ref, OptionsSHA256: current.SHA256, Choices: choices})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000144")); !errors.Is(err, eviedb.ErrReviewStale) {
		t.Fatalf("superseded owner choice accepted: %v", err)
	}
	p, err = f.store.PrepareOwnerCandidateReview(ctx, a, changed.Ref, "accept")
	if err != nil {
		t.Fatal(err)
	}
	seedReviewPerson(t, f, "Maya Third", "Maya", "90000000-0000-4000-8000-000000000145")
	if _, err = f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000146")); !errors.Is(err, eviedb.ErrReviewStale) {
		t.Fatalf("changed graph accepted stale resolution: %v", err)
	}
	if _, err = f.store.PrepareOwnerCandidateReview(ctx, a, changed.Ref, "accept"); !errors.Is(err, eviedb.ErrReviewStale) {
		t.Fatalf("changed alternatives silently rebound: %v", err)
	}
	fresh, err := f.store.OwnerCandidateIdentityOptions(ctx, a, changed.Ref)
	if err != nil {
		t.Fatal(err)
	}
	changed, err = f.store.ChooseOwnerCandidateIdentity(ctx, a, memory.ReviewIdentityDecision{Candidate: changed.Ref, OptionsSHA256: fresh.SHA256, Choices: choices})
	if err != nil {
		t.Fatal(err)
	}
	p, err = f.store.PrepareOwnerCandidateReview(ctx, a, changed.Ref, "accept")
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000147"))
	if err != nil {
		t.Fatal(err)
	}
	history, err := f.store.InspectOwnerCandidateIdentityRevision(ctx, a, ref.ID, 1)
	if err != nil || history.Choices.Object.EntityID != first || history.Revision != 1 {
		t.Fatalf("abandoned interpretation missing %+v %v", history, err)
	}
	if result.Operation.ClaimIDs[0] != p.Effect.Claims[0].Claim.ID || p.Effect.Claims[0].ObjectEntity.ID != second {
		t.Fatal("review lost exact chosen identity")
	}
	replay, err := f.store.VerifySemanticProjection(ctx)
	if err != nil || !replay.Valid {
		t.Fatalf("reused identity replay %+v %v", replay, err)
	}
}

func TestOwnerReviewIdentityRejectsUnsupportedAndUnversionedProposals(t *testing.T) {
	for _, test := range []struct {
		name       string
		generation memory.CompilerGeneration
		edit       func(*memory.ExtractorCandidate)
	}{
		{"old generation", compilerGeneration(), func(c *memory.ExtractorCandidate) {}},
		{"unsupported name", identityGeneration(), func(c *memory.ExtractorCandidate) { c.Identity.Object.Name = "Invented" }},
		{"unoffered mention support", identityGeneration(), func(c *memory.ExtractorCandidate) {
			c.Identity.Object.Support.EventID = "90000000-0000-4000-8000-000000000148"
		}},
		{"unresolved with model ID", identityGeneration(), func(c *memory.ExtractorCandidate) {
			c.Proposition.Object.EntityID = "90000000-0000-4000-8000-000000000148"
		}},
		{"confidence above one", identityGeneration(), func(c *memory.ExtractorCandidate) { v := 1.1; c.Identity.Confidence = &v }},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newCompilerFixture(t)
			sel := f.selection(t, "I work with Maya.", true)
			result, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), sel, test.generation, &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
				c := f.candidate(r)
				identityProposal(&c)
				test.edit(&c)
				return compilerOutput(r, []memory.ExtractorCandidate{c}), nil
			}})
			if err == nil || len(result.Candidates) != 0 {
				t.Fatalf("unsafe proposal admitted %+v %v", result, err)
			}
		})
	}
}

func TestOwnerReviewIdentityCompoundRollbackAndNoDefinitionRedefinition(t *testing.T) {
	ctx := context.Background()
	f := newCompilerFixture(t)
	compiled := compileIdentity(t, f, "I work with Maya.", func(c *memory.ExtractorCandidate, _ memory.CompilerRequest) { identityProposal(c) })
	a, err := f.store.LocalOwnerReviewContext(ctx, "global")
	if err != nil {
		t.Fatal(err)
	}
	options, err := f.store.OwnerCandidateIdentityOptions(ctx, a, candidateRef(compiled))
	if err != nil {
		t.Fatal(err)
	}
	chosen, err := f.store.ChooseOwnerCandidateIdentity(ctx, a, memory.ReviewIdentityDecision{Candidate: candidateRef(compiled), OptionsSHA256: options.SHA256, Choices: memory.ReviewIdentityChoices{Object: &memory.ReviewEntityChoice{Create: true}, Predicate: &memory.ReviewPredicateChoice{Create: true}}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.store.PrepareOwnerCandidateReview(ctx, a, chosen.Ref, "accept")
	if err != nil {
		t.Fatal(err)
	}
	// Force failure after dependent identity writes, inside the real transaction.
	if _, err = f.db.Exec(`CREATE TRIGGER identity_test_abort BEFORE INSERT ON semantic_source_links BEGIN SELECT RAISE(ABORT,'injected source write failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000149")); err == nil {
		t.Fatal("injected transaction failure succeeded")
	}
	for _, check := range []struct {
		query string
		id    any
	}{{`SELECT count(*) FROM semantic_entities WHERE entity_id=?`, p.Effect.Claims[0].ObjectEntity.ID}, {`SELECT count(*) FROM semantic_predicates WHERE predicate_id=?`, p.Effect.Claims[0].Predicate.ID}, {`SELECT count(*) FROM semantic_operations WHERE operation_id=?`, p.Effect.OperationID}, {`SELECT count(*) FROM memory_review_resolutions WHERE candidate_id=?`, chosen.Ref.ID}} {
		var n int
		if err = f.db.QueryRow(check.query, check.id).Scan(&n); err != nil || n != 0 {
			t.Fatalf("partial dependency writes: %d %v", n, err)
		}
	}
	if _, err = f.db.Exec(`DROP TRIGGER identity_test_abort`); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000149")); err != nil {
		t.Fatal(err)
	}
	next := compileIdentity(t, f, "I work with Maya.", func(c *memory.ExtractorCandidate, _ memory.CompilerRequest) {
		identityProposal(c)
		c.Identity.Predicate.Label = "secretly means employs"
	})
	opts, err := f.store.OwnerCandidateIdentityOptions(ctx, a, candidateRef(next))
	if err != nil {
		t.Fatal(err)
	}
	for _, choice := range []*memory.ReviewPredicateChoice{{Create: true}, {PredicateID: p.Effect.Claims[0].Predicate.ID}} {
		if _, err = f.store.ChooseOwnerCandidateIdentity(ctx, a, memory.ReviewIdentityDecision{Candidate: candidateRef(next), OptionsSHA256: opts.SHA256, Choices: memory.ReviewIdentityChoices{Object: &memory.ReviewEntityChoice{EntityID: p.Effect.Claims[0].ObjectEntity.ID}, Predicate: choice}}); err == nil {
			t.Fatal("existing Predicate meaning was redefined")
		}
	}
}

func TestOwnerReviewIdentityScopeAndPromotionPreserveSourceBoundary(t *testing.T) {
	ctx := context.Background()
	f := newCompilerFixture(t)
	selection := f.selection(t, "I work with Maya.", true)
	selection.Destination = "session:" + string(f.session.ID)
	compiled, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), selection, identityGeneration(), &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		c := f.candidate(r)
		identityProposal(&c)
		return compilerOutput(r, []memory.ExtractorCandidate{c}), nil
	}})
	if err != nil || compiled.State != "completed_candidates" {
		t.Fatalf("compile %+v %v", compiled, err)
	}
	a, err := f.store.LocalOwnerReviewContext(ctx, selection.Destination)
	if err != nil {
		t.Fatal(err)
	}
	global, err := f.store.LocalOwnerReviewContext(ctx, "global")
	if err != nil {
		t.Fatal(err)
	}
	ref := candidateRef(compiled)
	if options, err := f.store.OwnerCandidateIdentityOptions(ctx, global, ref); !errors.Is(err, eviedb.ErrOwnerReviewUnauthorized) || len(options.Object) > 0 {
		t.Fatalf("cross-scope alternatives leaked %+v %v", options, err)
	}
	options, err := f.store.OwnerCandidateIdentityOptions(ctx, a, ref)
	if err != nil {
		t.Fatal(err)
	}
	chosen, err := f.store.ChooseOwnerCandidateIdentity(ctx, a, memory.ReviewIdentityDecision{Candidate: ref, OptionsSHA256: options.SHA256, Choices: memory.ReviewIdentityChoices{Object: &memory.ReviewEntityChoice{Create: true}, Predicate: &memory.ReviewPredicateChoice{Create: true}}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.store.PrepareOwnerCandidateReview(ctx, a, chosen.Ref, "accept")
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000150"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Effect.Claims[0].Claim.ScopeKey != selection.Destination || p.Effect.Claims[0].ObjectEntity.ScopeKey != selection.Destination {
		t.Fatal("identity creation widened destination")
	}
	var scope string
	if err = f.db.QueryRow(`SELECT s.scope_key FROM semantic_predicates p JOIN semantic_scopes s ON s.scope_id=p.scope_id WHERE p.predicate_id=?`, p.Effect.Claims[0].Predicate.ID).Scan(&scope); err != nil || scope != "global" {
		t.Fatalf("definition not global %s %v", scope, err)
	}
	outsider, err := f.store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.InspectSemanticEntity(ctx, outsider.ScopeContext(), p.Effect.Claims[0].ObjectEntity.ID); err == nil {
		t.Fatal("global context discovered session identity")
	}
	// A global name match cannot pull the private session Entity into alternatives.
	globalCandidate := compileIdentity(t, f, "I work with Maya.", func(c *memory.ExtractorCandidate, _ memory.CompilerRequest) { identityProposal(c) })
	globalOptions, err := f.store.OwnerCandidateIdentityOptions(ctx, global, candidateRef(globalCandidate))
	if err != nil {
		t.Fatal(err)
	}
	if len(globalOptions.Object) != 0 {
		t.Fatalf("session identity leaked through shared global Predicate: %+v", globalOptions)
	}
	illegal := memory.ReviewIdentityChoices{Object: &memory.ReviewEntityChoice{EntityID: p.Effect.Claims[0].ObjectEntity.ID}, Predicate: &memory.ReviewPredicateChoice{PredicateID: p.Effect.Claims[0].Predicate.ID}}
	if _, err = f.store.ChooseOwnerCandidateIdentity(ctx, global, memory.ReviewIdentityDecision{Candidate: candidateRef(globalCandidate), OptionsSHA256: globalOptions.SHA256, Choices: illegal}); err == nil {
		t.Fatal("model/owner field widened identity scope")
	}
	event := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Promote this reviewed relationship to global memory."})
	promotion, err := f.store.PreparePromotion(ctx, f.session.ScopeContext(), memory.PromotionRequest{IdempotencyKey: "idem:v1:90000000-0000-4000-8000-000000000151", SourceEventID: event.ID, SourceClaimID: result.Operation.ClaimIDs[0], DestinationScopeKey: "global"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(memory.ApprovalPayload{Decision: memory.ApprovalApproved, ProposalSHA256: promotion.ProposalSHA256, PreparedSHA256: promotion.PreparedSHA256})
	if err != nil {
		t.Fatal(err)
	}
	f.append(t, memory.EventInput{ParentID: promotion.Evidence.EventID, Type: memory.EventApproval, ExecutionID: memory.ExecutionID(promotion.OperationID), Payload: payload})
	promoted, err := f.store.ApplyPromotion(ctx, f.lease, promotion)
	if err != nil {
		t.Fatal(err)
	}
	source, err := f.store.InspectSemanticObject(ctx, outsider.ScopeContext(), memory.SemanticObjectSourceLink, promotion.Sources[0].ID)
	if err != nil || source.Source.Evidence == "" {
		t.Fatalf("explicit Promotion lost disclosure %+v %v", source, err)
	}
	if _, err = f.db.Exec(`UPDATE memory_review_authorization SET source_policy='changed-detector-v2'`); err != nil {
		t.Fatal(err)
	}
	source, err = f.store.InspectSemanticObject(ctx, outsider.ScopeContext(), memory.SemanticObjectSourceLink, promotion.Sources[0].ID)
	if err != nil || source.Source.Evidence != "" {
		t.Fatalf("new identity Promotion bypassed origin policy %+v %v", source, err)
	}
	if promoted.DestinationClaimID == result.Operation.ClaimIDs[0] {
		t.Fatal("Promotion reused narrow Claim identity")
	}
	verified, err := f.store.VerifySemanticProjection(ctx)
	if err != nil || !verified.Valid {
		t.Fatalf("identity scope/promotion replay %+v %v", verified, err)
	}
}

func TestOwnerReviewIdentityProjectDecisionUsesContextAnchor(t *testing.T) {
	ctx := context.Background()
	f := newCompilerFixture(t)
	project, err := f.store.RegisterProject(ctx, "Memory Project", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := f.store.CreateProjectSession(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.session = session
	f.lease, err = f.store.AcquireTurnLease(ctx, session.ID, "identity-project", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	event := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Use a durable relational database."})
	seed, err := f.store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{IdempotencyKey: "idem:v1:90000000-0000-4000-8000-000000000152", SourceEventID: event.ID, Predicate: "constraint", PredicateLabel: "constraint", Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "durable relational database"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ApplyRememberLiteral(ctx, f.lease, seed); err != nil {
		t.Fatal(err)
	}
	seedReviewPerson(t, f, "Project Collaborator", "Collaborator", "90000000-0000-4000-8000-000000000154")
	selection := f.selection(t, "For Memory Project, we decided to use Postgres.", true)
	selection.Destination = "project:" + string(project.ID)
	compiled, err := f.store.CompileCandidateUnit(ctx, session.ScopeContext(), selection, identityGeneration(), &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		c := f.candidate(r)
		c.Proposition.PredicateID = ""
		c.Proposition.Object.Literal.Value = "Postgres"
		c.Proposition.SubjectEntityID = ""
		for _, e := range r.Entities {
			if e.AnchorKind == "context" && e.ScopeKey == selection.Destination {
				c.Proposition.SubjectEntityID = e.ID
			}
		}
		c.Identity = &memory.CandidateIdentityProposal{Predicate: &memory.PredicateDefinition{Token: "database_choice", Label: "chosen database", ObjectConstraint: "text", Cardinality: memory.CardinalityOne}, Uncertainty: "Explicit project decision; implementation status remains unknown."}
		return compilerOutput(r, []memory.ExtractorCandidate{c}), nil
	}})
	if err != nil || compiled.State != "completed_candidates" {
		t.Fatalf("project compilation %+v %v", compiled, err)
	}
	a, err := f.store.LocalOwnerReviewContext(ctx, selection.Destination)
	if err != nil {
		t.Fatal(err)
	}
	options, err := f.store.OwnerCandidateIdentityOptions(ctx, a, candidateRef(compiled))
	if err != nil {
		t.Fatal(err)
	}
	chosen, err := f.store.ChooseOwnerCandidateIdentity(ctx, a, memory.ReviewIdentityDecision{Candidate: candidateRef(compiled), OptionsSHA256: options.SHA256, Choices: memory.ReviewIdentityChoices{Predicate: &memory.ReviewPredicateChoice{Create: true}}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.store.PrepareOwnerCandidateReview(ctx, a, chosen.Ref, "accept")
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000153"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Effect.Claims[0].Subject.AnchorKind != "context" || p.Effect.Claims[0].Claim.ScopeKey != selection.Destination || len(result.Operation.ResultingRevisions) != 2 {
		t.Fatalf("project/global structural effects %+v", p.Effect)
	}
	verified, err := f.store.VerifySemanticProjection(ctx)
	if err != nil || !verified.Valid {
		t.Fatalf("project decision replay %+v %v", verified, err)
	}
}

func TestOwnerReviewIdentityGenerationAndShapeCompatibility(t *testing.T) {
	old := compilerGeneration()
	oldID, oldBytes, err := memory.CompilerGenerationIdentity(old)
	if err != nil {
		t.Fatal(err)
	}
	oldEncoded, err := json.Marshal(old)
	if err != nil || string(oldBytes) != string(oldEncoded) {
		t.Fatal("old generation canonical bytes changed")
	}
	next := identityGeneration()
	nextID, _, err := memory.CompilerGenerationIdentity(next)
	if err != nil || nextID == oldID {
		t.Fatal("identity policy did not pin a new generation")
	}
	mixed := next
	mixed.ValidationPolicy = memory.CompilerPolicyVersion
	if _, _, err = memory.CompilerGenerationIdentity(mixed); err == nil {
		t.Fatal("mixed identity contracts admitted")
	}
	oldCandidate := memory.ExtractorCandidate{Proposition: memory.ClaimProposition{Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "tea"}}}, Support: []memory.EvidenceLocator{}, Context: []memory.EvidenceLocator{}}
	raw, err := json.Marshal(oldCandidate)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"identity"`) {
		t.Fatalf("v1 byte shape widened %s", raw)
	}
	var decoded memory.ExtractorCandidate
	if err = memory.DecodeCompilerJSON(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	fields["identity"] = json.RawMessage(`null`)
	noncanonical, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err = memory.DecodeCompilerJSON(noncanonical, &decoded); err == nil {
		t.Fatal("explicit extension null silently changed v1 shape")
	}
}

func TestOwnerReviewIdentityChoiceCASAcrossStores(t *testing.T) {
	ctx := context.Background()
	f := newCompilerFixture(t)
	compiled := compileIdentity(t, f, "I work with Maya.", func(c *memory.ExtractorCandidate, _ memory.CompilerRequest) { identityProposal(c) })
	db2, err := eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	store2 := eviedb.NewStore(db2)
	stores := []*eviedb.Store{f.store, store2}
	a, err := f.store.LocalOwnerReviewContext(ctx, "global")
	if err != nil {
		t.Fatal(err)
	}
	options, err := f.store.OwnerCandidateIdentityOptions(ctx, a, candidateRef(compiled))
	if err != nil {
		t.Fatal(err)
	}
	decision := memory.ReviewIdentityDecision{Candidate: candidateRef(compiled), OptionsSHA256: options.SHA256, Choices: memory.ReviewIdentityChoices{Object: &memory.ReviewEntityChoice{Create: true}, Predicate: &memory.ReviewPredicateChoice{Create: true}}}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, store := range stores {
		go func(store *eviedb.Store) {
			authority, err := store.LocalOwnerReviewContext(ctx, "global")
			if err != nil {
				results <- err
				return
			}
			<-start
			_, err = store.ChooseOwnerCandidateIdentity(ctx, authority, decision)
			results <- err
		}(store)
	}
	close(start)
	success, stale := 0, 0
	for range stores {
		err := <-results
		if err == nil {
			success++
		} else if errors.Is(err, eviedb.ErrReviewStale) {
			stale++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || stale != 1 {
		t.Fatalf("choice race success=%d stale=%d", success, stale)
	}
	var count int
	if err = f.db.QueryRow(`SELECT count(*) FROM memory_review_identity_revisions WHERE candidate_id=?`, decision.Candidate.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate choice revisions=%d %v", count, err)
	}
}
