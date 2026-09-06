package eviedb_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

func temporalGeneration() memory.CompilerGeneration {
	g := compilerGeneration()
	g.EntityPolicy = memory.CompilerTemporalPolicyV3
	g.PredicatePolicy = g.EntityPolicy
	g.ValidationPolicy = g.EntityPolicy
	g.EquivalencePolicy = g.EntityPolicy
	g.EffectPolicy = g.EntityPolicy
	return g
}
func compileTemporal(t *testing.T, f *compilerFixture, content string, adapt func(*memory.ExtractorCandidate)) memory.Compilation {
	t.Helper()
	selection := f.selection(t, content, true)
	result, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), selection, temporalGeneration(), &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		if r.IdentityPolicy != memory.CompilerTemporalPolicyV3 {
			t.Fatal("unversioned temporal request")
		}
		c := f.candidate(r)
		c.Temporal = &memory.CandidateTemporalProposal{Meaning: "assertion"}
		adapt(&c)
		return compilerOutput(r, []memory.ExtractorCandidate{c}), nil
	}})
	if err != nil || result.State != "completed_candidates" {
		t.Fatalf("compile temporal state=%s reason=%s: %v", result.State, result.Reason, err)
	}
	return result
}
func temporalAuthority(t *testing.T, f *compilerFixture) eviedb.OwnerReviewContext {
	t.Helper()
	a, err := f.store.LocalOwnerReviewContext(context.Background(), "global")
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func chooseTemporal(t *testing.T, f *compilerFixture, a eviedb.OwnerReviewContext, ref memory.CandidateRef, mode memory.CorrectionMode) memory.OwnerCandidate {
	t.Helper()
	ctx := context.Background()
	options, err := f.store.OwnerCandidateTemporalOptions(ctx, a, ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(options.Alternatives) != 1 {
		t.Fatalf("expected exact prior Claim %+v", options)
	}
	item, err := f.store.ChooseOwnerCandidateTemporal(ctx, a, memory.ReviewTemporalDecision{Candidate: ref, OptionsSHA256: options.SHA256, Choice: memory.ReviewTemporalChoice{OldClaimID: options.Alternatives[0].Claim.ID, Mode: mode}})
	if err != nil {
		t.Fatal(err)
	}
	return item
}
func assertTemporalReplay(t *testing.T, f *compilerFixture) {
	t.Helper()
	result, err := f.store.VerifySemanticProjection(context.Background())
	if err != nil || !result.Valid {
		b, _ := json.MarshalIndent(result, "", "  ")
		t.Fatalf("temporal replay %v %s", err, b)
	}
}

func TestOwnerReviewTemporalCorrectionChangedVersusError(t *testing.T) {
	for _, mode := range []memory.CorrectionMode{memory.CorrectionError, memory.CorrectionChanged} {
		t.Run(string(mode), func(t *testing.T) {
			ctx := context.Background()
			f := newCompilerFixture(t)
			a := temporalAuthority(t, f)
			effective := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
			content := "I now drink coffee, beginning May 1 2025. Previously I drank tea."
			if mode == memory.CorrectionError {
				content = "I was mistaken about tea. I have always drunk coffee."
			}
			compiled := compileTemporal(t, f, content, func(c *memory.ExtractorCandidate) {
				c.Proposition.Object.Literal.Value = "coffee"
				c.Temporal.Correction = &memory.CandidateCorrectionProposal{Modes: []memory.CorrectionMode{mode}}
				if mode == memory.CorrectionChanged {
					c.Temporal.Correction.EffectiveTime = &effective
					c.ValidTime.From = &effective
				}
			})
			if _, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(compiled), "accept"); err == nil || !strings.Contains(err.Error(), "needs_choice") {
				t.Fatalf("unresolved correction accepted: %v", err)
			}
			selected := chooseTemporal(t, f, a, candidateRef(compiled), mode)
			p, err := f.store.PrepareOwnerCandidateReview(ctx, a, selected.Ref, "accept")
			if err != nil {
				t.Fatal(err)
			}
			if p.Version != "owner-review-preview-v3" || p.Effect.Correction == nil || p.Effect.Correction.Mode != mode || p.Effect.Claims[0].Claim.Object.Literal.Value != "coffee" {
				t.Fatalf("wrong preview %+v", p)
			}
			oldID := p.Effect.Correction.OldClaim.ID
			if mode == memory.CorrectionChanged && (p.Effect.Correction.ValidTimeEffect.OldAfter.To == nil || !p.Effect.Correction.ValidTimeEffect.OldAfter.To.Equal(effective)) {
				t.Fatal("changed interval not closed")
			}
			if mode == memory.CorrectionError && p.Effect.Correction.ValidTimeEffect.OldAfter.To != nil {
				t.Fatal("error invented effective date")
			}
			result, err := f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000242"))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Operation.TransactionTime.After(effective) {
				t.Fatal("transaction time used world date")
			}
			old, err := f.store.InspectSemanticObject(ctx, f.session.ScopeContext(), memory.SemanticObjectClaim, oldID)
			if err != nil {
				t.Fatal(err)
			}
			if len(old.Lifecycle) != 2 || old.Lifecycle[len(old.Lifecycle)-1].State != memory.SemanticStateSuperseded || old.Claim.Object.Literal.Value != "tea" || len(old.Sources) != 1 {
				t.Fatalf("old history lost %+v", old)
			}
			current, err := f.store.InspectSemanticObject(ctx, f.session.ScopeContext(), memory.SemanticObjectClaim, result.Operation.ClaimIDs[0])
			if err != nil {
				t.Fatal(err)
			}
			if current.Claim.Object.Literal.Value != "coffee" || current.Sources[0].Source.ObservedAt == "" || current.Sources[0].Source.ObservedAt == effective.Format(time.RFC3339Nano) {
				t.Fatal("world/observed times conflated")
			}
			before := effective.Add(-time.Hour)
			view, err := f.store.InspectClaims(ctx, f.session.ScopeContext(), memory.ClaimQuery{ValidAt: &before})
			if err != nil {
				t.Fatal(err)
			}
			tea := false
			for _, c := range view.Claims {
				if c.ID == oldID {
					tea = true
				}
			}
			if tea != (mode == memory.CorrectionChanged) {
				t.Fatalf("changed/error history distinction lost %+v", view)
			}
			assertTemporalReplay(t, f)
		})
	}
}

