package eviedb_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"strings"
	"testing"
	"time"
)

func workspaceDestinationFixture(t *testing.T) *compilerFixture {
	t.Helper()
	f := newCompilerFixture(t)
	ctx := context.Background()
	if err := f.store.ReleaseTurnLease(ctx, f.session.ID, f.lease.HolderID, f.lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	workspace, err := f.store.RegisterWorkspace(ctx, "General")
	if err != nil {
		t.Fatal(err)
	}
	f.session, err = f.store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, reviewTestReceipt())
	if err != nil {
		t.Fatal(err)
	}
	f.lease, err = f.store.AcquireTurnLease(ctx, f.session.ID, "destination-test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestRememberRecommendedDestinationsPreserveSourceAndReplay(t *testing.T) {
	for _, destination := range []memory.MemoryDestination{memory.MemoryEverywhere, memory.MemoryWorkspace, memory.MemorySession} {
		t.Run(string(destination), func(t *testing.T) {
			f := workspaceDestinationFixture(t)
			ctx := context.Background()
			source := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Remember I prefer concise answers."})
			req := memory.RememberLiteralRequest{Destination: destination, IdempotencyKey: "idem:v1:90000000-0000-4000-8000-000000000871", SourceEventID: source.ID, Predicate: "response_style", PredicateLabel: "response style", Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "Concise answers"}}
			p, err := f.store.PrepareRememberLiteral(ctx, f.session.ScopeContext(), req)
			if err != nil {
				t.Fatal(err)
			}
			expected, _ := memory.ResolveMemoryDestination(f.session.ScopeContext(), destination, false)
			if p.Scope.Key != expected || p.Source.ScopeKey != "workspace:"+string(f.session.WorkspaceID) {
				t.Fatalf("incorrect destination/source: %+v", p)
			}
			var count int
			f.db.QueryRow(`SELECT count(*) FROM semantic_claims WHERE claim_id=?`, p.ClaimID).Scan(&count)
			if count != 0 {
				t.Fatal("prepare wrote a claim")
			}
			changed := p
			changed.Request.Destination = memory.MemoryWorkspace
			if destination == memory.MemoryWorkspace {
				changed.Request.Destination = memory.MemoryEverywhere
			}
			if _, err := f.store.ApplyRememberLiteral(ctx, f.lease, changed); err == nil {
				t.Fatal("changed destination accepted")
			}
			if _, err := f.store.ApplyRememberLiteral(ctx, f.lease, p); err != nil {
				t.Fatal(err)
			}
			req.Destination = changed.Request.Destination
			if _, err := f.store.PrepareRememberLiteral(ctx, f.session.ScopeContext(), req); !errors.Is(err, eviedb.ErrIdempotencyConflict) {
				t.Fatalf("retry destination: %v", err)
			}
			other, err := f.store.CreateGlobalSession(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if destination == memory.MemoryEverywhere {
				inspection, err := f.store.InspectSemanticObject(ctx, other.ScopeContext(), memory.SemanticObjectClaim, p.ClaimID)
				if err != nil {
					t.Fatal(err)
				}
				for _, source := range inspection.Sources {
					if source.Source.Evidence != "" {
						t.Fatal("workspace quote leaked globally")
					}
				}
			}
			report, err := f.store.VerifySemanticProjection(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !report.Valid {
				t.Fatalf("replay mismatch: %+v", report)
			}
		})
	}
}

func TestCandidateRecommendedDestinationsKeepSourceInbox(t *testing.T) {
	for index, destination := range []memory.MemoryDestination{memory.MemoryEverywhere, memory.MemoryWorkspace, memory.MemorySession} {
		t.Run(string(destination), func(t *testing.T) {
			f := workspaceDestinationFixture(t)
			ctx := context.Background()
			selection := f.selection(t, "I prefer coffee.", true)
			selection.Destination = "workspace:" + string(f.session.WorkspaceID)
			extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
				c := f.candidate(r)
				c.Destination = destination
				c.Proposition.Object.Literal.Value = "coffee"
				return compilerOutput(r, []memory.ExtractorCandidate{c}), nil
			}}
			generation := compilerGeneration()
			generation.ScopePolicy = memory.CompilerScopePolicyV1
			generation.Schema = json.RawMessage(scopeTestSchema)
			compiled, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), selection, generation, extractor)
			if err != nil || compiled.State != "completed_candidates" {
				t.Fatalf("compile=%+v err=%v", compiled, err)
			}
			a, err := f.store.LocalOwnerReviewContext(ctx, selection.Destination)
			if err != nil {
				t.Fatal(err)
			}
			p, err := f.store.PrepareOwnerCandidateReview(ctx, a, candidateRef(compiled), "accept")
			if err != nil {
				t.Fatal(err)
			}
			expected, _ := memory.ResolveMemoryDestination(f.session.ScopeContext(), destination, false)
			if p.ScopeKey != selection.Destination || p.Effect.Scope.Key != expected || p.Effect.Claims[0].Sources[0].ScopeKey != selection.Destination {
				t.Fatal("source/destination conflated")
			}
			if err := f.store.ReleaseTurnLease(ctx, f.session.ID, f.lease.HolderID, f.lease.FencingToken); err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.Exec(`UPDATE sessions SET status='closed' WHERE id=?`, f.session.ID); err != nil {
				t.Fatal(err)
			}
			result, err := f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, fmt.Sprintf("90000000-0000-4000-8000-%012d", 880+index)))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.store.InspectOwnerReviewOperation(ctx, a, result.Operation.OperationID); err != nil {
				t.Fatal(err)
			}
			global, err := f.store.LocalOwnerReviewContext(ctx, "global")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.store.InspectOwnerReviewOperation(ctx, global, result.Operation.OperationID); err == nil {
				t.Fatal("full source preview leaked to global inbox")
			}
			other, err := f.store.CreateGlobalSession(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if destination == memory.MemoryEverywhere {
				inspection, err := f.store.InspectSemanticObject(ctx, other.ScopeContext(), memory.SemanticObjectClaim, result.Operation.ClaimIDs[0])
				if err != nil {
					t.Fatal(err)
				}
				encoded := fmt.Sprintf("%+v", inspection)
				if strings.Contains(encoded, "I prefer coffee.") {
					t.Fatal("private candidate evidence leaked")
				}
			}
			if _, err := f.store.ArchiveWorkspace(ctx, f.session.WorkspaceID); err != nil {
				t.Fatal(err)
			}
			report, err := f.store.VerifySemanticProjection(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !report.Valid {
				t.Fatalf("replay mismatch: %+v", report)
			}
		})
	}
}

