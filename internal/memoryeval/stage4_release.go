package memoryeval

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

// Stage4Submission carries metadata and independently adjudicated observations.
// It contains no source text, gold meanings, extractor input, or capability to
// run a model. Custodian and human attestations are external trust boundaries;
// hashes bind their exact documents, not the truth of an attestation.
type Stage4Submission struct {
	Version   string             `json:"version"`
	Plan      *Stage4ReleasePlan `json:"plan"`
	Approvals []Stage4Approval   `json:"approvals"`
	Custody   *Stage4Custody     `json:"custody"`
	Execution *Stage4Execution   `json:"execution"`
}

type Stage4ReleasePlan struct {
	Version               string              `json:"version"`
	ID                    string              `json:"id"`
	FrozenAt              time.Time           `json:"frozen_at"`
	Configuration         Stage4Configuration `json:"configuration"`
	BaselineConfiguration Stage4Configuration `json:"baseline_configuration"`
	CorpusSHA256          string              `json:"corpus_sha256"`
	GoldSHA256            string              `json:"gold_sha256"`
	RubricSHA256          string              `json:"rubric_sha256"`
	WorkloadSHA256        string              `json:"workload_sha256"`
	EnvironmentSHA256     string              `json:"environment_sha256"`
	SourceTree            string              `json:"source_tree"`
	Cases                 []Stage4Case        `json:"cases"`
	Repetitions           int                 `json:"repetitions"`
	Workloads             []string            `json:"workloads"`
	Gates                 []Stage4NumericGate `json:"gates"`
	Limits                []string            `json:"limits"`
}

type Stage4Configuration struct {
	GenerationID         string `json:"generation_id"`
	ModelSHA256          string `json:"model_sha256"`
	RuntimeSHA256        string `json:"runtime_sha256"`
	PromptSHA256         string `json:"prompt_sha256"`
	SchemaSHA256         string `json:"schema_sha256"`
	DecodingSHA256       string `json:"decoding_sha256"`
	EvidencePolicySHA256 string `json:"evidence_policy_sha256"`
}

type Stage4Case struct {
	ID               string `json:"id"`
	HistoryID        string `json:"history_id"`
	NarrativeFamily  string `json:"narrative_family"`
	RequiredMemories int    `json:"required_memories"`
}

// Metric names are a closed contract. Thresholds come from the actual pilot,
// never defaults in this evaluator. Point estimates and paired differences are
// explicit; confidence intervals are descriptive and do not replace a gate.
type Stage4NumericGate struct {
	Metric         string  `json:"metric"`
	Limit          float64 `json:"limit"`
	Direction      string  `json:"direction"`  // at_least or at_most
	Statistic      string  `json:"statistic"`  // rate, mean, p95, or max
	Comparison     string  `json:"comparison"` // candidate or paired_delta
	MinimumSamples int     `json:"minimum_samples"`
}

type Stage4Approval struct {
	Kind           string    `json:"kind"` // model_selection, gold_review, pilot_review, release_gates
	Reviewer       string    `json:"reviewer"`
	ReviewedAt     time.Time `json:"reviewed_at"`
	EvidenceSHA256 string    `json:"evidence_sha256"`
	SubjectSHA256  string    `json:"subject_sha256"`
	Status         string    `json:"status"`
}

type Stage4Custody struct {
	CorpusSHA256              string           `json:"corpus_sha256"`
	GoldSHA256                string           `json:"gold_sha256"`
	RecordSHA256              string           `json:"record_sha256"`
	Custodian                 string           `json:"custodian"`
	CompleteHistories         bool             `json:"complete_histories"`
	IndependentOfTuning       bool             `json:"independent_of_tuning"`
	ReviewedAt                time.Time        `json:"reviewed_at"`
	ExcludedNarrativeFamilies []string         `json:"excluded_narrative_families"`
	Exposures                 []Stage4Exposure `json:"exposures"`
}

type Stage4Exposure struct {
	CampaignID string    `json:"campaign_id"`
	PlanSHA256 string    `json:"plan_sha256"`
	Kind       string    `json:"kind"`
	RecordedAt time.Time `json:"recorded_at"`
}

