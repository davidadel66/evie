import { useEffect, useId, useState, type KeyboardEvent, type ReactNode } from "react";
import { subagentAPI, type Subagent, type SubagentAPI, type SubagentReport, type SubagentTrace } from "../api/subagents";
import { ToolDebugDetails, ToolInspection } from "../artifacts/ToolInspection";
import { ActivityItems } from "../chat/Activity";
import type { ToolItem } from "../chat/activityModel";
import { InertLink, Markdown } from "../chat/Markdown";
import { ChevronDown } from "../ui/Icon";
import { subagentActionText, subagentDefaultView, subagentElapsed, subagentFinished, subagentProgressLabel, subagentReportFacts, subagentSourceText, subagentStateLabel, subagentStateTone, subagentUsageText, type SubagentView } from "./subagentView";
import { useSubagentList } from "./useSubagents";

// The children one chat row started (chat-activity.spec.md, subagent
// inspector amendment): each child's state and latest action, kept current
// while any is unfinished; one child's own trace or its report
// (artifacts.spec.md); and one of its actions.
export function SubagentInspection({tool, sessionId, api = subagentAPI}: {tool: ToolItem; sessionId: string; api?: SubagentAPI}) {
  const {list, problem} = useSubagentList(sessionId, tool, tool.result === undefined, api);
  const [open, setOpen] = useState<string>();
  const now = useNow(!!list?.some((child) => !subagentFinished(child.state)));
  const child = list?.find((item) => item.executionId === open);
  if (child) return <SubagentDetail child={child} sessionId={sessionId} api={api} now={now} onBack={() => setOpen(undefined)} />;
  if (!list) return <div className="p-5">{problem ? <p role="alert" className="text-danger text-xs">{problem}</p> : <p role="status" className="text-muted-text text-xs">Loading subagents…</p>}</div>;
  if (list.length === 0) return <ToolInspection tool={tool} />;
  return <SubagentList list={list} tool={tool} now={now} problem={problem} onOpen={setOpen} />;
}

export function SubagentList({list, tool, now, problem, onOpen}: {list: Subagent[]; tool: ToolItem; now: number; problem?: string; onOpen: (executionId: string) => void}) {
  return <div className="space-y-5 p-5">
    <div>
      <h2 className="text-ink text-base font-medium">{list.length === 1 ? "1 subagent" : `${list.length} subagents`}</h2>
      <p className="text-muted-text mt-1 text-xs">{subagentProgressLabel(list)}{tool.result === undefined ? "" : " · results returned to Evie"}</p>
      {problem && <p role="alert" className="text-amber-ink mt-2 text-xs">{problem}</p>}
    </div>
    <ul className="space-y-1">
      {list.map((item) => <li key={item.executionId}>
        <button type="button" aria-label={`Open subagent: ${item.objective}`} onClick={() => onOpen(item.executionId)}
          className="hover:bg-hover focus-visible:outline-teal -mx-2 flex w-[calc(100%+1rem)] min-w-0 cursor-pointer flex-col gap-1 rounded-md px-2 py-2 text-left focus-visible:outline-2">
          <span className="flex w-full min-w-0 items-baseline gap-3 text-[13px]">
            <StateLabel child={item} />
            <span className="text-body line-clamp-2 min-w-0 flex-1 break-words">{item.objective}</span>
            <span className="text-muted-text shrink-0 text-xs">{subagentElapsed(item, now)}</span>
          </span>
          <span className="text-muted-text block w-full truncate text-xs">
            {item.latest ? subagentActionText(item.latest) : item.state === "admitted" ? "Waiting for a free slot" : "No actions yet"}
            {item.actions > 0 && ` · ${item.actions} ${item.actions === 1 ? "action" : "actions"}`}
          </span>
        </button>
      </li>)}
    </ul>
    <details className="border-hair border-t pt-4">
      <summary className="text-muted-text hover:text-body focus-visible:outline-teal cursor-pointer rounded-sm text-xs focus-visible:outline-2 focus-visible:outline-offset-4">Debug details</summary>
      <ToolDebugDetails tool={tool} />
    </details>
  </div>;
}

function SubagentDetail({child, sessionId, api, now, onBack}: {child: Subagent; sessionId: string; api: SubagentAPI; now: number; onBack: () => void}) {
  const trace = useSubagentTrace(sessionId, child, api);
  const report = useSubagentReport(sessionId, child, api);
  const [selected, setSelected] = useState<string>();
  // Unset follows the default view until David picks one.
  const [view, setView] = useState<SubagentView>();
  const action = trace.value?.items.find((item): item is ToolItem => item.kind === "tool" && item.key === selected);
  if (action) return <div>
    <BackButton label="Back to subagent" onClick={() => setSelected(undefined)} />
    <ToolInspection tool={action} />
  </div>;
  return <SubagentDetailView child={child} trace={trace.value} problem={trace.problem} report={report.value} reportProblem={report.problem} view={view} now={now} onBack={onBack} onSelect={setSelected} onView={setView} />;
}

