package main

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/workflowruntime"
)

// Stage 14 sub-stage 11: the owner sees and pauses schedules, reads and
// marks notifications, and sees the daily cap from the CLI.

var scheduledSplitDir = filepath.Join("..", "..", "internal", "workflow", "testdata", "scheduled-split")

var notificationLine = regexp.MustCompile(`#(\d+) `)

func TestWorkflowCLISchedulesNotificationsAndStatus(t *testing.T) {
	ctx := t.Context()
	c := newWorkflowCLI(t)
	w, err := c.store.RegisterWorkspace(ctx, "Splits")
	if err != nil {
		t.Fatal(err)
	}
	card := c.mustRun("workflows", "propose", string(w.ID), scheduledSplitDir, "--fixtures", filepath.Join("..", "..", "internal", "workflow", "testdata", "scheduled-split.fixtures.json"))
	if !strings.Contains(card, "approving also activates this schedule") {
		t.Fatalf("card:\n%s", card)
	}
	m := approveLine.FindStringSubmatch(card)
	approved := c.mustRun("workflows", "approve", m[1], "--hash", m[2], "--authority", m[3], "--expect-version", m[4])
	if !strings.Contains(approved, "Its schedule (0 7 * * * America/New_York) is active") {
		t.Fatalf("approve:\n%s", approved)
	}
	listed := c.mustRun("workflows", "schedules", string(w.ID))
	if !strings.Contains(listed, "scheduled-split v1  0 7 * * * America/New_York  catch-up each_missed (at most 3)  active") || !strings.Contains(listed, "next: ") {
		t.Fatalf("schedules:\n%s", listed)
	}
	paused := c.mustRun("workflows", "schedule", "pause", string(w.ID), "scheduled-split", "--reason", "away")
	if !strings.Contains(paused, "Paused the schedule of scheduled-split version 1") {
		t.Fatalf("pause:\n%s", paused)
	}
	if listed := c.mustRun("workflows", "schedules", string(w.ID)); !strings.Contains(listed, "paused") || !strings.Contains(listed, "evie workflows schedule resume") {
		t.Fatalf("paused schedules:\n%s", listed)
	}
	if _, err := c.run("workflows", "schedule", "pause", string(w.ID), "scheduled-split"); err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("pausing twice: %v", err)
	}
	raw := c.mustRun("workflows", "schedule", "resume", "--json", string(w.ID), "scheduled-split", "--version", "1")
	var control eviedb.WorkflowScheduleControl
	if err := json.Unmarshal([]byte(raw), &control); err != nil || control.Action != "resume" || control.Surface != "cli" {
		t.Fatalf("resume --json = %s (%v)", raw, err)
	}
	if none := c.mustRun("workflows", "notifications", string(w.ID)); !strings.Contains(none, "No workflow notifications") {
		t.Fatalf("notifications before any run:\n%s", none)
	}
	// A run that finishes notifies; the owner marks it read.
	c.mustRun("workflows", "run", string(w.ID), "scheduled-split", "--input", "day=2026-10-05", "--input", "at=07:00", "--input", "total_cents=1000", "--input", "people=3")
	listed = c.mustRun("workflows", "notifications", string(w.ID))
	id := notificationLine.FindStringSubmatch(listed)
	if id == nil || !strings.Contains(listed, "unread") || !strings.Contains(listed, "scheduled-split run") || !strings.Contains(listed, "succeeded") {
		t.Fatalf("notifications:\n%s", listed)
	}
	status := c.mustRun("workflows", "status", string(w.ID))
	if !strings.Contains(status, "Daily cap: not enforced") || !strings.Contains(status, "Notifications: 1 unread of 1") || !strings.Contains(status, "scheduled-split v1") {
		t.Fatalf("status:\n%s", status)
	}
	if out := c.mustRun("workflows", "notifications", "read", id[1]); !strings.Contains(out, "Marked 1 notification read") {
		t.Fatalf("read:\n%s", out)
	}
	if none := c.mustRun("workflows", "notifications", string(w.ID)); !strings.Contains(none, "No workflow notifications") {
		t.Fatalf("after reading:\n%s", none)
	}
	raw = c.mustRun("workflows", "notifications", "--json", "--all", string(w.ID))
	var shown struct {
		Notifications []workflowruntime.Notification `json:"notifications"`
	}
	if err := json.Unmarshal([]byte(raw), &shown); err != nil || len(shown.Notifications) != 1 || shown.Notifications[0].ReadAt == nil {
		t.Fatalf("notifications --json --all = %s (%v)", raw, err)
	}
	c.mustRun("workflows", "notifications", "unread", id[1])
	if out := c.mustRun("workflows", "notifications", "read", "--all"); !strings.Contains(out, "Marked 1 notification read") {
		t.Fatalf("read --all:\n%s", out)
	}
	for _, args := range [][]string{
		{"workflows", "schedule", "stop", string(w.ID), "scheduled-split"},
		{"workflows", "schedule", "pause", string(w.ID)},
		{"workflows", "schedules", "a", "b"},
		{"workflows", "notifications", "read"},
		{"workflows", "notifications", "read", "x"},
		{"workflows", "notifications", "a", "b"},
		{"workflows", "status", "a", "b"},
	} {
		if _, err := c.run(args...); err == nil {
			t.Fatalf("%q was accepted", args)
		}
	}
}

// The daily cap comes from the environment with conservative defaults;
// a malformed value refuses to start the runtime.
func TestWorkflowDailyCapSetting(t *testing.T) {
	env := func(values map[string]string) func(string) string { return func(k string) string { return values[k] } }
	cap, err := workflowDailyCap(env(nil))
	if err != nil || cap.Tokens != 10_000_000 || cap.MicroUSD != 5_000_000 {
		t.Fatalf("defaults = %+v, %v", cap, err)
	}
	cap, err = workflowDailyCap(env(map[string]string{"EVIE_WORKFLOW_DAILY_TOKENS": "2000000", "EVIE_WORKFLOW_DAILY_USD": "8.50"}))
	if err != nil || cap.Tokens != 2_000_000 || cap.MicroUSD != 8_500_000 {
		t.Fatalf("set = %+v, %v", cap, err)
	}
	if cap, err = workflowDailyCap(env(map[string]string{"EVIE_WORKFLOW_DAILY_USD": "0"})); err != nil || cap.MicroUSD != 0 {
		t.Fatalf("no spending = %+v, %v", cap, err)
	}
	for _, bad := range []map[string]string{
		{"EVIE_WORKFLOW_DAILY_TOKENS": "-1"}, {"EVIE_WORKFLOW_DAILY_TOKENS": "lots"}, {"EVIE_WORKFLOW_DAILY_TOKENS": "2000000000"},
		{"EVIE_WORKFLOW_DAILY_USD": "-1"}, {"EVIE_WORKFLOW_DAILY_USD": "NaN"}, {"EVIE_WORKFLOW_DAILY_USD": "100000"},
	} {
		if _, err := workflowDailyCap(env(bad)); err == nil {
			t.Fatalf("%v was accepted", bad)
		}
	}
}
