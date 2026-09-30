import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { DayTransaction } from "../api/spending";
import { DayTransactionRows, SpendingDayDialog } from "./SpendingDayDialog";
import { dayLabel } from "./spendingPresentation";

const record: DayTransaction = {
  id: "txn-1", date: "2026-09-21", name: "SHOP PURCHASE #123", merchantName: "Neighborhood shop", netCents: "-12000",
  classification: "classified", revision: "snapshot", legacyCategory: null, legacySource: null,
  entries: [{ id: "1", category: "Home", amountCents: "8000", source: "rule" }, { id: "2", category: "Work", amountCents: "4000", source: "human" }],
};
const callbacks = { onSaving: () => undefined, onReload: () => undefined, onSaved: () => undefined };
function render(rows = [record], categories = ["Home", "Work", "Groceries"]) {
  return renderToStaticMarkup(<DayTransactionRows rows={rows} categories={categories} busy={false} {...callbacks} />);
}

describe("Daily transaction popup", () => {
  it("shows descriptions, exact signed totals, and separate editable split categories", () => {
    const html = render();
    for (const text of ["Neighborhood shop", "SHOP PURCHASE #123", "−120.00", "Home", "Work", "−80.00", "−40.00", "Edit category for Neighborhood shop, split 1", "Edit category for Neighborhood shop, split 2"]) expect(html).toContain(text);
    expect(html).not.toContain("Choose category");
    expect(html).not.toContain("<form");
  });

  it("retains unmatched rows, refund signs, and inconsistent saved amounts visibly", () => {
    const html = render([
      { ...record, id: "unknown", entries: [], classification: "unclassified", netCents: "200" },
      { ...record, id: "mismatch", entries: [{ id: "3", category: "Home", amountCents: "3000", source: "human" }] },
    ]);
    expect(html).toContain("Unclassified");
    expect(html).toContain("+2.00");
    expect(html).toContain("−30.00");
    expect(html).toContain("−120.00");
  });

  it("escapes bank descriptions and category labels without losing exact large amounts", () => {
    const html = render([{ ...record, name: "<script>private</script>", netCents: "900719925474099301", entries: [{ id: "9007199254740993", category: "<b>Home</b>", amountCents: "-900719925474099301", source: "human" }] }]);
    expect(html).toContain("+9,007,199,254,740,993.01");
    expect(html).toContain("&lt;script&gt;private&lt;/script&gt;");
    expect(html).toContain("&lt;b&gt;Home&lt;/b&gt;");
    expect(html).not.toContain("<script>");
  });

  it("uses a native dialog with an exact calendar date and concise loading state", () => {
    const html = renderToStaticMarkup(<SpendingDayDialog date="2024-02-29" onClose={() => undefined} onSaved={() => undefined} />);
    expect(html).toContain("<dialog");
    expect(html).toContain("aria-labelledby");
    expect(html).toContain("February 29, 2024");
    expect(html).toContain("Close day transactions");
    expect(html).toContain("Loading…");
    expect(dayLabel("0001-01-01")).toBe("January 1, 1");
  });

  it("disables editing honestly when no category choices exist", () => {
    const html = render([record], []);
    expect(html).toContain("No categories set up.");
    expect(html).toContain("disabled");
  });
});
