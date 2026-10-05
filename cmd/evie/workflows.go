package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/davidadel66/evie/internal/workflow"
)

const workflowsUsage = "usage: evie workflows validate [--json] <dir> | evie workflows simulate [--json] <dir> --fixtures <file>" +
	" | evie workflows templates [--json]" +
	" | evie workflows propose [--json] <workspace-id> (<dir> | --template <name>) [--fixtures <file>] | evie workflows list [--json] [<workspace-id>]" +
	" | evie workflows show [--json] <workspace-id> <name> | evie workflows diff [--json] <proposal-id>" +
	" | evie workflows approve [--json] <proposal-id> --hash <executable-hash> --authority <authority-hash> --expect-version <n>" +
	" | evie workflows deactivate [--json] <workspace-id> <name> --version <n> [--reason <text>]" +
	" | evie workflows run [--json] [--detach] <workspace-id> <name> [--input <name>=<value>]... [--input-json <file>] [--rerun-of <run-id> --reason <text>]" +
	" | evie workflows runs [--json] [<workspace-id>] [--workflow <name>] | evie workflows inspect [--json] <run-id>" +
	" | evie workflows report [--json] <run-id> <execution-id>" +
	" | evie workflows substitute [--json] <run-id> <capability> <version>" +
	" | evie workflows reconcile [--json] [--detach] <run-id> <step> --outcome happened|did-not-happen [--evidence <text>] [--output-json <file>]" +
	" | evie workflows answer [--json] [--detach] <run-id> <step> (--answer <json> | --answer-file <file>) [--session <session-id>]" +
	" | evie workflows interruptions [--json] [--all] [<workspace-id>] [--session <session-id>] [--run <run-id>]" +
	" | evie workflows cancel [--json] [--detach] <run-id> [--reason <text>]" +
	" | evie workflows schedules [--json] [<workspace-id>]" +
	" | evie workflows schedule pause|resume [--json] <workspace-id> <name> [--version <n>] [--reason <text>]" +
	" | evie workflows notifications [--json] [--all] [<workspace-id>] [--session <session-id>] [--run <run-id>] [--limit <n>]" +
	" | evie workflows notifications read|unread [--json] <id>... | evie workflows notifications read --all [--workspace <workspace-id>]" +
	" | evie workflows status [--json] [<workspace-id>]" +
	" | evie workflows migrate [--json] [--detach] <run-id> --to-version <n> [--reason <text>]" +
	" | evie workflows retention status [--json] | evie workflows retention sweep [--dry-run] [--json]"

// workflowActivationCommands need the database and the procedural
// repository; runWorkflowActivation answers them once both are available
// (Stage 14 sub-stage 2; run, runs and inspect from sub-stage 3; substitute
// from sub-stage 5; reconcile from sub-stage 6; answer and interruptions
// from sub-stage 7; cancel from sub-stage 8; report from sub-stage 10;
// schedules, schedule, notifications and status from sub-stage 11; migrate
// and retention from sub-stage 12).
var workflowActivationCommands = []string{"propose", "list", "show", "diff", "approve", "deactivate", "run", "runs", "inspect", "substitute", "reconcile",
	"answer", "interruptions", "cancel", "report", "schedules", "schedule", "notifications", "status", "migrate", "retention"}

// accessNotChecked is printed with a standalone review.
const accessNotChecked = "Workspace Access is not checked outside a Workspace; evie workflows propose <workspace-id> <dir> checks it.\n"

