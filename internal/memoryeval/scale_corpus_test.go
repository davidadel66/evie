package memoryeval

import (
	"slices"
	"strings"
	"testing"
	"unicode"
)

func TestScaleCorpusIsDeterministicValidAndRealisticallySized(t *testing.T) {
	options := DefaultScaleCorpusOptions()
	first, second := GenerateScaleCorpus(options), GenerateScaleCorpus(options)
	if first.Digest() != second.Digest() {
		t.Fatal("the same seed produced different corpora")
	}
	options.Seed++
	if GenerateScaleCorpus(options).Digest() == first.Digest() {
		t.Fatal("a different seed produced the same corpus")
	}
	for _, corpus := range []ScaleCorpus{first, GenerateScaleCorpus(LargeScaleCorpusOptions())} {
		if err := corpus.Validate(); err != nil {
			t.Fatalf("%s corpus invalid: %v", corpus.Options.Tier, err)
		}
		stats := ScaleCorpusStatistics(corpus)
		if stats.OwnerMessages < 500 || stats.OwnerMessages > 5000 || stats.AssistantMessages < stats.OwnerMessages/2 {
			t.Fatalf("%s corpus is not shaped like long-running history: %+v", corpus.Options.Tier, stats)
		}
		if stats.ProjectSessions == 0 || stats.Areas < 2 || stats.PrivateMessages == 0 || stats.LowContent == 0 {
			t.Fatalf("%s corpus lacks areas, private or low-content history: %+v", corpus.Options.Tier, stats)
		}
		if stats.Corrections < 2 || stats.Retirements < 2 || stats.AcceptedClaims < 10 {
			t.Fatalf("%s corpus lacks lifecycle history: %+v", corpus.Options.Tier, stats)
		}
	}
	families := map[string]bool{}
	for _, probe := range first.Probes {
		families[probe.Family+"/"+probe.Path] = true
	}
	for _, want := range []string{"relevant/automatic", "one_word_answer/automatic", "low_content/automatic", "privacy/automatic", "stale/automatic", "stale/memory_search", "relevant/memory_search_conversations"} {
		if !families[want] {
			t.Fatalf("default corpus lacks %s probes", want)
		}
	}
}

func TestScaleCorpusRejectsUnknownLabels(t *testing.T) {
	corpus := GenerateScaleCorpus(DefaultScaleCorpusOptions())
	corpus.Probes[0].Required = []string{"n.missing"}
	if err := corpus.Validate(); err == nil || !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("typo in a required key was accepted: %v", err)
	}
}

// Dense coverage is only measurable if lexical search cannot find the target:
// no probe query word may occur in any generated message.
func TestScaleDenseProbeQueriesHaveNoLexicalOverlap(t *testing.T) {
	corpus := GenerateScaleCorpus(LargeScaleCorpusOptions())
	words := map[string]bool{}
	split := func(text string) []string {
		return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	}
	for _, message := range corpus.Messages() {
		for _, word := range split(message.Text) {
			words[word] = true
		}
	}
	dense := 0
	for _, probe := range corpus.Probes {
		if probe.Family != ScaleFamilyDenseCoverage {
			continue
		}
		dense++
		for _, word := range split(probe.Query) {
			if words[word] {
				t.Fatalf("dense probe %s query word %q also appears in history", probe.ID, word)
			}
		}
	}
	if dense != LargeScaleCorpusOptions().DenseTargets {
		t.Fatalf("dense probes = %d", dense)
	}
}

func scaleTestProbe(t *testing.T, c ScaleCorpus, id string) ScaleProbe {
	t.Helper()
	for _, probe := range c.Probes {
		if probe.ID == id {
			return probe
		}
	}
	t.Fatalf("missing probe %s", id)
	return ScaleProbe{}
}

