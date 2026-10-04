package workflow

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// This file is what the Workflow Runtime needs to run map steps and agent
// steps (Stage 14 sub-stage 9): a map step's bounds, failure policy and
// item, its list resolved from a prior step's output, each item's input,
// function call or AI step, and the map's output in input order; and an
// agent step's worker, pinned Model Policy, reviewed objective, budget and
// result schema, with the worker result an agent step outputs, its typed
// result taken from the report's "## Result" block. The simulator builds
// map and agent outputs with the same code, so a run and its dry run
// cannot disagree. Nothing here runs a worker or a model.

// MapPlan is a map step as a run executes it.
type MapPlan struct {
	ID       string
	MaxItems int
	// Parallel is the step's max_parallel, never more than the run's
	// limits.max_parallel.
	Parallel int
	// OnItemFailure is "fail" or "partial"; MinSucceeded applies to
	// partial.
	OnItemFailure string
	MinSucceeded  int
	Item          ItemPlan
}

// ItemPlan is a map step's item: its kind and its Retry Policy.
type ItemPlan struct {
	Kind     string
	Attempts int
	Backoff  time.Duration
	Function string
}

// ItemOutcome is how one item ended: its output, or the error of its last
// attempt. Items that never ran are not outcomes.
type ItemOutcome struct {
	Index     int
	Succeeded bool
	Output    any
	Error     string
}

// MapStep returns a map step's plan.
func (v *Validated) MapStep(id string) (MapPlan, bool) {
	n, ok := v.steps[id]
	if !ok || n.kind != KindMap || n.item == nil {
		return MapPlan{}, false
	}
	return MapPlan{ID: id, MaxItems: n.step.MaxItems, Parallel: min(n.step.MaxParallel, v.limits.parallel), OnItemFailure: n.step.OnItemFailure,
		MinSucceeded: n.step.MinSucceeded, Item: ItemPlan{Kind: n.item.kind, Attempts: n.item.attempts, Backoff: n.item.backoff, Function: n.item.step.Function}}, true
}

// mapNode returns a map step's compiled node.
func (v *Validated) mapNode(id string) (*node, error) {
	n, ok := v.steps[id]
	if !ok || n.kind != KindMap || n.item == nil || n.over == nil {
		return nil, fmt.Errorf("%s is not a map step", id)
	}
	return n, nil
}

// MapList resolves the list a map step runs over from the outputs of the
// steps that have run. A value that is not a list is an error; so is a
// reference to a step that has not run.
func (v *Validated) MapList(id string, inputs, steps map[string]any) ([]any, error) {
	n, err := v.mapNode(id)
	if err != nil {
		return nil, err
	}
	if err := requireRan(n, *n.over, steps); err != nil {
		return nil, err
	}
	list, ok := resolveRef(*n.over, inputs, steps, nil).([]any)
	if !ok {
		return nil, fmt.Errorf("%s: %s is not a list", n.path, n.over.Raw)
	}
	return list, nil
}

// ItemInput resolves a map item's declared input for one element.
func (v *Validated) ItemInput(id string, inputs, steps map[string]any, element any) (map[string]any, error) {
	n, err := v.mapNode(id)
	if err != nil {
		return nil, err
	}
	for _, name := range sortedKeys(n.item.args) {
		if a := n.item.args[name]; !a.isConst {
			if err := requireRan(n.item, *a.ref, steps); err != nil {
				return nil, err
			}
		}
	}
	return n.item.inputOf(inputs, steps, element), nil
}

// CallItemFunction runs a deterministic map item's reviewed function once.
func (v *Validated) CallItemFunction(id string, input map[string]any) (any, error) {
	n, err := v.mapNode(id)
	if err != nil {
		return nil, err
	}
	if n.item.kind != KindDeterministic || n.item.fn == nil {
		return nil, fmt.Errorf("%s does not run a deterministic item", id)
	}
	out, failure, broken := n.item.call(input)
	if failure != nil {
		return nil, failure
	}
	if broken != nil {
		return nil, broken
	}
	return out, nil
}

