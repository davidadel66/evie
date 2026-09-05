package eviedb

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func recurrenceEncodingFixture() (memory.CompilerGeneration, memory.CompilerRequest, memory.MemoryCandidate) {
	g := compilerCommitGeneration()
	g.EntityPolicy = memory.CompilerTemporalPolicyV3
	g.PredicatePolicy = g.EntityPolicy
	g.ValidationPolicy = g.EntityPolicy
	g.EffectPolicy = g.EntityPolicy
	g.EquivalencePolicy = memory.CompilerEquivalencePolicyV2
	locator := memory.EvidenceLocator{EventID: "event-1", EventPart: memory.EvidenceContent, LocatorKind: memory.LocatorWhole, EvidenceSHA256: strings.Repeat("a", 64)}
	r := memory.CompilerRequest{Window: memory.CompilerWindow{Selection: memory.CompilationSelection{Destination: "global", SessionID: "session-1", RootID: "event-1", Cutoff: 2}}, Entities: []memory.SemanticEntity{{ID: "entity-1", ScopeKey: "global", CanonicalName: "Owner", EntityType: "person", AnchorKind: "owner"}}, Predicates: []memory.SemanticPredicate{{ID: "predicate-1", Token: "drinks", Version: 1, Label: "Drinks", ObjectConstraint: "text", Cardinality: memory.CardinalityMany}}}
	instant := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	c := memory.MemoryCandidate{Proposal: memory.ExtractorCandidate{Proposition: memory.ClaimProposition{SubjectEntityID: "entity-1", PredicateID: "predicate-1", Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "tea"}}, Polarity: memory.PolarityAffirmed}, ValidTime: memory.ValidTime{From: &instant}, Support: []memory.EvidenceLocator{locator}, Context: []memory.EvidenceLocator{}, Identity: &memory.CandidateIdentityProposal{}, Temporal: &memory.CandidateTemporalProposal{Meaning: "assertion", Correction: &memory.CandidateCorrectionProposal{Modes: []memory.CorrectionMode{memory.CorrectionError, memory.CorrectionChanged}, EffectiveTime: &instant}}}, Support: []memory.CompilerSource{{SourceType: memory.SourceTypeUserMessage, Locator: locator, SessionID: "session-1", ScopeKey: "global", Sequence: 1, FormatVersion: 1, ObservedAt: "2026-09-05T10:00:00Z", Actor: memory.SemanticActorOwner, Authority: memory.AuthorityOwnerStatement, Usage: "new_support", Evidence: "I drink tea."}}, Context: []memory.CompilerSource{}}
	return g, r, c
}
func TestCompilerRecurrenceV2CanonicalGoldenAndCosmeticInvariance(t *testing.T) {
	g, r, c := recurrenceEncodingFixture()
	original := string(compilerJSON(c))
	exact, _, err := compilerRecurrenceCanonical(g, r, c)
	if err != nil {
		t.Fatal(err)
	}
	golden, err := os.ReadFile("testdata/compiler-recurrence-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(golden)) != string(exact) {
		t.Fatalf("canonical recurrence changed:\n%s", exact)
	}
	confidence := 0.9
	c.Proposal.Identity.Confidence = &confidence
	c.Proposal.Identity.Uncertainty = "different model prose"
	c.ID = "another"
	c.ReviewState = "accepted"
	c.ReviewRevision = 99
	c.EquivalentTo = "different"
	g.Prompt = "A different extraction prompt"
	g.ModelArtifact = "different model"
	g.Decoding.Seed++
	zone := time.FixedZone("east", 2*60*60)
	x := c.Proposal.ValidTime.From.In(zone)
	c.Proposal.ValidTime.From = &x
	c.Proposal.Temporal.Correction.Modes[0], c.Proposal.Temporal.Correction.Modes[1] = c.Proposal.Temporal.Correction.Modes[1], c.Proposal.Temporal.Correction.Modes[0]
	again, _, err := compilerRecurrenceCanonical(g, r, c)
	if err != nil || string(again) != string(exact) {
		t.Fatalf("cosmetic difference %s %v", again, err)
	}
	_, _, base := recurrenceEncodingFixture()
	_, _, err = compilerRecurrenceCanonical(g, r, base)
	if err != nil || string(compilerJSON(base)) != original {
		t.Fatal("canonicalization mutated extraction")
	}
}
func TestCompilerRecurrenceFullEffectDifferences(t *testing.T) {
	mutations := map[string]func(*memory.CompilerGeneration, *memory.CompilerRequest, *memory.MemoryCandidate){
		"scope": func(_ *memory.CompilerGeneration, r *memory.CompilerRequest, _ *memory.MemoryCandidate) {
			r.Window.Selection.Destination = "session:other"
		},
		"lineage": func(_ *memory.CompilerGeneration, r *memory.CompilerRequest, _ *memory.MemoryCandidate) {
			r.Window.Selection.SessionID = "another"
		},
		"Entity definition": func(_ *memory.CompilerGeneration, r *memory.CompilerRequest, _ *memory.MemoryCandidate) {
			r.Entities[0].EntityType = "organization"
		},
		"Predicate definition": func(_ *memory.CompilerGeneration, r *memory.CompilerRequest, _ *memory.MemoryCandidate) {
			r.Predicates[0].Cardinality = memory.CardinalityOne
		},
		"literal": func(_ *memory.CompilerGeneration, _ *memory.CompilerRequest, c *memory.MemoryCandidate) {
			c.Proposal.Proposition.Object.Literal.Value = "coffee"
		},
		"polarity": func(_ *memory.CompilerGeneration, _ *memory.CompilerRequest, c *memory.MemoryCandidate) {
			c.Proposal.Proposition.Polarity = memory.PolarityDenied
		},
		"modality": func(_ *memory.CompilerGeneration, _ *memory.CompilerRequest, c *memory.MemoryCandidate) {
			c.Proposal.Temporal.Meaning = "plan"
		},
		"correction": func(_ *memory.CompilerGeneration, _ *memory.CompilerRequest, c *memory.MemoryCandidate) {
			c.Proposal.Temporal.Correction.Modes = c.Proposal.Temporal.Correction.Modes[:1]
		},
		"effective instant": func(_ *memory.CompilerGeneration, _ *memory.CompilerRequest, c *memory.MemoryCandidate) {
			x := c.Proposal.Temporal.Correction.EffectiveTime.Add(time.Hour)
			c.Proposal.Temporal.Correction.EffectiveTime = &x
		},
		"source authority": func(_ *memory.CompilerGeneration, _ *memory.CompilerRequest, c *memory.MemoryCandidate) {
			c.Support[0].Authority = memory.AuthorityToolObservation
		},
		"source hash": func(_ *memory.CompilerGeneration, _ *memory.CompilerRequest, c *memory.MemoryCandidate) {
			c.Support[0].Locator.EvidenceSHA256 = strings.Repeat("b", 64)
		},
		"source contract": func(_ *memory.CompilerGeneration, _ *memory.CompilerRequest, c *memory.MemoryCandidate) {
			c.Support[0].FormatVersion = 2
		},
		"source context": func(_ *memory.CompilerGeneration, _ *memory.CompilerRequest, c *memory.MemoryCandidate) {
			c.Context = append(c.Context, c.Support[0])
		},
		"policy": func(g *memory.CompilerGeneration, _ *memory.CompilerRequest, _ *memory.MemoryCandidate) {
			g.EffectPolicy = "future"
		},
	}
	g, r, c := recurrenceEncodingFixture()
	base, _, _ := compilerRecurrenceCanonical(g, r, c)
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			g, r, c := recurrenceEncodingFixture()
			mutate(&g, &r, &c)
			got, _, err := compilerRecurrenceCanonical(g, r, c)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) == string(base) {
				t.Fatal("different effect suppressed")
			}
		})
	}
}
func TestCompilerRecurrenceUnresolvedIdentityIsSourceBound(t *testing.T) {
	g, r, c := recurrenceEncodingFixture()
	c.Proposal.Proposition.SubjectEntityID = ""
	c.Proposal.Identity.Subject = &memory.EntityMention{Name: "Maya", EntityType: "person", Support: c.Proposal.Support[0]}
	first, _, err := compilerRecurrenceCanonical(g, r, c)
	if err != nil {
		t.Fatal(err)
	}
	r.Entities = append(r.Entities, memory.SemanticEntity{ID: "known-maya", ScopeKey: "global", CanonicalName: "Maya", EntityType: "person"})
	second, _, err := compilerRecurrenceCanonical(g, r, c)
	if err != nil || string(first) != string(second) {
		t.Fatal("current name options changed the original source-bound placeholder")
	}
	c.Proposal.Identity.Subject.Support.EventID = "other-source"
	third, _, err := compilerRecurrenceCanonical(g, r, c)
	if err != nil || string(third) == string(second) {
		t.Fatal("different source-bound person suppressed")
	}
	c.Proposal.Identity.Subject = nil
	c.Proposal.Proposition.SubjectEntityID = "known-maya"
	fourth, _, err := compilerRecurrenceCanonical(g, r, c)
	if err != nil || string(fourth) == string(second) {
		t.Fatal("unknown collapsed into known Entity")
	}
}
func TestCompilerRecurrenceGenerationPinsMaterialFields(t *testing.T) {
	base := compilerCommitGeneration()
	id, bytes, err := memory.CompilerGenerationIdentity(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"model_artifact", "quantization", "runtime_version", "prompt", "tokenizer_sha256", "token_bound_proof_sha256", "equivalence_policy"} {
		t.Run(field, func(t *testing.T) {
			var fields map[string]any
			_ = json.Unmarshal(bytes, &fields)
			switch field {
			case "tokenizer_sha256", "token_bound_proof_sha256":
				fields[field] = strings.Repeat("f", 64)
			case "equivalence_policy":
				fields[field] = memory.CompilerEquivalencePolicyV2
			default:
				fields[field] = fields[field].(string) + " changed"
			}
			raw, _ := json.Marshal(fields)
			var changed memory.CompilerGeneration
			_ = json.Unmarshal(raw, &changed)
			next, _, err := memory.CompilerGenerationIdentity(changed)
			if err != nil || next == id {
				t.Fatalf("material generation field %s: %s %v", field, next, err)
			}
		})
	}
}
