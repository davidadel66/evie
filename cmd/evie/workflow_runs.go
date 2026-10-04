package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflow"
	"github.com/davidadel66/evie/internal/workflowruntime"
)

// The owner's workflow run commands (Stage 14 sub-stage 3, 2026-10-04;
// --detach and leases from sub-stage 4):
//
//	evie workflows run [--detach] <workspace-id> <name> [--input <name>=<value>]... [--input-json <file>]
//	evie workflows runs [<workspace-id>] [--workflow <name>]
//	evie workflows inspect <run-id>
//
// run starts a run of the Workspace's active version and, by default, waits
// for it in the foreground: this process executes it (another Evie process
// may claim it first; then this one watches) until it finishes or pauses.
// With --detach it only records the run and returns: a long-lived Evie
// process (evie serve or the REPL) executes it in the background. Stopping
// the command never cancels a run: once this process's lease expires,
// another process recovers it from its last checkpoint. A run already
// holding the same Run Key is shown instead and nothing new starts. A failed
// or paused run exits with an error after it is shown. Each command takes
// --json.

func workflowEncoder(out io.Writer) *json.Encoder {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder
}

func runWorkflowRun(ctx context.Context, args []string, out io.Writer, rt workflowCommands) error {
	usage := errors.New(workflowsUsage)
	req := workflowruntime.RunRequest{TextInputs: map[string]string{}, Surface: "cli"}
	var positional []string
	asJSON, inputFile := false, ""
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--json":
			asJSON = true
		case "--detach":
			if req.Detach {
				return usage
			}
			req.Detach = true
		case "--input", "--input-json":
			if i+1 >= len(args) {
				return usage
			}
			value := args[i+1]
			i++
			if a == "--input-json" {
				if inputFile != "" {
					return usage
				}
				inputFile = value
				continue
			}
			name, text, ok := strings.Cut(value, "=")
			if _, seen := req.TextInputs[name]; !ok || name == "" || seen {
				return usage
			}
			req.TextInputs[name] = text
		default:
			if strings.HasPrefix(a, "-") {
				return usage
			}
			positional = append(positional, a)
		}
	}
	if len(positional) != 2 {
		return usage
	}
	req.WorkspaceID, req.Workflow = memory.WorkspaceID(positional[0]), positional[1]
	if inputFile != "" {
		data, err := os.ReadFile(inputFile)
		if err != nil {
			return err
		}
		if req.Inputs, err = workflow.ParseInputs(data); err != nil {
			return fmt.Errorf("%s: %w", inputFile, err)
		}
	}
	res, err := rt.StartRun(ctx, req)
	if err != nil {
		return err
	}
	if asJSON {
		if err := workflowEncoder(out).Encode(res); err != nil {
			return err
		}
	} else {
		if res.Existing {
			fmt.Fprintf(out, "A run of %s already holds this Run Key; nothing new was started.\n", res.Run.Run.Workflow)
		}
		if _, err := io.WriteString(out, workflowruntime.RunText(res.Run)); err != nil {
			return err
		}
		if req.Detach && !res.Existing {
			fmt.Fprintf(out, "Detached: a running Evie (evie serve or the REPL) executes this run in the background; follow it with evie workflows inspect %s\n", res.Run.Run.ID)
		}
	}
	switch run := res.Run.Run; {
	case res.Existing:
	case run.State == eviedb.WorkflowRunFailed:
		return fmt.Errorf("workflow run %s failed: %s", run.ID, run.Error)
	case run.State == eviedb.WorkflowRunWaitingForDependency:
		return fmt.Errorf("workflow run %s is paused: %s", run.ID, run.Error)
	}
	return nil
}

// workflowSchedulerConfig is the Workflow Runtime's scheduler configuration
// from the environment: EVIE_WORKFLOW_WORKERS bounds how many runs one
// long-lived process advances at once (default 2, at most 32).
func workflowSchedulerConfig(getenv func(string) string) (workflowruntime.SchedulerConfig, error) {
	var c workflowruntime.SchedulerConfig
	if raw := getenv("EVIE_WORKFLOW_WORKERS"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > workflowruntime.MaxSchedulerWorkers {
			return c, fmt.Errorf("EVIE_WORKFLOW_WORKERS requires a whole number from 1 to %d", workflowruntime.MaxSchedulerWorkers)
		}
		c.Workers = n
	}
	return c, nil
}

// startWorkflowScheduler runs the Workflow Runtime's background scheduler
// (Stage 14 sub-stage 4) in a long-lived REPL or web process, like the
// memory background pass: it executes queued runs, recovers runs whose
// process died once their leases expire, and retries after durable
// backoffs. The returned stop lets nodes in flight finish and releases
// every lease, waiting at most five seconds.
func startWorkflowScheduler(ctx context.Context, rt *workflowruntime.Runtime) func() {
	stop := rt.Start(ctx)
	return func() {
		if err := stop(); err != nil {
			log.Printf("workflow scheduler shutdown: %v", err)
		}
	}
}

func listWorkflowRuns(ctx context.Context, args []string, out io.Writer, rt workflowCommands) error {
	a, ok := parseWorkflowArgs(args, "workflow")
	if !ok || len(a.positional) > 1 {
		return errors.New(workflowsUsage)
	}
	filter := workflowruntime.RunFilter{Workflow: a.flags["workflow"]}
	if len(a.positional) == 1 {
		filter.WorkspaceID = memory.WorkspaceID(a.positional[0])
	}
	runs, err := rt.ListRuns(ctx, filter)
	if err != nil {
		return err
	}
	if a.json {
		return workflowEncoder(out).Encode(struct {
			Runs []eviedb.WorkflowRun `json:"runs"`
		}{runs})
	}
	if len(runs) == 0 {
		_, err := io.WriteString(out, "No workflow runs.\n")
		return err
	}
	for _, run := range runs {
		fmt.Fprintln(out, workflowruntime.RunLine(run))
	}
	return nil
}

func inspectWorkflowRun(ctx context.Context, args []string, out io.Writer, rt workflowCommands) error {
	a, ok := parseWorkflowArgs(args)
	if !ok || len(a.positional) != 1 {
		return errors.New(workflowsUsage)
	}
	d, err := rt.InspectRun(ctx, a.positional[0])
	if err != nil {
		return err
	}
	if a.json {
		return workflowEncoder(out).Encode(d)
	}
	_, err = io.WriteString(out, workflowruntime.RunText(d))
	return err
}
