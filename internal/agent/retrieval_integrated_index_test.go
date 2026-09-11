package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

type integratedIndexGates struct {
	Resources struct {
		BuildMS      int64 `json:"index_build_ms_max"`
		RebuildMS    int64 `json:"index_rebuild_ms_max"`
		StorageBytes int64 `json:"derived_index_database_and_wal_growth_bytes_max"`
		RSSBytes     int64 `json:"warm_incremental_worker_rss_growth_bytes_max"`
	} `json:"resources"`
}

type integratedIndexStorage struct {
	DatabaseBytes int64  `json:"database_bytes"`
	WALBytes      int64  `json:"wal_bytes"`
	DerivedBytes  *int64 `json:"derived_page_bytes,omitempty"`
	UpperBound    int64  `json:"derived_storage_upper_bound_bytes"`
	Method        string `json:"method"`
	DBStatError   string `json:"dbstat_error,omitempty"`
	Error         string `json:"error,omitempty"`
}

type integratedIndexRSS struct {
	PID      int    `json:"pid"`
	Bytes    *int64 `json:"bytes,omitempty"`
	Platform string `json:"platform"`
	Method   string `json:"method"`
	Error    string `json:"error,omitempty"`
}

type integratedIndexEndpointRSS struct {
	Method    string             `json:"method"`
	Processes []map[string]int64 `json:"processes"`
	Bytes     *int64             `json:"observed_process_family_rss_bytes,omitempty"`
	Error     string             `json:"error,omitempty"`
}

type integratedIndexMaintenance struct {
	ElapsedNS int64                      `json:"elapsed_ns"`
	Batches   int                        `json:"refresh_batches"`
	Coverage  memory.RetrievalCoverage   `json:"coverage"`
	Progress  []memory.RetrievalCoverage `json:"progress"`
	Error     string                     `json:"error,omitempty"`
}

type integratedIndexTurn struct {
	Query       string                      `json:"query"`
	SearchQuery string                      `json:"bounded_search_query"`
	ElapsedNS   int64                       `json:"elapsed_ns"`
	Dispatches  []integratedDispatch        `json:"dispatches"`
	Requests    []json.RawMessage           `json:"encoded_requests"`
	Calls       []integratedKernelCall      `json:"kernel_calls"`
	References  []memory.RetrievalReference `json:"canonical_references"`
	Unchanged   bool                        `json:"accepted_revisions_unchanged"`
	Inspectable bool                        `json:"delivered_sources_match_current_inspection"`
	Error       string                      `json:"error,omitempty"`
}

type integratedIndexReport struct {
	Version                string                              `json:"version"`
	CaseID                 string                              `json:"case_id"`
	SourceRecords          int                                 `json:"source_records"`
	SeedSHA256             string                              `json:"seed_database_sha256"`
	SeedIndexNS            int64                               `json:"original_seed_index_elapsed_ns"`
	SeedIndexBatches       int                                 `json:"original_seed_index_batches"`
	SeedCoverage           memory.RetrievalCoverage            `json:"original_seed_index_coverage"`
	Storage                map[string]integratedIndexStorage   `json:"storage"`
	Coverage               map[string]memory.RetrievalCoverage `json:"coverage"`
	Baseline               integratedIndexTurn                 `json:"baseline"`
	Reopened               integratedIndexTurn                 `json:"reopened"`
	Rebuilt                integratedIndexTurn                 `json:"rebuilt"`
	RestartAgreement       bool                                `json:"restart_agreement"`
	RebuildAgreement       bool                                `json:"rebuild_agreement"`
	Rebuild                integratedIndexMaintenance          `json:"rebuild"`
	AppendNS               int64                               `json:"public_append_elapsed_ns"`
	Incremental            integratedIndexMaintenance          `json:"incremental_refresh"`
	IncrementalSource      memory.SemanticSource               `json:"incremental_original_source"`
	IncrementalTurn        integratedIndexTurn                 `json:"incremental_turn"`
	IncrementalInspectable bool                                `json:"incremental_source_inspectable"`
	IncrementalWithheld    bool                                `json:"incremental_source_withheld_by_policy"`
	IncrementalPolicy      string                              `json:"incremental_access_outcome"`
	RSSBefore              integratedIndexRSS                  `json:"warm_incremental_rss_before"`
	RSSAfter               integratedIndexRSS                  `json:"warm_incremental_rss_after"`
	RSSDelta               *int64                              `json:"warm_incremental_rss_delta_bytes,omitempty"`
	EndpointRSSBefore      integratedIndexEndpointRSS          `json:"endpoint_rss_before"`
	EndpointRSSAfter       integratedIndexEndpointRSS          `json:"endpoint_rss_after"`
	InferenceRequests      *int64                              `json:"endpoint_inference_requests"`
	InferenceInputBytes    *int64                              `json:"endpoint_inference_input_bytes"`
	InferenceObservation   string                              `json:"endpoint_observation"`
	Failures               []string                            `json:"failures"`
	TestFailed             bool                                `json:"test_failed"`
}

