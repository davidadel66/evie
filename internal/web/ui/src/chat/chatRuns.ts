import type { WorkflowRunState } from "../api/workflows";
import type { ToolItem } from "./activityModel";

// Runs Evie started in a chat (workspace-activity.spec.md story 4): a
// workflow_start call's result names the run it started, or the run that
// already had those inputs.

export type ChatRunStart = { runId: string; workflow: string; version: number; state: WorkflowRunState };

/** The run a finished workflow_start call names, if it names one. */
export function workflowStartRun(tool: ToolItem): ChatRunStart | undefined {
  if (tool.name !== "workflow_start" || tool.isErr || !tool.result) return undefined;
  try {
    const value = JSON.parse(tool.result) as { run_id?: unknown; workflow?: unknown; version?: unknown; state?: unknown };
    if (typeof value.run_id !== "string" || value.run_id === "" || typeof value.workflow !== "string") return undefined;
    return {runId: value.run_id, workflow: value.workflow, version: typeof value.version === "number" ? value.version : 0, state: (value.state as WorkflowRunState) ?? "queued"};
  } catch {
    return undefined;
  }
}
