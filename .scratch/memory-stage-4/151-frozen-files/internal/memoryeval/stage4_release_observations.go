package memoryeval

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

type stage4ObservationDefinition struct {
	Name                string
	Unit                string
	NumeratorUnit       string
	DenominatorUnit     string
	RequiredNumericGate bool
	Human               bool
}

// Required reporting is deliberately broader than mandatory numerical gates.
// The pilot can declare a gate for any listed metric; no new threshold is
// invented merely because an observation is required by the Stage 4 protocol.
var stage4ObservationDefinitions = []stage4ObservationDefinition{
	{Name: "foreground_finalization_ns", Unit: "nanoseconds", RequiredNumericGate: true},
	{Name: "terminal_commit_ns", Unit: "nanoseconds", RequiredNumericGate: true},
	{Name: "candidate_freshness_ns", Unit: "nanoseconds", RequiredNumericGate: true},
	{Name: "model_rss_bytes", Unit: "bytes", RequiredNumericGate: true},
	{Name: "database_growth_bytes", Unit: "bytes", RequiredNumericGate: true},
	{Name: "wal_peak_bytes", Unit: "bytes", RequiredNumericGate: true},
	{Name: "compiler_backlog", Unit: "persisted_events", RequiredNumericGate: true},
	{Name: "review_backlog", Unit: "candidates", RequiredNumericGate: true},
	{Name: "active_review_ns_per_useful_change", Unit: "nanoseconds/useful_change", NumeratorUnit: "nanoseconds", DenominatorUnit: "useful_changes", RequiredNumericGate: true, Human: true},
	{Name: "queue_latency_ns", Unit: "nanoseconds"},
	{Name: "inference_latency_ns", Unit: "nanoseconds"},
	{Name: "validation_latency_ns", Unit: "nanoseconds"},
	{Name: "database_completion_latency_ns", Unit: "nanoseconds"},
	{Name: "publication_latency_ns", Unit: "nanoseconds"},
	{Name: "evie_rss_bytes", Unit: "bytes"},
	{Name: "host_used_memory_bytes", Unit: "bytes"},
	{Name: "evie_cpu_percent", Unit: "percent"},
	{Name: "model_cpu_percent", Unit: "percent"},
	{Name: "host_cpu_percent", Unit: "percent"},
	{Name: "database_bytes", Unit: "bytes"},
	{Name: "wal_bytes", Unit: "bytes"},
	{Name: "inbox_age_ns", Unit: "nanoseconds"},
	{Name: "completed_events_per_second", Unit: "persisted_events/second", NumeratorUnit: "persisted_events", DenominatorUnit: "seconds"},
	{Name: "candidate_publications_per_second", Unit: "candidates/second", NumeratorUnit: "candidates", DenominatorUnit: "seconds"},
	{Name: "source_arrival_events_per_second", Unit: "persisted_events/second", NumeratorUnit: "persisted_events", DenominatorUnit: "seconds"},
	{Name: "review_useful_changes_per_second", Unit: "useful_changes/second", NumeratorUnit: "useful_changes", DenominatorUnit: "seconds", Human: true},
	{Name: "candidates_per_useful_accepted_change", Unit: "candidates/useful_change", NumeratorUnit: "candidates", DenominatorUnit: "useful_changes", Human: true},
}

func stage4ObservationCatalog() map[string]stage4ObservationDefinition {
	result := map[string]stage4ObservationDefinition{}
	for _, definition := range stage4ObservationDefinitions {
		result["infrastructure/"+definition.Name] = definition
	}
	return result
}

func validateStage4MeasurementBinding(p Stage4ReleasePlan, b Stage4MeasurementBinding, kind string) error {
	if !slices.Contains(p.Workloads, b.Workload) || b.Repetition < 1 || b.Repetition > p.Repetitions || !canonicalSHA256.MatchString(b.ArtifactSHA256) || b.StartedAt.Before(p.FrozenAt) || b.FinishedAt.Before(b.StartedAt) || b.Kind != kind || b.ConfigurationSHA256 != Stage4Digest(p.Configuration) || b.BaselineConfigurationSHA256 != Stage4Digest(p.BaselineConfiguration) || b.EnvironmentSHA256 != p.EnvironmentSHA256 || b.WorkloadSHA256 != p.WorkloadSHA256 || b.SourceTree != p.SourceTree || len(b.Limits) == 0 {
		return fmt.Errorf("observation lacks exact workload/repetition/configuration/environment/source binding, ordered times, actual execution kind, or explicit sampling limitations")
	}
	return nil
}

