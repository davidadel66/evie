import { useEffect, useState } from "react";
import {
  defaultUsageQuery, inspectUsage, shiftDate,
  type Counter, type UsageQuery, type UsageReport, type UsageSource,
} from "../api/usage";
import { accountTokens, chartSegments, formatTokens } from "./usagePresentation";

export function Usage() {
  const [query, setQuery] = useState(defaultUsageQuery);
  const [draft, setDraft] = useState(query);
  const [revision, setRevision] = useState(0);
  const [report, setReport] = useState<UsageReport>();
  const [problem, setProblem] = useState("");
  const [loading, setLoading] = useState(true);
  const [selected, setSelected] = useState("evie-conversation");
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    setLoading(true);
    inspectUsage(query, controller.signal).then((next) => {
      if (controller.signal.aborted) return;
      setReport(next); setProblem("");
      const delay = next.sources.some((source) => source.state === "indexing") ? 3000 : 60000;
      timer = setTimeout(() => setRevision((value) => value + 1), delay);
    }).catch((error: unknown) => {
      if (!controller.signal.aborted) setProblem(error instanceof Error ? error.message : "Usage is unavailable.");
    }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => { controller.abort(); clearTimeout(timer); };
  }, [query, revision]);
  return <UsageView report={report} draft={draft} selected={selected} loading={loading} problem={problem}
    onDraft={setDraft} onApply={() => setQuery({ ...draft })} onRefresh={() => setRevision((value) => value + 1)} onSelect={setSelected} />;
}

type Props = {
  report?: UsageReport; draft: UsageQuery; selected: string; loading: boolean; problem?: string;
  onDraft: (query: UsageQuery) => void; onApply: () => void; onRefresh: () => void; onSelect: (id: string) => void;
};
const control = "border-hair-strong bg-card text-body focus-visible:ring-teal rounded border px-2 py-1.5 text-xs focus-visible:ring-1";