func TestOwnerReviewTemporalUnknownTimeConflictNegationAndSupport(t *testing.T) {
	ctx := context.Background()
	f := newCompilerFixture(t)
	a := temporalAuthority(t, f)
	// Uncertain dates remain unknown; an ordinary assertion keeps the earlier
	// proposition active and exposes opposite polarity rather than choosing a winner.
	compiled := compileTemporal(t, f, "I no longer drink tea; I don't know when I stopped.", func(c *memory.ExtractorCandidate) { c.Proposition.Polarity = memory.PolarityDenied })
	p, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(compiled), "accept")
	if err != nil {
		t.Fatal(err)
	}
	if p.Effect.Claims[0].Claim.ValidTime.From != nil || p.Effect.Claims[0].Claim.ValidTime.To != nil || len(p.Effect.Claims[0].Conflicts) == 0 || p.Effect.Correction != nil {
		t.Fatalf("invented date or automatic winner %+v", p.Effect)
	}
	first, err := f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000243"))
	if err != nil {
		t.Fatal(err)
	}
	second := compileTemporal(t, f, "To repeat: I do not drink tea.", func(c *memory.ExtractorCandidate) { c.Proposition.Polarity = memory.PolarityDenied })
	support, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(second), "accept")
	if err != nil {
		t.Fatal(err)
	}
	if support.Effect.Claims[0].Create || support.Effect.Claims[0].Claim.ID != first.Operation.ClaimIDs[0] || !support.Effect.Claims[0].Sources[0].Create {
		t.Fatal("additional support duplicated proposition")
	}
	if _, err = f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(support, "90000000-0000-4000-8000-000000000244")); err != nil {
		t.Fatal(err)
	}
	var claims, sources int
	if err = f.db.QueryRow(`SELECT count(*) FROM semantic_claims`).Scan(&claims); err != nil {
		t.Fatal(err)
	}
	if err = f.db.QueryRow(`SELECT count(*) FROM semantic_source_links WHERE claim_id=?`, first.Operation.ClaimIDs[0]).Scan(&sources); err != nil {
		t.Fatal(err)
	}
	if claims != 2 || sources != 2 {
		t.Fatalf("claims/sources %d/%d", claims, sources)
	}
	unknown := compileTemporal(t, f, "I changed to coffee, but don't know when.", func(c *memory.ExtractorCandidate) {
		c.Proposition.Object.Literal.Value = "coffee"
		c.Temporal.Correction = &memory.CandidateCorrectionProposal{Modes: []memory.CorrectionMode{memory.CorrectionChanged}}
	})
	options, err := f.store.OwnerCandidateTemporalOptions(ctx, a, candidateRef(unknown))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.store.ChooseOwnerCandidateTemporal(ctx, a, memory.ReviewTemporalDecision{Candidate: candidateRef(unknown), OptionsSHA256: options.SHA256, Choice: memory.ReviewTemporalChoice{OldClaimID: options.Alternatives[0].Claim.ID, Mode: memory.CorrectionChanged}})
	if err == nil || !strings.Contains(err.Error(), "effective time") {
		t.Fatalf("unknown changed date invented %v", err)
	}
	assertTemporalReplay(t, f)
}

