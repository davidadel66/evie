package agent

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
)

type denseExperimentRecord struct{ ID, Scope, Kind, Text string }
type denseExperimentCase struct {
	ID, Family, Scope, Kind, Category, Query string
	Expected, Forbidden                      []string
}

type denseExperimentObservation struct {
	Path            string   `json:"path"`
	InputSHA256     []string `json:"input_sha256,omitempty"`
	InputBytes      []int    `json:"input_bytes,omitempty"`
	SecretInputs    int      `json:"secret_inputs"`
	HTTPStatus      int      `json:"http_status"`
	ElapsedNS       int64    `json:"elapsed_ns"`
	TransportFailed bool     `json:"transport_failed"`
}

type denseExperimentObserver struct {
	server *httptest.Server
	mu     sync.Mutex
	rows   []denseExperimentObservation
}

// This observer forwards original bytes to the actual selected local model.
// It records content-free input hashes/counts, never supplies fixture vectors.
func observeDenseExperiment(t *testing.T, endpoint string) *denseExperimentObserver {
	t.Helper()
	if endpoint != "http://127.0.0.1:11565" {
		t.Fatal("experiment requires the declared owned literal loopback endpoint")
	}
	transport := &http.Transport{Proxy: nil, DisableCompression: true, MaxConnsPerHost: 1, MaxIdleConns: 1}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return fmt.Errorf("experiment observation never follows redirects")
	}}
	t.Cleanup(transport.CloseIdleConnections)
	observer := &denseExperimentObserver{}
	observer.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		row := denseExperimentObservation{Path: r.URL.Path}
		defer func() {
			row.ElapsedNS = time.Since(started).Nanoseconds()
			observer.mu.Lock()
			observer.rows = append(observer.rows, row)
			observer.mu.Unlock()
		}()
		if r.URL.Path != "/api/tags" && r.URL.Path != "/api/embed" {
			row.HTTPStatus = http.StatusNotFound
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			row.HTTPStatus = http.StatusBadRequest
			http.Error(w, "invalid observation request", row.HTTPStatus)
			return
		}
		if r.URL.Path == "/api/embed" {
			var data struct {
				Input []string `json:"input"`
			}
			if err = json.Unmarshal(body, &data); err != nil {
				t.Error("production embedding input was not valid JSON")
			}
			for _, input := range data.Input {
				row.InputSHA256 = append(row.InputSHA256, memory.CompilerHash([]byte(input)))
				row.InputBytes = append(row.InputBytes, len(input))
				if memory.HasRetrievalSecret([]byte(input)) {
					row.SecretInputs++
				}
			}
		}
		forward, err := http.NewRequestWithContext(r.Context(), r.Method, endpoint+r.URL.Path, strings.NewReader(string(body)))
		if err != nil {
			t.Error(err)
			return
		}
		forward.Header.Set("Content-Type", r.Header.Get("Content-Type"))
		response, err := client.Do(forward)
		if err != nil {
			row.TransportFailed = true
			row.HTTPStatus = http.StatusBadGateway
			http.Error(w, "local model unavailable", row.HTTPStatus)
			return
		}
		defer response.Body.Close()
		row.HTTPStatus = response.StatusCode
		w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
		w.WriteHeader(response.StatusCode)
		_, _ = io.Copy(w, response.Body)
	}))
	t.Cleanup(observer.server.Close)
	return observer
}

func denseExperimentWrite(t *testing.T, path string, value any) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(value)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("write immutable experiment artifact: %v %v", err, closeErr)
	}
}

func denseExperimentScope(record memory.Session) string {
	if record.WorkspaceID != "" {
		return "workspace:" + string(record.WorkspaceID)
	}
	if record.ProjectID != "" {
		return "project:" + string(record.ProjectID)
	}
	return "global"
}

