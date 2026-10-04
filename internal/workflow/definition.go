// Package workflow loads, validates, hashes, reviews and simulates reviewed
// Workflow Definitions (workflow-runtime.spec.md, Stage 14 sub-stage 1).
//
// A definition is a directory holding a strict workflow.yaml, a
// human-readable README.md, Markdown prompt files and JSON schemas. This
// package owns the closed Definition model, the accepted YAML subset, the
// validation rules, the executable hash, the review report and an
// effect-suppressed simulation over fixtures. It does not run workflows:
// activation, runs, leases and effects belong to later sub-stages. It
// imports no other Evie package and no network or process code, so nothing
// here can reach a model, worker or capability provider.
package workflow

import (
	"time"

	"gopkg.in/yaml.v3"
)

// SchemaVersion is the only workflow.yaml schema version this runtime reads.
const SchemaVersion = 1

// Step kinds. Map and agent were added by the 2026-10-04 amendment.
const (
	KindDeterministic = "deterministic"
	KindCapability    = "capability"
	KindAI            = "ai"
	KindBranch        = "branch"
	KindInterrupt     = "interrupt"
	KindMap           = "map"
	KindAgent         = "agent"
)

// Worker presets an agent step may run: the existing immutable subagent
// presets, by the same names.
const (
	WorkerResearch = "research"
	WorkerExplore  = "explore"
	WorkerBuild    = "build"
)

// Definition is the closed workflow.yaml schema. Fields holding run-time
// values (defaults, literal inputs, condition values) stay YAML nodes until
// validation converts them under the accepted subset's rules; a node of kind
// 0 was not written.
type Definition struct {
	SchemaVersion int                    `yaml:"schema_version"`
	Name          string                 `yaml:"name"`
	Description   string                 `yaml:"description"`
	Inputs        map[string]InputSpec   `yaml:"inputs"`
	RunKey        *RunKeyRule            `yaml:"run_key"`
	ModelPolicies map[string]ModelPolicy `yaml:"model_policies"`
	Limits        *Limits                `yaml:"limits"`
	Authority     *Authority             `yaml:"authority"`
	Schedule      *Schedule              `yaml:"schedule"`
	Retention     *Retention             `yaml:"retention"`
	Start         string                 `yaml:"start"`
	Steps         []Step                 `yaml:"steps"`
	Outputs       map[string]string      `yaml:"outputs"`
}

// InputSpec declares one typed run input.
type InputSpec struct {
	Type        string      `yaml:"type"`
	Description string      `yaml:"description"`
	Required    bool        `yaml:"required"`
	Default     yaml.Node   `yaml:"default"`
	Enum        []yaml.Node `yaml:"enum"`
	Minimum     *float64    `yaml:"minimum"`
	Maximum     *float64    `yaml:"maximum"`
	MaxLength   *int64      `yaml:"max_length"`
	Sensitive   bool        `yaml:"sensitive"`
}

// RunKeyRule names the inputs whose values identify one logical execution.
type RunKeyRule struct {
	Inputs []string `yaml:"inputs"`
}

// ModelPolicy pins the exact provider, model and parameters of AI and agent
// steps (ADR 0019).
type ModelPolicy struct {
	Version         int      `yaml:"version"`
	Provider        string   `yaml:"provider"`
	Model           string   `yaml:"model"`
	Temperature     *float64 `yaml:"temperature"`
	MaxOutputTokens int64    `yaml:"max_output_tokens"`
}

// Limits bound one run. Every field is required.
type Limits struct {
	MaxDuration string `yaml:"max_duration"`
	MaxAttempts int    `yaml:"max_attempts"`
	MaxParallel int    `yaml:"max_parallel"`
	MaxTokens   int64  `yaml:"max_tokens"`
}

// Authority is the requested Standing Authority: exact capability operations.
type Authority struct {
	Operations []Operation `yaml:"operations"`
}

// Operation is one requested capability operation on one connection's exact
// resources.
type Operation struct {
	Capability     string   `yaml:"capability"`
	Connection     string   `yaml:"connection"`
	Resources      []string `yaml:"resources"`
	MaxCallsPerRun int      `yaml:"max_calls_per_run"`
}

// Schedule is a reviewed Workflow Schedule.
type Schedule struct {
	Cron     string               `yaml:"cron"`
	Timezone string               `yaml:"timezone"`
	CatchUp  *CatchUp             `yaml:"catch_up"`
	Inputs   map[string]yaml.Node `yaml:"inputs"`
}

// CatchUp is the schedule's Catch-up Policy.
type CatchUp struct {
	Policy  string `yaml:"policy"`
	MaxRuns int    `yaml:"max_runs"`
}

// Retention classifies run records for redaction, retention and export.
type Retention struct {
	Classification string `yaml:"classification"`
}

// Step is one graph node, or a map step's item. Fields that do not belong to
// its kind are refused by validation, which uses the keys actually written.
type Step struct {
	ID          string               `yaml:"id"`
	Kind        string               `yaml:"kind"`
	Description string               `yaml:"description"`
	Next        string               `yaml:"next"`
	Retry       *Retry               `yaml:"retry"`
	Input       map[string]yaml.Node `yaml:"input"`

	Function string `yaml:"function"`

	Capability string `yaml:"capability"`
	Contract   string `yaml:"contract"`
	Connection string `yaml:"connection"`
	Resource   string `yaml:"resource"`

	ModelPolicy  string `yaml:"model_policy"`
	Prompt       string `yaml:"prompt"`
	OutputSchema string `yaml:"output_schema"`

	Worker    string  `yaml:"worker"`
	Objective string  `yaml:"objective"`
	Budget    *Budget `yaml:"budget"`

	Cases   []Case `yaml:"cases"`
	Default string `yaml:"default"`

	Message        string `yaml:"message"`
	ResponseSchema string `yaml:"response_schema"`

	Over          string `yaml:"over"`
	MaxItems      int    `yaml:"max_items"`
	MaxParallel   int    `yaml:"max_parallel"`
	OnItemFailure string `yaml:"on_item_failure"`
	MinSucceeded  int    `yaml:"min_succeeded"`
	Item          *Step  `yaml:"item"`

	keys map[string]bool
}

// Retry is a step's bounded Retry Policy.
type Retry struct {
	MaxAttempts int    `yaml:"max_attempts"`
	Backoff     string `yaml:"backoff"`
}

// Budget is an agent step's per-attempt time and token budget.
type Budget struct {
	Time   string `yaml:"time"`
	Tokens int64  `yaml:"tokens"`
}

// Case is one ordered branch case.
type Case struct {
	When *Condition `yaml:"when"`
	Next string     `yaml:"next"`
}

// Condition compares one referenced value with an optional literal.
type Condition struct {
	Ref   string    `yaml:"ref"`
	Op    string    `yaml:"op"`
	Value yaml.Node `yaml:"value"`
}

// Bounds of the closed schema.
const (
	maxRunDuration     = 168 * time.Hour
	maxRunAttempts     = 10_000
	maxRunParallel     = 32
	maxRunTokens       = 1_000_000_000
	maxStepAttempts    = 5
	maxBackoff         = 10 * time.Minute
	maxMapItems        = 1_000
	maxCallsPerRun     = 1_000
	maxCatchUpRuns     = 31
	maxRunKeyInputs    = 8
	maxOutputTokens    = 200_000
	maxMessageBytes    = 2_000
	maxDefinitionBytes = 256 << 10
	maxPromptBytes     = 64 << 10
	maxSchemaBytes     = 64 << 10
)
