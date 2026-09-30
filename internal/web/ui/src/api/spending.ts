export type SpendingDay = {
  date: string;
  inflowCents: string;
  outflowCents: string;
  netCents: string;
  transactions: number;
};

export type SpendingReport = {
  year: number;
  years: number[];
  linkedBanks: number;
  days: SpendingDay[];
  pendingTransactions: number;
  undatedTransactions: number;
  currency: string | null;
  refreshedAt: string | null;
};

export type SpendingRefresh = {
  added: number;
  modified: number;
  removed: number;
  banksSucceeded: number;
  banksFailed: number;
  refreshedAt: string | null;
};

export type SpendingStatus = "all" | "unclassified" | "classified" | "pending";
export type SpendingEntry = { category: string; amountCents: string; source: string };
export type SpendingTransaction = {
  id: string; date: string; name: string; merchantName: string; netCents: string;
  classification: Exclude<SpendingStatus, "all">;
  entries: SpendingEntry[];
  legacyCategory: string | null; legacySource: string | null;
};
export type SpendingTransactionsQuery = { month: string; status: SpendingStatus; offset: number; pageSize: number };
export type SpendingTransactionsPage = SpendingTransactionsQuery & {
  total: number; hasMore: boolean;
  counts: { all: number; unclassified: number; classified: number; pending: number };
  transactions: SpendingTransaction[];
};

export type CashFlowTotals = {
  inflowCents: string; outflowCents: string; netCents: string; transactions: number;
};
export type CashFlowMonth = CashFlowTotals & { month: string };
export type CashFlowCategory = CashFlowTotals & {
  kind: "category" | "unclassified" | "unreconciled"; category: string | null;
};
export type CashFlowQuery = { month: string; historyEnd: string; asOfDate: string };
export type CashFlowReport = CashFlowQuery & {
  history: CashFlowMonth[]; selected: CashFlowTotals; categories: CashFlowCategory[];
  pendingTransactions: number; undatedTransactions: number;
};

export type DayTransaction = Omit<SpendingTransaction, "entries"> & {
  revision: string;
  entries: (SpendingEntry & { id: string })[];
};
export type SpendingDayQuery = { date: string; offset: number; pageSize: number };
export type SpendingDayPage = SpendingDayQuery & {
  total: number; hasMore: boolean; categories: string[];
  summary: SpendingDay; transactions: DayTransaction[];
};
export type SpendingCategoryUpdate = {
  transactionId: string; entryId: string | null; category: string; revision: string;
};

export class SpendingCategoryError extends Error {
  readonly conflict: boolean;
  constructor(message: string, conflict: boolean) { super(message); this.conflict = conflict; }
}

async function spendingRequest<T>(action: "summary" | "refresh" | "transactions" | "cash-flow" | "day" | "category", body: object, signal?: AbortSignal): Promise<T> {
  const response = await fetch(`/api/data/spending/${action}`, {
    method: "POST", headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body), signal,
  });
  if (!response.ok) {
    if (action === "category") {
      if (response.status === 409) throw new SpendingCategoryError("This transaction changed. Reload before editing.", true);
      if (response.status === 400) throw new SpendingCategoryError("Choose an available category and try again.", false);
      throw new SpendingCategoryError("Category could not be saved. Reload to check its current value.", false);
    }
    if (response.status === 409) throw new Error("A bank refresh is already running.");
    if (action === "refresh") throw new Error("Banks could not be refreshed.");
    if (action === "transactions") throw new Error("Transactions could not be loaded.");
    if (action === "cash-flow") throw new Error("Cash flow could not be loaded.");
    if (action === "day") throw new Error("Day transactions could not be loaded.");
    throw new Error("Spending could not be loaded.");
  }
  return response.json();
}

export function inspectSpending(year: number, signal?: AbortSignal): Promise<SpendingReport> {
  return spendingRequest<SpendingReport>("summary", { year }, signal);
}

export function refreshSpending(signal?: AbortSignal): Promise<SpendingRefresh> {
  return spendingRequest<SpendingRefresh>("refresh", {}, signal);
}

export function inspectSpendingTransactions(query: SpendingTransactionsQuery, signal?: AbortSignal): Promise<SpendingTransactionsPage> {
  return spendingRequest<SpendingTransactionsPage>("transactions", query, signal);
}

export function inspectCashFlow(query: CashFlowQuery, signal?: AbortSignal): Promise<CashFlowReport> {
  return spendingRequest<CashFlowReport>("cash-flow", query, signal);
}

export function inspectSpendingDay(query: SpendingDayQuery, signal?: AbortSignal): Promise<SpendingDayPage> {
  return spendingRequest<SpendingDayPage>("day", query, signal);
}

export function updateSpendingCategory(query: SpendingCategoryUpdate): Promise<DayTransaction> {
  return spendingRequest<DayTransaction>("category", query);
}