func TestScaleScoreClassifiesItemsAndStaleOutcomes(t *testing.T) {
	c := GenerateScaleCorpus(DefaultScaleCorpusOptions())
	global := ScaleAreaGlobal
	observations := []ScaleObservation{
		{ProbeID: "auto.tyres", Status: "success", Items: []ScaleItem{
			{Key: "n.tyres", Kind: "conversation_excerpt", Area: global},                       // required
			{Key: "n.pressure", Kind: "conversation_excerpt", Area: global},                    // private, off-topic
			{Key: "n.greenhouse_app", Kind: "conversation_excerpt", Area: "project:gardenapp"}, // scope leak
		}},
		{ProbeID: "auto.low.thanks", Status: "success", Items: []ScaleItem{
			{Key: ScaleSessionKeyPrefix + "user_message.0", Kind: "conversation_excerpt", Area: global},
			{Key: "", Kind: "conversation_excerpt", Area: global},
		}},
		{ProbeID: "auto.work", Status: "success", Items: []ScaleItem{
			{Key: "m.c.employer", Kind: "conversation_excerpt", Area: global, Status: "active", CurrentStatus: "active"},
		}},
		{ProbeID: "memory_search.work", Status: "success", Items: []ScaleItem{
			{Key: "c.employer.new", Kind: "accepted_memory", Area: global, Status: "active", CurrentStatus: "active"},
			{Key: "c.employer", Kind: "accepted_memory", Area: global, Status: "active", CurrentStatus: "superseded"},
		}},
		{ProbeID: "auto.dentist", Status: "success", Items: []ScaleItem{
			{Key: "c.dentist", Kind: "accepted_memory", Area: global},
			{Key: "c.dentist.drift", Kind: "accepted_memory", Area: global},
		}},
		{ProbeID: "memory_search.dentist", Status: "success", Items: []ScaleItem{
			{Key: "c.dentist", Kind: "accepted_memory", Area: global, ConflictKeys: []string{"c.dentist.drift"}},
			{Key: "c.dentist.drift", Kind: "accepted_memory", Area: global},
		}},
	}
	report := ScoreScale(c, observations)
	if report.ScopeLeaks != 1 || report.Unmapped != 1 {
		t.Fatalf("boundary counts = leaks %d unmapped %d", report.ScopeLeaks, report.Unmapped)
	}
	auto := report.Paths[ScalePathAutomatic]
	if auto.Items != 8 || auto.Required != 3 || auto.Tolerated != 1 || auto.Unwanted != 4 || auto.Private != 1 || auto.SameSession != 1 || auto.RequiredTotal != 4 || auto.RequiredFound != 3 {
		t.Fatalf("automatic counts = %+v", auto)
	}
	if auto.Precision == nil || *auto.Precision != 0.5 || auto.Recall == nil || *auto.Recall != 0.75 {
		t.Fatalf("automatic rates = precision %v recall %v", auto.Precision, auto.Recall)
	}
	outcomes := map[string]string{}
	for _, o := range report.Stale.Outcomes {
		outcomes[o.ProbeID+"/"+o.Scenario] = o.Outcome
	}
	for key, want := range map[string]string{
		"auto.work/" + ScaleStaleCorrection:             "stale_as_current",
		"memory_search.work/" + ScaleStaleCorrection:    "labelled",
		"auto.dentist/" + ScaleStaleLabelDrift:          "both_unlinked",
		"memory_search.dentist/" + ScaleStaleLabelDrift: "linked",
	} {
		if outcomes[key] != want {
			t.Fatalf("%s outcome = %q, want %q", key, outcomes[key], want)
		}
	}
	if report.Stale.Misses != 2 || report.Stale.Checks != 4 {
		t.Fatalf("stale summary = %+v", report.Stale)
	}
	for _, target := range report.Targets {
		if target.ID == "M1.low_content_injects_nothing_unrelated" && target.Met {
			t.Fatal("unwanted low-content injection met its target")
		}
	}
	probe := scaleTestProbe(t, c, "auto.tyres")
	for _, result := range report.Probes {
		if result.ID == probe.ID && !slices.Contains(result.Items, "n.pressure(private)=unwanted") {
			t.Fatalf("private off-topic item not labelled: %v", result.Items)
		}
	}
}

// The M7 target accepts "found or reported as truncated": a missed dense
// target counts only when its outcome carried the dense scan gap, which is
// distinct from "partial" (pending index work also produces that).
func TestScaleScoreCountsDenseScanGapAsReportedTruncation(t *testing.T) {
	c := GenerateScaleCorpus(LargeScaleCorpusOptions())
	var dense []ScaleProbe
	for _, probe := range c.Probes {
		if probe.Family == ScaleFamilyDenseCoverage {
			dense = append(dense, probe)
		}
	}
	observe := func(missed string, gap bool) []ScaleObservation {
		var observations []ScaleObservation
		for _, probe := range dense {
			observation := ScaleObservation{ProbeID: probe.ID, Status: "partial", ToolPartial: true, Items: []ScaleItem{}}
			if probe.ID != missed {
				observation.Items = append(observation.Items, ScaleItem{Key: probe.Required[0], Kind: "conversation_excerpt", Area: ScaleAreaGlobal})
			} else if gap {
				observation.Gaps = []string{"dense_scan_budget"}
			}
			observations = append(observations, observation)
		}
		return observations
	}
	m7 := func(report ScaleReport) ScaleTarget {
		for _, target := range report.Targets {
			if target.ID == "M7.dense_covers_all_vectors" {
				return target
			}
		}
		t.Fatal("no M7 target")
		return ScaleTarget{}
	}
	reported := ScoreScale(c, observe(dense[3].ID, true))
	if reported.Dense.TargetsFound != len(dense)-1 || reported.Dense.MissedWithScanGap != 1 || reported.Dense.ScanGapOutcomes != 1 || !m7(reported).Met {
		t.Fatalf("a gap-reported miss did not satisfy M7: %+v %+v", reported.Dense, m7(reported))
	}
	silent := ScoreScale(c, observe(dense[3].ID, false))
	if silent.Dense.MissedWithScanGap != 0 || m7(silent).Met {
		t.Fatalf("a silent miss (partial only) satisfied M7: %+v %+v", silent.Dense, m7(silent))
	}
}

