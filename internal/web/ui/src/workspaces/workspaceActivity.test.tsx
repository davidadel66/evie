import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { StoredSession, Workspace } from "../api/contextSessions";
import type { WorkflowCapStatus, WorkflowInterruption } from "../api/workflows";
import { WorkspaceHeader } from "./WorkspaceHeader";
import { WorkspaceHome } from "./Workspaces";
import { compactCount, needsByWorkspace, workflowSpendLine } from "./workspaceActivity";

const workspace: Workspace = {id: "w", displayName: "Finance", state: "active", currentRevisionId: "r", createdAt: "", updatedAt: "", defaultPresetId: "standard", allowedPresetIds: ["standard"]};
const session = {id: "s1", title: "Budget check", workspaceId: "w", status: "active"} as StoredSession;

const caps: WorkflowCapStatus = {enforced: true, day: "2026-10-09", reset_at: "2026-10-10T00:00:00Z", cap: {tokens: 2_000_000, micro_usd: 5_000_000},
  spent: {day: "2026-10-09", tokens: 412_345, micro_usd: 3_181_000, model_calls: 4, worker_calls: 2}, reached: false, paused_runs: []};

function ask(workspace_id: string, run_id: string, seq: number, resolved = false): WorkflowInterruption {
  return {run_id, seq, workspace_id, workflow: "finance-review", kind: "question", step: "review", checkpoint: 3, message: "Review", request: {}, response_schema: {},
    opened_at: "2026-10-09T09:30:00Z", resolution: resolved ? {kind: "answered", actor: "owner", recorded_at: "2026-10-09T10:00:00Z"} : undefined};
}

function home(tab?: "runs" | "workflows" | "settings", waiting = 0) {
  return renderToStaticMarkup(<WorkspaceHome workspace={workspace} sessions={[session]} busy={false} tab={tab} waiting={waiting} caps={caps} onNewChat={()=>{}} onResume={()=>{}} />);
}

describe("what needs the owner now", () => {
  it("counts runs with an open interruption once per run, per Workspace", () => {
    expect(needsByWorkspace([ask("finance", "run-1", 1), ask("finance", "run-1", 2), ask("finance", "run-2", 1), ask("code", "run-3", 1), ask("code", "run-4", 1, true)]))
      .toEqual({finance: 2, code: 1});
    expect(needsByWorkspace([])).toEqual({});
  });
});

describe("today's workflow spend", () => {
  it("reads token counts at a glance", () => {
    expect([950, 1_000, 1_500, 12_345, 412_345, 999_499, 999_500, 1_250_000, 2_000_000].map(compactCount))
      .toEqual(["950", "1k", "1.5k", "12k", "412k", "999k", "1M", "1.3M", "2M"]);
  });

  it("says workflows, so nobody reads it as total spend, and drops the cap when none is enforced", () => {
    expect(workflowSpendLine(caps)).toBe("Workflows today: 412k of 2M tokens · $3.18");
    expect(workflowSpendLine({...caps, enforced: false})).toBe("Workflows today: 412k tokens · $3.18");
  });
});

describe("Workspace header", () => {
  it("has Runs, Workflows and Settings tabs, the selected one marked", () => {
    const html = renderToStaticMarkup(<WorkspaceHeader tab="workflows" waiting={0} onTab={()=>{}} />);
    expect([...html.matchAll(/role="tab"[^>]*>([A-Za-z]+)/g)].map((m) => m[1])).toEqual(["Runs", "Workflows", "Settings"]);
    expect(html).toMatch(/aria-selected="true"[^>]*>Workflows/);
    expect(html.match(/aria-selected="true"/g)).toHaveLength(1);
    // The top bar's breadcrumb names the Workspace; the header does not repeat it.
    expect(html).not.toContain("Finance");
  });

  it("badges the Runs tab with the runs waiting on the owner", () => {
    const html = renderToStaticMarkup(<WorkspaceHeader tab="runs" waiting={2} onTab={()=>{}} />);
    expect(html).toMatch(/Runs<span aria-hidden="true"[^>]*>2<\/span><span class="sr-only">, 2 runs wait on you<\/span>/);
    expect(renderToStaticMarkup(<WorkspaceHeader tab="runs" waiting={0} onTab={()=>{}} />)).not.toContain("wait on you");
  });

  it("shows today's workflow spend once it is known, in red when the cap is reached", () => {
    expect(renderToStaticMarkup(<WorkspaceHeader tab="runs" waiting={0} onTab={()=>{}} />)).not.toContain("Workflows today");
    expect(renderToStaticMarkup(<WorkspaceHeader tab="runs" waiting={0} caps={caps} onTab={()=>{}} />)).toContain("Workflows today: 412k of 2M tokens · $3.18");
    expect(renderToStaticMarkup(<WorkspaceHeader tab="runs" waiting={0} caps={{...caps, reached: true}} onTab={()=>{}} />)).toMatch(/text-danger[^>]*>Workflows today/);
  });
});

describe("Workspace tabs", () => {
  it("opens on Runs, with the run list and nothing from the other tabs", () => {
    const html = home();
    expect(html).toMatch(/aria-selected="true"[^>]*>Runs/);
    expect(html).toContain('aria-label="Workflow runs"');
    expect(html).not.toContain('aria-label="Workflows"');
    expect(html).not.toContain("Conversations");
    expect(html).not.toContain('aria-label="Merge checks"');
  });

  it("shows the workflow definitions on Workflows", () => {
    const html = home("workflows");
    expect(html).toContain('aria-label="Workflows"');
    expect(html).not.toContain('aria-label="Workflow runs"');
  });

  it("keeps every settings section and the conversations on Settings", () => {
    const html = home("settings");
    expect(html).toContain('aria-label="Merge checks"');
    expect(html).toContain("Conversations");
    expect(html).toContain("Budget check");
    expect(html).toMatch(/<button[^>]*>.*New chat<\/button>/);
    expect(html).not.toContain('aria-label="Workflow runs"');
  });

  it("no longer lists workflow notifications on the page; the notifications icon does", () => {
    for (const tab of ["runs", "workflows", "settings"] as const) expect(home(tab)).not.toContain('aria-label="Workflow notifications"');
  });

  it("carries the waiting badge and the spend line into the header", () => {
    const html = home("settings", 3);
    expect(html).toContain("3 runs wait on you");
    expect(html).toContain("Workflows today: 412k of 2M tokens");
  });
});
