import type { CashFlowCategory, CashFlowMonth } from "../api/spending";
import { formatFlow, shiftMonth } from "./spendingPresentation";

export function cashFlowHistoryEnd(month: string, end: string): string {
  if (month > end) return month;
  const start = shiftMonth(end, -11) ?? "0001-01";
  return month < start ? shiftMonth(month, 11) ?? "9999-12" : end;
}

export function percentOf(numerator: string, denominator: string): string {
  const top = BigInt(numerator), bottom = BigInt(denominator);
  if (bottom <= 0n) return "—";
  const tenths = ((top < 0n ? -top : top) * 1000n + bottom / 2n) / bottom;
  return `${top < 0n && tenths > 0n ? "−" : ""}${tenths / 10n}.${tenths % 10n}%`;
}

export function categoryLabel(row: CashFlowCategory): string {
  if (row.kind === "unclassified") return "Unclassified";
  if (row.kind === "unreconciled") return "Needs review";
  const name = row.category ?? "Unnamed category";
  return ["unclassified", "needs review"].includes(name.trim().toLowerCase()) ? `${name} (category)` : name;
}

export function categoryKey(row: CashFlowCategory): string {
  return `${row.kind}:${row.category ?? ""}`;
}

export function categoryWidth(cents: string, total: string): number {
  const numerator = BigInt(cents), denominator = BigInt(total);
  return denominator <= 0n ? 0 : Math.min(100, Number(numerator * 10000n / denominator) / 100);
}

export function sortedCategories(rows: CashFlowCategory[], side: "inflowCents" | "outflowCents"): CashFlowCategory[] {
  return rows.filter((row) => BigInt(row[side]) > 0n).sort((a, b) => {
    const left = BigInt(a[side]), right = BigInt(b[side]);
    return left === right ? categoryKey(a).localeCompare(categoryKey(b)) : left > right ? -1 : 1;
  });
}

export function cashFlowScale(history: CashFlowMonth[]): bigint {
  const max = history.reduce((value, month) => [month.inflowCents, month.outflowCents].reduce((largest, cents) => BigInt(cents) > largest ? BigInt(cents) : largest, value), 0n);
  if (max === 0n) return 100n;
  const target = max + (max + 9n) / 10n;
  const magnitude = 10n ** BigInt(target.toString().length - 1);
  return [1n, 2n, 5n, 10n].map((factor) => factor * magnitude).find((value) => value >= target)!;
}

export function cashFlowY(cents: string, scale: bigint): number {
  return 124 - Number(BigInt(cents) * 1000000n / scale) / 10000;
}

export function axisAmount(cents: bigint): string {
  const abs = cents < 0n ? -cents : cents;
  const sign = cents < 0n ? "−" : "";
  for (const [size, suffix] of [[100000000000n, "B"], [100000000n, "M"], [100000n, "K"]] as const) {
    if (abs >= size) {
      const tenths = abs * 10n / size;
      return `${sign}${tenths / 10n}${tenths % 10n ? `.${tenths % 10n}` : ""}${suffix}`;
    }
  }
  return formatFlow(cents.toString(), false).replace(/\.00$/, "");
}
