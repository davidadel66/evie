import { CashFlowView } from "ui";
import * as React from "react";

// The app root (App.tsx) sets these classes; every Evie screen sits on them.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);

const noop = () => {};
const today = "2026-09-26";

// Twelve months ending September 2026. The first bank was linked in November
// 2025, so October 2025 has no records; April carried a third paycheck.
const flow = (month: string, inflow: number, outflow: number, transactions: number) => ({
  month, inflowCents: String(inflow), outflowCents: String(outflow), netCents: String(inflow - outflow), transactions,
});
const history = [
  flow("2025-10", 0, 0, 0),
  flow("2025-11", 806800, 745210, 112),
  flow("2025-12", 806800, 912430, 141),
  flow("2026-01", 825600, 701220, 104),
  flow("2026-02", 825600, 688900, 97),
  flow("2026-03", 825600, 734150, 118),
  flow("2026-04", 1238400, 709880, 109),
  flow("2026-05", 825600, 791330, 123),
  flow("2026-06", 825600, 842670, 131),
  flow("2026-07", 825600, 758420, 115),
  flow("2026-08", 806800, 621000, 108),
  flow("2026-09", 806800, 812300, 119),
];

type Row = { kind: "category" | "unclassified" | "unreconciled"; category: string | null; inflow?: number; outflow?: number; transactions: number };
const rows = (list: Row[]) => list.map(({ kind, category, inflow = 0, outflow = 0, transactions }) => ({
  kind, category, inflowCents: String(inflow), outflowCents: String(outflow), netCents: String(inflow - outflow), transactions,
}));

// September: 8,068.00 in, 8,123.00 out. Outflow has more than six categories,
// so the "Show all" control appears; a hotel stay is still unclassified.
const september = {
  month: "2026-09", historyEnd: "2026-09", asOfDate: today, history,
  selected: history[11], pendingTransactions: 1, undatedTransactions: 0,
  categories: rows([
    { kind: "category", category: "Paychecks", inflow: 800000, transactions: 2 },
    { kind: "category", category: "Refunds", inflow: 6800, transactions: 1 },
    { kind: "category", category: "Mortgage", outflow: 245000, transactions: 1 },
    { kind: "category", category: "Auto Loan", outflow: 120000, transactions: 1 },
    { kind: "category", category: "Groceries", outflow: 98571, transactions: 9 },
    { kind: "category", category: "Shopping", outflow: 64234, transactions: 6 },
    { kind: "category", category: "Dining", outflow: 61615, transactions: 11 },
    { kind: "category", category: "Insurance", outflow: 60000, transactions: 1 },
    { kind: "category", category: "Health", outflow: 48700, transactions: 2 },
    { kind: "category", category: "Utilities", outflow: 18640, transactions: 1 },
    { kind: "category", category: "Internet", outflow: 8999, transactions: 1 },
    { kind: "category", category: "Gas", outflow: 8540, transactions: 2 },
    { kind: "category", category: "Subscriptions", outflow: 6998, transactions: 4 },
    { kind: "category", category: "Coffee", outflow: 5355, transactions: 9 },
    { kind: "unclassified", category: null, outflow: 32900, transactions: 1 },
    { kind: "unreconciled", category: null, outflow: 32748, transactions: 2 },
  ]),
};

// August closed with a surplus and no unclassified spending.
const august = {
  ...september, month: "2026-08", selected: history[10],
  categories: rows([
    { kind: "category", category: "Paychecks", inflow: 806800, transactions: 2 },
    { kind: "category", category: "Mortgage", outflow: 245000, transactions: 1 },
    { kind: "category", category: "Auto Loan", outflow: 120000, transactions: 1 },
    { kind: "category", category: "Groceries", outflow: 87410, transactions: 8 },
    { kind: "category", category: "Dining", outflow: 52330, transactions: 9 },
    { kind: "category", category: "Insurance", outflow: 60000, transactions: 1 },
    { kind: "category", category: "Utilities", outflow: 21455, transactions: 1 },
    { kind: "category", category: "Subscriptions", outflow: 6998, transactions: 4 },
    { kind: "category", category: "Gas", outflow: 12680, transactions: 3 },
    { kind: "category", category: "Coffee", outflow: 6115, transactions: 10 },
    { kind: "category", category: "Internet", outflow: 8999, transactions: 1 },
    { kind: "unreconciled", category: null, outflow: 13, transactions: 1 },
  ]),
};

export const September2026 = () => (
  <AppFrame>
    <CashFlowView month="2026-09" today={today} report={september as any} history={history} loading={false} onMonth={noop} onRetry={noop} />
  </AppFrame>
);

export const AugustSurplus = () => (
  <AppFrame>
    <CashFlowView month="2026-08" today={today} report={august as any} history={history} loading={false} onMonth={noop} onRetry={noop} />
  </AppFrame>
);

export const MonthWithoutRecords = () => (
  <AppFrame>
    <CashFlowView month="2025-10" today={today} history={history} loading={false} onMonth={noop} onRetry={noop}
      report={{ ...september, month: "2025-10", selected: history[0], categories: [] } as any} />
  </AppFrame>
);

export const Loading = () => (
  <AppFrame>
    <CashFlowView month="2026-09" today={today} history={[]} loading={true} onMonth={noop} onRetry={noop} />
  </AppFrame>
);

export const ProblemWithPreviousRecords = () => (
  <AppFrame>
    <CashFlowView month="2026-09" today={today} report={september as any} history={history} loading={false}
      problem="Cash flow could not be loaded." onMonth={noop} onRetry={noop} />
  </AppFrame>
);
