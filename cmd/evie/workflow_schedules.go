package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflowruntime"
)

// Schedules, notifications, the daily cap and status (Stage 14 sub-stage
// 11, 2026-10-04). Every command here is the owner's: a chat reads its
// runs' notices only through workflow_status and changes nothing.

// The operator's daily cap on all workflow runs (UTC day): conservative
// defaults that let one deep-research run's expected spend (about $1 and 3
// million tokens) through several times over while stopping a runaway
// schedule within a day. EVIE_WORKFLOW_DAILY_TOKENS is a whole number from
// 0 to 1,000,000,000; EVIE_WORKFLOW_DAILY_USD a dollar amount from 0 to
// 10,000 (0 allows no AI calls or workers).
const (
	envWorkflowDailyTokens       = "EVIE_WORKFLOW_DAILY_TOKENS"
	envWorkflowDailyUSD          = "EVIE_WORKFLOW_DAILY_USD"
	defaultWorkflowDailyTokens   = 10_000_000
	defaultWorkflowDailyMicroUSD = 5_000_000
	maxWorkflowDailyTokens       = 1_000_000_000
	maxWorkflowDailyUSD          = 10_000
)

// workflowDailyCap resolves the daily cap every Evie process enforces; a
// malformed value refuses to start the runtime, like EVIE_WORKFLOW_WORKERS.
func workflowDailyCap(getenv func(string) string) (*eviedb.WorkflowDailyCap, error) {
	c := &eviedb.WorkflowDailyCap{Tokens: defaultWorkflowDailyTokens, MicroUSD: defaultWorkflowDailyMicroUSD}
	if raw := strings.TrimSpace(getenv(envWorkflowDailyTokens)); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 || n > maxWorkflowDailyTokens {
			return nil, fmt.Errorf("%s requires a whole number of tokens from 0 to %d", envWorkflowDailyTokens, maxWorkflowDailyTokens)
		}
		c.Tokens = n
	}
	if raw := strings.TrimSpace(getenv(envWorkflowDailyUSD)); raw != "" {
		usd, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(usd) || usd < 0 || usd > maxWorkflowDailyUSD {
			return nil, fmt.Errorf("%s requires a dollar amount from 0 to %d", envWorkflowDailyUSD, maxWorkflowDailyUSD)
		}
		c.MicroUSD = int64(math.Round(usd * 1_000_000))
	}
	return c, nil
}

// listWorkflowSchedules answers `evie workflows schedules [--json]
// [<workspace-id>]`: every current version's schedule with its state, its
// next slot and its latest decided slot.
func listWorkflowSchedules(ctx context.Context, args []string, out io.Writer, rt workflowCommands) error {
	a, ok := parseWorkflowArgs(args)
	if !ok || len(a.positional) > 1 {
		return errors.New(workflowsUsage)
	}
	var ws memory.WorkspaceID
	if len(a.positional) == 1 {
		ws = memory.WorkspaceID(a.positional[0])
	}
	list, err := rt.Schedules(ctx, ws)
	if err != nil {
		return err
	}
	if a.json {
		return workflowEncoder(out).Encode(struct {
			Schedules []workflowruntime.ScheduleStatus `json:"schedules"`
		}{list})
	}
	_, err = io.WriteString(out, workflowruntime.SchedulesText(list))
	return err
}

// controlWorkflowSchedule answers `evie workflows schedule pause|resume
// [--json] <workspace-id> <name> [--version <n>] [--reason <text>]`: the
// owner pauses or resumes the current version's schedule without a new
// approval; the record is immutable.
func controlWorkflowSchedule(ctx context.Context, args []string, out io.Writer, rt workflowCommands) error {
	usage := errors.New(workflowsUsage)
	if len(args) == 0 || (args[0] != "pause" && args[0] != "resume") {
		return usage
	}
	a, ok := parseWorkflowArgs(args[1:], "version", "reason")
	if !ok || len(a.positional) != 2 {
		return usage
	}
	c := workflowruntime.ScheduleControl{WorkspaceID: memory.WorkspaceID(a.positional[0]), Workflow: a.positional[1], Action: args[0], Reason: a.flags["reason"], Surface: "cli"}
	if raw, ok := a.flags["version"]; ok {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 1 {
			return usage
		}
		c.Version = v
	}
	control, err := rt.ControlSchedule(ctx, c)
	if err != nil {
		return err
	}
	if a.json {
		return workflowEncoder(out).Encode(control)
	}
	if control.Action == "pause" {
		_, err = fmt.Fprintf(out, "Paused the schedule of %s version %d: no slot starts until you resume it (evie workflows schedule resume %s %s); slots meanwhile are not caught up.\n",
			control.Workflow, control.Version, control.WorkspaceID, control.Workflow)
		return err
	}
	_, err = fmt.Fprintf(out, "Resumed the schedule of %s version %d: its next slot starts at its time; slots while it was paused are not caught up.\n", control.Workflow, control.Version)
	return err
}