type Stage4Execution struct {
	InputCorpusSHA256               string                    `json:"input_corpus_sha256"`
	PlanSHA256                      string                    `json:"plan_sha256"`
	CampaignID                      string                    `json:"campaign_id"`
	EnvironmentSHA256               string                    `json:"environment_sha256"`
	SourceTree                      string                    `json:"source_tree"`
	InputManifestSHA256             string                    `json:"input_manifest_sha256"`
	InputsContainOnlySourceEvidence bool                      `json:"inputs_contain_only_source_evidence"`
	Custodian                       string                    `json:"custodian"`
	Adjudicator                     string                    `json:"adjudicator"`
	AdjudicatedAt                   time.Time                 `json:"adjudicated_at"`
	AdjudicationSHA256              string                    `json:"adjudication_sha256"`
	Runs                            []Stage4ScoredRun         `json:"runs"`
	Conformance                     []Stage4ConformanceCheck  `json:"conformance"`
	Measurements                    []Stage4Measurement       `json:"measurements"`
	ReviewOutcomes                  []Stage4ReviewObservation `json:"review_outcomes"`
}

type Stage4ScoredRun struct {
	CaseID               string                 `json:"case_id"`
	Repetition           int                    `json:"repetition"`
	Role                 string                 `json:"role"`
	ConfigurationSHA256  string                 `json:"configuration_sha256"`
	StartedAt            time.Time              `json:"started_at"`
	FinishedAt           time.Time              `json:"finished_at"`
	Status               string                 `json:"status"` // ok or failed; failures remain in all denominators
	Failure              string                 `json:"failure"`
	RawOutputSHA256      string                 `json:"raw_output_sha256"`
	RetainedOutputSHA256 string                 `json:"retained_output_sha256"`
	Proposals            []Stage4ScoredProposal `json:"proposals"`
}

type Stage4ScoredProposal struct {
	SHA256        string   `json:"sha256"`
	Retained      bool     `json:"retained"`
	Label         string   `json:"label"`          // same semantic labels as the frozen spike scorer
	RequiredMatch *int     `json:"required_match"` // zero-based independently reviewed gold identity
	Errors        []string `json:"errors"`         // identity, temporal, source_attribution
}

type Stage4ConformanceCheck struct {
	Boundary       string    `json:"boundary"`
	Command        string    `json:"command"`
	ArtifactSHA256 string    `json:"artifact_sha256"`
	StartedAt      time.Time `json:"started_at"`
	Passed         int       `json:"passed"`
	Failed         int       `json:"failed"`
	Errors         int       `json:"errors"`
	Skipped        int       `json:"skipped"`
}

// Each value is either observed samples in the metric's fixed unit, or a
// numerator/denominator pair in the catalog's fixed units. No zero-valued
// aggregate can substitute for missing observations.
type Stage4Observation struct {
	Samples     []float64 `json:"samples,omitempty"`
	Numerator   *float64  `json:"numerator,omitempty"`
	Denominator *float64  `json:"denominator,omitempty"`
}

type Stage4MeasurementBinding struct {
	Workload                    string    `json:"workload"`
	Repetition                  int       `json:"repetition"`
	ArtifactSHA256              string    `json:"artifact_sha256"`
	Kind                        string    `json:"kind"`
	StartedAt                   time.Time `json:"started_at"`
	FinishedAt                  time.Time `json:"finished_at"`
	ConfigurationSHA256         string    `json:"configuration_sha256"`
	BaselineConfigurationSHA256 string    `json:"baseline_configuration_sha256"`
	EnvironmentSHA256           string    `json:"environment_sha256"`
	WorkloadSHA256              string    `json:"workload_sha256"`
	SourceTree                  string    `json:"source_tree"`
	Limits                      []string  `json:"limits"`
}

type Stage4Measurement struct {
	Stage4MeasurementBinding
	Metric               string            `json:"metric"`
	Baseline             Stage4Observation `json:"baseline"`
	Candidate            Stage4Observation `json:"candidate"`
	HumanReceiptVerified bool              `json:"human_receipt_verified"`
}

