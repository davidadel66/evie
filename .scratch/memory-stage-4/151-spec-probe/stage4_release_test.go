package memoryeval_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memoryeval"
)

func TestStage4ReleaseAbsentEvidenceCannotAuthorizeHoldoutOrCompilation(t *testing.T) {
	report := memoryeval.AssessStage4Release(memoryeval.Stage4Submission{Version: "memory-stage4-release-submission-v1"})
	if report.Ready || report.HoldoutRunAuthorized || report.Status != "pending" {
		t.Fatalf("absent evidence must remain pending: %+v", report)
	}
	if len(report.Gates) < 8 {
		t.Fatalf("missing prerequisite gates: %+v", report.Gates)
	}
	if report.Panels[2].Status != memoryeval.StatusNotPopulated || report.Panels[3].Status != memoryeval.StatusNotPopulated {
		t.Fatal("Stage 5 panels must remain deferred")
	}
}

// These are metadata-only unit examples, not a corpus, model run, pilot,
// adjudication, or holdout. No narrative or gold meaning is authored here.
func releaseExample() memoryeval.Stage4Submission {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	h := func(s string) string { return memoryeval.Stage4Digest(s) }
	cfg := memoryeval.Stage4Configuration{GenerationID: "candidate", ModelSHA256: h("model"), RuntimeSHA256: h("runtime"), PromptSHA256: h("prompt"), SchemaSHA256: h("schema"), DecodingSHA256: h("decoding"), EvidencePolicySHA256: h("policy")}
	baseline := cfg
	baseline.GenerationID = "baseline"
	baseline.PromptSHA256 = h("baseline")
	p := &memoryeval.Stage4ReleasePlan{Version: "memory-stage4-release-plan-v1", ID: "metadata-unit-example", FrozenAt: at, Configuration: cfg, BaselineConfiguration: baseline, CorpusSHA256: h("unavailable source bytes"), GoldSHA256: h("unavailable gold bytes"), RubricSHA256: h("rubric"), WorkloadSHA256: h("workload"), EnvironmentSHA256: h("environment"), SourceTree: strings.Repeat("a", 40), Cases: []memoryeval.Stage4Case{{ID: "opaque-1", HistoryID: "history-1", NarrativeFamily: "family-a", RequiredMemories: 1}, {ID: "opaque-2", HistoryID: "history-2", NarrativeFamily: "family-b", RequiredMemories: 1}}, Repetitions: 2, Workloads: []string{"workload-1", "workload-2"}, Limits: []string{"Metadata-only unit test; no experimental or population claim."}}
	for _, population := range []string{"raw", "retained"} {
		for _, name := range []string{"supported_useful_precision", "required_memory_recall", "identity_errors", "temporal_errors", "source_attribution_errors", "unwanted_proposals", "failed_runs"} {
			direction := "at_most"
			limit := .5
			if name == "supported_useful_precision" || name == "required_memory_recall" {
				direction = "at_least"
			}
			p.Gates = append(p.Gates, memoryeval.Stage4NumericGate{Metric: "quality/" + population + "/" + name, Limit: limit, Direction: direction, Statistic: "rate", Comparison: "candidate", MinimumSamples: 1})
		}
	}
	for _, name := range []string{"foreground_finalization_ns", "terminal_commit_ns", "candidate_freshness_ns", "model_rss_bytes", "database_growth_bytes", "wal_peak_bytes", "compiler_backlog", "review_backlog", "active_review_ns_per_useful_change"} {
		p.Gates = append(p.Gates, memoryeval.Stage4NumericGate{Metric: "infrastructure/" + name, Limit: 10, Direction: "at_most", Statistic: "mean", Comparison: "candidate", MinimumSamples: 2})
	}
	s := memoryeval.Stage4Submission{Version: "memory-stage4-release-submission-v1", Plan: p}
	for _, a := range []struct{ kind, subject string }{{"model_selection", memoryeval.Stage4Digest(cfg)}, {"gold_review", p.GoldSHA256}, {"pilot_review", p.WorkloadSHA256}, {"release_gates", memoryeval.Stage4Digest(p)}} {
		s.Approvals = append(s.Approvals, memoryeval.Stage4Approval{Kind: a.kind, Reviewer: "unit-example-reviewer", ReviewedAt: at, EvidenceSHA256: h(a.kind), SubjectSHA256: a.subject, Status: "human_approved"})
	}
	s.Custody = &memoryeval.Stage4Custody{CorpusSHA256: p.CorpusSHA256, GoldSHA256: p.GoldSHA256, RecordSHA256: h("custody"), Custodian: "unit-example-custodian", CompleteHistories: true, IndependentOfTuning: true, ReviewedAt: at, Exposures: []memoryeval.Stage4Exposure{{CampaignID: "unit-example", PlanSHA256: memoryeval.Stage4Digest(p), Kind: "frozen_final_campaign", RecordedAt: at.Add(time.Second)}}}
	for i := 1; i <= 12; i++ {
		s.Custody.ExcludedNarrativeFamilies = append(s.Custody.ExcludedNarrativeFamilies, fmt.Sprintf("N%02d", i))
	}
	e := &memoryeval.Stage4Execution{PlanSHA256: memoryeval.Stage4Digest(p), CampaignID: "unit-example", EnvironmentSHA256: p.EnvironmentSHA256, SourceTree: p.SourceTree, InputManifestSHA256: h("inputs"), InputCorpusSHA256: p.CorpusSHA256, InputsContainOnlySourceEvidence: true, Custodian: s.Custody.Custodian, Adjudicator: "unit-example-adjudicator", AdjudicatedAt: at.Add(time.Hour), AdjudicationSHA256: h("adjudication")}
	for _, c := range p.Cases {
		for rep := 1; rep <= p.Repetitions; rep++ {
			for _, role := range []string{"candidate", "baseline"} {
				config := cfg
				if role == "baseline" {
					config = baseline
				}
				match := 0
				e.Runs = append(e.Runs, memoryeval.Stage4ScoredRun{CaseID: c.ID, Repetition: rep, Role: role, ConfigurationSHA256: memoryeval.Stage4Digest(config), StartedAt: at.Add(2 * time.Second), FinishedAt: at.Add(3 * time.Second), Status: "ok", RawOutputSHA256: h("raw"), RetainedOutputSHA256: h("retained"), Proposals: []memoryeval.Stage4ScoredProposal{{SHA256: h("opaque-proposal"), Retained: true, Label: "required_useful", RequiredMatch: &match}}})
			}
		}
	}
	for _, boundary := range []string{"scope", "authority", "source_binding", "persistence", "replay"} {
		e.Conformance = append(e.Conformance, memoryeval.Stage4ConformanceCheck{Boundary: boundary, Command: "unit-example-not-executed", ArtifactSHA256: h(boundary), StartedAt: at.Add(time.Second), Passed: 1})
	}
	for _, g := range p.Gates {
		if strings.HasPrefix(g.Metric, "infrastructure/") {
			for _, w := range p.Workloads {
				for rep := 1; rep <= p.Repetitions; rep++ {
					e.Measurements = append(e.Measurements, memoryeval.Stage4Measurement{Metric: g.Metric, Workload: w, Repetition: rep, ArtifactSHA256: h(g.Metric), Kind: "actual_integrated_model", StartedAt: at.Add(time.Second), Baseline: 1, Candidate: 2, HumanReceiptVerified: true})
				}
			}
		}
	}
	s.Execution = e
	return s
}