export function UsageView({ report, draft, selected, loading, problem, onDraft, onApply, onRefresh, onSelect }: Props) {
  const source = report?.sources.find((row) => row.id === selected) ?? report?.sources.find((row) => row.kind === "evie") ?? report?.sources[0];
  return <section aria-label="Usage" className="min-h-0 flex-1 overflow-y-auto p-5 sm:p-7">
    <div className="flex flex-wrap items-start justify-between gap-4">
      <div><h2 className="text-ink text-base font-medium">Token usage</h2>
        <p className="text-muted-text mt-1 text-xs">{report ? `${report.period.from} – ${shiftDate(report.period.to, -1)} · ${report.period.timezone}` : "Collecting recorded usage"}</p>
      </div>
      <form className="flex flex-wrap items-end gap-2" onSubmit={(event) => { event.preventDefault(); onApply(); }}>
        <label className="text-muted-text flex flex-col gap-1 text-[11px]">From<input required type="date" value={draft.from} onChange={(event) => onDraft({ ...draft, from: event.target.value })} className={control} /></label>
        <label className="text-muted-text flex flex-col gap-1 text-[11px]">Through<input required type="date" value={shiftDate(draft.to, -1)} onChange={(event) => { if (event.target.value) onDraft({ ...draft, to: shiftDate(event.target.value, 1) }); }} className={control} /></label>
        <button type="submit" className={control}>Apply</button>
        <button type="button" onClick={onRefresh} className={control} disabled={loading}>{loading ? "Refreshing…" : "Refresh"}</button>
      </form>
    </div>
    {problem && <p role="alert" className="text-amber-ink border-amber-hair bg-amber-bg mt-4 rounded border p-3 text-xs">{problem}{report && " Previous results remain visible below."}</p>}
    {!report && <p role="status" className="text-muted-text py-12 text-center text-xs">{loading ? "Reading usage sources…" : "Usage could not be loaded."}</p>}
    {report && <>
      <div className="mt-6 overflow-x-auto"><table className="w-full text-right text-xs">
        <caption className="sr-only">Usage sources. Account totals and local measurements overlap and must not be added together.</caption>
        <thead className="text-muted-text border-hair border-b text-[11px] font-normal"><tr>{["Source / account", "Reported total", "Input", "Cached input", "Output", "Cache share"].map((label, index) => <th key={label} className={`px-2 py-2 font-normal ${index === 0 ? "text-left" : "whitespace-nowrap"}`}>{label}</th>)}</tr></thead>
        <tbody>{report.sources.map((row) => {
          const metrics = row.summary?.metrics;
          const accountTotal = row.account ? accountTokens(row, report.period) : null;
          return <tr key={row.id} className={`${row.id === source?.id ? "bg-selected" : ""} border-hair border-b`}>
            <td className="min-w-[210px] px-2 py-3 text-left"><button type="button" aria-pressed={row.id === source?.id} onClick={() => onSelect(row.id)} className="focus-visible:ring-teal rounded text-left focus-visible:ring-1"><span className="text-ink block font-medium">{row.label}</span><span className="text-muted-text mt-0.5 block text-[11px]">{row.kind === "codex-account" ? "Account activity · provider dates" : row.kind === "evie" ? "Recorded conversation responses" : "Local activity · account unassigned"}{row.state === "indexing" ? " · collecting" : row.state === "unavailable" ? " · unavailable" : row.state === "stale" ? " · stale" : ""}</span></button></td>
            <td className="px-2 py-3 font-mono">{formatTokens(accountTotal ?? metrics?.total.value)}</td>
            <td className="px-2 py-3 font-mono">{formatTokens(metrics?.input.value)}</td>
            <td className="px-2 py-3 font-mono">{formatTokens(metrics?.cached.value)}</td>
            <td className="px-2 py-3 font-mono">{formatTokens(metrics?.output.value)}</td>
            <td className="px-2 py-3 font-mono">{percent(metrics?.cachePercent)}</td>
          </tr>;
        })}</tbody>
      </table></div>
      <p className="text-muted-text mt-2 text-[11px]">Reported totals can have different coverage. Cached input is included in input; reasoning is included in output.</p>
      {source && <SourceDetail source={source} period={report.period} />}
    </>}
  </section>;
}