// Accepted and Edited are mutually exclusive final dispositions: Edited means
// accepted after an edit. Rejected and Deferred complete the reviewed-candidate
// partition. Multiple clicks on one candidate do not create extra outcomes.
type Stage4ReviewCounts struct {
	Accepted              int64 `json:"accepted"`
	Edited                int64 `json:"edited"`
	Rejected              int64 `json:"rejected"`
	Deferred              int64 `json:"deferred"`
	UsefulAcceptedChanges int64 `json:"useful_accepted_changes"`
	ActiveNanoseconds     int64 `json:"active_nanoseconds"`
	ElapsedNanoseconds    int64 `json:"elapsed_nanoseconds"`
}

type Stage4ReviewObservation struct {
	Stage4MeasurementBinding
	Baseline             Stage4ReviewCounts `json:"baseline"`
	Candidate            Stage4ReviewCounts `json:"candidate"`
	HumanReceiptVerified bool               `json:"human_receipt_verified"`
}

// Summaries retain both distributions and aggregate ratio denominators. All
// raw observations and their checked evidence bindings also remain in the report.
type Stage4ObservationSummary struct {
	UnavailablePairs int      `json:"unavailable_pairs"`
	ObservationCount int      `json:"observation_count"`
	Numerator        float64  `json:"numerator"`
	Denominator      float64  `json:"denominator"`
	Mean             *float64 `json:"mean"`
	P50              *float64 `json:"p50"`
	P95              *float64 `json:"p95"`
	Maximum          *float64 `json:"maximum"`
}

type Stage4MeasurementSummary struct {
	Metric              string                   `json:"metric"`
	Unit                string                   `json:"unit"`
	NumeratorUnit       string                   `json:"numerator_unit,omitempty"`
	DenominatorUnit     string                   `json:"denominator_unit,omitempty"`
	Workload            string                   `json:"workload"`
	Repetitions         int                      `json:"repetitions"`
	NumericGateDeclared bool                     `json:"numeric_gate_declared"`
	Baseline            Stage4ObservationSummary `json:"baseline"`
	Candidate           Stage4ObservationSummary `json:"candidate"`
}

type Stage4GateResult struct {
	ID      string   `json:"id"`
	Status  string   `json:"status"` // pass, fail, pending
	Detail  string   `json:"detail"`
	Value   *float64 `json:"value,omitempty"`
	Limit   *float64 `json:"limit,omitempty"`
	Samples int      `json:"samples"`
}

type Stage4ReleaseReport struct {
	FrozenPlan           *Stage4ReleasePlan         `json:"frozen_plan,omitempty"`
	ObservedFailures     []Stage4RunFailure         `json:"observed_failures"`
	Version              string                     `json:"version"`
	SubmissionSHA256     string                     `json:"submission_sha256"`
	PlanSHA256           string                     `json:"plan_sha256,omitempty"`
	Status               string                     `json:"status"`
	Ready                bool                       `json:"ready_for_ongoing_compilation"`
	HoldoutRunAuthorized bool                       `json:"holdout_run_authorized"`
	Gates                []Stage4GateResult         `json:"gates"`
	Panels               []PanelResult              `json:"panels"`
	Measurements         []Stage4Measurement        `json:"measurements"`
	MeasurementSummaries []Stage4MeasurementSummary `json:"measurement_summaries"`
	ReviewOutcomes       []Stage4ReviewObservation  `json:"review_outcomes"`
	Quality              []Stage4QualitySummary     `json:"quality"`
	Paired               []Stage4QualityDelta       `json:"paired_quality_deltas"`
	Limits               []string                   `json:"limits"`
	FollowUp             string                     `json:"follow_up"`
}

// Stage4Digest is the SHA-256 of the canonical Go JSON encoding. The CLI also
// records the hash of original submission bytes to distinguish input encoding.
func Stage4Digest(value any) string {
	b, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(b))
}

