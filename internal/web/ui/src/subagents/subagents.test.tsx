import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { subagentAPI, type Subagent, type SubagentReport } from "../api/subagents";
import { Activity } from "../chat/Activity";
import { activityTurns, type ToolItem } from "../chat/activityModel";
import { appendUser, reduce, type Item } from "../store/reducer";
import { SubagentDetailView, SubagentList } from "./SubagentInspection";
import { startsSubagents, subagentActionText, subagentElapsed, subagentProgressLabel } from "./subagentView";

const child = (overrides: Partial<Subagent>): Subagent => ({
  executionId: "child", worker: "research", key: "key", objective: "Find evidence on scaling laws.", state: "running",
  createdAt: "2026-10-12T13:08:25Z", startedAt: "2026-10-12T13:10:00Z", actions: 0, ...overrides,
});

const delegateCall: ToolItem = {kind: "tool", key: "intent", id: "call", name: "delegate_research", args: '{"assignments":[]}', startedAt: 0};

describe("subagent presentation", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("counts finished children for the delegate row", () => {
    const list = [child({state: "succeeded"}), child({state: "partial"}), child({state: "running"}), child({state: "admitted"})];
    expect(subagentProgressLabel(list)).toBe("Researching · 2 of 4 done");
    expect(subagentProgressLabel([child({worker: "explore", state: "failed"})])).toBe("Exploring · 1 of 1 done");
    expect(subagentProgressLabel([])).toBeNull();
  });

  it("reads a child's latest action like its chat row", () => {
    expect(subagentActionText({name: "web_search", args: '{"query":"scaling laws 2026"}', done: false})).toBe("Search web requested · scaling laws 2026");
    expect(subagentActionText({name: "web_fetch", args: '{"url":"https://epoch.ai"}', done: true})).toBe("Fetched page · https://epoch.ai");
  });

  it("times a child from its start, and only its own run once it ended", () => {
    const now = Date.parse("2026-10-12T13:12:05Z");
    expect(subagentElapsed(child({}), now)).toBe("2m 5s");
    expect(subagentElapsed(child({endedAt: "2026-10-12T13:10:42Z", state: "succeeded"}), now)).toBe("42s");
    expect(subagentElapsed(child({startedAt: undefined, state: "admitted"}), now)).toBeNull();
  });

  it("opens delegate rows as subagents and leaves other tools alone", () => {
    expect(startsSubagents(delegateCall)).toBe(true);
    expect(startsSubagents({...delegateCall, name: "continue_research"})).toBe(true);
    expect(startsSubagents({...delegateCall, name: "web_search"})).toBe(false);
  });

  it("lists each child with its state, assignment, time and latest action", () => {
    const html = renderToStaticMarkup(<SubagentList tool={delegateCall} now={Date.parse("2026-10-12T13:11:00Z")} onOpen={() => undefined} list={[
      child({executionId: "a", state: "succeeded", objective: "Scaling evidence", endedAt: "2026-10-12T13:10:30Z", actions: 12, latest: {name: "web_fetch", args: '{"url":"https://epoch.ai"}', done: true}}),
      child({executionId: "b", state: "running", objective: "Self-improvement evidence", actions: 52, latest: {name: "web_search", args: '{"query":"AlphaEvolve"}', done: false}}),
      child({executionId: "c", state: "admitted", objective: "Governance", startedAt: undefined}),
    ]} />);
    expect(html).toContain("3 subagents");
    expect(html).toContain("Researching · 1 of 3 done");
    for (const text of ["Done", "Working", "Queued", "Scaling evidence", "30s", "1m 0s", "Fetched page · https://epoch.ai", "12 actions", "Search web requested · AlphaEvolve", "52 actions", "Waiting for a free slot"]) {
      expect(html).toContain(text);
    }
    expect(html).toContain('aria-label="Open subagent: Self-improvement evidence"');
    expect(html).toContain("Debug details");
  });

  it("shows a child's assignment, actions and report from its trace", () => {
    const items: Item[] = [
      {kind: "user", key: "u", text: "framed assignment"},
      {kind: "tool", key: "t1", id: "t1", name: "web_search", args: '{"query":"METR time horizons"}', result: "results", startedAt: 0},
      {kind: "tool", key: "t2", id: "t2", name: "web_fetch", args: '{"url":"https://metr.org"}', startedAt: 0},
      {kind: "assistant", key: "a", text: "Findings: horizons double every seven months.", streaming: false},
    ];
    const html = renderToStaticMarkup(<SubagentDetailView child={child({objective: "Measure agent horizons", context: "Owner builds an agent harness.", actions: 2})} trace={{subagent: child({}), items}} now={Date.parse("2026-10-12T13:10:10Z")} onBack={() => undefined} onSelect={() => undefined} />);
    for (const text of ["All subagents", "Working", "Measure agent horizons", "Context Evie gave it", "Owner builds an agent harness.", "10s", "2 actions", "Findings: horizons double every seven months."]) expect(html).toContain(text);
    expect(html).not.toContain("framed assignment");
  });

  it("shows a finished child's report as an inert document", () => {
    const text = [
      "## Findings",
      "Quoted page: <script>alert(1)</script> and <img src=x onerror=alert(2)> inline.",
      "",
      "<div class=\"embed\"><iframe src=\"https://evil.example/frame\"></iframe></div>",
      "",
      "See [bad](javascript:alert(3)), [local](/api/settings), [source](https://example.com/a) and [plain](http://example.org/b).",
      "",
      "![tracking pixel](https://evil.example/pixel.png) ![local picture](/logo.png)",
    ].join("\n");
    const report = {executionId: "child", worker: "research", status: "succeeded" as const, text, totalBytes: text.length, omittedBytes: 0};
    const html = renderToStaticMarkup(<SubagentDetailView child={child({state: "succeeded", endedAt: "2026-10-12T13:10:42Z"})} report={report} now={0} onBack={() => undefined} onSelect={() => undefined} />);
    for (const visible of ["&lt;script&gt;alert(1)&lt;/script&gt;", "onerror=alert(2)", "&lt;iframe", "bad", "local", "local picture"]) expect(html).toContain(visible);
    for (const element of ["<script", "<img", "<iframe", '<link rel="preload"']) expect(html).not.toContain(element);
    expect(html).not.toMatch(/<[^>]*\son[a-z]+=/i);
    const hrefs = [...html.matchAll(/href="([^"]*)"/g)].map((match) => match[1]);
    expect(hrefs).toEqual(["https://example.com/a", "http://example.org/b", "https://evil.example/pixel.png"]);
    expect(html).toMatch(/<a [^>]*href="https:\/\/example\.com\/a"[^>]*target="_blank"[^>]*rel="noopener noreferrer"[^>]*>source<\/a>/);
    expect(html).toMatch(/<a [^>]*href="https:\/\/evil\.example\/pixel\.png"[^>]*>tracking pixel<\/a>/);
  });

  it("opens on Report once a child ended with a report, and on Activity while it runs or without one", () => {
    const items: Item[] = [{kind: "user", key: "u", text: "framed"}, {kind: "assistant", key: "a", text: "Activity sentinel.", streaming: false}];
    const done = child({state: "succeeded", endedAt: "2026-10-12T13:10:42Z"});
    const render = (props: Partial<Parameters<typeof SubagentDetailView>[0]>) =>
      renderToStaticMarkup(<SubagentDetailView child={done} trace={{subagent: done, items}} now={0} onBack={() => undefined} onSelect={() => undefined} {...props} />);
    const selected = (html: string) => html.match(/aria-selected="true"[^>]*>([^<]+)</)?.[1];

    const withReport = render({report: {executionId: "child", worker: "research", status: "succeeded", text: "Report sentinel.", totalBytes: 16, omittedBytes: 0}});
    expect(selected(withReport)).toBe("Report");
    expect(withReport).toContain("Report sentinel.");
    expect(withReport).not.toContain("Activity sentinel.");
    // Before its report is read, a child that succeeded is expected to have one.
    expect(selected(render({}))).toBe("Report");

    const running = render({child: child({state: "running"})});
    expect(selected(running)).toBe("Activity");
    expect(running).toContain("Activity sentinel.");

    const without = render({child: child({state: "interrupted", endedAt: "2026-10-12T13:10:42Z"}), report: {executionId: "child", worker: "research", status: "interrupted", reason: "no_accepted_final_answer", totalBytes: 0, omittedBytes: 0}});
    expect(selected(without)).toBe("Activity");
    expect(without).toContain("Activity sentinel.");
    // A record from before final report events succeeded without one.
    expect(selected(render({report: {executionId: "child", worker: "research", status: "succeeded", totalBytes: 0, omittedBytes: 0}}))).toBe("Activity");

    // A tab David picked stays picked.
    expect(selected(render({view: "activity", report: {executionId: "child", worker: "research", status: "succeeded", text: "x", totalBytes: 1, omittedBytes: 0}}))).toBe("Activity");
  });

  it("says when a child produced no report, stopped partway, is still running, or was cut", () => {
    const render = (state: Subagent["state"], report?: SubagentReport) =>
      renderToStaticMarkup(<SubagentDetailView child={child({state, endedAt: state === "running" ? undefined : "2026-10-12T13:10:42Z"})} report={report} view="report" now={0} onBack={() => undefined} onSelect={() => undefined} />);

    const none = render("interrupted", {executionId: "child", worker: "explore", status: "interrupted", reason: "no_accepted_final_answer", totalBytes: 0, omittedBytes: 0});
    expect(none).toContain("This worker produced no report.");
    expect(none).toContain("Explore worker · Interrupted · Reason: no accepted final answer");
    expect(none).not.toContain("Written by the worker");

    const partial = render("partial", {executionId: "child", worker: "research", status: "partial", reason: "time_budget", text: "Half the findings.", totalBytes: 18, omittedBytes: 0});
    expect(partial).toContain("Research worker · Partial report · Reason: time budget");
    expect(partial).toContain("This report is partial: the worker stopped before it finished.");
    // Evie's line sits above the worker's text, outside it.
    expect(partial.indexOf("Research worker · Partial report")).toBeLessThan(partial.indexOf("Written by the worker"));
    expect(partial.indexOf("Written by the worker")).toBeLessThan(partial.indexOf("Half the findings."));

    expect(render("running")).toContain("The report appears when the worker finishes.");

    const cut = render("succeeded", {executionId: "child", worker: "research", status: "succeeded", text: "Long findings.", totalBytes: 1_200_011, omittedBytes: 151_436});
    expect(cut).toContain("Cut for display: the last 151,436 of its 1,200,011 bytes are not shown.");
    expect(render("succeeded", {executionId: "child", worker: "research", status: "succeeded", text: "Whole.", totalBytes: 6, omittedBytes: 0})).not.toContain("Cut for display");
  });

  it("reads one call's children and one child's trace for the open conversation", async () => {
    const fetchMock = vi.fn(async () => new Response('{"subagents":[]}'));
    vi.stubGlobal("fetch", fetchMock);
    await subagentAPI.list("session", "turn", "call");
    await subagentAPI.trace("session", "child");
    expect(fetchMock).toHaveBeenNthCalledWith(1, "/api/subagents/list", expect.objectContaining({body: JSON.stringify({sessionId: "session", turnId: "turn", callId: "call"})}));
    expect(fetchMock).toHaveBeenNthCalledWith(2, "/api/subagents/trace", expect.objectContaining({body: JSON.stringify({sessionId: "session", executionId: "child"})}));
  });

  it("reads one child's report for the open conversation", async () => {
    const fetchMock = vi.fn(async () => new Response('{"report":{"executionId":"child","worker":"research","status":"succeeded","text":"x","totalBytes":1,"omittedBytes":0}}'));
    vi.stubGlobal("fetch", fetchMock);
    expect((await subagentAPI.report("session", "child")).text).toBe("x");
    expect(fetchMock).toHaveBeenCalledWith("/api/subagents/report", expect.objectContaining({method: "POST", body: JSON.stringify({sessionId: "session", executionId: "child"})}));
  });

  it("names a waiting delegate row by what it does", () => {
    let items = appendUser([], "research this");
    items = reduce(items, {type: "turn_started", id: "u", status: "working", startedAt: 1000});
    items = reduce(items, {type: "tool_call", id: "call", name: "delegate_research", args: '{"assignments":[]}'});
    const html = renderToStaticMarkup(<Activity group={activityTurns(items, true)[0]} onAnswer={() => {}} onOpenTool={() => {}} />);
    expect(html).toContain("Research with subagents requested");
    expect(html).not.toContain("delegate research");
  });
});
