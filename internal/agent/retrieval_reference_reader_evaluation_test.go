package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
)

type referenceReaderClient struct {
	t        *testing.T
	test     referenceCase
	provider *openrouter.Client
	capture  *productionReaderCapture
	requests []openrouter.ChatRequest
	answers  []string
}

func (c *referenceReaderClient) ChatStream(ctx context.Context, request openrouter.ChatRequest, handlers openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	c.requests = append(c.requests, request)
	checkReferenceEvidence(c.t, c.test, request, len(c.requests) == 1)
	if c.t.Failed() {
		return openrouter.ChatResponse{}, errors.New("reference evidence contract failed before model dispatch")
	}
	if len(c.requests) > 4 {
		return openrouter.ChatResponse{}, errors.New("reference reader exceeded four model calls")
	}
	if c.provider == nil {
		return assistantStep("Reference candidate contract checked; no model quality claim.", nil).res, nil
	}
	stem := fmt.Sprintf("%s-%02d", c.test.name, len(c.requests))
	c.capture.stem = stem
	productionReaderWrite(c.t, c.capture.directory, stem+"-composed-request.json", request)
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	started := time.Now()
	response, err := c.provider.ChatStream(ctx, request, handlers)
	productionReaderWrite(c.t, c.capture.directory, stem+"-timing.json", map[string]any{"elapsed_ns": time.Since(started).Nanoseconds(), "usage": response.Usage, "error": fmt.Sprint(err)})
	if err != nil {
		return response, err
	}
	productionReaderWrite(c.t, c.capture.directory, stem+"-normalized-response.json", response)
	if len(response.Choices) != 1 {
		return response, errors.New("unexpected reference reader choice count")
	}
	for _, call := range response.Choices[0].Message.ToolCalls {
		switch call.Function.Name {
		case "memory_search", "memory_search_conversations", "memory_expand_conversation":
		default:
			return response, errors.New("reference reader requested capability outside existing read tools")
		}
	}
	c.answers = append(c.answers, response.Choices[0].Message.Content)
	return response, nil
}

func TestMemoryStage5ReferenceReaderEvidenceContract(t *testing.T) { runReferenceReaderCases(t, false) }
func TestMemoryStage5ReferenceReaderPreflight(t *testing.T) {
	if os.Getenv("EVIE_RUN_REFERENCE_READER_PREFLIGHT") != "1" {
		t.Skip("reference reader metadata discovery is opt-in")
	}
	productionReaderSetup(t, "reference-preflight")
}
func TestMemoryStage5ReferenceReaderEvaluation(t *testing.T) {
	if os.Getenv("EVIE_RUN_REFERENCE_READER_EVAL") != "1" {
		t.Skip("actual reference production reader evaluation is opt-in")
	}
	runReferenceReaderCases(t, true)
}

