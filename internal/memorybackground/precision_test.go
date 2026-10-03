package memorybackground

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

// A small scale-eval-style probe of proposal precision on a fixed corpus.
// The model is the scripted fake, and its scripted output deliberately
// contains realistic mistakes (a value read from a question, a value nobody
// said, Workspace memory in a Global conversation, a secret, a
// non-canonical date, repeats, a change outside the conversation's scope).
// The probe therefore measures what the deterministic layers (strict
// validation, scope routing, similarity and dedupe, Prepare's source
// binding, the inbox's duplicate and decline checks) let through to the
// owner's inbox. Real-model precision and recall must be measured live with
// DeepSeek V4.1 Flash on the memory scale corpus; this probe cannot.
//
// Set EVIE_MEMORY_BACKGROUND_EVAL_REPORT=/path/report.json to write the
// per-case report.

type precisionCase struct {
	name    string
	session string // "global" or "workspace"
	setup   func(f *fixture, sessions map[string]memory.Session)
	message string
	extract []map[string]any
	decide  []map[string]any
	// useful lists the values whose proposals a careful owner would approve;
	// every other queued proposal counts as a false positive.
	useful []string
}

type precisionReport struct {
	Case       string   `json:"case"`
	Queued     []string `json:"queued"`
	Useful     []string `json:"useful"`
	FalsePos   []string `json:"false_positives"`
	Noops      int      `json:"noops"`
	Skipped    int      `json:"skipped"`
	Duplicates int      `json:"duplicates"`
}

