export type Counter = { value: string | null; reported: number };
export type UsageMetrics = {
  calls: number; measuredCalls: number;
  input: Counter; cached: Counter; output: Counter; total: Counter;
  reasoning: Counter; cacheWrite: Counter;
  cacheInput: string | null; cacheCalls: number; cachePercent: number | null; anomalies: number;
};
export type UsageDay = { date: string; metrics: UsageMetrics };
export type UsageSummary = {
  metrics: UsageMetrics; daily: UsageDay[];
  models: { name: string; metrics: UsageMetrics }[];
  firstObserved: string | null; lastObserved: string | null;
};
export type UsageAccount = {
  plan: string; lifetimeTokens: string | null;
  daily: { date: string; tokens: string }[];
  windows: { name: string; usedPercent: number; minutes: number; resetsAt: string }[];
};
export type UsageSource = {
  id: string; kind: "evie" | "codex-local" | "codex-account";
  label: string; state: string; coverage: string; message?: string;
  collectedAt: string | null; summary: UsageSummary | null; account: UsageAccount | null;
};
export type UsageQuery = { from: string; to: string; timezone: string };
export type UsageReport = { period: UsageQuery; sources: UsageSource[] };

export async function inspectUsage(query: UsageQuery, signal?: AbortSignal): Promise<UsageReport> {
  const response = await fetch("/api/data/usage/summary", {
    method: "POST", headers: { "Content-Type": "application/json" },
    body: JSON.stringify(query), signal,
  });
  if (!response.ok) {
    throw new Error(response.status === 400
      ? "Choose a valid date range of up to 90 days."
      : "Usage is unavailable. Check that the current Evie server is running.");
  }
  return response.json();
}

export function calendarDate(date: Date): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
}
export function shiftDate(date: string, days: number): string {
  const value = new Date(`${date}T12:00:00Z`);
  value.setUTCDate(value.getUTCDate() + days);
  return value.toISOString().slice(0, 10);
}
export function defaultUsageQuery(): UsageQuery {
  const today = calendarDate(new Date());
  return { from: shiftDate(today, -29), to: shiftDate(today, 1), timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC" };
}
