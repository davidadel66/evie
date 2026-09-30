import { afterEach, describe, expect, it, vi } from "vitest";
import { inspectCashFlow, inspectSpending, inspectSpendingDay, inspectSpendingTransactions, refreshSpending, SpendingCategoryError, updateSpendingCategory } from "./spending";

afterEach(() => vi.unstubAllGlobals());

describe("Spending API", () => {
  it("keeps reading saved totals separate from an explicit bank refresh", async () => {
    const fetchMock = vi.fn(async () => new Response("{}"));
    vi.stubGlobal("fetch", fetchMock);
    const controller = new AbortController();
    await inspectSpending(2024, controller.signal);
    await refreshSpending(controller.signal);
    expect(fetchMock).toHaveBeenNthCalledWith(1, "/api/data/spending/summary", expect.objectContaining({ method: "POST", body: '{"year":2024}', signal: controller.signal }));
    expect(fetchMock).toHaveBeenNthCalledWith(2, "/api/data/spending/refresh", expect.objectContaining({ method: "POST", body: "{}", signal: controller.signal }));
  });

  it("preserves partial results and exact cents", async () => {
    const result = { banksSucceeded: 1, banksFailed: 1, added: 3, modified: 1, removed: 0, refreshedAt: null };
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify(result))));
    expect(await refreshSpending()).toEqual(result);
    const report = { days: [{ netCents: "900719925474099301" }] };
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify(report))));
    expect(await inspectSpending(2024)).toEqual(report);
  });

  it("reports overlap and never exposes raw provider errors", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("secret-access-token", { status: 409 })));
    await expect(refreshSpending()).rejects.toThrow("already running");
    vi.stubGlobal("fetch", vi.fn(async () => new Response("secret-access-token", { status: 500 })));
    await expect(refreshSpending()).rejects.toThrow("Banks could not be refreshed");
    await expect(inspectSpending(2024)).rejects.toThrow("Spending could not be loaded");
    await expect(inspectSpendingTransactions({ month: "2026-09", status: "all", offset: 0, pageSize: 50 })).rejects.toThrow("Transactions could not be loaded");
    await expect(inspectCashFlow({ month: "2026-09", historyEnd: "2026-09", asOfDate: "2026-09-21" })).rejects.toThrow("Cash flow could not be loaded");
  });

  it("posts the exact selected month, filter and page with cancellation", async () => {
    const fetchMock = vi.fn(async () => new Response("{}"));
    vi.stubGlobal("fetch", fetchMock);
    const controller = new AbortController();
    const query = { month: "2026-09", status: "unclassified" as const, offset: 50, pageSize: 50 };
    await inspectSpendingTransactions(query, controller.signal);
    expect(fetchMock).toHaveBeenCalledWith("/api/data/spending/transactions", expect.objectContaining({ method: "POST", body: JSON.stringify(query), signal: controller.signal }));
  });

  it("reads monthly cash flow with a stable window, local date cutoff and cancellation", async () => {
    const report = { selected: { netCents: "900719925474099301" } };
    const fetchMock = vi.fn(async () => new Response(JSON.stringify(report)));
    vi.stubGlobal("fetch", fetchMock);
    const controller = new AbortController();
    const query = { month: "2026-08", historyEnd: "2026-09", asOfDate: "2026-09-21" };
    expect(await inspectCashFlow(query, controller.signal)).toEqual(report);
    expect(fetchMock).toHaveBeenCalledWith("/api/data/spending/cash-flow", expect.objectContaining({ method: "POST", body: JSON.stringify(query), signal: controller.signal }));
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("loads an exact day and sends explicit category edits with opaque revisions and exact entry ids", async () => {
    const fetchMock = vi.fn(async () => new Response("{}"));
    vi.stubGlobal("fetch", fetchMock);
    const controller = new AbortController();
    const query = { date: "2026-09-21", offset: 50, pageSize: 50 };
    await inspectSpendingDay(query, controller.signal);
    const change = { transactionId: "transaction", entryId: "900719925474099301", category: "Groceries", revision: "saved-state-hash" };
    await updateSpendingCategory(change);
    expect(fetchMock).toHaveBeenNthCalledWith(1, "/api/data/spending/day", expect.objectContaining({ method: "POST", body: JSON.stringify(query), signal: controller.signal }));
    expect(fetchMock).toHaveBeenNthCalledWith(2, "/api/data/spending/category", expect.objectContaining({ method: "POST", body: JSON.stringify(change) }));
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("distinguishes stale category writes from a bank refresh and hides private errors", async () => {
    const query = { transactionId: "transaction", entryId: null, category: "Groceries", revision: "saved-state-hash" };
    vi.stubGlobal("fetch", vi.fn(async () => new Response("private-token", { status: 409 })));
    await expect(updateSpendingCategory(query)).rejects.toMatchObject({ conflict: true, message: "This transaction changed. Reload before editing." });
    vi.stubGlobal("fetch", vi.fn(async () => new Response("private-token", { status: 400 })));
    await expect(updateSpendingCategory(query)).rejects.toBeInstanceOf(SpendingCategoryError);
    await expect(updateSpendingCategory(query)).rejects.toThrow("Choose an available category");
    vi.stubGlobal("fetch", vi.fn(async () => new Response("private-token", { status: 500 })));
    await expect(updateSpendingCategory(query)).rejects.toThrow("Category could not be saved");
    await expect(inspectSpendingDay({ date: "2026-09-21", offset: 0, pageSize: 50 })).rejects.toThrow("Day transactions could not be loaded");
  });
});