export function SubagentDetailView({child, trace, problem, report, reportProblem, view: picked, now, onBack, onSelect, onView = () => undefined}: {
  child: Subagent; trace?: SubagentTrace; problem?: string; report?: SubagentReport; reportProblem?: string;
  /** The view David picked; unset follows the default. */
  view?: SubagentView;
  now: number; onBack: () => void; onSelect: (key: string) => void; onView?: (view: SubagentView) => void;
}) {
  const live = !subagentFinished(child.state);
  const view = picked ?? subagentDefaultView(child, report);
  const id = useId();
  const tabs: SubagentView[] = ["report", "activity"];
  const onKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    event.preventDefault();
    const next = view === "report" ? "activity" : "report";
    onView(next);
    (event.currentTarget.parentElement?.children[tabs.indexOf(next)] as HTMLButtonElement | undefined)?.focus();
  };
  return <div>
    <BackButton label="All subagents" onClick={onBack} />
    <div className="space-y-5 px-5 pb-5">
    <div className="space-y-2">
      <p className="flex items-baseline gap-3 text-[13px]"><StateLabel child={child} /><span className="text-muted-text text-xs">
        {[subagentElapsed(child, now), `${child.actions} ${child.actions === 1 ? "action" : "actions"}`, child.worker].filter(Boolean).join(" · ")}
      </span></p>
      <p className="text-body max-h-60 overflow-auto text-[13px] leading-relaxed whitespace-pre-wrap break-words">{child.objective}</p>
      {child.context && <details className="text-muted-text text-xs">
        <summary className="hover:text-body cursor-pointer">Context Evie gave it</summary>
        <p className="mt-2 max-h-60 overflow-auto leading-relaxed whitespace-pre-wrap break-words">{child.context}</p>
      </details>}
    </div>
    <div className="border-hair border-t">
      <div role="tablist" aria-label="Subagent view" className="border-hair flex gap-4 border-b">
        {tabs.map((tab) => <button key={tab} id={`${id}-${tab}`} type="button" role="tab" aria-selected={view === tab} aria-controls={`${id}-panel`} tabIndex={view === tab ? 0 : -1}
          onClick={() => onView(tab)} onKeyDown={onKeyDown}
          className={`focus-visible:outline-teal -mb-px cursor-pointer border-b-2 py-2 text-xs focus-visible:outline-2 ${view === tab ? "border-teal text-body" : "text-muted-text hover:text-body border-transparent"}`}>
          {tab === "report" ? "Report" : "Activity"}
        </button>)}
      </div>
      <div id={`${id}-panel`} role="tabpanel" aria-labelledby={`${id}-${view}`} className="space-y-3 pt-4">
      {view === "report" ? <SubagentReportView child={child} report={report} problem={reportProblem} /> : <>
      {problem && <p role="alert" className="text-amber-ink text-xs">{problem}</p>}
      {!trace ? !problem && <p role="status" className="text-muted-text text-xs">Loading its activity…</p>
        : <ActivityItems items={trace.items} active={live} onAnswer={() => undefined} onOpenTool={(key) => onSelect(key)} onOpenFile={(key) => onSelect(key)} />}
      {trace && trace.items.length <= 1 && <p className="text-muted-text text-xs">{live ? "Starting…" : "No activity was recorded."}</p>}
      </>}
      </div>
    </div>
    </div>
  </div>;
}

/** A child's stored report as an inert document, below Evie's record of the
 *  attempt (how it ended, what it cost, the sources Evie checked and its
 *  notes) and above the worker's own limitations; or why there is none to
 *  show. */