func AssessStage4Release(s Stage4Submission, verification ...Stage4EvidenceVerification) Stage4ReleaseReport {
	r := Stage4ReleaseReport{Version: "memory-stage4-release-report-v1", SubmissionSHA256: Stage4Digest(s), Status: "pending", Panels: EmptyPanels(), Gates: []Stage4GateResult{}, Quality: []Stage4QualitySummary{}, Paired: []Stage4QualityDelta{}, Limits: []string{
		"Offline assessment of submitted metadata and adjudicated counts; hashes verify identity, not human or custodian honesty.",
		"No model call, holdout-source access, or production activation is performed or authorized by this command.",
		"Retrieval and production-answer quality remain deferred to Stage 5.",
	}, FollowUp: "Resolve pending prerequisites before a separately authorized custodian run. Never tune on an exposed final holdout; move exposed families to development/regression and curate fresh material."}
	add := func(id, status, detail string) {
		r.Gates = append(r.Gates, Stage4GateResult{ID: id, Status: status, Detail: detail})
	}
	if s.Version != "memory-stage4-release-submission-v1" {
		add("submission", "fail", "Unknown submission version.")
	}
	if s.Plan == nil {
		for _, id := range []string{"frozen_plan", "model_selection", "gold_review", "pilot_review", "release_gates", "holdout_custody", "complete_paired_runs", "accepted_state_conformance", "learned_extraction", "integrated_workloads"} {
			add(id, "pending", "Required evidence has not been supplied.")
		}
		r.finish()
		return r
	}
	if len(verification) == 1 && verification[0].submissionSHA256 == Stage4Digest(s) {
		add("artifact_integrity", "pass", "Every referenced receipt and raw/retained output artifact was checked against its SHA-256; the proof binds this exact submission.")
	} else {
		add("artifact_integrity", "pending", "Referenced receipt and output bytes have not been verified for this exact submission.")
	}
	p := s.Plan
	r.PlanSHA256 = Stage4Digest(p)
	r.FrozenPlan = p
	r.Limits = append(r.Limits, p.Limits...)
	if err := p.validate(); err != nil {
		add("frozen_plan", "fail", err.Error())
		r.finish()
		return r
	}
	add("frozen_plan", "pass", "Configuration, corpus, rubric, environment, repetitions, baseline, workload and numerical gates are fixed by the plan digest.")
	expected := map[string]string{"model_selection": Stage4Digest(p.Configuration), "gold_review": p.GoldSHA256, "pilot_review": p.WorkloadSHA256, "release_gates": r.PlanSHA256}
	seen := map[string]bool{}
	for _, a := range s.Approvals {
		subject, ok := expected[a.Kind]
		if !ok || seen[a.Kind] {
			add("approvals", "fail", "Unknown or duplicate prerequisite approval.")
			continue
		}
		seen[a.Kind] = true
		if a.Status != "human_approved" || strings.TrimSpace(a.Reviewer) == "" || !canonicalSHA256.MatchString(a.EvidenceSHA256) || a.SubjectSHA256 != subject || a.ReviewedAt.IsZero() || a.ReviewedAt.After(p.FrozenAt) {
			add(a.Kind, "fail", "Approval is incomplete, not human-approved, post-freeze, or bound to different evidence.")
		} else {
			add(a.Kind, "pass", "Human approval is bound to the frozen prerequisite; source evidence remains separately auditable.")
		}
	}
	for _, id := range []string{"model_selection", "gold_review", "pilot_review", "release_gates"} {
		if !seen[id] {
			add(id, "pending", "Matching actual human approval has not been supplied.")
		}
	}
	if s.Custody == nil {
		add("holdout_custody", "pending", "Independent final-holdout custody has not been supplied.")
	} else if err := validateStage4Custody(*p, *s.Custody, r.PlanSHA256, s.Execution); err != nil {
		add("holdout_custody", "fail", err.Error())
	} else {
		add("holdout_custody", "pass", "Independent complete-history custody matches the frozen corpus; exposure record is consistent with this assessment.")
	}
	if s.Execution == nil {
		for _, id := range []string{"complete_paired_runs", "accepted_state_conformance", "learned_extraction", "integrated_workloads"} {
			add(id, "pending", "The frozen final configuration has not been evaluated.")
		}
		for _, g := range p.Gates {
			add(g.Metric, "pending", "No final observations exist; this numerical gate has not been evaluated.")
		}
		r.finish()
		return r
	}
	e := s.Execution
	if err := validateStage4Execution(*p, *e, r.PlanSHA256, s.Custody); err != nil {
		add("execution_binding", "fail", err.Error())
		r.finish()
		return r
	}
	add("execution_binding", "pass", "Execution and separate scoring are bound to the frozen plan, environment, source tree, inputs and custody.")
	complete, err := validateStage4Runs(*p, *e)
	if err != nil {
		add("complete_paired_runs", "fail", err.Error())
		r.finish()
		return r
	}
	if !complete {
		add("complete_paired_runs", "pending", "Missing planned case/role/repetition observations; no selective subset can pass.")
	} else {
		add("complete_paired_runs", "pass", "Every planned candidate/baseline case and repetition is recorded, including failed attempts.")
	}
	r.Quality, r.Paired = summarizeStage4Quality(*p, e.Runs)
	if !complete {
		r.Paired = []Stage4QualityDelta{}
	}
	for _, run := range e.Runs {
		if run.Status == "failed" {
			r.ObservedFailures = append(r.ObservedFailures, Stage4RunFailure{CaseID: run.CaseID, Repetition: run.Repetition, Role: run.Role, Failure: run.Failure, RawOutputSHA256: run.RawOutputSHA256})
		}
	}
	validateStage4Conformance(*p, *e, add)
	evaluateStage4NumericGates(*p, *e, &r)
	r.finish()
	return r
}