func integratedIndexReadStorage(db *sql.DB, path string) integratedIndexStorage {
	result := integratedIndexStorage{Method: "dbstat-derived-pages-plus-all-WAL-upper-bound"}
	for _, file := range []struct {
		path string
		size *int64
	}{{path, &result.DatabaseBytes}, {path + "-wal", &result.WALBytes}} {
		info, err := os.Stat(file.path)
		if err != nil && !os.IsNotExist(err) {
			result.Error = err.Error()
			return result
		}
		if err == nil {
			*file.size = info.Size()
		}
	}
	var derived int64
	err := db.QueryRow(`SELECT COALESCE(SUM(d.pgsize),0) FROM dbstat AS d
LEFT JOIN sqlite_schema AS s ON s.name=d.name
WHERE d.name GLOB 'memory_dense_*' OR d.name GLOB 'memory_retrieval_*'
OR s.tbl_name GLOB 'memory_dense_*' OR s.tbl_name GLOB 'memory_retrieval_*'`).Scan(&derived)
	if err != nil {
		result.DBStatError = err.Error()
		result.Method = "dbstat-unavailable-conservative-whole-database-plus-WAL-upper-bound"
		result.UpperBound = result.DatabaseBytes + result.WALBytes
		return result
	}
	result.DerivedBytes = &derived
	result.UpperBound = derived + result.WALBytes
	return result
}

func integratedIndexReadRSS() integratedIndexRSS {
	result := integratedIndexRSS{PID: os.Getpid(), Platform: runtime.GOOS, Method: "ps -o rss= -p current-pid; KiB converted to bytes; model server excluded"}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		result.Error = "RSS measurement is not declared for this platform"
		return result
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(ctx, "ps", "-o", "rss=", "-p", strconv.Itoa(result.PID)).Output()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	kib, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil || kib <= 0 {
		result.Error = "ps did not return a positive RSS in KiB"
		return result
	}
	bytes := kib * 1024
	result.Bytes = &bytes
	return result
}

