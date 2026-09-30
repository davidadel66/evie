import { useEffect, useRef, useState } from "react";
import { inspectSpending, refreshSpending, type SpendingDay, type SpendingReport } from "../api/spending";
import { calendarDate } from "../api/usage";
import { calendarWeeks, flowColor, formatFlow, monthLabel, shiftMonth } from "./spendingPresentation";
import { SpendingTransactions } from "./SpendingTransactions";
import { CashFlow } from "./CashFlow";
import { SpendingDayDialog } from "./SpendingDayDialog";
import { SpendingAccounts } from "./SpendingAccounts";

export function Spending() {
  const [month, setMonth] = useState(() => calendarDate(new Date()).slice(0, 7));
  const [revision, setRevision] = useState(0);
  const [report, setReport] = useState<SpendingReport>();
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [problem, setProblem] = useState("");
  const [refreshProblem, setRefreshProblem] = useState("");
  const refreshRequest = useRef<AbortController | null>(null);
  const year = Number(month.slice(0, 4));

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setProblem("");
    inspectSpending(year, controller.signal).then((next) => {
      if (!controller.signal.aborted) setReport(next);
    }).catch((error: unknown) => {
      if (!controller.signal.aborted) setProblem(error instanceof Error ? error.message : "Spending could not be loaded.");
    }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [year, revision]);

  useEffect(() => () => refreshRequest.current?.abort(), []);

  const refresh = async () => {
    if (refreshRequest.current) return;
    const controller = new AbortController();
    refreshRequest.current = controller;
    setRefreshing(true); setRefreshProblem("");
    try {
      const result = await refreshSpending(controller.signal);
      if (controller.signal.aborted) return;
      if (result.banksFailed) {
        setRefreshProblem(`${result.banksSucceeded} ${result.banksSucceeded === 1 ? "bank" : "banks"} refreshed; ${result.banksFailed} failed.`);
      }
    } catch (error: unknown) {
      if (!controller.signal.aborted) setRefreshProblem(error instanceof Error ? error.message : "Banks could not be refreshed.");
    } finally {
      if (!controller.signal.aborted) {
        // A failed refresh may still have committed updates from another bank.
        setRevision((value) => value + 1); setRefreshing(false);
      }
      if (refreshRequest.current === controller) refreshRequest.current = null;
    }
  };

  return <SpendingView month={month} report={report?.year === year ? report : undefined} loading={loading}
    refreshing={refreshing} problem={problem || refreshProblem} revision={revision}
    onMonth={setMonth} onRefresh={refresh} onRetry={() => setRevision((value) => value + 1)} onSaved={() => setRevision((value) => value + 1)} />;
}

type Props = {
  month: string; report?: SpendingReport; loading: boolean; refreshing: boolean;
  problem?: string; today?: string; revision?: number;
  onMonth: (month: string) => void; onRefresh: () => void; onRetry: () => void;
  onSaved?: () => void;
};
const control = "border-hair-strong bg-card text-body focus-visible:ring-teal rounded-md border px-3 py-2 text-xs focus-visible:ring-2 focus-visible:outline-none disabled:cursor-default disabled:opacity-40";