func TestOwnerReviewTemporalPlansRequireExactPredicateAndRemainQualified(t *testing.T) {
	for _, meaning := range []string{"plan", "possibility"} {
		t.Run(meaning, func(t *testing.T) {
			ctx := context.Background()
			f := newCompilerFixture(t)
			a := temporalAuthority(t, f)
			token, label := memory.PlanPredicateToken, memory.PlanPredicateLabel
			if meaning == "possibility" {
				token, label = memory.PossibilityPredicateToken, memory.PossibilityPredicateLabel
			}
			content := "I am considering moving to Paris next year."
			if meaning == "plan" {
				content = "I intend to move to Paris next year."
			}
			compiled := compileTemporal(t, f, content, func(c *memory.ExtractorCandidate) {
				c.Temporal.Meaning = meaning
				c.Proposition.PredicateID = ""
				c.Proposition.Object.Literal.Value = "move to Paris next year"
				c.Identity = &memory.CandidateIdentityProposal{Predicate: &memory.PredicateDefinition{Token: token, Label: label, ObjectConstraint: "text", Cardinality: memory.CardinalityMany}}
			})
			if _, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(compiled), "accept"); err == nil {
				t.Fatal("implicit modal Predicate creation")
			}
			options, err := f.store.OwnerCandidateIdentityOptions(ctx, a, candidateRef(compiled))
			if err != nil {
				t.Fatal(err)
			}
			selected, err := f.store.ChooseOwnerCandidateIdentity(ctx, a, memory.ReviewIdentityDecision{Candidate: candidateRef(compiled), OptionsSHA256: options.SHA256, Choices: memory.ReviewIdentityChoices{Predicate: &memory.ReviewPredicateChoice{Create: true}}})
			if err != nil {
				t.Fatal(err)
			}
			p, err := f.store.PrepareOwnerCandidateReview(ctx, a, selected.Ref, "accept")
			if err != nil {
				t.Fatal(err)
			}
			if p.Effect.Correction != nil || p.Effect.Claims[0].Predicate.Token != token || !p.Effect.Claims[0].Predicate.Create {
				t.Fatal("plan became completed change")
			}
			result, err := f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000245"))
			if err != nil {
				t.Fatal(err)
			}
			claim, err := f.store.InspectSemanticObject(ctx, f.session.ScopeContext(), memory.SemanticObjectClaim, result.Operation.ClaimIDs[0])
			if err != nil {
				t.Fatal(err)
			}
			if claim.Claim.Predicate.Label != label || claim.Claim.Object.Literal.Value != "move to Paris next year" || claim.Claim.ValidTime.From != nil {
				t.Fatalf("qualification lost %+v", claim)
			}
			assertTemporalReplay(t, f)
		})
	}
}

