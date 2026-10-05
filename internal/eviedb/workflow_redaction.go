package eviedb

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/davidadel66/evie/internal/workflow"
)

// Redaction of one run's recorded values (Stage 14 sub-stage 12; ADR
// 0045, workflow-runtime.spec.md: "Workflow field classifications and
// Workspace policy govern redaction, retention, and export"). The same
// rewriting masks values on the owner's surfaces (marker "[sensitive]")
// and, under a retention policy, redacts them from storage (marker
// "[redacted]").
//
// What is sensitive comes from the run's pinned version:
//
//   - under the "ordinary" classification, the values of the inputs it
//     marks sensitive: the run's input fields, each step input (and map
//     item input, and branch read) that its definition binds to such an
//     input, run outputs that name one, and, in every other recorded value
//     and text, the input's value itself wherever it appears (a string of
//     at least three characters, a number of at least three digits);
//   - under the "sensitive" classification, every value the run recorded:
//     inputs, step outputs, checkpoints, effect inputs and responses,
//     questions and answers, model responses; texts (errors, notes,
//     notices) are rewritten whole when redacted from storage, and keep
//     only the sensitive inputs' values out when masked, so the owner
//     still sees why a run failed.
//
// Structure is kept: object keys, list positions, record identities,
// states, times, idempotency keys, receipts' outcomes and authority
// evaluations never change.

// Retention markers.
const (
	WorkflowMaskMarker    = "[sensitive]"
	WorkflowRedactMarker  = "[redacted]"
	minSensitiveTextRunes = 3
)

// WorkflowRedactor rewrites one run's recorded values.
type WorkflowRedactor struct {
	sensitivity workflow.Sensitivity
	marker      string
	// values and texts rewrite every value and every text whole (the
	// sensitive classification); otherwise only the sensitive inputs'.
	values bool
	texts  bool
	// secrets are the sensitive input values as text; numbers the numeric
	// ones, matched as whole values.
	secrets []string
	numbers []string
}

// NewWorkflowRedactor builds the redactor of a run whose accepted input is
// inputs, for its version's sensitivity. wholeTexts rewrites texts whole
// under the sensitive classification (storage redaction); masking keeps
// them and only takes the sensitive inputs' values out.
func NewWorkflowRedactor(s workflow.Sensitivity, inputs json.RawMessage, marker string, wholeTexts bool) WorkflowRedactor {
	r := WorkflowRedactor{sensitivity: s, marker: marker, values: s.Classification == workflow.ClassificationSensitive}
	r.texts = r.values && wholeTexts
	values, _ := decodeJSONObject(inputs)
	names := s.Inputs
	if r.values {
		names = make([]string, 0, len(values))
		for name := range values {
			names = append(names, name)
		}
		slices.Sort(names)
	}
	for _, name := range names {
		r.collect(values[name])
	}
	// Longer values first, so a value containing another is taken out whole.
	slices.SortFunc(r.secrets, func(a, b string) int { return len(b) - len(a) })
	r.secrets = slices.Compact(r.secrets)
	return r
}

func (r *WorkflowRedactor) collect(v any) {
	switch x := v.(type) {
	case string:
		if x != r.marker && len([]rune(strings.TrimSpace(x))) >= minSensitiveTextRunes {
			r.secrets = append(r.secrets, x)
		}
	case json.Number:
		digits := strings.TrimLeft(x.String(), "-")
		if len(strings.ReplaceAll(digits, ".", "")) >= minSensitiveTextRunes {
			r.numbers = append(r.numbers, x.String())
			r.secrets = append(r.secrets, x.String())
		}
	case map[string]any:
		for _, item := range x {
			r.collect(item)
		}
	case []any:
		for _, item := range x {
			r.collect(item)
		}
	}
}

// Active reports whether the redactor changes anything.
func (r WorkflowRedactor) Active() bool {
	return r.values || len(r.sensitivity.Inputs) > 0
}

// decodeJSONObject decodes an object keeping numbers as written.
func decodeJSONObject(data json.RawMessage) (map[string]any, bool) {
	v, ok := decodeJSON(data)
	m, isObject := v.(map[string]any)
	return m, ok && isObject
}

