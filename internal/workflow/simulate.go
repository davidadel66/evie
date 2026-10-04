package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// Fixtures supply everything a simulation would otherwise get from outside:
// run inputs and, per step, AI outputs, agent worker results, capability
// read results, effect responses and interruption responses.
type Fixtures struct {
	Inputs map[string]json.RawMessage `json:"inputs"`
	Steps  map[string]StepFixture     `json:"steps"`
}

// AttemptFixture is one attempt's outcome: an output, or an error.
type AttemptFixture struct {
	Output json.RawMessage `json:"output,omitempty"`
	Error  string          `json:"error,omitempty"`
	Tokens int64           `json:"tokens,omitempty"`
}

// ItemFixture is a step's or map item's outcome: one attempt given inline,
// or a list of attempts for retries.
type ItemFixture struct {
	AttemptFixture
	Attempts []AttemptFixture `json:"attempts,omitempty"`
}

// StepFixture is one step's fixture. Map steps use items (by index) and
// item_default; interruptions use response.
type StepFixture struct {
	ItemFixture
	Items       map[string]ItemFixture `json:"items,omitempty"`
	ItemDefault *ItemFixture           `json:"item_default,omitempty"`
	Response    json.RawMessage        `json:"response,omitempty"`
}

// ParseFixtures reads one strict JSON fixture document.
func ParseFixtures(data []byte) (Fixtures, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var f Fixtures
	if err := dec.Decode(&f); err != nil {
		return Fixtures{}, fmt.Errorf("fixtures: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Fixtures{}, errors.New("fixtures must be one JSON document")
	}
	return f, nil
}

// Simulation is an effect-suppressed dry run: a deterministic trace of what
// the workflow would do with these fixtures.
type Simulation struct {
	Workflow       string          `json:"workflow"`
	ExecutableHash string          `json:"executable_hash"`
	RunKey         string          `json:"run_key"`
	Status         string          `json:"status"`
	Error          string          `json:"error,omitempty"`
	Inputs         map[string]any  `json:"inputs"`
	Trace          []TraceEntry    `json:"trace"`
	Previews       []EffectPreview `json:"previews"`
	Outputs        map[string]any  `json:"outputs"`
	Attempts       int             `json:"attempts"`
	Tokens         int64           `json:"tokens"`
	Notice         string          `json:"notice"`
}

// TraceEntry is one attempt, map summary, branch decision, effect preview
// or interruption.
type TraceEntry struct {
	Step    string `json:"step"`
	Kind    string `json:"kind"`
	Item    *int   `json:"item,omitempty"`
	Attempt int    `json:"attempt,omitempty"`
	Status  string `json:"status"`
	Input   any    `json:"input,omitempty"`
	Output  any    `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
	Tokens  int64  `json:"tokens,omitempty"`
	Next    string `json:"next,omitempty"`
	Note    string `json:"note,omitempty"`
}

// EffectPreview is an external effect the run would request. It was not
// executed and is not an Effect Receipt.
type EffectPreview struct {
	Step       string `json:"step"`
	Capability string `json:"capability"`
	Contract   string `json:"contract"`
	Connection string `json:"connection"`
	Resource   string `json:"resource"`
	Input      any    `json:"input"`
}

// Simulation outcomes.
const (
	SimulationSucceeded = "succeeded"
	SimulationFailed    = "failed"
	SimulationWaiting   = "waiting_for_human"
)

const simulationNotice = "Effects suppressed: nothing was executed, no Effect Receipt was written and no authority was granted."

// errRunFailed ends a simulated run; its message is the run's error.
type errRunFailed struct{ msg string }

func (e errRunFailed) Error() string { return e.msg }

func runFailed(format string, args ...any) error {
	return errRunFailed{fmt.Sprintf(format, args...)}
}

type simulator struct {
	v        *Validated
	f        Fixtures
	sim      *Simulation
	inputs   map[string]any
	state    map[string]any
	attempts int
	tokens   int64
}

// Simulate runs the definition against fixtures with effects suppressed. It
// calls no model, worker or capability provider and writes nothing. A
// fixture problem is an error; a failure the workflow itself would have
// (invalid output, exhausted retries, limits) is a failed simulation.
func (v *Validated) Simulate(f Fixtures) (*Simulation, error) {
	s := &simulator{v: v, f: f, state: map[string]any{}}
	if err := s.checkFixtures(); err != nil {
		return nil, err
	}
	inputs, err := v.acceptInputs(f.Inputs)
	if err != nil {
		return nil, err
	}
	s.inputs = inputs
	s.sim = &Simulation{
		Workflow: v.definition.Name, ExecutableHash: v.hash, RunKey: v.runKey(inputs),
		Inputs: inputs, Trace: []TraceEntry{}, Previews: []EffectPreview{}, Outputs: map[string]any{},
		Notice: simulationNotice,
	}
	status, err := s.run()
	s.sim.Attempts, s.sim.Tokens = s.attempts, s.tokens
	var failed errRunFailed
	switch {
	case errors.As(err, &failed):
		s.sim.Status, s.sim.Error = SimulationFailed, failed.msg
		return s.sim, nil
	case err != nil:
		return nil, err
	}
	s.sim.Status = status
	if status == SimulationSucceeded {
		for name, ref := range v.definition.Outputs {
			r, _ := parseRef(ref)
			if r.Root == refInputs {
				s.sim.Outputs[name] = walkValue(inputs[r.Name], r.Path)
				continue
			}
			if out, ran := s.state[r.Name]; ran {
				s.sim.Outputs[name] = walkValue(out, r.Path)
			}
		}
	}
	return s.sim, nil
}

func (s *simulator) checkFixtures() error {
	for _, id := range sortedKeys(s.f.Steps) {
		n, ok := s.v.steps[id]
		if !ok {
			return fmt.Errorf("fixtures name unknown step %s", id)
		}
		f := s.f.Steps[id]
		hasAttempt := len(f.Output) > 0 || f.Error != "" || len(f.Attempts) > 0
		switch n.kind {
		case KindDeterministic:
			return fmt.Errorf("steps.%s is deterministic: it runs its reviewed code and takes no fixture", id)
		case KindBranch:
			return fmt.Errorf("steps.%s is a branch: it evaluates its cases and takes no fixture", id)
		case KindMap:
			if hasAttempt || len(f.Response) > 0 {
				return fmt.Errorf("steps.%s is a map: give items and item_default", id)
			}
			if n.item.kind == KindDeterministic && (len(f.Items) > 0 || f.ItemDefault != nil) {
				return fmt.Errorf("steps.%s runs a deterministic item: it takes no fixture", id)
			}
			for key := range f.Items {
				if i, err := strconv.Atoi(key); err != nil || i < 0 || strconv.Itoa(i) != key {
					return fmt.Errorf("steps.%s.items: %q is not an item index", id, key)
				}
			}
		case KindInterrupt:
			if hasAttempt || len(f.Items) > 0 || f.ItemDefault != nil {
				return fmt.Errorf("steps.%s is an interruption: give response", id)
			}
		default:
			if len(f.Items) > 0 || f.ItemDefault != nil || len(f.Response) > 0 {
				return fmt.Errorf("steps.%s is a %s step: give output or attempts", id, n.kind)
			}
			if n.cap != nil && n.cap.Effect && (f.Error != "" || len(f.Attempts) > 0) {
				return fmt.Errorf("steps.%s is an effect: it is only previewed, so give its simulated output; effect failures and reconciliation belong to the effect protocol (sub-stage 6)", id)
			}
		}
	}
	return nil
}

// acceptInputs applies defaults and validates inputs as a run start would.
func (v *Validated) acceptInputs(raw map[string]json.RawMessage) (map[string]any, error) {
	out := map[string]any{}
	for _, name := range sortedKeys(raw) {
		in, ok := v.inputs[name]
		if !ok {
			return nil, fmt.Errorf("inputs.%s: the workflow declares no such input", name)
		}
		value, err := parseCanonicalJSON(raw[name])
		if err == nil {
			err = checkValue(in.t, value)
		}
		if err != nil {
			return nil, fmt.Errorf("inputs.%s: %v", name, err)
		}
		out[name] = value
	}
	for _, name := range sortedKeys(v.inputs) {
		in := v.inputs[name]
		if _, given := out[name]; given {
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

// runKey derives the Run Key: the workflow name and the digest of the
// canonical JSON of the rule's inputs.
func (v *Validated) runKey(inputs map[string]any) string {
	selected := map[string]any{}
	for _, name := range v.definition.RunKey.Inputs {
		selected[name] = inputs[name]
	}
	data, _ := canonicalBytes(selected)
	return v.definition.Name + ":sha256:" + digest(data)
}

func (s *simulator) entry(e TraceEntry) { s.sim.Trace = append(s.sim.Trace, e) }

func (s *simulator) run() (string, error) {
	id := s.v.definition.Start
	for id != "" {
		n := s.v.steps[id]
		next := n.step.Next
		switch n.kind {
		case KindBranch:
			next = s.branch(n)
		case KindMap:
			out, err := s.mapStep(n)
			if err != nil {
				return "", err
			}
			s.state[id] = out
		case KindInterrupt:
			out, waiting, err := s.interrupt(n)
			if err != nil {
				return "", err
			}
			if waiting {
				return SimulationWaiting, nil
			}
			s.state[id] = out
		default:
			fixture := s.f.Steps[id]
			out, err := s.attemptStep(n, &fixture.ItemFixture, nil, nil)
			var failed itemFailed
			if errors.As(err, &failed) {
				return "", runFailed("%s", failed.msg)
			}
			if err != nil {
				return "", err
			}
			s.state[id] = out
		}
		id = next
	}
	return SimulationSucceeded, nil
}

func (s *simulator) resolveArgs(n *node, item any) map[string]any {
	out := map[string]any{}
	for name, a := range n.args {
		if a.isConst {
			out[name] = a.literal
			continue
		}
		out[name] = s.resolve(*a.ref, item)
	}
	return out
}

func (s *simulator) resolve(r Ref, item any) any {
	switch r.Root {
	case refInputs:
		return walkValue(s.inputs[r.Name], r.Path)
	case refSteps:
		return walkValue(s.state[r.Name], r.Path)
	case refItem:
		return walkValue(item, r.Path)
	}
	return nil
}

func (s *simulator) countAttempt(n *node) error {
	if s.attempts+1 > s.v.limits.attempts {
		return runFailed("%s: the run reached limits.max_attempts %d", n.path, s.v.limits.attempts)
	}
	s.attempts++
	return nil
}

func (s *simulator) countTokens(n *node, tokens int64) error {
	if tokens < 0 {
		return fmt.Errorf("%s: fixture tokens must not be negative", n.path)
	}
	if n.kind == KindAgent && tokens > n.tokens {
		return fmt.Errorf("%s: the fixture reports %s tokens; the step's budget is %s", n.path, grouped(tokens), grouped(n.tokens))
	}
	s.tokens += tokens
	if s.tokens > s.v.limits.tokens {
		return runFailed("%s: the run exceeded limits.max_tokens %s", n.path, grouped(s.v.limits.tokens))
	}
	return nil
}

// attemptStep runs one step or item under its retry policy. A failure after
// the last attempt is returned as errRunFailed for steps and as an item
// error (itemErr) for map items.
func (s *simulator) attemptStep(n *node, fixture *ItemFixture, index *int, item any) (any, error) {
	input := s.resolveArgs(n, item)
	var lastErr string
	for attempt := 1; attempt <= n.attempts; attempt++ {
		if err := s.countAttempt(n); err != nil {
			return nil, err
		}
		e := TraceEntry{Step: n.id, Kind: n.kind, Item: index, Attempt: attempt, Input: input}
		out, note, tokens, failure, err := s.once(n, fixture, attempt, input)
		if err != nil {
			return nil, err
		}
		e.Tokens, e.Note = tokens, note
		if failure != "" {
			e.Status, e.Error = "failed", failure
			s.entry(e)
			if err := s.countTokens(n, tokens); err != nil {
				return nil, err
			}
			lastErr = failure
			continue
		}
		e.Status, e.Output = "succeeded", out
		if n.kind == KindCapability && n.cap.Effect {
			e.Status = "previewed"
			e.Note = "effect suppressed: not executed; the output is the fixture's simulated response"
			s.sim.Previews = append(s.sim.Previews, EffectPreview{Step: n.id, Capability: n.cap.ID, Contract: n.cap.Contract, Connection: n.step.Connection, Resource: n.step.Resource, Input: input})
		}
		s.entry(e)
		if err := s.countTokens(n, tokens); err != nil {
			return nil, err
		}
		return out, nil
	}
	return nil, itemFailed{fmt.Sprintf("%s failed after %s: %s", n.path, plural(n.attempts, "attempt"), lastErr)}
}

// itemFailed is a step or item that failed after its retries.
type itemFailed struct{ msg string }

func (e itemFailed) Error() string { return e.msg }

func fixtureAttempt(f *ItemFixture, attempt int) (AttemptFixture, bool) {
	if f == nil {
		return AttemptFixture{}, false
	}
	if len(f.Attempts) > 0 {
		if attempt > len(f.Attempts) {
			return AttemptFixture{}, false
		}
		return f.Attempts[attempt-1], true
	}
	if attempt == 1 && (len(f.Output) > 0 || f.Error != "") {
		return f.AttemptFixture, true
	}
	return AttemptFixture{}, false
}

// once performs one attempt. failure is the workflow-visible failure of
// this attempt; err is a fixture or internal problem.
func (s *simulator) once(n *node, fixture *ItemFixture, attempt int, input map[string]any) (out any, note string, tokens int64, failure string, err error) {
	if n.kind == KindDeterministic {
		result, callErr := n.fn.Call(input)
		if callErr != nil {
			return nil, "", 0, callErr.Error(), nil
		}
		if checkErr := checkValue(n.fn.Result, result); checkErr != nil {
			return nil, "", 0, "", fmt.Errorf("%s: %s returned a value outside its result type: %v", n.path, n.fn.ID(), checkErr)
		}
		return result, "", 0, "", nil
	}
	a, ok := fixtureAttempt(fixture, attempt)
	if !ok {
		return nil, "", 0, "", fmt.Errorf("%s: no fixture for attempt %d", n.path, attempt)
	}
	if a.Error != "" {
		if len(a.Output) > 0 {
			return nil, "", 0, "", fmt.Errorf("%s: attempt %d gives both output and error", n.path, attempt)
		}
		return nil, "", a.Tokens, a.Error, nil
	}
	value, parseErr := parseCanonicalJSON(a.Output)
	if parseErr != nil {
		return nil, "", 0, "", fmt.Errorf("%s: attempt %d output: %v", n.path, attempt, parseErr)
	}
	switch n.kind {
	case KindAgent:
		result, note, failure, err := agentResult(n, value, a.Tokens)
		return result, note, a.Tokens, failure, err
	case KindCapability:
		if checkErr := checkValue(n.output, value); checkErr != nil {
			return nil, "", a.Tokens, "malformed response: " + checkErr.Error(), nil
		}
	default:
		if checkErr := checkValue(n.output, value); checkErr != nil {
			return nil, "", a.Tokens, "invalid output: " + checkErr.Error(), nil
		}
	}
	return value, "", a.Tokens, "", nil
}

// agentFixture is what a worker returns: the harness's result fields.
type agentFixture struct {
	Status      string          `json:"status"`
	Summary     string          `json:"summary"`
	Report      string          `json:"report"`
	Sources     []agentSource   `json:"sources"`
	Limitations []string        `json:"limitations"`
	Result      json.RawMessage `json:"result"`
}

type agentSource struct {
	URL   string `json:"url"`
	Cited bool   `json:"cited"`
}

// agentResult builds the worker result an agent step sees, taking the typed
// result from the report's ## Result section as the runtime will.
func agentResult(n *node, value any, tokens int64) (any, string, string, error) {
	data, _ := canonicalBytes(value)
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var f agentFixture
	if err := dec.Decode(&f); err != nil {
		return nil, "", "", fmt.Errorf("%s: agent fixture: %v", n.path, err)
	}
	note := ""
	switch f.Status {
	case "succeeded":
	case "partial":
		note = "the worker wrapped up at its budget (partial)"
	case "failed", "cancelled", "interrupted":
		return nil, "", "worker ended " + f.Status, nil
	default:
		return nil, "", "", fmt.Errorf("%s: agent fixture status %q; use succeeded, partial, failed, cancelled or interrupted", n.path, f.Status)
	}
	report := f.Report
	if len(f.Result) > 0 {
		if report != "" {
			return nil, "", "", fmt.Errorf("%s: give an agent fixture's report or its result, not both", n.path)
		}
		resultValue, err := parseCanonicalJSON(f.Result)
		if err != nil {
			return nil, "", "", fmt.Errorf("%s: agent fixture result: %v", n.path, err)
		}
		block, _ := canonicalBytes(resultValue)
		report = "## Summary\n" + f.Summary + "\n\n## Result\n```json\n" + string(block) + "\n```\n"
	}
	if report == "" {
		report = "## Summary\n" + f.Summary + "\n"
	}
	sources := []any{}
	for _, src := range f.Sources {
		sources = append(sources, map[string]any{"url": src.URL, "cited": src.Cited})
	}
	limitations := []any{}
	for _, l := range f.Limitations {
		limitations = append(limitations, l)
	}
	out := map[string]any{
		"status": f.Status, "summary": f.Summary, "report": report,
		"sources": sources, "limitations": limitations, "tokens": tokens,
	}
	if n.schema != nil {
		result, err := extractAgentResult(report)
		if err == nil {
			err = checkValue(n.schema, result)
		}
		if err != nil {
			return nil, note, "typed result: " + err.Error(), nil
		}
		out["result"] = result
	}
	if err := checkValue(n.output, out); err != nil {
		return nil, "", "", fmt.Errorf("%s: agent fixture: %v", n.path, err)
	}
	return out, note, "", nil
}

var (
	resultHeading = regexp.MustCompile(`(?m)^##[ \t]+Result[ \t]*$`)
	nextHeading   = regexp.MustCompile(`(?m)^#{1,2}[ \t]`)
	jsonBlock     = regexp.MustCompile("(?ms)^```json[ \t]*\n(.*?)^```[ \t]*$")
)

// extractAgentResult takes the last fenced json block of the report's last
// "## Result" section.
func extractAgentResult(report string) (any, error) {
	headings := resultHeading.FindAllStringIndex(report, -1)
	if len(headings) == 0 {
		return nil, errors.New("the report has no ## Result section")
	}
	section := report[headings[len(headings)-1][1]:]
	if end := nextHeading.FindStringIndex(section); end != nil {
		section = section[:end[0]]
	}
	blocks := jsonBlock.FindAllStringSubmatch(section, -1)
	if len(blocks) == 0 {
		return nil, errors.New("the ## Result section has no ```json block")
	}
	value, err := parseCanonicalJSON([]byte(blocks[len(blocks)-1][1]))
	if err != nil {
		return nil, fmt.Errorf("the ## Result block is not valid JSON: %v", err)
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, errors.New("the ## Result block is not a JSON object")
	}
	return value, nil
}

func (s *simulator) mapStep(n *node) (any, error) {
	list, ok := s.resolve(*n.over, nil).([]any)
	if !ok {
		return nil, runFailed("%s: %s is not a list", n.path, n.over.Raw)
	}
	maxItems, parallel := n.step.MaxItems, min(n.step.MaxParallel, s.v.limits.parallel)
	if len(list) > maxItems {
		s.entry(TraceEntry{Step: n.id, Kind: n.kind, Status: "failed", Error: fmt.Sprintf("%d items; max_items %d", len(list), maxItems), Note: "no item started"})
		return nil, runFailed("%s: the list has %d items; max_items %d, so no item started", n.path, len(list), maxItems)
	}
	fixture := s.f.Steps[n.id]
	results, failures := []any{}, []any{}
	for i, element := range list {
		index := i
		var itemFixture *ItemFixture
		if f, ok := fixture.Items[strconv.Itoa(i)]; ok {
			itemFixture = &f
		} else if fixture.ItemDefault != nil {
			itemFixture = fixture.ItemDefault
		} else if n.item.kind != KindDeterministic {
			return nil, fmt.Errorf("%s: no fixture for item %d", n.path, i)
		}
		out, err := s.attemptStep(n.item, itemFixture, &index, element)
		var failed itemFailed
		switch {
		case errors.As(err, &failed):
			failures = append(failures, map[string]any{"index": int64(i), "item": element, "error": failed.msg})
			if n.step.OnItemFailure == "fail" {
				s.entry(TraceEntry{Step: n.id, Kind: n.kind, Status: "failed", Error: failed.msg, Note: "stopped at the first failed item"})
				return nil, runFailed("%s: item %d failed: %s", n.path, i, failed.msg)
			}
		case err != nil:
			return nil, err
		default:
			results = append(results, map[string]any{"index": int64(i), "item": element, "output": out})
		}
	}
	out := map[string]any{
		"results": results, "failures": failures,
		"succeeded": int64(len(results)), "failed": int64(len(failures)),
	}
	note := fmt.Sprintf("%d of %d items succeeded; simulated in index order, a run executes at most %d at once", len(results), len(list), parallel)
	if n.step.OnItemFailure == "partial" && len(results) < n.step.MinSucceeded {
		s.entry(TraceEntry{Step: n.id, Kind: n.kind, Status: "failed", Output: out, Note: note})
		return nil, runFailed("%s: %d of %d items succeeded; min_succeeded %d", n.path, len(results), len(list), n.step.MinSucceeded)
	}
	s.entry(TraceEntry{Step: n.id, Kind: n.kind, Status: "succeeded", Output: out, Note: note})
	return out, nil
}

func (s *simulator) branch(n *node) string {
	for i, c := range n.cases {
		if evaluate(c, s.resolve(c.ref, nil)) {
			s.entry(TraceEntry{Step: n.id, Kind: n.kind, Status: "branched", Next: c.next, Note: fmt.Sprintf("case %d matched: %s %s", i+1, c.ref.Raw, c.op)})
			return c.next
		}
	}
	s.entry(TraceEntry{Step: n.id, Kind: n.kind, Status: "branched", Next: n.step.Default, Note: "no case matched; default"})
	return n.step.Default
}

func evaluate(c compiledCase, v any) bool {
	switch c.op {
	case "empty", "not_empty":
		empty := false
		switch x := v.(type) {
		case nil:
			empty = true
		case []any:
			empty = len(x) == 0
		case string:
			empty = x == ""
		}
		return empty == (c.op == "empty")
	case "eq":
		return equalValues(v, c.value)
	case "ne":
		return !equalValues(v, c.value)
	}
	a, okA := number(v)
	b, okB := number(c.value)
	if !okA || !okB {
		return false
	}
	switch c.op {
	case "gt":
		return a > b
	case "gte":
		return a >= b
	case "lt":
		return a < b
	case "lte":
		return a <= b
	}
	return false
}

func (s *simulator) interrupt(n *node) (any, bool, error) {
	input := s.resolveArgs(n, nil)
	e := TraceEntry{Step: n.id, Kind: n.kind, Input: input}
	raw := s.f.Steps[n.id].Response
	if len(raw) == 0 {
		e.Status, e.Note = "waiting", n.step.Message
		s.entry(e)
		return nil, true, nil
	}
	if err := s.countAttempt(n); err != nil {
		return nil, false, err
	}
	value, err := parseCanonicalJSON(raw)
	if err != nil {
		return nil, false, fmt.Errorf("%s: response: %v", n.path, err)
	}
	if err := checkValue(n.schema, value); err != nil {
		e.Status, e.Error = "failed", err.Error()
		s.entry(e)
		return nil, false, runFailed("%s: the response is invalid: %v", n.path, err)
	}
	e.Status, e.Output, e.Note = "answered", value, n.step.Message
	s.entry(e)
	return value, false, nil
}

// Text renders the simulation for the terminal.
func (s *Simulation) Text() string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	line("Simulation of %s", s.Workflow)
	line("Executable hash: %s", s.ExecutableHash)
	line("Run key: %s", s.RunKey)
	line("Status: %s", s.Status)
	if s.Error != "" {
		line("Error: %s", s.Error)
	}
	var inputs []string
	for _, name := range sortedKeys(s.Inputs) {
		inputs = append(inputs, name+"="+formatValue(s.Inputs[name]))
	}
	line("Inputs: %s", strings.Join(inputs, ", "))
	line("Trace")
	for _, e := range s.Trace {
		label := e.Step
		if e.Item != nil {
			label = fmt.Sprintf("%s #%d", e.Step, *e.Item)
		}
		if e.Attempt > 0 {
			label += fmt.Sprintf(" attempt %d", e.Attempt)
		}
		text := fmt.Sprintf("  %s: %s", label, e.Status)
		if e.Status == "previewed" {
			for _, p := range s.Previews {
				if p.Step == e.Step {
					input, _ := canonicalBytes(p.Input)
					text = fmt.Sprintf("  PREVIEW %s %s %s on %s %s: %s", e.Step, p.Capability, p.Contract, p.Connection, p.Resource, input)
				}
			}
		}
		if e.Next != "" {
			text += " -> " + e.Next
		}
		if e.Tokens > 0 {
			text += fmt.Sprintf(" (%s tokens)", grouped(e.Tokens))
		}
		for _, detail := range []string{e.Error, e.Note} {
			if detail != "" {
				text += " - " + detail
			}
		}
		line("%s", text)
	}
	if len(s.Outputs) > 0 {
		line("Outputs")
		for _, name := range sortedKeys(s.Outputs) {
			data, _ := canonicalBytes(s.Outputs[name])
			line("  %s: %s", name, data)
		}
	}
	line("Totals: %s, %s tokens", plural(s.Attempts, "attempt"), grouped(s.Tokens))
	line("%s", s.Notice)
	return b.String()
}