func stage4ObservationValues(o Stage4Observation, d stage4ObservationDefinition) ([]float64, error) {
	if d.NumeratorUnit != "" {
		if len(o.Samples) > 0 || o.Numerator == nil || o.Denominator == nil || !finite(*o.Numerator) || !finite(*o.Denominator) || *o.Numerator > 9_000_000_000_000_000 || *o.Denominator > 9_000_000_000_000_000 || *o.Numerator < 0 || *o.Denominator < 0 || math.Trunc(*o.Numerator) != *o.Numerator || (d.DenominatorUnit != "seconds" && math.Trunc(*o.Denominator) != *o.Denominator) {
			return nil, fmt.Errorf("rate observations require their exact nonnegative count/time numerator and denominator, without substituted sample values")
		}
		if *o.Denominator == 0 {
			return nil, nil
		}
		value := *o.Numerator / *o.Denominator
		if !finite(value) {
			return nil, fmt.Errorf("rate value is not finite")
		}
		return []float64{value}, nil
	}
	if o.Numerator != nil || o.Denominator != nil || len(o.Samples) == 0 || len(o.Samples) > 100000 {
		return nil, fmt.Errorf("sample observations require a nonempty bounded sample set, not an invented zero aggregate")
	}
	for _, value := range o.Samples {
		if !finite(value) || value < 0 {
			return nil, fmt.Errorf("observed samples must be finite and nonnegative")
		}
	}
	return slices.Clone(o.Samples), nil
}

func inspectStage4Measurements(p Stage4ReleasePlan, e Stage4Execution, r *Stage4ReleaseReport) (map[string][]Stage4Measurement, map[string]string) {
	catalog := stage4ObservationCatalog()
	groups := map[string][]Stage4Measurement{}
	invalid := map[string]string{}
	seen := map[string]bool{}
	r.Measurements = slices.Clone(e.Measurements)
	r.ReviewOutcomes = slices.Clone(e.ReviewOutcomes)
	add := func(id, status, detail string) {
		r.Gates = append(r.Gates, Stage4GateResult{ID: id, Status: status, Detail: detail})
	}
	for _, m := range e.Measurements {
		definition, known := catalog[m.Metric]
		if !known {
			add("measurement_binding", "fail", "Observation refers to an unknown protocol metric.")
			continue
		}
		key := fmt.Sprintf("%s/%s/%d", m.Metric, m.Workload, m.Repetition)
		if seen[key] {
			invalid[m.Metric] = "Duplicate observation for the same metric, workload and repetition."
		}
		seen[key] = true
		if err := validateStage4MeasurementBinding(p, m.Stage4MeasurementBinding, "actual_integrated_model"); err != nil {
			invalid[m.Metric] = err.Error()
		}
		for _, o := range []Stage4Observation{m.Baseline, m.Candidate} {
			if _, err := stage4ObservationValues(o, definition); err != nil {
				invalid[m.Metric] = err.Error()
			}
		}
		if definition.Human && !m.HumanReceiptVerified {
			invalid[m.Metric] = "Active review and usefulness observations require verified actual human receipts."
		}
		groups[m.Metric] = append(groups[m.Metric], m)
	}
	reviewErrors := inspectStage4ReviewOutcomes(p, e, groups, add)
	for metric, reason := range reviewErrors {
		invalid[metric] = reason
	}
	gated := map[string]bool{}
	for _, g := range p.Gates {
		gated[g.Metric] = true
	}
	for _, definition := range stage4ObservationDefinitions {
		metric := "infrastructure/" + definition.Name
		samples := groups[metric]
		if reason := invalid[metric]; reason != "" {
			add("observation/"+definition.Name, "fail", reason)
			continue
		}
		if len(samples) != len(p.Workloads)*p.Repetitions {
			add("observation/"+definition.Name, "pending", "Required reported observations are missing for one or more frozen workload/repetition pairs; this is separate from a numerical threshold.")
			continue
		}
		complete := true
		for _, w := range p.Workloads {
			summary := Stage4MeasurementSummary{Metric: metric, Unit: definition.Unit, NumeratorUnit: definition.NumeratorUnit, DenominatorUnit: definition.DenominatorUnit, Workload: w, NumericGateDeclared: gated[metric]}
			baseline, candidate := []Stage4Observation{}, []Stage4Observation{}
			for _, m := range samples {
				if m.Workload == w {
					summary.Repetitions++
					baseline = append(baseline, m.Baseline)
					candidate = append(candidate, m.Candidate)
				}
			}
			summary.Baseline = summarizeStage4Observations(baseline, definition)
			summary.Candidate = summarizeStage4Observations(candidate, definition)
			if summary.Baseline.ObservationCount == 0 || summary.Candidate.ObservationCount == 0 || summary.Baseline.UnavailablePairs > 0 || summary.Candidate.UnavailablePairs > 0 {
				complete = false
			}
			r.MeasurementSummaries = append(r.MeasurementSummaries, summary)
		}
		if complete {
			add("observation/"+definition.Name, "pass", "Required observations, sample counts, rate denominators, per-workload summaries and exact evidence bindings are retained; no undeclared threshold was applied.")
		} else {
			add("observation/"+definition.Name, "pending", "A measured zero denominator makes this required observation unavailable; no replacement value or perfect score is invented.")
		}
	}
	return groups, invalid
}