// ItemAIStep returns an AI map item's prompt, policy and schema, under its
// map step's ID.
func (v *Validated) ItemAIStep(id string) (AIStep, bool) {
	n, err := v.mapNode(id)
	if err != nil || n.item.kind != KindAI {
		return AIStep{}, false
	}
	return v.aiStepOf(id, n.item)
}

// CheckItemOutput validates an AI map item's proposal against its schema.
func (v *Validated) CheckItemOutput(id string, value any) error {
	n, err := v.mapNode(id)
	if err != nil {
		return err
	}
	if n.item.kind != KindAI || n.item.schema == nil {
		return fmt.Errorf("%s does not run an AI item", id)
	}
	return checkValue(n.item.schema, value)
}

// MapOutput is a map step's output from its items' outcomes: the items
// that succeeded in input order (index, element, output), then those that
// failed with their errors, and both counts. It never depends on the order
// items finished in, and is checked against the step's output type.
func (v *Validated) MapOutput(id string, list []any, outcomes []ItemOutcome) (any, error) {
	n, err := v.mapNode(id)
	if err != nil {
		return nil, err
	}
	out, err := mapOutput(list, outcomes)
	if err != nil {
		return nil, err
	}
	if n.output != nil {
		if err := checkValue(n.output, out); err != nil {
			return nil, fmt.Errorf("%s: output: %v", n.path, err)
		}
	}
	return out, nil
}

func mapOutput(list []any, outcomes []ItemOutcome) (map[string]any, error) {
	sorted := slices.Clone(outcomes)
	slices.SortFunc(sorted, func(a, b ItemOutcome) int { return a.Index - b.Index })
	results, failures := []any{}, []any{}
	for i, o := range sorted {
		if o.Index < 0 || o.Index >= len(list) || i > 0 && sorted[i-1].Index == o.Index {
			return nil, fmt.Errorf("item %d is not one outcome of a %d-item list", o.Index, len(list))
		}
		if o.Succeeded {
			results = append(results, map[string]any{"index": int64(o.Index), "item": list[o.Index], "output": o.Output})
		} else {
			failures = append(failures, map[string]any{"index": int64(o.Index), "item": list[o.Index], "error": o.Error})
		}
	}
	return map[string]any{"results": results, "failures": failures, "succeeded": int64(len(results)), "failed": int64(len(failures))}, nil
}

// AgentPlan is an agent step, or a map step's agent item (Item; ID is then
// the map step's), as a run executes it: the worker preset, the pinned
// Model Policy, the reviewed objective's exact bytes and digest, the
// result schema's canonical JSON and digest (none when the step declares
// no output schema) and the per-attempt budget.
type AgentPlan struct {
	ID              string
	Item            bool
	Worker          string
	Policy          PolicyReport
	Objective       []byte
	ObjectiveSHA256 string
	Schema          []byte
	SchemaSHA256    string
	Budget          time.Duration
	Tokens          int64
	Attempts        int
	Backoff         time.Duration
}

// agentNode returns an agent step's node, or a map step's agent item.
func (v *Validated) agentNode(id string) (*node, bool) {
	n, ok := v.steps[id]
	switch {
	case !ok:
		return nil, false
	case n.kind == KindAgent:
		return n, true
	case n.kind == KindMap && n.item != nil && n.item.kind == KindAgent:
		return n.item, true
	}
	return nil, false
}

