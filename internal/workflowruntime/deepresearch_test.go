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

// The deep-research example runs end to end (Stage 14 sub-stage 9): an AI
// step plans angles, a map runs one research worker per angle, collect_claims
// keeps only sources a worker fetched, expand_ballots gives each claim three
// independent verify workers, majority_filter keeps the claims a majority
// supports, and an AI step writes the report from them. Every model and the
// Web are scripted fakes: nothing reaches the network.
func TestWorkflowDeepResearchRunsEndToEndWithFakes(t *testing.T) {
	ctx := context.Background()
	chat := newFakeChat(func(_ context.Context, req openrouter.ChatRequest) (openrouter.ChatResponse, error) {
		if strings.Contains(req.Messages[0].Content, "You plan research") {
			return reply(`{"angles":[{"id":"energy","question":"How much energy do batteries store?"},{"id":"rules","question":"Which rules limit range?"}]}`, 300, 60)(ctx, req)
		}
		return reply(`{"title":"Electric aircraft range","summary":"Energy density limits range.","body_markdown":"Batteries store far less energy than fuel.","citations":["https://example.org/energy-1"]}`, 900, 200)(ctx, req)
	})
	model := &workerModel{answer: func(_ context.Context, r openrouter.ChatRequest) (openrouter.ChatResponse, error) {
		data := dataOf(r)
		if claim, ok := data["claim"].(string); ok {
			// A verify worker: it reads a page, then votes.
			if toolResults(r) == 0 {
				return fetchCalls("https://example.org/check"), nil
			}
			verdict := "supported"
			if strings.Contains(claim, "unfounded") {
				verdict = "refuted"
			}
			return finalReport("Checked the claim.", map[string]any{"verdict": verdict, "rationale": "Independent sources agree."}, "https://example.org/check"), nil
		}
		angle, _ := data["angle_id"].(string)
		fetched := "https://example.org/" + angle + "-1"
		if toolResults(r) == 0 {
			return fetchCalls(fetched), nil
		}
		claims := []any{
			map[string]any{"text": "Claim from " + angle + " that holds.", "sources": []any{fetched, "https://example.org/never-fetched"}},
		}
		if angle == "rules" {
			claims = append(claims, map[string]any{"text": "An unfounded claim from rules.", "sources": []any{fetched}})
		}
		return finalReport("Researched "+angle+".", map[string]any{"claims": claims}, fetched), nil
	}}
	h, _ := newAgentHarness(t, chat, model)
	w := h.researchWorkspace()
	h.approve(h.propose(w.ID, filepath.Join(testdata, "deep-research"), deepResearchFixtures(t)))
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
	if report["title"] != "Electric aircraft range" {
		t.Fatalf("report = %v", report)
	}
	accepted := outputs["verified_claims"].([]any)
	var texts []string
	for _, c := range accepted {
		claim := c.(map[string]any)
		texts = append(texts, claim["text"].(string))
		for _, s := range claim["sources"].([]any) {
			if s == "https://example.org/never-fetched" {
				t.Fatalf("a source no worker fetched survived: %v", claim)
			}
		}
	}
	if strings.Join(texts, "|") != "Claim from energy that holds.|Claim from rules that holds." {
		t.Fatalf("verified claims = %v", texts)
	}
	// 2 research workers and 3 verify workers for each of 3 claims.
	if len(d.Workers) != 2+9 {
		t.Fatalf("workers = %d\n%s", len(d.Workers), RunText(d))
	}
	views := MapViews(d)
	if len(views) != 2 || len(views[0].Items) != 2 || len(views[1].Items) != 9 {
		t.Fatalf("map views = %+v", views)
	}
	// The report's model saw the verified and rejected claims as data, with
	// a source no worker fetched only as unverified, and no tools.
	captured := chat.captured()
	synthesis := captured[len(captured)-1]
	user := synthesis.Messages[len(synthesis.Messages)-1].Content
	if !strings.Contains(user, `"text":"Claim from energy that holds."`) || !strings.Contains(user, `"rejected_claims":[{"angle_id":"rules"`) ||
		!strings.Contains(user, `"unverified_sources":["https://example.org/never-fetched"]`) || len(synthesis.Tools) != 0 {
		t.Fatalf("synthesis request = %s", user)
	}
	var comp composition
	if err := json.Unmarshal(d.Run.Composition, &comp); err != nil || len(comp.Workers) != 1 || comp.Workers[0].Worker != "research" {
		t.Fatalf("composition = %s", d.Run.Composition)
	}
	text := RunText(d)
	for _, want := range []string{"Map steps.research from checkpoint 1", "Map steps.verify from checkpoint 4", "Run spend:", "AI spend:", "Worker spend:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("inspection lacks %q:\n%s", want, text)
		}
	}
	checkOrdering(t, d)
}
