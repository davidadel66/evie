import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { UsageMetrics, UsageSource, UsageQuery } from "../api/usage";
import { UsageView } from "./Usage";
import { accountTokens, chartSegments, formatTokens } from "./usagePresentation";

const period: UsageQuery = { from: "2026-09-01", to: "2026-09-03", timezone: "America/Detroit" };
const metrics: UsageMetrics = {
  calls: 3, measuredCalls: 3,
  input: { value: "1500", reported: 3 }, cached: { value: "90", reported: 2 },
  output: { value: "30", reported: 2 }, total: { value: "1030", reported: 2 },
  reasoning: { value: null, reported: 0 }, cacheWrite: { value: "0", reported: 2 },
  cacheInput: "1000", cacheCalls: 2, cachePercent: 9, anomalies: 0,
};
const evie: UsageSource = {
  id: "evie", kind: "evie", label: "Evie", state: "partial", collectedAt: "2026-09-03T00:00:00Z",
  coverage: "Recorded conversation responses", message: "Compaction is excluded.", account: null,
  summary: { metrics, daily: [{ date: "2026-09-01", metrics }], models: [], firstObserved: null, lastObserved: null },
};
const codex: UsageSource = {
  id: "account", kind: "codex-account", label: "Codex account", state: "ready", coverage: "Provider dates", collectedAt: null, summary: null,
  account: { plan: "pro", lifetimeTokens: "9007199254740993", daily: [{ date: "2026-09-01", tokens: "5000" }, { date: "2026-09-03", tokens: "9999" }], windows: [] },
};
function render(selected: string, sources: UsageSource[]) {
  return renderToStaticMarkup(<UsageView report={{ period, sources }} draft={period} selected={selected} loading={false}
    onDraft={() => undefined} onApply={() => undefined} onRefresh={() => undefined} onSelect={() => undefined} />);
}

describe("Usage", () => {
  it("shows weighted cache and counter coverage without turning unknown into zero", () => {
    const html = render("evie", [evie]);
    for (const text of ["9.0%", "1,500", "Reasoning", "Cache writes", "0/3 calls", "2/3 calls", "Compaction is excluded.", "1,000 input tokens across 2 compatible calls"]) expect(html).toContain(text);
    expect(html).toContain("—");
    expect(html).toMatch(/Reported total<\/span>.*?1,030.*?2\/3 calls/);
    expect(chartSegments(metrics)).toBeNull();
  });
  it("does not double-count cache or reasoning in a complete daily stack", () => {
    const complete = { ...metrics, calls: 2, input: { value: "1000", reported: 2 }, cached: { value: "90", reported: 2 }, reasoning: { value: "20", reported: 2 } };
    expect(chartSegments(complete)).toEqual([90n, 910n, 30n]);
    expect(chartSegments({ ...complete, anomalies: 1 })).toBeNull();
  });
  it("keeps account dates separate and Evie visible when Codex is unavailable", () => {
    expect(accountTokens(codex, period)).toBe("5000");
    expect(accountTokens(codex, { ...period, from: "2026-09-02" })).toBeNull();
    const html = render("account", [codex, evie]);
    expect(html).toContain("Daily account activity · provider dates");
    expect(html).toContain("Missing days are unknown");
    expect(html).toContain("5,000");
    expect(html).toContain("Daily values");
    expect(html).toContain("Codex account daily token values");
    expect(html).toContain("Unavailable</td>");
    const unavailable = { ...codex, state: "unavailable", account: null };
    expect(render("evie", [unavailable, evie])).toContain("1,500");
  });
  it("formats large integer counters without converting them to floating point", () => {
    expect(formatTokens("9999")).toBe("9,999");
    expect(formatTokens("9007199254740993")).toBe("9007199.25B");
    expect(formatTokens(null)).toBe("—");
    expect(formatTokens("0")).toBe("0");
  });
});
