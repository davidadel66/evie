import { describe, expect, it } from "vitest";
import type { WorkflowDefinition, WorkflowRun, WorkflowScheduleStatus } from "../api/workflows";
import { lineRange, needsReview, recentRuns, runDots, scheduleText, stepNotes, workflowStatus, yamlLine } from "./workflowView";

const schedule = (cron: string, timezone = "America/Detroit") => ({cron, timezone, catch_up: "skip", inputs: []});
const definition = (status: WorkflowDefinition["status"], pending = 0): WorkflowDefinition => ({workspace_id: "finance", name: "finance-review", status, pending});

describe("when a workflow runs", () => {
  it("reads common schedules in words, with the zone's city", () => {
    expect(scheduleText(undefined)).toBe("On request");
    expect(scheduleText(schedule("0 9 * * *"))).toBe("Daily 09:00 Detroit");
    expect(scheduleText(schedule("30 9 * * 1"))).toBe("Mondays 09:30 Detroit");
    expect(scheduleText(schedule("0 8 * * 1-5", "UTC"))).toBe("Weekdays 08:00 UTC");
    expect(scheduleText(schedule("0 7 * * *", "America/New_York"))).toBe("Daily 07:00 New York");
  });

  it("shows any other schedule as its cron", () => {
    expect(scheduleText(schedule("*/15 * * * *"))).toBe("*/15 * * * * (Detroit)");
    expect(scheduleText(schedule("0 9 1 * *"))).toBe("0 9 1 * * (Detroit)");
    expect(scheduleText(schedule("0 9 * * 1,3"))).toBe("0 9 * * 1,3 (Detroit)");
  });
});

describe("a workflow's status", () => {
  const paused = {paused: true} as WorkflowScheduleStatus;
  it("says whether it runs and whether a version waits for approval", () => {
    expect(workflowStatus(definition("active"))).toEqual({label: "Active", tone: "muted"});
    expect(workflowStatus(definition("active"), paused)).toEqual({label: "Paused", tone: "muted"});
    expect(workflowStatus(definition("active", 1), paused)).toEqual({label: "Update needs approval", tone: "amber"});
    expect(workflowStatus(definition("proposed", 1))).toEqual({label: "Needs approval", tone: "amber"});
    expect(workflowStatus(definition("blocked"))).toEqual({label: "Blocked", tone: "danger"});
    expect(workflowStatus(definition("inactive"))).toEqual({label: "Inactive", tone: "faint"});
  });

  it("offers the approval card for a waiting proposal or a definition that is not running", () => {
    expect(needsReview(definition("active"))).toBe(false);
    expect(needsReview(definition("active", 1))).toBe(true);
    expect(needsReview(definition("proposed", 1))).toBe(true);
    expect(needsReview(definition("blocked"))).toBe(true);
    expect(needsReview(definition("inactive"))).toBe(true);
  });
});

describe("a workflow's last runs", () => {
  it("takes its newest runs and shows them oldest first", () => {
    const run = (id: string, workflow: string, state: WorkflowRun["state"]) => ({id, workflow, state}) as WorkflowRun;
    const runs = [run("5", "finance-review", "waiting_for_human"), run("4", "finance-daily", "succeeded"), run("3", "finance-review", "failed"), run("2", "finance-review", "succeeded")];
    expect(recentRuns(runs, "finance-review", 2).map((r) => r.id)).toEqual(["5", "3"]);
    expect(runDots(recentRuns(runs, "finance-review"))).toEqual(["ok", "danger", "amber"]);
  });
});

describe("a workflow's steps and YAML", () => {
  it("notes where a step leads and how often it tries", () => {
    expect(stepNotes({id: "gather", kind: "capability", next: ["propose"], max_attempts: 2, first_line: 76, last_line: 89})).toEqual(["then propose", "up to 2 attempts"]);
    expect(stepNotes({id: "any", kind: "branch", next: ["nothing", "review"], first_line: 117, last_line: 126})).toEqual(["then nothing or review"]);
    expect(stepNotes({id: "apply", kind: "capability", max_attempts: 1, first_line: 1, last_line: 1})).toEqual([]);
    expect(lineRange({id: "a", kind: "ai", first_line: 54, last_line: 59})).toBe("L54–59");
    expect(lineRange({id: "a", kind: "ai", first_line: 54, last_line: 54})).toBe("L54");
  });

  it("splits a YAML line into its key, the rest and a trailing comment", () => {
    expect(yamlLine("schema_version: 1")).toEqual({indent: "", dash: "", key: "schema_version", rest: ": 1", comment: "", depth: 0});
    expect(yamlLine("  - id: gather")).toEqual({indent: "  ", dash: "- ", key: "id", rest: ": gather", comment: "", depth: 4});
    expect(yamlLine('  cron: "30 9 * * 1"  # Mondays')).toMatchObject({key: "cron", rest: ': "30 9 * * 1" ', comment: " # Mondays"});
    expect(yamlLine("# A comment")).toMatchObject({key: "", comment: "# A comment"});
    expect(yamlLine("      Answer {} to accept.")).toMatchObject({key: "", rest: "      Answer {} to accept."});
  });
});
