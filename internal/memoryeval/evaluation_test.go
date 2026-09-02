package memoryeval

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEvaluationReportSchemaReservesAllPanelsAndFutureComponents(t *testing.T) {
	path := filepath.Join("..", "..", "cmd", "evie", "docs", "fixtures", "semantic-memory", "evaluation", "v1", "report.schema.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	encoded := string(data)
	for _, required := range []string{"semantic_conformance", "learned_extraction", "retrieval_provenance", "answer_abstention", "components", "baseline_run_id", "failure_taxonomy", "delta"} {
		if !strings.Contains(encoded, required) {
			t.Fatalf("report schema omits %q", required)
		}
	}
	if closed, ok := schema["additionalProperties"].(bool); !ok || closed {
		t.Fatalf("report schema is not closed: additionalProperties=%v", schema["additionalProperties"])
	}
}

func TestGeneratedReportSatisfiesClosedContract(t *testing.T) {
	report := Report{
		ReportSchemaVersion: 1,
		Run:                 RunIdentity{ID: "run-1", Commit: "abc123", StartedAt: time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)},
		Fixture:             FixtureIdentity{ManifestVersion: 1, FixtureVersion: "semantic-stage-3-v1", DatasetSHA256: "sha256:0000000000000000000000000000000000000000000000000000000000000000"},
		Environment:         Environment{Hardware: "test", OS: "test/test", GoVersion: "go-test", SQLiteVersion: "sqlite-test", JournalMode: "wal", JournalSetup: "foreign_keys=on", Repetitions: 1, Conditions: []string{"fixed fixture"}},
		Components:          map[string]ComponentIdentity{"semantic_kernel": {Name: "eviedb", Version: "stage-3"}},
		Cardinality:         Cardinality{Cases: 1},
		Panels:              EmptyPanels(),
		Cases:               []CaseResult{{ID: "semantic", Panel: PanelSemanticConformance, Gate: true, Status: StatusPassed, DurationNS: 1}},
		Metrics:             []Metric{SummarizeMetric("query", "ns", "warm", []int64{1})},
	}
	report.Summarize()
	if err := report.Validate(); err != nil {
		t.Fatalf("valid report rejected: %v", err)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Report
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatalf("strict report decode: %v", err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("generated JSON violates report contract: %v", err)
	}
}

func TestManifestValidationRequiresFrozenStage3Corpus(t *testing.T) {
	manifest, err := loadFrozenTestManifest()
	if err != nil {
		t.Fatalf("load frozen manifest: %v", err)
	}

	manifest.Cases[0].Expected.Snapshot = nil
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "snapshot") {
		t.Fatalf("missing snapshot error = %v", err)
	}
}

func TestManifestRejectsIgnoredRequestFieldsScopesAndGeneratedIDs(t *testing.T) {
	load := func(t *testing.T) Manifest {
		t.Helper()
		manifest, err := loadFrozenTestManifest()
		if err != nil {
			t.Fatal(err)
		}
		return manifest
	}
	t.Run("operation request", func(t *testing.T) {
		manifest := load(t)
		var request map[string]any
		if err := json.Unmarshal(manifest.Cases[0].Operations[0].Request, &request); err != nil {
			t.Fatal(err)
		}
		request["predciate"] = "ignored typo"
		manifest.Cases[0].Operations[0].Request, _ = json.Marshal(request)
		if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("unknown operation request field error=%v", err)
		}
	})
	t.Run("operation schema version", func(t *testing.T) {
		manifest := load(t)
		manifest.Cases[0].Operations[0].SchemaVersion = 2
		if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "schema version") {
			t.Fatalf("kind/schema mismatch error=%v", err)
		}
	})
	t.Run("typed operation enum", func(t *testing.T) {
		manifest := load(t)
		var request map[string]any
		_ = json.Unmarshal(manifest.Cases[0].Operations[0].Request, &request)
		request["predicate_cardinality"] = "bogus"
		manifest.Cases[0].Operations[0].Request, _ = json.Marshal(request)
		if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "predicate_cardinality") {
			t.Fatalf("invalid request enum error=%v", err)
		}
	})
	t.Run("rejection request", func(t *testing.T) {
		manifest := load(t)
		manifest.Cases[0].Expected.Rejections[0].Request = json.RawMessage(`{"session_id":"40000000-0000-4000-8000-000000000101","scope_key":"workspace:20000000-0000-4000-8000-000000000999","ignored":true}`)
		if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("unknown rejection request field error=%v", err)
		}
	})
	t.Run("source scope", func(t *testing.T) {
		manifest := load(t)
		manifest.Cases[0].Sources[0].ScopeKey = "global"
		if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "source scope") {
			t.Fatalf("source scope mismatch error=%v", err)
		}
	})
	t.Run("generated alias", func(t *testing.T) {
		manifest := load(t)
		manifest.Cases[0].Operations[0].GeneratedIDs["subject_alias_id"] = "62000000-0000-4000-8000-000000000199"
		if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "not asserted") {
			t.Fatalf("unasserted generated ID error=%v", err)
		}
	})
}

