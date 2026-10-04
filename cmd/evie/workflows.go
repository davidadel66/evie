package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/davidadel66/evie/internal/workflow"
)

const workflowsUsage = "usage: evie workflows validate [--json] <dir> | evie workflows simulate [--json] <dir> --fixtures <file>"

// runWorkflowManagement answers `evie workflows validate <dir>`, which prints
// a Workflow Definition's review report, and `evie workflows simulate <dir>
// --fixtures <file>`, which prints an effect-suppressed simulation trace
// (Stage 14 sub-stage 1, 2026-10-04). Both work on any directory and touch
// no database: binding a definition to a Workspace, procedural Git and
// activation are sub-stage 2, and capability providers declare workflow
// contracts in sub-stage 5. Until then the definition is reviewed against an
// empty capability catalog and an empty Workspace Access, so a capability
// node is reported as unauthorized here; agent budgets are bounded by the
// operator's subagent policy.
func runWorkflowManagement(_ context.Context, args []string, out io.Writer, getenv func(string) string) (bool, error) {
	if len(args) == 0 || args[0] != "workflows" {
		return false, nil
	}
	usage := errors.New(workflowsUsage)
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
	env := workflow.Environment{
		Capabilities: workflow.NewCatalog(),
		Workers:      workflow.WorkerCeiling{Deadline: policy.Deadline, Tokens: policy.TokenBudget},
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
		_, err := io.WriteString(out, validated.Report().Text())
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
	_, err = io.WriteString(out, simulation.Text())
	return true, err
}