func (r *Stage4ReleaseReport) finish() {
	r.Status = "pass"
	for _, g := range r.Gates {
		if g.Status == "fail" {
			r.Status = "fail"
			break
		}
		if g.Status == "pending" {
			r.Status = "pending"
		}
	}
	r.Ready = r.Status == "pass"
	// A successful assessment is evidence, never permission or automatic activation.
	r.HoldoutRunAuthorized = false
	for i := range 2 {
		prefix := "conformance/"
		if i == 1 {
			prefix = "quality/"
		}
		status := StatusNotPopulated
		for _, g := range r.Gates {
			if strings.HasPrefix(g.ID, prefix) {
				if status == StatusNotPopulated {
					status = StatusPassed
				}
				if g.Status == "fail" {
					status = StatusFailed
				}
				if g.Status == "pending" && status != StatusFailed {
					status = StatusSkipped
				}
			}
		}
		r.Panels[i].Status = status
	}
}

func (p Stage4ReleasePlan) validate() error {
	if p.Version != "memory-stage4-release-plan-v1" || p.ID == "" || p.FrozenAt.IsZero() || !validStage4Tree(p.SourceTree) || len(p.Cases) == 0 || len(p.Cases) > 10000 || p.Repetitions < 2 || p.Repetitions > 100 || len(p.Workloads) == 0 || len(p.Limits) == 0 {
		return fmt.Errorf("Incomplete or unsupported frozen plan; at least two repetitions and explicit workload limits are required.")
	}
	for _, c := range []Stage4Configuration{p.Configuration, p.BaselineConfiguration} {
		if c.GenerationID == "" {
			return fmt.Errorf("Missing generation identity.")
		}
		for _, h := range []string{c.ModelSHA256, c.RuntimeSHA256, c.PromptSHA256, c.SchemaSHA256, c.DecodingSHA256, c.EvidencePolicySHA256} {
			if !canonicalSHA256.MatchString(h) {
				return fmt.Errorf("Incomplete frozen configuration identity.")
			}
		}
	}
	if Stage4Digest(p.Configuration) == Stage4Digest(p.BaselineConfiguration) {
		return fmt.Errorf("Candidate and baseline configurations must be distinct.")
	}
	for _, h := range []string{p.CorpusSHA256, p.GoldSHA256, p.RubricSHA256, p.WorkloadSHA256, p.EnvironmentSHA256} {
		if !canonicalSHA256.MatchString(h) {
			return fmt.Errorf("Incomplete corpus, rubric, workload or environment identity.")
		}
	}
	seen := map[string]bool{}
	required := 0
	families := map[string]bool{}
	historyFamilies := map[string]string{}
	for _, c := range p.Cases {
		if c.ID == "" || c.HistoryID == "" || c.NarrativeFamily == "" || c.RequiredMemories < 0 || c.RequiredMemories > 10000 || seen[c.ID] {
			return fmt.Errorf("Invalid or duplicate case metadata.")
		}
		if family, exists := historyFamilies[c.HistoryID]; exists && family != c.NarrativeFamily {
			return fmt.Errorf("Every window from one history must belong to the same narrative family.")
		}
		historyFamilies[c.HistoryID] = c.NarrativeFamily
		seen[c.ID] = true
		required += c.RequiredMemories
		families[c.NarrativeFamily] = true
	}
	if required == 0 || len(families) < 2 {
		return fmt.Errorf("A useful-recall denominator and at least two independent narrative families are required.")
	}
	seen = map[string]bool{}
	for _, w := range p.Workloads {
		if w == "" || seen[w] {
			return fmt.Errorf("Invalid or duplicate workload identity.")
		}
		seen[w] = true
	}
	return validateStage4Gates(p.Gates)
}

