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
	for _, want := range []string{"relevant/automatic", "low_content/automatic", "privacy/automatic", "stale/automatic", "stale/memory_search", "relevant/memory_search_conversations"} {
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

func TestScaleScoreReportsUndefinedRatesAsNull(t *testing.T) {
	c := GenerateScaleCorpus(DefaultScaleCorpusOptions())
	report := ScoreScale(c, []ScaleObservation{{ProbeID: "auto.unrelated.math", Status: "empty", Items: []ScaleItem{}}})
	counts := report.Paths[ScalePathAutomatic]
	if counts.Precision != nil || counts.UnwantedRate != nil || counts.Recall != nil {
		t.Fatalf("empty denominators invented rates: %+v", counts)
	}
}
