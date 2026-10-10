import { describe, expect, it } from "vitest";
import type { WorkflowInterruption, WorkflowRun, WorkflowRunDetail, WorkflowStepOutline } from "../api/workflows";
import { duration } from "../ui/time";
import { enumChoices, groupRuns, latestStates, mapProgress, runNote, runSteps, runSummary, stepProgress, summarizeOutputs } from "./runView";

// finance-review's shape: a branch either ends the run or asks the owner.
const outline: WorkflowStepOutline[] = [
  {id: "gather", kind: "capability", description: "Read the candidates.", next: ["propose"], first_line: 76, last_line: 89},
  {id: "propose", kind: "ai", description: "Propose one category per candidate.", next: ["gate"], first_line: 91, last_line: 103},
  {id: "any", kind: "branch", next: ["nothing", "review"], first_line: 117, last_line: 126},
  {id: "nothing", kind: "deterministic", description: "No proposal survived; end without asking.", first_line: 128, last_line: 134},
  {id: "review", kind: "interrupt", description: "The owner accepts, rejects or changes each line.", next: ["apply"], first_line: 136, last_line: 143},
  {id: "apply", kind: "capability", description: "Insert the accepted entries.", first_line: 145, last_line: 157},
];

const base: WorkflowRun = {id: "run-1", workspace_id: "finance", workflow: "finance-review", version: 1, commit: "c", executable_hash: "sha256:e", authority_hash: "sha256:a",
  workspace_revision: "r", access_revision: 1, run_key: "finance-review:2026-10-05", inputs: {}, state: "running", surface: "schedule", checkpoint: 2,
  created_at: "2026-10-09T09:30:00Z", started_at: "2026-10-09T09:30:00Z"};

const states = (steps: ReturnType<typeof runSteps>) => Object.fromEntries(steps.map((step) => [step.id, step.state]));

describe("a run's steps", () => {
  it("marks done, current and not reached while a run is running", () => {
    const run = {...base, next_step: "propose", step_states: {gather: "succeeded", propose: "running"} as const};
    expect(states(runSteps(run, outline))).toEqual({gather: "done", propose: "running", any: "pending", nothing: "pending", review: "pending", apply: "pending"});
  });

  it("marks the step waiting on the owner", () => {
    const run = {...base, state: "waiting_for_human" as const, next_step: "review", step_states: {gather: "succeeded", propose: "succeeded", any: "succeeded"} as const};
    const steps = runSteps(run, outline);
    expect(states(steps).review).toBe("waiting");
    expect(stepProgress(run, steps)).toBe("step 5 of 6");
  });

  it("marks the branch not taken as skipped once the run succeeded", () => {
    const run = {...base, state: "succeeded" as const, step_states: {gather: "succeeded", propose: "succeeded", any: "succeeded", review: "succeeded", apply: "succeeded"} as const};
    const steps = runSteps(run, outline);
    expect(states(steps)).toEqual({gather: "done", propose: "done", any: "done", nothing: "skipped", review: "done", apply: "done"});
    expect(stepProgress(run, steps)).toBe("6 steps");
  });

  it("leaves the steps after a failure not reached, not skipped", () => {
    const run = {...base, state: "failed" as const, error: "steps.gather failed: rate limited", step_states: {gather: "failed"} as const};
    expect(states(runSteps(run, outline))).toEqual({gather: "failed", propose: "pending", any: "pending", nothing: "pending", review: "pending", apply: "pending"});
  });

  it("marks the step a cancelled run was in as cancelled", () => {
    const run = {...base, state: "cancelled" as const, step_states: {gather: "succeeded", propose: "running"} as const};
    expect(states(runSteps(run, outline)).propose).toBe("cancelled");
  });

  it("reads a run detail's latest attempt per step, map items aside", () => {
    const attempt = (seq: number, step: string, state: "running" | "succeeded" | "failed", item?: number) =>
      ({seq, step, kind: "capability", attempt: 1, input_checkpoint: 0, input: {}, state, started_at: "", item});
    const detail = {run: base, checkpoints: [], attempts: [attempt(2, "gather", "succeeded"), attempt(1, "gather", "failed"), attempt(3, "fan", "succeeded", 0), attempt(4, "fan", "running")]} as WorkflowRunDetail;
    expect(latestStates(detail)).toEqual({gather: "succeeded", fan: "running"});
  });
});

