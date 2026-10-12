import { WorkflowInterruptionAnswer } from "ui";
import * as React from "react";
// The app root (App.tsx) sets these classes; every Evie screen sits on them.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);

const noop = () => {};
const runId = "5c3b17e5-9292-42eb-b89c-56b61ad65794";

// finance-review's review step: the proposed classifications wait for David.
const review = {
  run_id: runId, seq: 1, workspace_id: "finance", workflow: "finance-review", kind: "question", step: "review", checkpoint: 4,
  message: "Review the proposed classifications in this step's input (lines). Answer {} to accept every line as proposed, with its rule; {\"reject\": [ids]} to leave those lines uncategorized.",
  request: { input: { lines: [
    { id: "DdP5D", merchant_name: "Tavern", net_cents: -6549, category: "Dining", confidence: 0.95, rule: true },
    { id: "Q2m7K", merchant_name: "DTE Energy", net_cents: -14210, category: "Utilities", confidence: 0.99, rule: true },
    { id: "x9Lp1", merchant_name: "Meijer", net_cents: -8732, category: "Groceries", confidence: 0.81, rule: false },
  ] } },
  response_schema: { type: "object", properties: { reject: { type: "array", items: { type: "string" } } }, additionalProperties: false },
  opened_at: "2026-10-07T18:20:41Z",
};

// finance.apply_review did not answer within its deadline: did it happen?
const effect = {
  ...review, seq: 2, kind: "effect_reconciliation", step: "apply", checkpoint: 5, intent_seq: 1,
  message: "Effect Intent #1 (finance.apply_review on finance-local ledger) has an unknown outcome.",
  request: { intent: 1, capability: "finance.apply_review", connection: "finance-local", idempotency_key: "sha256:2f9c41e0b7", reason: "the provider did not answer within 30s" },
  response_schema: { type: "object", required: ["outcome"], properties: { outcome: { enum: ["happened", "did-not-happen"] }, evidence: { type: "string" }, output: { type: "object" } } },
  opened_at: "2026-10-07T18:31:12Z",
};

export const ReviewQuestion = () => (
  <AppFrame><WorkflowInterruptionAnswer interruption={review as any} busy={false} onAnswer={noop} /></AppFrame>
);
export const EffectReconciliation = () => (
  <AppFrame><WorkflowInterruptionAnswer interruption={effect as any} busy={false} onAnswer={noop} /></AppFrame>
);
export const Busy = () => (
  <AppFrame><WorkflowInterruptionAnswer interruption={review as any} busy={true} onAnswer={noop} /></AppFrame>
);
