package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type temporalCLIExtractor struct {
	reviewCLIExtractor
	adapt func(*memory.ExtractorCandidate)
}

func (e temporalCLIExtractor) Extract(ctx context.Context, g memory.CompilerGeneration, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
	extraction, err := e.reviewCLIExtractor.Extract(ctx, g, r)
	if err != nil {
		return extraction, err
	}
	var response memory.CompilerResponse
	if err = json.Unmarshal(extraction.Raw, &response); err != nil {
		return extraction, err
	}
	response.Candidates[0].Temporal = &memory.CandidateTemporalProposal{Meaning: "assertion"}
	e.adapt(&response.Candidates[0])
	extraction.Raw, err = json.Marshal(response)
	return extraction, err
}

func TestOwnerReviewTemporalCLIClosedSession(t *testing.T) {
	for _, scenario := range []string{"error", "changed", "negation", "unknown", "plan", "possibility", "additional-support"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store := eviedb.NewStore(db)
			session, err := store.CreateGlobalSession(ctx)
			if err != nil {
				t.Fatal(err)
			}
			lease, err := store.AcquireTurnLease(ctx, session.ID, "temporal-cli", time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			appendEvent := func(input memory.EventInput) memory.Event {
				t.Helper()
				e, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, input)
				if err != nil {
					t.Fatal(err)
				}
				return e
			}
			seed := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I drink tea."})
			explicit, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{IdempotencyKey: "idem:v1:91000000-0000-4000-8000-000000000342", SourceEventID: seed.ID, Predicate: "drink", PredicateLabel: "drink", Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "tea"}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.ApplyRememberLiteral(ctx, lease, explicit); err != nil {
				t.Fatal(err)
			}
			content := map[string]string{"error": "I was mistaken about tea. I drink coffee.", "changed": "Since May 1 2025 I changed from tea to coffee.", "negation": "I do not drink tea.", "unknown": "I drink coffee now but do not know when I started.", "plan": "I intend to move to Paris next year.", "possibility": "I am considering moving to Paris next year.", "additional-support": "I still drink tea."}[scenario]
			root := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: content})
			last := appendEvent(memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "Recorded."})
			g := reviewCLIGeneration()
			g.EntityPolicy = memory.CompilerTemporalPolicyV3
			g.PredicatePolicy = g.EntityPolicy
			g.ValidationPolicy = g.EntityPolicy
			g.EquivalencePolicy = g.EntityPolicy
			g.EffectPolicy = g.EntityPolicy
			effective := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
			extractor := temporalCLIExtractor{reviewCLIExtractor: reviewCLIExtractor{explicit.Subject.ID, explicit.Predicate.ID}, adapt: func(c *memory.ExtractorCandidate) {
				switch scenario {
				case "error", "changed":
					c.Temporal.Correction = &memory.CandidateCorrectionProposal{Modes: []memory.CorrectionMode{memory.CorrectionMode(scenario)}}
					if scenario == "changed" {
						c.Temporal.Correction.EffectiveTime = &effective
						c.ValidTime.From = &effective
					}
				case "negation":
					c.Proposition.Object.Literal.Value = "tea"
					c.Proposition.Polarity = memory.PolarityDenied
				case "additional-support":
					c.Proposition.Object.Literal.Value = "tea"
				case "plan", "possibility":
					token, label := memory.PlanPredicateToken, memory.PlanPredicateLabel
					if scenario == "possibility" {
						token, label = memory.PossibilityPredicateToken, memory.PossibilityPredicateLabel
					}
					c.Temporal.Meaning = scenario
					c.Proposition.PredicateID = ""
					c.Proposition.Object.Literal.Value = "move to Paris next year"
					c.Identity = &memory.CandidateIdentityProposal{Predicate: &memory.PredicateDefinition{Token: token, Label: label, ObjectConstraint: "text", Cardinality: memory.CardinalityMany}}
				}
			}}
			compiled, err := store.CompileCandidateUnit(ctx, session.ScopeContext(), memory.CompilationSelection{SessionID: session.ID, RootID: root.ID, Cutoff: last.Sequence, Destination: "global"}, g, extractor)
			if err != nil || compiled.State != "completed_candidates" {
				t.Fatalf("compile %s: %v", compiled.State, err)
			}
			if err = store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(`UPDATE sessions SET status='closed' WHERE id=?`, session.ID); err != nil {
				t.Fatal(err)
			}
			run := func(target any, args ...string) {
				t.Helper()
				var out bytes.Buffer
				handled, err := runOwnerReviewManagement(ctx, append([]string{"memory-review"}, args...), &out, store)
				if !handled || err != nil {
					t.Fatalf("CLI %v: %v", args, err)
				}
				if err = json.Unmarshal(out.Bytes(), target); err != nil {
					t.Fatal(err)
				}
			}
			var page memory.OwnerCandidatePage
			run(&page, "inbox", "--scope", "global")
			if len(page.Candidates) != 1 {
				t.Fatal("missing candidate")
			}
			item := page.Candidates[0]
			if scenario == "error" || scenario == "changed" {
				var options memory.ReviewTemporalOptions
				run(&options, "temporal-options", "--scope", "global", "--id", item.Ref.ID)
				choice, _ := json.Marshal(memory.ReviewTemporalChoice{OldClaimID: explicit.ClaimID, Mode: memory.CorrectionMode(scenario)})
				run(&item, "temporal-choose", "--scope", "global", "--id", item.Ref.ID, "--options", options.SHA256, "--choices", string(choice))
				var historical memory.ReviewTemporalRevision
				run(&historical, "temporal-revision", "--scope", "global", "--id", item.Ref.ID, "--interpretation", "1")
				if historical.Choice.Mode != memory.CorrectionMode(scenario) {
					t.Fatal("choice history lost")
				}
			}
			if scenario == "plan" || scenario == "possibility" {
				var options memory.ReviewIdentityOptions
				run(&options, "alternatives", "--scope", "global", "--id", item.Ref.ID)
				choice, _ := json.Marshal(memory.ReviewIdentityChoices{Predicate: &memory.ReviewPredicateChoice{Create: true}})
				run(&item, "choose", "--scope", "global", "--id", item.Ref.ID, "--options", options.SHA256, "--choices", string(choice))
			}
			var preview memory.ReviewPreview
			run(&preview, "prepare", "--scope", "global", "--id", item.Ref.ID, "--revision", fmt.Sprint(item.Ref.ReviewRevision), "--interpretation", fmt.Sprint(item.Ref.InterpretationRevision), "--action", "accept")
			if scenario == "additional-support" && preview.Effect.Claims[0].Create {
				t.Fatal("additional source creates duplicate Claim")
			}
			var result memory.ReviewResult
			run(&result, "resolve", "--scope", "global", "--preview", preview.ID, "--digest", preview.SHA256, "--delivery", "idem:v1:91000000-0000-4000-8000-000000000343", "--action", "accept")
			var operation memory.OwnerReviewOperation
			run(&operation, "operation", "--scope", "global", "--id", string(result.Operation.OperationID))
			if operation.Preview.Effect.Claims[0].Sources[0].Evidence != content {
				t.Fatal("original evidence lost")
			}
			inspecting, err := store.CreateGlobalSession(ctx)
			if err != nil {
				t.Fatal(err)
			}
			claim, err := store.InspectSemanticObject(ctx, inspecting.ScopeContext(), memory.SemanticObjectClaim, result.Operation.ClaimIDs[0])
			if err != nil {
				t.Fatal(err)
			}
			if claim.Claim.Predicate != preview.Effect.Claims[0].Predicate { // Created flag belongs only to the effect.
				expected := preview.Effect.Claims[0].Predicate
				expected.Create = false
				if claim.Claim.Predicate != expected {
					t.Fatal("accepted Predicate meaning changed")
				}
			}
			replay, err := store.VerifySemanticProjection(ctx)
			if err != nil || !replay.Valid {
				t.Fatalf("CLI replay %v %+v", err, replay)
			}
		})
	}
}
