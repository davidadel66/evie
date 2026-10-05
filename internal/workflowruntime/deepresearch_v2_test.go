package workflowruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/workflow"
)

// The built-in deep-research v2 (2026-10-04) runs end to end with
// scripted fakes through the generic proposal and approval path: an AI
// scope, one search worker per angle (searching only), dedup_sources@1
// under the fifteen-source budget, one extractor per source (its claims
// count only when the harness saw it fetch its source), rank_claims@1,
// three adversarial verifiers per claim, tally_adversarial@1, the AI
// report and check_citations@1. Nothing reaches the network.

func builtinDeepResearchDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "workflow", "builtin", "deep-research"))
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func builtinDeepResearchFixtures(t *testing.T) *workflow.Fixtures {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(builtinDeepResearchDir(t), "fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	return fixtures(t, string(data))
}

// deepResearchV2Chat answers the scope step with three angles and the
// writer with a report citing two verified sources and one refuted one.
func deepResearchV2Chat() *fakeChat {
	return newFakeChat(func(ctx context.Context, req openrouter.ChatRequest) (openrouter.ChatResponse, error) {
		if strings.Contains(req.Messages[0].Content, "You scope a deep-research run") {
			return reply(`{"angles":[
 {"label":"energy","query":"aviation battery energy density","rationale":"The physical limit."},
 {"label":"rules","query":"electric aircraft reserve rules","rationale":"Rules that cut range."},
 {"label":"skeptical","query":"electric aircraft range hype","rationale":"Overstated claims."}]}`, 300, 80)(ctx, req)
		}
		report := map[string]any{
			"title":   "Electric aircraft range",
			"summary": "Cell energy density limits range [S1], and reserve rules cut it further [S2].",
			"body_markdown": "## Findings\n\n### Energy density\nConfidence: high · Vote: 3-0\nAviation cells store about 250 to 300 Wh/kg [S1].\nEvidence: an independent handbook agrees.\n\n" +
				"### Reserve rules\nConfidence: low · Vote: 2-1\nA 30-minute reserve applies [S2]. A 1,000 km range is promised [S3].\nEvidence: one verifier disagreed.\n\n" +
				"## Caveats\n\nOne claim could not be verified; refuted claims were left out.\n\n## Open questions\n\n- How fast are cells improving?\n- Do hybrids help?\n",
			"citations": []any{
				map[string]any{"marker": "S1", "url": "https://example.org/evidence/cells"},
				map[string]any{"marker": "S2", "url": "https://example.org/evidence/reserves"},
				map[string]any{"marker": "S3", "url": "https://example.org/hype"},
			},
		}
		b, _ := json.Marshal(report)
		return reply(string(b), 2000, 600)(ctx, req)
	})
}

func searchHit(url, relevance string) map[string]any {
	return map[string]any{"url": url, "title": "Page " + url, "snippet": "relevant", "relevance": relevance}
}

// deepResearchV2Workers scripts every worker by its assignment data. With
// failVerifiers every verifier fails (an infrastructure failure).
func deepResearchV2Workers(failVerifiers bool) *workerModel {
	filler := func(prefix string, n int, relevance string) []any {
		var out []any
		for i := 1; i <= n; i++ {
			out = append(out, searchHit(fmt.Sprintf("https://example.org/%s%d", prefix, i), relevance))
		}
		return out
	}
	searches := map[string][]any{
		"energy": append([]any{searchHit("https://example.org/cells", "high"), searchHit("https://example.org/fuel", "high")}, filler("e", 4, "medium")...),
		// The same cell page spelled differently is a duplicate.
		"rules": append([]any{searchHit("https://www.example.org/cells/", "high"), searchHit("https://example.org/reserves", "high"),
			searchHit("https://example.org/unread", "high"), searchHit("https://example.org/broken", "high")}, filler("r", 2, "low")...),
		"skeptical": append([]any{searchHit("https://example.org/hype", "high")}, filler("k", 5, "low")...),
	}
	type extraction struct {
		quality, claim, importance string
	}
	claims := map[string]extraction{
		"https://example.org/cells":    {"primary", "Aviation cells store about 250 to 300 Wh/kg.", "central"},
		"https://example.org/reserves": {"secondary", "Electric aircraft keep a 30-minute energy reserve under VFR.", "central"},
		"https://example.org/fuel":     {"primary", "Jet fuel stores about 12,000 Wh/kg.", "supporting"},
		"https://example.org/hype":     {"blog", "Electric aircraft will fly 1,000 km by 2027.", "central"},
		"https://example.org/unread":   {"primary", "A claim from a page its extractor never read.", "central"},
	}
	return &workerModel{answer: func(_ context.Context, r openrouter.ChatRequest) (openrouter.ChatResponse, error) {
		data := dataOf(r)
		switch {
		case data["voter"] != nil:
			claim, _ := data["claim"].(string)
			voter := int(data["voter"].(float64))
			if failVerifiers {
				return finalReport("The verifier could not finish.", nil), nil
			}
			var evidence string
			refuted, confidence := false, "high"
			switch {
			case strings.Contains(claim, "250 to 300"):
				evidence = "https://example.org/evidence/cells"
			case strings.Contains(claim, "reserve"):
				evidence, confidence = "https://example.org/evidence/reserves", "medium"
				refuted = voter == 3
			case strings.Contains(claim, "1,000 km"):
				evidence = "https://example.org/evidence/range"
				refuted = voter != 3
			case strings.Contains(claim, "Jet fuel"):
				evidence = "https://example.org/evidence/fuel"
				if voter == 2 {
					return finalReport("No verdict.", nil), nil
				}
				if voter == 3 {
					// Its evidence names a page it never fetched.
					if toolResults(r) == 0 {
						return fetchCalls("https://example.org/elsewhere"), nil
					}
					return finalReport("Checked.", map[string]any{"refuted": false, "evidence": "It matches.", "evidence_urls": []any{evidence}, "confidence": "high"}, evidence), nil
				}
			}
			if toolResults(r) == 0 {
				return fetchCalls(evidence), nil
			}
			return finalReport("Checked the claim.", map[string]any{"refuted": refuted, "evidence": "What the evidence page says.", "evidence_urls": []any{evidence}, "confidence": confidence}, evidence), nil
		case data["url"] != nil:
			url := data["url"].(string)
			if url == "https://example.org/broken" {
				return finalReport("The extractor broke.", nil), nil
			}
			e, ok := claims[url]
			result := map[string]any{"source_quality": "unreliable", "claims": []any{}}
			if ok {
				result = map[string]any{"source_quality": e.quality, "claims": []any{map[string]any{"claim": e.claim, "quote": "As the page says: " + e.claim, "importance": e.importance}}}
			}
			if url == "https://example.org/unread" {
				// It reports claims without reading the page.
				return finalReport("Extracted.", result), nil
			}
			if toolResults(r) == 0 {
				return fetchCalls(url), nil
			}
			return finalReport("Extracted.", result, url), nil
		case data["query"] != nil:
			label, _ := data["label"].(string)
			return finalReport("Searched "+label+".", map[string]any{"results": searches[label]}), nil
		}
		return finalReport("Unexpected assignment.", nil), nil
	}}
}

func approveBuiltinDeepResearch(t *testing.T, h *runHarness) memory.WorkspaceID {
	t.Helper()
	w := h.researchWorkspace()
	card := h.propose(w.ID, builtinDeepResearchDir(t), builtinDeepResearchFixtures(t))
	if card.Proposal.Simulation == nil || card.Proposal.Simulation.Status != "succeeded" || len(card.Refusals) != 0 || len(card.Requirements.Authority) != 0 {
		t.Fatalf("card = %+v refusals %v", card.Proposal, card.Refusals)
	}
	h.approve(card)
	return w.ID
}

func TestWorkflowDeepResearchV2RunsEndToEndWithFakes(t *testing.T) {
	chat := deepResearchV2Chat()
	model := deepResearchV2Workers(false)
	h, _ := newAgentHarness(t, chat, model)
	ws := approveBuiltinDeepResearch(t, h)
	res, err := h.startRun(h.rt, ws, "deep-research", map[string]string{"question": "What limits the range of electric aircraft?"})
	if err != nil {
		t.Fatal(err)
	}
	d := res.Run
	if d.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("run = %+v\n%s", d.Run, RunText(d))
	}
	outputs, err := decodeObject(d.Run.Outputs)
	if err != nil {
		t.Fatal(err)
	}
	if outputs["outcome"] != "confirmed" {
		t.Fatalf("outcome = %v", outputs["outcome"])
	}
	if _, ok := outputs["inconclusive"]; ok {
		t.Fatal("a confirmed run produced the inconclusive report")
	}
	// The report kept the two verified citations and removed the refuted
	// claim's own source, which no surviving verifier fetched.
	report := outputs["report"].(map[string]any)
	if report["status"] != "corrected" || strings.Contains(report["body_markdown"].(string), "[S3]") {
		t.Fatalf("report = %v", report)
	}
	citations, _ := json.Marshal(report["citations"])
	if string(citations) != `[{"claim_ids":["c1"],"marker":"S1","url":"https://example.org/evidence/cells"},{"claim_ids":["c2"],"marker":"S2","url":"https://example.org/evidence/reserves"}]` {
		t.Fatalf("citations = %s", citations)
	}
	removed, _ := json.Marshal(report["removed"])
	if !strings.Contains(string(removed), `"value":"S3"`) {
		t.Fatalf("removed = %s", removed)
	}
	for _, section := range []string{"## Findings", "## Caveats", "## Open questions"} {
		if !strings.Contains(report["body_markdown"].(string), section) {
			t.Fatalf("report lacks %s", section)
		}
	}
	confidence := map[string]string{}
	for _, c := range outputs["confirmed_claims"].([]any) {
		m := c.(map[string]any)
		confidence[m["text"].(string)] = m["confidence"].(string) + " " + m["vote"].(string)
	}
	// Primary 3-0 with a high rating is high; secondary 2-1 without one
	// starts medium and drops to low.
	if confidence["Aviation cells store about 250 to 300 Wh/kg."] != "high 3-0" ||
		confidence["Electric aircraft keep a 30-minute energy reserve under VFR."] != "low 2-1" || len(confidence) != 2 {
		t.Fatalf("confirmed = %v", confidence)
	}
	refuted := outputs["refuted_claims"].([]any)
	if len(refuted) != 1 || refuted[0].(map[string]any)["text"] != "Electric aircraft will fly 1,000 km by 2027." || refuted[0].(map[string]any)["vote"] != "1-2" {
		t.Fatalf("refuted = %v", refuted)
	}
	unverified := outputs["unverified_claims"].([]any)
	if len(unverified) != 1 || unverified[0].(map[string]any)["text"] != "Jet fuel stores about 12,000 Wh/kg." ||
		unverified[0].(map[string]any)["missing"] != int64(1) || unverified[0].(map[string]any)["invalid"] != int64(1) {
		t.Fatalf("unverified = %v", unverified)
	}
	stats := outputs["stats"].(map[string]any)
	for name, want := range map[string]int64{"candidates": 18, "duplicates": 1, "budget_dropped": 2, "sources": 15, "extraction_failed": 1,
		"claims_unfetched": 1, "claims_verified": 4, "ballots": 12, "confirmed": 2, "refuted": 1, "unverified": 1} {
		if stats[name] != want {
			t.Errorf("stats.%s = %v, want %v", name, stats[name], want)
		}
	}
	// The writer saw the confirmed claims' fetched sources, never the
	// refuted claim's evidence or the unread page.
	captured := chat.captured()
	if len(captured) != 2 {
		t.Fatalf("%d AI calls", len(captured))
	}
	synthesis := captured[1].Messages[1].Content
	if !strings.Contains(synthesis, `"https://example.org/evidence/cells"`) || strings.Contains(synthesis, "evidence/range") || strings.Contains(synthesis, "unread") {
		t.Fatalf("synthesis input = %s", synthesis)
	}
	// 3 searchers, 15 extractors, 3 verifiers for each of 4 claims.
	if len(d.Workers) != 3+15+12 {
		t.Fatalf("workers = %d\n%s", len(d.Workers), RunText(d))
	}
	// Searchers fetched nothing; every other worker got its own data.
	for _, r := range model.recorded() {
		data := dataOf(r)
		if data["query"] != nil && slices.ContainsFunc(r.Messages, func(m openrouter.Message) bool { return m.Role == "tool" }) {
			t.Fatal("a searcher fetched a page")
		}
	}
	checkOrdering(t, d)
}

