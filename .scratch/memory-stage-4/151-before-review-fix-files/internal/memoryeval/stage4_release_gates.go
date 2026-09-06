package memoryeval

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

var stage4InfrastructureMetrics = []string{"foreground_finalization_ns", "terminal_commit_ns", "candidate_freshness_ns", "model_rss_bytes", "database_growth_bytes", "wal_peak_bytes", "compiler_backlog", "review_backlog", "active_review_ns_per_useful_change"}
var stage4ConformanceBoundaries = []string{"scope", "authority", "source_binding", "persistence", "replay"}

func validateStage4Gates(gates []Stage4NumericGate) error {
	required := map[string]bool{}
	for _, population := range []string{"raw", "retained"} {
		for _, name := range stage4QualityNames {
			required["quality/"+population+"/"+name] = true
		}
	}
	for _, name := range stage4InfrastructureMetrics {
		required["infrastructure/"+name] = true
	}
	seen := map[string]bool{}
	for _, g := range gates {
		if !required[g.Metric] || seen[g.Metric] || !finite(g.Limit) || g.MinimumSamples < 1 || !slices.Contains([]string{"at_least", "at_most"}, g.Direction) || !slices.Contains([]string{"candidate", "paired_delta"}, g.Comparison) {
			return fmt.Errorf("Unknown, duplicate, nonnumeric, or incomplete release gate %q.", g.Metric)
		}
		seen[g.Metric] = true
		if strings.HasPrefix(g.Metric, "quality/") {
			positive := strings.HasSuffix(g.Metric, "supported_useful_precision") || strings.HasSuffix(g.Metric, "required_memory_recall")
			direction := "at_most"
			if positive {
				direction = "at_least"
			}
			if g.Direction != direction || g.Statistic != "rate" || g.Comparison != "candidate" || g.Limit < 0 || g.Limit > 1 || (positive && g.Limit == 0) {
				return fmt.Errorf("Quality gates require explicit rates, positive precision/recall floors, and maximum error rates.")
			}
		} else if !slices.Contains([]string{"mean", "p95", "max"}, g.Statistic) || g.Direction != "at_most" || (g.Comparison == "candidate" && g.Limit < 0) {
			return fmt.Errorf("Infrastructure gates require a declared mean/p95/max ceiling and candidate or paired-delta comparison.")
		}
	}
	for name := range required {
		if !seen[name] {
			return fmt.Errorf("Missing numerical release gate %q; approval rate cannot replace semantic quality or active review cost.", name)
		}
	}
	return nil
}

func validateStage4Conformance(p Stage4ReleasePlan, e Stage4Execution, add func(string, string, string)) {
	seen := map[string]bool{}
	for _, c := range e.Conformance {
		id := "conformance/" + c.Boundary
		if !slices.Contains(stage4ConformanceBoundaries, c.Boundary) || seen[c.Boundary] {
			add(id, "fail", "Unknown or duplicate deterministic boundary.")
			continue
		}
		seen[c.Boundary] = true
		if c.Command == "" || !canonicalSHA256.MatchString(c.ArtifactSHA256) || c.StartedAt.Before(p.FrozenAt) || c.Passed < 1 || c.Failed < 0 || c.Errors < 0 || c.Skipped < 0 {
			add(id, "fail", "Missing frozen-configuration rerun, executable command, checked evidence, or positive test count.")
		} else if c.Failed > 0 || c.Errors > 0 || c.Skipped > 0 {
			add(id, "fail", "A failed, errored, or skipped deterministic boundary blocks readiness regardless of averages.")
		} else {
			add(id, "pass", "All declared deterministic checks passed on the frozen configuration; zero failures, errors, or skips.")
		}
	}
	for _, boundary := range stage4ConformanceBoundaries {
		if !seen[boundary] {
			add("conformance/"+boundary, "pending", "Required deterministic boundary has not been re-run on the frozen release configuration.")
		}
	}
}

