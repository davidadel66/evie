package memoryeval

import (
	"fmt"
	"math/rand/v2"
	"slices"
)

type Stage4RunFailure struct {
	CaseID          string `json:"case_id"`
	Repetition      int    `json:"repetition"`
	Role            string `json:"role"`
	Failure         string `json:"failure"`
	RawOutputSHA256 string `json:"raw_output_sha256"`
}

type Stage4Rate struct {
	Numerator   int      `json:"numerator"`
	Denominator int      `json:"denominator"`
	Value       *float64 `json:"value"`
}

type Stage4QualitySummary struct {
	Role            string     `json:"role"`
	Population      string     `json:"population"`
	Proposals       int        `json:"proposals"`
	SupportedUseful Stage4Rate `json:"supported_useful_precision"`
	RequiredRecall  Stage4Rate `json:"required_memory_recall"`
	IdentityErrors  Stage4Rate `json:"identity_errors"`
	TemporalErrors  Stage4Rate `json:"temporal_errors"`
	SourceErrors    Stage4Rate `json:"source_attribution_errors"`
	Unwanted        Stage4Rate `json:"unwanted_proposals"`
	Failures        Stage4Rate `json:"failed_runs"`
}

type Stage4QualityDelta struct {
	Metric       string   `json:"metric"`
	Candidate    *float64 `json:"candidate"`
	Baseline     *float64 `json:"baseline"`
	Delta        *float64 `json:"paired_delta"`
	ClusterCount int      `json:"independent_narrative_families"`
	IntervalLow  *float64 `json:"bootstrap_95_percent_low"`
	IntervalHigh *float64 `json:"bootstrap_95_percent_high"`
	Uncertainty  string   `json:"uncertainty"`
}

func stage4Rate(n, d int) Stage4Rate {
	r := Stage4Rate{Numerator: n, Denominator: d}
	if d > 0 {
		v := float64(n) / float64(d)
		r.Value = &v
	}
	return r
}

func validateStage4Runs(p Stage4ReleasePlan, e Stage4Execution) (bool, error) {
	cases := map[string]Stage4Case{}
	for _, c := range p.Cases {
		cases[c.ID] = c
	}
	seen := map[string]bool{}
	for _, run := range e.Runs {
		c, ok := cases[run.CaseID]
		if !ok || run.Repetition < 1 || run.Repetition > p.Repetitions || (run.Role != "candidate" && run.Role != "baseline") {
			return false, fmt.Errorf("Unplanned case, role, or repetition in extraction observations.")
		}
		key := fmt.Sprintf("%s/%s/%d", run.Role, run.CaseID, run.Repetition)
		if seen[key] {
			return false, fmt.Errorf("Duplicate extraction observation; failed attempts cannot be replaced by retries.")
		}
		seen[key] = true
		cfg := p.Configuration
		if run.Role == "baseline" {
			cfg = p.BaselineConfiguration
		}
		if run.ConfigurationSHA256 != Stage4Digest(cfg) || run.StartedAt.Before(p.FrozenAt) || run.FinishedAt.Before(run.StartedAt) || e.AdjudicatedAt.Before(run.FinishedAt) || !canonicalSHA256.MatchString(run.RawOutputSHA256) || !canonicalSHA256.MatchString(run.RetainedOutputSHA256) {
			return false, fmt.Errorf("Run configuration, times, raw output, or retained-output binding is invalid.")
		}
		if (run.Status != "ok" && run.Status != "failed") || (run.Status == "failed" && run.Failure == "") || (run.Status == "ok" && run.Failure != "") {
			return false, fmt.Errorf("Every failed attempt needs an explicit failure; success cannot conceal a failure.")
		}
		for _, q := range run.Proposals {
			if run.Status == "failed" && q.Retained {
				return false, fmt.Errorf("A failed extraction attempt cannot claim published retained candidates.")
			}
			if !canonicalSHA256.MatchString(q.SHA256) {
				return false, fmt.Errorf("Proposal digest is missing.")
			}
			if !slices.Contains([]string{"required_useful", "optional_useful", "unsupported", "unwanted_but_true"}, q.Label) {
				return false, fmt.Errorf("Every raw proposal requires actual human semantic adjudication; unresolved labels cannot pass.")
			}
			if (q.Label == "required_useful") != (q.RequiredMatch != nil) || (q.RequiredMatch != nil && (*q.RequiredMatch < 0 || *q.RequiredMatch >= c.RequiredMemories)) {
				return false, fmt.Errorf("Required-memory match is missing, outside the gold denominator, or attached to another label.")
			}
			seenError := map[string]bool{}
			for _, kind := range q.Errors {
				if !slices.Contains([]string{"identity", "temporal", "source_attribution"}, kind) || seenError[kind] {
					return false, fmt.Errorf("Unknown or duplicate semantic error category.")
				}
				seenError[kind] = true
			}
			if len(q.Errors) > 0 && (q.Label == "required_useful" || q.Label == "optional_useful") {
				return false, fmt.Errorf("A proposal with identity, temporal, or attribution errors cannot count as supported useful.")
			}
		}
	}
	return len(seen) == len(p.Cases)*p.Repetitions*2, nil
}

