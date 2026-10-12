import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { subagentAPI, type Subagent } from "../api/subagents";
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

  it("reads one call's children and one child's trace for the open conversation", async () => {
    const fetchMock = vi.fn(async () => new Response('{"subagents":[]}'));
    vi.stubGlobal("fetch", fetchMock);
    await subagentAPI.list("session", "turn", "call");
    await subagentAPI.trace("session", "child");
    expect(fetchMock).toHaveBeenNthCalledWith(1, "/api/subagents/list", expect.objectContaining({body: JSON.stringify({sessionId: "session", turnId: "turn", callId: "call"})}));
    expect(fetchMock).toHaveBeenNthCalledWith(2, "/api/subagents/trace", expect.objectContaining({body: JSON.stringify({sessionId: "session", executionId: "child"})}));
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
