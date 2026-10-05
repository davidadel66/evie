package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflow"
	"github.com/davidadel66/evie/internal/workflowruntime"
)

// Built-in and Global workflows (owner decision 2026-10-04): the owner's
// commands over the workflows built into Evie, which every Workspace may
// run without a proposal or approval, and over Global custom definitions.
//
//	evie workflows builtin list [--json] [--workspace <id>]
//	evie workflows builtin disable|enable [--json] <name> [--reason <text>]
//	evie workflows customize [--json] <name> (--workspace <id> | --global) [--fixtures <file>]
//	evie workflows global list [--json] | global show [--json] <name>
//	evie workflows global deactivate [--json] <name> --version <n> [--reason <text>]
//
// Customizing copies the built-in into procedural Git as a proposal with
// its bundled dry run; `evie workflows approve` approves it as any
// proposal, and an approved copy overrides the built-in in its scope
// (Workspace copy, then Global copy, then the built-in).

// workflowBuiltinCommands is the runtime as these commands use it.
type workflowBuiltinCommands interface {
	Builtins(context.Context, memory.WorkspaceID) ([]workflowruntime.BuiltinView, error)
	ControlBuiltin(context.Context, workflowruntime.BuiltinControl) (eviedb.WorkflowBuiltinControl, error)
	Customize(context.Context, workflowruntime.Customization) (workflowruntime.Card, error)
	ListGlobal(context.Context) ([]eviedb.WorkflowDefinitionState, error)
	ShowGlobal(context.Context, string) (eviedb.WorkflowDefinitionState, error)
}

const workflowBuiltinUsage = " | evie workflows builtin list [--json] [--workspace <workspace-id>] | evie workflows builtin disable|enable [--json] <name> [--reason <text>]" +
	" | evie workflows customize [--json] <name> (--workspace <workspace-id> | --global) [--fixtures <file>]" +
	" | evie workflows global list [--json] | evie workflows global show [--json] <name> | evie workflows global deactivate [--json] <name> --version <n> [--reason <text>]"

func workflowBuiltinCommand(ctx context.Context, args []string, out io.Writer, rt workflowCommands, usage error) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if len(args) == 0 {
		return usage
	}
	switch args[0] {
	case "list":
		a, ok := parseWorkflowArgs(args[1:], "workspace")
		if !ok || len(a.positional) != 0 {
			return usage
		}
		views, err := rt.Builtins(ctx, memory.WorkspaceID(a.flags["workspace"]))
		if err != nil {
			return err
		}
		if a.json {
			return encoder.Encode(struct {
				Builtins []workflowruntime.BuiltinView `json:"builtins"`
			}{views})
		}
		if len(views) == 0 {
			_, err := io.WriteString(out, "No workflows are built into this Evie.\n")
			return err
		}
		for _, v := range views {
			_, _ = io.WriteString(out, builtinText(v))
		}
		_, err = io.WriteString(out, "Built-ins run in every Workspace without an approval (they hold no Standing Authority); the Workspace must allow their workers. "+
			"An approved copy overrides one (evie workflows customize <name> --workspace <id> | --global).\n")
		return err
	case "disable", "enable":
		a, ok := parseWorkflowArgs(args[1:], "reason")
		if !ok || len(a.positional) != 1 {
			return usage
		}
		enabled := args[0] == "enable"
		c, err := rt.ControlBuiltin(ctx, workflowruntime.BuiltinControl{Name: a.positional[0], Enabled: enabled, Reason: a.flags["reason"], Surface: "cli"})
		if err != nil {
			return err
		}
		if a.json {
			return encoder.Encode(c)
		}
		if enabled {
			fmt.Fprintf(out, "Enabled the built-in %s for every Workspace: chats and evie workflows run can start it again, and its paused runs continue.\n", c.Name)
		} else {
			fmt.Fprintf(out, "Disabled the built-in %s for every Workspace: no new run starts, and its runs in progress pause at their next step. An approved copy of the same name still runs. Enable it with evie workflows builtin enable %s.\n", c.Name, c.Name)
		}
		return nil
	}
	return usage
}

