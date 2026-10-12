import { WorkflowNotificationList } from "ui";
import * as React from "react";
// The app root (App.tsx) sets these classes; every Evie screen sits on them.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);

const noop = () => {};
const waiting = { id: 41, workspace_id: "finance", workflow: "finance-review", run_id: "5c3b17e5-9292-42eb-b89c-56b61ad65794", kind: "waiting_for_human", run_state: "waiting_for_human",
  surface: "cli", created_at: "2026-10-07T18:20:41Z", text: "finance-review run 5c3b17e5 is waiting for your answer at steps.review (started from the CLI)" };
const succeeded = { id: 40, workspace_id: "finance", workflow: "finance-daily", run_id: "9cbf8363-9052-4e3f-b43b-b0b2cd888518", kind: "succeeded", run_state: "succeeded",
  surface: "schedule", created_at: "2026-10-07T13:00:06Z", text: "finance-daily run 9cbf8363 succeeded (started by its schedule): 6 added, 2 removed, 6 categorized" };
const failed = { id: 38, workspace_id: "finance", workflow: "finance-daily", run_id: "0e7a2d41-5f1c-4b7e-9a63-2d8c1f0b4e92", kind: "failed", run_state: "failed",
  detail: "steps.check failed: 1 banks failed to sync", surface: "schedule", created_at: "2026-10-06T13:00:07Z", text: "finance-daily run 0e7a2d41 failed (started by its schedule): steps.check failed: 1 banks failed to sync" };
const skipped = { id: 37, workspace_id: "finance", workflow: "finance-review", kind: "schedule_skipped",
  detail: "catch-up is skip; the 2026-10-05 09:30 slot was missed while Evie was stopped", surface: "schedule", created_at: "2026-10-05T19:02:00Z",
  text: "finance-review skipped its 2026-10-05 09:30 America/Detroit slot: Evie was stopped and catch-up is skip" };
const capped = { id: 35, workspace_id: "finance", workflow: "deep-research", run_id: "7f3e9c12-8b4d-4a6e-b2c5-1d0f6e8a9b74", kind: "cap_reached", run_state: "waiting_for_dependency",
  detail: "daily workflow cap reached for 2026-10-04 (UTC)", surface: "chat", created_at: "2026-10-04T22:41:10Z", read_at: "2026-10-05T01:10:00Z",
  text: "deep-research run 7f3e9c12 paused at the daily workflow cap: daily workflow cap reached for 2026-10-04 (UTC)" };
const earlier = { ...succeeded, id: 34, run_id: "2b6d8e1f-4a3c-4d5e-9f70-1a2b3c4d5e6f", created_at: "2026-10-04T13:00:05Z", read_at: "2026-10-04T14:20:00Z",
  text: "finance-daily run 2b6d8e1f succeeded (started by its schedule): 3 added, 0 removed, 3 categorized" };

export const Unread = () => (
  <AppFrame><WorkflowNotificationList notifications={[waiting, succeeded, failed, skipped] as any} busy={false} onRead={noop} /></AppFrame>
);
export const ReadOnesToo = () => (
  <AppFrame><WorkflowNotificationList notifications={[waiting, succeeded, capped, earlier] as any} busy={false} onRead={noop} /></AppFrame>
);
export const Busy = () => (
  <AppFrame><WorkflowNotificationList notifications={[waiting, failed] as any} busy={true} onRead={noop} /></AppFrame>
);
export const Empty = () => (
  <AppFrame><WorkflowNotificationList notifications={[]} busy={false} onRead={noop} /></AppFrame>
);
