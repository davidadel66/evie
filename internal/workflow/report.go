package workflow

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Report is what the owner reviews before approval: the graph, contracts,
// data flow, resources, policies, requested authority, schedule, retention,
// limits and the exact files behind the executable hash.
type Report struct {
	Workflow       string             `json:"workflow"`
	SchemaVersion  int                `json:"schema_version"`
	Description    string             `json:"description,omitempty"`
	ExecutableHash string             `json:"executable_hash"`
	Start          string             `json:"start"`
	Graph          []GraphNode        `json:"graph"`
	Inputs         []InputReport      `json:"inputs"`
	Outputs        []OutputReport     `json:"outputs"`
	RunKey         []string           `json:"run_key"`
	Capabilities   []CapabilityReport `json:"capabilities"`
	Workers        []WorkerReport     `json:"workers"`
	Functions      []FunctionReport   `json:"functions"`
	Resources      []ResourceReport   `json:"resources"`
	Retries        []RetryReport      `json:"retry_policies"`
	ModelPolicies  []PolicyReport     `json:"model_policies"`
	Authority      []AuthorityReport  `json:"requested_authority"`
	Schedule       *ScheduleReport    `json:"schedule,omitempty"`
	Retention      RetentionReport    `json:"retention"`
	Limits         LimitsReport       `json:"limits"`
	WorstCase      BoundsReport       `json:"worst_case"`
	Files          []FileReport       `json:"files"`
	Warnings       []string           `json:"warnings"`
}

// GraphNode is one step in graph order, with its summary and edges.
type GraphNode struct {
	ID      string   `json:"id"`
	Kind    string   `json:"kind"`
	Summary string   `json:"summary"`
	Item    string   `json:"item,omitempty"`
	Input   []string `json:"input,omitempty"`
	Next    []string `json:"next"`
	Line    string   `json:"line"`
}

type InputReport struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Summary     string `json:"summary"`
	Sensitive   bool   `json:"sensitive,omitempty"`
}

type OutputReport struct {
	Name   string `json:"name"`
	From   string `json:"from"`
	Always bool   `json:"always"`
	Step   string `json:"step,omitempty"`
}

// CapabilityReport is one required Capability Contract. An effect also
// reports who reconciles an unknown outcome ("provider" or "owner") and
// what its Resource Conflict Keys cover (Stage 14 sub-stage 6).
type CapabilityReport struct {
	ID           string `json:"id"`
	Contract     string `json:"contract"`
	Effect       bool   `json:"effect"`
	Idempotent   bool   `json:"idempotent,omitempty"`
	Reconcile    string `json:"reconcile,omitempty"`
	ConflictKeys string `json:"conflict_keys,omitempty"`
}

func capabilityReport(c *CapabilityContract) CapabilityReport {
	r := CapabilityReport{ID: c.ID, Contract: c.Contract, Effect: c.Effect, Idempotent: c.Idempotent}
	if c.Effect {
		r.Reconcile, r.ConflictKeys = "owner", c.ConflictKeys
		if c.Reconciles {
			r.Reconcile = "provider"
		}
		if r.ConflictKeys == "" {
			r.ConflictKeys = DefaultConflictKeys
		}
	}
	return r
}

// summary describes the contract's kind for review.
func (c CapabilityReport) summary() string {
	return EffectSummary(c.Effect, c.Idempotent, c.Reconcile == "provider", c.ConflictKeys)
}

type WorkerReport struct {
	Step         string `json:"step"`
	Worker       string `json:"worker"`
	MaxItems     int    `json:"max_items,omitempty"`
	Attempts     int    `json:"attempts"`
	BudgetTime   string `json:"budget_time"`
	BudgetTokens int64  `json:"budget_tokens"`
}

type FunctionReport struct {
	ID      string   `json:"id"`
	Summary string   `json:"summary"`
	Steps   []string `json:"steps"`
}

type ResourceReport struct {
	Connection string   `json:"connection"`
	Resource   string   `json:"resource"`
	Access     string   `json:"access"`
	Steps      []string `json:"steps"`
}

type RetryReport struct {
	Step     string `json:"step"`
	Attempts int    `json:"attempts"`
	Backoff  string `json:"backoff,omitempty"`
}