func summarizeStage4Quality(p Stage4ReleasePlan, runs []Stage4ScoredRun) ([]Stage4QualitySummary, []Stage4QualityDelta) {
	summaries := []Stage4QualitySummary{}
	for _, role := range []string{"baseline", "candidate"} {
		for _, population := range []string{"raw", "retained"} {
			summaries = append(summaries, stage4Quality(p, runs, role, population, nil))
		}
	}
	families := []string{}
	for _, c := range p.Cases {
		if !slices.Contains(families, c.NarrativeFamily) {
			families = append(families, c.NarrativeFamily)
		}
	}
	slices.Sort(families)
	// Aggregate each family once before resampling. Runtime then depends on
	// family count rather than re-reading every repeated proposal 14,000 times.
	caseFamily := map[string]string{}
	familyPlans := map[string]Stage4ReleasePlan{}
	familyRuns := map[string][]Stage4ScoredRun{}
	for _, c := range p.Cases {
		caseFamily[c.ID] = c.NarrativeFamily
		fp := familyPlans[c.NarrativeFamily]
		fp.Cases = append(fp.Cases, c)
		familyPlans[c.NarrativeFamily] = fp
	}
	for _, run := range runs {
		family := caseFamily[run.CaseID]
		familyRuns[family] = append(familyRuns[family], run)
	}
	cached := map[string]map[string]Stage4QualitySummary{}
	for _, family := range families {
		cached[family] = map[string]Stage4QualitySummary{}
		for _, role := range []string{"candidate", "baseline"} {
			for _, population := range []string{"raw", "retained"} {
				cached[family][role+"/"+population] = stage4Quality(familyPlans[family], familyRuns[family], role, population, nil)
			}
		}
	}
	deltas := []Stage4QualityDelta{}
	// Cluster entire narrative families, retaining all history windows and paired
	// repetitions together. Do not pretend repeated variants are independent.
	for _, population := range []string{"raw", "retained"} {
		b := qualityMetrics(stage4Quality(p, runs, "baseline", population, nil))
		c := qualityMetrics(stage4Quality(p, runs, "candidate", population, nil))
		for _, name := range stage4QualityNames {
			entry := Stage4QualityDelta{Metric: "quality/" + population + "/" + name, Baseline: b[name].Value, Candidate: c[name].Value, ClusterCount: len(families), Uncertainty: "Paired narrative-family cluster bootstrap, 1,000 resamples, fixed PCG seed (151, 4). Descriptive percentile interval; a small curated corpus is not a population guarantee."}
			if entry.Baseline != nil && entry.Candidate != nil {
				v := *entry.Candidate - *entry.Baseline
				entry.Delta = &v
			}
			rng := rand.New(rand.NewPCG(151, 4))
			values := []float64{}
			for range 1000 {
				weights := map[string]int{}
				for range len(families) {
					weights[families[rng.IntN(len(families))]]++
				}
				bs := qualityMetrics(combineStage4Families(cached, weights, "baseline/"+population))[name].Value
				cs := qualityMetrics(combineStage4Families(cached, weights, "candidate/"+population))[name].Value
				if bs != nil && cs != nil {
					values = append(values, *cs-*bs)
				}
			}
			if len(values) == 1000 {
				slices.Sort(values)
				lo, hi := values[24], values[974]
				entry.IntervalLow = &lo
				entry.IntervalHigh = &hi
			} else {
				entry.Uncertainty += " Interval unavailable because some resamples have a zero denominator; no perfect-precision value is invented."
			}
			deltas = append(deltas, entry)
		}
	}
	return summaries, deltas
}

