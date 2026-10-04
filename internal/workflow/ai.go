package workflow

import (
	"fmt"
	"slices"
)

// This file is what the Workflow Runtime needs to run AI steps and durable
// interruptions (Stage 14 sub-stage 7; ADR 0010, 0019, 0044): the exact
// reviewed prompt with its hash, the pinned Model Policy and the output
// schema of an AI step, the question and typed answer of an interruption,
// and deterministic validation of a proposal or an answer. Nothing here
// reaches a model: the runtime sends the request and records what came
// back.

// AIStep is one AI step as a run executes it: the Model Policy it names
// (exact provider, model and parameters), the exact bytes of its reviewed
// prompt and their digest, and the canonical JSON of its output schema and
// its digest.
type AIStep struct {
	ID           string
	Policy       PolicyReport
	Prompt       []byte
	PromptSHA256 string
	Schema       []byte
	SchemaSHA256 string
}

// InterruptStep is one durable interruption: what the owner is asked and
// the canonical JSON of the typed answer's schema.
type InterruptStep struct {
	ID           string
	Message      string
	Schema       []byte
	SchemaSHA256 string
}

// canonicalSchema is a loaded schema file's canonical JSON and its digest.
func (v *Validated) canonicalSchema(file string) ([]byte, string, bool) {
	data, ok := v.files[file]
	if !ok {
		return nil, "", false
	}
	_, canonical, err := parseSchema(data)
	if err != nil {
		return nil, "", false
	}
	return canonical, "sha256:" + digest(canonical), true
}

// AIStep returns an AI step's prompt, policy and schema.
func (v *Validated) AIStep(id string) (AIStep, bool) {
	n, ok := v.steps[id]
	if !ok || n.kind != KindAI {
		return AIStep{}, false
	}
	return v.aiStepOf(id, n)
}

// aiStepOf is an AI node's (a step's or a map item's) prompt, policy and
// schema under id.
func (v *Validated) aiStepOf(id string, n *node) (AIStep, bool) {
	m, ok := v.definition.ModelPolicies[n.step.ModelPolicy]
	if !ok {
		return AIStep{}, false
	}
	prompt, ok := v.files[n.step.Prompt]
	if !ok {
		return AIStep{}, false
	}
	schema, schemaDigest, ok := v.canonicalSchema(n.step.OutputSchema)
	if !ok {
		return AIStep{}, false
	}
	return AIStep{
		ID:     id,
		Policy: PolicyReport{Name: n.step.ModelPolicy, Version: m.Version, Provider: m.Provider, Model: m.Model, Temperature: m.Temperature, MaxOutputTokens: m.MaxOutputTokens},
		Prompt: slices.Clone(prompt), PromptSHA256: "sha256:" + digest(prompt),
		Schema: schema, SchemaSHA256: schemaDigest,
	}, true
}

// InterruptStep returns an interruption step's message and answer schema.
func (v *Validated) InterruptStep(id string) (InterruptStep, bool) {
	n, ok := v.steps[id]
	if !ok || n.kind != KindInterrupt {
		return InterruptStep{}, false
	}
	schema, schemaDigest, ok := v.canonicalSchema(n.step.ResponseSchema)
	if !ok {
		return InterruptStep{}, false
	}
	return InterruptStep{ID: id, Message: n.step.Message, Schema: schema, SchemaSHA256: schemaDigest}, true
}

// CheckStepOutput validates an AI step's proposal or an interruption's
// answer against the step's schema, deterministically: types, required
// fields, closed objects, bounds and enums.
func (v *Validated) CheckStepOutput(id string, value any) error {
	n, ok := v.steps[id]
	if !ok || (n.kind != KindAI && n.kind != KindInterrupt) || n.schema == nil {
		return fmt.Errorf("%s is not an AI or interruption step", id)
	}
	return checkValue(n.schema, value)
}

// ModelPolicies lists the Model Policies the definition's steps use, by
// name, as a run pins them in its Execution Composition (ADR 0019).
func (v *Validated) ModelPolicies() []PolicyReport {
	used := map[string]bool{}
	for _, n := range v.steps {
		for _, m := range []*node{n, n.item} {
			if m != nil && (m.kind == KindAI || m.kind == KindAgent) && m.step.ModelPolicy != "" {
				used[m.step.ModelPolicy] = true
			}
		}
	}
	out := []PolicyReport{}
	for _, name := range sortedKeys(used) {
		m := v.definition.ModelPolicies[name]
		out = append(out, PolicyReport{Name: name, Version: m.Version, Provider: m.Provider, Model: m.Model, Temperature: m.Temperature, MaxOutputTokens: m.MaxOutputTokens})
	}
	return out
}
