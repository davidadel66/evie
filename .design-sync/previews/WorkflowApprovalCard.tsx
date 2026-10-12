import { WorkflowApprovalCard } from "ui";
import * as React from "react";
// The app root (App.tsx) sets these classes; every Evie screen sits on them.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);

const noop = () => {};
const hash = (seed: string) => "sha256:" + seed.padEnd(64, "0");

const reviewAuthority = [
  { capability: "finance.review_candidates", connection: "finance-local", resources: ["ledger"], max_calls_per_run: 2 },
  { capability: "finance.apply_review", connection: "finance-local", resources: ["ledger"], max_calls_per_run: 2 },
];
const reviewSchedule = { cron: "30 9 * * 1", timezone: "America/Detroit", catch_up: "skip", inputs: [] };

// finance-review has never been approved: the card shows everything it binds.
const first = {
  proposal: {
    id: "prop-finance-review-1", workspace_id: "finance", name: "finance-review", commit: "8a709be1c2d3".padEnd(40, "0"),
    executable_hash: hash("3c9e02"), authority_hash: hash("d41e77"),
    simulation: { status: "succeeded", attempts: 5, tokens: 18_420, previews: [{}, {}], notice: "Effects suppressed" },
  },
  expected_version: 0, needs_approval: true,
  diff: {
    to_hash: hash("3c9e02"), executable_changed: true, documentation_changed: false,
    executable: [
      { section: "Steps", added: ["gather: capability finance.review_candidates", "propose: ai (classifier v1)", "gate: deterministic", "review: interrupt", "apply: capability finance.apply_review"] },
      { section: "Limits", added: ["max duration 168h", "max tokens 60000"] },
    ],
    authority: ["+ finance.review_candidates on finance-local: ledger, at most 2 calls per run", "+ finance.apply_review on finance-local: ledger, at most 2 calls per run"],
  },
  requirements: { authority: reviewAuthority, authority_hash: hash("d41e77"), workers: [], schedule: reviewSchedule },
  patch: "",
};

// finance-daily version 3 is active; version 4 raises a limit and a call cap.
const current = {
  workspace_id: "finance", name: "finance-daily", version: 3, proposal_id: "prop-finance-daily-3", commit: "f25cc0208605".padEnd(40, "0"),
  executable_hash: hash("7741a9"), authority_hash: hash("b5f5c1"),
  authority: [
    { capability: "finance.sync_ledger", connection: "finance-local", resources: ["ledger"], max_calls_per_run: 3 },
    { capability: "finance.categorize_ledger", connection: "finance-local", resources: ["ledger"], max_calls_per_run: 3 },
  ],
  workers: [], schedule: { cron: "0 9 * * *", timezone: "America/Detroit", catch_up: "skip", inputs: ["day"] },
  approver: { actor: "owner", surface: "cli" }, approved_at: "2026-10-01T14:12:00Z",
};
const upgrade = {
  proposal: {
    id: "prop-finance-daily-4", workspace_id: "finance", name: "finance-daily", commit: "c2369a1e77b0".padEnd(40, "0"),
    executable_hash: hash("9a1f33"), authority_hash: hash("e0c2a8"),
    simulation: { status: "succeeded", attempts: 3, tokens: 0, previews: [{}], notice: "Effects suppressed" },
  },
  current, current_status: "active", expected_version: 3, needs_approval: true,
  diff: {
    from_hash: hash("7741a9"), to_hash: hash("9a1f33"), executable_changed: true, documentation_changed: true,
    executable: [{ section: "Limits", removed: ["max duration 30m"], added: ["max duration 45m"] }],
    authority: ["~ finance.sync_ledger on finance-local: max calls per run 3 -> 5"],
  },
  requirements: {
    authority: [{ ...current.authority[0], max_calls_per_run: 5 }, current.authority[1]], authority_hash: hash("e0c2a8"), workers: [],
    schedule: { ...current.schedule, max_runs: 1 },
  },
  patch: "",
};
// Version 4 of finance-daily would be refused now: the Workspace's access no longer covers it.
const refused = { ...upgrade, refusals: ["finance.sync_ledger on finance-local with 5 calls per run is outside this Workspace's access (revision 4)", "the dry run's effect previews were not suppressed"] };
const docsOnly = { ...upgrade, needs_approval: false, proposal: { ...upgrade.proposal, documents_version: 3 }, diff: { ...upgrade.diff, executable_changed: false, executable: [], authority: [] } };

export const FirstApproval = () => (
  <AppFrame><WorkflowApprovalCard card={first as any} busy={false} onApprove={noop} onClose={noop} /></AppFrame>
);
export const VersionUpgrade = () => (
  <AppFrame><WorkflowApprovalCard card={upgrade as any} busy={false} onApprove={noop} onClose={noop} /></AppFrame>
);
export const Refused = () => (
  <AppFrame><WorkflowApprovalCard card={refused as any} busy={false} onApprove={noop} onClose={noop} /></AppFrame>
);
export const DocumentationOnly = () => (
  <AppFrame><WorkflowApprovalCard card={docsOnly as any} busy={false} onApprove={noop} onClose={noop} /></AppFrame>
);
export const Busy = () => (
  <AppFrame><WorkflowApprovalCard card={first as any} busy={true} onApprove={noop} onClose={noop} /></AppFrame>
);
