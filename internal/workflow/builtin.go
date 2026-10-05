package workflow

import (
	"fmt"
	"slices"
)

// Built-in and Global workflows (owner decision 2026-10-04; the "Built-in
// workflows" amendment of workflow-runtime.spec.md). A built-in is a
// reviewed definition compiled into Evie that every Workspace may run
// without a proposal or approval; a Global custom definition is the
// owner's approved copy that every Workspace may run. Neither is approved
// for one Workspace, so neither may hold anything a Workflow Approval
// grants (ADR 0008): QualifyBuiltin is the rule both must satisfy, checked
// when Evie records a built-in version, when the owner proposes or
// approves a Global definition, and again when a run of either starts.
//
// A definition qualifies when it
//
//  1. calls no capability (reads and effects both need Standing Authority,
//     which only a Workspace approval grants);
//  2. requests no Standing Authority;
//  3. declares no schedule (a schedule starts unattended runs, which only
//     an approval activates, ADR 0020);
//  4. runs only the read-only research and explore workers (no build
//     worker, so nothing writes a repository and nothing can reach the
//     merge gate).
//
// Interruptions, AI, deterministic, branch and map steps are allowed: they
// have no effect outside Evie. A Workspace custom definition, approved for
// one Workspace with its own authority, is not bound by this rule.

// standaloneWorkers are the worker presets a built-in or Global definition
// may run.
var standaloneWorkers = []string{WorkerResearch, WorkerExplore}

// QualifyBuiltin lists why a validated definition's report cannot be a
// built-in or Global workflow; none means it qualifies.
func QualifyBuiltin(r Report) []string {
	var out []string
	for _, n := range r.Graph {
		if n.Kind == KindCapability {
			out = append(out, fmt.Sprintf("steps.%s calls a capability; built-in and Global workflows hold no Standing Authority, so they call no capability", n.ID))
		}
	}
	if len(r.Authority) > 0 {
		out = append(out, "it requests Standing Authority; built-in and Global workflows request none (only a Workspace approval grants it)")
	}
	if r.Schedule != nil {
		out = append(out, "it declares a schedule; built-in and Global workflows start only when you or a chat start them")
	}
	for _, w := range r.Workers {
		if !slices.Contains(standaloneWorkers, w.Worker) {
			out = append(out, fmt.Sprintf("steps.%s runs %s workers; built-in and Global workflows run only the read-only research and explore workers", w.Step, w.Worker))
		}
	}
	return out
}

// The built-ins are the definitions embedded under the bundled root (see
// templates.go): each folder is one built-in named after it, with a
// fixtures.json dry run beside its reviewed files, which customizing it
// uses and which is never committed.

// Builtins lists the built-in workflows by name.
func Builtins() []TemplateInfo { return Templates() }

// IsBuiltin reports whether Evie has a built-in workflow of that name.
func IsBuiltin(name string) bool {
	_, ok := BuiltinSource(name)
	return ok
}

// BuiltinSource reads one built-in's files.
func BuiltinSource(name string) (Source, bool) { return TemplateSource(name) }

// BuiltinFixtures is the dry run bundled with a built-in.
func BuiltinFixtures(name string) (Fixtures, error) { return TemplateFixtures(name) }