func runReferenceReaderCases(t *testing.T, model bool) {
	var provider *openrouter.Client
	var capture *productionReaderCapture
	profile, err := openrouter.NewExplicitContextProfile(DefaultModel, 32768, 24576, 768)
	if err != nil {
		t.Fatal(err)
	}
	directory := os.Getenv("EVIE_MEMORY_READER_ARTIFACTS")
	if model {
		raw, err := os.ReadFile(filepath.Join(directory, "freeze.json"))
		if err != nil {
			t.Fatal(err)
		}
		var freeze struct {
			FixtureFiles map[string]string                    `json:"fixture_files_sha256"`
			Profile      openrouter.ContextProfileDiagnostics `json:"profile"`
		}
		if err = json.Unmarshal(raw, &freeze); err != nil {
			t.Fatal(err)
		}
		if freeze.FixtureFiles["retrieval_reference_reader_evaluation_test.go"] == "" {
			t.Fatal("reference fixture not frozen")
		}
		for name, want := range freeze.FixtureFiles {
			source, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(source)
			if hex.EncodeToString(digest[:]) != want {
				t.Fatalf("frozen reference fixture changed: %s", name)
			}
		}
		provider, profile, capture = productionReaderSetup(t, "reference-evaluation")
		if profile.Diagnostics() != freeze.Profile {
			t.Fatal("reference reader model/profile changed since freeze")
		}
	}
	for _, name := range referenceCaseNames {
		t.Run(name, func(t *testing.T) {
			f := newRetrievalFixture(t)
			test := prepareReferenceCase(t, f, name)
			f.refresh()
			before, err := f.store.InspectClaims(context.Background(), test.reader.ScopeContext(), memory.ClaimQuery{})
			if err != nil {
				t.Fatal(err)
			}
			var definitions []tools.Tool
			for _, capability := range plugins.NewMemory(f.store).ToolCapabilities() {
				switch capability.Tool.Schema.Function.Name {
				case "memory_search", "memory_search_conversations", "memory_expand_conversation":
					definitions = append(definitions, capability.Tool)
				}
			}
			client := &referenceReaderClient{t: t, test: test, provider: provider, capture: capture}
			holder := memory.LeaseHolderID("reference-reader-" + string(test.reader.ID))
			session := NewWithToolset(client, profile, f.store.BindHistory(test.reader.ID, holder), test.reader.ScopeContext(), f.store.BindTurnOwner(test.reader.ID, holder), tools.NewToolset(definitions))
			started := time.Now()
			err = session.Send(context.Background(), test.question, &recorder{}, nil)
			if model {
				productionReaderWrite(t, directory, name+"-case.json", map[string]any{"name": name, "question": test.question, "original_preferences": test.required, "forbidden_source_ids": test.forbidden, "setup": test.setup, "whole_turn_elapsed_ns": time.Since(started).Nanoseconds(), "model_calls": len(client.requests), "answers": client.answers, "error": fmt.Sprint(err), "scripted_initial_searches": false})
				productionReaderWrite(t, directory, name+"-all-composed-requests.json", client.requests)
			}
			if err != nil {
				t.Fatal(err)
			}
			after, err := f.store.InspectClaims(context.Background(), test.reader.ScopeContext(), memory.ClaimQuery{})
			if err != nil || !reflect.DeepEqual(before.Claims, after.Claims) || !reflect.DeepEqual(before.ScopeRevisions, after.ScopeRevisions) {
				t.Fatal("reference hypothesis changed accepted facts or identities")
			}
			events, err := f.store.LoadEvents(context.Background(), test.reader.ID)
			if err != nil {
				t.Fatal(err)
			}
			var snapshots []memory.ContextSnapshotPayload
			for _, event := range events {
				if event.Type != memory.EventContextSnapshot {
					continue
				}
				var snapshot memory.ContextSnapshotPayload
				if err = json.Unmarshal(event.Payload, &snapshot); err != nil {
					t.Fatal(err)
				}
				if snapshot.Memory == nil || snapshot.Memory.Interpretation == nil {
					continue
				}
				input := snapshot.Memory.Interpretation
				if input.CurrentBytes > 512 || input.EarlierMessages > 2 || input.EarlierBytes > 768 || input.ExaminedEarlierMessages > 16 || input.ExaminedEarlierBytes > 6144 || input.SummaryBytes > 512 || input.QueryBytes > 1024 || input.ExactSelectors > 2 || input.ExactQueryBytes > 256 {
					t.Fatal("reference interpretation exceeded declared input bounds")
				}
				raw, _ := json.Marshal(input)
				for _, word := range []string{"Maya", "Nora", "jasmine", "birthday", "ceramic"} {
					if strings.Contains(string(raw), word) {
						t.Fatal("content-free diagnostics copied source or query text")
					}
				}
				snapshots = append(snapshots, snapshot)
			}
			if len(snapshots) == 0 {
				t.Fatal("reference request has no interpretation receipt")
			}
			if test.compaction != "" && (snapshots[len(snapshots)-1].ActiveCompactionEventID != test.compaction || snapshots[len(snapshots)-1].Memory.Interpretation.SummaryBytes == 0) {
				t.Fatal("compacted reference lost original continuity")
			}
			if model {
				productionReaderWrite(t, directory, name+"-request-snapshots.json", snapshots)
				if len(client.answers) == 0 {
					t.Fatal("reference reader produced no answer")
				}
				checks := referenceAnswerChecks(test, client.answers[len(client.answers)-1])
				productionReaderWrite(t, directory, name+"-automated-checks.json", checks)
				for check, pass := range checks {
					if !pass {
						t.Errorf("predeclared reference check failed: %s; raw answer retained", check)
					}
				}
			}
		})
	}
}

// These markers supplement the frozen semantic rubric; they do not claim to
// determine whether a question is materially necessary or source-grounded.
func referenceAnswerChecks(test referenceCase, answer string) map[string]bool {
	lower := strings.ToLower(answer)
	checks := map[string]bool{"nonempty_answer": strings.TrimSpace(answer) != "", "does_not_claim_new_save": !strings.Contains(lower, "i saved") && !strings.Contains(lower, "i've saved") && !strings.Contains(lower, "i have saved"), "excluded_preference_absent": !strings.Contains(lower, "sapphire")}
	switch test.name {
	case "mother_or_sister", "compacted_ambiguous":
		checks["recipient_options_named"] = (strings.Contains(lower, "mother") || strings.Contains(lower, "maya")) && (strings.Contains(lower, "sister") || strings.Contains(lower, "nora"))
		checks["one_focused_question_marker"] = strings.Count(answer, "?") == 1
		checks["gift_not_guessed_before_disambiguation"] = !strings.Contains(lower, "jasmine") && !strings.Contains(lower, "ceramic")
	case "no_evidence":
		checks["no_invented_identity_or_preference"] = !strings.Contains(lower, "maya") && !strings.Contains(lower, "nora") && !strings.Contains(lower, "jasmine") && !strings.Contains(lower, "ceramic")
		checks["honest_insufficiency_or_clarification"] = strings.Contains(answer, "?") || strings.Contains(lower, "don't know") || strings.Contains(lower, "not enough") || strings.Contains(lower, "need more")
	default:
		if test.name == "opaque_alias" {
			checks["accepted_alias_not_left_unresolved"] = !strings.Contains(lower, "couldn’t confirm") && !strings.Contains(lower, "could not confirm") && !strings.Contains(lower, "couldn't confirm") && !strings.Contains(lower, "if mom-27") && !strings.Contains(lower, "if you mean")
		}
		checks["supported_gift_named"] = strings.Contains(lower, "jasmine") && strings.Contains(lower, "tea")
		checks["other_gift_not_substituted"] = !strings.Contains(lower, "ceramic")
	}
	return checks
}