// workflowNotifications answers `evie workflows notifications [--json]
// [--all] [<workspace-id>] [--session <id>] [--run <id>] [--limit <n>]`
// (unread ones by default, newest first) and `evie workflows notifications
// read|unread <id>...` or `read --all [--workspace <id>]`.
func workflowNotifications(ctx context.Context, args []string, out io.Writer, rt workflowCommands) error {
	usage := errors.New(workflowsUsage)
	all := false
	rest := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--all" {
			if all {
				return usage
			}
			all = true
			continue
		}
		rest = append(rest, a)
	}
	if len(rest) > 0 && (rest[0] == "read" || rest[0] == "unread") {
		a, ok := parseWorkflowArgs(rest[1:], "workspace")
		if !ok {
			return usage
		}
		m := workflowruntime.NotificationMark{Read: rest[0] == "read", All: all, WorkspaceID: memory.WorkspaceID(a.flags["workspace"]), Surface: "cli"}
		switch {
		case all && (!m.Read || len(a.positional) > 0):
			return usage
		case !all && (len(a.positional) == 0 || a.flags["workspace"] != ""):
			return usage
		}
		for _, raw := range a.positional {
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || id < 1 {
				return usage
			}
			m.IDs = append(m.IDs, id)
		}
		n, err := rt.MarkNotifications(ctx, m)
		if err != nil {
			return err
		}
		if a.json {
			return workflowEncoder(out).Encode(struct {
				Marked int `json:"marked"`
			}{n})
		}
		noun := "notifications"
		if n == 1 {
			noun = "notification"
		}
		_, err = fmt.Fprintf(out, "Marked %d %s %s.\n", n, noun, rest[0])
		return err
	}
	a, ok := parseWorkflowArgs(rest, "session", "run", "limit")
	if !ok || len(a.positional) > 1 {
		return usage
	}
	f := eviedb.WorkflowNotificationFilter{SessionID: memory.SessionID(a.flags["session"]), RunID: a.flags["run"], All: all}
	if len(a.positional) == 1 {
		f.WorkspaceID = memory.WorkspaceID(a.positional[0])
	}
	if raw, ok := a.flags["limit"]; ok {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 200 {
			return usage
		}
		f.Limit = n
	}
	list, err := rt.Notifications(ctx, f)
	if err != nil {
		return err
	}
	if a.json {
		return workflowEncoder(out).Encode(struct {
			Notifications []workflowruntime.Notification `json:"notifications"`
		}{list})
	}
	_, err = io.WriteString(out, workflowruntime.NotificationsText(list))
	return err
}

// workflowStatus answers `evie workflows status [--json] [<workspace-id>]`:
// the daily cap with today's spend, the notices and the schedules.
func workflowStatus(ctx context.Context, args []string, out io.Writer, rt workflowCommands) error {
	a, ok := parseWorkflowArgs(args)
	if !ok || len(a.positional) > 1 {
		return errors.New(workflowsUsage)
	}
	var ws memory.WorkspaceID
	if len(a.positional) == 1 {
		ws = memory.WorkspaceID(a.positional[0])
	}
	s, err := rt.Status(ctx, ws)
	if err != nil {
		return err
	}
	if a.json {
		return workflowEncoder(out).Encode(s)
	}
	_, err = io.WriteString(out, s.Text())
	return err
}
