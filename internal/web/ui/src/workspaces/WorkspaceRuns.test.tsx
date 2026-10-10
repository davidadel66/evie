import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { WorkflowInterruption, WorkflowRun, WorkflowRunDetail, WorkflowStepOutline } from "../api/workflows";
import { runNote, runSteps } from "./runView";
import type { Workspace } from "../api/contextSessions";
import { RunList, RunPanel, RunPanelLoading, WorkspaceRuns, type RunRow } from "./WorkspaceRuns";

const now = new Date("2026-10-09T12:30:00Z");
const outline: WorkflowStepOutline[] = [
  {id: "scan", kind: "capability", description: "List outdated packages.", first_line: 10, last_line: 14},
  {id: "decide", kind: "interrupt", description: "Ask about major upgrades.", first_line: 16, last_line: 20},
  {id: "bump", kind: "agent", description: "Upgrade and run the suite.", first_line: 22, last_line: 30},
];
const base: WorkflowRun = {id: "8f21aaaa-0000", workspace_id: "code", workflow: "deps-weekly", version: 4, commit: "c0ffee", executable_hash: "sha256:9e41c07ab2",
  authority_hash: "sha256:a", workspace_revision: "r14", access_revision: 1, run_key: "deps-weekly:2026-W41", inputs: {}, state: "running", surface: "schedule",
  checkpoint: 2, created_at: "2026-10-09T09:30:00Z", started_at: "2026-10-09T09:30:00Z"};
const waiting: WorkflowRun = {...base, state: "waiting_for_human", next_step: "decide", step_states: {scan: "succeeded"}};
const failed: WorkflowRun = {...base, id: "5a11bbbb", workflow: "release-notes", version: 2, state: "failed", surface: "cli", error: "steps.scan failed: GitHub API rate limited (resets 18:10 UTC).",
  step_states: {scan: "failed"}, created_at: "2026-10-08T12:00:00Z", finished_at: "2026-10-08T12:00:41Z"};
const succeeded: WorkflowRun = {...base, id: "4d20cccc", state: "succeeded", outputs: {upgraded: 14}, step_states: {scan: "succeeded", decide: "succeeded", bump: "succeeded"},
  created_at: "2026-10-05T09:00:00Z", finished_at: "2026-10-05T09:14:00Z"};

const ask = (schema: Record<string, unknown>): WorkflowInterruption => ({run_id: waiting.id, seq: 1, workspace_id: "code", workflow: "deps-weekly", kind: "question", step: "decide",
  checkpoint: 2, message: "Two major upgrades need a call.", request: {}, response_schema: schema, opened_at: "2026-10-09T09:31:00Z"});

function row(run: WorkflowRun): RunRow {
  const steps = runSteps(run, outline);
  return {run, steps, note: runNote(run, steps)};
}

function panel(run: WorkflowRun, extra: Partial<WorkflowRunDetail> = {}, sessionTitle?: string) {
  const detail: WorkflowRunDetail = {run, attempts: [], checkpoints: [], ...extra};
  return renderToStaticMarkup(<RunPanel detail={detail} steps={runSteps(run, outline)} now={now} busy={false} sessionTitle={sessionTitle}
    onClose={()=>{}} onOpenSession={()=>{}} onAnswer={()=>{}} onCancel={()=>{}} onRerun={()=>{}} />);
}