func TestRememberEntityRecommendedDestinationAndPrivateReferenceCeiling(t *testing.T) {
	for _, destination := range []memory.MemoryDestination{memory.MemoryEverywhere, memory.MemoryWorkspace, memory.MemorySession} {
		t.Run(string(destination), func(t *testing.T) {
			f := workspaceDestinationFixture(t)
			ctx := context.Background()
			private := seedReviewPerson(t, f, "Maya Private", "Maya", "90000000-0000-4000-8000-000000000901")
			source := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I know Robin."})
			req := memory.RememberEntityRequest{Destination: destination, IdempotencyKey: "idem:v1:90000000-0000-4000-8000-000000000902", SourceEventID: source.ID, Predicate: "knows", PredicateLabel: "knows", Subject: memory.EntitySelector{EntityID: f.subject}, Object: memory.EntitySelector{Create: true, CanonicalName: "Robin", EntityType: "person", Alias: "Robin"}}
			if destination == memory.MemoryEverywhere {
				bad := req
				bad.Object = memory.EntitySelector{EntityID: private}
				if _, err := f.store.PrepareRememberEntity(ctx, f.session.ScopeContext(), bad); err == nil {
					t.Fatal("workspace entity escaped globally")
				}
			}
			p, err := f.store.PrepareRememberEntity(ctx, f.session.ScopeContext(), req)
			if err != nil {
				t.Fatal(err)
			}
			expected, _ := memory.ResolveMemoryDestination(f.session.ScopeContext(), destination, false)
			if p.Scope.Key != expected || p.Source.ScopeKey != "workspace:"+string(f.session.WorkspaceID) {
				t.Fatal("wrong source/destination")
			}
			if _, err := f.store.ApplyRememberEntity(ctx, f.lease, p); err != nil {
				t.Fatal(err)
			}
			assertTemporalReplay(t, f)
		})
	}
}

