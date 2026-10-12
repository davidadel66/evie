import { WorkflowRunList } from "ui";
import * as React from "react";
// The app root (App.tsx) sets these classes; every Evie screen sits on them.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);

const noop = () => {};
const base = {
  id: "9cbf8363-9052-4e3f-b43b-b0b2cd888518", workspace_id: "finance", workflow: "finance-daily", version: 3, commit: "f25cc0208605".padEnd(40, "0"),
  executable_hash: "sha256:" + "7741".padEnd(64, "a"), authority_hash: "sha256:" + "b5f5".padEnd(64, "b"), workspace_revision: "r1", access_revision: 1,
  run_key: "finance-daily:sha256:" + "6a3d".padEnd(64, "c"), inputs: { day: "2026-10-07", at: "09:00" }, state: "succeeded",
  outputs: { added: 6, modified: 0, removed: 2, failed_banks: 0, categorized: 6, uncategorized: 126 },
  surface: "schedule", checkpoint: 3, created_at: "2026-10-07T13:00:04Z", finished_at: "2026-10-07T13:00:06Z",
};
const runs = [
  { ...base, id: "5c3b17e5-9292-42eb-b89c-56b61ad65794", workflow: "finance-review", version: 1, state: "waiting_for_human", outputs: undefined, finished_at: undefined, surface: "cli", checkpoint: 4, next_step: "review", created_at: "2026-10-07T18:20:01Z" },
  base,
  { ...base, id: "0e7a2d41-5f1c-4b7e-9a63-2d8c1f0b4e92", state: "failed", outputs: undefined, error: "steps.check failed: 1 banks failed to sync", created_at: "2026-10-06T13:00:03Z", finished_at: "2026-10-06T13:00:07Z" },
  { ...base, id: "b1d4f0a7-3c2e-4d9b-8f16-7a5e9c2d0b31", workflow: "deep-research", version: 1, state: "cancelled", outputs: undefined, surface: "chat", session_id: "chat-2fd8", error: "cancelled by the owner via web: asked the wrong question", created_at: "2026-10-05T21:14:30Z" },
  { ...base, id: "7f3e9c12-8b4d-4a6e-b2c5-1d0f6e8a9b74", state: "waiting_for_dependency", outputs: undefined, finished_at: undefined, error: "daily workflow cap reached for 2026-10-04 (UTC)", not_before: "2026-10-05T00:00:00Z", created_at: "2026-10-04T13:00:02Z" },
  { ...base, id: "c8a1b2d3-e4f5-4a6b-8c7d-9e0f1a2b3c4d", state: "running", outputs: undefined, finished_at: undefined, surface: "web", checkpoint: 1, next_step: "categorize", created_at: "2026-10-08T13:00:05Z" },
];

export const Recent = () => (
  <AppFrame><WorkflowRunList runs={runs as any} busy={false} onInspect={noop} /></AppFrame>
);
export const Busy = () => (
  <AppFrame><WorkflowRunList runs={runs.slice(0, 3) as any} busy={true} onInspect={noop} /></AppFrame>
);
export const Empty = () => (
  <AppFrame><WorkflowRunList runs={[]} busy={false} onInspect={noop} /></AppFrame>
);
