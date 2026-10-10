import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { WorkflowRun, WorkflowStepOutline } from "../api/workflows";
import { Panel } from "../artifacts/Panel";
import type { Item } from "../store/reducer";
import { runSteps } from "../workspaces/runView";
import { Chat } from "./Chat";
import { workflowStartRun } from "./chatRuns";
import type { ToolItem } from "./activityModel";
import { RunCard } from "./RunCard";

const start = (result: string | undefined, over: Partial<ToolItem> = {}): ToolItem =>
  ({kind: "tool", key: "t-1", id: "call-1", name: "workflow_start", args: "{}", result, startedAt: 0, ...over}) as ToolItem;
const started = JSON.stringify({run_id: "5c3b17e5-aaaa", workflow: "finance-review", version: 1, state: "running", existing: false, max_tokens: 60000,
  note: "Run 5c3b17e5-aaaa started in the background."});

const outline: WorkflowStepOutline[] = [
  {id: "gather", kind: "capability", description: "Read the candidates.", first_line: 1, last_line: 2},
  {id: "review", kind: "interrupt", description: "The owner answers each line.", first_line: 3, last_line: 4},
  {id: "apply", kind: "capability", first_line: 5, last_line: 6},
];
const run: WorkflowRun = {id: "5c3b17e5-aaaa", workspace_id: "finance", workflow: "finance-review", version: 1, commit: "c", executable_hash: "e", authority_hash: "a",
  workspace_revision: "r", access_revision: 1, run_key: "k", inputs: {}, state: "running", surface: "chat", session_id: "s1", checkpoint: 1, next_step: "gather",
  created_at: "2026-10-09T09:30:00Z", started_at: "2026-10-09T09:30:00Z", step_states: {gather: "running"}};

describe("runs a chat started", () => {
  it("are read from a finished workflow_start call's result", () => {
    expect(workflowStartRun(start(started))).toEqual({runId: "5c3b17e5-aaaa", workflow: "finance-review", version: 1, state: "running"});
    expect(workflowStartRun(start(started, {isErr: true}))).toBeUndefined();
    expect(workflowStartRun(start(undefined))).toBeUndefined();
    expect(workflowStartRun(start(started, {name: "workflow_status"}))).toBeUndefined();
    expect(workflowStartRun(start("invalid workflow_start arguments: name is blank"))).toBeUndefined();
    expect(workflowStartRun(start(JSON.stringify({workflow: "finance-review"})))).toBeUndefined();
  });
});

describe("run card", () => {
  const card = (r?: WorkflowRun) => renderToStaticMarkup(<RunCard start={workflowStartRun(start(started))!} run={r} steps={r ? runSteps(r, outline) : []} onOpen={() => {}} />);

  it("shows what workflow_start reported until the runs list is read", () => {
    const html = card();
    expect(html).toContain('aria-label="Run of finance-review, Running"');
    expect(html).toContain(">finance-review<");
    expect(html).toContain(">Run<");
  });

  it("shows the current step and progress while running", () => {
    const html = card(run);
    expect(html).toContain("Running gather: Read the candidates.");
    expect(html).toContain("step 1 of 3");
  });

  it("turns amber when the run waits on the owner", () => {
    const html = card({...run, state: "waiting_for_human", next_step: "review", step_states: {gather: "succeeded"}});
    expect(html).toContain('aria-label="Run of finance-review, Waiting for you"');
    expect(html).toContain("Waiting for you at review: The owner answers each line.");
    expect(html).toMatch(/text-amber flex-none text-xs">Waiting for you</);
    expect(html).toContain("step 2 of 3");
  });

  it("shows how long a finished run took", () => {
    const html = card({...run, state: "succeeded", finished_at: "2026-10-09T09:32:06Z", outputs: {applied: 12}, step_states: {gather: "succeeded", review: "succeeded", apply: "succeeded"}});
    expect(html).toContain(">Succeeded<");
    expect(html).toContain(">2m 06s<");
    expect(html).toContain("applied 12");
  });
});

describe("chat", () => {
  const items: Item[] = [
    {kind: "user", key: "u-1", text: "Run the finance review"},
    start(started, {key: "t-1"}),
    {kind: "assistant", key: "a-1", text: "I started finance-review; it will ask you about each line.", streaming: false},
  ];

  it("puts a turn's run card after Evie's reply, once per run", () => {
    // Starting the same inputs twice returns the same run: one card.
    const html = renderToStaticMarkup(<Chat items={[items[0], items[1], start(started, {key: "t-2"}), items[2]]} queued={[]} streaming={false} onAnswer={() => undefined} workspaceId="finance" onOpenRun={() => {}} />);
    const reply = html.indexOf("I started finance-review");
    const cardAt = html.indexOf('aria-label="Run of finance-review');
    expect(reply).toBeGreaterThan(0);
    expect(cardAt).toBeGreaterThan(reply);
    expect(html.match(/aria-label="Run of finance-review/g)).toHaveLength(1);
  });

  it("shows no card for a turn that started no run", () => {
    const html = renderToStaticMarkup(<Chat items={[items[0], items[2]]} queued={[]} streaming={false} onAnswer={() => undefined} workspaceId="finance" />);
    expect(html).not.toContain("Run of");
  });
});

describe("chat Inspector", () => {
  it("opens a run as a workflow run", () => {
    const html = renderToStaticMarkup(<Panel focused={false} onClose={() => undefined} target={{kind: "workflow-run", runId: "5c3b17e5-aaaa"}} />);
    expect(html).toContain("Workflow run");
    expect(html).toContain("Loading run…");
  });
});