// When every verifier fails, the run still ends with an honest summary: no
// claim was verified, which is an infrastructure failure, not a finding,
// and no report is written.
func TestWorkflowDeepResearchV2VerifierFailureIsInconclusive(t *testing.T) {
	chat := deepResearchV2Chat()
	h, _ := newAgentHarness(t, chat, deepResearchV2Workers(true))
	ws := approveBuiltinDeepResearch(t, h)
	res, err := h.startRun(h.rt, ws, "deep-research", map[string]string{"question": "What limits the range of electric aircraft?"})
	if err != nil {
		t.Fatal(err)
	}
	d := res.Run
	if d.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("run = %+v\n%s", d.Run, RunText(d))
	}
	outputs, err := decodeObject(d.Run.Outputs)
	if err != nil {
		t.Fatal(err)
	}
	if outputs["outcome"] != "verifiers_failed" {
		t.Fatalf("outcome = %v", outputs["outcome"])
	}
	if _, ok := outputs["report"]; ok {
		t.Fatal("a run whose verifiers all failed wrote a report")
	}
	inconclusive := outputs["inconclusive"].(map[string]any)
	if !strings.Contains(inconclusive["summary"].(string), "infrastructure failure, not a research finding") ||
		!strings.Contains(inconclusive["body_markdown"].(string), "## Unverified claims") {
		t.Fatalf("inconclusive = %v", inconclusive)
	}
	if n := len(chat.captured()); n != 1 {
		t.Fatalf("%d AI calls; only the scope step should run", n)
	}
	var verifyFailed int
	for _, a := range d.Attempts {
		if a.Step == "verify" && a.Item != nil && a.State == eviedb.WorkflowAttemptFailed {
			verifyFailed++
		}
	}
	if verifyFailed != 12 {
		t.Fatalf("%d verifier attempts failed\n%s", verifyFailed, RunText(d))
	}
}

