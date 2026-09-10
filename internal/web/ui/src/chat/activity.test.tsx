import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { appendUser, reduce, type Item } from "../store/reducer";
import { Activity } from "./Activity";
import { activityTurns, toolLabel } from "./activityModel";

describe("activity transcript", () => {
  it("retains elapsed time for a plain completed response with no public summary", () => {
    const turn = {id: "u", startedAt: 1000, finishedAt: 5000, status: "complete"} as const;
    const items: Item[] = [{kind: "user", key: "u", text: "hello", turn}, {kind: "assistant", key: "a", text: "Hi.", streaming: false, phase: "final_answer", turn}];
    const html = renderToStaticMarkup(<Activity group={activityTurns(items, false)[0]} onAnswer={() => {}} />);
    expect(html).toContain("Worked");
    expect(html).toContain("for 4s");
    expect(html).not.toContain("Hi.");
  });
  it("reconciles streamed text into progress and a final answer in one stable turn", () => {
    let items = appendUser([], "inspect plugins");
    items = reduce(items, { type: "turn_started", id: "root", startedAt: 1000, status: "working" });
    items = reduce(items, { type: "delta", text: "partial" });
    expect(activityTurns(items, true)[0].answer).toEqual([]);
    items = reduce(items, { type: "assistant_done", content: "Checking.", parts: [{text: "Checking.", phase: "commentary"}], terminal: false });
    items = reduce(items, {type: "tool_call", id: "c", name: "read_file", args: '{"path":"/src/plugins.go"}'});
    items = reduce(items, {type: "tool_result", id: "c", content: "package plugins", isError: false});
    items = reduce(items, {type: "delta", text: "Checked.Done."});
    items = reduce(items, {type: "assistant_done", content: "Checked.Done.", parts: [{text: "Checked.", phase: "commentary"}, {text: "Done.", phase: "final_answer"}], terminal: true, finishedAt: 5000});
    items = reduce(items, {type: "turn_done"});
    const [turn] = activityTurns(items, false);
    expect(turn.activity.map((item) => item.kind)).toEqual(["assistant", "tool", "assistant"]);
    expect(turn.answer.map((item) => item.text)).toEqual(["Done."]);
    expect(turn.turn).toMatchObject({id: "root", status: "complete", startedAt: 1000, finishedAt: 5000});
    expect(items.every((item) => item.turn?.id === "root")).toBe(true);
    expect(JSON.stringify(items)).not.toContain("partial");
  });

  it("joins a paginated partial turn without mixing the next user", () => {
    const turn = {id: "u", startedAt: 1000, finishedAt: 5000, status: "complete"} as const;
    const suffix: Item[] = [{kind: "assistant", key: "a", text: "done", streaming: false, phase: "final_answer", turn}];
    expect(activityTurns(suffix, false)[0].key).toBe("u");
    const all: Item[] = [{kind: "user", key: "u", text: "first", turn}, ...suffix, {kind: "user", key: "u2", text: "second"}];
    expect(activityTurns(all, true).map((group) => group.key)).toEqual(["u", "u2"]);
    expect(activityTurns(all, true)[0].active).toBe(false);
  });

  it("hides empty reasoning rows and keeps failures and approvals outside a collapsed body", () => {
    const items: Item[] = [
      {kind: "user", key: "u", text: "test"},
      {kind: "reasoning", key: "r", text: "", streaming: false, startedAt: 1000, ms: 4000},
      {kind: "tool", key: "t", id: "c", name: "bash", args: '{"command":"go test ./..."}', result: "exit 1", isErr: true, startedAt: 1000},
      {kind: "tool", key: "p", id: "p", name: "edit_file", args: "{}", startedAt: 1000, approval: {reqId: "approval", state: "pending"}},
    ];
    const html = renderToStaticMarkup(<Activity group={activityTurns(items, false)[0]} onAnswer={() => {}} />);
    expect(html).not.toContain("Thought for");
    expect(html).toContain("Approval required");
    expect(html).toContain("Command failed");
    expect(html).toContain("go test ./...");
    expect(html).toContain("Arguments");
    expect(html).toContain("Result");
    expect(html).toContain('aria-expanded="false"');
    expect(html).toContain("<summary");
  });

  it("does not call requested or declined edits successful", () => {
    const tool: Extract<Item, {kind: "tool"}> = {kind: "tool", key: "t", id: "c", name: "edit_file", args: '{"path":"/tmp/x"}', startedAt: 0};
    expect(toolLabel(tool).label).toBe("Edit file requested");
    expect(toolLabel({...tool, result: "declined", isErr: true, approval: {reqId: "p", state: "declined"}}).label).toBe("Edit file declined");
    expect(toolLabel({...tool, result: "ok"}).label).toBe("Edited file");
    expect(toolLabel({...tool, name: "bash", args: '{"command":"cat file; rm file"}', result: "ok"}).label).toBe("Ran command");
  });

  it("keeps exact arguments and removes expired approval controls as soon as the result arrives", () => {
    let items = appendUser([], "test");
    items = reduce(items, {type: "tool_call", id: "c", name: "edit_file", args: '{"id":9007199254740993,"key":1,"key":2}'});
    items = reduce(items, {type: "approval_request", id: "p", name: "edit_file", args: "{}"});
    items = reduce(items, {type: "tool_result", id: "c", content: "expired", isError: true});
    const html = renderToStaticMarkup(<Activity group={activityTurns(items, true)[0]} onAnswer={() => {}} />);
    expect(html).toContain("9007199254740993");
    expect(html).toContain("&quot;key&quot;:1,&quot;key&quot;:2");
    expect(html).not.toContain("Approval required");
    expect(items[1]).toMatchObject({approval: {state: "expired"}});
  });

  it("stops on transport failure without promoting provisional text to a final answer", () => {
    let items = appendUser([], "test");
    items = reduce(items, {type: "turn_started", id: "u", status: "working", startedAt: 1000});
    items = reduce(items, {type: "delta", text: "partial"});
    items = reduce(items, {type: "error", message: "network"});
    const [turn] = activityTurns(items, false);
    expect(turn.turn?.status).toBe("incomplete");
    expect(turn.answer).toEqual([]);
    expect(turn.activity).toMatchObject([{kind: "assistant", streaming: false}]);
  });
});
