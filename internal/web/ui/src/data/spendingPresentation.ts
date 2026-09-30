export function shiftMonth(month: string, offset: number): string | null {
  const [year, number] = month.split("-").map(Number);
  const index = year * 12 + number - 1 + offset;
  if (index < 12 || index >= 120000) return null;
  return `${String(Math.floor(index / 12)).padStart(4, "0")}-${String(index % 12 + 1).padStart(2, "0")}`;
}

export function monthLabel(month: string): string {
  return new Date(`${month}-01T12:00:00Z`).toLocaleDateString("en-US", { month: "long", year: "numeric", timeZone: "UTC" });
}

export function dayLabel(date: string): string {
  return new Date(`${date}T12:00:00Z`).toLocaleDateString("en-US", { month: "long", day: "numeric", year: "numeric", timeZone: "UTC" });
}

export function calendarWeeks(month: string): (string | null)[][] {
  const [year, number] = month.split("-").map(Number);
  const start = new Date(`${month}-01T12:00:00Z`).getUTCDay();
  const dates: (string | null)[] = Array.from({ length: start }, () => null);
  for (let day = 1; day <= 31; day++) {
    const date = spendingDate(year, number - 1, day);
    if (date) dates.push(date);
  }
  while (dates.length % 7) dates.push(null);
  return Array.from({ length: dates.length / 7 }, (_, index) => dates.slice(index * 7, index * 7 + 7));
}

export function spendingDate(year: number, month: number, day: number): string | null {
  const date = new Date(0);
  date.setUTCFullYear(year, month, day);
  return date.getUTCMonth() === month ? `${String(year).padStart(4, "0")}-${String(month + 1).padStart(2, "0")}-${String(day).padStart(2, "0")}` : null;
}

// Work in integer cents through formatting; even large totals retain every cent.
export function formatFlow(cents: string, signed = true): string {
  const value = BigInt(cents);
  const magnitude = value < 0n ? -value : value;
  const sign = value < 0n ? "−" : value > 0n && signed ? "+" : "";
  return `${sign}${(magnitude / 100n).toLocaleString("en-US")}.${String(magnitude % 100n).padStart(2, "0")}`;
}

export function flowColor(cents: string, maximum: bigint): string {
  const value = BigInt(cents);
  if (value === 0n || maximum === 0n) return "var(--color-selected)";
  const magnitude = value < 0n ? -value : value;
  const proportion = Math.min(100, Number(magnitude * 100n / maximum));
  const strength = 18 + Math.round(Math.sqrt(proportion / 100) * 32);
  return `color-mix(in srgb, var(--color-${value > 0n ? "teal" : "danger"}) ${strength}%, var(--color-card))`;
}
