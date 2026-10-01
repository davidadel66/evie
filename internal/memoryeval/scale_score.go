package memoryeval

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
)

const ScaleReportVersion = 1

const (
	ScaleClassRequired  = "required"
	ScaleClassTolerated = "tolerated"
	ScaleClassUnwanted  = "unwanted"
)

// ScaleItem is one piece of evidence delivered to the answering model,
// translated from runtime IDs back to corpus keys by the replay driver.
type ScaleItem struct {
	Key           string   `json:"key"`
	Kind          string   `json:"kind"`
	Area          string   `json:"area"`
	Intent        string   `json:"intent,omitempty"`
	Status        string   `json:"status,omitempty"`
	CurrentStatus string   `json:"current_status,omitempty"`
	Paths         []string `json:"paths,omitempty"`
	RelatedKeys   []string `json:"related_keys,omitempty"`
	ConflictKeys  []string `json:"conflict_keys,omitempty"`
}

// ScaleObservation is what one probe actually delivered. Status is the
// EVIE_MEMORY_DATA status; ToolStatus and ToolPartial come from the
// model-visible outcome of a scripted read tool.
type ScaleObservation struct {
	ProbeID     string      `json:"probe_id"`
	Status      string      `json:"status"`
	Items       []ScaleItem `json:"items"`
	ToolStatus  []string    `json:"tool_status,omitempty"`
	ToolPartial bool        `json:"tool_partial,omitempty"`
}

type ScaleCorpusStats struct {
	Sessions          int `json:"sessions"`
	GlobalSessions    int `json:"global_sessions"`
	ProjectSessions   int `json:"project_sessions"`
	OwnerMessages     int `json:"owner_messages"`
	AssistantMessages int `json:"assistant_messages"`
	LowContent        int `json:"low_content_owner_messages"`
	PrivateMessages   int `json:"private_messages"`
	Needles           int `json:"needles"`
	AcceptedClaims    int `json:"accepted_claims"`
	Corrections       int `json:"corrections"`
	Retirements       int `json:"retirements"`
	Areas             int `json:"areas"`
	Topics            int `json:"topics"`
	Probes            int `json:"probes"`
}

// ScaleCounts aggregates delivered items. Precision counts required and
// on-topic tolerated items as wanted; recall counts unique required keys.
type ScaleCounts struct {
	Probes             int      `json:"probes"`
	ProbesWithItems    int      `json:"probes_with_items"`
	ProbesWithUnwanted int      `json:"probes_with_unwanted"`
	Items              int      `json:"items"`
	Required           int      `json:"required_items"`
	Tolerated          int      `json:"tolerated_items"`
	Unwanted           int      `json:"unwanted_items"`
	Private            int      `json:"unwanted_private_items"`
	SameSession        int      `json:"unwanted_same_session_items"`
	RequiredTotal      int      `json:"required_total"`
	RequiredFound      int      `json:"required_found"`
	Precision          *float64 `json:"precision"`
	UnwantedRate       *float64 `json:"unwanted_rate"`
	Recall             *float64 `json:"recall"`
}

type ScaleFamilyMetrics struct {
	Path   string `json:"path"`
	Family string `json:"family"`
	ScaleCounts
}

type ScaleStaleOutcome struct {
	ProbeID  string `json:"probe_id"`
	Path     string `json:"path"`
	Scenario string `json:"scenario"`
	Issue    string `json:"issue"`
	Control  bool   `json:"control,omitempty"`
	Outcome  string `json:"outcome"`
	Miss     bool   `json:"miss"`
}

type ScaleStaleSummary struct {
	Checks          int                 `json:"checks"`
	Misses          int                 `json:"misses"`
	ControlChecks   int                 `json:"control_checks"`
	ControlFailures int                 `json:"control_failures"`
	Outcomes        []ScaleStaleOutcome `json:"outcomes"`
}

// ScaleDenseMetrics is filled by the large-tier driver from the database it
// built; the scorer only counts the dense-coverage probe outcomes.
// PartialOutcomes counts model-visible "partial" outcomes. It is not a
// scan-truncation signal: the probe's own just-appended message leaves dense
// work pending, which already makes every dense-enabled search partial.
type ScaleDenseMetrics struct {
	VectorsInScope    int      `json:"vectors_in_global_scope"`
	ScanLimit         int      `json:"scan_limit"`
	ReachableFraction *float64 `json:"reachable_fraction"`
	Targets           int      `json:"targets"`
	TargetsFound      int      `json:"targets_found"`
	TargetsReachable  int      `json:"targets_reachable"`
	ReachableFound    int      `json:"reachable_targets_found"`
	UnreachableFound  int      `json:"unreachable_targets_found"`
	PartialOutcomes   int      `json:"partial_outcomes"`
}