func evaluateStage4NumericGates(p Stage4ReleasePlan, e Stage4Execution, r *Stage4ReleaseReport) {
	metrics := map[string]Stage4Rate{}
	for _, s := range r.Quality {
		if s.Role == "candidate" {
			for name, v := range qualityMetrics(s) {
				metrics["quality/"+s.Population+"/"+name] = v
			}
		}
	}
	known := map[string]bool{}
	for _, g := range p.Gates {
		known[g.Metric] = true
	}
	invalid := map[string]string{}
	groups := map[string][]Stage4Measurement{}
	seen := map[string]bool{}
	for _, m := range e.Measurements {
		if !known[m.Metric] || !strings.HasPrefix(m.Metric, "infrastructure/") {
			r.Gates = append(r.Gates, Stage4GateResult{ID: "measurement_binding", Status: "fail", Detail: "Observation refers to an unplanned infrastructure metric."})
			continue
		}
		key := fmt.Sprintf("%s/%s/%d", m.Metric, m.Workload, m.Repetition)
		if seen[key] || !slices.Contains(p.Workloads, m.Workload) || m.Repetition < 1 || m.Repetition > p.Repetitions || !canonicalSHA256.MatchString(m.ArtifactSHA256) || m.StartedAt.Before(p.FrozenAt) || !finite(m.Baseline) || !finite(m.Candidate) || m.Baseline < 0 || m.Candidate < 0 {
			invalid[m.Metric] = "Invalid, duplicate, or unplanned paired observation."
		}
		seen[key] = true
		if m.Kind != "actual_integrated_model" {
			invalid[m.Metric] = "Scripted infrastructure, development spike, or unverified model observations cannot satisfy final integrated-model gates."
		}
		if strings.HasSuffix(m.Metric, "active_review_ns_per_useful_change") && !m.HumanReceiptVerified {
			invalid[m.Metric] = "Actual human active-review observations need separately verified useful accepted-change receipts; approval rate is not accuracy."
		}
		groups[m.Metric] = append(groups[m.Metric], m)
	}
	for _, g := range p.Gates {
		result := Stage4GateResult{ID: g.Metric, Status: "pending", Detail: "Insufficient observations for the predeclared gate.", Limit: &g.Limit}
		if strings.HasPrefix(g.Metric, "quality/") {
			rate := metrics[g.Metric]
			result.Value = rate.Value
			result.Samples = rate.Denominator
			if rate.Value == nil {
				result.Detail = "The denominator is zero; abstention is not perfect precision."
			}
		} else {
			if reason := invalid[g.Metric]; reason != "" {
				result.Status = "fail"
				result.Detail = reason
				r.Gates = append(r.Gates, result)
				continue
			}
			samples := groups[g.Metric]
			result.Samples = len(samples)
			if len(samples) != len(p.Workloads)*p.Repetitions {
				result.Detail = "Every declared workload and repetition requires a matched disabled/baseline and candidate observation."
				r.Gates = append(r.Gates, result)
				continue
			}
			// Every workload must satisfy the same frozen ceiling. Pooling
			// small and large histories would hide a failing scale boundary.
			worst := math.Inf(-1)
			for _, workload := range p.Workloads {
				values := []float64{}
				for _, sample := range samples {
					if sample.Workload != workload {
						continue
					}
					value := sample.Candidate
					if g.Comparison == "paired_delta" {
						value -= sample.Baseline
					}
					values = append(values, value)
				}
				worst = math.Max(worst, stage4Statistic(values, g.Statistic))
			}
			result.Value = &worst
		}
		if result.Value != nil && result.Samples >= g.MinimumSamples {
			pass := *result.Value <= g.Limit
			if g.Direction == "at_least" {
				pass = *result.Value >= g.Limit
			}
			result.Status = "fail"
			if pass {
				result.Status = "pass"
			}
			result.Detail = "Observed " + g.Statistic + " " + g.Comparison + " compared with the unchanged predeclared " + g.Direction + " threshold."
		}
		r.Gates = append(r.Gates, result)
	}
}

func stage4Statistic(values []float64, statistic string) float64 {
	slices.Sort(values)
	if statistic == "mean" {
		sum := 0.0
		for _, v := range values {
			sum += v
		}
		return sum / float64(len(values))
	}
	if statistic == "p95" {
		return values[int(math.Ceil(.95*float64(len(values))))-1]
	}
	return values[len(values)-1]
}