// runWorkflowManagement answers `evie workflows validate <dir>`, which prints
// a Workflow Definition's review report, and `evie workflows simulate <dir>
// --fixtures <file>`, which prints an effect-suppressed simulation trace
// (Stage 14 sub-stage 1, 2026-10-04). Both work on any directory and touch
// no database. Since sub-stage 5 the definition is reviewed against the
// workflow capabilities compiled into Evie; Workspace Access belongs to a
// Workspace, so it is not checked here, and proposing the definition to a
// Workspace (`evie workflows propose`, sub-stage 2) checks it. Agent
// budgets are bounded by the operator's subagent policy. The activation
// commands are left to runWorkflowActivation, after the database opens.
func runWorkflowManagement(_ context.Context, args []string, out io.Writer, getenv func(string) string) (bool, error) {
	if len(args) == 0 || args[0] != "workflows" {
		return false, nil
	}
	usage := errors.New(workflowsUsage)
	if len(args) >= 2 && slices.Contains(workflowActivationCommands, args[1]) {
		return false, nil
	}
	if len(args) >= 2 && args[1] == "templates" {
		return true, listWorkflowTemplates(args[2:], out, usage)
	}
	if len(args) < 2 || (args[1] != "validate" && args[1] != "simulate") {
		return true, usage
	}
	var dir, fixtures string
	asJSON, haveFixtures := false, false
	for i := 2; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--json":
			asJSON = true
		case a == "--fixtures":
			if i+1 >= len(args) || haveFixtures {
				return true, usage
			}
			fixtures, haveFixtures = args[i+1], true
			i++
		case strings.HasPrefix(a, "-"), dir != "":
			return true, usage
		default:
			dir = a
		}
	}
	if dir == "" || haveFixtures != (args[1] == "simulate") {
		return true, usage
	}
	policy, err := configuredSubagentPolicy(getenv)
	if err != nil {
		return true, fmt.Errorf("invalid Subagents configuration: %w", err)
	}
	catalog, err := workflowCapabilityCatalog()
	if err != nil {
		return true, err
	}
	env := workflow.Environment{
		Capabilities:    catalog,
		Workers:         workflow.WorkerCeiling{Deadline: policy.Deadline, Tokens: policy.TokenBudget},
		AccessUnchecked: true,
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	validated, err := workflow.Validate(workflow.DirSource{Dir: dir}, env)
	var invalid *workflow.InvalidError
	if errors.As(err, &invalid) {
		if asJSON {
			if err := encoder.Encode(map[string]any{"valid": false, "problems": invalid.Problems}); err != nil {
				return true, err
			}
		} else {
			fmt.Fprintf(out, "%s is not a valid Workflow Definition:\n", dir)
			for _, p := range invalid.Problems {
				fmt.Fprintf(out, "  %s: %s\n", p.Path, p.Message)
			}
		}
		return true, fmt.Errorf("%s is invalid (%d problems)", dir, len(invalid.Problems))
	}
	if err != nil {
		return true, err
	}
	if args[1] == "validate" {
		if asJSON {
			return true, encoder.Encode(map[string]any{"valid": true, "report": validated.Report()})
		}
		_, err := io.WriteString(out, validated.Report().Text()+accessNotChecked)
		return true, err
	}
	data, err := os.ReadFile(fixtures)
	if err != nil {
		return true, err
	}
	parsed, err := workflow.ParseFixtures(data)
	if err != nil {
		return true, err
	}
	simulation, err := validated.Simulate(parsed)
	if err != nil {
		return true, err
	}
	if asJSON {
		return true, encoder.Encode(simulation)
	}
	_, err = io.WriteString(out, simulation.Text()+accessNotChecked)
	return true, err
}

// listWorkflowTemplates answers `evie workflows templates [--json]` (Stage
// 14 sub-stage 10): the reviewed definitions bundled with Evie, which `evie
// workflows propose <workspace-id> --template <name>` proposes to a
// Workspace for the owner's approval. It touches no database.
func listWorkflowTemplates(args []string, out io.Writer, usage error) error {
	asJSON := false
	for _, a := range args {
		if a != "--json" {
			return usage
		}
		asJSON = true
	}
	templates := workflow.Templates()
	if asJSON {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(struct {
			Templates []workflow.TemplateInfo `json:"templates"`
		}{templates})
	}
	if len(templates) == 0 {
		_, err := io.WriteString(out, "No workflow templates are bundled with this Evie.\n")
		return err
	}
	for _, t := range templates {
		fmt.Fprintf(out, "%s\n  %s\n  inputs: %s\n  propose: evie workflows propose <workspace-id> --template %s\n", t.Name, t.Description, strings.Join(t.Inputs, ", "), t.Name)
	}
	_, err := io.WriteString(out, "Proposing records the template's reviewed files and dry run for the Workspace; approving it is a separate step, and approval starts no run.\n")
	return err
}