function SourceDetail({ source, period }: { source: UsageSource; period: UsageQuery }) {
  const summary = source.summary;
  const metrics = summary?.metrics;
  return <div className="border-hair mt-6 border-t pt-5" aria-live="polite">
    <div className="flex flex-wrap items-start justify-between gap-2"><div><h3 className="text-ink text-base font-medium">{source.label}</h3><p className="text-muted-text mt-1 text-[11px]">{source.coverage}</p></div><span className="text-muted-text text-[11px]">{source.collectedAt ? `Collected ${timestamp(source.collectedAt)}` : "Collection pending"}</span></div>
    {source.message && <p className="text-muted-text mt-2 max-w-3xl text-xs">{source.message}</p>}
    {metrics && <>
      <div className="text-muted-text mt-4 flex flex-wrap gap-x-6 gap-y-2 text-xs"><span>Measured calls <b className="text-body font-mono font-normal">{metrics.measuredCalls.toLocaleString()} / {metrics.calls.toLocaleString()}</b></span><span>Cached input <b className="text-body font-mono font-normal">{percent(metrics.cachePercent)}</b></span><span>First in range <b className="text-body font-normal">{timestamp(summary.firstObserved)}</b></span></div>
      {metrics.calls === 0 ? <p className="text-muted-text py-10 text-center text-xs">No recorded calls in this period.{source.state === "indexing" ? " Local collection is still running." : ""}</p> : <TokenChart source={source} period={period} />}
      <div className="mt-6 grid gap-7 lg:grid-cols-2">
        <div><h4 className="text-body mb-2 text-xs font-medium">Measurement coverage</h4>
          <MetricRow label="Reported total" value={metrics.total} calls={metrics.calls} />
          <MetricRow label="Input" value={metrics.input} calls={metrics.calls} />
          <MetricRow label="Cached input" value={metrics.cached} calls={metrics.calls} />
          <MetricRow label="Output" value={metrics.output} calls={metrics.calls} />
          <MetricRow label="Reasoning · within output" value={metrics.reasoning} calls={metrics.calls} />
          <MetricRow label="Cache writes · within input" value={metrics.cacheWrite} calls={metrics.calls} />
          <p className="text-muted-text mt-2 text-[11px]">Cache share uses {formatTokens(metrics.cacheInput)} input tokens across {metrics.cacheCalls} compatible calls.{metrics.anomalies > 0 ? ` ${metrics.anomalies} calls have inconsistent counters; affected ratios and chart splits are unavailable.` : ""}</p>
        </div>
        <div><h4 className="text-body mb-2 text-xs font-medium">{source.kind === "evie" ? "Requested models" : "Model attribution"}</h4>
          {summary.models.map((model) => <div key={model.name} className="border-hair flex items-start justify-between gap-4 border-b py-2 text-xs"><span className="text-muted-text min-w-0 break-all">{model.name}</span><span className="text-body whitespace-nowrap font-mono">{formatTokens(model.metrics.total.value)} tokens</span></div>)}
          <p className="text-muted-text mt-2 text-[11px]">Charges are not recorded in this view. Token counts do not determine a subscription bill.</p>
        </div>
      </div>
    </>}
    {source.account && <>
      <div className="text-muted-text mt-4 flex flex-wrap gap-6 text-xs"><span>Reported dates in range <b className="text-body font-mono font-normal">{source.account.daily.filter((day) => day.date >= period.from && day.date < period.to).length}</b></span><span>Lifetime tokens <b className="text-body font-mono font-normal">{formatTokens(source.account.lifetimeTokens)}</b></span><span>Plan <b className="text-body font-normal">{source.account.plan || "Unavailable"}</b></span></div>
      <TokenChart source={source} period={period} />
      <h4 className="text-body mt-6 text-xs font-medium">Current allowances</h4>
      {source.account.windows.length === 0 && <p className="text-muted-text mt-2 text-xs">Allowance information is unavailable.</p>}
      <div className="mt-3 grid gap-5 sm:grid-cols-2">{source.account.windows.map((window) => <div key={`${window.name}:${window.minutes}`}><div className="text-muted-text flex justify-between gap-3 text-xs"><span>{window.name} · {windowLabel(window.minutes)}</span><span className="text-body font-mono">{percent(window.usedPercent)} used</span></div><div className="bg-hair mt-2 h-1 overflow-hidden rounded" role="progressbar" aria-label={`${window.name} ${windowLabel(window.minutes)} used`} aria-valuenow={window.usedPercent} aria-valuemin={0} aria-valuemax={100}><div className="bg-teal h-full" style={{ width: `${window.usedPercent}%` }} /></div><p className="text-muted-text mt-2 text-[11px]">Resets {timestamp(window.resetsAt)}</p></div>)}</div>
    </>}
  </div>;
}

function MetricRow({ label, value, calls }: { label: string; value: Counter; calls: number }) {
  return <div className="border-hair flex flex-wrap justify-between gap-2 border-b py-2 text-xs"><span className="text-muted-text">{label}</span><span className="text-body font-mono">{formatTokens(value.value)} <span className="text-muted-text font-sans text-[11px]">· {value.reported}/{calls} calls</span></span></div>;
}