func TestStage4ReleaseCompleteMetadataAndIndependentPanels(t *testing.T) {
	r := assessWithEvidence(t, releaseExample())
	if !r.Ready || r.Status != "pass" || r.HoldoutRunAuthorized {
		t.Fatalf("unexpected result: %+v", r.Gates)
	}
	if len(r.Quality) != 4 || len(r.Paired) != 14 {
		t.Fatalf("missing raw/retained comparisons: %+v", r)
	}
	for _, q := range r.Quality {
		if q.RequiredRecall.Numerator != 4 || q.RequiredRecall.Denominator != 4 || q.Proposals != 4 {
			t.Fatalf("wrong worked counts: %+v", q)
		}
	}
	if r.Panels[0].Status != memoryeval.StatusPassed || r.Panels[1].Status != memoryeval.StatusPassed || r.Panels[2].Status != memoryeval.StatusNotPopulated {
		t.Fatalf("panels combined: %+v", r.Panels)
	}
	for _, d := range r.Paired {
		if d.Delta == nil || *d.Delta != 0 || d.ClusterCount != 2 || d.IntervalLow == nil || *d.IntervalLow != 0 {
			t.Fatalf("bad paired cluster uncertainty: %+v", d)
		}
	}
}

func TestStage4ReleaseRefusesMissingMismatchedAndContaminatedEvidence(t *testing.T) {
	cases := map[string]func(*memoryeval.Stage4Submission){
		"missing model approval":   func(s *memoryeval.Stage4Submission) { s.Approvals = s.Approvals[1:] },
		"model approval not human": func(s *memoryeval.Stage4Submission) { s.Approvals[0].Status = "proposed" },
		"duplicate approval":       func(s *memoryeval.Stage4Submission) { s.Approvals = append(s.Approvals, s.Approvals[0]) },
		"late approval":            func(s *memoryeval.Stage4Submission) { s.Approvals[0].ReviewedAt = s.Plan.FrozenAt.Add(time.Second) },
		"changed threshold":        func(s *memoryeval.Stage4Submission) { s.Plan.Gates[0].Limit = .1 },
		"zero recall threshold":    func(s *memoryeval.Stage4Submission) { s.Plan.Gates[1].Limit = 0 },
		"unknown approval metric":  func(s *memoryeval.Stage4Submission) { s.Plan.Gates[0].Metric = "approval_rate" },
		"missing numeric gate":     func(s *memoryeval.Stage4Submission) { s.Plan.Gates = s.Plan.Gates[1:] },
		"development family":       func(s *memoryeval.Stage4Submission) { s.Plan.Cases[0].NarrativeFamily = "N01" },
		"missing custody":          func(s *memoryeval.Stage4Submission) { s.Custody = nil },
		"custody not independent":  func(s *memoryeval.Stage4Submission) { s.Custody.IndependentOfTuning = false },
		"missing exposure":         func(s *memoryeval.Stage4Submission) { s.Custody.Exposures = nil },
		"prior tuning exposure": func(s *memoryeval.Stage4Submission) {
			s.Custody.Exposures = append(s.Custody.Exposures, s.Custody.Exposures[0])
		},
		"wrong gold":                func(s *memoryeval.Stage4Submission) { s.Custody.GoldSHA256 = memoryeval.Stage4Digest("wrong") },
		"gold in input":             func(s *memoryeval.Stage4Submission) { s.Execution.InputsContainOnlySourceEvidence = false },
		"wrong input corpus":        func(s *memoryeval.Stage4Submission) { s.Execution.InputCorpusSHA256 = memoryeval.Stage4Digest("wrong") },
		"wrong source tree":         func(s *memoryeval.Stage4Submission) { s.Execution.SourceTree = strings.Repeat("b", 40) },
		"wrong runtime environment": func(s *memoryeval.Stage4Submission) { s.Execution.EnvironmentSHA256 = memoryeval.Stage4Digest("wrong") },
		"missing repetition":        func(s *memoryeval.Stage4Submission) { s.Execution.Runs = s.Execution.Runs[1:] },
		"duplicate repetition":      func(s *memoryeval.Stage4Submission) { s.Execution.Runs = append(s.Execution.Runs, s.Execution.Runs[0]) },
		"wrong run configuration": func(s *memoryeval.Stage4Submission) {
			s.Execution.Runs[0].ConfigurationSHA256 = memoryeval.Stage4Digest("wrong")
		},
		"unjudged raw proposal":       func(s *memoryeval.Stage4Submission) { s.Execution.Runs[0].Proposals[0].Label = "pending" },
		"out of range gold match":     func(s *memoryeval.Stage4Submission) { n := 1; s.Execution.Runs[0].Proposals[0].RequiredMatch = &n },
		"hidden semantic error":       func(s *memoryeval.Stage4Submission) { s.Execution.Runs[0].Proposals[0].Errors = []string{"identity"} },
		"missing conformance":         func(s *memoryeval.Stage4Submission) { s.Execution.Conformance = nil },
		"scope violation":             func(s *memoryeval.Stage4Submission) { s.Execution.Conformance[0].Failed = 1 },
		"skipped replay":              func(s *memoryeval.Stage4Submission) { s.Execution.Conformance[4].Skipped = 1 },
		"missing workload repetition": func(s *memoryeval.Stage4Submission) { s.Execution.Measurements = s.Execution.Measurements[1:] },
		"scripted pilot": func(s *memoryeval.Stage4Submission) {
			s.Execution.Measurements[0].Kind = "scripted_infrastructure_only"
		},
		"unverified human receipt": func(s *memoryeval.Stage4Submission) {
			for i := range s.Execution.Measurements {
				s.Execution.Measurements[i].HumanReceiptVerified = false
			}
		},
		"duplicate workload observation": func(s *memoryeval.Stage4Submission) {
			s.Execution.Measurements = append(s.Execution.Measurements, s.Execution.Measurements[0])
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := releaseExample()
			mutate(&s)
			r := assessWithEvidence(t, s)
			if r.Ready || r.Status == "pass" || r.HoldoutRunAuthorized {
				t.Fatalf("invalid evidence passed: %+v", r.Gates)
			}
		})
	}
}