func stage4Quality(p Stage4ReleasePlan, runs []Stage4ScoredRun, role, population string, weights map[string]int) Stage4QualitySummary {
	cases := map[string]Stage4Case{}
	for _, c := range p.Cases {
		cases[c.ID] = c
	}
	proposals, useful, required, matched, identity, temporal, source, unwanted, failures, attempts := 0, 0, 0, 0, 0, 0, 0, 0, 0, 0
	for _, run := range runs {
		if run.Role != role {
			continue
		}
		c := cases[run.CaseID]
		weight := 1
		if weights != nil {
			weight = weights[c.NarrativeFamily]
		}
		if weight == 0 {
			continue
		}
		required += c.RequiredMemories * weight
		attempts += weight
		if run.Status != "ok" {
			failures += weight
		}
		matches := map[int]bool{}
		for _, q := range run.Proposals {
			if population == "retained" && !q.Retained {
				continue
			}
			proposals += weight
			if q.Label == "required_useful" || q.Label == "optional_useful" {
				useful += weight
			}
			if q.RequiredMatch != nil {
				matches[*q.RequiredMatch] = true
			}
			if q.Label == "unwanted_but_true" {
				unwanted += weight
			}
			if slices.Contains(q.Errors, "identity") {
				identity += weight
			}
			if slices.Contains(q.Errors, "temporal") {
				temporal += weight
			}
			if slices.Contains(q.Errors, "source_attribution") {
				source += weight
			}
		}
		matched += len(matches) * weight
	}
	return Stage4QualitySummary{Role: role, Population: population, Proposals: proposals, SupportedUseful: stage4Rate(useful, proposals), RequiredRecall: stage4Rate(matched, required), IdentityErrors: stage4Rate(identity, proposals), TemporalErrors: stage4Rate(temporal, proposals), SourceErrors: stage4Rate(source, proposals), Unwanted: stage4Rate(unwanted, proposals), Failures: stage4Rate(failures, attempts)}
}

var stage4QualityNames = []string{"supported_useful_precision", "required_memory_recall", "identity_errors", "temporal_errors", "source_attribution_errors", "unwanted_proposals", "failed_runs"}

func qualityMetrics(s Stage4QualitySummary) map[string]Stage4Rate {
	return map[string]Stage4Rate{"supported_useful_precision": s.SupportedUseful, "required_memory_recall": s.RequiredRecall, "identity_errors": s.IdentityErrors, "temporal_errors": s.TemporalErrors, "source_attribution_errors": s.SourceErrors, "unwanted_proposals": s.Unwanted, "failed_runs": s.Failures}
}

func combineStage4Families(cached map[string]map[string]Stage4QualitySummary, weights map[string]int, key string) Stage4QualitySummary {
	var result Stage4QualitySummary
	add := func(target *Stage4Rate, source Stage4Rate, weight int) {
		*target = stage4Rate(target.Numerator+source.Numerator*weight, target.Denominator+source.Denominator*weight)
	}
	for family, weight := range weights {
		s := cached[family][key]
		result.Proposals += s.Proposals * weight
		add(&result.SupportedUseful, s.SupportedUseful, weight)
		add(&result.RequiredRecall, s.RequiredRecall, weight)
		add(&result.IdentityErrors, s.IdentityErrors, weight)
		add(&result.TemporalErrors, s.TemporalErrors, weight)
		add(&result.SourceErrors, s.SourceErrors, weight)
		add(&result.Unwanted, s.Unwanted, weight)
		add(&result.Failures, s.Failures, weight)
	}
	return result
}