type ScaleProbeResult struct {
	ID              string   `json:"id"`
	Family          string   `json:"family"`
	Path            string   `json:"path"`
	Status          string   `json:"status"`
	Items           []string `json:"items"`
	RequiredMissing []string `json:"required_missing,omitempty"`
}

// ScaleTarget is an acceptance criterion from the harness-review plan. Met is
// computed, never asserted: an unmet target is recorded baseline behavior.
type ScaleTarget struct {
	ID          string `json:"id"`
	Issue       string `json:"issue"`
	Description string `json:"description"`
	Met         bool   `json:"met"`
	Observed    string `json:"observed"`
}

type ScaleReport struct {
	ReportVersion int                    `json:"report_version"`
	CorpusVersion string                 `json:"corpus_version"`
	CorpusSHA256  string                 `json:"corpus_sha256"`
	Options       ScaleCorpusOptions     `json:"options"`
	Corpus        ScaleCorpusStats       `json:"corpus"`
	Paths         map[string]ScaleCounts `json:"paths"`
	Families      []ScaleFamilyMetrics   `json:"families"`
	Stale         ScaleStaleSummary      `json:"stale"`
	ScopeLeaks    int                    `json:"scope_leaks"`
	Unmapped      int                    `json:"unmapped_items"`
	Dense         *ScaleDenseMetrics     `json:"dense,omitempty"`
	Targets       []ScaleTarget          `json:"targets"`
	Probes        []ScaleProbeResult     `json:"probes"`
}

func ScaleCorpusStatistics(c ScaleCorpus) ScaleCorpusStats {
	stats := ScaleCorpusStats{Areas: len(c.Areas), Probes: len(c.Probes)}
	topics := map[string]bool{}
	for _, session := range c.Sessions {
		stats.Sessions++
		if session.Area == ScaleAreaGlobal {
			stats.GlobalSessions++
		} else {
			stats.ProjectSessions++
		}
		for _, step := range session.Steps {
			stats.OwnerMessages++
			topics[step.Owner.Topic] = true
			if step.Owner.Topic == "smalltalk" {
				stats.LowContent++
			}
			if step.Owner.Private {
				stats.PrivateMessages++
			}
			if strings.HasPrefix(step.Owner.Key, "n.") {
				stats.Needles++
			}
			if step.Assistant != nil {
				stats.AssistantMessages++
				if step.Assistant.Private {
					stats.PrivateMessages++
				}
			}
			switch step.Kind {
			case ScaleStepRemember:
				stats.AcceptedClaims++
			case ScaleStepCorrect:
				stats.AcceptedClaims++
				stats.Corrections++
			case ScaleStepRetire:
				stats.Retirements++
			}
		}
	}
	stats.Topics = len(topics)
	return stats
}