// AgentStep returns an agent step's plan, or that of a map step's agent
// item.
func (v *Validated) AgentStep(id string) (AgentPlan, bool) {
	n, ok := v.agentNode(id)
	if !ok {
		return AgentPlan{}, false
	}
	m, ok := v.definition.ModelPolicies[n.step.ModelPolicy]
	if !ok {
		return AgentPlan{}, false
	}
	objective, ok := v.files[n.step.Objective]
	if !ok {
		return AgentPlan{}, false
	}
	p := AgentPlan{ID: id, Item: n.isItem, Worker: n.step.Worker,
		Policy:    PolicyReport{Name: n.step.ModelPolicy, Version: m.Version, Provider: m.Provider, Model: m.Model, Temperature: m.Temperature, MaxOutputTokens: m.MaxOutputTokens},
		Objective: slices.Clone(objective), ObjectiveSHA256: "sha256:" + digest(objective), Budget: n.budget, Tokens: n.tokens,
		Attempts: n.attempts, Backoff: n.backoff}
	if n.step.OutputSchema != "" {
		schema, schemaDigest, ok := v.canonicalSchema(n.step.OutputSchema)
		if !ok {
			return AgentPlan{}, false
		}
		p.Schema, p.SchemaSHA256 = schema, schemaDigest
	}
	return p, true
}

// WorkerResult is what a worker returned, as the runtime reads it from the
// harness: its terminal status, the report's summary, its full report, the
// sources it fetched (each marked cited in the report or not), its
// limitations and the tokens the run counts for it.
type WorkerResult struct {
	Status      string
	Summary     string
	Report      string
	Sources     []WorkerSource
	Limitations []string
	Tokens      int64
}

// WorkerSource is a URL a worker fetched and whether its report cites it.
type WorkerSource struct {
	URL   string
	Cited bool
}

// ErrWorkerEnded is the failure of an agent attempt whose worker ended
// failed, cancelled or interrupted: it produced no output.
var ErrWorkerEnded = errors.New("the worker produced no result")

type workerEndedError struct{ status string }

func (e workerEndedError) Error() string { return "worker ended " + e.status }
func (e workerEndedError) Unwrap() error { return ErrWorkerEnded }

// AgentOutput is an agent step's (or agent item's) output from its
// worker's result: the result contract, with the typed result from the
// report's last "## Result" json block when the step declares an output
// schema. succeeded and partial (a wrap-up report, noted) succeed; any
// other status (ErrWorkerEnded), a typed result that is missing or outside
// the schema, and a result outside the contract fail the attempt.
func (v *Validated) AgentOutput(id string, r WorkerResult) (any, string, error) {
	n, ok := v.agentNode(id)
	if !ok {
		return nil, "", fmt.Errorf("%s is not an agent step", id)
	}
	out, note, failure, broken := agentOutput(n, r)
	if broken != nil {
		return nil, note, fmt.Errorf("the worker result is outside the result contract: %v", broken)
	}
	return out, note, failure
}

// agentOutput builds the output. failure is the attempt's own failure;
// broken is a result outside the contract (a harness or fixture defect).
func agentOutput(n *node, r WorkerResult) (out any, note string, failure, broken error) {
	switch r.Status {
	case "succeeded":
	case "partial":
		note = "the worker wrapped up at its budget (partial)"
	case "failed", "cancelled", "interrupted":
		return nil, "", workerEndedError{r.Status}, nil
	default:
		return nil, "", nil, fmt.Errorf("unknown worker status %q", r.Status)
	}
	sources := []any{}
	for _, src := range r.Sources {
		sources = append(sources, map[string]any{"url": src.URL, "cited": src.Cited})
	}
	limitations := []any{}
	for _, l := range r.Limitations {
		limitations = append(limitations, l)
	}
	result := map[string]any{
		"status": r.Status, "summary": r.Summary, "report": r.Report,
		"sources": sources, "limitations": limitations, "tokens": r.Tokens,
	}
	if n.schema != nil {
		typed, err := extractAgentResult(r.Report)
		if err == nil {
			err = checkValue(n.schema, typed)
		}
		if err != nil {
			return nil, note, fmt.Errorf("typed result: %v", err), nil
		}
		result["result"] = typed
	}
	if err := checkValue(n.output, result); err != nil {
		return nil, note, nil, err
	}
	return result, note, nil, nil
}
