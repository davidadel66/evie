import { UsageView } from "ui";
import * as React from "react";

// The app root (App.tsx) sets these classes; every Evie screen sits on
// them, so previews render on the same dark surface and text color.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);

// Thirty days ending yesterday, the inspector's default range.
const period = { from: "2026-09-08", to: "2026-10-08", timezone: "America/Detroit" };
const days = Array.from({ length: 30 }, (_, index) => {
  const date = new Date(Date.UTC(2026, 8, 8 + index)).toISOString().slice(0, 10);
  return { date, index };
});

// A complete per-day counter set: every counter reported on every call, so
// the stacked chart can split cached input, other input and output.
const counters = (calls: number, input: number, cached: number, output: number, reasoning: number) => ({
  calls, measuredCalls: calls,
  input: { value: String(input), reported: calls }, cached: { value: String(cached), reported: calls },
  output: { value: String(output), reported: calls }, total: { value: String(input + output), reported: calls },
  reasoning: { value: String(reasoning), reported: calls }, cacheWrite: { value: String(Math.round(cached * 0.12)), reported: calls },
  cacheInput: String(input), cacheCalls: calls, cachePercent: input ? Math.round(cached / input * 1000) / 10 : null, anomalies: 0,
});
// Weekday-heavy conversation days; quiet weekends; two days with no calls.
const weight = [11, 14, 9, 17, 12, 4, 2, 13, 19, 8, 15, 10, 3, 0, 16, 12, 18, 9, 14, 5, 2, 12, 21, 11, 16, 13, 4, 0, 15, 20];
const evieDaily = days.map(({ date, index }) => {
  const calls = weight[index] * 3;
  const input = calls * 9100;
  return { date, metrics: counters(calls, input, Math.round(input * 0.71), calls * 640, calls * 180) };
});
const sum = (pick: (day: typeof evieDaily[number]) => number) => evieDaily.reduce((total, day) => total + pick(day), 0);
const evieTotals = counters(
  sum((day) => day.metrics.calls), sum((day) => Number(day.metrics.input.value)), sum((day) => Number(day.metrics.cached.value)),
  sum((day) => Number(day.metrics.output.value)), sum((day) => Number(day.metrics.reasoning.value)),
);

const evie = {
  id: "evie-conversation", kind: "evie", label: "Evie conversations", state: "ready", collectedAt: "2026-10-08T12:41:07Z",
  coverage: "Recorded conversation responses through OpenRouter", message: "Memory compaction and background compiler calls are excluded; they are measured under their own job runs.",
  account: null,
  summary: {
    metrics: evieTotals, daily: evieDaily, firstObserved: "2026-09-08T11:02:13Z", lastObserved: "2026-10-07T22:14:50Z",
    models: [
      { name: "anthropic/claude-sonnet-4.5", metrics: counters(612, 5700400, 4120000, 402100, 110200) },
      { name: "openai/gpt-5", metrics: counters(188, 1690200, 1118000, 121400, 46100) },
      { name: "google/gemini-2.5-flash", metrics: counters(104, 571900, 380200, 48700, 0) },
    ],
  },
};
const codexLocal = {
  id: "codex-local", kind: "codex-local", label: "Codex CLI · local sessions", state: "indexing", collectedAt: "2026-10-08T12:40:30Z",
  coverage: "Session logs under ~/.codex, parsed locally", account: null,
  summary: { metrics: counters(241, 3880000, 3410000, 96200, 40500), daily: [], firstObserved: "2026-09-12T08:55:00Z", lastObserved: "2026-10-07T21:03:11Z", models: [{ name: "gpt-5-codex", metrics: counters(241, 3880000, 3410000, 96200, 40500) }] },
};
const codexAccount = {
  id: "codex-account", kind: "codex-account", label: "Codex account", state: "ready", collectedAt: "2026-10-08T12:40:30Z",
  coverage: "Provider-reported account activity", summary: null,
  account: {
    plan: "Pro", lifetimeTokens: "418206113",
    daily: days.filter(({ index }) => weight[index] > 0 && index % 7 !== 6).map(({ date, index }) => ({ date, tokens: String(weight[index] * 48000 + 12000) })),
    windows: [
      { name: "Primary", usedPercent: 37, minutes: 300, resetsAt: "2026-10-08T16:00:00Z" },
      { name: "Weekly", usedPercent: 62, minutes: 10080, resetsAt: "2026-10-12T04:00:00Z" },
    ],
  },
};
const report = { period, sources: [evie, codexLocal, codexAccount] } as any;
const noop = () => {};
const callbacks = { onDraft: noop, onApply: noop, onRefresh: noop, onSelect: noop };

export const EvieConversations = () => (
  <AppFrame>
    <UsageView report={report} draft={period} selected="evie-conversation" loading={false} {...callbacks} />
  </AppFrame>
);

export const CodexAccount = () => (
  <AppFrame>
    <UsageView report={report} draft={period} selected="codex-account" loading={false} {...callbacks} />
  </AppFrame>
);

export const Refreshing = () => (
  <AppFrame>
    <UsageView report={report} draft={period} selected="evie-conversation" loading={true} {...callbacks} />
  </AppFrame>
);

export const Collecting = () => (
  <AppFrame>
    <UsageView draft={period} selected="evie-conversation" loading={true} {...callbacks} />
  </AppFrame>
);

export const ProblemWithPreviousReport = () => (
  <AppFrame>
    <UsageView report={report} draft={{ ...period, from: "2026-06-01" }} selected="evie-conversation" loading={false}
      problem="Choose a valid date range of up to 90 days." {...callbacks} />
  </AppFrame>
);

export const Unavailable = () => (
  <AppFrame>
    <UsageView draft={period} selected="evie-conversation" loading={false}
      problem="Usage is unavailable. Check that the current Evie server is running." {...callbacks} />
  </AppFrame>
);