// ScoreScale classifies every delivered item against the corpus labels. It is
// a pure function of the corpus and observations so it can be unit tested and
// re-run over retained observations.
func ScoreScale(c ScaleCorpus, observations []ScaleObservation) ScaleReport {
	report := ScaleReport{ReportVersion: ScaleReportVersion, CorpusVersion: c.Version, CorpusSHA256: c.Digest(), Options: c.Options,
		Corpus: ScaleCorpusStatistics(c), Paths: map[string]ScaleCounts{}}
	messages := c.Messages()
	byProbe := map[string]ScaleObservation{}
	for _, observation := range observations {
		byProbe[observation.ProbeID] = observation
	}
	type familyKey struct{ path, family string }
	families := map[familyKey]*ScaleCounts{}
	var familyOrder []familyKey
	for _, probe := range c.Probes {
		observation, ok := byProbe[probe.ID]
		if !ok {
			continue
		}
		key := familyKey{probe.Path, probe.Family}
		if families[key] == nil {
			families[key] = &ScaleCounts{}
			familyOrder = append(familyOrder, key)
		}
		pathCounts := report.Paths[probe.Path]
		result := ScaleProbeResult{ID: probe.ID, Family: probe.Family, Path: probe.Path, Status: observation.Status, Items: []string{}}
		found := map[string]bool{}
		unwanted := false
		var probeCounts ScaleCounts
		for _, item := range observation.Items {
			class := ScaleClassUnwanted
			topic, private := scaleItemLabels(c, messages, item.Key)
			switch {
			case !scaleAreaAllowed(probe.Area, item):
				// Out-of-scope evidence never earns credit, even if it is a target.
				report.ScopeLeaks++
			case item.Key == "":
				report.Unmapped++
			case strings.HasPrefix(item.Key, ScaleSessionKeyPrefix):
				probeCounts.SameSession++
			case slices.Contains(probe.Required, item.Key):
				class = ScaleClassRequired
				found[item.Key] = true
			case slices.Contains(probe.Topics, topic):
				class = ScaleClassTolerated
			}
			probeCounts.Items++
			switch class {
			case ScaleClassRequired:
				probeCounts.Required++
			case ScaleClassTolerated:
				probeCounts.Tolerated++
			default:
				probeCounts.Unwanted++
				unwanted = true
				if private {
					probeCounts.Private++
				}
			}
			label := item.Key
			if label == "" {
				label = "unmapped"
			}
			if private {
				label += "(private)"
			}
			result.Items = append(result.Items, label+"="+class)
		}
		// Rank order is not measured, and ties between equal-scoring Claims break
		// on random IDs; a sorted list keeps the baseline stable across
		// unrelated changes to how many IDs a run draws.
		sort.Strings(result.Items)
		probeCounts.Probes = 1
		if len(observation.Items) > 0 {
			probeCounts.ProbesWithItems = 1
		}
		if unwanted {
			probeCounts.ProbesWithUnwanted = 1
		}
		probeCounts.RequiredTotal = len(probe.Required)
		for _, key := range probe.Required {
			if found[key] {
				probeCounts.RequiredFound++
			} else {
				result.RequiredMissing = append(result.RequiredMissing, key)
			}
		}
		pathCounts.add(probeCounts)
		report.Paths[probe.Path] = pathCounts
		families[key].add(probeCounts)
		for _, check := range probe.Checks {
			outcome := ScaleStaleOutcome{ProbeID: probe.ID, Path: probe.Path, Scenario: check.Scenario, Issue: check.Issue, Control: check.Control}
			outcome.Outcome, outcome.Miss = scaleCheck(check, observation.Items)
			if check.Control {
				report.Stale.ControlChecks++
				if outcome.Miss {
					report.Stale.ControlFailures++
				}
			} else {
				report.Stale.Checks++
				if outcome.Miss {
					report.Stale.Misses++
				}
			}
			report.Stale.Outcomes = append(report.Stale.Outcomes, outcome)
		}
		report.Probes = append(report.Probes, result)
	}
	for path, counts := range report.Paths {
		counts.finish()
		report.Paths[path] = counts
	}
	for _, key := range familyOrder {
		counts := *families[key]
		counts.finish()
		report.Families = append(report.Families, ScaleFamilyMetrics{Path: key.path, Family: key.family, ScaleCounts: counts})
	}
	report.Dense = scaleDenseOutcomes(c, byProbe)
	report.Targets = ScaleTargetsFor(report)
	return report
}

func scaleItemLabels(c ScaleCorpus, messages map[string]ScaleMessage, key string) (string, bool) {
	if message, ok := messages[key]; ok {
		return message.Topic, message.Private
	}
	if claim, ok := c.Claim(key); ok {
		return claim.Topic, false
	}
	return "", false
}

// Conversation evidence must come from the probe's own Context Scope. Accepted
// Claims may additionally come from Global memory.
func scaleAreaAllowed(probeArea string, item ScaleItem) bool {
	if item.Area == probeArea {
		return true
	}
	return item.Kind == "accepted_memory" && item.Area == ScaleAreaGlobal
}

func (c *ScaleCounts) add(other ScaleCounts) {
	c.Probes += other.Probes
	c.ProbesWithItems += other.ProbesWithItems
	c.ProbesWithUnwanted += other.ProbesWithUnwanted
	c.Items += other.Items
	c.Required += other.Required
	c.Tolerated += other.Tolerated
	c.Unwanted += other.Unwanted
	c.Private += other.Private
	c.SameSession += other.SameSession
	c.RequiredTotal += other.RequiredTotal
	c.RequiredFound += other.RequiredFound
}

func (c *ScaleCounts) finish() {
	c.Precision = scaleRatio(c.Required+c.Tolerated, c.Items)
	c.UnwantedRate = scaleRatio(c.Unwanted, c.Items)
	c.Recall = scaleRatio(c.RequiredFound, c.RequiredTotal)
}

func scaleRatio(numerator, denominator int) *float64 {
	if denominator == 0 {
		return nil
	}
	value := math.Round(float64(numerator)/float64(denominator)*10000) / 10000
	return &value
}