type PolicyReport struct {
	Name            string   `json:"name"`
	Version         int      `json:"version"`
	Provider        string   `json:"provider"`
	Model           string   `json:"model"`
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxOutputTokens int64    `json:"max_output_tokens"`
}

type AuthorityReport struct {
	Capability     string   `json:"capability"`
	Connection     string   `json:"connection"`
	Resources      []string `json:"resources"`
	MaxCallsPerRun int      `json:"max_calls_per_run"`
}

type ScheduleReport struct {
	Cron     string   `json:"cron"`
	Timezone string   `json:"timezone"`
	CatchUp  string   `json:"catch_up"`
	MaxRuns  int      `json:"max_runs,omitempty"`
	Inputs   []string `json:"inputs"`
}

type RetentionReport struct {
	Classification  string   `json:"classification"`
	SensitiveInputs []string `json:"sensitive_inputs,omitempty"`
}

type LimitsReport struct {
	MaxDuration string `json:"max_duration"`
	MaxAttempts int    `json:"max_attempts"`
	MaxParallel int    `json:"max_parallel"`
	MaxTokens   int64  `json:"max_tokens"`
}

type BoundsReport struct {
	AgentTokens int64 `json:"agent_tokens"`
	Attempts    int   `json:"attempts"`
}

type FileReport struct {
	Path   string `json:"path"`
	Role   string `json:"role"`
	Digest string `json:"digest"`
	Hashed bool   `json:"hashed"`
}

func (v *validator) buildReport(hash string) Report {
	def := v.def
	r := Report{
		Workflow: def.Name, SchemaVersion: def.SchemaVersion, Description: strings.TrimSpace(def.Description),
		ExecutableHash: hash, Start: def.Start, RunKey: def.RunKey.Inputs,
		Retention: RetentionReport{Classification: def.Retention.Classification},
		Limits:    LimitsReport{formatDuration(v.limits.duration), v.limits.attempts, v.limits.parallel, v.limits.tokens},
		WorstCase: BoundsReport{v.worstTokens, v.worstTries},
		Warnings:  append([]string{}, v.warnings...),
		Graph:     []GraphNode{}, Inputs: []InputReport{}, Outputs: []OutputReport{},
		Capabilities: []CapabilityReport{}, Workers: []WorkerReport{}, Functions: []FunctionReport{},
		Resources: []ResourceReport{}, Retries: []RetryReport{}, ModelPolicies: []PolicyReport{},
		Authority: []AuthorityReport{}, Files: []FileReport{},
	}
	caps := map[string]CapabilityReport{}
	resources := map[string]*ResourceReport{}
	fnSteps := map[string][]string{}
	for _, id := range v.order {
		n := v.steps[id]
		r.Graph = append(r.Graph, graphNode(n))
		for _, x := range []*node{n, n.item} {
			if x == nil {
				continue
			}
			label := x.id
			if x.isItem {
				label = x.id + " (each item)"
			}
			if x.attempts > 1 {
				r.Retries = append(r.Retries, RetryReport{Step: label, Attempts: x.attempts, Backoff: optionalDuration(x.backoff)})
			}
			if x.fn != nil {
				fnSteps[x.fn.ID()] = append(fnSteps[x.fn.ID()], x.id)
			}
			if x.kind == KindAgent {
				w := WorkerReport{Step: x.id, Worker: x.step.Worker, Attempts: x.attempts, BudgetTime: formatDuration(x.budget), BudgetTokens: x.tokens}
				if x.isItem {
					w.MaxItems = n.step.MaxItems
				}
				r.Workers = append(r.Workers, w)
			}
		}
		if n.cap != nil {
			caps[n.cap.ID] = capabilityReport(n.cap)
			key := n.step.Connection + "\x00" + n.step.Resource
			res := resources[key]
			if res == nil {
				res = &ResourceReport{Connection: n.step.Connection, Resource: n.step.Resource, Access: "read"}
				resources[key] = res
			}
			if n.cap.Effect {
				res.Access = "write"
			}
			res.Steps = append(res.Steps, n.id)
		}
	}
	for _, id := range sortedKeys(caps) {
		r.Capabilities = append(r.Capabilities, caps[id])
	}
	for _, key := range sortedKeys(resources) {
		r.Resources = append(r.Resources, *resources[key])
	}
	for _, id := range sortedKeys(fnSteps) {
		r.Functions = append(r.Functions, FunctionReport{ID: id, Summary: v.functions[id].Summary, Steps: fnSteps[id]})
	}
	for _, name := range sortedKeys(v.inputs) {
		in := v.inputs[name]
		r.Inputs = append(r.Inputs, InputReport{Name: name, Description: in.spec.Description, Summary: inputSummary(in), Sensitive: in.spec.Sensitive})
		if in.spec.Sensitive {
			r.Retention.SensitiveInputs = append(r.Retention.SensitiveInputs, name)
		}
	}
	for _, o := range v.outputs {
		r.Outputs = append(r.Outputs, OutputReport{Name: o.name, From: o.ref.Raw, Always: o.always, Step: stepOf(o.ref)})
	}
	for _, name := range sortedKeys(def.ModelPolicies) {
		m := def.ModelPolicies[name]
		r.ModelPolicies = append(r.ModelPolicies, PolicyReport{name, m.Version, m.Provider, m.Model, m.Temperature, m.MaxOutputTokens})
	}
	if def.Authority != nil {
		for _, op := range def.Authority.Operations {
			r.Authority = append(r.Authority, AuthorityReport{op.Capability, op.Connection, op.Resources, op.MaxCallsPerRun})
		}
	}
	if s := def.Schedule; s != nil {
		sr := &ScheduleReport{Cron: strings.Join(strings.Fields(s.Cron), " "), Timezone: s.Timezone, CatchUp: s.CatchUp.Policy, MaxRuns: s.CatchUp.MaxRuns}
		for _, name := range sortedKeys(s.Inputs) {
			node := s.Inputs[name]
			value, _ := nodeValue(&node)
			text, isText := value.(string)
			if !isText || !isRefText(text) {
				text = formatValue(value)
			}
			sr.Inputs = append(sr.Inputs, name+"="+text)
		}
		r.Schedule = sr
	}
	r.Files = append(r.Files, FileReport{Path: "README.md", Role: "documentation", Digest: v.readmeDigest(), Hashed: false})
	for _, p := range sortedKeys(v.files) {
		role := "prompt"
		if strings.HasSuffix(p, ".json") {
			role = "schema"
		}
		r.Files = append(r.Files, FileReport{Path: p, Role: role, Digest: "sha256:" + digest(v.files[p]), Hashed: true})
	}
	return r
}