func compileScopedCandidates(t *testing.T, f *compilerFixture, generation memory.CompilerGeneration, content string, adapt func(memory.ExtractorCandidate) []memory.ExtractorCandidate) (memory.Compilation, eviedb.OwnerReviewContext) {
	t.Helper()
	ctx := context.Background()
	selection := f.selection(t, content, true)
	selection.Destination = "workspace:" + string(f.session.WorkspaceID)
	generation.ScopePolicy = memory.CompilerScopePolicyV1
	generation.Schema = json.RawMessage(scopeTestSchema)
	result, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), selection, generation, &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		c := f.candidate(r)
		c.Destination = memory.MemoryEverywhere
		return compilerOutput(r, adapt(c)), nil
	}})
	if err != nil || result.State != "completed_candidates" {
		t.Fatalf("compile %+v: %v", result, err)
	}
	a, err := f.store.LocalOwnerReviewContext(ctx, selection.Destination)
	if err != nil {
		t.Fatal(err)
	}
	return result, a
}

func TestCandidateRecommendedIdentityAndTemporalDestinations(t *testing.T) {
	t.Run("identity", func(t *testing.T) {
		f := workspaceDestinationFixture(t)
		ctx := context.Background()
		seedReviewPerson(t, f, "Maya Private", "Maya", "90000000-0000-4000-8000-000000000903")
		compiled, a := compileScopedCandidates(t, f, identityGeneration(), "I work with Maya.", func(c memory.ExtractorCandidate) []memory.ExtractorCandidate {
			identityProposal(&c)
			return []memory.ExtractorCandidate{c}
		})
		options, err := f.store.OwnerCandidateIdentityOptions(ctx, a, candidateRef(compiled))
		if err != nil {
			t.Fatal(err)
		}
		if options.ScopeKey != "global" || len(options.Object) != 0 {
			t.Fatalf("private identity offered globally: %+v", options)
		}
		choices := memory.ReviewIdentityChoices{Object: &memory.ReviewEntityChoice{Create: true}}
		if len(options.Predicates) > 0 {
			choices.Predicate = &memory.ReviewPredicateChoice{PredicateID: options.Predicates[0].ID}
		} else {
			choices.Predicate = &memory.ReviewPredicateChoice{Create: true}
		}
		chosen, err := f.store.ChooseOwnerCandidateIdentity(ctx, a, memory.ReviewIdentityDecision{Candidate: candidateRef(compiled), OptionsSHA256: options.SHA256, Choices: choices})
		if err != nil {
			t.Fatal(err)
		}
		p, err := f.store.PrepareOwnerCandidateReview(ctx, a, chosen.Ref, "accept")
		if err != nil {
			t.Fatal(err)
		}
		if p.Effect.Scope.Key != "global" {
			t.Fatal("wrong effect destination")
		}
		if _, err = f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000904")); err != nil {
			t.Fatal(err)
		}
		assertTemporalReplay(t, f)
	})
	t.Run("correction", func(t *testing.T) {
		f := workspaceDestinationFixture(t)
		ctx := context.Background()
		compiled, a := compileScopedCandidates(t, f, temporalGeneration(), "I was mistaken about tea. I drink coffee.", func(c memory.ExtractorCandidate) []memory.ExtractorCandidate {
			c.Proposition.Object.Literal.Value = "coffee"
			c.Temporal = &memory.CandidateTemporalProposal{Meaning: "assertion", Correction: &memory.CandidateCorrectionProposal{Modes: []memory.CorrectionMode{memory.CorrectionError}}}
			return []memory.ExtractorCandidate{c}
		})
		chosen := chooseTemporal(t, f, a, candidateRef(compiled), memory.CorrectionError)
		p, err := f.store.PrepareOwnerCandidateReview(ctx, a, chosen.Ref, "accept")
		if err != nil {
			t.Fatal(err)
		}
		if p.Effect.Scope.Key != "global" || p.Effect.Correction == nil {
			t.Fatal("missing global correction")
		}
		if _, err = f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(p, "90000000-0000-4000-8000-000000000905")); err != nil {
			t.Fatal(err)
		}
		assertTemporalReplay(t, f)
	})
}