func decodeJSON(data json.RawMessage) (any, bool) {
	if len(data) == 0 {
		return nil, false
	}
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.UseNumber()
	var v any
	if d.Decode(&v) != nil {
		return nil, false
	}
	return v, true
}

// encodeJSON encodes compactly with sorted keys, keeping text as written
// (no HTML escaping).
func encodeJSON(v any) json.RawMessage {
	var b strings.Builder
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		return nil
	}
	return json.RawMessage(strings.TrimSuffix(b.String(), "\n"))
}

// Text rewrites a recorded text.
func (r WorkflowRedactor) Text(s string) string {
	// A value already redacted from storage stays as it is.
	if s == "" || s == WorkflowRedactMarker {
		return s
	}
	if r.texts {
		return r.marker
	}
	for _, secret := range r.secrets {
		s = strings.ReplaceAll(s, secret, r.marker)
	}
	return s
}

// scrub rewrites the sensitive inputs' values inside a value.
func (r WorkflowRedactor) scrub(v any) any {
	switch x := v.(type) {
	case string:
		if x == r.marker {
			return x
		}
		return r.Text(x)
	case json.Number:
		if slices.Contains(r.numbers, x.String()) {
			return r.marker
		}
		return x
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, item := range x {
			out[k] = r.scrub(item)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = r.scrub(item)
		}
		return out
	}
	return v
}

// whole replaces a value: an object keeps its keys, anything else becomes
// the marker.
func (r WorkflowRedactor) whole(v any) any {
	if m, ok := v.(map[string]any); ok {
		out := make(map[string]any, len(m))
		for k, item := range m {
			out[k] = r.marker
			if item == WorkflowRedactMarker {
				out[k] = item
			}
		}
		return out
	}
	if v == WorkflowRedactMarker {
		return v
	}
	return r.marker
}

// rewrite applies the rewriting to a JSON document: whole under the
// sensitive classification, otherwise the named top-level fields replaced
// and the sensitive values scrubbed from the rest. Unreadable documents
// are returned as they are.
func (r WorkflowRedactor) rewrite(data json.RawMessage, fields []string) json.RawMessage {
	if len(data) == 0 || !r.Active() {
		return data
	}
	v, ok := decodeJSON(data)
	if !ok {
		return data
	}
	if r.values {
		v = r.whole(v)
	} else {
		if m, isObject := v.(map[string]any); isObject && len(fields) > 0 {
			for _, name := range fields {
				if item, present := m[name]; present && item != WorkflowRedactMarker {
					m[name] = r.marker
				}
			}
		}
		v = r.scrub(v)
	}
	out := encodeJSON(v)
	if out == nil || string(out) == string(normalizedJSON(data)) {
		return data
	}
	return out
}

func normalizedJSON(data json.RawMessage) json.RawMessage {
	v, ok := decodeJSON(data)
	if !ok {
		return data
	}
	return encodeJSON(v)
}

// Inputs rewrites the run's accepted input.
func (r WorkflowRedactor) Inputs(data json.RawMessage) json.RawMessage {
	return r.rewrite(data, r.sensitivity.Inputs)
}

// Outputs rewrites the run's outputs.
func (r WorkflowRedactor) Outputs(data json.RawMessage) json.RawMessage {
	return r.rewrite(data, r.sensitivity.Outputs)
}

// Value rewrites any other recorded value: a step output, a checkpoint, a
// response, an answer.
func (r WorkflowRedactor) Value(data json.RawMessage) json.RawMessage { return r.rewrite(data, nil) }

// StepInput rewrites a node attempt's resolved input (or an Effect
// Intent's input): the fields its step binds to a sensitive input.
func (r WorkflowRedactor) StepInput(step string, item bool, kind string, data json.RawMessage) json.RawMessage {
	fields := r.sensitivity.StepArgs[step]
	switch {
	case item:
		fields = r.sensitivity.ItemArgs[step]
	case kind == workflow.KindBranch:
		fields = r.sensitivity.BranchReads[step]
	}
	return r.rewrite(data, fields)
}

