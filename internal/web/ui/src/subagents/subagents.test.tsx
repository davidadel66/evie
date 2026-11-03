import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { subagentAPI, type Subagent, type SubagentReport } from "../api/subagents";
import { Activity } from "../chat/Activity";
import { activityTurns, type ToolItem } from "../chat/activityModel";
import { appendUser, reduce, type Item } from "../store/reducer";
import { SubagentDetailView, SubagentList, SubagentReportView } from "./SubagentInspection";
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
    const html = renderToStaticMarkup(<SubagentDetailView child={child({objective: "Measure agent horizons", context: "Owner builds an agent harness.", actions: 2})} trace={{subagent: child({}), items}} now={Date.parse("2026-10-12T13:10:10Z")} onBack={() => undefined} onSelect={() => undefined} onView={() => undefined} />);
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
    const html = renderToStaticMarkup(<SubagentDetailView child={child({state: "succeeded", endedAt: "2026-10-12T13:10:42Z"})} report={report} now={0} onBack={() => undefined} onSelect={() => undefined} onView={() => undefined} />);
    for (const visible of ["&lt;script&gt;alert(1)&lt;/script&gt;", "onerror=alert(2)", "&lt;iframe", "bad", "local", "local picture"]) expect(html).toContain(visible);
    for (const element of ["<script", "<img", "<iframe", '<link rel="preload"']) expect(html).not.toContain(element);
    expect(html).not.toMatch(/<[^>]*\son[a-z]+=/i);
    const hrefs = [...html.matchAll(/href="([^"]*)"/g)].map((match) => match[1]);
    expect(hrefs).toEqual(["https://example.com/a", "http://example.org/b", "https://evil.example/pixel.png"]);
    expect(html).toMatch(/<a [^>]*href="https:\/\/example\.com\/a"[^>]*target="_blank"[^>]*rel="noopener noreferrer"[^>]*>source<\/a>/);
    expect(html).toMatch(/<a [^>]*href="https:\/\/evil\.example\/pixel\.png"[^>]*>tracking pixel<\/a>/);
  });

  it("shows a linked image in a report as its link's text, never an anchor inside an anchor", () => {
    const text = "Status: [![build](https://img.shields.io/x.svg)](https://github.com/o/r).";
    const report = {executionId: "child", worker: "research", status: "succeeded" as const, text, totalBytes: text.length, omittedBytes: 0};
    const html = renderToStaticMarkup(<SubagentDetailView child={child({state: "succeeded", endedAt: "2026-10-12T13:10:42Z"})} report={report} view="report" now={0} onBack={() => undefined} onSelect={() => undefined} onView={() => undefined} />);
    let depth = 0;
    for (const tag of html.match(/<\/?a\b/g) ?? []) {
      depth += tag === "<a" ? 1 : -1;
      expect(depth).toBeLessThanOrEqual(1);
    }
    expect([...html.matchAll(/href="([^"]*)"/g)].map((match) => match[1])).toEqual(["https://github.com/o/r"]);
    expect(html).toMatch(/<a [^>]*href="https:\/\/github\.com\/o\/r"[^>]*><span>build<\/span><\/a>/);
  });

  it("opens on Report once a child ended with a report, and on Activity while it runs or without one", () => {
    const items: Item[] = [{kind: "user", key: "u", text: "framed"}, {kind: "assistant", key: "a", text: "Activity sentinel.", streaming: false}];
    const done = child({state: "succeeded", endedAt: "2026-10-12T13:10:42Z"});
    const render = (props: Partial<Parameters<typeof SubagentDetailView>[0]>) =>
      renderToStaticMarkup(<SubagentDetailView child={done} trace={{subagent: done, items}} now={0} onBack={() => undefined} onSelect={() => undefined} onView={() => undefined} {...props} />);
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
      renderToStaticMarkup(<SubagentDetailView child={child({state, endedAt: state === "running" ? undefined : "2026-10-12T13:10:42Z"})} report={report} view="report" now={0} onBack={() => undefined} onSelect={() => undefined} onView={() => undefined} />);

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

  it("shows Evie's record of a report above the worker's text and outside it", () => {
    const report: SubagentReport = {
      executionId: "child", worker: "research", status: "succeeded", text: "Worker sentinel: horizons double.", totalBytes: 33, omittedBytes: 0,
      sources: [
        {url: "https://epoch.ai/data", fetched: true, cited: true},
        {url: "https://metr.org/blog", fetched: false, cited: true},
        {url: "https://fetched.example/uncited", fetched: true, cited: false},
      ],
      unverifiedUrls: ["https://unverified.example/claim"],
      limitations: ["Only two sources were read."],
      notes: ["URLs in unverified_urls are cited in the report but were never fetched or returned by a search."],
      usage: {input_tokens: 500, output_tokens: 50, total_tokens: 550, incomplete: true},
      modelResponses: 3,
    };
    const render = (value: SubagentReport) => renderToStaticMarkup(<SubagentDetailView child={child({state: value.status, endedAt: "2026-10-12T13:10:42Z"})} report={value} view="report" now={0} onBack={() => undefined} onSelect={() => undefined} onView={() => undefined} />);
    const html = render(report);

    // Evie's facts, in order, then the worker's text, then its limitations.
    const order = [
      "Research worker · Done",
      "At least 550 tokens · 3 model responses",
      "Sources Evie verified",
      "https://epoch.ai/data", "Fetched · cited in the report",
      "https://metr.org/blog", "Only listed by a search · cited in the report",
      "https://fetched.example/uncited", "Fetched · not cited",
      "Cited URLs Evie couldn&#x27;t verify", "https://unverified.example/claim",
      "Notes from Evie", "were never fetched or returned by a search.",
      "Written by the worker", "Worker sentinel: horizons double.",
      "Limitations, written by the worker", "Only two sources were read.",
    ];
    const at = order.map((text) => html.indexOf(text));
    expect(at).not.toContain(-1);
    expect(at).toEqual([...at].sort((a, b) => a - b));
    // None of Evie's facts sit inside the worker's text.
    const start = html.indexOf('<section aria-label="Written by the worker"');
    const worker = html.slice(start, html.indexOf("</section>", start));
    expect(worker).toContain("Worker sentinel");
    for (const fact of order.slice(0, order.indexOf("Written by the worker"))) expect(worker).not.toContain(fact);
    expect(html).toMatch(/<section aria-label="Limitations, written by the worker"/);
    expect(html).toMatch(/<section aria-label="Notes from Evie"/);

    // Source URLs open outside Evie, with no opener or referrer.
    const hrefs = [...html.matchAll(/href="([^"]*)"/g)].map((match) => match[1]);
    expect(hrefs).toEqual(["https://epoch.ai/data", "https://metr.org/blog", "https://fetched.example/uncited", "https://unverified.example/claim"]);
    for (const anchor of html.match(/<a [^>]*>/g) ?? []) {
      expect(anchor).toContain('target="_blank"');
      expect(anchor).toContain('rel="noopener noreferrer"');
      expect(anchor).toMatch(/referrerPolicy="no-referrer"/i);
    }

    // Complete usage is not a lower bound; one response is one.
    expect(render({...report, usage: {total_tokens: 1200}, modelResponses: 1})).toContain("1,200 tokens · 1 model response");
  });

  it("keeps the token counts a report's usage has, as a lower bound when any is missing", () => {
    const usage = (value: SubagentReport["usage"]) => renderToStaticMarkup(<SubagentReportView child={child({state: "succeeded", endedAt: "2026-10-12T13:10:42Z"})}
      report={{executionId: "child", worker: "research", status: "succeeded", text: "x", totalBytes: 1, omittedBytes: 0, usage: value, modelResponses: 2}} />);
    expect(usage({input_tokens: 5000, incomplete: true})).toContain("At least 5,000 tokens · 2 model responses");
    expect(usage({output_tokens: 70})).toContain("At least 70 tokens · 2 model responses");
    expect(usage({input_tokens: 5000, output_tokens: 70})).toContain(">5,070 tokens · 2 model responses");
    expect(usage({total_tokens: 1, input_tokens: 1})).toContain(">1 token · 2 model responses");
    expect(usage({incomplete: true})).toContain("Tokens not reported · 2 model responses");
  });

  it("marks sources recorded before Evie checked them, and shows hostile text as text", () => {
    const report: SubagentReport = {
      executionId: "child", worker: "research", status: "succeeded", text: "Old report.", totalBytes: 11, omittedBytes: 0,
      sources: [
        {url: "https://example.com/a", fetched: false, cited: false, legacy: true},
        {url: "javascript:alert(1)", fetched: false, cited: false, legacy: true},
      ],
      limitations: ["<script>alert(2)</script>"],
      notes: ["<img src=x onerror=alert(3)>"],
      usage: {total_tokens: 40},
      modelResponses: 1,
    };
    const html = renderToStaticMarkup(<SubagentDetailView child={child({state: "succeeded", endedAt: "2026-10-12T13:10:42Z"})} report={report} view="report" now={0} onBack={() => undefined} onSelect={() => undefined} onView={() => undefined} />);
    expect(html).toContain("Recorded before Evie checked sources");
    expect(html).not.toContain("Sources Evie verified");
    expect(html).not.toContain("cited in the report");
    expect(html.indexOf("Recorded before Evie checked sources")).toBeLessThan(html.indexOf("https://example.com/a"));
    expect(html).toContain("javascript:alert(1)");
    // A URL that is not a link is not styled as one either.
    expect(html).toContain('<span class="break-all">javascript:alert(1)</span>');
    for (const visible of ["&lt;script&gt;alert(2)&lt;/script&gt;", "&lt;img src=x onerror=alert(3)&gt;"]) expect(html).toContain(visible);
    for (const element of ["<script", "<img"]) expect(html).not.toContain(element);
    expect(html).not.toMatch(/<[^>]*\son[a-z]+=/i);
    expect([...html.matchAll(/href="([^"]*)"/g)].map((match) => match[1])).toEqual(["https://example.com/a"]);
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
