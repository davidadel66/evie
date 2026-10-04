import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { modelCallSpend, type WorkflowInterruption, type WorkflowRun } from "../api/workflows";
import { WorkflowInterruptionAnswer } from "./WorkflowInterruptionAnswer";
import { WorkflowRunView } from "./WorkspaceWorkflowRuns";

const question: WorkflowInterruption = {
  run_id: "run-1", seq: 1, workspace_id: "w", workflow: "receipt-split", kind: "question", step: "confirm", checkpoint: 3,
  message: "Evie read this receipt's total with low confidence. Reply with the total in cents.",
  request: {input: {proposed_total_cents: 1150, receipt_id: "r-2"}},
  response_schema: {type: "object", required: ["total_cents"], properties: {total_cents: {type: "integer"}}, additionalProperties: false},
  session_id: "s-1", opened_at: "2026-10-04T12:00:00Z",
};

const effect: WorkflowInterruption = {
  ...question, seq: 2, kind: "effect_reconciliation", step: "post", intent_seq: 1,
  message: "Did steps.post's effect happen? Effect Intent #1 (ledger.post on conn-ledger account:household) has an unknown outcome.",
  request: {intent: 1, capability: "ledger.post", idempotency_key: "sha256:abc", reason: "the provider did not answer within 30s"},
  response_schema: {type: "object", required: ["outcome"]},
};

const run: WorkflowRun = {
  id: "run-1", workspace_id: "w", workflow: "receipt-split", version: 1, commit: "c".repeat(40),
  executable_hash: "sha256:" + "a".repeat(64), authority_hash: "sha256:" + "b".repeat(64), workspace_revision: "r", access_revision: 0,
  run_key: "receipt-split:sha256:" + "d".repeat(64), inputs: {receipt_id: "r-2"}, state: "waiting_for_human",
  surface: "web", checkpoint: 3, next_step: "confirm", created_at: "2026-10-04T12:00:00Z",
};

describe("Workflow interruptions", () => {
  it("asks a question with its input, its answer schema and an answer form", () => {
    const html = renderToStaticMarkup(<WorkflowInterruptionAnswer interruption={question} busy={false} onAnswer={() => {}} />);
    expect(html).toContain("steps.confirm asks");
    expect(html).toContain("low confidence");
    expect(html).toContain("&quot;proposed_total_cents&quot;: 1150");
    expect(html).toContain("Answer schema");
    expect(html).toContain("Your answer (a JSON object)");
    expect(html).toMatch(/<button[^>]*>Answer<\/button>/);
    expect(html).toMatch(/<textarea[^>]*>{}<\/textarea>/);
  });

  it("presents an unknown effect with its evidence and the reconciliation's shape", () => {
    const html = renderToStaticMarkup(<WorkflowInterruptionAnswer interruption={effect} busy={true} onAnswer={() => {}} />);
    expect(html).toContain("Did steps.post&#x27;s effect happen?");
    expect(html).toContain("Evidence");
    expect(html).toContain("the provider did not answer within 30s");
    expect(html).toContain("Your reconciliation (a JSON object)");
    expect(html).toMatch(/<button[^>]*disabled=""[^>]*>Answer<\/button>/);
    expect(html).toContain("&quot;outcome&quot;: &quot;did-not-happen&quot;");
  });

  it("shows a run waiting for an answer, answered interruptions and the AI spend", () => {
    const waiting = renderToStaticMarkup(<WorkflowRunView detail={{run, attempts: [], checkpoints: [], interruptions: [question]}} onClose={() => {}} onAnswer={() => {}} />);
    expect(waiting).toContain("Waiting for you");
    expect(waiting).toContain("steps.confirm asks");
    const answered = renderToStaticMarkup(<WorkflowRunView onClose={() => {}} detail={{
      run: {...run, state: "succeeded", outputs: {confirmed_total_cents: 1300}},
      attempts: [], checkpoints: [],
      interruptions: [{...question, resolution: {kind: "answered", answer: {total_cents: 1300}, actor: "owner", surface: "web", session_id: "s-2", recorded_at: "2026-10-04T14:00:00Z"}}],
      model_calls: [
        {attempt_seq: 2, attempt_state: "interrupted", step: "read", policy: "reader", policy_version: 1, model: "deepseek/deepseek-v4.1-flash",
          estimated_input_tokens: 200, estimated_output_tokens: 500, estimated_micro_usd: 4200},
        {attempt_seq: 3, attempt_state: "succeeded", step: "read", policy: "reader", policy_version: 1, model: "deepseek/deepseek-v4.1-flash",
          estimated_input_tokens: 200, estimated_output_tokens: 500, estimated_micro_usd: 4200, finished_at: "2026-10-04T12:00:01Z",
          requests: 1, input_tokens: 120, output_tokens: 15, micro_usd: 180},
      ],
    }} />);
    expect(answered).not.toContain("Waiting for you");
    expect(answered).toContain("Interruption #1 at confirm: answered via web from session s-2: {&quot;total_cents&quot;:1300}");
    expect(answered).toContain("read attempt #2: reader v1 deepseek/deepseek-v4.1-flash, 700 tokens (estimated: it may have been billed)");
    expect(answered).toContain("AI spend: 835 tokens, $0.004380");
    expect(modelCallSpend({attempt_seq: 1, attempt_state: "running", step: "s", policy: "p", policy_version: 1, model: "m",
      estimated_input_tokens: 1, estimated_output_tokens: 2, estimated_micro_usd: 3})).toEqual({tokens: 3, microUSD: 3});
  });
});
