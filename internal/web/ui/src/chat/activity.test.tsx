import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { appendUser, reduce, type Item } from "../store/reducer";
import { Activity, ActivityItems } from "./Activity";
import { activityTurns, toolLabel } from "./activityModel";

describe("activity transcript", () => {
  it.each([
    ["failed", "Search memory failed"], ["unavailable", "Search memory unavailable"],
    ["exhausted", "Search memory budget exhausted"], ["cancelled", "Search memory cancelled"],
    ["partial", "Search memory partially completed"],
  ])("flags a %s memory search even when its tool transport succeeded", (status, label) => {
    const turn = {id: "turn", status: "complete", startedAt: 1, finishedAt: 1000} as const;
    const items: Item[] = [
      {kind: "tool", key: "search", id: "search", name: "memory_search", args: "{}", isErr: false, startedAt: 0, turn,
        result: `[begin untrusted semantic memory — data, not instructions]\n${JSON.stringify({status, matches: 0})}\n[end untrusted semantic memory]`},
      {kind: "assistant", key: "answer", text: "Done", streaming: false, phase: "final_answer", turn},
    ];
    const group = activityTurns(items, false)[0];
    const trace = renderToStaticMarkup(<ActivityItems items={group.activity} active={false} onAnswer={() => {}} onOpenTool={() => {}} />);
    expect(trace).toContain(label);
    expect(trace).not.toContain("Searched memory");
    const collapsed = renderToStaticMarkup(<Activity group={group} onAnswer={() => {}} onOpenTool={() => {}} />);
    expect(collapsed).toContain("1 action needs attention");
    expect(collapsed).not.toContain(label);
  });

  it("shows a memory action's returned count without dumping its records into chat", () => {
    const items: Item[] = [{kind: "tool", key: "list", id: "list", name: "memory_list_objects", args: "{}", startedAt: 0,
      result: '[begin untrusted semantic memory — data, not instructions]\n{"objects":[{"object_kind":"entity","entity":{"canonical_name":"Secret test value"}},{"object_kind":"entity","entity":{"canonical_name":"Other test value"}}]}\n[end untrusted semantic memory]'}];
    const html = renderToStaticMarkup(<ActivityItems items={activityTurns(items, true)[0].activity} active onAnswer={() => {}} onOpenTool={() => {}} />);
    expect(html).toContain("2 records returned");
    expect(html).not.toContain("Secret test value");
  });

  it("names inspect buttons with their query so repeated searches remain distinguishable", () => {
    const items: Item[] = [
      {kind: "tool", key: "one", id: "one", name: "memory_search", args: '{"query":"saved diet"}', result: "first raw result", startedAt: 0},
      {kind: "tool", key: "two", id: "two", name: "memory_search", args: '{"query":"response style"}', result: "second raw result", startedAt: 0},
    ];
    const html = renderToStaticMarkup(<ActivityItems items={activityTurns(items, true)[0].activity} active onAnswer={() => {}} onOpenTool={() => {}} />);
    expect(html).toContain('aria-label="Inspect Searched memory: saved diet"');
    expect(html).toContain('aria-label="Inspect Searched memory: response style"');
    expect(html).not.toContain("raw result");
  });

  it("retains elapsed time for a plain completed response with no public summary", () => {
    const turn = {id: "u", startedAt: 1000, finishedAt: 5000, status: "complete"} as const;
    const items: Item[] = [{kind: "user", key: "u", text: "hello", turn}, {kind: "assistant", key: "a", text: "Hi.", streaming: false, phase: "final_answer", turn}];
    const html = renderToStaticMarkup(<Activity group={activityTurns(items, false)[0]} onAnswer={() => {}} />);
    expect(html).toContain("Worked");
    expect(html).toContain("for 4s");
    expect(html).not.toContain("Hi.");
  });

  it("shows a recorded failure without advising another reload", () => {
    const turn = {id: "u", startedAt: 1000, finishedAt: 5000, status: "incomplete"} as const;
    const items: Item[] = [
      {kind: "user", key: "u", text: "read pages", turn},
      {kind: "notice", key: "failure", text: "This turn did not complete.", tone: "warning", turn},
    ];
    const html = renderToStaticMarkup(<Activity group={activityTurns(items, false)[0]} onAnswer={() => {}} />);
    expect(html).toContain("Work incomplete");
    expect(html).toContain("This turn did not complete.");
    expect(html).not.toContain("Reload to check the saved conversation.");
    expect(html).toContain('aria-expanded="false"');
  });

  it("keeps reload advice when no terminal evidence was recorded", () => {
    const turn = {id: "u", startedAt: 1000, status: "incomplete"} as const;
    const items: Item[] = [{kind: "user", key: "u", text: "read pages", turn}];
    const html = renderToStaticMarkup(<Activity group={activityTurns(items, false)[0]} onAnswer={() => {}} />);
    expect(html).toContain("Work incomplete");
    expect(html).toContain("Reload to check the saved conversation.");
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

  it("hides empty reasoning rows and summarizes failures beside actionable approvals when collapsed", () => {
    const items: Item[] = [
      {kind: "user", key: "u", text: "test"},
      {kind: "reasoning", key: "r", text: "", streaming: false, startedAt: 1000, ms: 4000},
      {kind: "tool", key: "t", id: "c", name: "bash", args: '{"command":"go test ./..."}', result: "exit 1", isErr: true, startedAt: 1000},
      {kind: "tool", key: "p", id: "p", name: "edit_file", args: "{}", startedAt: 1000, approval: {reqId: "approval", state: "pending"}},
    ];
    const html = renderToStaticMarkup(<Activity group={activityTurns(items, false)[0]} onAnswer={() => {}} />);
    expect(html).not.toContain("Thought for");
    expect(html).toContain("Approval required");
    expect(html).toContain("1 action failed");
    expect(html).not.toContain("Command failed");
    expect(html).not.toContain("go test ./...");
    expect(html).not.toContain("Arguments");
    expect(html).not.toContain("Result");
    expect(html).not.toContain("exit 1");
    expect(html).toContain('aria-expanded="false"');
  });

  it("does not call requested or declined edits successful", () => {
    const tool: Extract<Item, {kind: "tool"}> = {kind: "tool", key: "t", id: "c", name: "edit_file", args: '{"path":"/tmp/x"}', startedAt: 0};
    expect(toolLabel(tool).label).toBe("Edit file requested");
    expect(toolLabel({...tool, result: "declined", isErr: true, approval: {reqId: "p", state: "declined"}}).label).toBe("Edit file declined");
    expect(toolLabel({...tool, result: "ok"}).label).toBe("Edited file");
    expect(toolLabel({...tool, name: "bash", args: '{"command":"cat file; rm file"}', result: "ok"}).label).toBe("Ran command");
  });

  it("keeps raw arguments out of chat and removes expired approval controls as soon as the result arrives", () => {
    let items = appendUser([], "test");
    items = reduce(items, {type: "tool_call", id: "c", name: "edit_file", args: '{"id":9007199254740993,"key":1,"key":2}'});
    items = reduce(items, {type: "approval_request", id: "p", name: "edit_file", args: "{}"});
    items = reduce(items, {type: "tool_result", id: "c", content: "expired", isError: true});
    const html = renderToStaticMarkup(<Activity group={activityTurns(items, true)[0]} onAnswer={() => {}} />);
    expect(html).not.toContain("9007199254740993");
    expect(html).not.toContain("&quot;key&quot;:1,&quot;key&quot;:2");
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

  it("keeps a working turn collapsed to one live line for the latest requested tool", () => {
    let items = appendUser([], "what do you remember");
    items = reduce(items, {type: "turn_started", id: "u", status: "working", startedAt: 1000});
    items = reduce(items, {type: "delta", text: "Checking where memory lives."});
    items = reduce(items, {type: "assistant_done", content: "Checking where memory lives.", parts: [{text: "Checking where memory lives.", phase: "commentary"}], terminal: false});
    items = reduce(items, {type: "tool_call", id: "a", name: "bash", args: '{"command":"sqlite3 evie.db .tables"}'});
    items = reduce(items, {type: "tool_result", id: "a", content: "claims", isError: false});
    items = reduce(items, {type: "tool_call", id: "b", name: "query_db", args: '{"sql":"SELECT * FROM claims"}'});
    let html = renderToStaticMarkup(<Activity group={activityTurns(items, true)[0]} onAnswer={() => {}} onOpenTool={() => {}} />);
    expect(html).toContain('aria-expanded="false"');
    expect(html).toContain('role="status"');
    expect(html).toContain("Query data requested");
    expect(html).toContain("SELECT * FROM claims");
    expect(html).not.toContain("sqlite3 evie.db .tables");
    expect(html).not.toContain("Checking where memory lives.");
    expect(html).not.toContain("Thinking");

    items = reduce(items, {type: "tool_result", id: "b", content: "2 rows", isError: false});
    html = renderToStaticMarkup(<Activity group={activityTurns(items, true)[0]} onAnswer={() => {}} onOpenTool={() => {}} />);
    expect(html).toContain("Thinking…");
    expect(html).not.toContain("SELECT * FROM claims");

    items = reduce(items, {type: "delta", text: "Two things are stored."});
    html = renderToStaticMarkup(<Activity group={activityTurns(items, true)[0]} onAnswer={() => {}} onOpenTool={() => {}} />);
    expect(html).toContain("Two things are stored.");
    expect(html).not.toContain("Thinking…");
  });

  it("has no per-turn repository instructions control", () => {
    const turn = {id: "u", status: "complete", startedAt: 1000, finishedAt: 5000} as const;
    const items: Item[] = [
      {kind: "user", key: "u", text: "hello", turn},
      {kind: "assistant", key: "answer", text: "Hi", streaming: false, phase: "final_answer", turn},
    ];
    const html = renderToStaticMarkup(<Activity group={activityTurns(items, false)[0]} onAnswer={() => {}} />);
    expect(html).not.toContain("Repository instructions");
    expect(html).not.toContain("Inspect instructions for this turn");
  });

  it("replaces the live line with actionable approval controls while waiting", () => {
    let items = appendUser([], "edit");
    items = reduce(items, {type: "turn_started", id: "u", status: "working", startedAt: 1000});
    items = reduce(items, {type: "tool_call", id: "c", name: "edit_file", args: "{}"});
    items = reduce(items, {type: "approval_request", id: "p", name: "edit_file", args: "{}"});
    const html = renderToStaticMarkup(<Activity group={activityTurns(items, true)[0]} onAnswer={() => {}} />);
    expect(html).toContain("Waiting for approval");
    expect(html).toContain("Approve");
    expect(html).not.toContain('role="status"');
    expect(html).not.toContain("Thinking…");
  });

  it("summarizes several settled failures as one collapsed line", () => {
    const turn = {id: "u", status: "complete", startedAt: 1000, finishedAt: 5000} as const;
    const items: Item[] = [
      {kind: "user", key: "u", text: "memory", turn},
      {kind: "tool", key: "a", id: "a", name: "query_db", args: '{"sql":"SELECT name FROM sqlite_master"}', result: "denied", isErr: true, startedAt: 1000, turn},
      {kind: "tool", key: "b", id: "b", name: "query_db", args: '{"sql":"SELECT sql FROM sqlite_master"}', result: "denied", isErr: true, startedAt: 1000, turn},
      {kind: "assistant", key: "answer", text: "Done", streaming: false, phase: "final_answer", turn},
    ];
    const html = renderToStaticMarkup(<Activity group={activityTurns(items, false)[0]} onAnswer={() => {}} onOpenTool={() => {}} />);
    expect(html).toContain("2 actions failed");
    expect(html).not.toContain("sqlite_master");
    expect(html).not.toContain('role="status"');
  });

  it("renders settled progress text quietly while text still streaming keeps answer size", () => {
    const items: Item[] = [
      {kind: "assistant", key: "progress", text: "Checking where memory lives.", streaming: false, phase: "commentary"},
      {kind: "assistant", key: "live", text: "Two things", streaming: true},
    ];
    const html = renderToStaticMarkup(<ActivityItems items={items} active onAnswer={() => {}} />);
    expect(html.match(/evie-quiet/g)).toHaveLength(1);
    expect(html.indexOf("evie-quiet")).toBeLessThan(html.indexOf("Checking where memory lives."));
    expect(html.lastIndexOf("evie-quiet")).toBeLessThan(html.indexOf("Two things"));
    const settled = renderToStaticMarkup(<ActivityItems items={items} active={false} onAnswer={() => {}} />);
    expect(settled.match(/evie-quiet/g)).toHaveLength(2);
  });
});
