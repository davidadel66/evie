package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

type integratedConfiguration struct {
	Conditions  []string `json:"conditions"`
	Repetitions int      `json:"reader_repetitions_per_case_condition"`
	ReaderCases int      `json:"reader_cases"`
	ReaderTurns int      `json:"reader_turns"`
	Resources   struct {
		WorkingTokens       int64 `json:"working_tokens"`
		OutputReserveTokens int64 `json:"output_reserve_tokens"`
		ReaderCalls         int   `json:"reader_calls_per_turn_max"`
		ReaderDeadlineMS    int64 `json:"reader_turn_deadline_ms"`
		LocalRepetitions    int   `json:"local_repetitions_per_case_condition"`
	} `json:"resources"`
}

type integratedFreeze struct {
	Partition           string                               `json:"partition"`
	WorkloadPath        string                               `json:"workload_path"`
	WorkloadSHA256      string                               `json:"workload_sha256"`
	GatesPath           string                               `json:"gates_path"`
	GatesSHA256         string                               `json:"gates_sha256"`
	RubricPath          string                               `json:"rubric_path"`
	RubricSHA256        string                               `json:"rubric_sha256"`
	InputsPath          string                               `json:"inputs_path"`
	InputManifestSHA256 string                               `json:"input_manifest_sha256"`
	BinarySHA256        string                               `json:"binary_sha256"`
	Endpoint            string                               `json:"endpoint"`
	Profile             openrouter.ContextProfileDiagnostics `json:"profile"`
	Configuration       integratedConfiguration              `json:"configuration"`
}

