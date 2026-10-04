package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"time"
)

// This file is what the Workflow Runtime needs to execute a validated
// definition (Stage 14 sub-stage 3): the step graph, accepted run inputs and
// their Run Key, each step's resolved input, deterministic function calls,
// branch decisions and the run's outputs. The simulator uses the same
// helpers, so a run and its dry run cannot disagree about semantics. Nothing
// here stores, schedules or retries: the runtime records every attempt and
// checkpoint around these pure calls.

// StepPlan is what the runtime needs to schedule one step.
type StepPlan struct {
	ID   string
	Kind string
	// Next is the step after this one; "" ends the run. A branch's next
	// step is its decision.
	Next string
	// Attempts is the step's Retry Policy: at most this many attempts.
	Attempts int
	// Backoff is the wait between attempts.
	Backoff time.Duration
	// Function is a deterministic step's name@version.
	Function string
}

// RunLimits are the definition's limits on one run.
type RunLimits struct {
	Duration time.Duration
	Attempts int
	Parallel int
	Tokens   int64
}

// BranchDecision is a branch step's evaluation: the step it chose, which
// case matched (1-based; 0 is the default), a note for the record and the
// values it read, by reference.
type BranchDecision struct {
	Next string         `json:"next"`
	Case int            `json:"case"`
	Note string         `json:"note"`
	Read map[string]any `json:"read"`
}

// Start is the first step.
func (v *Validated) Start() string { return v.definition.Start }

// Step returns one step's plan.
func (v *Validated) Step(id string) (StepPlan, bool) {
	n, ok := v.steps[id]
	if !ok {
		return StepPlan{}, false
	}
	p := StepPlan{ID: id, Kind: n.kind, Attempts: n.attempts, Backoff: n.backoff, Function: n.step.Function}
	if n.kind != KindBranch {
		p.Next = n.step.Next
	}
	return p, true
}

// StepKinds lists the distinct kinds of every step and map item, sorted.
func (v *Validated) StepKinds() []string {
	seen := map[string]bool{}
	for _, n := range v.steps {
		seen[n.kind] = true
		if n.item != nil {
			seen[n.item.kind] = true
		}
	}
	return sortedKeys(seen)
}

// Functions lists the reviewed functions the definition calls, sorted.
func (v *Validated) Functions() []string {
	seen := map[string]bool{}
	for _, n := range v.steps {
		for _, m := range []*node{n, n.item} {
			if m != nil && m.fn != nil {
				seen[m.fn.ID()] = true
			}
		}
	}
	return sortedKeys(seen)
}

// Limits are the definition's run limits.
func (v *Validated) Limits() RunLimits {
	return RunLimits{Duration: v.limits.duration, Attempts: v.limits.attempts, Parallel: v.limits.parallel, Tokens: v.limits.tokens}
}

// maxInputBytes bounds one run's input document.
const maxInputBytes = 64 << 10

// ParseInputs reads a run input document: one JSON object of input names
// to values, without repeated names. Values stay raw until AcceptInputs
// checks them against the declared inputs.
func ParseInputs(data []byte) (map[string]json.RawMessage, error) {
	if len(data) > maxInputBytes {
		return nil, fmt.Errorf("run inputs are larger than %d KiB", maxInputBytes>>10)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, errors.New("run inputs must be one JSON object of input names to values")
	}
	out := map[string]json.RawMessage{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("run inputs: %w", err)
		}
		name, _ := tok.(string)
		if _, dup := out[name]; dup {
			return nil, fmt.Errorf("run inputs name %s twice", name)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("run inputs: %w", err)
		}
		out[name] = raw
	}
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("run inputs: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("run inputs must be one JSON object")
	}
	return out, nil
}

