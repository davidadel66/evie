import { WorkflowList } from "ui";
import * as React from "react";
// The app root (App.tsx) sets these classes; every Evie screen sits on them.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);

const noop = () => {};
const hash = (seed: string) => "sha256:" + seed.padEnd(64, "0");

const daily = {
  workspace_id: "finance", name: "finance-daily", status: "active", pending: 0,
  current: {
    workspace_id: "finance", name: "finance-daily", version: 3, proposal_id: "prop-finance-daily-3", commit: "f25cc0208605".padEnd(40, "0"),
    executable_hash: hash("7741a9"), authority_hash: hash("b5f5c1"),
    authority: [
      { capability: "finance.sync_ledger", connection: "finance-local", resources: ["ledger"], max_calls_per_run: 3 },
      { capability: "finance.categorize_ledger", connection: "finance-local", resources: ["ledger"], max_calls_per_run: 3 },
    ],
    workers: [], schedule: { cron: "0 9 * * *", timezone: "America/Detroit", catch_up: "skip", inputs: ["day"] },
    approver: { actor: "owner", surface: "cli" }, approved_at: "2026-10-01T14:12:00Z",
  },
};
const review = {
  workspace_id: "finance", name: "finance-review", status: "active", pending: 1,
  current: {
    ...daily.current, name: "finance-review", version: 1, proposal_id: "prop-finance-review-1", commit: "8a709be1c2d3".padEnd(40, "0"),
    executable_hash: hash("3c9e02"), authority_hash: hash("d41e77"),
    authority: [
      { capability: "finance.review_candidates", connection: "finance-local", resources: ["ledger"], max_calls_per_run: 2 },
      { capability: "finance.apply_review", connection: "finance-local", resources: ["ledger"], max_calls_per_run: 2 },
    ],
    schedule: { cron: "30 9 * * 1", timezone: "America/Detroit", catch_up: "skip", inputs: [] },
    approved_at: "2026-10-06T13:40:00Z",
  },
  proposals: [{ id: "prop-finance-review-2", executable_hash: hash("5b2d10"), simulation_status: "succeeded", pending: true, proposed_at: "2026-10-07T20:05:00Z" }],
};
const blockedReview = {
  ...review, status: "blocked", pending: 0, proposals: [],
  blocked: ["Workspace Access revision 4 revoked finance.apply_review on finance-local"],
};
const inactiveDaily = {
  ...daily, status: "inactive", pending: 0,
  current: { ...daily.current, deactivated: { reason: "replacing the check step", surface: "web", at: "2026-10-07T16:02:00Z" } },
};
const proposedOnly = { workspace_id: "finance", name: "finance-monthly", status: "proposed", pending: 1 };

export const Finance = () => (
  <AppFrame><WorkflowList definitions={[daily, review] as any} busy={false} onReview={noop} onDeactivate={noop} /></AppFrame>
);
export const BlockedAndInactive = () => (
  <AppFrame><WorkflowList definitions={[blockedReview, inactiveDaily, proposedOnly] as any} busy={false} onReview={noop} onDeactivate={noop} /></AppFrame>
);
export const Busy = () => (
  <AppFrame><WorkflowList definitions={[daily, review] as any} busy={true} onReview={noop} onDeactivate={noop} /></AppFrame>
);
export const Empty = () => (
  <AppFrame><WorkflowList definitions={[]} busy={false} onReview={noop} onDeactivate={noop} /></AppFrame>
);