func TestOwnerReviewTemporalStaleChoicePolicyAndAtomicRollback(t *testing.T) {
	for _, failure := range []string{"choice", "scope", "policy", "source", "rollback", "reject"} {
		t.Run(failure, func(t *testing.T) {
			ctx := context.Background()
			f := newCompilerFixture(t)
			a := temporalAuthority(t, f)
			compiled := compileTemporal(t, f, "I was mistaken about tea. I drink coffee.", func(c *memory.ExtractorCandidate) {
				c.Proposition.Object.Literal.Value = "coffee"
				c.Temporal.Correction = &memory.CandidateCorrectionProposal{Modes: []memory.CorrectionMode{memory.CorrectionError, memory.CorrectionChanged}}
			})
			selected := chooseTemporal(t, f, a, candidateRef(compiled), memory.CorrectionError)
			action := "accept"
			if failure == "reject" {
				action = "reject"
			}
			p, err := f.store.PrepareOwnerCandidateReview(ctx, a, selected.Ref, action)
			if err != nil {
				t.Fatal(err)
			}
			switch failure {
			case "choice":
				_ = chooseTemporal(t, f, a, selected.Ref, memory.CorrectionError)
			case "scope":
				if _, err = f.db.Exec(`UPDATE semantic_scopes SET revision=revision+1 WHERE scope_key='global'`); err != nil {
					t.Fatal(err)
				}
			case "policy":
				if _, err = f.db.Exec(`UPDATE memory_review_authorization SET source_policy='changed'`); err != nil {
					t.Fatal(err)
				}
			case "source":
				if _, err = f.db.Exec(`DROP TRIGGER events_append_only_update`); err != nil {
					t.Fatal(err)
				}
				if _, err = f.db.Exec(`UPDATE events SET content='changed' WHERE id=?`, compiled.Candidates[0].Support[0].Locator.EventID); err != nil {
					t.Fatal(err)
				}
			case "rollback":
				if _, err = f.db.Exec(`CREATE TRIGGER fail_temporal BEFORE INSERT ON semantic_claim_corrections BEGIN SELECT RAISE(ABORT,'temporal rollback');END`); err != nil {
					t.Fatal(err)
				}
			}
			result, err := f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000246"))
			if failure == "reject" {
				if err != nil || result.Operation != nil {
					t.Fatalf("reject %+v %v", result, err)
				}
			} else if err == nil {
				t.Fatal("invalid correction committed")
			}
			var corrections, claims, operations int
			for _, q := range []struct {
				sql string
				v   *int
			}{{`SELECT count(*) FROM semantic_claim_corrections`, &corrections}, {`SELECT count(*) FROM semantic_claims`, &claims}, {`SELECT count(*) FROM semantic_operations WHERE schema_version=6`, &operations}} {
				if err = f.db.QueryRow(q.sql).Scan(q.v); err != nil {
					t.Fatal(err)
				}
			}
			if corrections != 0 || claims != 1 || operations != 0 {
				t.Fatalf("partial acceptance %d/%d/%d", corrections, claims, operations)
			}
		})
	}
}

