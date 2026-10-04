package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/procedural"
	"github.com/davidadel66/evie/internal/workflow"
	"github.com/davidadel66/evie/internal/workflowruntime"
)

// workflowCommands is the Workflow Runtime as the owner's CLI uses it.
type workflowCommands interface {
	Propose(context.Context, workflowruntime.Proposal) (workflowruntime.Card, error)
	Card(context.Context, string) (workflowruntime.Card, error)
	Approve(context.Context, workflowruntime.Approval) (eviedb.WorkflowActivation, error)
	Deactivate(context.Context, workflowruntime.Deactivation) (eviedb.WorkflowActivation, error)
	List(context.Context, memory.WorkspaceID) ([]eviedb.WorkflowDefinitionState, error)
	Show(context.Context, memory.WorkspaceID, string) (eviedb.WorkflowDefinitionState, error)
}

// newWorkflowRuntime is the production Workflow Runtime: the kernel store,
// the procedural repository at ~/.evie/procedural, no capability contracts
// yet (providers declare them in sub-stage 5) and the operator's subagent
// ceiling for agent budgets.
func newWorkflowRuntime(store *eviedb.Store, getenv func(string) string) (*workflowruntime.Runtime, error) {
	policy, err := configuredSubagentPolicy(getenv)
	if err != nil {
		return nil, fmt.Errorf("invalid Subagents configuration: %w", err)
	}
	root, err := procedural.DefaultRoot()
	if err != nil {
		return nil, err
	}
	return workflowruntime.New(workflowruntime.Config{
		Store: store, Repo: procedural.New(root), Catalog: workflow.NewCatalog(),
		Workers: workflow.WorkerCeiling{Deadline: policy.Deadline, Tokens: policy.TokenBudget},
	})
}

// workflowArgs are one activation command's positional arguments and flags.
type workflowArgs struct {
	positional []string
	flags      map[string]string
	json       bool
}

// parseWorkflowArgs reads args after the subcommand: --json, the flags in
// accepted (each with one value, at most once) and positionals.
func parseWorkflowArgs(args []string, accepted ...string) (workflowArgs, bool) {
	out := workflowArgs{flags: map[string]string{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			out.json = true
		case strings.HasPrefix(a, "--"):
			name := strings.TrimPrefix(a, "--")
			_, seen := out.flags[name]
			if !slices.Contains(accepted, name) || seen || i+1 >= len(args) {
				return out, false
			}
			out.flags[name] = args[i+1]
			i++
		case strings.HasPrefix(a, "-"):
			return out, false
		default:
			out.positional = append(out.positional, a)
		}
	}
	return out, true
}