// scalePresentedAsCurrent is evidence the model would read as a current,
// unqualified owner fact: no historical intent, no non-active status, and no
// relation or conflict marking it against other memory.
func scalePresentedAsCurrent(item ScaleItem) bool {
	active := func(status string) bool { return status == "" || status == "active" }
	return item.Intent != "historical" && active(item.Status) && active(item.CurrentStatus) && len(item.RelatedKeys) == 0 && len(item.ConflictKeys) == 0
}

func scaleCheck(check ScaleStaleCheck, items []ScaleItem) (string, bool) {
	if len(check.StaleKeys) > 0 {
		outcome := "absent"
		for _, item := range items {
			if !slices.Contains(check.StaleKeys, item.Key) {
				continue
			}
			if scalePresentedAsCurrent(item) {
				return "stale_as_current", true
			}
			outcome = "labelled"
		}
		return outcome, false
	}
	first, second := check.LinkKeys[0], check.LinkKeys[1]
	var haveFirst, haveSecond bool
	for _, item := range items {
		other := ""
		switch item.Key {
		case first:
			haveFirst, other = true, second
		case second:
			haveSecond, other = true, first
		default:
			continue
		}
		if slices.Contains(item.RelatedKeys, other) || slices.Contains(item.ConflictKeys, other) {
			return "linked", false
		}
	}
	switch {
	case haveFirst && haveSecond:
		return "both_unlinked", true
	case haveFirst:
		return "only_first", true
	case haveSecond:
		return "only_second", true
	}
	return "absent", true
}

func scaleDenseOutcomes(c ScaleCorpus, byProbe map[string]ScaleObservation) *ScaleDenseMetrics {
	var dense *ScaleDenseMetrics
	for _, probe := range c.Probes {
		if probe.Family != ScaleFamilyDenseCoverage {
			continue
		}
		observation, ok := byProbe[probe.ID]
		if !ok {
			continue
		}
		if dense == nil {
			dense = &ScaleDenseMetrics{}
		}
		dense.Targets++
		for _, item := range observation.Items {
			if slices.Contains(probe.Required, item.Key) {
				dense.TargetsFound++
				break
			}
		}
		if observation.ToolPartial {
			dense.PartialOutcomes++
		}
	}
	return dense
}

// ScaleTargetsFor evaluates the plan acceptance criteria against a report.
func ScaleTargetsFor(r ScaleReport) []ScaleTarget {
	family := func(path, name string) ScaleCounts {
		for _, f := range r.Families {
			if f.Path == path && f.Family == name {
				return f.ScaleCounts
			}
		}
		return ScaleCounts{}
	}
	scenario := func(names ...string) (checks, misses int) {
		for _, outcome := range r.Stale.Outcomes {
			if slices.Contains(names, outcome.Scenario) && !outcome.Control {
				checks++
				if outcome.Miss {
					misses++
				}
			}
		}
		return checks, misses
	}
	low := family(ScalePathAutomatic, ScaleFamilyLowContent)
	privacy := family(ScalePathAutomatic, ScaleFamilyPrivacy)
	targets := []ScaleTarget{
		{ID: "M1.low_content_injects_nothing_unrelated", Issue: "M1", Description: "low-content follow-ups such as \"thanks!\" inject no unwanted items",
			Met: low.Probes > 0 && low.Unwanted == 0, Observed: fmt.Sprintf("%d unwanted of %d injected items across %d probes", low.Unwanted, low.Items, low.Probes)},
		{ID: "M1.privacy_injects_no_private_item", Issue: "M1", Description: "unrelated requests sharing a word with private history inject no private item",
			Met: privacy.Probes > 0 && privacy.Private == 0, Observed: fmt.Sprintf("%d private items across %d probes", privacy.Private, privacy.Probes)},
	}
	for _, t := range []struct {
		id, issue, description string
		scenarios              []string
	}{
		{"M2.corrected_source_not_current", "M2", "after a correction the old source is not delivered as an unqualified current statement", []string{ScaleStaleCorrection}},
		{"M2.retired_restatement_flagged", "M2", "a retired fact restated without a source link, before or after retirement, is not delivered as unflagged current evidence", []string{ScaleStaleRetiredRestated, ScaleStaleRetiredRepeat}},
		{"M3.different_wording_detected", "M3", "a later owner statement with different wording is linked to the saved claim it contradicts", []string{ScaleStaleNewerWording}},
		{"M4.drift_conflicts_warned", "M4", "predicate label or cardinality drift still produces a conflict warning", []string{ScaleStaleLabelDrift, ScaleStaleCardinalityDrift}},
	} {
		checks, misses := scenario(t.scenarios...)
		targets = append(targets, ScaleTarget{ID: t.id, Issue: t.issue, Description: t.description, Met: checks > 0 && misses == 0, Observed: fmt.Sprintf("%d misses of %d checks", misses, checks)})
	}
	if r.Dense != nil {
		// A truncation report would also satisfy the plan, but no outcome field
		// distinguishes scan truncation today; whichever signal Stage 12 adds
		// must be taught to the driver before this target can count it.
		targets = append(targets, ScaleTarget{ID: "M7.dense_covers_all_vectors", Issue: "M7",
			Description: "dense recall finds targets anywhere in the vector table",
			Met:         r.Dense.Targets > 0 && r.Dense.TargetsFound == r.Dense.Targets,
			Observed: fmt.Sprintf("%d of %d targets found; %d of %d vectors inside the scan bound; %d unreachable targets found", r.Dense.TargetsFound, r.Dense.Targets,
				min(r.Dense.VectorsInScope, r.Dense.ScanLimit), r.Dense.VectorsInScope, r.Dense.UnreachableFound)})
	}
	return targets
}

