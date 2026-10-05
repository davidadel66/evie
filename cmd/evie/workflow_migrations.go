package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/davidadel66/evie/internal/workflow"
	"github.com/davidadel66/evie/internal/workflowruntime"
)

// Migration and retention (Stage 14 sub-stage 12, 2026-10-04). Every
// command here is the owner's; no model tool migrates a run or sweeps.

// The operator's retention policy for each classification
// (workflowruntime.ParseRetentionPolicy), over the defaults: mask on,
// redact never, delete notices never, compact after seven days.
const (
	envWorkflowRetentionOrdinary  = "EVIE_WORKFLOW_RETENTION_ORDINARY"
	envWorkflowRetentionSensitive = "EVIE_WORKFLOW_RETENTION_SENSITIVE"
)

// workflowRetention resolves the retention policies every Evie process
// applies; a malformed policy refuses to start the runtime.
func workflowRetention(getenv func(string) string) (workflowruntime.RetentionConfig, error) {
	c := workflowruntime.DefaultRetention()
	var err error
	if c.Ordinary, err = workflowruntime.ParseRetentionPolicy(getenv(envWorkflowRetentionOrdinary), c.Ordinary); err != nil {
		return c, fmt.Errorf("%s: %w", envWorkflowRetentionOrdinary, err)
	}
	if c.Sensitive, err = workflowruntime.ParseRetentionPolicy(getenv(envWorkflowRetentionSensitive), c.Sensitive); err != nil {
		return c, fmt.Errorf("%s: %w", envWorkflowRetentionSensitive, err)
	}
	return c, nil
}

// migrateWorkflowRun answers `evie workflows migrate [--json] [--detach]
// <run-id> --to-version <n> [--reason <text>]`: the owner moves an
// unfinished run to the definition's newer active version as a Successor
// Run, which by default the command waits for like a start.
func migrateWorkflowRun(ctx context.Context, args []string, out io.Writer, rt workflowCommands) error {
	usage := errors.New(workflowsUsage)
	detach := false
	rest := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--detach" {
			if detach {
				return usage
			}
			detach = true
			continue
		}
		rest = append(rest, a)
	}
	a, ok := parseWorkflowArgs(rest, "to-version", "reason")
	if !ok || len(a.positional) != 1 {
		return usage
	}
	version, err := strconv.ParseInt(a.flags["to-version"], 10, 64)
	if err != nil || version < 1 {
		return usage
	}
	res, err := rt.Migrate(ctx, workflowruntime.MigrationRequest{RunID: a.positional[0], ToVersion: version, Reason: a.flags["reason"], Detach: detach, Surface: "cli"})
	var refused *workflowruntime.MigrationRefusedError
	if errors.As(err, &refused) && !a.json {
		fmt.Fprintf(out, "Run %s was not migrated: %s\n", refused.RunID, refused.Reason)
		for _, p := range refused.Problems {
			fmt.Fprintf(out, "  %s: %s\n", p.Path, p.Message)
		}
		return err
	}
	if err != nil {
		return err
	}
	if a.json {
		return workflowEncoder(out).Encode(res)
	}
	m := res.Migration
	fmt.Fprintf(out, "Migrated run %s (version %d) to run %s (version %d), resuming at steps.%s; run %s is superseded and its history is unchanged.\n",
		m.PredecessorID, m.FromVersion, m.SuccessorID, m.ToVersion, m.Resume, m.PredecessorID)
	if _, err := io.WriteString(out, workflowruntime.RunText(res.Successor)); err != nil {
		return err
	}
	if detach {
		fmt.Fprintf(out, "Detached: a running Evie (evie serve or the REPL) executes run %s; follow it with evie workflows inspect %s\n", m.SuccessorID, m.SuccessorID)
	}
	return nil
}

// workflowRetentionCommand answers `evie workflows retention status
// [--json]` (the policies, the sweep's latest summary and totals, and what
// is due now) and `evie workflows retention sweep [--dry-run] [--json]`
// (the sweep now, under the runner lease; a dry run records nothing).
func workflowRetentionCommand(ctx context.Context, args []string, out io.Writer, rt workflowCommands) error {
	usage := errors.New(workflowsUsage)
	if len(args) == 0 || args[0] != "status" && args[0] != "sweep" {
		return usage
	}
	asJSON, dryRun := false, false
	for _, a := range args[1:] {
		switch {
		case a == "--json" && !asJSON:
			asJSON = true
		case a == "--dry-run" && args[0] == "sweep" && !dryRun:
			dryRun = true
		default:
			return usage
		}
	}
	if args[0] == "sweep" {
		res, err := rt.Sweep(ctx, workflowruntime.SweepRequest{DryRun: dryRun, Surface: "cli"})
		if err != nil {
			return err
		}
		if asJSON {
			return workflowEncoder(out).Encode(res)
		}
		_, err = io.WriteString(out, workflowruntime.SweepText(res))
		return err
	}
	status, err := rt.RetentionStatus(ctx)
	if err != nil {
		return err
	}
	if asJSON {
		return workflowEncoder(out).Encode(status)
	}
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	line("Retention policies (%s, %s)", envWorkflowRetentionOrdinary, envWorkflowRetentionSensitive)
	for _, class := range []string{workflow.ClassificationOrdinary, workflow.ClassificationSensitive} {
		line("  %s: %s", class, status.Policies[class])
	}
	if s := status.State; s.LastSweepAt != nil {
		line("Last sweep at %s: %s", s.LastSweepAt.Format("2006-01-02 15:04:05Z07:00"), s.LastSummary)
	} else {
		line("No sweep has run yet.")
	}
	if s := status.State; s.Runner != "" && s.RunnerExpiresAt != nil {
		line("Sweeping now: %s (until %s)", s.Runner, s.RunnerExpiresAt.Format("2006-01-02 15:04:05Z07:00"))
	}
	totals := slices.Clone(status.State.Totals)
	if len(totals) > 0 {
		line("Recorded so far")
	}
	for _, t := range totals {
		line("  %s %s: %d records, %d bytes in %d actions", t.Action, t.DataClass, t.Records, t.Bytes, t.Actions)
	}
	line("Due now (a dry run; evie workflows retention sweep applies it)")
	b.WriteString(workflowruntime.SweepText(status.Due))
	_, err = io.WriteString(out, b.String())
	return err
}