export function SpendingView({ month, report, loading, refreshing, problem, today = calendarDate(new Date()), revision = 0, onMonth, onRefresh, onRetry, onSaved = () => undefined }: Props) {
  const [view, setView] = useState<"overview" | "transactions">("overview");
  const previous = shiftMonth(month, -1), next = shiftMonth(month, 1);
  const days = report?.days.filter((day) => day.date.startsWith(`${month}-`)) ?? [];
  return <section aria-label="Spending" className="min-h-0 min-w-0 flex-1 overflow-y-auto p-5 sm:p-7">
    <SpendingAccounts onLinked={onSaved} />
    <div className="border-hair flex flex-wrap items-center justify-between gap-x-6 gap-y-3 border-b">
      <nav aria-label="Spending views" className="flex self-stretch">
        {(["overview", "transactions"] as const).map((item) => <button key={item} type="button" aria-current={view === item ? "page" : undefined} onClick={() => setView(item)} className={`${view === item ? "border-teal text-ink" : "border-transparent text-muted-text hover:text-body"} focus-visible:ring-teal border-b px-3 py-3 text-xs focus-visible:ring-2 focus-visible:outline-none`}>{item === "overview" ? "Overview" : "Transactions"}</button>)}
      </nav>
      <div className="mb-2 flex items-center gap-1" aria-label="Month navigation">
        <button type="button" className={control} aria-label="Previous month" disabled={!previous} onClick={() => previous && onMonth(previous)}><Chevron direction="left" /></button>
        <h3 className="text-ink min-w-36 px-2 text-center text-sm font-medium" aria-live="polite">{monthLabel(month)}</h3>
        <button type="button" className={control} aria-label="Next month" disabled={!next} onClick={() => next && onMonth(next)}><Chevron direction="right" /></button>
        {month !== today.slice(0, 7) && <button type="button" className={`${control} ml-1`} onClick={() => onMonth(today.slice(0, 7))}>This month</button>}
      </div>
      <button type="button" className={`${control} mb-2 flex items-center gap-2`} onClick={onRefresh} disabled={refreshing || report?.linkedBanks === 0} title={report?.refreshedAt ? `Last fetched ${new Date(report.refreshedAt).toLocaleString()}` : undefined}>
        <svg aria-hidden="true" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6"><path d="M20 7v5h-5M4 17v-5h5M6.1 7a7 7 0 0 1 11.8-1L20 9M4 15l2.1 3A7 7 0 0 0 17.9 17" /></svg>
        {refreshing ? "Fetching latest…" : "Fetch latest"}
      </button>
    </div>
    {problem && <div role="alert" className="text-amber-ink border-amber-hair bg-amber-bg mt-4 rounded-md border p-3 text-xs">{problem}{!report && <button type="button" onClick={onRetry} className="ml-3 underline underline-offset-2">Try again</button>}</div>}
    {view === "transactions" ? <SpendingTransactions month={month} revision={revision} /> : <>
      {report?.linkedBanks !== 0 && <CashFlow month={month} today={today} revision={revision} onMonth={onMonth} />}
      {!report && <p role="status" className="text-muted-text py-16 text-center text-xs">{loading ? "Loading…" : "Spending is unavailable."}</p>}
      {report && (report.linkedBanks === 0 ? <div className="mt-6 py-12"><h3 className="text-body text-sm">No connected banks.</h3><button type="button" className={`${control} mt-4`} onClick={onRetry}>Check connections</button></div> : <>
        <div className="border-hair mt-7 flex flex-wrap items-center justify-between gap-3 border-t pt-5">
          <h3 className="text-body text-sm font-medium">Daily net flow</h3>
          <div className="text-muted-text flex flex-wrap items-center gap-4 text-[11px]"><span><i className="bg-danger mr-1.5 inline-block h-2 w-2 rounded-sm" />Net outflow</span><span><i className="bg-teal mr-1.5 inline-block h-2 w-2 rounded-sm" />Net inflow</span></div>
        </div>
        {days.length === 0 && <p role="status" className="text-muted-text mt-4 text-xs">No posted transactions.</p>}
        {loading && <p role="status" className="text-muted-text mt-3 text-xs">Updating…</p>}
        <FlowCalendar key={month} month={month} days={days} today={today} onSaved={onSaved} />
      </>)}
    </>}
  </section>;
}

function Chevron({ direction }: { direction: "left" | "right" }) {
  return <svg aria-hidden="true" width="14" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5"><path d={direction === "left" ? "M10 3 5 8l5 5" : "m6 3 5 5-5 5"} /></svg>;
}

function FlowCalendar({ month, days, today, onSaved }: { month: string; days: SpendingDay[]; today: string; onSaved: () => void }) {
  const [selected, setSelected] = useState<string>();
  const byDate = new Map(days.map((day) => [day.date, day]));
  const maximum = days.filter((day) => day.date <= today).reduce((max, day) => { const n = BigInt(day.netCents); const abs = n < 0n ? -n : n; return abs > max ? abs : max; }, 0n);
  return <>
    {selected && <SpendingDayDialog key={selected} date={selected} onClose={() => setSelected(undefined)} onSaved={onSaved} />}
    <div className="mt-4 max-w-full overflow-x-auto pb-2" tabIndex={0} role="region" aria-label="Monthly net flow calendar; scroll horizontally on small screens">
      <table className="w-full min-w-[560px] table-fixed border-separate border-spacing-1 text-[11px] tabular-nums">
        <caption className="sr-only">Daily net flow for {monthLabel(month)}. Positive amounts are net inflows; negative amounts are net outflows.</caption>
        <thead><tr>{["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"].map((day) => <th scope="col" key={day} className="text-muted-text pb-2 pl-2 text-left font-normal">{day}</th>)}</tr></thead>
        <tbody>{calendarWeeks(month).map((week, index) => <tr key={index}>{week.map((date, column) => {
          if (!date) return <td key={column} />;
          const future = date > today;
          const day = future ? undefined : byDate.get(date);
          const description = day ? `${date}: net ${formatFlow(day.netCents)}; money in ${formatFlow(day.inflowCents, false)}; money out ${formatFlow(day.outflowCents, false)}; ${day.transactions} ${day.transactions === 1 ? "transaction" : "transactions"}` : `${date}: ${future ? "future date" : "no recorded transactions"}`;
          const content = <><span className={`${date === today ? "text-teal font-semibold" : "text-muted-text"} block text-[11px]`} aria-current={date === today ? "date" : undefined}>{Number(date.slice(8))}</span><span className="text-ink mt-3 block overflow-hidden text-ellipsis whitespace-nowrap text-xs font-medium">{day ? formatFlow(day.netCents) : future ? "\u00a0" : "—"}</span></>;
          return <td key={column} className={`${future ? "bg-card/40" : "bg-card"} h-[76px] rounded-md align-top`} style={day ? { backgroundColor: flowColor(day.netCents, maximum) } : undefined}>
            {!future ? <button type="button" title={description} aria-label={description} aria-haspopup="dialog" onClick={() => setSelected(date)} className="focus-visible:ring-teal block h-full w-full cursor-pointer rounded-md p-2 text-left focus-visible:ring-2 focus-visible:outline-none">{content}</button> : <div className="p-2" aria-label={description}>{content}</div>}
          </td>;
        })}</tr>)}</tbody>
      </table>
    </div>
  </>;
}