func TestLifecycleSchemaVersionTracksGraphTransitions(t *testing.T) {
	for _, test := range []struct {
		name          string
		schemaVersion int
		objectKind    string
		want          bool
	}{{"claim v3", 3, "claim", true}, {"graph link v5", 5, "graph_link", true}, {"graph link rejects v3", 3, "graph_link", false}, {"compound entity v5", 5, "entity", true}, {"local entity v3", 3, "entity", true}, {"alias v3", 3, "alias", true}, {"alias rejects v5", 5, "alias", false}} {
		t.Run(test.name, func(t *testing.T) {
			operation := FixtureOperation{Kind: "retire_memory", SchemaVersion: test.schemaVersion, Request: json.RawMessage(`{"object_kind":"` + test.objectKind + `"}`)}
			if got := fixtureOperationSchemaAllowed(operation); got != test.want {
				t.Fatalf("schema allowed=%t want=%t", got, test.want)
			}
		})
	}
}

func loadFrozenTestManifest() (Manifest, error) {
	path := filepath.Join("..", "..", "cmd", "evie", "docs", "fixtures", "semantic-memory", "evaluation", "v1", "manifest.json")
	return LoadManifest(path)
}

func TestReportAppliesOnlyPairedBaselineMetrics(t *testing.T) {
	report := Report{Metrics: []Metric{
		SummarizeMetric("current_query", "ns", "warm", []int64{20}),
		SummarizeMetric("new_metric", "bytes", "fixed_fixture", []int64{10}),
	}}
	baseline := Report{Run: RunIdentity{ID: "baseline-1"}, Metrics: []Metric{SummarizeMetric("current_query", "ns", "warm", []int64{15})}}
	report.ApplyBaseline(baseline)
	if report.BaselineRunID != "baseline-1" || report.Metrics[0].Baseline == nil || report.Metrics[0].Delta == nil || report.Metrics[0].Delta.P50 != 5 {
		t.Fatalf("paired baseline = %+v", report)
	}
	if report.Metrics[1].Baseline != nil || report.Metrics[1].Delta != nil || report.Metrics[0].Threshold != nil {
		t.Fatalf("baseline invented an unmatched comparison or threshold: %+v", report)
	}
}

func TestReportKeepsHardGatesAndPanelsSeparate(t *testing.T) {
	report := Report{
		ReportSchemaVersion: 1,
		Run:                 RunIdentity{ID: "run-1", Commit: "abc123", StartedAt: time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)},
		Fixture:             FixtureIdentity{ManifestVersion: 1, FixtureVersion: "semantic-stage-3-v1", DatasetSHA256: "sha256:fixture"},
		Environment:         Environment{Hardware: "test", OS: "test/test", GoVersion: "go-test", SQLiteVersion: "sqlite-test", JournalMode: "wal", Repetitions: 3, Conditions: []string{"cold-open", "warm-query"}},
		Panels:              EmptyPanels(),
		Cases:               []CaseResult{{ID: "scope-isolation", Panel: PanelSemanticConformance, Gate: true, Status: StatusFailed, Failure: &Failure{Taxonomy: FailureScopeLeakage, Message: "sibling scope visible"}}},
		Metrics:             []Metric{{Name: "operation_commit", Unit: "ns", Current: MetricValues{P50: 10, P95: 20, Max: 30}, Baseline: &MetricValues{P50: 8, P95: 16, Max: 24}, Delta: &MetricValues{P50: 2, P95: 4, Max: 6}}},
	}
	report.Summarize()
	if report.Passed() || report.Panels[0].Status != StatusFailed {
		t.Fatalf("hard gate was averaged away: %+v", report)
	}
	for _, panel := range report.Panels[1:] {
		if panel.Status != StatusNotPopulated || panel.CaseCount != 0 {
			t.Fatalf("future panel was populated: %+v", panel)
		}
	}

	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"baseline_run_id", "delta", "failure_taxonomy", "learned_extraction", "retrieval_provenance", "answer_abstention"} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("machine report omitted %q: %s", field, encoded)
		}
	}
	markdown := report.Markdown()
	for _, text := range []string{"Semantic memory evaluation", "Hard-gate failures", "scope-isolation", "Paired performance", "Not populated in Stage 3"} {
		if !strings.Contains(markdown, text) {
			t.Fatalf("human summary omitted %q:\n%s", text, markdown)
		}
	}
}

func TestMetricSummaryUsesFixedRepetitionsWithoutThreshold(t *testing.T) {
	metric := SummarizeMetric("current_query", "ns", "warm", []int64{40, 10, 30, 20, 50})
	if metric.Current.P50 != 30 || metric.Current.P95 != 50 || metric.Current.Max != 50 || metric.Repetitions != 5 {
		t.Fatalf("summary = %+v", metric)
	}
	if metric.Threshold != nil || metric.Baseline != nil || metric.Delta != nil {
		t.Fatalf("initial baseline invented comparison or threshold: %+v", metric)
	}
}