func integratedIndexReadEndpointRSS(endpoint string) integratedIndexEndpointRSS {
	result := integratedIndexEndpointRSS{Method: "frozen literal loopback listener via lsof; owned descendant PID/PPID/RSS snapshots via pgrep/ps; observed family only, not peak memory"}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "http" || parsed.Port() == "" || net.ParseIP(parsed.Hostname()) == nil || !net.ParseIP(parsed.Hostname()).IsLoopback() {
		result.Error = "listener RSS discovery is declared only for literal loopback HTTP endpoints; Unix sockets require separately frozen PID telemetry"
		return result
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(ctx, "lsof", "-nP", "-iTCP@"+net.JoinHostPort(parsed.Hostname(), parsed.Port()), "-sTCP:LISTEN", "-t").Output()
	if err != nil || len(strings.Fields(string(raw))) != 1 {
		result.Error = "owned endpoint listener PID is unavailable or ambiguous"
		return result
	}
	root, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || root <= 0 {
		result.Error = "invalid observed endpoint PID"
		return result
	}
	pids := []int{root}
	parents := map[int]int{root: -1}
	var total int64
	for i := 0; i < len(pids); i++ {
		if len(pids) > 32 {
			result.Error = "owned process-family discovery exceeded 32-process bound"
			return result
		}
		pid := pids[i]
		line, err := exec.CommandContext(ctx, "ps", "-o", "pid=,ppid=,rss=", "-p", strconv.Itoa(pid)).Output()
		fields := strings.Fields(string(line))
		if err != nil || len(fields) != 3 {
			result.Error = "an observed endpoint process exited or its RSS could not be sampled"
			return result
		}
		values := make([]int64, 3)
		for k, field := range fields {
			values[k], err = strconv.ParseInt(field, 10, 64)
			if err != nil {
				result.Error = "invalid endpoint process metric"
				return result
			}
		}
		if values[0] != int64(pid) || i > 0 && values[1] != int64(parents[pid]) || values[2] <= 0 {
			result.Error = "endpoint process ownership or RSS changed during sampling"
			return result
		}
		bytes := values[2] * 1024
		result.Processes = append(result.Processes, map[string]int64{"pid": values[0], "parent_pid": values[1], "rss_bytes": bytes})
		total += bytes
		children, childErr := exec.CommandContext(ctx, "pgrep", "-P", strconv.Itoa(pid)).Output()
		if childErr != nil {
			if exit, ok := childErr.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
				result.Error = "owned descendant discovery is unavailable"
				return result
			}
		}
		for _, child := range strings.Fields(string(children)) {
			id, err := strconv.Atoi(child)
			if err != nil || id <= 0 {
				result.Error = "invalid descendant PID"
				return result
			}
			if _, seen := parents[id]; !seen {
				parents[id] = pid
				pids = append(pids, id)
			}
		}
	}
	result.Bytes = &total
	return result
}

func integratedIndexRefresh(store *eviedb.Store, rebuild bool, budget time.Duration) integratedIndexMaintenance {
	result := integratedIndexMaintenance{}
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	started := time.Now()
	var err error
	if rebuild {
		result.Coverage, err = store.RebuildMemoryEmbeddings(ctx)
		result.Progress = append(result.Progress, result.Coverage)
	}
	for err == nil && result.Batches < 10000 {
		result.Coverage, err = store.RefreshMemoryIndex(ctx, 256)
		result.Batches++
		result.Progress = append(result.Progress, result.Coverage)
		if result.Coverage.State == "active" && result.Coverage.Pending == 0 {
			break
		}
	}
	result.ElapsedNS = time.Since(started).Nanoseconds()
	if err != nil {
		result.Error = err.Error()
	} else if result.Coverage.State != "active" || result.Coverage.Pending != 0 {
		result.Error = "bounded refresh did not reach active zero-pending coverage"
	}
	return result
}

func integratedIndexCanonical(refs []memory.RetrievalReference) ([]memory.RetrievalReference, error) {
	// Marshal-copy preserves nested slices while avoiding mutation of a saved
	// receipt or the actual provider trace being compared.
	raw, err := json.Marshal(refs)
	if err != nil {
		return nil, err
	}
	var result []memory.RetrievalReference
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	for i := range result {
		if !result[i].AsKnownAtConstrained {
			result[i].AsKnownAt = time.Time{}
		}
		if !result[i].ValidAtConstrained {
			result[i].ValidAt = time.Time{}
		}
		result[i].RetrievalGeneration = ""
	}
	return result, nil
}

