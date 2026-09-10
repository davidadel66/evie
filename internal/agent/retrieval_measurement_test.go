package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

type retrievalSliceSamples struct {
	Unit    string  `json:"unit"`
	Samples []int64 `json:"samples"`
	P50     int64   `json:"p50"`
	P95     int64   `json:"p95"`
	Max     int64   `json:"max"`
}

func summarizeRetrievalSlice(unit string, values []int64) retrievalSliceSamples {
	ordered := append([]int64(nil), values...)
	slices.Sort(ordered)
	return retrievalSliceSamples{Unit: unit, Samples: values,
		P50: ordered[(50*len(ordered)+99)/100-1], P95: ordered[(95*len(ordered)+99)/100-1], Max: ordered[len(ordered)-1]}
}

// This development-only fixture measures the complete turn boundary with a
// scripted provider. It is neither learned-model quality nor held-out evidence.
func TestMemoryRetrievalSliceMeasurements(t *testing.T) {
	fixture := struct {
		Version     string   `json:"version"`
		Query       string   `json:"query"`
		UserMessage string   `json:"user_message"`
		Turns       int      `json:"turns"`
		Relevant    []string `json:"relevant"`
		Distractors []string `json:"distractors"`
	}{
		Version: "memory-stage5-accepted-slice-v1", Query: "saffron", UserMessage: "Look up my saved dining preferences.", Turns: 30,
		Relevant: []string{
			"saffron dining preference: vegetarian main courses",
			"saffron dining preference: mild seasoning",
			"saffron dining preference: unsweetened iced tea",
			"saffron dining preference: quiet tables",
			"saffron dining preference: early evening reservations",
		},
	}
	for i := 1; i <= 40; i++ {
		fixture.Distractors = append(fixture.Distractors, fmt.Sprintf("library shelf %02d holds the blue gardening notebook", i))
	}
	fixtureBytes, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	f := newRetrievalFixture(t)
	ctx := context.Background()
	owner := f.global()
	for _, value := range fixture.Distractors {
		f.remember(owner, memory.MemoryEverywhere, value)
	}
	expectedIDs := make([]memory.SemanticID, 0, len(fixture.Relevant))
	for _, value := range fixture.Relevant {
		expectedIDs = append(expectedIDs, f.remember(owner, memory.MemoryEverywhere, value).ClaimID)
	}
	before, err := f.store.InspectClaims(ctx, owner.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	coverageBefore, err := f.store.MemoryIndexCoverage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	buildStarted := time.Now()
	coverageAfter, err := f.store.RefreshMemoryIndex(ctx, 256)
	buildNanos := time.Since(buildStarted).Nanoseconds()
	if err != nil || coverageAfter.State != "active" || coverageAfter.Pending != 0 {
		t.Fatalf("fixture index did not become available: coverage=%+v error=%v", coverageAfter, err)
	}
	if len(before.Claims) != 45 || coverageBefore.Pending != 45 {
		t.Fatalf("fixture must contain 45 accepted pending Claims: accepted=%d coverage=%+v", len(before.Claims), coverageBefore)
	}

	var turnNanos, memoryBytes, requestBytes, requestBytesPerTurn, memoryWireOverhead []int64
	matchedEvidence := 0
	for turn := 0; turn < fixture.Turns; turn++ {
		reader := f.global()
		args, _ := json.Marshal(map[string]string{"query": fixture.Query})
		client := &fakeClient{steps: []step{
			assistantStep("", nil, toolCall("lookup-dining", "memory_search", string(args))),
			assistantStep("Evidence received.", nil),
		}}
		session := f.session(reader, client)
		started := time.Now()
		err := session.Send(ctx, fixture.UserMessage, &recorder{}, nil)
		turnNanos = append(turnNanos, time.Since(started).Nanoseconds())
		if err != nil {
			t.Fatalf("turn %d: %v", turn, err)
		}
		if len(client.reqs) != 2 {
			t.Fatalf("turn %d dispatched %d requests, want 2", turn, len(client.reqs))
		}
		var supplied struct {
			Status   string                     `json:"status"`
			Evidence []memory.RetrievalEvidence `json:"evidence"`
		}
		data := retrievalData(t, client.reqs[1])
		if err := json.Unmarshal([]byte(strings.TrimPrefix(data, "EVIE_MEMORY_DATA\n")), &supplied); err != nil {
			t.Fatal(err)
		}
		if supplied.Status != memory.RetrievalSuccess || len(supplied.Evidence) != 5 {
			t.Fatalf("turn %d supplied status=%s evidence=%d, want success with five relevant Claims", turn, supplied.Status, len(supplied.Evidence))
		}
		seen := make(map[memory.SemanticID]bool)
		for _, evidence := range supplied.Evidence {
			if evidence.Kind != memory.RetrievalAcceptedMemory || !slices.Contains(expectedIDs, evidence.ClaimID) || seen[evidence.ClaimID] {
				t.Fatalf("turn %d supplied unexpected or duplicate Claim %s", turn, evidence.ClaimID)
			}
			seen[evidence.ClaimID] = true
			matchedEvidence++
		}
		var totalBytes int64
		for _, request := range client.reqs {
			encoded, err := openrouter.RequestBytes(request)
			if err != nil {
				t.Fatal(err)
			}
			requestBytes = append(requestBytes, int64(len(encoded)))
			totalBytes += int64(len(encoded))
			withoutMemory := request
			withoutMemory.Messages = nil
			for _, message := range request.Messages {
				if message.Role == "user" && strings.HasPrefix(message.Content, "EVIE_MEMORY_DATA\n") {
					serialized, err := json.Marshal(message)
					if err != nil {
						t.Fatal(err)
					}
					memoryBytes = append(memoryBytes, int64(len(serialized)))
					continue
				}
				withoutMemory.Messages = append(withoutMemory.Messages, message)
			}
			stripped, err := openrouter.RequestBytes(withoutMemory)
			if err != nil {
				t.Fatal(err)
			}
			memoryWireOverhead = append(memoryWireOverhead, int64(len(encoded)-len(stripped)))
		}
		requestBytesPerTurn = append(requestBytesPerTurn, totalBytes)
	}
	var idleRefreshNanos []int64
	for i := 0; i < 30; i++ {
		started := time.Now()
		coverage, err := f.store.RefreshMemoryIndex(ctx, 256)
		idleRefreshNanos = append(idleRefreshNanos, time.Since(started).Nanoseconds())
		if err != nil || coverage.State != "active" || coverage.Pending != 0 {
			t.Fatalf("idle refresh changed complete coverage: %+v error=%v", coverage, err)
		}
	}
	after, err := f.store.InspectClaims(ctx, owner.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Claims) != len(before.Claims) || !reflect.DeepEqual(after.ScopeRevisions, before.ScopeRevisions) {
		t.Fatal("measurement turns changed accepted semantic state")
	}
	report := struct {
		Version              string                   `json:"version"`
		Fixture              any                      `json:"fixture"`
		FixtureSHA256        string                   `json:"fixture_sha256"`
		ObservedAt           string                   `json:"observed_at"`
		Runtime              map[string]any           `json:"runtime"`
		Configuration        map[string]any           `json:"configuration"`
		ExpectedClaimIDs     []memory.SemanticID      `json:"expected_claim_ids"`
		EvidenceMatched      int                      `json:"evidence_matched"`
		EvidenceExpected     int                      `json:"evidence_expected"`
		CoverageBefore       memory.RetrievalCoverage `json:"coverage_before"`
		CoverageAfter        memory.RetrievalCoverage `json:"coverage_after"`
		IndexBuildNanos      int64                    `json:"initial_index_build_ns"`
		WholeTurn            retrievalSliceSamples    `json:"whole_turn"`
		IdleIndexRefresh     retrievalSliceSamples    `json:"idle_index_refresh"`
		MemoryMessage        retrievalSliceSamples    `json:"memory_message_json"`
		CompleteRequest      retrievalSliceSamples    `json:"complete_provider_request"`
		CompleteTurnRequests retrievalSliceSamples    `json:"complete_turn_provider_requests"`
		MemoryWireOverhead   retrievalSliceSamples    `json:"memory_provider_request_delta"`
		SearchLatency        *retrievalSliceSamples   `json:"isolated_search_latency"`
		ModelQualityAssessed bool                     `json:"model_quality_assessed"`
		Limitations          []string                 `json:"limitations"`
	}{
		Version: "memory-stage5-slice-measurements-v1", Fixture: fixture, FixtureSHA256: memory.CompilerHash(fixtureBytes),
		ObservedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Runtime:    map[string]any{"go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "logical_cpus": runtime.NumCPU()},
		Configuration: map[string]any{
			"retrieval_version": retrievalVersion, "index_generation": coverageAfter.Generation, "searches_per_turn": retrievalSearchLimit,
			"results_per_search": retrievalResultLimit, "result_bytes_per_search": retrievalResultBytes, "serialized_bytes_per_turn": retrievalTurnBytes,
			"search_deadline_ns": retrievalSearchDeadline.Nanoseconds(), "cumulative_work_ns": retrievalTurnWork.Nanoseconds(),
			"index_batch_rows": 256, "reader": "scripted_provider", "requests_per_turn": 2, "percentile": "nearest_rank",
		},
		ExpectedClaimIDs: expectedIDs, EvidenceMatched: matchedEvidence, EvidenceExpected: fixture.Turns * len(fixture.Relevant),
		CoverageBefore: coverageBefore, CoverageAfter: coverageAfter, IndexBuildNanos: buildNanos,
		WholeTurn: summarizeRetrievalSlice("ns", turnNanos), IdleIndexRefresh: summarizeRetrievalSlice("ns", idleRefreshNanos),
		MemoryMessage: summarizeRetrievalSlice("bytes", memoryBytes), CompleteRequest: summarizeRetrievalSlice("bytes", requestBytes),
		CompleteTurnRequests: summarizeRetrievalSlice("bytes", requestBytesPerTurn), MemoryWireOverhead: summarizeRetrievalSlice("bytes", memoryWireOverhead),
		Limitations: []string{
			"Development lexical fixture only: 45 accepted Claims, 30 fresh sessions, one lookup per turn; no model, network, embedding, or held-out evaluation.",
			"Whole-turn timing brackets Session.Send, including durable history, tool execution, revalidation, and request composition; it excludes session creation and fixture setup.",
			"Isolated search latency is unavailable at the measured complete-turn seam; whole-turn latency is not reported as search latency.",
			"Memory message JSON bytes include escaping and wrapper; provider request delta excludes only that synthetic message and does not include count/status-only memory tool overhead.",
			"Initial index build has one sample; idle refresh has 30 samples. No peak memory or production throughput claim is made.",
			"These observations inform conservative slice settings and are not final release thresholds or answer-quality measurements.",
		},
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("MEMORY_RETRIEVAL_SLICE_REPORT=%s", encoded)
}
