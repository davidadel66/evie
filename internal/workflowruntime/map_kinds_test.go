package workflowruntime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/openrouter"
)

// A map's item may also be a deterministic step or an AI step (Stage 14
// sub-stage 9): deterministic items call their reviewed function once per
// element, AI items send each element's declared input to their pinned
// model with their own model call, counted against the run's tokens.

const splitMap = `schema_version: 1
name: split-each
description: Split a total into three shares, then split each share in two.
inputs:
  total:
    type: integer
    required: true
    minimum: 0
run_key:
  inputs: [total]
limits:
  max_duration: 5m
  max_attempts: 10
  max_parallel: 2
  max_tokens: 1
retention:
  classification: ordinary
start: split
steps:
  - id: split
    kind: deterministic
    function: split_evenly@1
    input:
      total: $inputs.total
      parts: 3
    next: halves
  - id: halves
    kind: map
    over: $steps.split.output.shares
    max_items: 3
    max_parallel: 2
    on_item_failure: fail
    item:
      kind: deterministic
      function: split_evenly@1
      input:
        total: $item
        parts: 2
outputs:
  halves: $steps.halves.output.results
`

func TestWorkflowMapOfDeterministicItems(t *testing.T) {
	h := newLeaseHarness(t)
	w := h.workspace()
	dir := writeDefinition(t, "split-each", splitMap, map[string]string{})
	h.approve(h.propose(w.ID, dir, fixtures(t, `{"inputs": {"total": 1001}}`)))
	res, err := h.startRun(h.rt, w.ID, "split-each", map[string]string{"total": "1001"})
	if err != nil {
		t.Fatal(err)
	}
	d := res.Run
	if d.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("run = %+v\n%s", d.Run, RunText(d))
	}
	var outputs struct {
		Halves []struct {
			Index  int `json:"index"`
			Item   int `json:"item"`
			Output struct {
				Shares []int `json:"shares"`
			} `json:"output"`
		} `json:"halves"`
	}
	if err := json.Unmarshal(d.Run.Outputs, &outputs); err != nil {
		t.Fatal(err)
	}
	if len(outputs.Halves) != 3 || outputs.Halves[0].Item != 334 || outputs.Halves[0].Output.Shares[0] != 167 || outputs.Halves[2].Index != 2 {
		t.Fatalf("outputs = %s", d.Run.Outputs)
	}
	checkOrdering(t, d)
}

const summarizeMap = `schema_version: 1
name: summarize-each
description: Plan topics, then summarize each with an AI item.
inputs:
  question:
    type: string
    required: true
run_key:
  inputs: [question]
model_policies:
  writer:
    version: 1
    provider: openrouter
    model: deepseek/deepseek-v4.1-flash
    temperature: 0
    max_output_tokens: 100
limits:
  max_duration: 5m
  max_attempts: 10
  max_parallel: 2
  max_tokens: 100000
retention:
  classification: ordinary
start: plan
steps:
  - id: plan
    kind: ai
    model_policy: writer
    prompt: prompts/plan.md
    input:
      question: $inputs.question
    output_schema: schemas/topics.schema.json
    next: summaries
  - id: summaries
    kind: map
    over: $steps.plan.output.topics
    max_items: 3
    max_parallel: 2
    on_item_failure: fail
    item:
      kind: ai
      model_policy: writer
      prompt: prompts/summary.md
      input:
        topic: $item
      output_schema: schemas/finding.schema.json
outputs:
  summaries: $steps.summaries.output.results
`

func TestWorkflowMapOfAIItemsRecordsAModelCallPerItem(t *testing.T) {
	chat := newFakeChat(func(ctx context.Context, req openrouter.ChatRequest) (openrouter.ChatResponse, error) {
		user := req.Messages[len(req.Messages)-1].Content
		if strings.Contains(req.Messages[0].Content, "Plan topics") {
			return reply(`{"topics":["x","y"]}`, 50, 10)(ctx, req)
		}
		topic := "x"
		if strings.Contains(user, `"topic":"y"`) {
			topic = "y"
		}
		return reply(`{"answer":"summary of `+topic+`"}`, 40, 10)(ctx, req)
	})
	h := newAIHarness(t, chat)
	w := h.workspace()
	dir := writeDefinition(t, "summarize-each", summarizeMap, map[string]string{"prompts/plan.md": "Plan topics for the question.\n",
		"prompts/summary.md": "Summarize the topic in one sentence.\n", "schemas/topics.schema.json": topicsSchema, "schemas/finding.schema.json": findingSchema})
	h.approve(h.propose(w.ID, dir, fixtures(t, `{"inputs": {"question": "Q"}, "steps": {"plan": {"output": {"topics": ["x"]}, "tokens": 10},
 "summaries": {"item_default": {"output": {"answer": "s"}, "tokens": 10}}}}`)))
	res, err := h.startRun(h.rt, w.ID, "summarize-each", map[string]string{"question": "Q"})
	if err != nil {
		t.Fatal(err)
	}
	d := res.Run
	if d.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("run = %+v\n%s", d.Run, RunText(d))
	}
	if !strings.Contains(string(d.Run.Outputs), `"output":{"answer":"summary of x"}`) || !strings.Contains(string(d.Run.Outputs), `"output":{"answer":"summary of y"}`) {
		t.Fatalf("outputs = %s", d.Run.Outputs)
	}
	items := 0
	for _, c := range d.ModelCalls {
		if c.Item != nil {
			items++
			if c.State() != eviedb.WorkflowModelCallFinished || c.Tokens() != 50 {
				t.Fatalf("item call = %+v", c)
			}
		}
	}
	if items != 2 {
		t.Fatalf("model calls = %+v", d.ModelCalls)
	}
	if p := h.progress(d.Run.ID); p.ModelTokens != 60+2*50 {
		t.Fatalf("spend = %d", p.ModelTokens)
	}
	checkOrdering(t, d)
}