// runWorkflowActivation answers the owner's workflow activation commands
// (Stage 14 sub-stage 2, 2026-10-04): propose a definition directory to a
// Workspace with an optional dry run, which commits its reviewed files to
// procedural Git and prints the approval card; list and show definitions
// with their status, history and why any is blocked; diff (the approval
// card of a proposal); approve exactly the hashes and version a card showed;
// and deactivate a version. Approval is the owner's only: no model tool
// reaches these commands, and approve needs the exact hashes the owner
// reviewed.
func runWorkflowActivation(ctx context.Context, args []string, out io.Writer, rt workflowCommands) (bool, error) {
	if len(args) < 2 || args[0] != "workflows" || !slices.Contains(workflowActivationCommands, args[1]) {
		return false, nil
	}
	usage := errors.New(workflowsUsage)
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	switch args[1] {
	case "propose":
		a, ok := parseWorkflowArgs(args[2:], "fixtures")
		if !ok || len(a.positional) != 2 {
			return true, usage
		}
		req := workflowruntime.Proposal{WorkspaceID: memory.WorkspaceID(a.positional[0]), Dir: a.positional[1], Surface: "cli"}
		if path, ok := a.flags["fixtures"]; ok {
			data, err := os.ReadFile(path)
			if err != nil {
				return true, err
			}
			f, err := workflow.ParseFixtures(data)
			if err != nil {
				return true, err
			}
			req.Fixtures = &f
		}
		card, err := rt.Propose(ctx, req)
		var invalid *workflow.InvalidError
		if errors.As(err, &invalid) {
			if a.json {
				_ = encoder.Encode(map[string]any{"valid": false, "problems": invalid.Problems})
			} else {
				fmt.Fprintf(out, "%s is not a valid Workflow Definition for this Workspace:\n", a.positional[1])
				for _, p := range invalid.Problems {
					fmt.Fprintf(out, "  %s: %s\n", p.Path, p.Message)
				}
			}
			return true, fmt.Errorf("%s is invalid (%d problems); nothing was proposed", a.positional[1], len(invalid.Problems))
		}
		if err != nil {
			return true, err
		}
		if a.json {
			return true, encoder.Encode(card)
		}
		_, err = io.WriteString(out, card.Text())
		return true, err
	case "list":
		a, ok := parseWorkflowArgs(args[2:])
		if !ok || len(a.positional) > 1 {
			return true, usage
		}
		var ws memory.WorkspaceID
		if len(a.positional) == 1 {
			ws = memory.WorkspaceID(a.positional[0])
		}
		defs, err := rt.List(ctx, ws)
		if err != nil {
			return true, err
		}
		if a.json {
			return true, encoder.Encode(struct {
				Workflows []eviedb.WorkflowDefinitionState `json:"workflows"`
			}{defs})
		}
		if len(defs) == 0 {
			_, err := io.WriteString(out, "No workflows have been proposed.\n")
			return true, err
		}
		for _, d := range defs {
			fmt.Fprintln(out, definitionLine(d))
		}
		return true, nil
	case "show":
		a, ok := parseWorkflowArgs(args[2:])
		if !ok || len(a.positional) != 2 {
			return true, usage
		}
		d, err := rt.Show(ctx, memory.WorkspaceID(a.positional[0]), a.positional[1])
		if err != nil {
			return true, err
		}
		if a.json {
			return true, encoder.Encode(d)
		}
		_, err = io.WriteString(out, definitionText(d))
		return true, err
	case "diff":
		a, ok := parseWorkflowArgs(args[2:])
		if !ok || len(a.positional) != 1 {
			return true, usage
		}
		card, err := rt.Card(ctx, a.positional[0])
		if err != nil {
			return true, err
		}
		if a.json {
			return true, encoder.Encode(card)
		}
		_, err = io.WriteString(out, card.Text())
		return true, err
	case "approve":
		a, ok := parseWorkflowArgs(args[2:], "hash", "authority", "expect-version")
		if !ok || len(a.positional) != 1 || len(a.flags) != 3 {
			return true, usage
		}
		expected, err := strconv.ParseInt(a.flags["expect-version"], 10, 64)
		if err != nil || expected < 0 {
			return true, usage
		}
		activation, err := rt.Approve(ctx, workflowruntime.Approval{
			ProposalID: a.positional[0], ExecutableHash: a.flags["hash"], AuthorityHash: a.flags["authority"],
			ExpectedVersion: expected, Surface: "cli", Detail: "evie workflows approve",
		})
		if err != nil {
			return true, err
		}
		if a.json {
			return true, encoder.Encode(activation)
		}
		fmt.Fprintf(out, "Activated %s version %d in Workspace %s (executable %s, authority %s). No run was started.\n",
			activation.Name, activation.Version, activation.WorkspaceID, activation.ExecutableHash, activation.AuthorityHash)
		if s := activation.Schedule; s != nil {
			fmt.Fprintf(out, "Its schedule (%s %s) is approved with it; scheduled runs start once schedules ship.\n", s.Cron, s.Timezone)
		}
		return true, nil
	case "deactivate":
		a, ok := parseWorkflowArgs(args[2:], "version", "reason")
		if !ok || len(a.positional) != 2 {
			return true, usage
		}
		version, err := strconv.ParseInt(a.flags["version"], 10, 64)
		if err != nil || version < 1 {
			return true, usage
		}
		activation, err := rt.Deactivate(ctx, workflowruntime.Deactivation{
			WorkspaceID: memory.WorkspaceID(a.positional[0]), Name: a.positional[1], Version: version, Reason: a.flags["reason"], Surface: "cli",
		})
		if err != nil {
			return true, err
		}
		if a.json {
			return true, encoder.Encode(activation)
		}
		fmt.Fprintf(out, "Deactivated %s version %d. No new run will start; approve a version to activate it again.\n", activation.Name, activation.Version)
		return true, nil
	}
	return true, usage
}