// Opt-in actual MiniLM integration regression. #165's previously evaluated
// partitions are reused; this is not an untouched #167/#168 release evaluation.
func TestMemoryStage5DenseExperiment(t *testing.T) {
	freezePath := os.Getenv("EVIE_DENSE_EXPERIMENT_FREEZE")
	if freezePath == "" {
		t.Skip("requires an immutable actual-model experiment freeze")
	}
	partition := os.Getenv("EVIE_DENSE_EXPERIMENT_PARTITION")
	if partition != "development" && partition != "heldout" {
		t.Fatal("select the reused #165 development or heldout partition explicitly")
	}
	if retrievalResultLimit != 8 || retrievalResultBytes != 12<<10 || retrievalTurnBytes != 36<<10 {
		t.Fatal("production retrieval caps differ from the declared experiment freeze")
	}
	output := os.Getenv("EVIE_DENSE_EXPERIMENT_OUTPUT")
	if err := os.Mkdir(output, 0700); err != nil {
		t.Fatal("require a fresh experiment partition output directory: ", err)
	}
	freezeBytes, err := os.ReadFile(freezePath)
	if err != nil {
		t.Fatal(err)
	}
	var freeze struct {
		BinarySHA256 string            `json:"binary_sha256"`
		FixtureRoot  string            `json:"fixture_root"`
		Fixtures     map[string]string `json:"fixture_sha256"`
		Endpoint     string            `json:"endpoint"`
	}
	if err := json.Unmarshal(freezeBytes, &freeze); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(executable)
	if err != nil || memory.CompilerHash(binary) != freeze.BinarySHA256 {
		t.Fatal("compiled experiment executable differs from the freeze")
	}
	for name, want := range freeze.Fixtures {
		data, err := os.ReadFile(filepath.Join(freeze.FixtureRoot, name))
		if err != nil || memory.CompilerHash(data) != want {
			t.Fatal("fixture changed after freeze: ", name)
		}
	}
	corpusBytes, err := os.ReadFile(filepath.Join(freeze.FixtureRoot, "corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus []denseExperimentRecord
	if err = json.Unmarshal(corpusBytes, &corpus); err != nil || len(corpus) != 389 {
		t.Fatal("expected the unchanged 389-record #165 corpus")
	}
	observer := observeDenseExperiment(t, freeze.Endpoint)
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", "")
	f := newRetrievalFixture(t)
	actors := denseScopeActors(t, f)
	keys := []string{"global", "general", "workspace", "project_a", "project_b"}
	sources := make(map[string]memory.Session)
	for i, actor := range actors {
		sources[keys[i]] = actor.current
	}
	byClaim := make(map[memory.SemanticID]string)
	byEvent := make(map[memory.EventID]string)
	mapping := make(map[string]any)
	setupStarted := time.Now()
	for _, record := range corpus {
		source, ok := sources[record.Scope]
		if !ok || record.ID == "" {
			t.Fatal("invalid frozen corpus scope or identity")
		}
		if record.Kind == "claim" {
			accepted := f.remember(source, "", record.Text)
			byClaim[accepted.ClaimID] = record.ID + "/claim"
			byEvent[accepted.Source.EventID] = record.ID + "/excerpt"
			mapping[record.ID] = map[string]any{"claim_id": accepted.ClaimID, "source_event_id": accepted.Source.EventID, "source_link_id": accepted.SourceLinkID, "scope": record.Scope, "original_corpus_text_sha256": memory.CompilerHash([]byte(record.Text)), "accepted_source_text_sha256": memory.CompilerHash([]byte(accepted.Source.Evidence))}
		} else if record.Kind == "conversation" {
			event := f.converse(source, record.Text)
			byEvent[event.ID] = record.ID + "/excerpt"
			mapping[record.ID] = map[string]any{"source_event_id": event.ID, "scope": record.Scope, "source_text_sha256": memory.CompilerHash([]byte(event.Content))}
		} else {
			t.Fatal("invalid frozen corpus evidence kind")
		}
	}
	for key, source := range sources {
		events, err := f.store.LoadEvents(context.Background(), source.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			if (event.Type == memory.EventUserMessage || event.Type == memory.EventAssistantMessage) && byEvent[event.ID] == "" {
				byEvent[event.ID] = fmt.Sprintf("aux/%s/%d", key, event.Sequence)
			}
		}
	}
	setupNS := time.Since(setupStarted).Nanoseconds()
	denseExperimentWrite(t, filepath.Join(output, "source-mapping.json"), mapping)
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", observer.server.URL)
	buildStarted := time.Now()
	var batches []map[string]any
	var coverage memory.RetrievalCoverage
	for i := 0; i < 10000; i++ {
		started := time.Now()
		coverage, err = f.store.RefreshMemoryIndex(context.Background(), 256)
		batches = append(batches, map[string]any{"elapsed_ns": time.Since(started).Nanoseconds(), "coverage": coverage, "failed": err != nil})
		if err != nil {
			denseExperimentWrite(t, filepath.Join(output, "build-failure.json"), batches)
			t.Fatal("actual model maintenance failed: ", err)
		}
		if coverage.State == "active" && coverage.Pending == 0 {
			break
		}
	}
	buildNS := time.Since(buildStarted).Nanoseconds()
	denseExperimentWrite(t, filepath.Join(output, "build.json"), map[string]any{"fixture_setup_ns": setupNS, "maintenance_ns": buildNS, "batch_limit": 256, "batches": batches, "final_coverage": coverage})
	if coverage.State != "active" || coverage.Pending != 0 {
		t.Fatal("retained corpus never reached active reconciled coverage")
	}
	// Question bytes were hashed earlier, but questions are decoded only after
	// the entire source corpus has been committed and indexed successfully.
	questions, err := os.ReadFile(filepath.Join(freeze.FixtureRoot, partition+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []denseExperimentCase
	if err = json.Unmarshal(questions, &cases); err != nil || len(cases) != 32 {
		t.Fatal("expected the unchanged 32-question #165 partition")
	}
	traceFile, err := os.OpenFile(filepath.Join(output, "traces.ndjson.gz"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	compressed := gzip.NewWriter(traceFile)
	traceWriter := json.NewEncoder(compressed)
	defer func() { _ = compressed.Close(); _ = traceFile.Close() }()
	var rows []map[string]any
	defer func() {
		observer.server.Close()
		observer.mu.Lock()
		observations := append([]denseExperimentObservation(nil), observer.rows...)
		observer.mu.Unlock()
		denseExperimentWrite(t, filepath.Join(output, "endpoint-observations.json"), observations)
		denseExperimentWrite(t, filepath.Join(output, "samples.json"), rows)
	}()
	resolved, err := standardManager(t, f.store).ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	freshReader := func(key string) memory.Session {
		source := sources[key]
		var record memory.Session
		var err error
		switch {
		case source.WorkspaceID != "":
			record, err = f.store.CreateWorkspaceSessionWithComposition(context.Background(), source.WorkspaceID, source.WorkspaceRevisionSnapshot, resolved.Receipt)
		case source.ProjectID != "":
			record, err = f.store.CreateProjectSession(context.Background(), source.ProjectID)
		default:
			record, err = f.store.CreateGlobalSession(context.Background())
		}
		if err != nil {
			t.Fatal(err)
		}
		return record
	}
	latencies := map[string][]int64{}
	matched := map[string]int{}
	expected := map[string]int{}
	paraphraseMatched := map[string]int{}
	paraphraseExpected := map[string]int{}
	forbiddenCount, secretEvidence, capViolations, unknownEvidence := 0, 0, 0, 0
	for repetition := 0; repetition < 3; repetition++ {
		for index, test := range cases {
			methods := []string{"lexical", "hybrid"}
			if (index+repetition)%2 == 1 {
				slices.Reverse(methods)
			}
			for _, method := range methods {
				if method == "hybrid" {
					t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", observer.server.URL)
				} else {
					t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", "")
				}
				reader := freshReader(test.Scope)
				before, err := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
				if err != nil {
					t.Fatal(err)
				}
				tool := "memory_search"
				if test.Kind == memory.RetrievalConversationExcerpt {
					tool = "memory_search_conversations"
				}
				arguments, _ := json.Marshal(map[string]string{"query": test.Query})
				client := &fakeClient{steps: []step{assistantStep("", nil, toolCall("experiment-lookup", tool, string(arguments))), assistantStep("Evidence received.", nil)}}
				started := time.Now()
				turnErr := f.session(reader, client).Send(context.Background(), "Retrieve the original fixture evidence.", &recorder{}, nil)
				elapsed := time.Since(started).Nanoseconds()
				var requests []json.RawMessage
				var requestBytes, memoryBytes []int
				for _, request := range client.reqs {
					encoded, err := openrouter.RequestBytes(request)
					if err != nil {
						t.Fatal(err)
					}
					requests = append(requests, encoded)
					requestBytes = append(requestBytes, len(encoded))
					for _, message := range request.Messages {
						if strings.HasPrefix(message.Content, "EVIE_MEMORY_DATA\n") {
							serialized, _ := json.Marshal(message)
							memoryBytes = append(memoryBytes, len(serialized))
						}
					}
				}
				events, err := f.store.LoadEvents(context.Background(), reader.ID)
				if err != nil {
					t.Fatal(err)
				}
				var receipt *memory.RetrievalReceipt
				for _, event := range events {
					if event.Type == memory.EventContextSnapshot {
						var snapshot memory.ContextSnapshotPayload
						if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
							t.Fatal(err)
						}
						receipt = snapshot.Memory
					}
					if event.Type == memory.EventUserMessage || event.Type == memory.EventAssistantMessage {
						byEvent[event.ID] = fmt.Sprintf("aux/reader/%s/%s/%s/%d/%d", test.Scope, method, test.ID, repetition, event.Sequence)
					}
				}
				var evidence []memory.RetrievalEvidence
				if len(client.reqs) > 0 {
					evidence = denseTurnEvidence(t, client.reqs[len(client.reqs)-1])
				}
				var ids []string
				violations, secrets, unknown := 0, 0, 0
				for _, item := range evidence {
					id := byClaim[item.ClaimID]
					if item.Kind == memory.RetrievalConversationExcerpt && len(item.Sources) == 1 {
						id = byEvent[item.Sources[0].EventID]
					}
					if id == "" {
						unknown++
					}
					ids = append(ids, id)
					if slices.Contains(test.Forbidden, id) || item.ScopeKey != denseExperimentScope(reader) && (item.Kind == memory.RetrievalConversationExcerpt || item.ScopeKey != "global") {
						violations++
					}
					data, _ := json.Marshal(item)
					if memory.HasRetrievalSecret(data) {
						secrets++
					}
				}
				count := 0
				for _, want := range test.Expected {
					if slices.Contains(ids, want) {
						count++
					}
				}
				caps := 0
				if len(evidence) > retrievalResultLimit {
					caps++
				}
				for _, size := range memoryBytes {
					if size > retrievalTurnBytes {
						caps++
					}
				}
				after, err := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
				unchanged := err == nil && reflect.DeepEqual(before.ScopeRevisions, after.ScopeRevisions)
				row := map[string]any{"case": test.ID, "category": test.Category, "scope": test.Scope, "kind": test.Kind, "method": method, "repetition": repetition, "ids": ids, "expected": test.Expected, "matched": count, "forbidden_or_scope_count": violations, "secret_evidence_count": secrets, "unmapped_evidence_count": unknown, "cap_violations": caps, "whole_turn_ns": elapsed, "complete_request_bytes": requestBytes, "memory_message_json_bytes": memoryBytes, "provider_calls": len(client.reqs), "turn_failed": turnErr != nil, "accepted_revisions_unchanged": unchanged}
				rows = append(rows, row)
				if err = traceWriter.Encode(map[string]any{"sample": row, "complete_requests": requests, "evidence": evidence, "receipt": receipt}); err != nil {
					t.Fatal(err)
				}
				if err = compressed.Flush(); err != nil {
					t.Fatal(err)
				}
				if turnErr != nil || !unchanged || len(client.reqs) != 2 {
					t.Fatalf("complete experiment turn failed; exact trace retained: %v", turnErr)
				}
				latencies[method] = append(latencies[method], elapsed)
				matched[method] += count
				expected[method] += len(test.Expected)
				if test.Category == "paraphrase" {
					paraphraseMatched[method] += count
					paraphraseExpected[method] += len(test.Expected)
				}
				forbiddenCount += violations
				secretEvidence += secrets
				capViolations += caps
				unknownEvidence += unknown
			}
		}
	}
	methods := make(map[string]any)
	paraphraseRecall := map[string]float64{}
	for _, method := range []string{"lexical", "hybrid"} {
		paraphraseRecall[method] = float64(paraphraseMatched[method]) / float64(paraphraseExpected[method])
		methods[method] = map[string]any{"matched": matched[method], "expected": expected[method], "evidence_recall": float64(matched[method]) / float64(expected[method]), "paraphrase_matched": paraphraseMatched[method], "paraphrase_expected": paraphraseExpected[method], "paraphrase_recall": paraphraseRecall[method], "whole_turn": summarizeRetrievalSlice("ns", latencies[method])}
	}
	observer.server.Close()
	observer.mu.Lock()
	secretInputs := 0
	for _, observation := range observer.rows {
		secretInputs += observation.SecretInputs
	}
	observer.mu.Unlock()
	gates := map[string]bool{"paraphrase_recall_at_least_85_percent": paraphraseRecall["hybrid"] >= .85, "paraphrase_improvement_at_least_10_percentage_points": paraphraseRecall["hybrid"]-paraphraseRecall["lexical"] >= .10, "whole_turn_p95_at_most_250ms": summarizeRetrievalSlice("ns", latencies["hybrid"]).P95 <= int64(250*time.Millisecond), "zero_forbidden_or_scope_violations": forbiddenCount == 0, "zero_secret_embedding_inputs": secretInputs == 0, "zero_secret_provider_evidence": secretEvidence == 0, "zero_cap_violations": capViolations == 0, "all_evidence_mapped": unknownEvidence == 0}
	denseExperimentWrite(t, filepath.Join(output, "report.json"), map[string]any{"version": "memory-stage5-dense-integration-v1", "partition": partition, "freeze_sha256": memory.CompilerHash(freezeBytes), "cases": len(cases), "repetitions": 3, "methods": methods, "gates": gates, "secret_embedding_inputs": secretInputs, "configuration": map[string]any{"results_per_search": retrievalResultLimit, "result_bytes": retrievalResultBytes, "turn_serialized_bytes": retrievalTurnBytes, "search_deadline_ns": retrievalSearchDeadline.Nanoseconds(), "cumulative_work_ns": retrievalTurnWork.Nanoseconds(), "reader": "scripted", "automatic_recall": false, "observer_overhead_included": true}})
	for name, passed := range gates {
		if !passed {
			t.Errorf("frozen integration gate failed: %s", name)
		}
	}
}