func (v *validator) readmeDigest() string {
	if v.readme == nil {
		return ""
	}
	return "sha256:" + digest(v.readme)
}

func stepOf(r Ref) string {
	if r.Root == refSteps {
		return r.Name
	}
	return ""
}

func inputSummary(in *inputInfo) string {
	parts := []string{string(in.t.Kind)}
	if in.spec.Required {
		parts = append(parts, "required")
	}
	if in.hasDefault {
		parts = append(parts, "default "+formatValue(in.def))
	}
	if len(in.t.Enum) > 0 {
		parts = append(parts, "one of "+formatEnum(in.t.Enum))
	}
	if in.t.Minimum != nil {
		parts = append(parts, "minimum "+formatNumber(*in.t.Minimum))
	}
	if in.t.Maximum != nil {
		parts = append(parts, "maximum "+formatNumber(*in.t.Maximum))
	}
	if in.t.MaxLength != nil {
		parts = append(parts, fmt.Sprintf("max length %d", *in.t.MaxLength))
	}
	if in.spec.Sensitive {
		parts = append(parts, "sensitive")
	}
	return strings.Join(parts, ", ")
}

func stepSummary(n *node) string {
	s := n.step
	switch n.kind {
	case KindDeterministic:
		return s.Function
	case KindCapability:
		kind := "read"
		if n.cap != nil && n.cap.Effect {
			kind = "effect"
			if n.cap.Idempotent {
				kind = "effect, idempotent"
			}
		}
		return fmt.Sprintf("%s %s (%s) on %s %s", s.Capability, s.Contract, kind, s.Connection, s.Resource)
	case KindAI:
		return fmt.Sprintf("model %s, prompt %s, output %s", s.ModelPolicy, s.Prompt, s.OutputSchema)
	case KindAgent:
		out := fmt.Sprintf("worker %s, model %s, objective %s, budget %s / %s tokens", s.Worker, s.ModelPolicy, s.Objective, formatDuration(n.budget), grouped(n.tokens))
		if s.OutputSchema != "" {
			out += ", output " + s.OutputSchema
		}
		return out
	case KindMap:
		policy := "stop at the first failed item"
		if s.OnItemFailure == "partial" {
			policy = fmt.Sprintf("partial (at least %d must succeed)", s.MinSucceeded)
			if s.MinSucceeded == 0 {
				policy = "partial (items may all fail)"
			}
		}
		return fmt.Sprintf("over %s: at most %d items, %d at once, %s", s.Over, s.MaxItems, s.MaxParallel, policy)
	case KindBranch:
		var cases []string
		for _, c := range s.Cases {
			cond := c.When.Ref + " " + c.When.Op
			if c.When.Value.Kind != 0 {
				value, _ := nodeValue(&c.When.Value)
				cond += " " + formatValue(value)
			}
			cases = append(cases, cond+" -> "+c.Next)
		}
		return strings.Join(cases, "; ") + "; otherwise -> " + s.Default
	case KindInterrupt:
		return "response " + s.ResponseSchema
	}
	return ""
}