func TestStage4ReleaseAbstentionAndValidationLossStayVisible(t *testing.T) {
	s := releaseExample()
	for i := range s.Execution.Runs {
		if s.Execution.Runs[i].Role == "candidate" {
			s.Execution.Runs[i].Proposals = nil
		}
	}
	r := assessWithEvidence(t, s)
	if r.Ready {
		t.Fatal("universal abstention passed")
	}
	for _, q := range r.Quality {
		if q.Role == "candidate" && (q.SupportedUseful.Value != nil || q.RequiredRecall.Value == nil || *q.RequiredRecall.Value != 0 || q.RequiredRecall.Denominator != 4) {
			t.Fatalf("abstention denominators hidden: %+v", q)
		}
	}
	s = releaseExample()
	for i := range s.Execution.Runs {
		if s.Execution.Runs[i].Role == "candidate" {
			s.Execution.Runs[i].Proposals[0].Retained = false
		}
	}
	r = assessWithEvidence(t, s)
	if r.Ready {
		t.Fatal("validation losses hidden")
	}
	if r.Quality[2].RequiredRecall.Numerator != 4 || r.Quality[3].RequiredRecall.Numerator != 0 {
		t.Fatalf("raw and retained collapsed: %+v", r.Quality)
	}
}

func TestStage4ReleaseFailedAttemptStillCountsRequiredMemories(t *testing.T) {
	s := releaseExample()
	run := &s.Execution.Runs[0]
	run.Status = "failed"
	run.Failure = "timeout"
	run.Proposals = nil
	r := assessWithEvidence(t, s)
	if r.Quality[2].RequiredRecall.Numerator != 3 || r.Quality[2].RequiredRecall.Denominator != 4 || r.Quality[2].Failures.Numerator != 1 || len(r.ObservedFailures) != 1 {
		t.Fatalf("failed attempt lost: %+v", r)
	}
}

