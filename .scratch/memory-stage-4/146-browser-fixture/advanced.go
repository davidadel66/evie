package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type advancedBrowserExtractor struct {
	build func(memory.CompilerRequest) []memory.ExtractorCandidate
}

func (advancedBrowserExtractor) ServerIdentity() string { return "scripted:advanced-browser" }
func (x advancedBrowserExtractor) Extract(_ context.Context, _ memory.CompilerGeneration, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
	raw, err := json.Marshal(memory.CompilerResponse{RequestID: r.ID, Candidates: x.build(r)})
	return eviedb.CompilerExtraction{Raw: raw, ReleaseEvidence: "completed"}, err
}
func browserCandidate(f *webReviewFixture, r memory.CompilerRequest, value string) memory.ExtractorCandidate {
	c := memory.ExtractorCandidate{Proposition: memory.ClaimProposition{SubjectEntityID: f.subject, PredicateID: f.predicate, Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: value}}, Polarity: memory.PolarityAffirmed}, Support: []memory.EvidenceLocator{}, Context: []memory.EvidenceLocator{}}
	for _, s := range r.Window.Sources {
		if s.Usage == "new_support" {
			c.Support = append(c.Support, s.Locator)
		}
		if s.Usage == "context" {
			c.Context = append(c.Context, s.Locator)
		}
	}
	return c
}
func browserAdvancedGeneration() memory.CompilerGeneration {
	g := webReviewGeneration()
	g.EntityPolicy = memory.CompilerTemporalPolicyV3
	g.PredicatePolicy = g.EntityPolicy
	g.ValidationPolicy = g.EntityPolicy
	g.EquivalencePolicy = g.EntityPolicy
	g.EffectPolicy = g.EntityPolicy
	return g
}
func seedAdvancedBrowserFixture(t *fixtureHarness, f *webReviewFixture) {
	ctx := context.Background()
	for index, team := range []string{"design", "engineering"} {
		e := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I work with Maya from " + team + ". These are distinct people."})
		p, err := f.store.PrepareRememberEntity(ctx, f.session.ScopeContext(), memory.RememberEntityRequest{IdempotencyKey: fmt.Sprintf("idem:v1:90000000-0000-4000-8000-%012d", 1460+index), SourceEventID: e.ID, Predicate: "works_with", PredicateLabel: "works with", Subject: memory.EntitySelector{EntityID: f.subject}, Object: memory.EntitySelector{Create: true, CanonicalName: "Maya", EntityType: "person", Alias: "Maya from " + team}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.store.ApplyRememberEntity(ctx, f.lease, p); err != nil {
			t.Fatal(err)
		}
		f.append(t, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: e.ID, Content: "Recorded."})
	}
	compile := func(label, text string, g memory.CompilerGeneration, clock bool, build func(memory.CompilerRequest) []memory.ExtractorCandidate) {
		root := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: text})
		parent := root.ID
		if clock {
			assistant := f.append(t, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Payload: []byte(`{"tool_calls":[{"id":"browser-clock","name":"get_time","arguments":"{}"}]}`)})
			intent := f.append(t, memory.EventInput{Type: memory.EventToolIntent, ParentID: assistant.ID, ExecutionID: "browser-clock-execution", Payload: []byte(`{"call":{"id":"browser-clock","name":"get_time","arguments":"{}"}}`)})
			observed := f.append(t, memory.EventInput{Type: memory.EventToolSucceeded, Role: memory.RoleTool, ParentID: intent.ID, ExecutionID: "browser-clock-execution", Content: "2026-09-04 11:42:00", Payload: []byte(`{"tool_call_id":"browser-clock","is_error":false}`)})
			parent = observed.ID
		}
		end := f.append(t, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: parent, Content: "Recorded for owner review."})
		result, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), memory.CompilationSelection{SessionID: f.session.ID, RootID: root.ID, Cutoff: end.Sequence, Destination: "global"}, g, advancedBrowserExtractor{build: build})
		if err != nil || result.State != "completed_candidates" {
			t.Fatalf("advanced browser %s: %s %s %v", label, result.State, result.Reason, err)
		}
		ids := []string{}
		for _, c := range result.Candidates {
			ids = append(ids, c.ID)
		}
		raw, _ := json.Marshal(map[string]any{"scenario": label, "candidate_ids": ids})
		fmt.Println("BROWSER_SCENARIO=" + string(raw))
	}
	compile("shared person and Predicate", "Maya works on Atlas and Orion. These are two projects for the same Maya.", browserIdentityGeneration(), false, func(r memory.CompilerRequest) []memory.ExtractorCandidate {
		out := []memory.ExtractorCandidate{}
		for _, project := range []string{"Atlas", "Orion"} {
			c := browserCandidate(f, r, project)
			c.Proposition.SubjectEntityID = ""
			c.Proposition.PredicateID = ""
			c.Identity = &memory.CandidateIdentityProposal{Subject: &memory.EntityMention{Name: "Maya", EntityType: "person", Support: c.Support[0]}, Predicate: &memory.PredicateDefinition{Token: "works_on", Label: "works on", ObjectConstraint: memory.PredicateObjectConstraint("text"), Cardinality: memory.CardinalityMany}, Uncertainty: "Choose the person explicitly. The two suggestions refer to the same proposed Maya only if you bind them in one group."}
			out = append(out, c)
		}
		return out
	})
	compile("error correction", "I was mistaken when I said I drink tea. I drink coffee.", browserAdvancedGeneration(), false, func(r memory.CompilerRequest) []memory.ExtractorCandidate {
		c := browserCandidate(f, r, "coffee")
		c.Temporal = &memory.CandidateTemporalProposal{Meaning: "assertion", Correction: &memory.CandidateCorrectionProposal{Modes: []memory.CorrectionMode{memory.CorrectionMode("error")}}}
		return []memory.ExtractorCandidate{c}
	})
	compile("uncompleted plan", "I intend to move to Boston next year. It is a plan, not a completed move.", browserAdvancedGeneration(), false, func(r memory.CompilerRequest) []memory.ExtractorCandidate {
		c := browserCandidate(f, r, "move to Boston next year")
		c.Proposition.PredicateID = ""
		c.Identity = &memory.CandidateIdentityProposal{Predicate: &memory.PredicateDefinition{Token: memory.PlanPredicateToken, Label: memory.PlanPredicateLabel, ObjectConstraint: memory.PredicateObjectConstraint("text"), Cardinality: memory.CardinalityMany}}
		c.Temporal = &memory.CandidateTemporalProposal{Meaning: "plan"}
		return []memory.ExtractorCandidate{c}
	})
	clockGeneration := webReviewGeneration()
	clockGeneration.EvidencePolicy = memory.CompilerClockEvidencePolicy
	compile("contracted local clock", "Using the checked local date, today I adopted tea as my standing drink.", clockGeneration, true, func(r memory.CompilerRequest) []memory.ExtractorCandidate {
		c := browserCandidate(f, r, "tea adopted on the checked local date 2026-09-04")
		c.TemporalQualification = "The clock has no timezone. The effective instant remains unknown."
		for i, loc := range c.Support {
			for _, s := range r.Window.Sources {
				if s.Locator.EventID == loc.EventID && s.Observation != nil {
					c.Support[i] = memory.EvidenceLocator{EventID: loc.EventID, EventPart: memory.EvidenceContent, LocatorKind: memory.LocatorUTF8ByteRange, LocatorValue: "0:10", EvidenceSHA256: memory.CompilerHash([]byte("2026-09-04"))}
				}
			}
		}
		return []memory.ExtractorCandidate{c}
	})
}

func browserIdentityGeneration() memory.CompilerGeneration {
	g := webReviewGeneration()
	g.EntityPolicy = memory.CompilerIdentityPolicyV2
	g.PredicatePolicy = g.EntityPolicy
	g.ValidationPolicy = g.EntityPolicy
	g.EquivalencePolicy = g.EntityPolicy
	g.EffectPolicy = g.EntityPolicy
	return g
}