func TestOwnerReviewTemporalTwoStoreChoiceRace(t *testing.T) {
	ctx := context.Background()
	f := newCompilerFixture(t)
	a := temporalAuthority(t, f)
	compiled := compileTemporal(t, f, "I was mistaken. I drink coffee.", func(c *memory.ExtractorCandidate) {
		c.Proposition.Object.Literal.Value = "coffee"
		c.Temporal.Correction = &memory.CandidateCorrectionProposal{Modes: []memory.CorrectionMode{memory.CorrectionError}}
	})
	options, err := f.store.OwnerCandidateTemporalOptions(ctx, a, candidateRef(compiled))
	if err != nil {
		t.Fatal(err)
	}
	db, err := eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	other := eviedb.NewStore(db)
	b, err := other.LocalOwnerReviewContext(ctx, "global")
	if err != nil {
		t.Fatal(err)
	}
	decision := memory.ReviewTemporalDecision{Candidate: candidateRef(compiled), OptionsSHA256: options.SHA256, Choice: memory.ReviewTemporalChoice{OldClaimID: options.Alternatives[0].Claim.ID, Mode: memory.CorrectionError}}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i, s := range []*eviedb.Store{f.store, other} {
		wg.Add(1)
		go func(i int, s *eviedb.Store) {
			defer wg.Done()
			auth := a
			if i == 1 {
				auth = b
			}
			_, err := s.ChooseOwnerCandidateTemporal(ctx, auth, decision)
			errs <- err
		}(i, s)
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
		t.Fatal(fmt.Sprintf("choice race %d/%d", success, stale))
	}
}

func TestOwnerReviewTemporalExactDuplicateAddsNoGraphRows(t *testing.T) {
	ctx := context.Background()
	f := newCompilerFixture(t)
	a := temporalAuthority(t, f)
	var root memory.EventID
	if err := f.db.QueryRow(`SELECT id FROM events WHERE session_id=? AND sequence=1`, f.session.ID).Scan(&root); err != nil {
		t.Fatal(err)
	}
	last := f.append(t, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root, Content: "Recorded."})
	selection := memory.CompilationSelection{SessionID: f.session.ID, RootID: root, Cutoff: last.Sequence, Destination: "global"}
	extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		c := f.candidate(r)
		c.Temporal = &memory.CandidateTemporalProposal{Meaning: "assertion"}
		return compilerOutput(r, []memory.ExtractorCandidate{c}), nil
	}}
	compiled, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), selection, temporalGeneration(), extractor)
	if err != nil || compiled.State != "completed_candidates" {
		t.Fatalf("compile duplicate %s %v", compiled.State, err)
	}
	p, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(compiled), "accept")
	if err != nil {
		t.Fatal(err)
	}
	if p.Effect.Claims[0].Create || p.Effect.Claims[0].Sources[0].Create {
		t.Fatal("exact duplicate produced graph rows")
	}
	decision := decisionFor(p, "90000000-0000-4000-8000-000000000347")
	first, err := f.store.ResolveOwnerCandidateReview(ctx, a, decision)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.store.ResolveOwnerCandidateReview(ctx, a, decision)
	if err != nil || again.AuditID != first.AuditID {
		t.Fatal("duplicate delivery created another outcome")
	}
	var claims, sources, ops int
	if err := f.db.QueryRow(`SELECT count(*) FROM semantic_claims`).Scan(&claims); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`SELECT count(*) FROM semantic_source_links`).Scan(&sources); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`SELECT count(*) FROM semantic_operations WHERE schema_version=6`).Scan(&ops); err != nil {
		t.Fatal(err)
	}
	if claims != 1 || sources != 1 || ops != 1 {
		t.Fatalf("duplicate claim/source/review counts %d/%d/%d", claims, sources, ops)
	}
	assertTemporalReplay(t, f)
}

