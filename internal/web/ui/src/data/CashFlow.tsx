import { useEffect, useRef, useState } from "react";
import { inspectCashFlow, type CashFlowCategory, type CashFlowMonth, type CashFlowReport } from "../api/spending";
import { formatFlow, monthLabel } from "./spendingPresentation";
import { axisAmount, cashFlowHistoryEnd, cashFlowScale, cashFlowY, categoryKey, categoryLabel, categoryWidth, percentOf, sortedCategories } from "./cashFlowPresentation";

export function CashFlow({ month, today, revision, onMonth }: { month: string; today: string; revision: number; onMonth: (month: string) => void }) {
  const [windowEnd, setWindowEnd] = useState(month);
  const [report, setReport] = useState<CashFlowReport>();
  const [loading, setLoading] = useState(true);
  const [problem, setProblem] = useState("");
  const [retry, setRetry] = useState(0);
  const historyEnd = cashFlowHistoryEnd(month, windowEnd);
  useEffect(() => { if (windowEnd !== historyEnd) setWindowEnd(historyEnd); }, [windowEnd, historyEnd]);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setProblem("");
    inspectCashFlow({ month, historyEnd, asOfDate: today }, controller.signal).then((next) => {
      if (!controller.signal.aborted) setReport(next);
    }).catch((error: unknown) => {
      if (!controller.signal.aborted) setProblem(error instanceof Error ? error.message : "Cash flow could not be loaded.");
    }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [month, historyEnd, today, revision, retry]);
  const sameWindow = report?.historyEnd === historyEnd && report.asOfDate === today;
  const matching = sameWindow && report.month === month ? report : undefined;
  return <CashFlowView month={month} today={today} report={matching} history={sameWindow ? report.history : []}
    loading={loading} problem={problem} onMonth={onMonth} onRetry={() => setRetry((value) => value + 1)} />;
}

type Props = {
  month: string; today: string; report?: CashFlowReport; history: CashFlowMonth[];
  loading: boolean; problem?: string; onMonth: (month: string) => void; onRetry: () => void;
};

export function CashFlowView({ month, today, report, history, loading, problem, onMonth, onRetry }: Props) {
  const total = report?.selected;
  const recorded = total && total.transactions > 0;
  return <section aria-label="Monthly cash flow" className="mt-5 min-w-0">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <h3 className="text-body text-sm font-medium">Cash flow</h3>
      <div className="text-muted-text flex flex-wrap items-center gap-4 text-[11px]">
        {loading && <span role="status">Loading…</span>}
        <span><i className="bg-teal mr-1.5 inline-block h-2 w-2 rounded-sm" />Inflow</span>
        <span><i className="bg-danger mr-1.5 inline-block h-2 w-2 rounded-sm" />Outflow</span>
        <span><i className="bg-ink mr-1.5 inline-block h-0.5 w-3 align-middle" />Net flow</span>
      </div>
    </div>
    {problem && <div role="alert" className="text-amber-ink border-amber-hair bg-amber-bg mt-3 rounded-md border p-3 text-xs">{problem}<button type="button" onClick={onRetry} className="ml-3 underline underline-offset-2">Retry</button></div>}
    {history.length > 0 && <MonthlyFlowChart history={history} selected={month} today={today} onMonth={onMonth} />}
    <dl className="border-hair mt-5 grid grid-cols-2 overflow-hidden rounded-lg border sm:grid-cols-4">
      {[
        { label: "Inflow", value: recorded ? formatFlow(total.inflowCents, false) : "—", color: "text-teal-hover" },
        { label: "Outflow", value: recorded ? formatFlow(total.outflowCents, false) : "—", color: "text-danger" },
        { label: "Net flow", value: recorded ? formatFlow(total.netCents) : "—", color: "text-ink" },
        { label: "Net rate", value: recorded ? percentOf(total.netCents, total.inflowCents) : "—", color: "text-ink" },
      ].map((metric, index) => <div key={metric.label} className={`${index % 2 ? "border-l" : ""} ${index >= 2 ? "border-t sm:border-t-0" : ""} ${index === 2 ? "sm:border-l" : ""} border-hair bg-card min-w-0 px-4 py-5`}>
        <dt className="text-muted-text text-xs" title={metric.label === "Net rate" ? "Net flow divided by inflow" : undefined}>{metric.label}</dt>
        <dd className={`${metric.color} mt-2 overflow-x-auto whitespace-nowrap text-xl font-medium tabular-nums`}>{metric.value}</dd>
      </div>)}
    </dl>
    {report && <div className="mt-6 grid items-start gap-5 lg:grid-cols-2">
      <CategoryBars title="Inflow" rows={report.categories} side="inflowCents" total={report.selected.inflowCents} />
      <CategoryBars title="Outflow" rows={report.categories} side="outflowCents" total={report.selected.outflowCents} />
    </div>}
  </section>;
}

