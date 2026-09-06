import type { UsageMetrics, UsageSource, UsageQuery } from "../api/usage";

export function chartSegments(metrics: UsageMetrics): bigint[] | null {
  if (metrics.calls === 0) return [0n, 0n, 0n];
  if (metrics.anomalies > 0 || [metrics.input, metrics.cached, metrics.output].some((counter) => counter.value === null || counter.reported !== metrics.calls)) return null;
  const input = BigInt(metrics.input.value!); const cached = BigInt(metrics.cached.value!);
  if (cached > input) return null;
  return [cached, input - cached, BigInt(metrics.output.value!)];
}
export function accountTokens(source: UsageSource, period: UsageQuery): string | null {
  const days = source.account?.daily.filter((day) => day.date >= period.from && day.date < period.to);
  return days?.length ? String(days.reduce((sum, day) => sum + BigInt(day.tokens), 0n)) : null;
}
export function formatTokens(value?: string | null): string {
  if (value == null) return "—";
  const count = BigInt(value);
  if (count < 10000n) return count.toLocaleString();
  const unit = count >= 1000000000n ? 1000000000n : count >= 1000000n ? 1000000n : 1000n;
  const suffix = unit === 1000000000n ? "B" : unit === 1000000n ? "M" : "k";
  const hundredths = count * 100n / unit;
  return `${hundredths / 100n}.${String(hundredths % 100n).padStart(2, "0")}${suffix}`;
}