func summarizeStage4Observations(observations []Stage4Observation, d stage4ObservationDefinition) Stage4ObservationSummary {
	summary := Stage4ObservationSummary{}
	values := []float64{}
	for _, observation := range observations {
		v, _ := stage4ObservationValues(observation, d)
		if len(v) == 0 {
			summary.UnavailablePairs++
		}
		values = append(values, v...)
		if observation.Numerator != nil {
			summary.Numerator += *observation.Numerator
		}
		if observation.Denominator != nil {
			summary.Denominator += *observation.Denominator
		}
	}
	summary.ObservationCount = len(values)
	if len(values) > 0 {
		mean, p50, p95, maxValue := stage4Statistic(values, "mean"), stage4Statistic(values, "p50"), stage4Statistic(values, "p95"), stage4Statistic(values, "max")
		summary.Mean = &mean
		summary.P50 = &p50
		summary.P95 = &p95
		summary.Maximum = &maxValue
	}
	return summary
}

func inspectStage4ReviewOutcomes(p Stage4ReleasePlan, e Stage4Execution, groups map[string][]Stage4Measurement, add func(string, string, string)) map[string]string {
	invalid := map[string]string{}
	seen := map[string]bool{}
	failed := false
	humanMetrics := []string{"infrastructure/active_review_ns_per_useful_change", "infrastructure/review_useful_changes_per_second", "infrastructure/candidates_per_useful_accepted_change"}
	fail := func(reason string) {
		failed = true
		add("observation/review_outcomes", "fail", reason)
		for _, metric := range humanMetrics {
			invalid[metric] = reason
		}
	}
	for _, review := range e.ReviewOutcomes {
		key := fmt.Sprintf("%s/%d", review.Workload, review.Repetition)
		if seen[key] {
			fail("Duplicate actual owner-review outcome record.")
			continue
		}
		seen[key] = true
		if err := validateStage4MeasurementBinding(p, review.Stage4MeasurementBinding, "actual_owner_review"); err != nil {
			fail(err.Error())
			continue
		}
		if !review.HumanReceiptVerified {
			fail("Owner-review outcomes require verified actual human receipts.")
			continue
		}
		for _, counts := range []Stage4ReviewCounts{review.Baseline, review.Candidate} {
			bounded := counts.Accepted <= 1_000_000_000 && counts.Edited <= 1_000_000_000 && counts.Rejected <= 1_000_000_000 && counts.Deferred <= 1_000_000_000 && counts.ActiveNanoseconds <= 9_000_000_000_000_000 && counts.ElapsedNanoseconds <= 9_000_000_000_000_000
			if !bounded || counts.Accepted < 0 || counts.Edited < 0 || counts.Rejected < 0 || counts.Deferred < 0 || counts.UsefulAcceptedChanges < 0 || counts.UsefulAcceptedChanges > counts.Accepted+counts.Edited || counts.ActiveNanoseconds < 0 || counts.ElapsedNanoseconds <= 0 || counts.ActiveNanoseconds > counts.ElapsedNanoseconds || counts.Accepted+counts.Edited+counts.Rejected+counts.Deferred == 0 {
				fail("Review outcomes need nonnegative complete action counts, bounded useful accepted changes, and actual active/elapsed times.")
			}
		}
		for _, metric := range humanMetrics {
			for _, m := range groups[metric] {
				if m.Workload != review.Workload || m.Repetition != review.Repetition {
					continue
				}
				for _, pair := range []struct {
					o Stage4Observation
					c Stage4ReviewCounts
				}{{m.Baseline, review.Baseline}, {m.Candidate, review.Candidate}} {
					numerator, denominator := float64(pair.c.ActiveNanoseconds), float64(pair.c.UsefulAcceptedChanges)
					if strings.HasSuffix(metric, "review_useful_changes_per_second") {
						numerator, denominator = float64(pair.c.UsefulAcceptedChanges), float64(pair.c.ElapsedNanoseconds)/1e9
					}
					if strings.HasSuffix(metric, "candidates_per_useful_accepted_change") {
						numerator = float64(pair.c.Accepted + pair.c.Edited + pair.c.Rejected + pair.c.Deferred)
					}
					if pair.o.Numerator == nil || pair.o.Denominator == nil || *pair.o.Numerator != numerator || *pair.o.Denominator != denominator {
						fail("Review metric numerator/denominator does not match the verified action/usefulness/time record.")
					}
				}
			}
		}
	}
	if !failed {
		if len(seen) != len(p.Workloads)*p.Repetitions {
			add("observation/review_outcomes", "pending", "Actual accept/edit/reject/defer outcome counts and active review times are required for every declared workload/repetition.")
		} else {
			add("observation/review_outcomes", "pass", "Actual accept/edit/reject/defer counts, usefulness denominator and active/elapsed review time are retained separately from accuracy.")
		}
	}
	return invalid
}
