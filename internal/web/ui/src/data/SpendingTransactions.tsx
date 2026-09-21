import { useEffect, useState } from "react";
import {
  inspectSpendingTransactions, type SpendingStatus, type SpendingTransaction,
  type SpendingTransactionsPage,
} from "../api/spending";
import { formatFlow, monthLabel } from "./spendingPresentation";

const filters: { value: SpendingStatus; label: string }[] = [
  { value: "all", label: "All" },
  { value: "unclassified", label: "Needs classification" },
  { value: "classified", label: "Classified" },
  { value: "pending", label: "Pending" },
];
const control = "border-hair-strong bg-card text-body focus-visible:ring-teal rounded-md border px-3 py-2 text-xs focus-visible:ring-2 focus-visible:outline-none disabled:opacity-40";

export function SpendingTransactions({ month, revision }: { month: string; revision: number }) {
  const [status, setStatus] = useState<SpendingStatus>("all");
  const [pagination, setPagination] = useState({ month, offset: 0 });
  const [page, setPage] = useState<SpendingTransactionsPage>();
  const [loading, setLoading] = useState(true);
  const [problem, setProblem] = useState("");
  const [retry, setRetry] = useState(0);
  const offset = pagination.month === month ? pagination.offset : 0;
  const pageSize = 50;
  useEffect(() => { setPagination({ month, offset: 0 }); }, [month]);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setProblem("");
    inspectSpendingTransactions({ month, status, offset, pageSize }, controller.signal).then((next) => {
      if (!controller.signal.aborted) setPage(next);
    }).catch((error: unknown) => {
      if (!controller.signal.aborted) setProblem(error instanceof Error ? error.message : "Transactions could not be loaded.");
    }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [month, status, offset, revision, retry]);
  const matching = page?.month === month && page.status === status && page.offset === offset ? page : undefined;
  return <SpendingTransactionsView month={month} status={status} page={matching}
    counts={page?.month === month ? page.counts : undefined} loading={loading} problem={problem} offset={offset}
    onStatus={(next) => { setStatus(next); setPagination({ month, offset: 0 }); }}
    onPage={(next) => setPagination({ month, offset: next })} onRetry={() => setRetry((value) => value + 1)} />;
}

type Props = {
  month: string; status: SpendingStatus; page?: SpendingTransactionsPage;
  counts?: SpendingTransactionsPage["counts"]; loading: boolean; problem?: string; offset: number;
  onStatus: (status: SpendingStatus) => void; onPage: (offset: number) => void; onRetry: () => void;
};

export function SpendingTransactionsView({ month, status, page, counts, loading, problem, offset, onStatus, onPage, onRetry }: Props) {
  const records = page?.transactions ?? [];
  return <section aria-label="Monthly transactions" className="mt-5 min-w-0">
    <div role="group" aria-label="Filter transactions" className="flex flex-wrap gap-2">
      {filters.map((filter) => <button type="button" key={filter.value} aria-pressed={filter.value === status} onClick={() => onStatus(filter.value)} className={`${control} ${filter.value === status ? "border-teal-hair bg-teal-deep text-teal-hover" : "hover:bg-hover"}`}>
        {filter.label}{" "}<span className="ml-2 tabular-nums">{counts ? counts[filter.value].toLocaleString() : "—"}</span>
      </button>)}
      <button type="button" className={`${control} ml-auto`} aria-label="Reload saved records" onClick={onRetry} disabled={loading}>Reload</button>
    </div>
    {problem && <div role="alert" className="text-amber-ink border-amber-hair bg-amber-bg mt-4 rounded-md border p-3 text-xs">{problem}{page && " Previous records remain visible."}<button type="button" className="ml-3 underline underline-offset-2" onClick={onRetry}>Try again</button></div>}
    {loading && <p role="status" className="text-muted-text mt-4 text-xs">Loading…</p>}
    {page && records.length === 0 && <p role="status" className="text-muted-text py-12 text-center text-sm">{offset > 0 ? "No transactions on this page." : status === "unclassified" ? "Nothing to classify." : status === "classified" ? "No classified transactions." : status === "pending" ? "No pending transactions." : "No transactions."}</p>}
    {records.length > 0 && <div className="mt-4 max-w-full overflow-x-auto" tabIndex={0} role="region" aria-label="Transaction records; scroll horizontally on small screens">
      <table className="w-full min-w-[760px] text-left text-xs">
        <caption className="sr-only">{monthLabel(month)} transactions. Positive net amounts are money in; negative amounts are money out.</caption>
        <thead className="border-hair text-muted-text border-b text-[11px]"><tr>{["Date", "Transaction", "Classification", "Source", "Net amount"].map((label) => <th scope="col" key={label} className={`px-3 py-3 font-normal ${label === "Net amount" ? "text-right" : ""}`}>{label}</th>)}</tr></thead>
        <tbody>{records.map((record) => <tr key={record.id} className="border-hair border-b align-top">
          <td className="text-muted-text whitespace-nowrap px-3 py-4 tabular-nums">{record.date}</td>
          <td className="max-w-72 px-3 py-4"><p className="text-body break-words font-medium">{record.merchantName || record.name || "Unnamed transaction"}</p>{record.merchantName && record.name && record.merchantName !== record.name && <p className="text-muted-text mt-1 break-words text-[11px]">{record.name}</p>}</td>
          <td className="min-w-44 px-3 py-4"><Classification record={record} /></td>
          <td className="text-muted-text px-3 py-4">{record.entries.length ? record.entries.map((entry, index) => <p key={index} className="mb-1 whitespace-nowrap">{sourceLabel(entry.source)}</p>) : record.legacySource ? <span>Legacy: {record.legacySource}</span> : "—"}</td>
          <td className={`${BigInt(record.netCents) > 0n ? "text-teal-hover" : "text-body"} whitespace-nowrap px-3 py-4 text-right tabular-nums`}>{formatFlow(record.netCents)}</td>
        </tr>)}</tbody>
      </table>
    </div>}
    {page && <div className="text-muted-text mt-4 flex flex-wrap items-center justify-between gap-3 text-xs">
      <p>{records.length ? `${offset + 1}–${offset + records.length} of ${page.total.toLocaleString()}` : `${page.total.toLocaleString()} matching transactions`}</p>
      <div className="flex gap-2"><button type="button" className={control} onClick={() => onPage(Math.max(0, offset - page.pageSize))} disabled={loading || offset === 0}>Previous page</button><button type="button" className={control} onClick={() => onPage(offset + page.pageSize)} disabled={loading || !page.hasMore}>Next page</button></div>
    </div>}
  </section>;
}

function Classification({ record }: { record: SpendingTransaction }) {
  return <>
    {record.classification === "pending" && <p className="text-muted-text mb-1">Pending</p>}
    {record.classification === "unclassified" && <p className="text-amber-ink mb-1">Needs classification</p>}
    {record.entries.map((entry, index) => {
      const net = (-BigInt(entry.amountCents)).toString();
      return <p key={index} className="text-body mb-1">{entry.category}{(record.entries.length > 1 || net !== record.netCents) && <span className="text-muted-text ml-2 whitespace-nowrap tabular-nums">{formatFlow(net)}</span>}</p>;
    })}
    {!record.entries.length && record.legacyCategory && <p className="text-muted-text mt-1 text-[11px]">Legacy label: {record.legacyCategory}</p>}
  </>;
}

function sourceLabel(source: string): string {
  if (source === "rule") return "Merchant rule";
  if (source === "human") return "Human";
  if (source === "manual") return "Manual (legacy)";
  if (source === "agent") return "Agent (legacy)";
  return source || "Not recorded";
}
