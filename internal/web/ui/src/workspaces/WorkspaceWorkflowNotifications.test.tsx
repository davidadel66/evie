import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { Workspace } from "../api/contextSessions";
import type { WorkflowCapStatus, WorkflowNotification } from "../api/workflows";
import { WorkspaceHome } from "./Workspaces";
import { WorkflowCapLine, WorkflowNotificationList } from "./WorkspaceWorkflowNotifications";

const workspace: Workspace = {id: "w", displayName: "Notes", state: "active", currentRevisionId: "r", createdAt: "", updatedAt: "", defaultPresetId: "standard", allowedPresetIds: ["standard"]};

const succeeded: WorkflowNotification = {id: 7, workspace_id: "w", workflow: "deep-research", run_id: "run-1", kind: "succeeded", run_state: "succeeded",
  surface: "chat", session_id: "s1", created_at: "2026-10-04T12:00:00Z", text: "deep-research run run-1 succeeded (started from a chat)"};
const capped: WorkflowNotification = {id: 8, workspace_id: "w", workflow: "deep-research", run_id: "run-2", kind: "cap_reached", run_state: "waiting_for_dependency",
  detail: "daily workflow cap reached for 2026-10-04 (UTC)", surface: "schedule", created_at: "2026-10-04T13:00:00Z",
  text: "deep-research run run-2 paused at the daily workflow cap: daily workflow cap reached for 2026-10-04 (UTC)", read_at: "2026-10-04T14:00:00Z"};

const caps: WorkflowCapStatus = {enforced: true, day: "2026-10-04", reset_at: "2026-10-05T00:00:00Z", cap: {tokens: 10_000_000, micro_usd: 5_000_000},
  spent: {day: "2026-10-04", tokens: 2_345_678, micro_usd: 1_234_567, model_calls: 3, worker_calls: 12}, reached: false, paused_runs: ["run-2"]};

describe("Workspace workflow notifications", () => {
  it("is a section of the Workspace home", () => {
    const html = renderToStaticMarkup(<WorkspaceHome workspace={workspace} sessions={[]} busy={false} onNewChat={()=>{}} onResume={()=>{}} />);
    expect(html).toContain('aria-label="Workflow notifications"');
  });

  it("lists each notice with what happened, and marks unread ones", () => {
    const html = renderToStaticMarkup(<WorkflowNotificationList notifications={[succeeded, capped]} busy={false} onRead={()=>{}} />);
    expect(html).toContain("deep-research run run-1 succeeded (started from a chat)");
    expect(html).toContain("paused at the daily workflow cap");
    expect(html).toContain("Unread");
    expect(html).toMatch(/<button[^>]*>Mark read<\/button>/);
    // A read notice has no button of its own.
    expect(html.match(/Mark read/g)?.length).toBe(1);
  });

  it("says when nothing is unread", () => {
    const html = renderToStaticMarkup(<WorkflowNotificationList notifications={[]} busy={false} onRead={()=>{}} />);
    expect(html).toContain("No unread workflow notifications.");
  });

  it("shows today's spend against the daily cap and the runs it paused", () => {
    const html = renderToStaticMarkup(<WorkflowCapLine caps={caps} />);
    expect(html).toContain("2,345,678 of 10,000,000 tokens");
    expect(html).toContain("$1.23 of $5.00");
    expect(html).toContain("1 run paused at the cap");
    expect(html).toContain("00:00 UTC");
    const off = renderToStaticMarkup(<WorkflowCapLine caps={{...caps, enforced: false, paused_runs: []}} />);
    expect(off).toContain("No daily cap is enforced");
  });
});
