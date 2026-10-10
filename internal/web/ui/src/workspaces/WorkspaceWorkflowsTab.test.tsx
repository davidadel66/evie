import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { WorkflowActivation, WorkflowCard, WorkflowDefinition, WorkflowDefinitionView, WorkflowRun, WorkflowScheduleStatus } from "../api/workflows";
import { workflowStatus } from "./workflowView";
import { WorkflowDialog, WorkflowTable, type WorkflowView } from "./WorkspaceWorkflowsTab";

const now = new Date("2026-10-09T12:00:00Z");
const activation = {version: 3, schedule: {cron: "30 9 * * 1", timezone: "America/Detroit", catch_up: "skip", inputs: []}, proposal_id: "p3",
  approved_at: "2026-10-07T20:00:00Z", approver: {actor: "owner", surface: "cli"}} as unknown as WorkflowActivation;
const active: WorkflowDefinition = {workspace_id: "finance", name: "finance-review", status: "active", pending: 0, current: activation, versions: [activation]};
const proposed: WorkflowDefinition = {workspace_id: "finance", name: "triage-issues", status: "proposed", pending: 1};
const run = (id: string, state: WorkflowRun["state"], created_at: string) => ({id, workflow: "finance-review", state, surface: "schedule", created_at}) as WorkflowRun;
const runs = [run("5c3b17e5-aaaa", "succeeded", "2026-10-07T20:28:00Z"), run("4a0b0000-bbbb", "failed", "2026-10-06T13:30:00Z")];
const source = `schema_version: 1
name: finance-review
description: Propose categories and apply the ones the owner accepts.

steps:
  - id: gather
    kind: capability
    next: review

  - id: review
    kind: interrupt
    description: The owner accepts, rejects or changes each line.
`;
const view: WorkflowDefinitionView = {workspace_id: "finance", name: "finance-review", version: 3, commit: "c", source,
  description: "Propose categories and apply the ones the owner accepts.",
  steps: [{id: "gather", kind: "capability", next: ["review"], max_attempts: 2, first_line: 6, last_line: 8},
    {id: "review", kind: "interrupt", description: "The owner accepts, rejects or changes each line.", first_line: 10, last_line: 12}]};
const schedule = {workflow: "finance-review", paused: false, state: "active", next: {due: "2026-10-12T13:30:00Z"}} as WorkflowScheduleStatus;

function dialog(over: Partial<{definition: WorkflowDefinition; card: WorkflowCard; mode: WorkflowView; highlight: number; runs: WorkflowRun[]}> = {}) {
  return renderToStaticMarkup(<WorkflowDialog definition={over.definition ?? active} view={view} runs={over.runs ?? runs} schedule={schedule} card={over.card}
    mode={over.mode ?? "steps"} highlight={over.highlight === undefined ? undefined : view.steps[over.highlight]} busy={false} now={now}
    onMode={()=>{}} onStep={()=>{}} onClose={()=>{}} onReview={()=>{}} onApprove={()=>{}} onCloseCard={()=>{}} onStart={()=>{}} onSchedule={()=>{}}
    onDeactivate={()=>{}} onOpenRun={()=>{}} onAllRuns={()=>{}} />);
}