// Request rewrites a question's request ({"input": ...}): the input fields
// its step binds to a sensitive input.
func (r WorkflowRedactor) Request(step string, data json.RawMessage) json.RawMessage {
	if r.values || len(data) == 0 || !r.Active() {
		return r.rewrite(data, nil)
	}
	m, ok := decodeJSONObject(data)
	if !ok {
		return data
	}
	if input, isObject := m["input"].(map[string]any); isObject {
		for _, name := range r.sensitivity.StepArgs[step] {
			if _, present := input[name]; present {
				input[name] = r.marker
			}
		}
	}
	scrubbed := encodeJSON(r.scrub(m))
	if scrubbed == nil || string(scrubbed) == string(normalizedJSON(data)) {
		return data
	}
	return scrubbed
}

// Detail rewrites a run's detail for an owner surface: its input, outputs,
// error, checkpoints, attempts, effects, interruptions and model calls.
func (r WorkflowRedactor) Detail(d WorkflowRunDetail) WorkflowRunDetail {
	if !r.Active() {
		return d
	}
	d.Run = r.Run(d.Run)
	checkpoints := make([]WorkflowCheckpoint, len(d.Checkpoints))
	for i, c := range d.Checkpoints {
		c.State = r.Value(c.State)
		checkpoints[i] = c
	}
	d.Checkpoints = checkpoints
	attempts := make([]WorkflowNodeAttempt, len(d.Attempts))
	for i, a := range d.Attempts {
		a.Input = r.StepInput(a.Step, a.Item != nil, a.Kind, a.Input)
		a.Output = r.Value(a.Output)
		a.Error, a.Note = r.Text(a.Error), r.Text(a.Note)
		attempts[i] = a
	}
	d.Attempts = attempts
	effects := make([]WorkflowEffectIntent, len(d.Effects))
	for i, e := range d.Effects {
		e.Input = r.StepInput(e.Step, false, workflow.KindCapability, e.Input)
		if u := e.Unknown; u != nil {
			copied := *u
			copied.Reason, copied.Response = r.Text(copied.Reason), r.Value(copied.Response)
			e.Unknown = &copied
		}
		if rc := e.Receipt; rc != nil {
			copied := *rc
			copied.Response, copied.Error, copied.Evidence = r.Value(copied.Response), r.Text(copied.Error), r.Text(copied.Evidence)
			e.Receipt = &copied
		}
		effects[i] = e
	}
	d.Effects = effects
	d.Interruptions = r.Interruptions(d.Interruptions)
	calls := make([]WorkflowModelCall, len(d.ModelCalls))
	for i, c := range d.ModelCalls {
		c.Response, c.Error = r.Text(c.Response), r.Text(c.Error)
		calls[i] = c
	}
	d.ModelCalls = calls
	events := make([]WorkflowRunEvent, len(d.Events))
	for i, e := range d.Events {
		e.Detail = r.Text(e.Detail)
		events[i] = e
	}
	d.Events = events
	workers := make([]WorkflowWorkerCall, len(d.Workers))
	for i, w := range d.Workers {
		w.Reason = r.Text(w.Reason)
		workers[i] = w
	}
	d.Workers = workers
	return d
}

// Run rewrites a run's input, outputs and error.
func (r WorkflowRedactor) Run(run WorkflowRun) WorkflowRun {
	if !r.Active() {
		return run
	}
	run.Inputs, run.Outputs, run.Error = r.Inputs(run.Inputs), r.Outputs(run.Outputs), r.Text(run.Error)
	return run
}

// Interruptions rewrites questions, their requests and answers.
func (r WorkflowRedactor) Interruptions(list []WorkflowInterruption) []WorkflowInterruption {
	if !r.Active() {
		return list
	}
	out := make([]WorkflowInterruption, len(list))
	for i, in := range list {
		in.Message = r.Text(in.Message)
		in.Request = r.Request(in.Step, in.Request)
		if res := in.Resolution; res != nil {
			copied := *res
			copied.Answer, copied.Detail = r.Value(copied.Answer), r.Text(copied.Detail)
			in.Resolution = &copied
		}
		out[i] = in
	}
	return out
}