func TestScaleScoreReportsUndefinedRatesAsNull(t *testing.T) {
	c := GenerateScaleCorpus(DefaultScaleCorpusOptions())
	report := ScoreScale(c, []ScaleObservation{{ProbeID: "auto.unrelated.math", Status: "empty", Items: []ScaleItem{}}})
	counts := report.Paths[ScalePathAutomatic]
	if counts.Precision != nil || counts.UnwantedRate != nil || counts.Recall != nil {
		t.Fatalf("empty denominators invented rates: %+v", counts)
	}
}

// Stage 13 teaches the scorer the Kernel's historical-claim link: an excerpt
// linked to a retired or superseded Claim is not presented as current, and a
// ClearKeys pair is over-linked only when the message arrives linked to that
// Claim (by relation, conflict or historical link).
func TestScaleScoreHistoricalLinksAndOverLinking(t *testing.T) {
	c := GenerateScaleCorpus(DefaultScaleCorpusOptions())
	global := ScaleAreaGlobal
	observations := []ScaleObservation{
		{ProbeID: "auto.work", Status: "success", Items: []ScaleItem{
			{Key: "m.c.employer", Kind: "conversation_excerpt", Area: global, Status: "superseded", CurrentStatus: "superseded", HistoricalKeys: []string{"c.employer", "c.employer.new"}},
		}},
		{ProbeID: "auto.coffee", Status: "success", Items: []ScaleItem{
			{Key: "m.coffee.restated", Kind: "conversation_excerpt", Area: global, Status: "active", CurrentStatus: "active", HistoricalKeys: []string{"c.coffee"}},
		}},
		{ProbeID: "memory_search_conversations.blue_bottle", Status: "success", Items: []ScaleItem{
			{Key: "m.coffee.restated", Kind: "conversation_excerpt", Area: global, HistoricalKeys: []string{"c.coffee"}},
			{Key: "m.coffee.airport", Kind: "conversation_excerpt", Area: global, HistoricalKeys: []string{"c.coffee"}},
		}},
		{ProbeID: "auto.blue_bottle", Status: "success", Items: []ScaleItem{
			{Key: "m.coffee.airport", Kind: "conversation_excerpt", Area: global},
		}},
		{ProbeID: "memory_search.home_city", Status: "success", Items: []ScaleItem{
			{Key: "c.home", Kind: "accepted_memory", Area: global},
			{Key: "m.boston.marathon", Kind: "conversation_excerpt", Area: global, RelatedKeys: []string{"c.home"}},
		}},
		{ProbeID: "memory_search.carrier", Status: "success", Items: []ScaleItem{
			{Key: "c.carrier", Kind: "accepted_memory", Area: global},
			{Key: "m.carrier.switch", Kind: "conversation_excerpt", Area: global, RelatedKeys: []string{"c.carrier"}},
		}},
	}
	report := ScoreScale(c, observations)
	outcomes := map[string]string{}
	for _, o := range report.Stale.Outcomes {
		key := o.ProbeID + "/" + o.Scenario
		if prior, ok := outcomes[key]; ok {
			key += "/" + prior
		}
		outcomes[key] = o.Outcome
	}
	for key, want := range map[string]string{
		"auto.work/" + ScaleStaleCorrection:                                   "labelled",
		"auto.coffee/" + ScaleStaleRetiredRestated:                            "labelled",
		"memory_search_conversations.blue_bottle/" + ScaleStaleOverFlag:       "over_linked",
		"auto.blue_bottle/" + ScaleStaleOverFlag:                              "clear",
		"memory_search.carrier/" + ScaleStaleNewerSavedValue:                  "linked",
		"memory_search.carrier/" + ScaleStaleOverLink:                         "absent",
		"memory_search.home_city/" + ScaleStaleOverLink:                       "over_linked",
		"memory_search.home_city/" + ScaleStaleOverLink + "/" + "over_linked": "absent",
	} {
		if outcomes[key] != want {
			t.Fatalf("%s outcome = %q, want %q (all: %v)", key, outcomes[key], want, outcomes)
		}
	}
	targets := map[string]ScaleTarget{}
	for _, target := range report.Targets {
		targets[target.ID] = target
	}
	if targets["M3.no_over_linking"].Met || targets["M2.no_over_flagging"].Met {
		t.Fatalf("over-linking met its precision targets: %+v", targets)
	}
	// home_city carries two different-wording checks (corpus v3 adds the
	// update that names the saved value) and carrier one.
	if targets["M3.different_wording_detected"].Observed != "2 misses of 3 checks" {
		t.Fatalf("M3 target does not cover every different-wording scenario: %+v", targets["M3.different_wording_detected"])
	}
	for _, probe := range c.Probes {
		for _, check := range probe.Checks {
			if check.Scenario == ScaleStaleOverLink && probe.Path == ScalePathConversationSearch {
				t.Fatalf("conversation search cannot link Claims, but %s carries an over-link check", probe.ID)
			}
		}
	}
}
