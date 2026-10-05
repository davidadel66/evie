package workflowruntime

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/openrouter"
)

// Stage 14 sub-stage 10 (2026-10-04): the bundled deep-research template
// proposed into a Workspace's procedural repository with its bundled dry
// run, approved by the owner, and run end to end with scripted fakes: AI
// planning, research workers, three verifiers per claim that must fetch
// their evidence, the tally, the AI report and the citation check. Nothing
// reaches the network. Since the built-in decision (2026-10-04) the
// bundled deep-research is v2 (deepresearch_v2_test.go); the sub-stage 10
// template is frozen in testdata/deep-research-template-v1 and proposed
// from that folder here, so its run keeps being checked.

// deepResearchTemplateV1 is the frozen sub-stage 10 template's folder.
var deepResearchTemplateV1 = filepath.Join(testdata, "deep-research-template-v1")

// deepResearchChat answers the template's AI steps: the plan, then a report
// that cites one verifier source, one source only a researcher fetched and
// one link nobody verified.
func deepResearchChat() *fakeChat {
	return newFakeChat(func(ctx context.Context, req openrouter.ChatRequest) (openrouter.ChatResponse, error) {
		if strings.Contains(req.Messages[0].Content, "You plan a deep-research run") {
			return reply(`{"angles":[{"id":"energy","question":"How much energy do aviation cells store?"},{"id":"rules","question":"Which rules limit electric range?"}]}`, 300, 60)(ctx, req)
		}
		report := map[string]any{
			"title":         "Electric aircraft range",
			"summary":       "Energy density limits range [S1].",
			"body_markdown": "## Energy\n\nEnergy density limits range [S1][S2]. Rules matter too [S3]. More at https://example.org/made-up.\n",
			"citations": []any{
				map[string]any{"marker": "S1", "url": "https://example.org/evidence/energy"},
				map[string]any{"marker": "S2", "url": "https://example.org/energy-1"},
				map[string]any{"marker": "S3", "url": "https://example.org/evidence/rules"},
			},
		}
		b, _ := json.Marshal(report)
		return reply(string(b), 900, 200)(ctx, req)
	})
}

// deepResearchWorkers scripts every worker: a researcher fetches one page
// per angle and reports claims citing it and a page it never fetched; a
// verifier fetches its own evidence page and votes, refuting the unfounded
// claim.
func deepResearchWorkers() *workerModel {
	return &workerModel{answer: func(_ context.Context, r openrouter.ChatRequest) (openrouter.ChatResponse, error) {
		data := dataOf(r)
		if claim, ok := data["claim"].(string); ok {
			topic := "rules"
			if strings.Contains(claim, "energy") {
				topic = "energy"
			}
			evidence := "https://example.org/evidence/" + topic
			if toolResults(r) == 0 {
				return fetchCalls(evidence), nil
			}
			verdict := "supported"
			if strings.Contains(claim, "unfounded") {
				verdict = "refuted"
			}
			return finalReport("Checked the claim.", map[string]any{"verdict": verdict, "evidence": []any{evidence}, "rationale": "An independent source agrees."}, evidence), nil
		}
		angle, _ := data["angle_id"].(string)
		fetched := "https://example.org/" + angle + "-1"
		if toolResults(r) == 0 {
			return fetchCalls(fetched), nil
		}
		claims := []any{map[string]any{"text": "Claim from " + angle + " that holds.", "sources": []any{fetched, "https://example.org/never-fetched"}}}
		if angle == "rules" {
			claims = append(claims, map[string]any{"text": "An unfounded claim from rules.", "sources": []any{fetched}})
		}
		return finalReport("Researched "+angle+".", map[string]any{"claims": claims}, fetched), nil
	}}
}

