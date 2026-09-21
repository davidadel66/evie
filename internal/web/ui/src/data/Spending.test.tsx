import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { SpendingReport } from "../api/spending";
import { SpendingView } from "./Spending";
import { calendarWeeks, flowColor, formatFlow, monthLabel, shiftMonth, spendingDate } from "./spendingPresentation";

const report: SpendingReport = {
  year: 2024, years: [2024, 2023], linkedBanks: 2,
  pendingTransactions: 2, undatedTransactions: 1, currency: null, refreshedAt: null,
  days: [
    { date: "2024-02-29", netCents: "-1234", inflowCents: "0", outflowCents: "1234", transactions: 1 },
    { date: "2024-01-01", netCents: "250025", inflowCents: "300000", outflowCents: "49975", transactions: 3 },
    { date: "2024-01-02", netCents: "0", inflowCents: "5000", outflowCents: "5000", transactions: 2 },
  ],
};
const callbacks = { onMonth: () => undefined, onRefresh: () => undefined, onRetry: () => undefined };
function render(overrides: Partial<Parameters<typeof SpendingView>[0]> = {}) {
  return renderToStaticMarkup(<SpendingView month="2024-01" report={report} loading={false} refreshing={false} today="2024-01-20" {...callbacks} {...overrides} />);
}

describe("Spending monthly overview", () => {
  it("shows only the selected month with arrows and separate Overview and Transactions views", () => {
    const html = render();
    for (const expected of ["Daily net flow", "Fetch latest", "+2,500.25", "money in 3,000.00; money out 499.75; 3 transactions", "Previous month", "Next month", "January 2024", "Overview", "Transactions"]) expect(html).toContain(expected);
    expect(html).toContain('scope="col"');
    expect(html).not.toContain("February");
    expect(html).not.toContain("−12.34");
    expect(html).not.toContain("Heatmap year");
    expect(html).not.toContain("USD");
  });

  it("distinguishes balanced, missing, future, and leap dates", () => {
    const html = render();
    expect(html).toContain("2024-01-02: net 0.00; money in 50.00");
    expect(html).toContain("2024-01-03: no recorded transactions");
    expect(html).toContain("2024-01-21: future date");
    const leap = render({ month: "2024-02", today: "2024-03-01" });
    expect(leap).toContain("2024-02-29: net −12.34");
    expect(leap).not.toContain("2024-02-30");
    expect(leap).toContain("This month");
  });

  it("keeps saved data visible on partial errors and gives honest empty and connection states", () => {
    const partial = render({ problem: "One bank could not finish.", refreshing: true });
    expect(partial).toContain('role="alert"');
    expect(partial).toContain("One bank could not finish.");
    expect(partial).toContain("+2,500.25");
    expect(partial).toContain("Fetching latest…");
    expect(partial).toContain("disabled");
    expect(render({ report: { ...report, days: [] } })).toContain("No posted transactions.");
    expect(render({ report: { ...report, linkedBanks: 0, days: [] } })).toContain("No connected banks.");
    expect(render({ report: undefined, loading: true })).toContain("Loading…");
    expect(render({ report: undefined, problem: "Unavailable" })).toContain("Try again");
  });

  it("navigates year boundaries and aligns calendar weeks without inventing dates", () => {
    expect(shiftMonth("2024-01", -1)).toBe("2023-12");
    expect(shiftMonth("2024-12", 1)).toBe("2025-01");
    expect(shiftMonth("0001-01", -1)).toBeNull();
    expect(shiftMonth("9999-12", 1)).toBeNull();
    expect(monthLabel("2024-02")).toBe("February 2024");
    expect(calendarWeeks("2024-02")[0]).toEqual([null, null, null, null, "2024-02-01", "2024-02-02", "2024-02-03"]);
    expect(calendarWeeks("2024-02").flat().filter(Boolean)).toHaveLength(29);
    expect(calendarWeeks("2025-02").flat().filter(Boolean)).toHaveLength(28);
    expect(calendarWeeks("2024-09")).toHaveLength(5);
    expect(calendarWeeks("2024-06")).toHaveLength(6);
    expect(spendingDate(2100, 1, 29)).toBeNull();
    expect(spendingDate(2000, 1, 29)).toBe("2000-02-29");
  });

  it("does not round large amounts or let another month or hidden future amount change the color scale", () => {
    expect(formatFlow("900719925474099301")).toBe("+9,007,199,254,740,993.01");
    expect(formatFlow("-1")).toBe("−0.01");
    expect(formatFlow("0")).toBe("0.00");
    expect(flowColor("-100", 100n)).toContain("--color-danger");
    expect(flowColor("100", 100n)).toContain("--color-teal");
    expect(flowColor("0", 0n)).toBe("var(--color-selected)");
    const future = { date: "2024-01-31", netCents: "9999999999", inflowCents: "9999999999", outflowCents: "0", transactions: 1 };
    const outside = { ...future, date: "2024-02-01" };
    expect(render({ report: { ...report, days: [...report.days, future, outside] } })).toContain(flowColor("250025", 250025n));
  });
});
