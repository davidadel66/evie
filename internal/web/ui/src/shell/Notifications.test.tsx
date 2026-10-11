import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { Notice } from "../api/notifications";
import { NotificationList, NotificationsMenu } from "./Notifications";
import { noticeContext, noticeDestination, noticeTime, unreadBadge } from "./notices";

const now = new Date("2026-10-09T12:00:00Z");
const names = {finance: "Finance", code: "evie-core"};

const waiting: Notice = {id: "workflow:9", source: "workflow", label: "finance-review", text: "Waiting for your answer", created_at: "2026-10-09T11:48:00Z",
  target: {kind: "workflow_run", workspace_id: "finance", run_id: "run-9"}};
const failed: Notice = {id: "workflow:8", source: "workflow", label: "deps-weekly", text: "Failed: steps.collect failed: rate limited", created_at: "2026-10-09T09:00:00Z",
  read_at: "2026-10-09T10:00:00Z", target: {kind: "workflow_run", workspace_id: "code", run_id: "run-8"}};

function menu(open: boolean, unread: number, notices = [waiting, failed]) {
  return renderToStaticMarkup(<NotificationsMenu notices={notices} unread={unread} workspaceNames={names} now={now} open={open}
    onToggle={()=>{}} onOpen={()=>{}} onMarkAllRead={()=>{}} />);
}

describe("notices", () => {
  it("name their Workspace beside what they are about", () => {
    expect(noticeContext(waiting, names)).toBe("Finance · finance-review");
    // A Workspace the snapshot no longer has, or a notice from no Workspace, shows its label alone.
    expect(noticeContext({...waiting, target: {kind: "workflow_run", workspace_id: "gone", run_id: "r"}}, names)).toBe("finance-review");
    expect(noticeContext({...waiting, target: {kind: "elsewhere"}}, names)).toBe("finance-review");
  });

  it("say when they arrived, relative for the last day", () => {
    expect(noticeTime("2026-10-09T11:59:40Z", now)).toBe("Just now");
    expect(noticeTime("2026-10-09T11:48:00Z", now)).toBe("12m ago");
    expect(noticeTime("2026-10-09T09:00:00Z", now)).toBe("3h ago");
    expect(noticeTime("2026-10-05T09:00:00Z", now)).toBe("Oct 5");
  });

  it("lead to what they are about: a run in the Runs tab, a Change on its review page", () => {
    expect(noticeDestination(waiting)).toEqual({tab: "runs", run: "run-9"});
    expect(noticeDestination({...waiting, target: {kind: "workspace_runs", workspace_id: "finance"}})).toEqual({tab: "runs"});
    // changes.spec.md stories 5c and 6a.
    const merged: Notice = {id: "change:10", source: "change", label: "Changes", text: "Pull request #7 merged once its checks passed.",
      created_at: "2026-10-09T11:50:00Z", target: {kind: "change", workspace_id: "code", change_id: "pr:7"}};
    expect(noticeDestination(merged)).toEqual({tab: "changes", change: "pr:7"});
    expect(noticeContext(merged, names)).toBe("evie-core · Changes");
  });

  it("cap the unread badge", () => {
    expect(unreadBadge(3)).toBe("3");
    expect(unreadBadge(120)).toBe("99+");
  });
});

describe("notifications list", () => {
  it("shows each notice with its Workspace, its text and when, unread ones marked", () => {
    const html = renderToStaticMarkup(<NotificationList notices={[waiting, failed]} workspaceNames={names} now={now} onOpen={()=>{}} />);
    expect(html).toContain("Finance · finance-review");
    expect(html).toContain("Waiting for your answer");
    expect(html).toContain("12m ago");
    expect(html).toContain("evie-core · deps-weekly");
    expect(html.match(/Unread: /g)).toHaveLength(1);
    // Every notice is one button that opens what it points to.
    expect(html.match(/<button/g)).toHaveLength(2);
  });

  it("says what will show up when there is nothing yet", () => {
    expect(renderToStaticMarkup(<NotificationList notices={[]} workspaceNames={names} now={now} onOpen={()=>{}} />))
      .toContain("Workflow runs and Changes that finish or need you show up here.");
  });
});

describe("notifications icon", () => {
  it("shows the unread count on the icon and keeps the list closed", () => {
    const html = menu(false, 1);
    expect(html).toMatch(/aria-label="Notifications, 1 unread"/);
    expect(html).toMatch(/aria-expanded="false"/);
    expect(html).not.toContain('role="dialog"');
    expect(menu(false, 120)).toContain(">99+<");
    expect(menu(false, 0)).toMatch(/aria-label="Notifications"/);
  });

  it("opens the list with Mark all read while anything is unread", () => {
    const html = menu(true, 1);
    expect(html).toMatch(/role="dialog" aria-label="Notifications"/);
    expect(html).toContain("Mark all read");
    expect(html).toContain("Finance · finance-review");
    expect(menu(true, 0, [failed])).not.toContain("Mark all read");
  });
});