func builtinText(v workflowruntime.BuiltinView) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	state := "enabled"
	if !v.Enabled {
		state = "disabled"
		if v.Control != nil && v.Control.Reason != "" {
			state += " (" + v.Control.Reason + ")"
		}
	}
	line("%s  %s", v.Name, state)
	line("  %s", v.Description)
	if len(v.Inputs) > 0 {
		line("  inputs: %s", strings.Join(v.Inputs, ", "))
	}
	if v.Problem != "" {
		line("  problem: %s", v.Problem)
	} else {
		version := "not run yet"
		if v.Version > 0 {
			version = fmt.Sprintf("version %d", v.Version)
		}
		line("  this Evie's version: %s (%s), workers %s, at most %d tokens a run", shortHash(v.ExecutableHash), version, strings.Join(v.Workers, ", "), v.MaxTokens)
	}
	for _, r := range v.Versions {
		if !r.Current {
			line("  earlier version %d: %s (commit %s), its runs continue on it", r.Version, shortHash(r.ExecutableHash), shortHash(r.Commit))
		}
	}
	if g := v.Global; g != nil && g.Status == eviedb.WorkflowActive && g.Current != nil {
		line("  overridden everywhere by the Global copy, version %d (evie workflows global show %s)", g.Current.Version, v.Name)
	}
	if in := v.Workspace; in != nil {
		where := map[string]string{"builtin": "the built-in", "global": "the Global copy", "workspace": "the Workspace's own copy"}[in.Source]
		text := fmt.Sprintf("  in Workspace %s: runs %s", in.WorkspaceID, where)
		if in.Version > 0 {
			text += fmt.Sprintf(" version %d", in.Version)
		}
		if len(in.Blocked) > 0 {
			text += "; cannot start: " + strings.Join(in.Blocked, "; ")
		}
		line("%s", text)
	}
	return b.String()
}

// customizeWorkflow answers `evie workflows customize`.
func customizeWorkflow(ctx context.Context, args []string, out io.Writer, rt workflowCommands, usage error) error {
	a, ok := parseWorkflowGlobalArgs(args, "workspace", "fixtures")
	if !ok || len(a.positional) != 1 || a.global == (a.flags["workspace"] != "") {
		return usage
	}
	req := workflowruntime.Customization{Name: a.positional[0], WorkspaceID: memory.WorkspaceID(a.flags["workspace"]), Global: a.global, Surface: "cli"}
	if path, ok := a.flags["fixtures"]; ok {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		f, err := workflow.ParseFixtures(data)
		if err != nil {
			return err
		}
		req.Fixtures = &f
	}
	card, err := rt.Customize(ctx, req)
	if err != nil {
		return err
	}
	if a.json {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(card)
	}
	_, err = io.WriteString(out, card.Text())
	return err
}

// globalWorkflowCommand answers `evie workflows global list|show|deactivate`.
func globalWorkflowCommand(ctx context.Context, args []string, out io.Writer, rt workflowCommands, usage error) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if len(args) == 0 {
		return usage
	}
	switch args[0] {
	case "list":
		a, ok := parseWorkflowArgs(args[1:])
		if !ok || len(a.positional) != 0 {
			return usage
		}
		defs, err := rt.ListGlobal(ctx)
		if err != nil {
			return err
		}
		if a.json {
			return encoder.Encode(struct {
				Workflows []eviedb.WorkflowDefinitionState `json:"workflows"`
			}{defs})
		}
		if len(defs) == 0 {
			_, err := io.WriteString(out, "No Global workflows have been proposed (evie workflows customize <name> --global copies a built-in).\n")
			return err
		}
		for _, d := range defs {
			fmt.Fprintln(out, "global"+strings.TrimPrefix(definitionLine(d), string(d.WorkspaceID)))
		}
		return nil
	case "show":
		a, ok := parseWorkflowArgs(args[1:])
		if !ok || len(a.positional) != 1 {
			return usage
		}
		d, err := rt.ShowGlobal(ctx, a.positional[0])
		if err != nil {
			return err
		}
		if a.json {
			return encoder.Encode(d)
		}
		_, err = io.WriteString(out, definitionText(d))
		return err
	case "deactivate":
		a, ok := parseWorkflowArgs(args[1:], "version", "reason")
		if !ok || len(a.positional) != 1 {
			return usage
		}
		version, err := strconv.ParseInt(a.flags["version"], 10, 64)
		if err != nil || version < 1 {
			return usage
		}
		activation, err := rt.Deactivate(ctx, workflowruntime.Deactivation{Global: true, Name: a.positional[0], Version: version, Reason: a.flags["reason"], Surface: "cli"})
		if err != nil {
			return err
		}
		if a.json {
			return encoder.Encode(activation)
		}
		fmt.Fprintf(out, "Deactivated Global %s version %d. Workspaces without their own approved %s run the built-in again, if Evie has one; its runs in progress pause at their next step.\n",
			activation.Name, activation.Version, activation.Name)
		return nil
	}
	return usage
}

// globalArgs are workflowArgs with a --global switch.
type globalArgs struct {
	workflowArgs
	global bool
}

func parseWorkflowGlobalArgs(args []string, accepted ...string) (globalArgs, bool) {
	var rest []string
	global := false
	for _, a := range args {
		if a == "--global" {
			if global {
				return globalArgs{}, false
			}
			global = true
			continue
		}
		rest = append(rest, a)
	}
	parsed, ok := parseWorkflowArgs(rest, accepted...)
	return globalArgs{workflowArgs: parsed, global: global}, ok
}
