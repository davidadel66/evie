package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// hashFormat names the canonical document the executable hash covers. A
// change to canonicalization is a new format and changes every hash.
const hashFormat = "evie.workflow.executable/1"

// The normalized definition: everything that affects behavior, with
// descriptions left out, defaults applied, durations in milliseconds and
// order-insensitive collections sorted. README.md, comments, quoting and
// formatting never reach it.
type normDefinition struct {
	SchemaVersion int           `json:"schema_version"`
	Name          string        `json:"name"`
	Inputs        []normInput   `json:"inputs"`
	RunKey        []string      `json:"run_key"`
	ModelPolicies []normPolicy  `json:"model_policies"`
	Limits        normLimits    `json:"limits"`
	Authority     []normOp      `json:"authority"`
	Schedule      *normSchedule `json:"schedule,omitempty"`
	Retention     string        `json:"retention"`
	Start         string        `json:"start"`
	Steps         []normStep    `json:"steps"`
	Outputs       []normPair    `json:"outputs"`
}

type normInput struct {
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Required  bool     `json:"required,omitempty"`
	Default   any      `json:"default,omitempty"`
	Enum      []any    `json:"enum,omitempty"`
	Minimum   *float64 `json:"minimum,omitempty"`
	Maximum   *float64 `json:"maximum,omitempty"`
	MaxLength *int64   `json:"max_length,omitempty"`
	Sensitive bool     `json:"sensitive,omitempty"`
}

type normPolicy struct {
	Name            string   `json:"name"`
	Version         int      `json:"version"`
	Provider        string   `json:"provider"`
	Model           string   `json:"model"`
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxOutputTokens int64    `json:"max_output_tokens"`
}

type normLimits struct {
	MaxDurationMS int64 `json:"max_duration_ms"`
	MaxAttempts   int   `json:"max_attempts"`
	MaxParallel   int   `json:"max_parallel"`
	MaxTokens     int64 `json:"max_tokens"`
}

type normOp struct {
	Capability     string   `json:"capability"`
	Connection     string   `json:"connection"`
	Resources      []string `json:"resources"`
	MaxCallsPerRun int      `json:"max_calls_per_run"`
}

type normSchedule struct {
	Cron           string     `json:"cron"`
	Timezone       string     `json:"timezone"`
	CatchUpPolicy  string     `json:"catch_up_policy"`
	CatchUpMaxRuns int        `json:"catch_up_max_runs,omitempty"`
	Inputs         []normPair `json:"inputs"`
}

type normPair struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

type normCase struct {
	Ref   string `json:"ref"`
	Op    string `json:"op"`
	Value any    `json:"value,omitempty"`
	Next  string `json:"next"`
}

type normStep struct {
	ID             string     `json:"id,omitempty"`
	Kind           string     `json:"kind"`
	Next           string     `json:"next,omitempty"`
	Attempts       int        `json:"attempts,omitempty"`
	BackoffMS      int64      `json:"backoff_ms,omitempty"`
	Input          []normPair `json:"input,omitempty"`
	Function       string     `json:"function,omitempty"`
	Capability     string     `json:"capability,omitempty"`
	Contract       string     `json:"contract,omitempty"`
	Connection     string     `json:"connection,omitempty"`
	Resource       string     `json:"resource,omitempty"`
	ModelPolicy    string     `json:"model_policy,omitempty"`
	Prompt         string     `json:"prompt,omitempty"`
	OutputSchema   string     `json:"output_schema,omitempty"`
	Worker         string     `json:"worker,omitempty"`
	Objective      string     `json:"objective,omitempty"`
	BudgetMS       int64      `json:"budget_ms,omitempty"`
	BudgetTokens   int64      `json:"budget_tokens,omitempty"`
	Cases          []normCase `json:"cases,omitempty"`
	Default        string     `json:"default,omitempty"`
	Message        string     `json:"message,omitempty"`
	ResponseSchema string     `json:"response_schema,omitempty"`
	Over           string     `json:"over,omitempty"`
	MaxItems       int        `json:"max_items,omitempty"`
	MaxParallel    int        `json:"max_parallel,omitempty"`
	OnItemFailure  string     `json:"on_item_failure,omitempty"`
	MinSucceeded   int        `json:"min_succeeded,omitempty"`
	Item           *normStep  `json:"item,omitempty"`
}