func definitionLine(d eviedb.WorkflowDefinitionState) string {
	line := fmt.Sprintf("%s  %s  %s", d.WorkspaceID, d.Name, d.Status)
	if d.Current != nil {
		line += fmt.Sprintf("  v%d %s", d.Current.Version, shortHash(d.Current.ExecutableHash))
	}
	if d.Pending > 0 {
		line += fmt.Sprintf("  %d pending", d.Pending)
	}
	if len(d.Blocked) > 0 {
		line += "  (" + strings.Join(d.Blocked, "; ") + ")"
	}
	return line
}

func shortHash(hash string) string {
	if len(hash) > len("sha256:")+12 {
		return hash[:len("sha256:")+12]
	}
	return hash
}

func activationText(b *strings.Builder, a eviedb.WorkflowActivation) {
	line := func(format string, args ...any) { fmt.Fprintf(b, format+"\n", args...) }
	line("  Executable hash: %s", a.ExecutableHash)
	line("  Authority hash:  %s", a.AuthorityHash)
	if len(a.Authority) == 0 {
		line("  Standing Authority: none")
	}
	for _, op := range a.Authority {
		line("  Standing Authority: %s on %s: %s, at most %d calls per run", op.Capability, op.Connection, strings.Join(op.Resources, ", "), op.MaxCallsPerRun)
	}
	if len(a.Workers) > 0 {
		line("  Workers: %s", strings.Join(a.Workers, ", "))
	}
	if s := a.Schedule; s != nil {
		line("  Schedule: %s %s (approved; scheduled runs start once schedules ship)", s.Cron, s.Timezone)
	}
	line("  Proposal %s, commit %s", a.ProposalID, a.Commit)
	line("  Pinned Workspace revision %s, access revision %d", a.WorkspaceRevision, a.AccessRevision)
	line("  approved by %s via %s at %s", a.Approver.Actor, a.Approver.Surface, a.ApprovedAt.Format("2006-01-02 15:04:05Z07:00"))
	if d := a.Deactivated; d != nil {
		reason := d.Reason
		if reason == "" {
			reason = "no reason given"
		}
		line("  deactivated via %s at %s: %s", d.Surface, d.At.Format("2006-01-02 15:04:05Z07:00"), reason)
	}
}

func definitionText(d eviedb.WorkflowDefinitionState) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	line("Workflow %s in Workspace %s", d.Name, d.WorkspaceID)
	line("Status: %s", d.Status)
	for _, reason := range d.Blocked {
		line("  blocked: %s", reason)
	}
	for i, v := range d.Versions {
		label := ""
		if i > 0 && v.Deactivated == nil {
			label = " (superseded)"
		}
		line("Version %d%s", v.Version, label)
		activationText(&b, v)
	}
	if len(d.Proposals) > 0 {
		line("Proposals")
	}
	for _, p := range d.Proposals {
		state := "superseded without approval"
		switch {
		case len(p.ApprovedVersions) > 0:
			versions := make([]string, len(p.ApprovedVersions))
			for i, v := range p.ApprovedVersions {
				versions[i] = strconv.FormatInt(v, 10)
			}
			state = "approved as version " + strings.Join(versions, ", ")
		case p.DocumentsVersion > 0:
			state = fmt.Sprintf("documentation of version %d (recorded, no approval needed)", p.DocumentsVersion)
		case p.Pending:
			state = "pending approval (evie workflows diff " + p.ID + ")"
		}
		line("  %s  %s  %s  dry run %s  %s", p.ID, p.ProposedAt.Format("2006-01-02 15:04"), shortHash(p.ExecutableHash), p.SimulationStatus, state)
	}
	return b.String()
}