func TestCandidateRecommendedBatchDestinations(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(fmt.Sprint(mixed), func(t *testing.T) {
			f := workspaceDestinationFixture(t)
			ctx := context.Background()
			source := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I drink water."})
			seed, err := f.store.PrepareRememberLiteral(ctx, f.session.ScopeContext(), memory.RememberLiteralRequest{Destination: memory.MemoryEverywhere, IdempotencyKey: "idem:v1:90000000-0000-4000-8000-000000000907", SourceEventID: source.ID, Predicate: "batch_drink", PredicateLabel: "batch drink", PredicateCardinality: memory.CardinalityMany, Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "water"}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.store.ApplyRememberLiteral(ctx, f.lease, seed); err != nil {
				t.Fatal(err)
			}
			f.predicate = seed.Predicate.ID
			compiled, a := compileScopedCandidates(t, f, compilerGeneration(), "I prefer coffee and juice.", func(c memory.ExtractorCandidate) []memory.ExtractorCandidate {
				c.Proposition.Object.Literal.Value = "coffee"
				other := c
				literal := *c.Proposition.Object.Literal
				literal.Value = "juice"
				other.Proposition.Object.Literal = &literal
				if mixed {
					other.Destination = memory.MemoryWorkspace
				}
				return []memory.ExtractorCandidate{c, other}
			})
			refs := []memory.CandidateRef{}
			for _, c := range compiled.Candidates {
				refs = append(refs, memory.CandidateRef{ID: c.ID, ReviewRevision: c.ReviewRevision})
			}
			p, err := f.store.PrepareOwnerCandidateBatch(ctx, a, independentBatch(refs))
			if mixed {
				if err == nil {
					t.Fatal("mixed applicability accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.store.ResolveOwnerCandidateBatch(ctx, a, batchDecision(p, "90000000-0000-4000-8000-000000000906")); err != nil {
				t.Fatal(err)
			}
			assertTemporalReplay(t, f)
		})
	}
}

const scopeTestSchema = `{"type":"object","properties":{"candidates":{"type":"array","items":{"type":"object","properties":{"destination":{"type":"string","enum":["everywhere","workspace","session"]}},"required":["destination"]}}}}`

func TestCompilerScopePolicyIsExplicitAndSchemaBound(t *testing.T) {
	legacy := compilerGeneration()
	legacyID, _, err := memory.CompilerGenerationIdentity(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if memory.CompilerSystemPrompt(legacy) != legacy.Prompt {
		t.Fatal("legacy prompt changed")
	}
	scoped := legacy
	scoped.ScopePolicy = memory.CompilerScopePolicyV1
	if _, _, err := memory.CompilerGenerationIdentity(scoped); err == nil {
		t.Fatal("incompatible schema accepted")
	}
	scoped.Schema = json.RawMessage(scopeTestSchema)
	id, _, err := memory.CompilerGenerationIdentity(scoped)
	if err != nil || id == legacyID {
		t.Fatalf("scope policy not bound: %v", err)
	}
	if !strings.Contains(memory.CompilerSystemPrompt(scoped), memory.MemoryScopeRecommendationInstructions) {
		t.Fatal("missing instructions")
	}
	for _, tc := range []struct {
		name        string
		policy      bool
		destination memory.MemoryDestination
		empty       bool
		want        string
	}{
		{"legacy destination", false, memory.MemoryEverywhere, false, "invalid"},
		{"missing destination", true, "", false, "invalid"},
		{"arbitrary destination", true, "workspace:another", false, "invalid"},
		{"abstention", true, "", true, "completed_empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := workspaceDestinationFixture(t)
			g := legacy
			if tc.policy {
				g = scoped
			}
			sel := f.selection(t, "I prefer coffee.", true)
			sel.Destination = "workspace:" + string(f.session.WorkspaceID)
			result, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), sel, g, &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
				cs := []memory.ExtractorCandidate{}
				if !tc.empty {
					c := f.candidate(r)
					c.Destination = tc.destination
					cs = append(cs, c)
				}
				return compilerOutput(r, cs), nil
			}})
			if tc.want == "invalid" {
				if result.State == "completed_candidates" || len(result.Candidates) > 0 {
					t.Fatalf("invalid output accepted %+v %v", result, err)
				}
			} else if err != nil || result.State != tc.want {
				t.Fatalf("abstention %+v %v", result, err)
			}
		})
	}
}