describe("Workflows table", () => {
  const html = renderToStaticMarkup(<WorkflowTable onOpen={()=>{}} rows={[
    {definition: active, description: view.description, status: workflowStatus(active, schedule), runs},
    {definition: proposed, status: workflowStatus(proposed), runs: []},
  ]} />);

  it("has the design's columns", () => {
    expect([...html.matchAll(/<span(?: class="[^"]*")?>(Workflow|Runs when|Last runs|Status)<\/span>/g)].map((m) => m[1])).toEqual(["Workflow", "Runs when", "Last runs", "Status"]);
  });

  it("shows each workflow's name, version, description, schedule, last runs and status", () => {
    expect(html).toContain(">finance-review<");
    expect(html).toContain(">v3<");
    expect(html).toContain("Propose categories and apply the ones the owner accepts.");
    expect(html).toContain("Mondays 09:30 Detroit");
    expect(html).toContain('aria-label="Last 2 runs"');
    // Oldest first: the failed run, then the succeeded one.
    expect([...html.matchAll(/rounded-\[2px\] (bg-[a-z-]+)/g)].map((m) => m[1])).toEqual(["bg-danger", "bg-ok-gutter"]);
    expect(html).toMatch(/text-muted-text text-right text-xs">Active</);
  });

  it("marks a workflow that needs approval and has never run", () => {
    expect(html).toContain(">triage-issues<");
    expect(html).toContain("On request");
    expect(html).toContain("None yet");
    expect(html).toMatch(/text-amber text-right text-xs">Needs approval</);
  });

  it("says when nothing was proposed", () => {
    expect(renderToStaticMarkup(<WorkflowTable rows={[]} onOpen={()=>{}} />)).toContain("No workflows have been proposed for this Workspace.");
  });
});

describe("Workflow pop-up", () => {
  it("names the workflow, what it does and when it runs", () => {
    const html = dialog();
    expect(html).toMatch(/role="dialog" aria-modal="true" aria-label="Workflow finance-review"/);
    expect(html).toContain("Propose categories and apply the ones the owner accepts.");
    expect(html).toContain("Runs Mondays 09:30 Detroit");
    expect(html).toContain("next Mon");
  });

  it("lists the steps with their kind, line range and notes, and offers the YAML", () => {
    const html = dialog();
    expect([...html.matchAll(/text-ink font-mono text-\[12\.5px\]">(\w+)</g)].map((m) => m[1])).toEqual(["gather", "review"]);
    expect(html).toContain(">L6–8<");
    expect(html).toContain(">then review<");
    expect(html).toContain(">up to 2 attempts<");
    expect(html).toContain("2 steps · click one to see its YAML");
    expect(html).toMatch(/aria-pressed="true"[^>]*>Steps</);
  });

  it("shows the YAML with line numbers and the chosen step's lines highlighted", () => {
    const html = dialog({mode: "yaml", highlight: 1});
    expect(html).toContain("12 lines");
    expect(html).toContain('id="wf-ln-1"');
    const highlighted = [...html.matchAll(/id="wf-ln-(\d+)" class="bg-teal-deep\/40/g)].map((m) => Number(m[1]));
    expect(highlighted).toEqual([10, 11, 12]);
    expect(html).toMatch(/<span class="text-teal">schema_version<\/span>: 1/);
  });

  it("offers Start a run, Pause schedule and Deactivate for an active workflow", () => {
    const html = dialog();
    expect(html).toContain("Start a run");
    expect(html).toContain(">Pause schedule<");
    expect(html).toContain(">Deactivate…<");
    expect(html).not.toContain("Review and approve");
  });

  it("leads with the approval for a workflow that was never approved, and shows the card once asked", () => {
    const html = dialog({definition: proposed, runs: []});
    expect(html).toContain("This workflow hasn&#x27;t been approved yet, so it can&#x27;t run.");
    expect(html).toContain(">Review and approve<");
    expect(html).not.toContain("Start a run");
    const card = {proposal: {id: "p1", workspace_id: "finance", name: "triage-issues", commit: "c", executable_hash: "sha256:e", authority_hash: "sha256:a"},
      expected_version: 0, needs_approval: true, diff: {to_hash: "sha256:e", executable_changed: true, executable: [], authority: ["+ github.issues write"], documentation_changed: false},
      requirements: {authority: [], authority_hash: "sha256:a", workers: []}, patch: ""} as WorkflowCard;
    const withCard = dialog({definition: proposed, runs: [], card});
    expect(withCard).toContain('aria-label="Approve triage-issues"');
    expect(withCard).toContain("Approve version 1");
    expect(withCard).not.toContain(">Review and approve<");
  });

  it("lists recent runs with a way to all of them, and the approved versions", () => {
    const html = dialog();
    expect(html).toContain(">5c3b17e5<");
    expect(html).toContain("Schedule · Oct 7");
    expect(html).toContain("All runs ›");
    expect(html).toMatch(/<summary[^>]*>Versions<\/summary>/);
    expect(html).toContain("from the cli");
    expect(dialog({runs: []})).toContain("No runs yet.");
  });
});