func TestWorkflowDeepResearchTemplateProposesApprovesAndRunsWithFakes(t *testing.T) {
	ctx := context.Background()
	chat := deepResearchChat()
	h, _ := newAgentHarness(t, chat, deepResearchWorkers())
	w := h.researchWorkspace()
	if _, err := h.rt.Propose(ctx, Proposal{WorkspaceID: w.ID, Template: "no-such-template", Surface: "cli"}); err == nil || !strings.Contains(err.Error(), "no bundled workflow template") {
		t.Fatalf("unknown template: %v", err)
	}
	if _, err := h.rt.Propose(ctx, Proposal{WorkspaceID: w.ID, Template: "deep-research", Dir: "/tmp", Surface: "cli"}); err == nil {
		t.Fatal("a template and a folder were proposed together")
	}
	// The template's bundled fixtures are its dry run: approval needs one.
	v1 := fixtures(t, readFile(t, filepath.Join(deepResearchTemplateV1, "fixtures.json")))
	card, err := h.rt.Propose(ctx, Proposal{WorkspaceID: w.ID, Dir: deepResearchTemplateV1, Fixtures: v1, Surface: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	if card.Proposal.Name != "deep-research" || card.Proposal.Simulation == nil || card.Proposal.Simulation.Status != "succeeded" || !card.NeedsApproval ||
		len(card.Refusals) != 0 || strings.Join(card.Requirements.Workers, ",") != "research" || len(card.Requirements.Authority) != 0 {
		t.Fatalf("card = %+v refusals %v", card.Proposal, card.Refusals)
	}
	if !strings.Contains(card.Patch, "prompts/verify.md") || strings.Contains(card.Patch, "b/fixtures.json") {
		t.Fatalf("committed files:\n%s", card.Patch)
	}
	h.approve(card)
	res, err := h.startRun(h.rt, w.ID, "deep-research", map[string]string{"question": "What limits the range of electric aircraft?"})
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
	report := outputs["report"].(map[string]any)
	if report["status"] != "corrected" ||
		report["body_markdown"] != "## Energy\n\nEnergy density limits range [S1]. Rules matter too [S3]. More at [unverified link removed].\n" {
		t.Fatalf("report = %v", report)
	}
	citations, _ := json.Marshal(report["citations"])
	if string(citations) != `[{"claim_ids":["c1"],"marker":"S1","url":"https://example.org/evidence/energy"},{"claim_ids":["c2"],"marker":"S3","url":"https://example.org/evidence/rules"}]` {
		t.Fatalf("citations = %s", citations)
	}
	removed, _ := json.Marshal(report["removed"])
	for _, want := range []string{`"value":"S2"`, `"value":"https://example.org/made-up"`} {
		if !strings.Contains(string(removed), want) {
			t.Fatalf("removed = %s", removed)
		}
	}
	var verified []string
	for _, c := range outputs["verified_claims"].([]any) {
		claim := c.(map[string]any)
		verified = append(verified, claim["text"].(string))
	}
	if strings.Join(verified, "|") != "Claim from energy that holds.|Claim from rules that holds." || len(outputs["rejected_claims"].([]any)) != 1 {
		t.Fatalf("verified %v rejected %v", verified, outputs["rejected_claims"])
	}
	// The writer saw only verifier-fetched sources for the verified claims.
	captured := chat.captured()
	synthesis := captured[len(captured)-1].Messages[1].Content
	if !strings.Contains(synthesis, `"sources":["https://example.org/evidence/energy"]`) || strings.Contains(synthesis, "never-fetched") ||
		strings.Contains(synthesis, `"https://example.org/energy-1"`) {
		t.Fatalf("synthesis input = %s", synthesis)
	}
	// 2 researchers and 3 verifiers for each of 3 claims.
	if len(d.Workers) != 2+9 {
		t.Fatalf("workers = %d\n%s", len(d.Workers), RunText(d))
	}
	// Agent outputs in checkpoints keep summaries and typed results, not
	// reports; each names the execution whose report the store keeps.
	last := d.Checkpoints[len(d.Checkpoints)-1]
	if len(last.State) > 32*1024 || strings.Contains(string(last.State), "## Details") || strings.Contains(string(last.State), `"report":"`) {
		t.Fatalf("final checkpoint is %d bytes:\n%.2000s", len(last.State), last.State)
	}
	var state map[string]any
	if err := json.Unmarshal(last.State, &state); err != nil {
		t.Fatal(err)
	}
	first := state["verify"].(map[string]any)["results"].([]any)[0].(map[string]any)["output"].(map[string]any)
	execution, _ := first["execution_id"].(string)
	if execution == "" || first["report_bytes"] == nil {
		t.Fatalf("verify output = %v", first)
	}
	// The owner reads it through the runtime (evie workflows report).
	full, err := h.rt.WorkerReport(ctx, d.Run.ID, execution)
	if err != nil || !strings.Contains(full.Report, "## Result") || full.Step != "verify" || full.Item == nil || full.Worker != "research" || full.Status != "succeeded" {
		t.Fatalf("the full report of %s: %+v %v", execution, full, err)
	}
	if _, err := h.rt.WorkerReport(ctx, "another-run", execution); err == nil {
		t.Fatal("a report was read under another run")
	}
	var comp composition
	if err := json.Unmarshal(d.Run.Composition, &comp); err != nil || comp.Runtime != RuntimeVersion || RuntimeVersion != "evie.workflow.runtime/6" {
		t.Fatalf("composition = %s", d.Run.Composition)
	}
	checkOrdering(t, d)
}

// A run an earlier Evie pinned to evie.workflow.runtime/5 continues with the
// output shape it started with: the full report in each agent output.
func TestWorkflowRuntimeFiveRunsKeepFullReports(t *testing.T) {
	ctx := context.Background()
	h, _ := newAgentHarness(t, nil, &workerModel{answer: answerResearch})
	w := h.researchWorkspace()
	h.approve(h.propose(w.ID, researchDefinition(t, 1, 100000, 500000), fixtures(t, researchFixtures)))
	earlier := h.process()
	earlier.rt.pin = "evie.workflow.runtime/5"
	res, err := earlier.rt.StartRun(ctx, RunRequest{WorkspaceID: w.ID, Workflow: "one-research", TextInputs: map[string]string{"question": "Q"}, Surface: "cli", Detach: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.rt.execute(ctx, res.Run.Run.ID); err != nil {
		t.Fatal(err)
	}
	d := h.inspect(res.Run.Run.ID)
	if d.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("run = %+v\n%s", d.Run, RunText(d))
	}
	var output map[string]any
	for _, a := range d.Attempts {
		if a.Step == "research" && a.State == eviedb.WorkflowAttemptSucceeded {
			if err := json.Unmarshal(a.Output, &output); err != nil {
				t.Fatal(err)
			}
		}
	}
	if report, _ := output["report"].(string); !strings.Contains(report, "## Result") {
		t.Fatalf("a /5 run's agent output = %v", output)
	}
	if _, has := output["execution_id"]; has {
		t.Fatalf("a /5 run's agent output changed shape: %v", output)
	}
}
