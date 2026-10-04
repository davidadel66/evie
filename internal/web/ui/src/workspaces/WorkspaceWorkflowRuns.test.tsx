import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { Workspace } from "../api/contextSessions";
import { parseRunInputs, type WorkflowRun, type WorkflowRunDetail } from "../api/workflows";
import { WorkspaceHome } from "./Workspaces";
import { WorkflowRunList, WorkflowRunStart, WorkflowRunView } from "./WorkspaceWorkflowRuns";

const workspace: Workspace = {id: "w", displayName: "Bills", state: "active", currentRevisionId: "r", createdAt: "", updatedAt: "", defaultPresetId: "standard", allowedPresetIds: ["standard"]};

const run: WorkflowRun = {
  id: "run-1", workspace_id: "w", workflow: "split-bill", version: 2, commit: "c".repeat(40),
  executable_hash: "sha256:" + "a".repeat(64), authority_hash: "sha256:" + "b".repeat(64), workspace_revision: "r", access_revision: 1,
  run_key: "split-bill:sha256:" + "d".repeat(64), inputs: {bill_id: "dinner-1", total_cents: 1000, people: 3}, state: "succeeded",
  outputs: {leftover_cents: 1, shares: [334, 333, 333]}, surface: "web", checkpoint: 3, created_at: "2026-10-04T12:00:00Z", finished_at: "2026-10-04T12:00:01Z",
};

const failed: WorkflowRun = {...run, id: "run-2", state: "failed", outputs: undefined, error: "steps.split failed after 3 attempts: parts must be between 1 and 1000"};

const detail: WorkflowRunDetail = {
  run,
  attempts: [
    {seq: 1, step: "split", kind: "deterministic", attempt: 1, input_checkpoint: 0, input: {total: 1000, parts: 3}, state: "succeeded", output: {remainder: 1}, next: "check", started_at: ""},
    {seq: 2, step: "check", kind: "branch", attempt: 1, input_checkpoint: 1, input: {}, state: "succeeded", output: {next: "leftover", case: 0}, next: "leftover", note: "no case matched; default", started_at: ""},
    {seq: 3, step: "leftover", kind: "deterministic", attempt: 1, input_checkpoint: 2, input: {total: 1, parts: 1}, state: "running", started_at: ""},
  ],
  checkpoints: [],
};

describe("Workspace workflow runs", () => {
  it("is a section of the Workspace home", () => {
    const html = renderToStaticMarkup(<WorkspaceHome workspace={workspace} sessions={[]} busy={false} onNewChat={()=>{}} onResume={()=>{}} />);
    expect(html).toContain('aria-label="Workflow runs"');
  });

  it("lists runs with their state, version and why one failed", () => {
    const html = renderToStaticMarkup(<WorkflowRunList runs={[run, failed]} busy={false} onInspect={()=>{}} />);
    expect(html).toContain("split-bill");
    expect(html).toContain("version 2");
    expect(html).toContain("Succeeded");
    expect(html).toContain("Failed");
    expect(html).toContain("parts must be between 1 and 1000");
    expect(html).toMatch(/<button[^>]*>Inspect<\/button>/);
    expect(renderToStaticMarkup(<WorkflowRunList runs={[]} busy={false} onInspect={()=>{}} />)).toContain("No runs yet");
  });

  it("shows each node attempt, the pins and the outputs", () => {
    const html = renderToStaticMarkup(<WorkflowRunView detail={detail} onClose={()=>{}} />);
    expect(html).toContain(run.run_key);
    expect(html).toContain(run.executable_hash);
    expect(html).toContain("Workspace revision r, access revision 1");
    expect(html).toContain("#1 split attempt 1: succeeded → check");
    expect(html).toContain("#2 check attempt 1: succeeded → leftover");
    expect(html).toContain("no case matched; default");
    expect(html).toContain("#3 leftover attempt 1: running");
    expect(html).toContain("in flight");
    expect(html).toContain("&quot;leftover_cents&quot;: 1");
  });

  it("starts only active workflows, with a JSON inputs object", () => {
    const html = renderToStaticMarkup(<WorkflowRunStart workflows={["split-bill"]} busy={false} onStart={()=>{}} />);
    expect(html).toContain("split-bill");
    expect(html).toMatch(/<button[^>]*>Start run<\/button>/);
    const none = renderToStaticMarkup(<WorkflowRunStart workflows={[]} busy={false} onStart={()=>{}} />);
    expect(none).toContain("No active workflow");
    expect(none).toMatch(/<button[^>]*disabled=""[^>]*>Start run<\/button>/);
    expect(parseRunInputs('{"bill_id": "b"}')).toEqual({bill_id: "b"});
    expect(parseRunInputs("")).toEqual({});
    expect(parseRunInputs("[1]")).toBeUndefined();
    expect(parseRunInputs("{")).toBeUndefined();
  });
});