func TestProposalPrecisionProbeOnAFixedCorpus(t *testing.T) {
	denied := func(item map[string]any) map[string]any { item["polarity"] = "denied"; return item }
	date := func(item map[string]any) map[string]any { item["value_type"] = "date"; return item }
	cases := []precisionCase{
		{name: "stated preference", session: "global", message: "From now on, keep your answers short.",
			extract: []map[string]any{memoryItem(0, "preference", "profile", "answer_length", "answer length", "short")}, useful: []string{"short"}},
		{name: "stated profile fact", session: "global", message: "For context, I live in Boston.",
			extract: []map[string]any{memoryItem(0, "profile_fact", "profile", "home_city", "home city", "Boston")}, useful: []string{"Boston"}},
		{name: "restated known memory", session: "global", message: "Like I said, I live in Boston.",
			setup:   func(f *fixture, s map[string]memory.Session) { approveAll(f) },
			extract: []map[string]any{memoryItem(0, "profile_fact", "profile", "home_city", "home city", "Boston")}},
		{name: "real-world change", session: "global", message: "I moved to Chicago last week.",
			extract: []map[string]any{memoryItem(0, "profile_fact", "profile", "home_city", "home city", "Chicago")},
			decide:  []map[string]any{decisionItem(0, "UPDATE", "m1", "changed")}, useful: []string{"Chicago"}},
		{name: "no longer true", session: "global", message: "Btw I'm off coffee now, for good.",
			setup: func(f *fixture, s map[string]memory.Session) {
				f.remember(s["global"], "I drink coffee daily.", "daily_drink", "daily drink", "coffee", memory.ClaimKindProfileFact)
			},
			extract: []map[string]any{denied(memoryItem(0, "profile_fact", "profile", "daily_drink", "daily drink", "coffee"))},
			decide:  []map[string]any{decisionItem(0, "INVALIDATE", "m1", "")}, useful: []string{"retire:daily drink"}},
		{name: "value read from a question", session: "global", message: "Do I even like jazz?",
			extract: []map[string]any{memoryItem(0, "preference", "profile", "music", "music", "jazz")}},
		{name: "value nobody said", session: "global", message: "Planning my week.",
			extract: []map[string]any{memoryItem(0, "profile_fact", "profile", "occupation", "occupation", "pilot")}},
		{name: "workspace memory in a global conversation", session: "global", message: "This repo uses tabs.",
			extract: []map[string]any{memoryItem(0, "fact", "workspace", "indentation", "indentation", "tabs")}},
		{name: "secret", session: "global", message: "Use this one.",
			extract: []map[string]any{memoryItem(0, "fact", "profile", "api_key", "api key", "password: correcthorsebattery")}},
		{name: "non-canonical date", session: "global", message: "My birthday is June 13.",
			extract: []map[string]any{date(memoryItem(0, "profile_fact", "profile", "birthday", "birthday", "June 13"))}},
		{name: "repeat in one batch", session: "workspace", message: "We deploy on Fridays. Again: deploys happen Fridays.",
			extract: []map[string]any{memoryItem(0, "fact", "workspace", "deploy_day", "deploy day", "Fridays"), memoryItem(0, "fact", "workspace", "deploy_day", "deploy day", "Fridays")},
			useful:  []string{"Fridays"}},
		{name: "change outside the conversation's scope", session: "workspace", message: "I relocated to Denver.",
			extract: []map[string]any{memoryItem(0, "profile_fact", "profile", "home_city", "home city", "Denver")},
			decide:  []map[string]any{decisionItem(0, "UPDATE", "m1", "changed")}},
	}

	f := newFixture(t)
	sessions := map[string]memory.Session{"global": f.globalSession(), "workspace": f.workspaceSession("Platform")}
	var reports []precisionReport
	queued, useful := 0, 0
	for _, c := range cases {
		if c.setup != nil {
			c.setup(f, sessions)
		}
		session := sessions[c.session]
		before := map[string]bool{}
		for _, item := range backgroundItems(f.inbox()) {
			before[item.ProposalID] = true
		}
		f.turn(session, c.message)
		f.model.extract(c.extract...)
		if c.decide != nil {
			f.model.decide(c.decide...)
		}
		run := f.mustRun(session)
		report := precisionReport{Case: c.name, Noops: run.Noops, Skipped: run.Skipped, Duplicates: run.Duplicates}
		for _, item := range backgroundItems(f.inbox()) {
			if before[item.ProposalID] {
				continue
			}
			label := proposalLabel(item)
			report.Queued = append(report.Queued, label)
			queued++
			if containsLabel(c.useful, label) {
				report.Useful = append(report.Useful, label)
				useful++
			} else {
				report.FalsePos = append(report.FalsePos, label)
				// What reaches the owner by mistake must say it is not the
				// owner's words.
				if item.Change == nil || !item.Change.EvieProposed {
					t.Errorf("%s: a false positive reached the inbox as the owner's words: %+v", c.name, item.Change)
				}
			}
		}
		for _, want := range c.useful {
			if !containsLabel(report.Useful, want) {
				t.Errorf("%s: the useful proposal %q was not queued (queued %v)", c.name, want, report.Queued)
			}
		}
		reports = append(reports, report)
		// Clear the inbox between cases the way the owner would: approve
		// what is useful, decline the rest.
		for _, item := range backgroundItems(f.inbox()) {
			if containsLabel(c.useful, proposalLabel(item)) {
				f.approve(item, false)
			} else if err := f.store.DeclineMemoryProposal(f.ctx, item.ProposalID); err != nil {
				t.Fatal(err)
			}
		}
	}
	precision := float64(useful) / float64(queued)
	t.Logf("proposal precision on the fixed corpus with scripted model output: %d useful of %d queued = %.2f", useful, queued, precision)
	for _, report := range reports {
		t.Logf("%-42s queued=%v false_positives=%v noops=%d skipped=%d duplicates=%d", report.Case, report.Queued, report.FalsePos, report.Noops, report.Skipped, report.Duplicates)
	}
	// 12 cases, 13 scripted candidates: 5 useful proposals reach the inbox,
	// 6 scripted mistakes are filtered deterministically, and the 2 that
	// pass (a question, an invented value) arrive labelled Evie-proposed.
	if useful != 5 || queued != 7 {
		t.Fatalf("useful %d of %d queued; the deterministic layers changed", useful, queued)
	}
	if path := os.Getenv("EVIE_MEMORY_BACKGROUND_EVAL_REPORT"); path != "" {
		encoded, _ := json.MarshalIndent(map[string]any{"precision": precision, "useful": useful, "queued": queued, "cases": reports,
			"note": "scripted model output; measures the deterministic layers only, not DeepSeek V4.1 Flash"}, "", "  ")
		if err := os.WriteFile(path, encoded, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func approveAll(f *fixture) {
	for {
		items := backgroundItems(f.inbox())
		if len(items) == 0 {
			return
		}
		f.approve(items[0], false)
	}
}

func proposalLabel(item memory.MemoryProposalItem) string {
	switch {
	case item.ToolName == memory.RetireToolName && item.Change != nil:
		label := strings.TrimPrefix(item.Change.Current, "owner — ")
		label, _, _ = strings.Cut(label, ":")
		return "retire:" + label
	default:
		var proposal struct {
			Literal          memory.TypedLiteral  `json:"literal"`
			ReplacementClaim memory.SemanticClaim `json:"replacement_claim"`
		}
		_ = json.Unmarshal([]byte(item.Proposal), &proposal)
		if proposal.ReplacementClaim.Object.Literal != nil {
			return proposal.ReplacementClaim.Object.Literal.Value
		}
		return proposal.Literal.Value
	}
}

func containsLabel(labels []string, label string) bool {
	for _, candidate := range labels {
		if candidate == label {
			return true
		}
	}
	return false
}