// Markdown is a compact human summary for test logs and the results document.
func (r ScaleReport) Markdown() string {
	var b strings.Builder
	format := func(value *float64) string {
		if value == nil {
			return "n/a"
		}
		return fmt.Sprintf("%.4f", *value)
	}
	fmt.Fprintf(&b, "Memory scale evaluation (%s, %s, seed %d)\n\n", r.CorpusVersion, r.Options.Tier, r.Options.Seed)
	fmt.Fprintf(&b, "Corpus: %d sessions (%d Global, %d project), %d owner + %d assistant messages, %d accepted claims, %d corrections, %d retirements, %d probes.\n\n",
		r.Corpus.Sessions, r.Corpus.GlobalSessions, r.Corpus.ProjectSessions, r.Corpus.OwnerMessages, r.Corpus.AssistantMessages, r.Corpus.AcceptedClaims, r.Corpus.Corrections, r.Corpus.Retirements, r.Corpus.Probes)
	b.WriteString("| Path | Family | Probes | Items | Required | Tolerated | Unwanted | Private | Same session | Precision | Unwanted rate | Recall |\n| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	paths := make([]string, 0, len(r.Paths))
	for path := range r.Paths {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	row := func(path, family string, c ScaleCounts) {
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %d | %d | %d | %d | %d | %s | %s | %d/%d (%s) |\n", path, family, c.Probes, c.Items, c.Required, c.Tolerated, c.Unwanted, c.Private, c.SameSession, format(c.Precision), format(c.UnwantedRate), c.RequiredFound, c.RequiredTotal, format(c.Recall))
	}
	for _, path := range paths {
		row(path, "all", r.Paths[path])
	}
	for _, f := range r.Families {
		row(f.Path, f.Family, f.ScaleCounts)
	}
	fmt.Fprintf(&b, "\nStale-fact checks: %d misses of %d; controls: %d failures of %d. Scope leaks: %d. Unmapped items: %d.\n\n", r.Stale.Misses, r.Stale.Checks, r.Stale.ControlFailures, r.Stale.ControlChecks, r.ScopeLeaks, r.Unmapped)
	b.WriteString("| Probe | Path | Scenario | Issue | Outcome | Miss |\n| --- | --- | --- | --- | --- | --- |\n")
	for _, o := range r.Stale.Outcomes {
		scenario := o.Scenario
		if o.Control {
			scenario += " (control)"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %t |\n", o.ProbeID, o.Path, scenario, o.Issue, o.Outcome, o.Miss)
	}
	if r.Dense != nil {
		fmt.Fprintf(&b, "\nDense: %d vectors in Global scope, scan limit %d (reachable fraction %s); %d/%d targets found (%d reachable, %d reachable found, %d unreachable found); %d partial outcomes.\n",
			r.Dense.VectorsInScope, r.Dense.ScanLimit, format(r.Dense.ReachableFraction), r.Dense.TargetsFound, r.Dense.Targets, r.Dense.TargetsReachable, r.Dense.ReachableFound, r.Dense.UnreachableFound, r.Dense.PartialOutcomes)
	}
	b.WriteString("\n| Target | Met | Observed |\n| --- | --- | --- |\n")
	for _, t := range r.Targets {
		fmt.Fprintf(&b, "| %s | %t | %s |\n", t.ID, t.Met, t.Observed)
	}
	return b.String()
}
