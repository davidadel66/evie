import { WorkflowRunView } from "ui";
import * as React from "react";

// The app root (App.tsx) sets these classes; every Evie screen sits on
// them, so previews render on the same dark surface and text color.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);

const run = {
  id: "9cbf8363-9052-4e3f-b43b-b0b2cd888518", workspace_id: "finance", workflow: "finance-daily", version: 1, commit: "f25cc0208605".padEnd(40, "0"),
  executable_hash: "sha256:" + "7741".padEnd(64, "a"), authority_hash: "sha256:" + "b5f5".padEnd(64, "b"), workspace_revision: "r1", access_revision: 1,
  run_key: "finance-daily:sha256:" + "6a3d".padEnd(64, "c"), inputs: { day: "2026-10-07", at: "09:00" }, state: "succeeded",
  outputs: { added: 6, modified: 0, removed: 2, failed_banks: 0, categorized: 6, uncategorized: 126 },
  surface: "schedule", checkpoint: 3, created_at: "2026-10-07T13:00:04Z", finished_at: "2026-10-07T13:00:06Z",
};
const succeeded = {
  run,
  attempts: [
    { seq: 1, step: "sync", kind: "capability", attempt: 1, input_checkpoint: 0, input: { day: "2026-10-07" }, state: "succeeded", output: { added: 6, removed: 2, failed_banks: 0 }, next: "categorize", started_at: "2026-10-07T13:00:04Z" },
    { seq: 2, step: "categorize", kind: "capability", attempt: 1, input_checkpoint: 1, input: { day: "2026-10-07" }, state: "succeeded", output: { categorized: 6, uncategorized: 126 }, next: "check", started_at: "2026-10-07T13:00:06Z" },
    { seq: 3, step: "check", kind: "deterministic", attempt: 1, input_checkpoint: 2, input: { count: 0, subject: "banks failed to sync" }, state: "succeeded", output: { count: 0 }, started_at: "2026-10-07T13:00:06Z" },
  ],
  checkpoints: [],
};
const waiting = {
  run: { ...run, id: "5c3b17e5-9292-42eb-b89c-56b61ad65794", workflow: "finance-review", state: "waiting_for_human", outputs: undefined, finished_at: undefined, surface: "cli", checkpoint: 4 },
  attempts: [
    { seq: 1, step: "gather", kind: "capability", attempt: 1, input_checkpoint: 0, input: { limit: 25 }, state: "succeeded", next: "propose", started_at: "2026-10-07T18:20:01Z" },
    { seq: 2, step: "propose", kind: "ai", attempt: 1, input_checkpoint: 1, input: {}, state: "succeeded", next: "gate", started_at: "2026-10-07T18:20:02Z" },
    { seq: 3, step: "gate", kind: "deterministic", attempt: 1, input_checkpoint: 2, input: {}, state: "succeeded", output: { kept: 18, dropped: 7 }, next: "any", started_at: "2026-10-07T18:20:40Z" },
    { seq: 4, step: "any", kind: "branch", attempt: 1, input_checkpoint: 3, input: {}, state: "succeeded", output: { next: "review", case: -1 }, next: "review", note: "no case matched; default", started_at: "2026-10-07T18:20:40Z" },
  ],
  checkpoints: [],
  interruptions: [{ run_id: "5c3b17e5-9292-42eb-b89c-56b61ad65794", seq: 1, workspace_id: "finance", workflow: "finance-review", kind: "question", checkpoint: 4, opened_at: "2026-10-07T18:20:41Z", response_schema: { type: "object" }, step: "review", message: "Review the proposed classifications in this step's input (lines). Answer {} to accept every line as proposed, with its rule.", request: { input: { lines: [{ id: "DdP5D", merchant_name: "Tavern", net_cents: -6549, category: "Dining", confidence: 0.95, rule: true }] } } }],
};
const failed = {
  run: { ...run, id: "run-failed", state: "failed", outputs: undefined, error: "steps.check failed: 1 banks failed to sync" },
  attempts: [
    { seq: 1, step: "sync", kind: "capability", attempt: 1, input_checkpoint: 0, input: { day: "2026-10-07" }, state: "succeeded", output: { added: 0, failed_banks: 1 }, next: "categorize", started_at: "2026-10-07T13:00:04Z" },
    { seq: 2, step: "categorize", kind: "capability", attempt: 1, input_checkpoint: 1, input: { day: "2026-10-07" }, state: "succeeded", output: { categorized: 0, uncategorized: 126 }, next: "check", started_at: "2026-10-07T13:00:05Z" },
    { seq: 3, step: "check", kind: "deterministic", attempt: 1, input_checkpoint: 2, input: { count: 1, subject: "banks failed to sync" }, state: "failed", error: "1 banks failed to sync", started_at: "2026-10-07T13:00:06Z" },
  ],
  checkpoints: [],
};
const noop = () => {};

export const Succeeded = () => (
  <AppFrame><WorkflowRunView detail={succeeded as any} onClose={noop} /></AppFrame>
);
export const WaitingForYou = () => (
  <AppFrame><WorkflowRunView detail={waiting as any} onClose={noop} onAnswer={noop} onCancel={noop} /></AppFrame>
);
export const Failed = () => (
  <AppFrame><WorkflowRunView detail={failed as any} onClose={noop} onCancel={noop} /></AppFrame>
);
