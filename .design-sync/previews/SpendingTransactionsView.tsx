import { SpendingTransactionsView } from "ui";
import * as React from "react";

// The app root (App.tsx) sets these classes; every Evie screen sits on
// them, so previews render on the same dark surface and text color.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);

const counts = { all: 133, unclassified: 26, classified: 93, pending: 0, excluded: 14 };
const page = {
  month: "2026-09", status: "all" as const, offset: 0, pageSize: 50, total: 133, hasMore: true, counts,
  transactions: [
    { id: "t1", date: "2026-09-30", name: "NOTION LABS INC. SAN FRANCISCO", merchantName: "Notion Labs Inc.", netCents: "-2400", classification: "classified" as const,
      entries: [{ category: "Subscriptions", amountCents: "2400", source: "agent" }], legacyCategory: null, legacySource: null },
    { id: "t2", date: "2026-09-29", name: "TST* ROCA KALAMAZOO MI", merchantName: "Roca", netCents: "-10800", classification: "classified" as const,
      entries: [{ category: "Dining", amountCents: "10800", source: "rule" }], legacyCategory: null, legacySource: null },
    { id: "t3", date: "2026-09-28", name: "LOWES #1234", merchantName: "Lowe's", netCents: "-17914", classification: "classified" as const,
      entries: [{ category: "Home Improvement", amountCents: "13674", source: "human" }, { category: "Shopping", amountCents: "4240", source: "human" }], legacyCategory: null, legacySource: null },
    { id: "t4", date: "2026-09-27", name: "AC HOTEL BETHESDA BETHESDA MD", merchantName: "", netCents: "-101474", classification: "unclassified" as const,
      entries: [], legacyCategory: null, legacySource: null },
    { id: "t5", date: "2026-09-26", name: "*1164 PAYMENT CITI CARD ONLINE ACH WEB", merchantName: "", netCents: "-200000", classification: "excluded" as const,
      entries: [{ category: "Credit Card Payments", amountCents: "200000", source: "rule" }], legacyCategory: null, legacySource: null },
    { id: "t6", date: "2026-09-25", name: "KROGER FUEL #612", merchantName: "Kroger Fuel", netCents: "-4270", classification: "pending" as const,
      entries: [], legacyCategory: null, legacySource: null },
  ],
};
const noop = () => {};

export const September = () => (
  <AppFrame>
    <SpendingTransactionsView month="2026-09" status="all" page={page} counts={counts} loading={false} offset={0} onStatus={noop} onPage={noop} onRetry={noop} />
  </AppFrame>
);

export const ExcludedFilter = () => (
  <AppFrame>
    <SpendingTransactionsView month="2026-09" status="excluded" counts={counts} loading={false} offset={0} onStatus={noop} onPage={noop} onRetry={noop}
    page={{ ...page, status: "excluded", total: 14, hasMore: false, transactions: page.transactions.filter((t) => t.classification === "excluded") }} />
  </AppFrame>
);

export const Loading = () => (
  <AppFrame>
    <SpendingTransactionsView month="2026-09" status="all" counts={counts} loading={true} offset={0} onStatus={noop} onPage={noop} onRetry={noop} />
  </AppFrame>
);

export const ProblemWithPreviousRecords = () => (
  <AppFrame>
    <SpendingTransactionsView month="2026-09" status="all" page={page} counts={counts} loading={false} offset={0}
    problem="Transactions could not be loaded." onStatus={noop} onPage={noop} onRetry={noop} />
  </AppFrame>
);
