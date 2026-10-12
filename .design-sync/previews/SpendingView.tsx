import { SpendingView } from "ui";
import * as React from "react";

// The app root (App.tsx) sets these classes; every Evie screen sits on them.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);

// September 2026 through Saturday the 26th: two paychecks in (8,068.00) and
// 8,123.00 out, so the month is running a small deficit. The 1st carries the
// mortgage, the 15th the auto loan; the 7th (Labor Day) has no postings.
const days = [
  { date: "2026-09-01", inflowCents: "0", outflowCents: "245525", netCents: "-245525", transactions: 2 },
  { date: "2026-09-02", inflowCents: "0", outflowCents: "14312", netCents: "-14312", transactions: 1 },
  { date: "2026-09-03", inflowCents: "0", outflowCents: "3010", netCents: "-3010", transactions: 2 },
  { date: "2026-09-04", inflowCents: "403400", outflowCents: "10800", netCents: "392600", transactions: 2 },
  { date: "2026-09-05", inflowCents: "0", outflowCents: "10398", netCents: "-10398", transactions: 2 },
  { date: "2026-09-06", inflowCents: "0", outflowCents: "6450", netCents: "-6450", transactions: 1 },
  { date: "2026-09-08", inflowCents: "0", outflowCents: "13794", netCents: "-13794", transactions: 3 },
  { date: "2026-09-09", inflowCents: "0", outflowCents: "12205", netCents: "-12205", transactions: 1 },
  { date: "2026-09-10", inflowCents: "0", outflowCents: "2209", netCents: "-2209", transactions: 2 },
  { date: "2026-09-11", inflowCents: "0", outflowCents: "4325", netCents: "-4325", transactions: 1 },
  { date: "2026-09-12", inflowCents: "0", outflowCents: "78640", netCents: "-78640", transactions: 2 },
  { date: "2026-09-13", inflowCents: "0", outflowCents: "24631", netCents: "-24631", transactions: 2 },
  { date: "2026-09-14", inflowCents: "0", outflowCents: "525", netCents: "-525", transactions: 1 },
  { date: "2026-09-15", inflowCents: "0", outflowCents: "123250", netCents: "-123250", transactions: 2 },
  { date: "2026-09-16", inflowCents: "0", outflowCents: "11088", netCents: "-11088", transactions: 1 },
  { date: "2026-09-17", inflowCents: "0", outflowCents: "1609", netCents: "-1609", transactions: 2 },
  { date: "2026-09-18", inflowCents: "403400", outflowCents: "17914", netCents: "385486", transactions: 2 },
  { date: "2026-09-19", inflowCents: "0", outflowCents: "12640", netCents: "-12640", transactions: 1 },
  { date: "2026-09-20", inflowCents: "0", outflowCents: "59797", netCents: "-59797", transactions: 2 },
  { date: "2026-09-21", inflowCents: "0", outflowCents: "525", netCents: "-525", transactions: 1 },
  { date: "2026-09-22", inflowCents: "0", outflowCents: "54100", netCents: "-54100", transactions: 2 },
  { date: "2026-09-23", inflowCents: "0", outflowCents: "10655", netCents: "-10655", transactions: 1 },
  { date: "2026-09-24", inflowCents: "0", outflowCents: "2610", netCents: "-2610", transactions: 2 },
  { date: "2026-09-25", inflowCents: "0", outflowCents: "14120", netCents: "-14120", transactions: 2 },
  { date: "2026-09-26", inflowCents: "0", outflowCents: "77168", netCents: "-77168", transactions: 2 },
];

const report = {
  year: 2026, years: [2026, 2025], linkedBanks: 2, days,
  pendingTransactions: 1, undatedTransactions: 0, currency: "USD", refreshedAt: "2026-09-26T13:05:00Z",
};
const today = "2026-09-26";

// SpendingView composes two data-fetching containers, SpendingAccounts and
// CashFlow, which POST to /api/data/spending/* on mount. There is no server
// behind the card, so this preview answers those two calls itself with data
// that matches the calendar above. The capture renders one story per page
// (?story=<Export>), so the stub keys the empty / disconnected / loading
// stories off that parameter; every other request passes through untouched.
const accounts = {
  linkAvailable: true,
  institutions: [
    { id: "ins_chase", name: "Chase", accountsKnown: true, accountsUpdatedAt: "2026-09-26T13:05:00Z",
      accounts: [{ id: "acc_1", name: "Total Checking", mask: "4821", type: "depository", subtype: "checking" }, { id: "acc_2", name: "Savings", mask: "0937", type: "depository", subtype: "savings" }] },
    { id: "ins_citi", name: "Citi", accountsKnown: true, accountsUpdatedAt: "2026-09-26T13:05:00Z",
      accounts: [{ id: "acc_3", name: "Double Cash", mask: "1164", type: "credit", subtype: "credit card" }] },
  ],
};
const monthly: Record<string, [number, number, number]> = {
  "2025-11": [806800, 745210, 112], "2025-12": [806800, 912430, 141], "2026-01": [825600, 701220, 104],
  "2026-02": [825600, 688900, 97], "2026-03": [825600, 734150, 118], "2026-04": [1238400, 709880, 109],
  "2026-05": [825600, 791330, 123], "2026-06": [825600, 842670, 131], "2026-07": [825600, 758420, 115],
  "2026-08": [806800, 621000, 108], "2026-09": [806800, 812300, 119],
};
const totalsFor = (month: string) => {
  const [inflow, outflow, transactions] = monthly[month] ?? [0, 0, 0];
  return { inflowCents: String(inflow), outflowCents: String(outflow), netCents: String(inflow - outflow), transactions };
};
const shift = (month: string, offset: number) => {
  const [year, number] = month.split("-").map(Number);
  const index = year * 12 + number - 1 + offset;
  return `${Math.floor(index / 12)}-${String((index % 12) + 1).padStart(2, "0")}`;
};
const category = (kind: string, name: string | null, inflow: number, outflow: number, transactions: number) =>
  ({ kind, category: name, inflowCents: String(inflow), outflowCents: String(outflow), netCents: String(inflow - outflow), transactions });