func validateStage4Custody(p Stage4ReleasePlan, c Stage4Custody, planHash string, e *Stage4Execution) error {
	if c.CorpusSHA256 != p.CorpusSHA256 || c.GoldSHA256 != p.GoldSHA256 || !canonicalSHA256.MatchString(c.RecordSHA256) || c.Custodian == "" || !c.CompleteHistories || !c.IndependentOfTuning || c.ReviewedAt.IsZero() || c.ReviewedAt.After(p.FrozenAt) {
		return fmt.Errorf("Custody must attest to independently curated, reviewed, complete histories before freeze, matching the corpus and gold.")
	}
	for i := 1; i <= 12; i++ {
		if !slices.Contains(c.ExcludedNarrativeFamilies, fmt.Sprintf("N%02d", i)) {
			return fmt.Errorf("Custody must explicitly exclude all original development and pilot families N01–N12.")
		}
	}
	for _, item := range p.Cases {
		if slices.Contains(c.ExcludedNarrativeFamilies, item.NarrativeFamily) {
			return fmt.Errorf("Final holdout includes an exposed development or pilot narrative family.")
		}
	}
	if e == nil {
		if len(c.Exposures) > 0 {
			return fmt.Errorf("Holdout has prior exposure without this frozen campaign; fresh independently curated material is required.")
		}
		return nil
	}
	if len(c.Exposures) != 1 {
		return fmt.Errorf("Exactly one logged frozen campaign exposure is required; interrupted or prior tuning exposure cannot be erased.")
	}
	x := c.Exposures[0]
	if x.Kind != "frozen_final_campaign" || x.CampaignID != e.CampaignID || x.PlanSHA256 != planHash || x.RecordedAt.Before(p.FrozenAt) || x.RecordedAt.IsZero() {
		return fmt.Errorf("Exposure was not logged for this frozen campaign before execution.")
	}
	return nil
}

func validateStage4Execution(p Stage4ReleasePlan, e Stage4Execution, planHash string, c *Stage4Custody) error {
	if e.InputCorpusSHA256 != p.CorpusSHA256 || e.PlanSHA256 != planHash || e.CampaignID == "" || e.EnvironmentSHA256 != p.EnvironmentSHA256 || e.SourceTree != p.SourceTree || !canonicalSHA256.MatchString(e.InputManifestSHA256) || !e.InputsContainOnlySourceEvidence || e.Custodian == "" || e.Adjudicator == "" || !canonicalSHA256.MatchString(e.AdjudicationSHA256) || e.AdjudicatedAt.Before(p.FrozenAt) {
		return fmt.Errorf("Execution identity, source-only input isolation, or independent human output adjudication is missing or mismatched.")
	}
	if c == nil || c.Custodian != e.Custodian || validateStage4Custody(p, *c, planHash, &e) != nil {
		return fmt.Errorf("Execution has no matching valid custody and pre-run exposure record.")
	}
	for _, run := range e.Runs {
		if run.StartedAt.Before(c.Exposures[0].RecordedAt) {
			return fmt.Errorf("Each attempt must begin after the matching campaign exposure was logged.")
		}
	}
	return nil
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func validStage4Tree(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