export function SubagentReportView({child, report, problem}: {child: Subagent; report?: SubagentReport; problem?: string}) {
  if (!subagentFinished(child.state)) return <p role="status" className="text-muted-text text-xs">The report appears when the worker finishes.</p>;
  if (problem) return <p role="alert" className="text-amber-ink text-xs">{problem}</p>;
  if (!report) return <p role="status" className="text-muted-text text-xs">Loading its report…</p>;
  const sources = report.sources ?? [];
  const verified = sources.filter((source) => !source.legacy);
  const legacy = sources.filter((source) => source.legacy);
  const unverified = report.unverifiedUrls ?? [];
  const notes = report.notes ?? [];
  const limitations = report.limitations ?? [];
  const usage = subagentUsageText(report);
  return <>
    <section aria-label="Recorded by Evie" className="space-y-3">
      <p className="text-muted-text text-xs">{subagentReportFacts(report)}</p>
      {usage && <p className="text-muted-text text-xs">{usage}</p>}
      {verified.length > 0 && <SourceList label="Sources Evie verified">
        {verified.map((source, i) => <li key={i}><SourceLink url={source.url} /><span className="text-muted-text block text-xs">{subagentSourceText(source)}</span></li>)}
      </SourceList>}
      {unverified.length > 0 && <SourceList label="Cited URLs Evie couldn't verify">
        {unverified.map((url, i) => <li key={i}><SourceLink url={url} /></li>)}
      </SourceList>}
      {legacy.length > 0 && <SourceList label="Recorded before Evie checked sources">
        {legacy.map((source, i) => <li key={i}><SourceLink url={source.url} /></li>)}
      </SourceList>}
      {notes.length > 0 && <ListSection label="Notes from Evie" items={notes} />}
    </section>
    {report.text === undefined ? <p className="text-body text-[13px]">This worker produced no report.</p> : <section aria-label="Written by the worker" className="space-y-2">
      <p className="text-muted-text text-xs">Written by the worker</p>
      {report.status === "partial" && <p className="text-amber-ink text-xs">This report is partial: the worker stopped before it finished.</p>}
      <div className="border-hair rounded-md border px-4 py-3"><Markdown text={report.text} streaming={false} inert /></div>
      {report.omittedBytes > 0 && <p className="text-amber-ink text-xs">
        Cut for display: the last {report.omittedBytes.toLocaleString("en-US")} of its {report.totalBytes.toLocaleString("en-US")} bytes are not shown.
      </p>}
    </section>}
    {limitations.length > 0 && <ListSection label="Limitations, written by the worker" items={limitations} />}
  </>;
}

function SourceList({label, children}: {label: string; children: ReactNode}) {
  return <section aria-label={label} className="space-y-1.5">
    <p className="text-muted-text text-xs">{label}</p>
    <ul className="space-y-1.5 text-[13px]">{children}</ul>
  </section>;
}

function SourceLink({url}: {url: string}) {
  return <InertLink href={url} className="text-teal-hover break-all underline underline-offset-2">{url}</InertLink>;
}

function ListSection({label, items}: {label: string; items: string[]}) {
  return <section aria-label={label} className="space-y-1.5">
    <p className="text-muted-text text-xs">{label}</p>
    <ul className="text-body list-disc space-y-1 pl-5 text-[13px] leading-relaxed break-words">
      {items.map((item, i) => <li key={i}>{item}</li>)}
    </ul>
  </section>;
}

function StateLabel({child}: {child: Subagent}) {
  const tone = subagentStateTone(child.state);
  const color = tone === "live" ? "text-teal" : tone === "attention" ? "text-amber-ink" : tone === "done" ? "text-body" : "text-muted-text";
  return <span className={`${color} shrink-0 text-xs font-medium`}>{subagentStateLabel(child.state)}</span>;
}

function BackButton({label, onClick}: {label: string; onClick: () => void}) {
  return <button type="button" onClick={onClick} className="text-muted-text hover:text-body focus-visible:outline-teal flex cursor-pointer items-center gap-1.5 rounded-sm px-5 pt-5 pb-3 text-xs focus-visible:outline-2">
    <span className="rotate-90"><ChevronDown size={12} /></span>{label}
  </button>;
}

// A child's trace, read again while it is unfinished.
function useSubagentTrace(sessionId: string, child: Subagent, api: SubagentAPI) {
  const [value, setValue] = useState<SubagentTrace>();
  const [problem, setProblem] = useState("");
  const live = !subagentFinished(child.state);
  useEffect(() => {
    const controller = new AbortController();
    let timer: number | undefined;
    const tick = () => api.trace(sessionId, child.executionId, controller.signal).then((next) => {
      if (controller.signal.aborted) return;
      setValue(next);
      setProblem("");
      if (live) timer = window.setTimeout(tick, 3000);
    }, (error: unknown) => {
      if (controller.signal.aborted) return;
      setProblem(error instanceof Error ? error.message : "The subagent could not be read.");
      if (live) timer = window.setTimeout(tick, 6000);
    });
    void tick();
    return () => { controller.abort(); window.clearTimeout(timer); };
  }, [api, sessionId, child.executionId, live]);
  return {value, problem};
}

// A child's stored report, read once after it finishes: a finished attempt's
// report never changes, so it is not polled.
function useSubagentReport(sessionId: string, child: Subagent, api: SubagentAPI): {value?: SubagentReport; problem?: string} {
  const [read, setRead] = useState<{executionId: string; value?: SubagentReport; problem?: string}>();
  const finished = subagentFinished(child.state);
  useEffect(() => {
    if (!finished) return;
    const controller = new AbortController();
    api.report(sessionId, child.executionId, controller.signal).then((value) => {
      if (!controller.signal.aborted) setRead({executionId: child.executionId, value});
    }, (error: unknown) => {
      if (!controller.signal.aborted) setRead({executionId: child.executionId, problem: error instanceof Error ? error.message : "The report could not be read."});
    });
    return () => controller.abort();
  }, [api, sessionId, child.executionId, finished]);
  return read?.executionId === child.executionId ? read : {};
}

function useNow(ticking: boolean): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!ticking) return;
    const timer = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [ticking]);
  return now;
}
