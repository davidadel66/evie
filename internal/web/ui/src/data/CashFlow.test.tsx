import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { CashFlowCategory, CashFlowReport } from "../api/spending";
import { CashFlowView } from "./CashFlow";
import { axisAmount, cashFlowHistoryEnd, cashFlowScale, cashFlowY, categoryKey, categoryLabel, categoryWidth, percentOf, sortedCategories } from "./cashFlowPresentation";

const zero = { inflowCents: "0", outflowCents: "0", netCents: "0", transactions: 0 };
const totals = { inflowCents: "759579", outflowCents: "532900", netCents: "226679", transactions: 10 };
const report: CashFlowReport = {
  month: "2026-09", historyEnd: "2026-09", asOfDate: "2026-09-21",
  selected: totals, pendingTransactions: 1, undatedTransactions: 0,
  history: [
    { month: "2026-06", ...totals }, { month: "2026-07", ...zero },
    { month: "2026-08", ...totals }, { month: "2026-09", ...totals },
  ],
  categories: [
    { kind: "category", category: "Paychecks", ...zero, inflowCents: "759579", netCents: "759579", transactions: 1 },
    { kind: "category", category: "Shopping", ...zero, outflowCents: "300000", netCents: "-300000", transactions: 7 },
    { kind: "unclassified", category: null, ...zero, outflowCents: "200000", netCents: "-200000", transactions: 1 },
    { kind: "unreconciled", category: null, ...zero, outflowCents: "32900", netCents: "-32900", transactions: 1 },
  ],
};
function render(overrides: Partial<Parameters<typeof CashFlowView>[0]> = {}) {
  return renderToStaticMarkup(<CashFlowView month={report.month} today={report.asOfDate} report={report} history={report.history} loading={false} onMonth={() => undefined} onRetry={() => undefined} {...overrides} />);
}

describe("Monthly cash flow", () => {
  it("renders raw totals and category shares with no inferred income or savings", () => {
    const html = render();
    for (const label of ["Inflow", "Outflow", "Net flow", "Net rate", "7,595.79", "5,329.00", "+2,266.79", "29.8%", "Paychecks", "Shopping", "Unclassified", "Needs review", "56.3%", "37.5%", "6.2%"]) expect(html).toContain(label);
    expect(html).not.toMatch(/Income|Savings|\$/);
    expect(html).toContain("Inflow by category");
    expect(html).toContain("Outflow by category");
  });

  it("exposes keyboard month buttons, current month-to-date amounts and gaps for absent months", () => {
    const html = render();
    expect(html.match(/aria-pressed=/g)).toHaveLength(4);
    expect(html).toContain('aria-pressed="true"');
    expect(html).toContain("September 2026 to date: inflow 7,595.79");
    expect(html).toContain("July 2026: no recorded transactions");
    expect(html.match(/<path /g)).toHaveLength(1);
    expect(html).toContain('stroke-dasharray="4 5"');
    expect(render({ history: [{ ...zero, month: "2026-10" }] })).toContain("October 2026: future month");
  });

  it("distinguishes no records, balanced flow and unavailable data without fabricating a rate", () => {
    const empty = render({ report: { ...report, selected: zero, categories: [] } });
    expect(empty.match(/<dd[^>]*>—<\/dd>/g)).toHaveLength(4);
    expect(empty).toContain("No inflow.");
    expect(empty).toContain("No outflow.");
    const balanced = render({ report: { ...report, selected: { ...zero, inflowCents: "100", outflowCents: "100", transactions: 2 } } });
    expect(balanced).toContain(">0.00</dd>");
    expect(balanced).toContain(">0.0%</dd>");
    const loading = render({ report: undefined, history: [], loading: true });
    expect(loading).toContain('role="status"');
    expect(loading).not.toContain("by category");
    const error = render({ problem: "Cash flow could not be loaded." });
    expect(error).toContain('role="alert"');
    expect(error).toContain("Retry");
    expect(error).toContain("7,595.79");
  });

  it("preserves very large amounts and negative net flow", () => {
    const html = render({ report: { ...report, selected: { inflowCents: "900719925474099301", outflowCents: "1801439850948198602", netCents: "-900719925474099301", transactions: 3 } } });
    expect(html).toContain("9,007,199,254,740,993.01");
    expect(html).toContain("18,014,398,509,481,986.02");
    expect(html).toContain("−9,007,199,254,740,993.01");
    expect(html).toContain("−100.0%");
  });
});

describe("Cash-flow presentation boundaries", () => {
  it("keeps the history window stable until the selected month leaves it", () => {
    expect(cashFlowHistoryEnd("2026-08", "2026-09")).toBe("2026-09");
    expect(cashFlowHistoryEnd("2025-10", "2026-09")).toBe("2026-09");
    expect(cashFlowHistoryEnd("2025-09", "2026-09")).toBe("2026-08");
    expect(cashFlowHistoryEnd("2026-10", "2026-09")).toBe("2026-10");
    expect(cashFlowHistoryEnd("0001-01", "0001-02")).toBe("0001-02");
    expect(cashFlowHistoryEnd("9999-12", "9999-11")).toBe("9999-12");
  });

  it("computes percentages from integer cents, including zero inflow and losses", () => {
    expect(percentOf("226679", "759579")).toBe("29.8%");
    expect(percentOf("1", "16")).toBe("6.3%");
    expect(percentOf("-5", "2")).toBe("−250.0%");
    expect(percentOf("0", "0")).toBe("—");
    expect(percentOf("-100", "0")).toBe("—");
    expect(percentOf("900719925474099301", "1801439850948198602")).toBe("50.0%");
    expect(categoryWidth("900719925474099301", "1801439850948198602")).toBe(50);
    expect(categoryWidth("1", "0")).toBe(0);
  });

  it("sorts only the requested flow side exactly and preserves synthetic bucket identity", () => {
    const named: CashFlowCategory = { kind: "category", category: "Unclassified", ...zero, outflowCents: "900719925474099301" };
    const synthetic: CashFlowCategory = { kind: "unclassified", category: null, ...zero, outflowCents: "900719925474099302" };
    expect(categoryKey(named)).not.toBe(categoryKey(synthetic));
    expect(categoryLabel(named)).toBe("Unclassified (category)");
    expect(categoryLabel(synthetic)).toBe("Unclassified");
    expect(categoryLabel({ ...named, category: "Needs review" })).toBe("Needs review (category)");
    expect(sortedCategories([named, report.categories[0], synthetic], "outflowCents")).toEqual([synthetic, named]);
    expect(sortedCategories(report.categories, "inflowCents")).toEqual([report.categories[0]]);
  });

  it("has finite symmetric chart scales for empty, tiny and huge values", () => {
    expect(cashFlowScale([])).toBe(100n);
    const tiny = cashFlowScale([{ ...zero, month: "2026-09", outflowCents: "1" }]);
    expect(tiny).toBe(2n);
    const scale = cashFlowScale([{ ...zero, month: "2026-09", outflowCents: "900719925474099301" }]);
    expect(scale).toBeGreaterThan(900719925474099301n);
    expect(cashFlowY("0", scale)).toBe(124);
    expect(cashFlowY(scale.toString(), scale)).toBe(24);
    expect(cashFlowY((-scale).toString(), scale)).toBe(224);
    expect(axisAmount(250000n)).toBe("2.5K");
    expect(axisAmount(-100000000n)).toBe("−1M");
    expect(axisAmount(50n)).toBe("0.50");
  });
});