function MonthlyFlowChart({ history, selected, today, onMonth }: { history: CashFlowMonth[]; selected: string; today: string; onMonth: (month: string) => void }) {
  const viewport = useRef<HTMLDivElement>(null);
  const selectedButton = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const container = viewport.current;
    if (!container) return;
    const revealSelection = () => {
      const button = selectedButton.current;
      if (!button) return;
      const right = button.offsetLeft + button.offsetWidth;
      if (button.offsetLeft < container.scrollLeft) container.scrollLeft = Math.max(0, button.offsetLeft - 12);
      else if (right > container.scrollLeft + container.clientWidth) container.scrollLeft = right - container.clientWidth + 12;
    };
    revealSelection();
    const observer = new ResizeObserver(revealSelection);
    observer.observe(container);
    return () => observer.disconnect();
  }, [selected, history]);
  const scale = cashFlowScale(history);
  const width = 884 / history.length;
  const center = (index: number) => 64 + (index + 0.5) * width;
  const baseline = cashFlowY("0", scale);
  const current = today.slice(0, 7);
  return <div ref={viewport} className="mt-4 max-w-full overflow-x-auto pb-1" tabIndex={0} role="region" aria-label="Monthly cash flow chart">
    <div className="relative min-w-[960px]">
      <svg viewBox="0 0 960 280" className="block w-full" aria-hidden="true">
        {[scale, scale / 2n, 0n, -scale / 2n, -scale].map((tick) => {
          const y = cashFlowY(tick.toString(), scale);
          return <g key={String(tick)}><line x1="64" x2="948" y1={y} y2={y} stroke={tick === 0n ? "var(--color-hair-input)" : "var(--color-hair)"} strokeWidth="1" /><text x="54" y={y + 4} textAnchor="end" fill="var(--color-muted-text)" fontSize="10">{axisAmount(tick)}</text></g>;
        })}
        {history.map((point, index) => {
          const x = center(index), bar = Math.min(54, width * 0.65);
          const upper = cashFlowY(point.inflowCents, scale);
          const lower = cashFlowY((-BigInt(point.outflowCents)).toString(), scale);
          const chosen = point.month === selected;
          return <g key={point.month}>
            <rect x={x - bar / 2} y={upper} width={bar} height={Math.max(0, baseline - upper)} fill="var(--color-teal)" opacity={chosen ? 0.95 : 0.32} />
            <rect x={x - bar / 2} y={baseline} width={bar} height={Math.max(0, lower - baseline)} fill="var(--color-danger)" opacity={chosen ? 0.95 : 0.32} />
            {!point.transactions && <text x={x} y={baseline - 7} textAnchor="middle" fill="var(--color-faint)" fontSize="11">—</text>}
            <text x={x} y="250" textAnchor="middle" fill={chosen ? "var(--color-ink)" : "var(--color-muted-text)"} fontSize="11" fontWeight={chosen ? 600 : 400}>{monthLabel(point.month).split(" ")[0].slice(0, 3)}</text>
            {(index === 0 || point.month.endsWith("-01")) && <text x={x} y="268" textAnchor="middle" fill="var(--color-faint)" fontSize="10">{point.month.slice(0, 4)}</text>}
          </g>;
        })}
        {history.map((point, index) => {
          if (!point.transactions) return null;
          const previous = history[index - 1];
          const x = center(index), y = cashFlowY(point.netCents, scale);
          return <g key={point.month}>
            {previous?.transactions > 0 && <path d={`M ${center(index - 1)} ${cashFlowY(previous.netCents, scale)} L ${x} ${y}`} stroke="var(--color-ink)" strokeWidth="2.5" fill="none" strokeDasharray={point.month === current ? "4 5" : undefined} strokeLinecap="round" />}
            <circle cx={x} cy={y} r={point.month === selected ? 3.5 : 2} fill="var(--color-ink)" />
          </g>;
        })}
      </svg>
      {history.map((point, index) => {
        const label = `${monthLabel(point.month)}${point.month === current ? " to date" : ""}: ${point.transactions ? `inflow ${formatFlow(point.inflowCents, false)}, outflow ${formatFlow(point.outflowCents, false)}, net ${formatFlow(point.netCents)}` : point.month > current ? "future month" : "no recorded transactions"}`;
        return <button ref={point.month === selected ? selectedButton : undefined} key={point.month} type="button" aria-label={label} aria-pressed={point.month === selected} title={label} onClick={() => onMonth(point.month)} style={{ left: `${(64 + index * width) / 9.6}%`, width: `${width / 9.6}%` }} className="focus-visible:ring-teal absolute inset-y-0 cursor-pointer rounded-sm hover:bg-ink/5 focus-visible:ring-2 focus-visible:outline-none" />;
      })}
    </div>
  </div>;
}