func inputLines(n *node) []string {
	var out []string
	for _, name := range sortedKeys(n.step.Input) {
		node := n.step.Input[name]
		value, _ := nodeValue(&node)
		text, isText := value.(string)
		if !isText || !isRefText(text) {
			text = formatValue(value)
		}
		out = append(out, name+"="+text)
	}
	return out
}

func graphNode(n *node) GraphNode {
	g := GraphNode{ID: n.id, Kind: n.kind, Summary: stepSummary(n), Next: append([]string{}, n.next...), Input: inputLines(n)}
	if n.item != nil {
		g.Item = fmt.Sprintf("item [%s] %s", n.item.kind, stepSummary(n.item))
		g.Input = inputLines(n.item)
	}
	g.Line = fmt.Sprintf("%s [%s] %s", n.id, n.kind, g.Summary)
	if n.kind != KindBranch {
		next := "end"
		if len(n.next) > 0 {
			next = n.next[0]
		}
		g.Line += " -> " + next
	}
	return g
}

// Text renders the report for the terminal.
func (r Report) Text() string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	section := func(title string, empty bool, items []string) {
		if empty {
			line("%s: none", title)
			return
		}
		line("%s", title)
		for _, item := range items {
			line("  %s", item)
		}
	}
	line("Workflow %s (schema %d)", r.Workflow, r.SchemaVersion)
	if r.Description != "" {
		line("%s", r.Description)
	}
	line("Executable hash: %s", r.ExecutableHash)
	line("")
	line("Graph (start: %s)", r.Start)
	for i, g := range r.Graph {
		line("  %d. %s", i+1, g.Line)
		if g.Item != "" {
			line("     %s", g.Item)
		}
		if len(g.Input) > 0 {
			label := "input"
			if g.Item != "" {
				label = "item input"
			}
			line("     %s: %s", label, strings.Join(g.Input, ", "))
		}
	}
	line("")
	var items []string
	for _, in := range r.Inputs {
		items = append(items, in.Name+": "+in.Summary)
	}
	section("Inputs", len(items) == 0, items)
	items = nil
	for _, o := range r.Outputs {
		when := "always"
		if !o.Always {
			when = "only when " + o.Step + " runs"
		}
		items = append(items, fmt.Sprintf("%s: %s (%s)", o.Name, o.From, when))
	}
	section("Outputs", len(items) == 0, items)
	line("Run key: %s + %s", r.Workflow, strings.Join(r.RunKey, ", "))
	items = nil
	for _, c := range r.Capabilities {
		items = append(items, fmt.Sprintf("%s %s: %s", c.ID, c.Contract, c.summary()))
	}
	section("Capability contracts", len(items) == 0, items)
	items = nil
	for _, w := range r.Workers {
		attempts := plural(w.Attempts, "attempt")
		if w.MaxItems > 0 {
			attempts = fmt.Sprintf("up to %d items x %s", w.MaxItems, attempts)
		}
		items = append(items, fmt.Sprintf("%s: %s worker, %s, %s / %s tokens each", w.Step, w.Worker, attempts, w.BudgetTime, grouped(w.BudgetTokens)))
	}
	section("Workers", len(items) == 0, items)
	items = nil
	for _, f := range r.Functions {
		items = append(items, fmt.Sprintf("%s: %s (%s)", f.ID, f.Summary, strings.Join(f.Steps, ", ")))
	}
	section("Deterministic functions", len(items) == 0, items)
	items = nil
	for _, res := range r.Resources {
		items = append(items, fmt.Sprintf("%s %s: %s (steps: %s)", res.Connection, res.Resource, res.Access, strings.Join(res.Steps, ", ")))
	}
	section("Resources", len(items) == 0, items)
	items = nil
	for _, rp := range r.Retries {
		item := fmt.Sprintf("%s: %s", rp.Step, plural(rp.Attempts, "attempt"))
		if rp.Backoff != "" {
			item += ", backoff " + rp.Backoff
		}
		items = append(items, item)
	}
	items = append(items, "every other step: 1 attempt")
	section("Retry policies", false, items)
	items = nil
	for _, m := range r.ModelPolicies {
		item := fmt.Sprintf("%s v%d: %s %s", m.Name, m.Version, m.Provider, m.Model)
		if m.Temperature != nil {
			item += ", temperature " + formatNumber(*m.Temperature)
		}
		items = append(items, item+", max output "+grouped(m.MaxOutputTokens)+" tokens")
	}
	section("Model policies", len(items) == 0, items)
	items = nil
	for _, a := range r.Authority {
		items = append(items, fmt.Sprintf("%s on %s: %s, at most %s per run", a.Capability, a.Connection, strings.Join(a.Resources, ", "), plural(a.MaxCallsPerRun, "call")))
	}
	section("Requested Standing Authority", len(items) == 0, items)
	if s := r.Schedule; s != nil {
		catchUp := "skip missed runs"
		if s.CatchUp == "each_missed" {
			catchUp = fmt.Sprintf("catch up each missed run (at most %d)", s.MaxRuns)
		}
		line("Schedule: %s %s, %s, inputs %s", s.Cron, s.Timezone, catchUp, strings.Join(s.Inputs, ", "))
	} else {
		line("Schedule: none (manual starts only)")
	}
	retention := "Retention: " + r.Retention.Classification
	if len(r.Retention.SensitiveInputs) > 0 {
		retention += "; sensitive inputs: " + strings.Join(r.Retention.SensitiveInputs, ", ")
	}
	line("%s", retention)
	line("Limits: max duration %s, %d attempts, %d at once, %s tokens", r.Limits.MaxDuration, r.Limits.MaxAttempts, r.Limits.MaxParallel, grouped(r.Limits.MaxTokens))
	line("Worst case: %s agent tokens, %d attempts", grouped(r.WorstCase.AgentTokens), r.WorstCase.Attempts)
	items = nil
	for _, f := range r.Files {
		note := ""
		if !f.Hashed {
			note = " (" + f.Role + ", not hashed)"
		}
		items = append(items, fmt.Sprintf("%s%s %s", f.Path, note, f.Digest))
	}
	section("Files", false, items)
	if len(r.Warnings) > 0 {
		section("Warnings", false, r.Warnings)
	}
	return b.String()
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}

// grouped writes an integer with thousands separators.
func grouped[N int | int64](n N) string {
	s := strconv.FormatInt(int64(n), 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	out := strings.Join(parts, ",")
	if neg {
		out = "-" + out
	}
	return out
}

// formatDuration writes a duration compactly: 2h, 10m, 1h30m, 2s.
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	var parts []string
	for _, u := range []struct {
		unit time.Duration
		name string
	}{{time.Hour, "h"}, {time.Minute, "m"}, {time.Second, "s"}, {time.Millisecond, "ms"}} {
		if n := d / u.unit; n > 0 {
			parts = append(parts, fmt.Sprintf("%d%s", n, u.name))
			d -= n * u.unit
		}
	}
	if len(parts) == 0 {
		return d.String()
	}
	return strings.Join(parts, "")
}

func optionalDuration(d time.Duration) string {
	if d == 0 {
		return ""
	}
	return formatDuration(d)
}