describe("the run list", () => {
  it("puts runs waiting on the owner first, then running, then earlier ones, leaving empty groups out", () => {
    const runs: WorkflowRun[] = [{...base, id: "a", state: "succeeded"}, {...base, id: "b", state: "waiting_for_human"}, {...base, id: "c", state: "running"},
      {...base, id: "d", state: "needs_reconciliation"}, {...base, id: "e", state: "failed"}];
    expect(groupRuns(runs).map((group) => [group.label, group.runs.map((run) => run.id)]))
      .toEqual([["Waiting for you", ["b", "d"]], ["Running", ["c"]], ["Earlier", ["a", "e"]]]);
    expect(groupRuns([{...base, state: "succeeded"}]).map((group) => group.label)).toEqual(["Earlier"]);
  });

  it("says in one line what a run asks, does, failed on or produced", () => {
    const waiting = {...base, state: "waiting_for_human" as const, next_step: "review", step_states: {gather: "succeeded"} as const};
    const question = {run_id: "run-1", step: "review", message: "Review the lines."} as WorkflowInterruption;
    expect(runNote(waiting, runSteps(waiting, outline), question)).toBe("Waiting for you at review: The owner accepts, rejects or changes each line.");
    const running = {...base, next_step: "propose"};
    expect(runNote(running, runSteps(running, outline))).toBe("Running propose: Propose one category per candidate.");
    expect(runNote({...base, state: "failed", error: "steps.gather failed: rate limited"}, [])).toBe("steps.gather failed: rate limited");
    expect(runNote({...base, state: "succeeded", outputs: {applied: 12, rules_minted: 3, day: "2026-10-05"}}, [])).toBe("applied 12 · rules_minted 3 · day 2026-10-05");
    expect(runNote({...base, state: "succeeded"}, [])).toBe("Succeeded");
  });

  it("summarizes outputs as scalars and list sizes, at most four", () => {
    expect(summarizeOutputs({shares: [334, 333, 333], leftover_cents: 1, nested: {a: 1}, ok: true, one: ["x"], five: 5})).toBe("shares 3 items · leftover_cents 1 · ok true · one 1 item");
    expect(summarizeOutputs({note: "x".repeat(41)})).toBe("");
  });

  it("leaves out values masked as sensitive, so a sensitive run reads Succeeded", () => {
    // finance-daily's outputs are masked on the owner's surfaces (retention classification sensitive).
    const masked = {added: "[sensitive]", categorized: "[sensitive]", day: "[sensitive]", failed_banks: "[sensitive]"};
    expect(summarizeOutputs(masked)).toBe("");
    expect(runNote({...base, state: "succeeded", outputs: masked}, [])).toBe("Succeeded");
    expect(summarizeOutputs({...masked, rules_minted: 3})).toBe("rules_minted 3");
  });
});

describe("the run panel", () => {
  it("offers buttons only for a question with one property and a fixed set of answers", () => {
    expect(enumChoices({type: "object", properties: {choice: {type: "string", enum: ["Bump both", "Only vitest", "Neither"]}}, required: ["choice"]}))
      .toEqual({property: "choice", values: ["Bump both", "Only vitest", "Neither"]});
    // finance-review's answer is an object of exceptions: a form, not buttons.
    expect(enumChoices({type: "object", properties: {reject: {type: "array"}, change: {type: "array"}, no_rule: {type: "array"}}})).toBeUndefined();
    expect(enumChoices({type: "object", properties: {n: {enum: [1, 2]}}})).toBeUndefined();
    expect(enumChoices({type: "string", enum: ["a"]})).toBeUndefined();
  });

  it("sums the duration, progress and spend of model and worker calls", () => {
    const detail = {run: {...base, state: "succeeded", finished_at: "2026-10-09T09:32:06Z", step_states: {}}, attempts: [], checkpoints: [],
      model_calls: [{finished_at: "x", input_tokens: 30_000, output_tokens: 8_000, micro_usd: 120_000} as never, {estimated_input_tokens: 2_000, estimated_output_tokens: 0, estimated_micro_usd: 10_000} as never],
      workers: [{finished_at: "x", counted_tokens: 22_000, micro_usd: 210_000} as never]} as WorkflowRunDetail;
    expect(runSummary(detail, runSteps(detail.run, outline.slice(0, 2)), new Date())).toBe("2m 06s · 2 steps · 62k tokens · $0.34");
  });

  it("reads lengths of time at a glance", () => {
    expect([41_000, 126_000, 240_000, 720_000, 11_520_000, 3_600_000].map(duration)).toEqual(["41s", "2m 06s", "4m", "12m", "3h 12m", "1h"]);
  });
});

describe("map steps", () => {
  it("count their items from the latest list: done, failed and running", () => {
    const attempt = (seq: number, state: "running" | "succeeded" | "failed", item?: number, checkpoint = 3) =>
      ({seq, step: "research", kind: "map", attempt: 1, input_checkpoint: checkpoint, input: item === undefined ? {items: ["a", "b", "c", "d", "e", "f"]} : {}, state, started_at: "", item});
    const detail = {run: base, checkpoints: [], attempts: [
      attempt(1, "running"),
      attempt(2, "succeeded", 0), attempt(3, "succeeded", 1), attempt(4, "failed", 2), attempt(5, "succeeded", 2),
      attempt(6, "succeeded", 3), attempt(7, "failed", 4), attempt(8, "running", 5),
    ]} as WorkflowRunDetail;
    expect(mapProgress(detail)).toEqual({research: "4 of 6 items done, 1 failed, 1 running"});
    expect(mapProgress({...detail, attempts: []})).toEqual({});
  });
});