function CategoryBars({ title, rows, side, total }: { title: string; rows: CashFlowCategory[]; side: "inflowCents" | "outflowCents"; total: string }) {
  const categories = sortedCategories(rows, side);
  const [expanded, setExpanded] = useState(false);
  const visible = expanded ? categories : categories.slice(0, 6);
  return <section aria-label={`${title} by category`} className="border-hair bg-card min-w-0 overflow-hidden rounded-lg border">
    <h4 className="border-hair text-body border-b px-4 py-3 text-sm font-medium">{title} by category</h4>
    {categories.length === 0 ? <p className="text-muted-text px-4 py-6 text-xs">No {title.toLowerCase()}.</p> : <ul className="space-y-2 p-3">
      {visible.map((category) => <li key={categoryKey(category)} className="relative overflow-hidden rounded-md" title={`${categoryLabel(category)}: ${formatFlow(category[side], false)} (${percentOf(category[side], total)})`}>
        <span aria-hidden="true" className={`${side === "inflowCents" ? "bg-teal/25" : "bg-danger/25"} absolute inset-y-0 left-0 rounded-md`} style={{ width: `${categoryWidth(category[side], total)}%` }} />
        <div className="relative flex flex-wrap items-center justify-between gap-x-3 gap-y-1 px-3 py-3 text-xs">
          <span className="text-body min-w-0 break-words">{categoryLabel(category)}</span>
          <span className="text-ink ml-auto whitespace-nowrap tabular-nums">{formatFlow(category[side], false)} <span className="text-muted-text">({percentOf(category[side], total)})</span></span>
        </div>
      </li>)}
    </ul>}
    {categories.length > 6 && <button type="button" aria-expanded={expanded} onClick={() => setExpanded((value) => !value)} className="text-muted-text hover:text-body focus-visible:ring-teal mb-3 ml-4 rounded text-xs focus-visible:ring-2 focus-visible:outline-none">{expanded ? "Show less" : `Show all ${categories.length}`}</button>}
  </section>;
}