func TestStage4ReleaseExposureMustPrecedeEveryAttempt(t *testing.T) {
	s := releaseExample()
	s.Custody.Exposures[0].RecordedAt = s.Plan.FrozenAt.Add(10 * time.Second)
	if r := assessWithEvidence(t, s); r.Ready {
		t.Fatal("run before logged exposure passed")
	}
}

func TestStage4ReleaseWorkloadFailureCannotBeAveragedAway(t *testing.T) {
	s := releaseExample()
	for i := range s.Execution.Measurements {
		m := &s.Execution.Measurements[i]
		if m.Metric == "infrastructure/foreground_finalization_ns" && m.Workload == "workload-1" {
			m.Candidate = 16
		}
	}
	if r := assessWithEvidence(t, s); r.Ready {
		t.Fatal("one workload failed its ceiling but pooled mean passed")
	}
}

func assessWithEvidence(t *testing.T, s memoryeval.Stage4Submission) memoryeval.Stage4ReleaseReport {
	t.Helper()
	candidates := []string{"model_selection", "gold_review", "pilot_review", "release_gates", "custody", "inputs", "adjudication", "raw", "retained", "scope", "authority", "source_binding", "persistence", "replay"}
	if s.Plan != nil {
		for _, g := range s.Plan.Gates {
			candidates = append(candidates, g.Metric)
		}
	}
	available := map[string][]byte{}
	for _, value := range candidates {
		data, _ := json.Marshal(value)
		available[memoryeval.Stage4Digest(value)] = data
	}
	artifacts := map[string][]byte{}
	for _, hash := range memoryeval.Stage4RequiredEvidence(s) {
		artifacts[hash] = available[hash]
	}
	verified, err := memoryeval.VerifyStage4Evidence(s, artifacts)
	if err != nil {
		return memoryeval.AssessStage4Release(s)
	}
	return memoryeval.AssessStage4Release(s, verified)
}

func TestStage4ReleaseEvidenceHashesMustBeVerifiedForExactSubmission(t *testing.T) {
	s := releaseExample()
	if r := memoryeval.AssessStage4Release(s); r.Ready {
		t.Fatal("self-declared artifact hashes passed")
	}
	if _, err := memoryeval.VerifyStage4Evidence(s, map[string][]byte{}); err == nil {
		t.Fatal("missing files passed")
	}
	artifacts := map[string][]byte{}
	for _, hash := range memoryeval.Stage4RequiredEvidence(s) {
		artifacts[hash] = []byte("wrong bytes")
	}
	if _, err := memoryeval.VerifyStage4Evidence(s, artifacts); err == nil {
		t.Fatal("mismatched file bytes passed")
	}
}

func TestSpecProbeSameHistoryCanManufactureClusters(t *testing.T) {
 s := releaseExample()
 s.Plan.Cases[1].HistoryID = s.Plan.Cases[0].HistoryID
 digest := memoryeval.Stage4Digest(s.Plan)
 for i := range s.Approvals { if s.Approvals[i].Kind == "release_gates" { s.Approvals[i].SubjectSHA256 = digest } }
 s.Custody.Exposures[0].PlanSHA256 = digest
 s.Execution.PlanSHA256 = digest
 r := assessWithEvidence(t,s)
 if !r.Ready { t.Fatalf("probe assumption false: %+v", r.Gates) }
 if r.Paired[0].ClusterCount != 2 { t.Fatalf("wrong cluster count: %+v",r.Paired) }
 t.Logf("CONFIRMED: ready=%v with one unique HistoryID assigned to two reported independent narrative families",r.Ready)
}