const septemberCategories = [
  category("category", "Paychecks", 800000, 0, 2), category("category", "Refunds", 6800, 0, 1),
  category("category", "Mortgage", 0, 245000, 1), category("category", "Auto Loan", 0, 120000, 1),
  category("category", "Groceries", 0, 98571, 9), category("category", "Shopping", 0, 64234, 6),
  category("category", "Dining", 0, 61615, 11), category("category", "Insurance", 0, 60000, 1),
  category("category", "Health", 0, 48700, 2), category("category", "Utilities", 0, 18640, 1),
  category("category", "Internet", 0, 8999, 1), category("category", "Gas", 0, 8540, 2),
  category("category", "Subscriptions", 0, 6998, 4), category("category", "Coffee", 0, 5355, 9),
  category("unclassified", null, 0, 32900, 1), category("unreconciled", null, 0, 32748, 2),
];
const cashFlowFor = (query: { month: string; historyEnd: string; asOfDate: string }, empty: boolean) => {
  const history = Array.from({ length: 12 }, (_, index) => shift(query.historyEnd, index - 11))
    .map((month) => ({ month, ...(empty && month === query.month ? totalsFor("none") : totalsFor(month)) }));
  return { ...query, history, selected: empty ? totalsFor("none") : totalsFor(query.month),
    categories: empty ? [] : query.month === "2026-09" ? septemberCategories : septemberCategories.slice(0, 1).concat(septemberCategories.slice(2, 10)),
    pendingTransactions: 1, undatedTransactions: 0 };
};
const browser = typeof window === "undefined" ? undefined : (window as any);
if (browser && !browser.__eviePreviewFetch) {
  const realFetch = browser.fetch.bind(browser);
  browser.__eviePreviewFetch = true;
  browser.fetch = (input: any, init?: any) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.href : input?.url ?? "";
    const story = new URLSearchParams(browser.location.search).get("story") ?? "";
    const json = (body: unknown) => Promise.resolve(new Response(JSON.stringify(body), { status: 200, headers: { "Content-Type": "application/json" } }));
    if (story === "Loading" && /\/api\/data\/spending\/(accounts|cash-flow)$/.test(url)) return new Promise<Response>(() => {});
    if (url.endsWith("/api/data/spending/accounts")) return json(story === "NoConnectedBanks" ? { linkAvailable: true, institutions: [] } : accounts);
    if (url.endsWith("/api/data/spending/cash-flow")) {
      let query = { month: "2026-09", historyEnd: "2026-09", asOfDate: today };
      try { query = { ...query, ...JSON.parse(String(init?.body ?? "{}")) }; } catch { /* keep defaults */ }
      return json(cashFlowFor(query, story === "EmptyMonth"));
    }
    return realFetch(input, init);
  };
}
const noop = () => {};
const callbacks = { onMonth: noop, onRefresh: noop, onRetry: noop, onSaved: noop };

export const September = () => (
  <AppFrame>
    <SpendingView month="2026-09" report={report} loading={false} refreshing={false} today={today} {...callbacks} />
  </AppFrame>
);

export const Refreshing = () => (
  <AppFrame>
    <SpendingView month="2026-09" report={report} loading={true} refreshing={true} today={today} {...callbacks} />
  </AppFrame>
);

export const PartialRefreshProblem = () => (
  <AppFrame>
    <SpendingView month="2026-09" report={report} loading={false} refreshing={false} today={today}
      problem="1 bank refreshed; 1 failed." {...callbacks} />
  </AppFrame>
);

export const EmptyMonth = () => (
  <AppFrame>
    <SpendingView month="2026-09" report={{ ...report, days: [] }} loading={false} refreshing={false} today={today} {...callbacks} />
  </AppFrame>
);

export const Loading = () => (
  <AppFrame>
    <SpendingView month="2026-09" loading={true} refreshing={false} today={today} {...callbacks} />
  </AppFrame>
);

export const NoConnectedBanks = () => (
  <AppFrame>
    <SpendingView month="2026-09" report={{ ...report, linkedBanks: 0, days: [], refreshedAt: null }} loading={false} refreshing={false} today={today} {...callbacks} />
  </AppFrame>
);