var decimalText = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`)

// textValue converts one command-line value to its declared type: text as
// written, integers and numbers in plain decimal, booleans as true or false.
func textValue(in *inputInfo, s string) (any, error) {
	switch in.t.Kind {
	case KindString:
		return s, nil
	case KindInteger:
		if !integerText.MatchString(s) {
			return nil, fmt.Errorf("%q is not a plain decimal integer", s)
		}
	case KindNumber:
		if !decimalText.MatchString(s) {
			return nil, fmt.Errorf("%q is not a plain decimal number", s)
		}
	case KindBoolean:
		if s != "true" && s != "false" {
			return nil, fmt.Errorf("%q is not true or false", s)
		}
	}
	return parseCanonicalJSON([]byte(s))
}

// AcceptInputs checks run inputs against the declared inputs and applies
// defaults: JSON values, and text values (from a command line) converted to
// each input's declared type. An input may be given only once.
func (v *Validated) AcceptInputs(values map[string]json.RawMessage, text map[string]string) (map[string]any, error) {
	given := map[string]any{}
	for _, name := range sortedKeys(values) {
		if _, ok := v.inputs[name]; !ok {
			return nil, fmt.Errorf("inputs.%s: the workflow declares no such input", name)
		}
		value, err := parseCanonicalJSON(values[name])
		if err != nil {
			return nil, fmt.Errorf("inputs.%s: %v", name, err)
		}
		given[name] = value
	}
	for _, name := range sortedKeys(text) {
		in, ok := v.inputs[name]
		switch {
		case !ok:
			return nil, fmt.Errorf("inputs.%s: the workflow declares no such input", name)
		case hasKey(values, name):
			return nil, fmt.Errorf("inputs.%s: given twice", name)
		}
		value, err := textValue(in, text[name])
		if err != nil {
			return nil, fmt.Errorf("inputs.%s: %v", name, err)
		}
		given[name] = value
	}
	out := map[string]any{}
	for _, name := range sortedKeys(given) {
		if err := checkValue(v.inputs[name].t, given[name]); err != nil {
			return nil, fmt.Errorf("inputs.%s: %v", name, err)
		}
		out[name] = given[name]
	}
	for _, name := range sortedKeys(v.inputs) {
		in := v.inputs[name]
		if _, ok := out[name]; ok {
			continue
		}
		switch {
		case in.hasDefault:
			out[name] = in.def
		case in.spec.Required:
			return nil, fmt.Errorf("inputs.%s: required", name)
		}
	}
	return out, nil
}

// RunKey derives the Run Key from accepted inputs: the workflow name and the
// digest of the canonical JSON of the run key rule's inputs (ADR 0021).
func (v *Validated) RunKey(inputs map[string]any) string {
	selected := map[string]any{}
	for _, name := range v.definition.RunKey.Inputs {
		selected[name] = inputs[name]
	}
	data, _ := canonicalBytes(selected)
	return v.definition.Name + ":sha256:" + digest(data)
}

// resolveRef reads one reference from run inputs, step outputs and the
// current map item. A field that is absent reads as nil.
func resolveRef(r Ref, inputs, steps map[string]any, item any) any {
	switch r.Root {
	case refInputs:
		return walkValue(inputs[r.Name], r.Path)
	case refSteps:
		return walkValue(steps[r.Name], r.Path)
	case refItem:
		return walkValue(item, r.Path)
	}
	return nil
}

// inputOf resolves a step's declared input.
func (n *node) inputOf(inputs, steps map[string]any, item any) map[string]any {
	out := map[string]any{}
	for name, a := range n.args {
		if a.isConst {
			out[name] = a.literal
			continue
		}
		out[name] = resolveRef(*a.ref, inputs, steps, item)
	}
	return out
}

// requireRan refuses a step output reference whose step has not run. A
// valid graph never asks for one; a damaged checkpoint could.
func requireRan(n *node, r Ref, steps map[string]any) error {
	if r.Root != refSteps {
		return nil
	}
	if _, ran := steps[r.Name]; !ran {
		return fmt.Errorf("%s: %s: step %s has not run", n.path, r.Raw, r.Name)
	}
	return nil
}

// StepInput resolves a step's declared input from run inputs and the
// outputs of the steps that have run.
func (v *Validated) StepInput(id string, inputs, steps map[string]any) (map[string]any, error) {
	n, ok := v.steps[id]
	if !ok {
		return nil, fmt.Errorf("unknown step %s", id)
	}
	for _, name := range sortedKeys(n.args) {
		if a := n.args[name]; !a.isConst {
			if err := requireRan(n, *a.ref, steps); err != nil {
				return nil, err
			}
		}
	}
	return n.inputOf(inputs, steps, nil), nil
}

// call runs a deterministic node's reviewed function once. failure is the
// function refusing its input (or panicking); broken is a result outside
// the function's declared type, which is a defect in the function.
func (n *node) call(input map[string]any) (out any, failure error, broken error) {
	defer func() {
		if r := recover(); r != nil {
			out, failure, broken = nil, fmt.Errorf("%s panicked: %v", n.fn.ID(), r), nil
		}
	}()
	result, err := n.fn.Call(input)
	if err != nil {
		return nil, err, nil
	}
	if err := checkValue(n.fn.Result, result); err != nil {
		return nil, nil, fmt.Errorf("%s: %s returned a value outside its result type: %v", n.path, n.fn.ID(), err)
	}
	return result, nil, nil
}

// CallFunction runs a deterministic step's reviewed function once on its
// resolved input. Every failure, including a panic or a result outside the
// declared type, is returned as an error for the attempt.
func (v *Validated) CallFunction(id string, input map[string]any) (any, error) {
	n, ok := v.steps[id]
	if !ok || n.kind != KindDeterministic || n.fn == nil {
		return nil, fmt.Errorf("%s is not a deterministic step", id)
	}
	out, failure, broken := n.call(input)
	if failure != nil {
		return nil, failure
	}
	if broken != nil {
		return nil, broken
	}
	return out, nil
}

// decide evaluates a branch's ordered cases.
func (n *node) decide(resolve func(Ref) any) BranchDecision {
	d := BranchDecision{Read: map[string]any{}}
	for _, c := range n.cases {
		d.Read[c.ref.Raw] = resolve(c.ref)
	}
	for i, c := range n.cases {
		if evaluate(c, d.Read[c.ref.Raw]) {
			d.Next, d.Case, d.Note = c.next, i+1, fmt.Sprintf("case %d matched: %s %s", i+1, c.ref.Raw, c.op)
			return d
		}
	}
	d.Next, d.Note = n.step.Default, "no case matched; default"
	return d
}

// Branch evaluates a branch step against run inputs and step outputs.
func (v *Validated) Branch(id string, inputs, steps map[string]any) (BranchDecision, error) {
	n, ok := v.steps[id]
	if !ok || n.kind != KindBranch {
		return BranchDecision{}, fmt.Errorf("%s is not a branch step", id)
	}
	for _, c := range n.cases {
		if err := requireRan(n, c.ref, steps); err != nil {
			return BranchDecision{}, err
		}
	}
	return n.decide(func(r Ref) any { return resolveRef(r, inputs, steps, nil) }), nil
}

// Outputs are the run's declared outputs: each one whose step ran (or that
// names a run input).
func (v *Validated) Outputs(inputs, steps map[string]any) map[string]any {
	out := map[string]any{}
	names := make([]string, 0, len(v.definition.Outputs))
	for name := range v.definition.Outputs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		r, err := parseRef(v.definition.Outputs[name])
		if err != nil {
			continue
		}
		if r.Root == refInputs {
			out[name] = walkValue(inputs[r.Name], r.Path)
			continue
		}
		if value, ran := steps[r.Name]; ran {
			out[name] = walkValue(value, r.Path)
		}
	}
	return out
}

// DecodeValue decodes one JSON document into canonical values (integers as
// int64, other numbers as float64), as checkpoints and inputs are read back.
func DecodeValue(data []byte) (any, error) { return parseCanonicalJSON(data) }

// EncodeValue is the canonical JSON encoding of a canonical value.
func EncodeValue(v any) ([]byte, error) { return canonicalBytes(v) }