func TestOwnerReviewTemporalTypedLiteralsRemainCanonical(t *testing.T) {
	for i, tc := range []struct {
		kind          memory.LiteralKind
		before, after string
	}{{memory.LiteralInteger, "1", "2"}, {memory.LiteralDecimal, "1.5", "2.5"}, {memory.LiteralBoolean, "false", "true"}, {memory.LiteralDate, "2025-01-01", "2025-01-02"}, {memory.LiteralDatetime, "2025-01-01T00:00:00.000000000Z", "2025-01-02T00:00:00.000000000Z"}} {
		t.Run(string(tc.kind), func(t *testing.T) {
			ctx := context.Background()
			f := newCompilerFixture(t)
			a := temporalAuthority(t, f)
			source := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "My recorded value is " + tc.before})
			explicit, err := f.store.PrepareRememberLiteral(ctx, f.session.ScopeContext(), memory.RememberLiteralRequest{IdempotencyKey: fmt.Sprintf("idem:v1:90000000-0000-4000-8000-%012d", 400+i), SourceEventID: source.ID, Predicate: "recorded_value", PredicateLabel: "recorded value", Literal: memory.TypedLiteral{Kind: tc.kind, Value: tc.before}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.store.ApplyRememberLiteral(ctx, f.lease, explicit); err != nil {
				t.Fatal(err)
			}
			f.predicate = explicit.Predicate.ID
			compiled := compileTemporal(t, f, "My recorded value is now "+tc.after, func(c *memory.ExtractorCandidate) {
				c.Proposition.Object.Literal = &memory.TypedLiteral{Kind: tc.kind, Value: tc.after}
			})
			p, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(compiled), "accept")
			if err != nil {
				t.Fatal(err)
			}
			result, err := f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, fmt.Sprintf("90000000-0000-4000-8000-%012d", 410+i)))
			if err != nil {
				t.Fatal(err)
			}
			inspected, err := f.store.InspectSemanticObject(ctx, f.session.ScopeContext(), memory.SemanticObjectClaim, result.Operation.ClaimIDs[0])
			if err != nil {
				t.Fatal(err)
			}
			if *inspected.Claim.Object.Literal != (memory.TypedLiteral{Kind: tc.kind, Value: tc.after}) || inspected.Claim.Predicate.ObjectConstraint != memory.PredicateObjectConstraint(tc.kind) {
				t.Fatalf("typed literal changed %+v", inspected.Claim)
			}
			assertTemporalReplay(t, f)
		})
	}
}

func TestOwnerReviewTemporalReusesEarlierReviewSourceTimestampEncoding(t *testing.T) {
	ctx := context.Background()
	f := newCompilerFixture(t)
	a := temporalAuthority(t, f)
	selection := f.selection(t, "I drink coffee.", true)
	for i, g := range []memory.CompilerGeneration{compilerGeneration(), temporalGeneration()} {
		extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
			c := f.candidate(r)
			c.Proposition.Object.Literal.Value = "coffee"
			if i == 1 {
				c.Temporal = &memory.CandidateTemporalProposal{Meaning: "assertion"}
			}
			return compilerOutput(r, []memory.ExtractorCandidate{c}), nil
		}}
		compiled, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), selection, g, extractor)
		if err != nil || compiled.State != "completed_candidates" {
			t.Fatalf("compile %s %v", compiled.State, err)
		}
		p, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(compiled), "accept")
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 && (p.Effect.Claims[0].Create || p.Effect.Claims[0].Sources[0].Create) {
			t.Fatal("v3 duplicated older review support")
		}
		if _, err = f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, fmt.Sprintf("90000000-0000-4000-8000-%012d", 450+i))); err != nil {
			t.Fatal(err)
		}
	}
	assertTemporalReplay(t, f)
}

func TestCompilerTemporalGenerationPolicyIsExplicit(t *testing.T) {
	g := temporalGeneration()
	if _, _, err := memory.CompilerGenerationIdentity(g); err != nil {
		t.Fatal(err)
	}
	g.EffectPolicy = memory.CompilerIdentityPolicyV2
	if _, _, err := memory.CompilerGenerationIdentity(g); err == nil {
		t.Fatal("mixed temporal interpretation generation admitted")
	}
	g = temporalGeneration()
	g.EvidencePolicy = memory.CompilerTemporalPolicyV3
	if _, _, err := memory.CompilerGenerationIdentity(g); err == nil {
		t.Fatal("temporal interpretation silently changed evidence policy")
	}
}
