import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { SpendingTransactionsPage } from "../api/spending";
import { SpendingTransactionsView } from "./SpendingTransactions";

const page: SpendingTransactionsPage = {
  month: "2026-09", status: "all", offset: 0, pageSize: 50, total: 4, hasMore: false,
  counts: { all: 4, classified: 2, unclassified: 1, pending: 1 },
  transactions: [
    { id: "one", date: "2026-09-20", name: "GROCERY STORE CARD", merchantName: "Grocery store", netCents: "-1234", classification: "classified", entries: [{ category: "Groceries", amountCents: "1234", source: "rule" }], legacyCategory: null, legacySource: null },
    { id: "two", date: "2026-09-19", name: "Hardware store", merchantName: "", netCents: "-3000", classification: "classified", entries: [{ category: "Home", amountCents: "2000", source: "human" }, { category: "Work", amountCents: "1000", source: "agent" }], legacyCategory: null, legacySource: null },
    { id: "three", date: "2026-09-18", name: "Unknown shop", merchantName: "", netCents: "-1200", classification: "unclassified", entries: [], legacyCategory: "Shopping", legacySource: "manual" },
    { id: "four", date: "2026-09-17", name: "Pending shop", merchantName: "", netCents: "-700", classification: "pending", entries: [], legacyCategory: null, legacySource: null },
  ],
};
function render(overrides: Partial<Parameters<typeof SpendingTransactionsView>[0]> = {}) {
  return renderToStaticMarkup(<SpendingTransactionsView month="2026-09" status="all" page={page} counts={page.counts} loading={false} offset={0} onStatus={() => undefined} onPage={() => undefined} onRetry={() => undefined} {...overrides} />);
}

describe("Spending transactions", () => {
  it("shows bank descriptions, classification filters, and actual recorded sources without inventing model proposals", () => {
    const html = render();
    for (const expected of ["Needs classification", "Classified", "Pending", "GROCERY STORE CARD", "Grocery store", "Groceries", "Merchant rule", "Human", "Agent (legacy)", "Legacy label: Shopping", "−12.34", "1–4 of 4"]) expect(html).toContain(expected);
    expect(html).not.toContain("Proposed");
    expect(html).not.toContain("Approved");
  });
  it("preserves multiple allocations and a single bank amount per transaction", () => {
    const html = render();
    for (const expected of ["Home", "Work", "−20.00", "−10.00", "−30.00"]) expect(html).toContain(expected);
    expect(html.match(/Hardware store/g)).toHaveLength(1);
  });
  it("shows an allocation amount when a single entry differs from the bank amount", () => {
    const record = { ...page.transactions[0], netCents: "-800", entries: [{ category: "Home", amountCents: "500", source: "human" }] };
    const html = render({ page: { ...page, transactions: [record] } });
    expect(html).toContain("−8.00");
    expect(html).toContain("−5.00");
  });
  it("keeps error, loading, filtered-empty and page-empty states explicit", () => {
    expect(render({ page: undefined, loading: true })).toContain("Loading…");
    expect(render({ problem: "Try later" })).toContain("Previous records remain visible");
    expect(render({ page: { ...page, transactions: [] }, status: "unclassified" })).toContain("Nothing to classify.");
    expect(render({ page: { ...page, transactions: [] }, offset: 50 })).toContain("No transactions on this page.");
  });
});