// A partial map with min_succeeded 0 (2026-10-04) succeeds over an empty
// list and when every item fails: its failures are data for later steps.
func TestWorkflowMapMinimumZeroSucceedsWhateverItsItems(t *testing.T) {
	for _, tc := range []struct {
		name   string
		topics string
	}{{"empty list", `[]`}, {"every item fails", `["x","y"]`}} {
		t.Run(tc.name, func(t *testing.T) {
			chat := newFakeChat(func(ctx context.Context, req openrouter.ChatRequest) (openrouter.ChatResponse, error) {
				if strings.Contains(req.Messages[0].Content, "Plan topics") {
					return reply(`{"topics":`+tc.topics+`}`, 50, 10)(ctx, req)
				}
				return reply(`not json`, 40, 10)(ctx, req)
			})
			h, _ := newAgentHarness(t, chat, &workerModel{answer: answerResearch})
			w := h.workspace()
			dir := writeDefinition(t, "summarize-each", strings.Replace(summarizeMap, "    on_item_failure: fail\n", "    on_item_failure: partial\n    min_succeeded: 0\n", 1),
				map[string]string{"prompts/plan.md": "Plan topics for the question.\n", "prompts/summary.md": "Summarize the topic in one sentence.\n",
					"schemas/topics.schema.json": topicsSchema, "schemas/finding.schema.json": findingSchema})
			h.approve(h.propose(w.ID, dir, fixtures(t, `{"inputs": {"question": "Q"}, "steps": {"plan": {"output": {"topics": []}, "tokens": 10}}}`)))
			res, err := h.rt.StartRun(context.Background(), RunRequest{WorkspaceID: w.ID, Workflow: "summarize-each", TextInputs: map[string]string{"question": "Q"}, Surface: "cli"})
			if err != nil {
				t.Fatal(err)
			}
			d := res.Run
			if d.Run.State != eviedb.WorkflowRunSucceeded {
				t.Fatalf("run = %+v\n%s", d.Run, RunText(d))
			}
			outputs, err := decodeObject(d.Run.Outputs)
			if err != nil {
				t.Fatal(err)
			}
			if summaries := outputs["summaries"].([]any); len(summaries) != 0 {
				t.Fatalf("summaries = %v", summaries)
			}
		})
	}
}