func integratedIndexRunTurn(t *testing.T, f *retrievalFixture, seed integratedSeed, profile openrouter.ContextProfile, question string, unavailable bool) integratedIndexTurn {
	t.Helper()
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	searchQuery := integratedProbeQuery(question)
	client := &integratedClient{t: t, f: f, seed: seed, condition: "tool_only"}
	client.scripted = func(d integratedDispatch) openrouter.ChatResponse {
		if d.Call != 1 {
			return assistantStep("The fixed index recovery probe is complete; no reader-quality claim.", nil).res
		}
		intent := seed.Case.Gold.OracleIntent
		if intent == "" {
			intent = memory.RetrievalCurrent
		}
		accepted := map[string]any{"query": searchQuery, "intent": intent}
		if seed.Case.Gold.OracleValidAt != nil {
			accepted["valid_at"] = seed.Case.Gold.OracleValidAt.Format(time.RFC3339Nano)
		}
		acceptedJSON, _ := json.Marshal(accepted)
		conversationJSON, _ := json.Marshal(map[string]string{"query": searchQuery, "intent": intent})
		return assistantStep("", nil, toolCall("index-accepted", "memory_search", string(acceptedJSON)), toolCall("index-conversation", "memory_search_conversations", string(conversationJSON))).res
	}
	session, history, reader := integratedSession(t, f, seed, "tool_only", client, profile)
	client.reader = reader
	before, beforeErr := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
	if unavailable {
		t.Setenv("EVIE_REMOTE_MEMORY", "off")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	client.started = time.Now()
	err := session.Send(ctx, question, &recorder{}, nil)
	result := integratedIndexTurn{Query: question, SearchQuery: searchQuery, ElapsedNS: time.Since(client.started).Nanoseconds(), Dispatches: client.dispatches, Requests: client.encodedRequests, Calls: history.calls}
	after, afterErr := f.store.InspectClaims(context.Background(), reader.ScopeContext(), memory.ClaimQuery{})
	result.Unchanged = beforeErr == nil && afterErr == nil && reflect.DeepEqual(before.ScopeRevisions, after.ScopeRevisions)
	if len(client.dispatches) > 0 {
		last := client.dispatches[len(client.dispatches)-1]
		var originalRefs []memory.RetrievalReference
		for _, evidence := range last.Evidence {
			originalRefs = append(originalRefs, evidence.Reference())
		}
		inspected, inspectionErr := f.store.InspectMemoryEvidence(context.Background(), reader.ScopeContext(), originalRefs)
		result.Inspectable = inspectionErr == nil && len(inspected) == len(last.Evidence)
		for i, item := range inspected {
			if i >= len(last.Evidence) || !item.Available || item.Evidence == nil || item.Evidence.Text != last.Evidence[i].Text || !reflect.DeepEqual(item.Evidence.Sources, last.Evidence[i].Sources) {
				result.Inspectable = false
			}
		}
		if !result.Inspectable && err == nil {
			err = fmt.Errorf("actual delivered source text/metadata differs from current inspection: %v", inspectionErr)
		}
		if last.Snapshot.Memory != nil {
			var canonicalErr error
			result.References, canonicalErr = integratedIndexCanonical(last.Snapshot.Memory.Evidence)
			if canonicalErr != nil && err == nil {
				err = canonicalErr
			}
		}
	}
	if err != nil {
		result.Error = err.Error()
	} else if !result.Unchanged || len(client.dispatches) != 2 {
		result.Error = fmt.Sprintf("public turn invariant failed: revisions=%t dispatches=%d before=%v after=%v", result.Unchanged, len(client.dispatches), beforeErr, afterErr)
	}
	return result
}

func integratedIndexMeasureCase(t *testing.T, seedDirectory string, seed integratedSeed, profile openrouter.ContextProfile, gates integratedIndexGates, report *integratedIndexReport) {
	t.Helper()
	*report = integratedIndexReport{Version: "integrated-index-v1", CaseID: seed.Case.ID, SourceRecords: len(seed.Case.Records), SeedSHA256: seed.SeedDatabaseSHA256,
		SeedIndexNS: seed.IndexElapsedNS, SeedIndexBatches: seed.IndexBatches, SeedCoverage: seed.IndexCoverage,
		Storage: map[string]integratedIndexStorage{}, Coverage: map[string]memory.RetrievalCoverage{},
		InferenceObservation: "not instrumented: frozen endpoint unchanged; public Store API has no inference request/input-byte counters; no proxy or inferred model-call measurements"}
	check := func(ok bool, message string) {
		if !ok {
			report.Failures = append(report.Failures, message)
		}
	}
	coverage := func(stage string, f *retrievalFixture) {
		got, err := f.store.MemoryIndexCoverage(context.Background())
		report.Coverage[stage] = got
		check(err == nil && got.State == "active" && got.Pending == 0, stage+" lacks active zero-pending coverage")
	}
	storage := func(stage string, f *retrievalFixture) {
		got := integratedIndexReadStorage(f.db, f.path)
		report.Storage[stage] = got
		check(got.Error == "" && got.UpperBound <= gates.Resources.StorageBytes, stage+" exceeds derived storage upper bound or cannot measure files")
	}
	check(seed.IndexElapsedNS <= int64(time.Duration(gates.Resources.BuildMS)*time.Millisecond), "original seed index build exceeded frozen wall-time bound")
	check(seed.IndexCoverage.State == "active" && seed.IndexCoverage.Pending == 0, "original seed coverage is incomplete")
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	baseline := integratedCloneSeed(t, seedDirectory, seed)
	coverage("baseline", baseline)
	storage("baseline", baseline)
	report.Baseline = integratedIndexRunTurn(t, baseline, seed, profile, seed.Question, seed.Case.MemoryMode == "unavailable")
	check(report.Baseline.Error == "", "baseline public turn failed: "+report.Baseline.Error)

	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	restarted := integratedCloneSeed(t, seedDirectory, seed)
	if err := restarted.db.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	restarted.db, err = eviedb.OpenDBAt(restarted.path)
	if err != nil {
		t.Fatal(err)
	}
	restarted.store = eviedb.NewStore(restarted.db)
	coverage("reopened", restarted)
	storage("reopened", restarted)
	report.Reopened = integratedIndexRunTurn(t, restarted, seed, profile, seed.Question, seed.Case.MemoryMode == "unavailable")
	report.RestartAgreement = report.Baseline.Error == "" && report.Reopened.Error == "" && reflect.DeepEqual(report.Baseline.References, report.Reopened.References)
	check(report.RestartAgreement, "restart changed canonical delivered source references or failed the public turn")

	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	rebuilt := integratedCloneSeed(t, seedDirectory, seed)
	report.Rebuild = integratedIndexRefresh(rebuilt.store, true, time.Duration(gates.Resources.RebuildMS)*time.Millisecond)
	check(report.Rebuild.Error == "" && report.Rebuild.ElapsedNS <= int64(time.Duration(gates.Resources.RebuildMS)*time.Millisecond), "rebuild failed or exceeded frozen wall-time bound: "+report.Rebuild.Error)
	coverage("rebuilt", rebuilt)
	storage("rebuilt", rebuilt)
	report.Rebuilt = integratedIndexRunTurn(t, rebuilt, seed, profile, seed.Question, seed.Case.MemoryMode == "unavailable")
	report.RebuildAgreement = report.Baseline.Error == "" && report.Rebuilt.Error == "" && reflect.DeepEqual(report.Baseline.References, report.Rebuilt.References)
	check(report.RebuildAgreement, "rebuild changed canonical delivered source references or failed the public turn")

	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	incremental := integratedCloneSeed(t, seedDirectory, seed)
	coverage("before_append", incremental)
	storage("before_append", incremental)
	report.RSSBefore = integratedIndexReadRSS()
	marker := "indexprobe" + strings.ReplaceAll(seed.Case.ID, "_", "")
	started := time.Now()
	sources := integratedConversation(t, incremental, seed.Readers["empty"], "The incremental maintenance marker is "+marker+".", "Recorded the original maintenance statement.")
	report.AppendNS = time.Since(started).Nanoseconds()
	report.IncrementalSource = sources[0]
	report.Coverage["after_append"], err = incremental.store.MemoryIndexCoverage(context.Background())
	check(err == nil, "incremental pending coverage could not be measured")
	report.Incremental = integratedIndexRefresh(incremental.store, false, time.Duration(gates.Resources.BuildMS)*time.Millisecond)
	report.RSSAfter = integratedIndexReadRSS()
	if report.RSSBefore.Bytes != nil && report.RSSAfter.Bytes != nil {
		delta := *report.RSSAfter.Bytes - *report.RSSBefore.Bytes
		report.RSSDelta = &delta
		check(max(delta, 0) <= gates.Resources.RSSBytes, "warm incremental current-process RSS growth exceeded frozen bound")
	} else {
		check(false, "current-process RSS unavailable; growth gate cannot pass")
	}
	check(report.Incremental.Error == "", "incremental refresh failed: "+report.Incremental.Error)
	coverage("incremental", incremental)
	storage("incremental", incremental)
	// The appended marker uses current temporal intent but the original case's
	// access policy. Disabled memory must not be bypassed for verification.
	probeSeed := seed
	probeSeed.Case.Gold.OracleIntent = memory.RetrievalCurrent
	probeSeed.Case.Gold.OracleValidAt = nil
	unavailable := seed.Case.MemoryMode == "unavailable"
	report.IncrementalTurn = integratedIndexRunTurn(t, incremental, probeSeed, profile, marker, unavailable)
	check(report.IncrementalTurn.Error == "", "incremental verification turn failed: "+report.IncrementalTurn.Error)
	if len(report.IncrementalTurn.Dispatches) > 0 {
		last := report.IncrementalTurn.Dispatches[len(report.IncrementalTurn.Dispatches)-1]
		if unavailable {
			denied := 0
			for _, message := range last.Request.Messages {
				if message.Role == "tool" && (message.ToolCallID == "index-accepted" || message.ToolCallID == "index-conversation") && strings.Contains(message.Content, "model-facing memory reads require EVIE_REMOTE_MEMORY=on") {
					denied++
				}
			}
			report.IncrementalWithheld = denied == 2 && len(report.IncrementalTurn.Calls) == 0 && last.Snapshot.Memory == nil && len(last.Evidence) == 0
			report.IncrementalPolicy = "read-grant-denied-before-kernel; no synthetic memory receipt"
			for _, wire := range report.IncrementalTurn.Requests {
				if strings.Contains(string(wire), string(sources[0].EventID)) {
					report.IncrementalWithheld = false
				}
			}
		}
		if !unavailable {
			report.IncrementalPolicy = "current-source-policy-applied"
		}
		for _, item := range last.Evidence {
			if !integratedHasEvent(item, sources[0].EventID) {
				continue
			}
			inspected, err := incremental.store.InspectMemoryEvidence(context.Background(), seed.Readers["recent"].ScopeContext(), []memory.RetrievalReference{item.Reference()})
			if err == nil && len(inspected) == 1 && inspected[0].Available && inspected[0].Evidence != nil {
				for _, source := range inspected[0].Evidence.Sources {
					if source.EventID == sources[0].EventID && source.Evidence == sources[0].Evidence && source.EvidenceSHA256 == sources[0].EvidenceSHA256 && source.LocatorValue == sources[0].LocatorValue && source.Authority == memory.AuthorityOwnerStatement {
						report.IncrementalInspectable = true
					}
				}
			}
		}
	}
	if unavailable {
		check(report.IncrementalWithheld, "incremental source bypassed the unavailable case policy")
	} else {
		check(report.IncrementalInspectable, "newly appended original source was not supplied and inspectable with its exact locator")
	}
}

func TestMemoryStage5IntegratedIndexMeasurements(t *testing.T) {
	if os.Getenv("EVIE_MEMORY_INTEGRATED_FREEZE") == "" {
		t.Skip("integrated index measurements require an immutable freeze")
	}
	freeze, workload, directory := integratedFrozenInputs(t)
	raw, err := os.ReadFile(freeze.GatesPath)
	if err != nil {
		t.Fatal(err)
	}
	var gates integratedIndexGates
	if err := json.Unmarshal(raw, &gates); err != nil {
		t.Fatal(err)
	}
	if gates.Resources.BuildMS != 30000 || gates.Resources.RebuildMS != 30000 || gates.Resources.StorageBytes != 32*1024*1024 || gates.Resources.RSSBytes != 512*1024*1024 {
		t.Fatal("frozen index gates differ from declared diagnostic")
	}
	_, profile, _ := productionReaderSetup(t, "integrated-index")
	if profile.Diagnostics() != freeze.Profile {
		t.Fatal("actual production context profile differs from freeze")
	}
	for _, test := range workload.Cases {
		t.Run(test.ID, func(t *testing.T) {
			report := integratedIndexReport{}
			defer func() {
				report.TestFailed = t.Failed()
				productionReaderWrite(t, directory, "index-"+test.ID+".json", report)
			}()
			seed := integratedLoadSeed(t, freeze.InputsPath, test)
			endpointBefore := integratedIndexReadEndpointRSS(freeze.Endpoint)
			integratedIndexMeasureCase(t, filepath.Join(freeze.InputsPath, test.ID), seed, profile, gates, &report)
			report.EndpointRSSBefore = endpointBefore
			report.EndpointRSSAfter = integratedIndexReadEndpointRSS(freeze.Endpoint)
			for _, failure := range report.Failures {
				t.Error(failure)
			}
		})
	}
}

// This tracer uses the existing scripted local embedding endpoint, real SQLite
// and complete public turns. Its samples are not written as pilot measurements.
func TestIntegratedIndexDiagnosticPreservesPublicSourcesAndUnavailablePolicy(t *testing.T) {
	workload := integratedLoadWorkload(t, integratedWorkloadDefault)
	cases := []integratedCase{workload.Cases[0]}
	for _, c := range workload.Cases {
		if c.MemoryMode == "unavailable" || c.Gold.OracleIntent == memory.RetrievalHistorical {
			cases = append(cases, c)
		}
	}
	if len(cases) != 3 {
		t.Fatal("development tracer requires available, historical and unavailable policy cases")
	}
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			endpoint := newDenseFixtureEndpoint(t)
			t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", endpoint.server.URL)
			t.Setenv("EVIE_REMOTE_MEMORY", "on")
			directory := t.TempDir()
			seed := integratedBuildSeed(t, c, directory)
			gates := integratedIndexGates{}
			gates.Resources.BuildMS, gates.Resources.RebuildMS = 30000, 30000
			gates.Resources.StorageBytes, gates.Resources.RSSBytes = 32*1024*1024, 512*1024*1024
			report := integratedIndexReport{}
			integratedIndexMeasureCase(t, directory, seed, testContextProfile("test-model"), gates, &report)
			if len(report.Failures) > 0 || !report.RestartAgreement || !report.RebuildAgreement {
				var receipt *memory.RetrievalReceipt
				if len(report.IncrementalTurn.Dispatches) > 0 {
					receipt = report.IncrementalTurn.Dispatches[len(report.IncrementalTurn.Dispatches)-1].Snapshot.Memory
				}
				t.Fatalf("public recovery tracer failed: %v; incremental receipt=%+v, error=%q, references=%d", report.Failures, receipt, report.IncrementalTurn.Error, len(report.IncrementalTurn.References))
			}
			if c.MemoryMode == "unavailable" {
				if len(report.Baseline.References) != 0 || !report.IncrementalWithheld || report.IncrementalInspectable {
					t.Fatal("unavailable fixture did not prove zero delivered evidence and withheld incremental source")
				}
			} else if len(report.Baseline.References) == 0 || !report.IncrementalInspectable {
				t.Fatalf("empty comparison cannot prove original source recovery and incremental indexing: calls=%+v incremental=%t", report.Baseline.Calls, report.IncrementalInspectable)
			}
			if c.Gold.OracleIntent == memory.RetrievalHistorical {
				found := false
				for _, ref := range report.Rebuilt.References {
					if ref.ClaimID == seed.Bindings[c.Gold.SupportSets[0][0]].ClaimID && ref.Intent == memory.RetrievalHistorical && ref.CurrentStatus == memory.SemanticStatusRetired && ref.ValidAtConstrained && c.Gold.OracleValidAt != nil && ref.ValidAt.Equal(*c.Gold.OracleValidAt) {
						found = true
					}
				}
				if !found {
					t.Fatal("historical restart/rebuild comparison lost the explicit valid-at date or marked retired source")
				}
			}
		})
	}
}