func integratedFrozenInputs(t *testing.T) (integratedFreeze, integratedWorkload, string) {
	t.Helper()
	path := os.Getenv("EVIE_MEMORY_INTEGRATED_FREEZE")
	output := os.Getenv("EVIE_MEMORY_READER_ARTIFACTS")
	inputs := os.Getenv("EVIE_MEMORY_INTEGRATED_INPUTS")
	if !filepath.IsAbs(path) || !filepath.IsAbs(output) || !filepath.IsAbs(inputs) {
		t.Fatal("integrated evaluation requires absolute immutable freeze, input and output paths")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var freeze integratedFreeze
	if err := json.Unmarshal(raw, &freeze); err != nil {
		t.Fatal(err)
	}
	if (freeze.Partition != "development" && freeze.Partition != "heldout") || freeze.InputsPath != inputs {
		t.Fatal("only the declared frozen partition and canonical inputs may be evaluated")
	}
	if freeze.Endpoint == "" || os.Getenv("EVIE_MEMORY_EMBEDDING_ENDPOINT") != freeze.Endpoint {
		t.Fatal("selected embedding endpoint differs from freeze")
	}
	configuration := freeze.Configuration
	if !reflect.DeepEqual(configuration.Conditions, integratedConditions) || configuration.Repetitions != 1 || configuration.ReaderCases != 24 || configuration.ReaderTurns != 144 || configuration.Resources.WorkingTokens != 24576 || configuration.Resources.OutputReserveTokens != 768 || configuration.Resources.ReaderCalls != 6 || configuration.Resources.ReaderDeadlineMS != 120000 {
		t.Fatal("integrated harness and frozen procedure disagree")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string]string{executable: freeze.BinarySHA256, freeze.WorkloadPath: freeze.WorkloadSHA256, freeze.GatesPath: freeze.GatesSHA256, freeze.RubricPath: freeze.RubricSHA256, filepath.Join(filepath.Dir(path), "input-manifest.json"): freeze.InputManifestSHA256}
	for file, want := range checks {
		contents, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if want == "" || memory.CompilerHash(contents) != want {
			t.Fatalf("frozen artifact differs: %s", filepath.Base(file))
		}
	}
	manifestBytes, err := os.ReadFile(filepath.Join(filepath.Dir(path), "input-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]string
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest) != 48 {
		t.Fatal("expected exactly a closed seed database and source map for every development case")
	}
	for relative, want := range manifest {
		if filepath.IsAbs(relative) || !filepath.IsLocal(relative) {
			t.Fatal("input manifest path escapes frozen input directory")
		}
		contents, err := os.ReadFile(filepath.Join(inputs, relative))
		if err != nil {
			t.Fatal(err)
		}
		if memory.CompilerHash(contents) != want {
			t.Fatalf("canonical input changed: %s", relative)
		}
	}
	workload := integratedLoadWorkload(t, freeze.WorkloadPath)
	if workload.Partition != freeze.Partition {
		t.Fatal("workload partition differs from immutable freeze")
	}
	return freeze, workload, output
}

func integratedLoadSeed(t *testing.T, inputs string, test integratedCase) integratedSeed {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(inputs, test.ID, "seed.json"))
	if err != nil {
		t.Fatal(err)
	}
	var seed integratedSeed
	if err := json.Unmarshal(raw, &seed); err != nil {
		t.Fatal(err)
	}
	if seed.Version != 1 || !reflect.DeepEqual(seed.Case, test) || len(seed.Bindings) != len(test.Records) {
		t.Fatal("canonical case/source map differs from frozen workload")
	}
	return seed
}

func TestMemoryStage5IntegratedReaderEvaluation(t *testing.T) {
	if os.Getenv("EVIE_MEMORY_INTEGRATED_FREEZE") == "" {
		t.Skip("actual integrated reader evaluation requires an immutable freeze")
	}
	freeze, workload, directory := integratedFrozenInputs(t)
	provider, profile, capture := productionReaderSetup(t, "integrated-evaluation")
	if profile.Diagnostics() != freeze.Profile {
		t.Fatal("actual production reader identity/profile changed after freeze")
	}
	for index, test := range workload.Cases {
		seed := integratedLoadSeed(t, freeze.InputsPath, test)
		for offset := range integratedConditions {
			condition := integratedConditions[(index+offset)%len(integratedConditions)]
			t.Run(test.ID+"/"+condition, func(t *testing.T) {
				t.Setenv("EVIE_REMOTE_MEMORY", "on")
				f := integratedCloneSeed(t, filepath.Join(freeze.InputsPath, test.ID), seed)
				client := &integratedClient{t: t, f: f, seed: seed, condition: condition, provider: provider, capture: capture, directory: directory}
				session, history, reader := integratedSession(t, f, seed, condition, client, profile)
				client.reader = reader
				before, err := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
				if err != nil {
					t.Fatal(err)
				}
				if test.MemoryMode == "unavailable" {
					t.Setenv("EVIE_REMOTE_MEMORY", "off")
				}
				ctx, cancel := context.WithTimeout(context.Background(), time.Duration(freeze.Configuration.Resources.ReaderDeadlineMS)*time.Millisecond)
				defer cancel()
				client.started = time.Now()
				err = session.Send(ctx, seed.Question, &recorder{}, nil)
				elapsed := time.Since(client.started).Nanoseconds()
				after, inspectErr := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
				unchanged := inspectErr == nil && reflect.DeepEqual(before.ScopeRevisions, after.ScopeRevisions)
				events, eventErr := f.store.LoadEvents(context.Background(), reader.ID)
				var final string
				if len(client.responses) > 0 {
					last := client.responses[len(client.responses)-1]
					if len(last.Choices) > 0 {
						final = last.Choices[0].Message.Content
					}
				}
				stem := test.ID + "-" + condition
				productionReaderWrite(t, directory, stem+"-case.json", map[string]any{
					"case_id": test.ID, "family": test.Family, "condition": condition, "repetition": 1, "rendered_question": seed.Question,
					"reader_session": reader, "whole_turn_elapsed_ns": elapsed, "model_calls": len(client.responses), "dispatch_count": len(client.dispatches),
					"model_elapsed_ns": client.modelElapsedNS, "kernel_searches": len(history.calls), "kernel_calls": history.calls,
					"last_runtime_accounting": client.lastAccounting(),
					"final_answer":            final, "error": fmt.Sprint(err), "semantic_revisions_unchanged": unchanged, "scope_revisions_before": before.ScopeRevisions, "scope_revisions_after": after.ScopeRevisions,
					"oracle_exact_selector": condition == "oracle", "scripted_initial_search": false, "model_controls_from_first_request": true, "scripted_compactor_quality_evaluated": false,
				})
				productionReaderWrite(t, directory, stem+"-events.json", events)
				if err != nil {
					t.Errorf("reader turn failed; complete available artifacts retained: %v", err)
				}
				if !unchanged || eventErr != nil {
					t.Errorf("reader persistence verification failed: %v %v", inspectErr, eventErr)
				}
			})
		}
	}
}
