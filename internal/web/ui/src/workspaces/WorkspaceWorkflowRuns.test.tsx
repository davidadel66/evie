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

  it("shows a paused run's reason, its lease and its recovery events", () => {
    const paused: WorkflowRun = {...run, id: "run-3", state: "waiting_for_dependency", outputs: undefined, finished_at: undefined,
      error: "workflow is not active: version 2 was deactivated", not_before: "2026-10-04T12:05:00Z"};
    const listed = renderToStaticMarkup(<WorkflowRunList runs={[paused]} busy={false} onInspect={()=>{}} />);
    expect(listed).toContain("Paused: workflow is not active: version 2 was deactivated");
    expect(listed).not.toContain("text-danger");
    const html = renderToStaticMarkup(<WorkflowRunView onClose={()=>{}} detail={{
      run: paused,
      attempts: [
        {seq: 1, step: "split", kind: "deterministic", attempt: 1, input_checkpoint: 0, input: {}, state: "interrupted", error: "interrupted: holder evie-1-a (lease 1) stopped before the attempt's outcome was recorded", started_at: "", lease_holder: "evie-1-a", lease_token: 1},
      ],
      checkpoints: [],
      lease: {run_id: "run-3", holder: "evie-2-b", token: 2, acquired_at: "2026-10-04T12:00:30Z", heartbeat_at: "2026-10-04T12:00:40Z", expires_at: "2026-10-04T12:01:00Z", released_at: "2026-10-04T12:00:41Z"},
      events: [
        {run_id: "run-3", seq: 1, kind: "lease_taken_over", holder: "evie-2-b", token: 2, detail: "holder evie-1-a's lease 1 expired", at: "2026-10-04T12:00:30Z"},
        {run_id: "run-3", seq: 2, kind: "paused", holder: "evie-2-b", token: 2, detail: "workflow is not active: version 2 was deactivated", at: "2026-10-04T12:00:41Z"},
      ],
    }} />);
    expect(html).toContain("Paused: workflow is not active: version 2 was deactivated");
    expect(html).not.toContain('role="alert"');
    expect(html).toContain("#1 split attempt 1: interrupted");
    expect(html).toContain("Lease released by evie-2-b (lease 2)");
    expect(html).toContain("Recovery");
    expect(html).toContain("lease_taken_over by evie-2-b (lease 2): holder evie-1-a&#x27;s lease 1 expired");
    const held = renderToStaticMarkup(<WorkflowRunView onClose={()=>{}} detail={{...detail, run: {...run, state: "running", outputs: undefined},
      lease: {run_id: "run-1", holder: "evie-3-c", token: 4, acquired_at: "", heartbeat_at: "2026-10-04T12:00:40Z", expires_at: "2026-10-04T12:01:00Z"}}} />);
    expect(held).toContain("Lease held by evie-3-c (lease 4) until");
    expect(held).not.toContain("Recovery");
  });

  it("offers a cancellation for an unfinished run and shows it once recorded", () => {
    const running: WorkflowRun = {...run, id: "run-4", state: "running", outputs: undefined, finished_at: undefined};
    const html = renderToStaticMarkup(<WorkflowRunView detail={{...detail, run: running}} onClose={()=>{}} onCancel={()=>{}} />);
    expect(html).toMatch(/<button[^>]*>Cancel run<\/button>/);
    expect(html).toContain('aria-label="Cancellation reason"');
    for (const state of ["succeeded", "failed", "cancelled", "cancelling"] as const) {
      const shown = renderToStaticMarkup(<WorkflowRunView detail={{...detail, run: {...running, state}}} onClose={()=>{}} onCancel={()=>{}} />);
      expect(shown).not.toContain(">Cancel run<");
    }
    expect(renderToStaticMarkup(<WorkflowRunView detail={{...detail, run: running}} onClose={()=>{}} />)).not.toContain(">Cancel run<");
    const cancellation = {run_id: "run-4", reason: "wrong bill", actor: "owner", surface: "web", from_state: "running" as const, requested_at: "2026-10-04T12:00:00Z"};
    const cancelling = renderToStaticMarkup(<WorkflowRunView onClose={()=>{}} onCancel={()=>{}} detail={{...detail, cancellation,
      run: {...running, state: "cancelling", error: "cancelling once its effect is reconciled: reconcile it with: evie workflows reconcile run-4 post"}}} />);
    expect(cancelling).toContain("Cancelling");
    expect(cancelling).toContain("Cancellation requested via web");
    expect(cancelling).toContain("wrong bill");
    expect(cancelling).toContain("cancelling once its effect is reconciled");
    expect(cancelling).not.toContain('role="alert"');
    expect(cancelling).not.toContain("Next:");
    const cancelled = renderToStaticMarkup(<WorkflowRunView onClose={()=>{}} detail={{...detail, cancellation,
      run: {...running, state: "cancelled", error: "cancelled by the owner via web: wrong bill", finished_at: "2026-10-04T12:00:02Z"}}} />);
    expect(cancelled).toContain("Cancelled via web");
    expect(cancelled).not.toContain('role="alert"');
    const listed = renderToStaticMarkup(<WorkflowRunList runs={[{...running, state: "cancelled", error: "cancelled by the owner via web: wrong bill"}]} busy={false} onInspect={()=>{}} />);
    expect(listed).toContain("Cancelled");
    expect(listed).not.toContain("text-danger");
  });

  it("shows the override a rerun was started under", () => {
    const html = renderToStaticMarkup(<WorkflowRunView onClose={()=>{}} detail={{...detail, override: {run_id: "run-1", rerun_of: "run-0",
      logical_key: "split-bill:sha256:" + "d".repeat(64), reason: "the bill was corrected", actor: "owner", surface: "cli", recorded_at: "2026-10-04T12:00:00Z"}}} />);
    expect(html).toContain("Rerun of run-0");
    expect(html).toContain("the bill was corrected");
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