type ChartDay = { date: string; segments: bigint[] | null };
function TokenChart({ source, period }: { source: UsageSource; period: UsageQuery }) {
  const account = source.account;
  const days: ChartDay[] = [];
  for (let date = period.from; date < period.to; date = shiftDate(date, 1)) {
    if (account) { const day = account.daily.find((day) => day.date === date); days.push({ date, segments: day ? [BigInt(day.tokens)] : null }); }
    else { const day = source.summary?.daily.find((day) => day.date === date); days.push({ date, segments: day ? chartSegments(day.metrics) : null }); }
  }
  const max = days.reduce((max, day) => { const value = day.segments?.reduce((a, b) => a + b, 0n) ?? 0n; return value > max ? value : max; }, 0n);
  const labels = account ? ["Reported account tokens"] : ["Cached input", "Other input", "Output"];
  const colors = account ? ["bg-teal"] : ["bg-teal", "bg-muted-text", "bg-amber"];
  return <div className="mt-6">
    <div className="flex flex-wrap justify-between gap-3 text-xs"><h4 className="text-body font-medium">{account ? "Daily account activity · provider dates" : "Daily recorded tokens"}</h4><div className="text-muted-text flex flex-wrap gap-3 text-[11px]">{labels.map((label, index) => <span key={label}><i className={`${colors[index]} mr-1.5 inline-block h-2 w-2 rounded-sm`} />{label}</span>)}</div></div>
    <div className="mt-4 grid grid-cols-[52px_minmax(0,1fr)] gap-2" role="img" aria-label={`${source.label} daily token chart. Missing breakdowns are marked with a dash.`}>
      <div className="text-muted-text flex h-32 flex-col justify-between text-right font-mono text-[11px]"><span>{formatTokens(String(max))}</span><span>{formatTokens(String(max / 2n))}</span><span>0</span></div>
      <div className="border-hair flex h-32 items-end gap-1 border-b">{days.map((day) => <div key={day.date} className="flex h-full min-w-0 flex-1 items-end justify-center" title={day.segments ? `${day.date}: ${day.segments.reduce((a, b) => a + b, 0n).toLocaleString()} tokens` : `${day.date}: unavailable`}>
        {day.segments ? <div className="flex w-full max-w-10 flex-col-reverse">{day.segments.map((value, index) => <div key={labels[index]} className={colors[index]} style={{ height: max === 0n ? 0 : `${128 * Number(value) / Number(max)}px` }} />)}</div> : <span className="text-muted-text text-[11px]">—</span>}
      </div>)}</div>
      <span className="text-muted-text text-right text-[11px]">tokens</span><div className="text-muted-text flex gap-1 text-center text-[11px]">{days.map((day, index) => <span key={day.date} className="min-w-0 flex-1 overflow-visible">{index % Math.max(1, Math.ceil(days.length / 5)) === 0 ? day.date.slice(5) : ""}</span>)}</div>
    </div>
    <p className="text-muted-text mt-3 text-[11px]">{account ? "These are provider calendar dates. Missing days are unknown; account activity is separate from local cache measurements." : "A dash means the daily split is unavailable or incomplete. Cache subsets are never counted twice."}</p>
    <details className="text-muted-text mt-3 text-xs">
      <summary className="focus-visible:ring-teal w-fit cursor-pointer rounded focus-visible:ring-1">Daily values</summary>
      <div className="mt-2 overflow-x-auto"><table className="w-full text-right text-xs">
        <caption className="sr-only">{source.label} daily token values</caption>
        <thead><tr><th scope="col" className="p-2 text-left font-normal">Date</th>{labels.map((label) => <th key={label} scope="col" className="p-2 font-normal">{label}</th>)}</tr></thead>
        <tbody>{days.map((day) => <tr key={day.date} className="border-hair border-t"><th scope="row" className="whitespace-nowrap p-2 text-left font-normal">{day.date}</th>{labels.map((label, index) => <td key={label} className="p-2 font-mono">{day.segments ? day.segments[index].toLocaleString() : "Unavailable"}</td>)}</tr>)}</tbody>
      </table></div>
    </details>
  </div>;
}

function percent(value?: number | null) { return value == null ? "—" : `${value.toFixed(1)}%`; }
function timestamp(value?: string | null) { return value ? new Date(value).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" }) : "—"; }
function windowLabel(minutes: number) { return minutes % 1440 === 0 ? `${minutes / 1440}-day window` : minutes % 60 === 0 ? `${minutes / 60}-hour window` : `${minutes}-minute window`; }
