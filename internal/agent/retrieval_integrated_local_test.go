package agent

import (
	"compress/gzip"
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

const integratedLocalScript = "v1: only tool_only and automatic_deeper request memory_search and memory_search_conversations concurrently on the original rendered question prefix through at most 32 Unicode letter/digit runs and 1024 UTF-8 bytes, preserving intervening punctuation and IDs, with current intent; on dispatch two, expand the first delivered conversation excerpt by one message before and after if present; then finish. All other conditions finish on their first dispatch. No gold-directed query, temporal override or answer-quality claim."

func integratedLocalResponse(condition, question string, d integratedDispatch) openrouter.ChatResponse {
	if condition == "tool_only" || condition == "automatic_deeper" {
		if d.Call == 1 {
			args, _ := json.Marshal(map[string]string{"query": integratedProbeQuery(question), "intent": "current"})
			return assistantStep("", nil, toolCall("local-accepted", "memory_search", string(args)), toolCall("local-conversation", "memory_search_conversations", string(args))).res
		}
		if d.Call == 2 {
			for _, e := range d.Evidence {
				if e.Kind == memory.RetrievalConversationExcerpt {
					args, _ := json.Marshal(map[string]any{"evidence_id": e.ID, "before": 1, "after": 1})
					return assistantStep("", nil, toolCall("local-neighbor", "memory_expand_conversation", string(args))).res
				}
			}
		}
	}
	return assistantStep("Scripted local retrieval/resource probe completed; no reader-quality claim.", nil).res
}

func TestMemoryStage5IntegratedLocalEvaluation(t *testing.T) {
	if os.Getenv("EVIE_MEMORY_INTEGRATED_FREEZE") == "" {
		t.Skip("local integrated measurement requires an immutable freeze")
	}
	freeze, workload, directory := integratedFrozenInputs(t)
	if freeze.Configuration.Resources.LocalRepetitions != 20 {
		t.Fatal("local fixture requires the frozen twenty-repetition procedure")
	}
	// Metadata discovery happens before every timed sample. It is a real
	// production profile lookup, not a guessed profile or a reader generation.
	_, profile, _ := productionReaderSetup(t, "integrated-local")
	if profile.Diagnostics() != freeze.Profile {
		t.Fatal("production context profile changed after freeze")
	}
	productionReaderWrite(t, directory, "local-procedure.json", map[string]any{"script": integratedLocalScript, "reader_model_calls": 0, "metadata_discovery_outside_timing": true, "fresh_canonical_clone_per_sample": true, "index_build_outside_timing": true, "no_additional_model_warmup": true, "first_repetition": "first observed sample for this case/condition; not a guaranteed cold model", "later_repetitions": "same selected endpoint, no forced unload; real warm/cold latency retained", "runtime_accounting_policy": "Final runtime work requires a successful turn and an investigation receipt on the final dispatch, or a successful turn with zero observed Kernel searches. Failed-turn final work is unknown; the last observed receipt is retained without treating it as final. Any unknown total makes the twenty-sample runtime quantile unavailable.", "summed_observed_kernel_search_measurement": "Lower bound from actual delegated SearchMemory calls; excludes preparation and revalidation outside those calls."})
	for index, test := range workload.Cases {
		seed := integratedLoadSeed(t, freeze.InputsPath, test)
		for offset := range integratedConditions {
			condition := integratedConditions[(index+offset)%len(integratedConditions)]
			t.Run(test.ID+"/"+condition, func(t *testing.T) {
				stem := test.ID + "-" + condition
				file, err := os.OpenFile(filepath.Join(directory, stem+"-local-traces.jsonl.gz"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
				if err != nil {
					t.Fatal(err)
				}
				compressed := gzip.NewWriter(file)
				encoder := json.NewEncoder(compressed)
				var first, whole, work, accountedWork []int64
				failures := 0
				for repetition := 1; repetition <= 20; repetition++ {
					t.Run(fmt.Sprintf("%02d", repetition), func(t *testing.T) {
						t.Setenv("EVIE_REMOTE_MEMORY", "on")
						f := integratedCloneSeed(t, filepath.Join(freeze.InputsPath, test.ID), seed)
						client := &integratedClient{t: t, f: f, seed: seed, condition: condition, scripted: func(d integratedDispatch) openrouter.ChatResponse {
							return integratedLocalResponse(condition, seed.Question, d)
						}}
						session, history, reader := integratedSession(t, f, seed, condition, client, profile)
						client.reader = reader
						before, err := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
						if err != nil {
							t.Fatal(err)
						}
						if test.MemoryMode == "unavailable" {
							t.Setenv("EVIE_REMOTE_MEMORY", "off")
						}
						ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
						defer cancel()
						client.started = time.Now()
						err = session.Send(ctx, seed.Question, &recorder{}, nil)
						elapsed := time.Since(client.started).Nanoseconds()
						after, inspectErr := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
						unchanged := inspectErr == nil && reflect.DeepEqual(before.ScopeRevisions, after.ScopeRevisions)
						var firstNS, kernelNS int64
						if len(client.dispatches) > 0 {
							firstNS = client.dispatches[0].ElapsedToDispatchNS
						}
						for _, call := range history.calls {
							kernelNS += call.ElapsedNS
						}
						whole = append(whole, elapsed)
						work = append(work, kernelNS)
						// Receipts describe work observed before their dispatch. A
						// failed turn may do more work before composition fails, so
						// its last receipt cannot establish the final runtime total.
						var accountedNS *int64
						accountingBasis := "failed_turn_final_work_unobserved"
						if err == nil {
							accountingBasis = "successful_turn_without_final_receipt"
							var finalAccounting *memory.RetrievalInvestigation
							if count := len(client.dispatches); count > 0 {
								if receipt := client.dispatches[count-1].Snapshot.Memory; receipt != nil {
									finalAccounting = receipt.Investigation
								}
							}
							if finalAccounting != nil {
								value := finalAccounting.KernelWorkNanoseconds
								accountedNS = &value
								accountingBasis = "successful_final_dispatch_receipt"
							} else if len(history.calls) == 0 && len(client.dispatches) > 0 {
								value := int64(0)
								accountedNS = &value
								accountingBasis = "successful_turn_without_kernel_searches"
							}
						}
						if accountedNS != nil {
							accountedWork = append(accountedWork, *accountedNS)
						}
						first = append(first, firstNS)
						result := map[string]any{"case_id": test.ID, "condition": condition, "repetition": repetition, "whole_turn_elapsed_ns": elapsed, "first_dispatch_elapsed_ns": firstNS, "summed_observed_kernel_search_ns": kernelNS, "runtime_kernel_work_ns": accountedNS, "runtime_accounting_available": accountedNS != nil, "runtime_accounting_basis": accountingBasis, "last_runtime_accounting": client.lastAccounting(), "kernel_searches": len(history.calls), "kernel_calls": history.calls, "dispatches": client.dispatches, "encoded_requests": client.encodedRequests, "error": fmt.Sprint(err), "semantic_revisions_unchanged": unchanged, "reader_model_calls": 0, "script": integratedLocalScript}
						if writeErr := encoder.Encode(result); writeErr != nil {
							t.Fatal(writeErr)
						}
						if err != nil || !unchanged || len(client.dispatches) == 0 {
							failures++
							t.Errorf("local sample failed with retained trace: %v %v", err, inspectErr)
						}
					})
				}
				if err := compressed.Close(); err != nil {
					t.Fatal(err)
				}
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
				var runtimeSummary *retrievalSliceSamples
				if len(accountedWork) == 20 {
					summary := summarizeRetrievalSlice("nanoseconds", accountedWork)
					runtimeSummary = &summary
				}
				productionReaderWrite(t, directory, stem+"-local-summary.json", map[string]any{"case_id": test.ID, "condition": condition, "samples": 20, "failures": failures, "first_dispatch": summarizeRetrievalSlice("nanoseconds", first), "whole_turn": summarizeRetrievalSlice("nanoseconds", whole), "summed_observed_kernel_search": summarizeRetrievalSlice("nanoseconds", work), "runtime_kernel_work": runtimeSummary, "runtime_accounting_complete": runtimeSummary != nil, "runtime_accounting_known_samples": len(accountedWork), "runtime_accounting_unknown_samples": 20 - len(accountedWork), "all_attempts_retained": true, "script": integratedLocalScript})
			})
		}
	}
}