// executableHash canonicalizes a valid definition with the content of its
// referenced prompts and schemas and the identities of its functions.
func executableHash(def *Definition, files map[string][]byte, functions map[string]Function) (string, error) {
	norm, prompts, schemas, fns, err := normalize(def)
	if err != nil {
		return "", err
	}
	doc := struct {
		Format     string            `json:"format"`
		Definition normDefinition    `json:"definition"`
		Prompts    map[string]string `json:"prompts"`
		Schemas    map[string]string `json:"schemas"`
		Functions  map[string]string `json:"functions"`
	}{Format: hashFormat, Definition: norm, Prompts: map[string]string{}, Schemas: map[string]string{}, Functions: map[string]string{}}
	for _, p := range prompts {
		data, ok := files[p]
		if !ok {
			return "", fmt.Errorf("prompt %s was not loaded", p)
		}
		doc.Prompts[p] = digest(data)
	}
	for _, p := range schemas {
		data, ok := files[p]
		if !ok {
			return "", fmt.Errorf("schema %s was not loaded", p)
		}
		_, canonical, err := parseSchema(data)
		if err != nil {
			return "", err
		}
		doc.Schemas[p] = digest(canonical)
	}
	for _, id := range fns {
		f, ok := functions[id]
		if !ok {
			return "", fmt.Errorf("function %s is not registered", id)
		}
		sig, err := signatureDigest(f)
		if err != nil {
			return "", err
		}
		doc.Functions[id] = sig
	}
	data, err := canonicalBytes(doc)
	if err != nil {
		return "", err
	}
	return "sha256:" + digest(data), nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// signatureDigest identifies a function's reviewed contract: name, version,
// parameters with their bounds, and result type.
func signatureDigest(f Function) (string, error) {
	data, err := canonicalBytes(struct {
		Name    string  `json:"name"`
		Version int     `json:"version"`
		Params  []Param `json:"params"`
		Result  *Type   `json:"result"`
	}{f.Name, f.Version, f.Params, f.Result})
	if err != nil {
		return "", err
	}
	return digest(data), nil
}

func ms(s string) int64 {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d.Milliseconds()
}

func normalize(def *Definition) (normDefinition, []string, []string, []string, error) {
	n := normDefinition{SchemaVersion: def.SchemaVersion, Name: def.Name, Start: def.Start}
	prompts, schemas, fns := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, name := range sortedKeys(def.Inputs) {
		spec := def.Inputs[name]
		in := normInput{Name: name, Type: spec.Type, Required: spec.Required, Minimum: spec.Minimum, Maximum: spec.Maximum, MaxLength: spec.MaxLength, Sensitive: spec.Sensitive}
		if spec.Default.Kind != 0 {
			d, err := nodeValue(&spec.Default)
			if err != nil {
				return n, nil, nil, nil, err
			}
			in.Default = d
		}
		for i := range spec.Enum {
			e, err := nodeValue(&spec.Enum[i])
			if err != nil {
				return n, nil, nil, nil, err
			}
			in.Enum = append(in.Enum, e)
		}
		n.Inputs = append(n.Inputs, in)
	}
	if def.RunKey != nil {
		n.RunKey = sortedStrings(def.RunKey.Inputs)
	}
	for _, name := range sortedKeys(def.ModelPolicies) {
		m := def.ModelPolicies[name]
		n.ModelPolicies = append(n.ModelPolicies, normPolicy{name, m.Version, m.Provider, m.Model, m.Temperature, m.MaxOutputTokens})
	}
	if l := def.Limits; l != nil {
		n.Limits = normLimits{ms(l.MaxDuration), l.MaxAttempts, l.MaxParallel, l.MaxTokens}
	}
	if def.Authority != nil {
		for _, op := range def.Authority.Operations {
			n.Authority = append(n.Authority, normOp{op.Capability, op.Connection, sortedStrings(op.Resources), op.MaxCallsPerRun})
		}
		sort.Slice(n.Authority, func(i, j int) bool {
			a, b := n.Authority[i], n.Authority[j]
			return a.Capability+"\x00"+a.Connection < b.Capability+"\x00"+b.Connection
		})
	}
	if s := def.Schedule; s != nil {
		ns := &normSchedule{Cron: strings.Join(strings.Fields(s.Cron), " "), Timezone: s.Timezone}
		if s.CatchUp != nil {
			ns.CatchUpPolicy, ns.CatchUpMaxRuns = s.CatchUp.Policy, s.CatchUp.MaxRuns
		}
		pairs, err := normValues(s.Inputs)
		if err != nil {
			return n, nil, nil, nil, err
		}
		ns.Inputs = pairs
		n.Schedule = ns
	}
	if def.Retention != nil {
		n.Retention = def.Retention.Classification
	}
	var step func(s *Step, isItem bool) (*normStep, error)
	step = func(s *Step, isItem bool) (*normStep, error) {
		ns := &normStep{
			Kind: s.Kind, Next: s.Next, Function: s.Function,
			Capability: s.Capability, Contract: s.Contract, Connection: s.Connection, Resource: s.Resource,
			ModelPolicy: s.ModelPolicy, Prompt: s.Prompt, OutputSchema: s.OutputSchema,
			Worker: s.Worker, Objective: s.Objective, Default: s.Default,
			Message: s.Message, ResponseSchema: s.ResponseSchema,
			Over: s.Over, MaxItems: s.MaxItems, MaxParallel: s.MaxParallel, OnItemFailure: s.OnItemFailure, MinSucceeded: s.MinSucceeded,
		}
		if !isItem {
			ns.ID = s.ID
		}
		if s.Kind != KindBranch && s.Kind != KindMap && s.Kind != KindInterrupt {
			ns.Attempts = 1
			if s.Retry != nil {
				ns.Attempts, ns.BackoffMS = s.Retry.MaxAttempts, ms(s.Retry.Backoff)
			}
		}
		if s.Budget != nil {
			ns.BudgetMS, ns.BudgetTokens = ms(s.Budget.Time), s.Budget.Tokens
		}
		input, err := normValues(s.Input)
		if err != nil {
			return nil, err
		}
		ns.Input = input
		for _, c := range s.Cases {
			nc := normCase{Next: c.Next}
			if c.When != nil {
				nc.Ref, nc.Op = c.When.Ref, c.When.Op
				if c.When.Value.Kind != 0 {
					v, err := nodeValue(&c.When.Value)
					if err != nil {
						return nil, err
					}
					nc.Value = v
				}
			}
			ns.Cases = append(ns.Cases, nc)
		}
		for _, p := range []string{s.Prompt, s.Objective} {
			if p != "" {
				prompts[p] = true
			}
		}
		for _, p := range []string{s.OutputSchema, s.ResponseSchema} {
			if p != "" {
				schemas[p] = true
			}
		}
		if s.Function != "" {
			fns[s.Function] = true
		}
		if s.Item != nil {
			item, err := step(s.Item, true)
			if err != nil {
				return nil, err
			}
			ns.Item = item
		}
		return ns, nil
	}
	for i := range def.Steps {
		ns, err := step(&def.Steps[i], false)
		if err != nil {
			return n, nil, nil, nil, err
		}
		n.Steps = append(n.Steps, *ns)
	}
	sort.Slice(n.Steps, func(i, j int) bool { return n.Steps[i].ID < n.Steps[j].ID })
	for _, name := range sortedKeys(def.Outputs) {
		n.Outputs = append(n.Outputs, normPair{name, def.Outputs[name]})
	}
	return n, sortedKeys(prompts), sortedKeys(schemas), sortedKeys(fns), nil
}

func normValues(m map[string]yaml.Node) ([]normPair, error) {
	var out []normPair
	for _, name := range sortedKeys(m) {
		node := m[name]
		v, err := nodeValue(&node)
		if err != nil {
			return nil, err
		}
		out = append(out, normPair{name, v})
	}
	return out, nil
}