describe("Runs tab list", () => {
  const html = renderToStaticMarkup(<RunList rows={[row(succeeded), row(failed), row(waiting)]} workflows={["deps-weekly", "release-notes"]} filter=""
    selectedId={failed.id} now={now} onFilter={()=>{}} onSelect={()=>{}} />);

  it("groups runs with their counts, waiting ones first", () => {
    expect([...html.matchAll(/<section aria-label="([^"]+)"/g)].map((m) => m[1])).toEqual(["Waiting for you", "Earlier"]);
    expect(html).toMatch(/Waiting for you<span[^>]*>1<\/span>/);
    expect(html).toMatch(/Earlier<span[^>]*>2<\/span>/);
  });

  it("filters by workflow with chips, all workflows first", () => {
    expect([...html.matchAll(/aria-pressed="(true|false)"[^>]*>([^<]+)</g)].map((m) => [m[2], m[1]]))
      .toEqual([["All workflows", "true"], ["deps-weekly", "false"], ["release-notes", "false"]]);
    const filtered = renderToStaticMarkup(<RunList rows={[row(succeeded), row(failed), row(waiting)]} workflows={["deps-weekly", "release-notes"]} filter="release-notes"
      now={now} onFilter={()=>{}} onSelect={()=>{}} />);
    expect(filtered).not.toContain("Waiting for you<");
    expect(filtered.match(/<li>/g)).toHaveLength(1);
  });

  it("shows each run's workflow, version, note, step bar, trigger and start, with no Change column yet", () => {
    expect([...html.matchAll(/text-\[11px\]"><span>(\w+)<\/span><span>(\w+)<\/span><span>(\w+)<\/span><span[^>]*>(\w+)</g)][0].slice(1)).toEqual(["Workflow", "Steps", "Trigger", "Started"]);
    expect(html).toContain("Waiting for you at decide: Ask about major upgrades.");
    expect(html).toContain('aria-label="scan done, decide waiting for you, bump not reached"');
    expect(html).toContain("steps.scan failed: GitHub API rate limited");
    expect(html).toContain("upgraded 14");
    expect(html).toContain(">v4<");
    expect(html).toContain(">CLI<");
    expect(html).toContain(">3h ago<");
    expect(html).toContain(">Oct 5<");
    expect(html).not.toContain(">Change<");
    expect(html.match(/aria-current="true"/g)).toHaveLength(1);
  });

  it("says when there are no runs", () => {
    expect(renderToStaticMarkup(<RunList rows={[]} workflows={[]} filter="" now={now} onFilter={()=>{}} onSelect={()=>{}} />)).toContain("No runs yet.");
  });
});

describe("Run panel", () => {
  it("asks a fixed-choice question with buttons, and says any chat can answer", () => {
    const html = panel(waiting, {interruptions: [ask({type: "object", properties: {choice: {enum: ["Bump both", "Only vitest", "Neither"]}}})]});
    expect(html).toContain("steps.decide needs your answer");
    expect(html).toContain("Two major upgrades need a call.");
    expect([...html.matchAll(/<button[^>]*>(Bump both|Only vitest|Neither)<\/button>/g)].map((m) => m[1])).toEqual(["Bump both", "Only vitest", "Neither"]);
    expect(html).toContain("You can also answer in any chat in this workspace.");
    expect(html).not.toContain("Your answer (a JSON object)");
  });

  it("falls back to the answer form for any other question, and hides answered ones", () => {
    const html = panel(waiting, {interruptions: [ask({type: "object", properties: {reject: {type: "array"}}})]});
    expect(html).toContain("Your answer (a JSON object)");
    const answered = panel(waiting, {interruptions: [{...ask({}), resolution: {kind: "answered", actor: "owner", recorded_at: ""}}]});
    expect(answered).not.toContain("needs your answer");
  });

  it("shows where it came from, its steps in order and its progress", () => {
    const html = panel(waiting);
    expect(html).toContain("deps-weekly");
    expect(html).toContain("Started by its schedule");
    expect(html).toMatch(/step 2 of 3/);
    expect([...html.matchAll(/font-mono text-\[12\.5px\]">(\w+)</g)].map((m) => m[1])).toEqual(["scan", "decide", "bump"]);
    expect(html).toContain(">waiting for you<");
    const chat = panel({...waiting, surface: "chat", session_id: "s1"}, {}, "Flaky archive test");
    expect(chat).toMatch(/Started by Evie in<\/span><button[^>]*>Flaky archive test<\/button>/);
  });

  it("leads with a failed run's error and offers no action", () => {
    const html = panel(failed);
    expect(html).toMatch(/role="alert"[^>]*>steps\.scan failed: GitHub API rate limited/);
    expect(html).not.toContain(">Cancel run<");
    expect(html).not.toContain(">Rerun<");
  });

  it("offers Cancel on an unfinished run and Rerun with a recorded reason on a succeeded one", () => {
    expect(panel(waiting)).toContain(">Cancel run</button>");
    const done = panel(succeeded);
    expect(done).toMatch(/placeholder="Why rerun it\? \(recorded\)"/);
    expect(done).toMatch(/<button type="submit" disabled=""[^>]*>Rerun<\/button>/);
    expect(done).not.toContain(">Cancel run<");
  });

  it("lists a succeeded run's outputs and keeps provenance behind a disclosure", () => {
    const html = panel(succeeded);
    expect(html).toMatch(/<dt class="text-faint">upgraded<\/dt><dd[^>]*>14<\/dd>/);
    expect(html).toMatch(/<details[^>]*><summary[^>]*>Provenance<\/summary>/);
    for (const value of ["deps-weekly:2026-W41", "sha256:9e41c07ab2", "Workspace revision r14"]) expect(html).toContain(value);
  });
});

describe("Run panel while a run loads", () => {
  const frame = (html: string) => html.match(/<aside aria-label="Run [^"]+" class="([^"]+)"/)?.[1];

  it("keeps the same frame, so the list beside it does not change width", () => {
    const loading = renderToStaticMarkup(<RunPanelLoading runId={waiting.id} onClose={()=>{}} />);
    expect(loading).toContain("Loading run…");
    expect(loading).toContain('aria-label="Close run"');
    expect(frame(loading)).toBeDefined();
    expect(frame(loading)).toBe(frame(panel(waiting)));
    expect(renderToStaticMarkup(<RunPanelLoading runId={waiting.id} problem="The run could not be read." onClose={()=>{}} />))
      .toMatch(/role="alert"[^>]*>The run could not be read\./);
  });

  it("is drawn as soon as a run is selected, before its detail arrives", () => {
    const workspace: Workspace = {id: "code", displayName: "evie", state: "active", currentRevisionId: "r", createdAt: "", updatedAt: ""};
    const html = renderToStaticMarkup(<WorkspaceRuns workspace={workspace} selectedId="run-9" onSelect={()=>{}} />);
    expect(html).toMatch(/<aside aria-label="Run run-9"/);
    expect(html).toContain("Loading run…");
    expect(renderToStaticMarkup(<WorkspaceRuns workspace={workspace} onSelect={()=>{}} />)).not.toContain("<aside");
  });
});
