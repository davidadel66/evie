import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, onTestFinished, vi } from "vitest";
import { appendUser, reduce, type Item } from "../store/reducer";
import { Activity, ActivityItems } from "./Activity";
import { actionSummary, activityTurns, thoughtTitle, toolLabel } from "./activityModel";

// The header button: while working, the live line.
const headerOf = (html: string) => html.slice(0, html.indexOf("</button>"));

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
    expect(html).toContain('title="Elapsed wall time for this turn');
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

    items = reduce(items, {type: "delta", text: "Two things are stored.\n\n1. claims"});
    html = renderToStaticMarkup(<Activity group={activityTurns(items, true)[0]} onAnswer={() => {}} onOpenTool={() => {}} />);
    expect(html).toContain("Two things are stored.");
    expect(html).toContain("Writing…");
    expect(html).not.toContain("Thinking…");
  });

  it("makes the header the live line: the current title, shimmering, then the running clock", () => {
    vi.useFakeTimers({now: 100_000});
    onTestFinished(() => { vi.useRealTimers(); });
    const header = (items: Item[]) => headerOf(renderToStaticMarkup(<Activity group={activityTurns(items, true)[0]} onAnswer={() => {}} onOpenTool={() => {}} />));
    let items = appendUser([], "think about it");
    items = reduce(items, {type: "turn_started", id: "u", status: "working", startedAt: 88_000});
    items = reduce(items, {type: "reasoning", text: "I need to think about what David is asking. Let me check memory for prior con"});
    let line = header(items);
    expect(line).toContain('role="status"');
    expect(line).toContain("evie-shimmer");
    expect(line).toContain("Thinking about what David is asking");
    expect(line).toContain("· 12s");
    expect(line).not.toContain("prior con");
    expect(line).not.toContain("Working…");

    items = reduce(items, {type: "reasoning", text: "text."});
    items = reduce(items, {type: "reasoning_done"});
    expect(header(items)).toContain("Checking memory for prior context");

    items = reduce(items, {type: "tool_call", id: "m", name: "memory_search", args: '{"query":"superintelligence"}'});
    line = header(items);
    expect(line).toContain("Search memory requested · superintelligence");
    expect(line).not.toContain("Checking memory");

    items = reduce(items, {type: "tool_result", id: "m", content: "none", isError: false});
    expect(header(items)).toContain("Thinking…");

    items = reduce(items, {type: "delta", text: "Nothing is saved about it.\n\n- No memory records"});
    line = header(items);
    expect(line).toContain("Writing…");
    expect(renderToStaticMarkup(<Activity group={activityTurns(items, true)[0]} onAnswer={() => {}} />)).toContain("No memory records");
  });

  it("keeps a short progress message off the page and shows its title on the line", () => {
    const render = (items: Item[]) => renderToStaticMarkup(<Activity group={activityTurns(items, true)[0]} onAnswer={() => {}} onOpenTool={() => {}} />);
    const body = (html: string) => html.slice(html.indexOf("</button>"));
    let items = appendUser([], "last 10 charges on my citi card");
    items = reduce(items, {type: "turn_started", id: "u", status: "working", startedAt: 1000});
    items = reduce(items, {type: "delta", text: "I'll look at the recent Citi card tran"});
    let html = render(items);
    expect(headerOf(html)).toContain("Writing…");
    expect(body(html)).not.toContain("Citi card tran");

    items = reduce(items, {type: "delta", text: "sactions.\n\n"});
    html = render(items);
    expect(headerOf(html)).toContain("Looking at the recent Citi card transactions");
    expect(body(html)).not.toContain("Citi card transactions");

    items = reduce(items, {type: "assistant_done", content: "I'll look at the recent Citi card transactions.", parts: [{text: "I'll look at the recent Citi card transactions.", phase: "commentary"}], terminal: false});
    expect(headerOf(render(items))).toContain("Looking at the recent Citi card transactions");

    const long = "These are all payments to Citi made from a bank account, not charges on the card itself. ".repeat(3);
    items = reduce(items, {type: "delta", text: long});
    html = render(items);
    expect(headerOf(html)).toContain("Writing…");
    expect(body(html)).toContain("not charges on the card itself");
  });

  it("titles the live line with a requested delegate's own label", () => {
    let items = appendUser([], "research it");
    items = reduce(items, {type: "turn_started", id: "u", status: "working", startedAt: 1000});
    items = reduce(items, {type: "tool_call", id: "d", name: "delegate_research", args: "{}"});
    const line = headerOf(renderToStaticMarkup(<Activity group={activityTurns(items, true)[0]} onAnswer={() => {}} onOpenTool={() => {}} />));
    expect(line).toContain("Research with subagents requested");
    expect(line).toContain("evie-shimmer");
  });

  it("heads a completed turn with its successful actions, then its time, without the shimmer", () => {
    const turn = {id: "u", status: "complete", startedAt: 1000, finishedAt: 5000} as const;
    const items: Item[] = [
      {kind: "user", key: "u", text: "septic costs", turn},
      {kind: "tool", key: "s", id: "s", name: "web_search", args: '{"query":"septic permit fee"}', result: "results", startedAt: 1000, turn},
      {kind: "tool", key: "f", id: "f", name: "web_fetch", args: '{"url":"https://example.org"}', result: "page", startedAt: 1000, turn},
      {kind: "tool", key: "c", id: "c", name: "bash", args: '{"command":"false"}', result: "exit 1", isErr: true, startedAt: 1000, turn},
      {kind: "assistant", key: "a", text: "Done", streaming: false, phase: "final_answer", turn},
    ];
    const line = headerOf(renderToStaticMarkup(<Activity group={activityTurns(items, false)[0]} onAnswer={() => {}} onOpenTool={() => {}} />));
    expect(line).toContain("Searched the web, read a page");
    expect(line).toContain("· 4s");
    expect(line).not.toContain("Worked");
    expect(line).not.toContain("evie-shimmer");
    expect(line).not.toContain('role="status"');
  });

  it("names each kind of successful action once, in order of first use", () => {
    const tool = (key: string, name: string, extra: Partial<Extract<Item, {kind: "tool"}>> = {}): Item =>
      ({kind: "tool", key, id: key, name, args: "{}", result: "ok", startedAt: 0, ...extra});
    expect(actionSummary([
      tool("a", "web_search"), tool("b", "web_fetch"), tool("c", "web_search"), tool("d", "web_fetch"),
      tool("e", "read_file"), tool("f", "edit_file"), tool("g", "edit_file"), tool("h", "edit_file"),
      tool("i", "bash"), tool("j", "query_db"), tool("k", "edit_db"), tool("l", "memory_search"), tool("m", "memory_traverse"),
      tool("n", "delegate_research"), tool("o", "mystery_tool"),
      tool("p", "bash", {isErr: true}), tool("q", "edit_file", {isErr: true, approval: {reqId: "q", state: "declined"}}),
      tool("r", "read_file", {result: undefined}),
    ])).toBe("Searched the web, read 2 pages, read a file, edited 3 files, ran a command, queried data, edited data, checked memory, used subagents, used a tool");
    expect(actionSummary([tool("a", "bash"), tool("b", "bash"), tool("c", "x_tool"), tool("d", "y_tool")])).toBe("Ran 2 commands, used 2 tools");
    expect(actionSummary([tool("a", "bash", {isErr: true})])).toBeNull();
    expect(actionSummary([])).toBeNull();
  });

  it("lists each thought by its title, opening to its full text only when there is more", () => {
    const trace = (text: string, streaming = false) => renderToStaticMarkup(<ActivityItems
      items={[{kind: "reasoning", key: "r", text, streaming, startedAt: 0}]} active={streaming} onAnswer={() => {}} />);
    const many = trace("The user wants permit costs. Let me check the county fee schedule.");
    expect(many).toContain("<details");
    expect(many.slice(0, many.indexOf("</summary>"))).toContain("Checking the county fee schedule");
    expect(many.slice(many.indexOf("</summary>"))).toContain("The user wants permit costs. Let me check the county fee schedule.");
    expect(many).not.toContain("Reasoning summary");

    const one = trace("Judging which cost sources are reliable.");
    expect(one).not.toContain("<details");
    expect(one).not.toContain("<svg");
    expect(one.match(/Judging which cost sources are reliable\./g)).toHaveLength(1);

    expect(trace("- Check the transcripts table.")).not.toContain("<details");
    expect(trace("Let me check the fees.")).not.toContain("<details");
    expect(trace("Weighing a shared we", true)).toContain("Thinking…");
  });

  it("titles a thought from its heading, else its latest complete sentence", () => {
    expect(thoughtTitle("Scaling slowed", true)).toBeNull();
    expect(thoughtTitle("Scaling slowed", false)).toBe("Scaling slowed");
    expect(thoughtTitle("First idea. Then the second", true)).toBe("First idea.");
    expect(thoughtTitle("Compare tools, e.g. search and fetch. Done", true)).toBe("Compare tools, e.g. search and fetch.");
    expect(thoughtTitle("**Planning the answer**\n\nLet me outline the cruxes. Then", true)).toBe("Planning the answer");
    expect(thoughtTitle("- Check the `transcripts` table.\n", true)).toBe("Check the transcripts table.");
    expect(thoughtTitle("First idea. Compare tools, e.g.", true)).toBe("First idea.");
    expect(thoughtTitle("First idea. Check the U.S. Census", true)).toBe("First idea.");
    expect(thoughtTitle("First idea. Ask Dr. Smith about it", true)).toBe("First idea.");
    expect(thoughtTitle("First idea. Try version 3.", true)).toBe("First idea.");
    expect(thoughtTitle("First idea. Try version 3.", false)).toBe("Try version 3.");
    expect(thoughtTitle("Scaling slowed in 2026. Then RL took over.", false)).toBe("Then RL took over.");
    expect(thoughtTitle("Ask Dr. Smith about the U.S. data. Done", true)).toBe("Ask Dr. Smith about the U.S. data.");
  });

  it("titles a thought with its latest intent, written as an action", () => {
    expect(thoughtTitle("The user wants permit costs. Let me check the county fee schedule. It may be a PDF.", false)).toBe("Checking the county fee schedule");
    expect(thoughtTitle("Let me look", false)).toBe("Looking");
    expect(thoughtTitle("Let me look", true)).toBeNull();
    expect(thoughtTitle("Scaling slowed. Let me check the dat", true)).toBe("Scaling slowed.");
    expect(thoughtTitle("Let me look up county septic costs.", false)).toBe("Looking up county septic costs");
    expect(thoughtTitle("I need to compare the two quotes.", false)).toBe("Comparing the two quotes");
    expect(thoughtTitle("I'll run the tests again.", false)).toBe("Running the tests again");
    expect(thoughtTitle("I’ll plan the reply.", false)).toBe("Planning the reply");
    expect(thoughtTitle("Now I should write the summary.", false)).toBe("Writing the summary");
    expect(thoughtTitle("Okay, let me quickly verify the totals.", false)).toBe("Verifying the totals");
    expect(thoughtTitle("I'm going to search for local well drillers.", false)).toBe("Searching for local well drillers");
    expect(thoughtTitle("I want to edit the config.", false)).toBe("Editing the config");
    expect(thoughtTitle("I'm checking the lender rules.", false)).toBe("Checking the lender rules");
    expect(thoughtTitle("Let me see if the file exists.", false)).toBe("Checking if the file exists");
    expect(thoughtTitle("I should tie this together.", false)).toBe("Tying this together");
    expect(thoughtTitle("Let me submit the form.", false)).toBe("Submitting the form");
    expect(thoughtTitle("Let me continue the list.", false)).toBe("Continuing the list");
    expect(thoughtTitle("Let me check the fee schedule, since the user asked about permits.", false)).toBe("Checking the fee schedule");
    expect(thoughtTitle("Let me compare A, B, and C.", false)).toBe("Comparing A, B, and C");
    expect(thoughtTitle("**Weighing lender rules**\n\nLet me check the bank.", false)).toBe("Weighing lender rules");
    expect(thoughtTitle("Let me ask Dr. Smith about the fees.", false)).toBe("Asking Dr. Smith about the fees");
    expect(thoughtTitle("Let me check the U.S. Census data.", false)).toBe("Checking the U.S. Census data");
    expect(thoughtTitle("Let me compare tools, e.g. search and fetch.", false)).toBe("Comparing tools, e.g. search and fetch");
    expect(thoughtTitle("I should not assume the totals.", false)).toBe("I should not assume the totals.");
    expect(thoughtTitle("I should probably search the county site.", false)).toBe("Searching the county site");
    expect(thoughtTitle("Let me check the 2024–2025 fee table.", false)).toBe("Checking the 2024–2025 fee table");
    expect(thoughtTitle("Let me check the fees—they changed.", false)).toBe("Checking the fees");
    expect(thoughtTitle("Let me re-check the totals.", false)).toBe("Re-checking the totals");
    expect(thoughtTitle("Let’s check the fees.", false)).toBe("Checking the fees");
    expect(thoughtTitle("Let me debug the test.", false)).toBe("Debugging the test");
    expect(thoughtTitle("I'll rerun the suite.", false)).toBe("Rerunning the suite");
    expect(thoughtTitle("Let me format the table.", false)).toBe("Formatting the table");
    expect(thoughtTitle("Let me reopen the file.", false)).toBe("Reopening the file");
    expect(thoughtTitle("I'll still need to verify this.", false)).toBe("Verifying this");
    expect(thoughtTitle("I should double-check the totals.", false)).toBe("Double-checking the totals");
    expect(thoughtTitle("Let me apply the patch.", false)).toBe("Applying the patch");
    expect(thoughtTitle("Let me deal with the lender.", false)).toBe("Dealing with the lender");
    expect(thoughtTitle("Let me check the fees, because they changed.", false)).toBe("Checking the fees");
    // "Parcel B." reads as an initial, so the sentences join; the intent after it still counts.
    expect(thoughtTitle("The user wants costs for Parcel B. Let me check the Michigan rules. It might be", true)).toBe("Checking the Michigan rules");
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
