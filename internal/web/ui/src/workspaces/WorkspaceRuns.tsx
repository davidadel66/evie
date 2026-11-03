import { useCallback, useEffect, useMemo, useState } from "react";
import type { Workspace } from "../api/contextSessions";
import { workflowRunAPI, type WorkflowCapStatus, type WorkflowInterruption, type WorkflowRun, type WorkflowRunAPI, type WorkflowRunDetail, type WorkflowStepOutline } from "../api/workflows";
import { ago } from "../ui/time";
import { workflowSpendLine } from "./workspaceActivity";
import { WorkflowInterruptionAnswer } from "./WorkflowInterruptionAnswer";
import { ReviewDeck } from "./ReviewDeck";
import { reviewable } from "./reviewLines";
import { ChevronDown } from "../ui/Icon";
import {
  acceptsEmptyAnswer, enumChoices, firstSentence, groupRuns, historyByWorkflow, isFinished, questionList, latestStates, mapProgress, outputValue, versionKey, runDuration, runNote, runOrigin, runStateLabel, runSteps, runSummary, runTask, runTone, summarizeOutputs,
  type RunStep, type RunTone, type StepState, type WorkflowHistory,
} from "./runView";

// The Runs tab (workspace-activity.spec.md story 2): runs grouped by what
// they need from the owner, filtered by workflow, each with a bar of its
// steps; a run opens in a panel beside the list with its question, its
// steps, its outputs, what it spent and where it came from.

const dot: Record<RunTone, string> = {amber: "bg-amber", teal: "bg-teal", ok: "bg-ok", danger: "bg-danger", ghost: "bg-ghost", faint: "bg-faint"};
const text: Record<RunTone, string> = {amber: "text-amber", teal: "text-teal", ok: "text-muted-text", danger: "text-danger", ghost: "text-muted-text", faint: "text-muted-text"};
const segment: Record<StepState, string> = {
  done: "bg-ok-gutter", running: "bg-teal", waiting: "bg-amber", failed: "bg-danger", cancelled: "bg-ghost", pending: "bg-hair-strong", skipped: "border border-hair-strong",
};
const stepWord: Record<StepState, string> = {done: "done", running: "running", waiting: "waiting for you", failed: "failed", cancelled: "cancelled", pending: "not reached", skipped: "skipped"};

/** One run in the list: the run, its steps and its one-line note. */
export type RunRow = { run: WorkflowRun; steps: RunStep[]; note: string; question?: WorkflowInterruption };

export function RunStepBar({steps}: {steps: RunStep[]}) {
  return <div role="img" aria-label={steps.map((step) => `${step.id} ${stepWord[step.state]}`).join(", ")} className="flex gap-0.5">
    {steps.map((step) => <span key={step.id} className={`h-1 min-w-1.5 flex-1 rounded-[2px] ${segment[step.state]}`} />)}
  </div>;
}

/** Today's workflow spend, right-aligned beside the filters: it concerns runs, so it lives here rather than on every tab (strip amendment, 2026-11-02). */
function SpendLine({caps}: {caps: WorkflowCapStatus}) {
  return <span className={`${caps.reached ? "text-danger" : "text-faint"} ml-auto text-[11.5px]`}>{workflowSpendLine(caps)}</span>;
}

export function RunList({rows, workflows, filter, selectedId, now, caps, onFilter, onSelect}: {
  rows: RunRow[];
  workflows: string[];
  filter: string;
  selectedId?: string;
  now: Date;
  /** Today's workflow spend, shown here since it concerns runs (strip amendment, 2026-11-02). */
  caps?: WorkflowCapStatus;
  onFilter: (workflow: string) => void;
  onSelect: (run: WorkflowRun) => void;
}) {
  const byRun = new Map(rows.map((row) => [row.run.id, row]));
  const shown = rows.map((row) => row.run).filter((run) => filter === "" || run.workflow === filter);
  const groups = groupRuns(shown).filter((group) => group.label !== "Earlier");
  const histories = historyByWorkflow(shown);
  const chip = (value: string, label: string, mono: boolean) => {
    const on = filter === value;
    return <button key={value || "all"} type="button" aria-pressed={on} onClick={() => onFilter(value)}
      className={`${on ? "bg-selected border-hair-input text-ink" : "border-hair text-muted-text hover:text-body"} ${mono ? "font-mono" : ""} rounded-full border px-3 py-1 text-xs`}>{label}</button>;
  };
  return <>
    {(workflows.length > 1 || caps) && <div className="mb-5 flex flex-wrap items-center gap-1.5">
      {workflows.length > 1 && [chip("", "All", false), ...workflows.map((name) => chip(name, name, true))]}
      {caps && <SpendLine caps={caps} />}
    </div>}
    {groups.length === 0 && histories.length === 0 && <p className="border-hair text-muted-text border-t px-2.5 py-4 text-[12.5px]">No runs yet.</p>}
    {groups.map((group) => <section key={group.label} aria-label={group.label} className="mb-7">
      <h3 className={`${group.tone === "amber" ? "text-amber-ink" : "text-muted-text"} flex items-baseline gap-2 px-2.5 pb-1.5 text-xs font-semibold`}>
        {group.label}<span className="text-muted-text font-normal">{group.runs.length}</span>
      </h3>
      <ul className="border-hair border-t">
        {group.runs.map((run) => <RunLine key={run.id} row={byRun.get(run.id)!} now={now} selected={selectedId === run.id} onSelect={onSelect} />)}
      </ul>
    </section>)}
    {histories.length > 0 && <section aria-label="History">
      <h3 className="text-muted-text flex items-baseline gap-2 px-2.5 pb-1.5 text-xs font-semibold">History<span className="text-fainter font-normal">{histories.reduce((sum, h) => sum + h.runs.length, 0)} runs</span></h3>
      <div className="border-hair border-t">
        {histories.map((history) => <WorkflowHistoryRows key={history.workflow} history={history} rows={byRun} now={now} selectedId={selectedId}
          open={filter === history.workflow || histories.length === 1 || history.runs.some((run) => run.id === selectedId)} onSelect={onSelect} />)}
      </div>
    </section>}
  </>;
}

/** A waiting or running run on one line: dot, workflow, its task or note, its step bar, when, and Answer when it waits. */
function RunLine({row, now, selected, onSelect}: {row: RunRow; now: Date; selected: boolean; onSelect: (run: WorkflowRun) => void}) {
  const {run, steps, question} = row;
  const tone = runTone(run);
  const waits = tone === "amber";
  return <li>
    <button type="button" onClick={() => onSelect(run)} aria-current={selected ? "true" : undefined}
      className={`${selected ? "bg-hover" : "hover:bg-hover"} border-hair flex w-full items-center gap-3.5 border-b px-2.5 py-[11px] text-left`}>
      <span aria-hidden="true" className={`h-2 w-2 flex-none rounded-full ${dot[tone]}`} />
      <span className="text-ink w-[150px] flex-none truncate font-mono text-[13px]">{run.workflow}</span>
      <span className="sr-only">{runStateLabel[run.state]}</span>
      <span className={`${waits ? "text-body" : "text-muted-text"} min-w-0 flex-1 truncate text-[13px]`}>{waits ? runTask(run, steps, question) : row.note}</span>
      <span className="hidden w-[100px] flex-none sm:block"><RunStepBar steps={steps} /></span>
      <span className="text-muted-text w-[64px] flex-none text-right text-xs">{ago(run.created_at, now)}</span>
      {waits && <span className="border-amber-hair-strong bg-amber-bg text-amber-ink flex-none rounded-[6px] border px-2.5 py-1 text-xs font-semibold">Answer</span>}
    </button>
  </li>;
}

const historyPage = 5;

/** One workflow's finished runs: a summary row with a dot per run that opens to the dated runs, a page at a time. */
function WorkflowHistoryRows({history, rows, now, selectedId, open, onSelect}: {
  history: WorkflowHistory; rows: Map<string, RunRow>; now: Date; selectedId?: string; open: boolean; onSelect: (run: WorkflowRun) => void;
}) {
  const [manual, setManual] = useState<boolean>();
  const [all, setAll] = useState(false);
  const expanded = manual ?? open;
  const latest = history.runs[0];
  const latestTone = runTone(latest);
  const listed = all ? history.runs : history.runs.slice(0, historyPage);
  return <div className="border-hair border-b">
    <button type="button" aria-expanded={expanded} onClick={() => setManual(!expanded)}
      className="hover:bg-hover flex w-full items-center gap-3.5 px-2.5 py-3 text-left">
      <span className={`${expanded ? "rotate-180" : ""} text-faint flex-none transition-transform`}><ChevronDown size={12} /></span>
      <span className="text-ink w-[150px] flex-none truncate font-mono text-[13px]">{history.workflow}</span>
      <span className="text-muted-text min-w-0 flex-1 truncate text-[13px]">
        {latest.state === "failed" ? <><span className="text-danger-ink">failed {ago(latest.created_at, now)}</span>{latest.error ? `, ${latest.error}` : ""}</> : history.summary}
      </span>
      <span role="img" aria-label={`Last ${history.dots.length} runs, oldest first: ${history.dots.map((d) => d.label).join(", ")}`} className="hidden flex-none gap-[3px] sm:flex">
        {history.dots.map((d) => <span key={d.run.id} title={d.label} className={`h-2 w-2 rounded-[2px] ${dot[d.tone]}`} />)}
      </span>
      <span className="text-muted-text w-[64px] flex-none text-right text-xs">{ago(latest.created_at, now)}</span>
      <span className="sr-only">{runStateLabel[latest.state]}</span>
      <span aria-hidden="true" className={`h-2 w-2 flex-none rounded-full ${dot[latestTone]} invisible`} />
    </button>
    {expanded && <ul className="border-hair border-t pb-2 pl-9">
      {listed.map((run) => {
        const tone = runTone(run);
        const row = rows.get(run.id);
        const note = run.state === "failed" ? run.error ?? "Failed" : run.state === "cancelled" ? "Cancelled" : summarizeOutputs(run.outputs);
        return <li key={run.id}>
          <button type="button" onClick={() => onSelect(run)} aria-current={selectedId === run.id ? "true" : undefined}
            className={`${selectedId === run.id ? "bg-hover" : "hover:bg-hover"} ${tone === "danger" ? "bg-danger-bg" : ""} flex w-full items-center gap-3.5 rounded-[6px] px-2.5 py-[7px] text-left`}>
            <span aria-hidden="true" className={`h-2 w-2 flex-none rounded-full ${dot[tone]}`} />
            <span className="text-body w-[64px] flex-none text-[12.5px]">{new Date(run.created_at).toLocaleDateString("en-US", {month: "short", day: "numeric"})}</span>
            <span className="sr-only">{runStateLabel[run.state]}</span>
            <span className={`${tone === "danger" ? "text-danger-ink" : "text-faint"} min-w-0 flex-1 truncate text-[12.5px]`}>{note}</span>
            {row && <span className="hidden w-[100px] flex-none sm:block"><RunStepBar steps={row.steps} /></span>}
            <span className="text-fainter w-[64px] flex-none text-right text-xs">{runDuration(run)}</span>
          </button>
        </li>;
      })}
      {history.runs.length > historyPage && !all && <li><button type="button" onClick={() => setAll(true)} className="text-teal px-2.5 py-[7px] text-xs">Show {history.runs.length - historyPage} more</button></li>}
    </ul>}
  </div>;
}

const timelineDot: Record<StepState, string> = {
  done: "bg-ok border-ok", waiting: "bg-amber border-amber", failed: "bg-danger border-danger", cancelled: "bg-ghost border-ghost",
  running: "border-teal", pending: "border-hair-input", skipped: "border-hair-input border-dashed",
};

/** The run panel's frame: the same width and header whether its run has loaded or not, so the list beside it never changes width. */
function RunPanelFrame({runId, onClose, children}: {runId: string; onClose: () => void; children: React.ReactNode}) {
  return <aside aria-label={`Run ${runId}`} className="border-hair bg-inspector absolute inset-0 z-10 flex min-w-0 flex-col border-l lg:static lg:w-[440px] lg:flex-none">
    <div className="border-hair flex h-12 flex-none items-center gap-2.5 border-b pr-3.5 pl-[22px]">
      <span className="text-faint text-xs">Run</span>
      <span className="text-muted-text truncate font-mono text-xs">{runId.slice(0, 8)}</span>
      <span className="flex-1" />
      <button type="button" onClick={onClose} title="Close (Esc)" aria-label="Close run" className="text-muted-text hover:bg-hover hover:text-ink h-7 w-7 rounded-[6px] text-[17px] leading-none">×</button>
    </div>
    {children}
  </aside>;
}

/** The panel while a newly selected run loads, or when it could not be read. */
export function RunPanelLoading({runId, problem, onClose}: {runId: string; problem?: string; onClose: () => void}) {
  return <RunPanelFrame runId={runId} onClose={onClose}>
    <div className="min-h-0 flex-1 overflow-y-auto px-[22px] pt-5 pb-8">
      {problem ? <p role="alert" className="text-danger text-xs">{problem}</p> : <p role="status" className="text-muted-text text-xs">Loading run…</p>}
    </div>
  </RunPanelFrame>;
}

/** The Runs tab's panel: the run's detail in the panel frame beside the list. */
export function RunPanel({onClose, ...props}: Parameters<typeof RunDetail>[0] & {onClose: () => void}) {
  return <RunPanelFrame runId={props.detail.run.id} onClose={onClose}><RunDetail {...props} /></RunPanelFrame>;
}

/** A run's detail: its question, error, steps, outputs and provenance, with Cancel or Rerun below; shared by the Runs panel and the chat Inspector. */
export function RunDetail({detail, steps, sessionTitle, now, busy, problem, onOpenSession, onAnswer, onCancel, onRerun}: {
  detail: WorkflowRunDetail;
  steps: RunStep[];
  /** The chat that started the run, when it is known. */
  sessionTitle?: string;
  now: Date;
  busy: boolean;
  problem?: string;
  onOpenSession?: (sessionId: string) => void;
  onAnswer: (step: string, answer: Record<string, unknown>) => void;
  onCancel: (reason: string) => void;
  onRerun: (reason: string) => void;
}) {
  const run = detail.run;
  const tone = runTone(run);
  const open = (detail.interruptions ?? []).filter((interruption) => !interruption.resolution);
  const [cancelling, setCancelling] = useState(false);
  const [reason, setReason] = useState("");
  // Which step shows its description: undefined means the live one, null none.
  const [openStep, setOpenStep] = useState<string | null | undefined>(undefined);
  const unfinished = !isFinished(run);
  const outputs = run.state === "succeeded" ? Object.entries(run.outputs ?? {}) : [];
  const maps = mapProgress(detail);
  const footer = (unfinished || run.state === "succeeded") && <div className="border-hair flex flex-none flex-wrap items-center gap-2 border-t px-[22px] py-3">
      {unfinished && !cancelling && <>
        <span className="text-faint flex-1 text-xs">Cancelling stops the steps that have not started.</span>
        <button type="button" disabled={busy} onClick={() => setCancelling(true)} className="border-hair-input text-body hover:bg-hover rounded-[6px] border px-3.5 py-1.5 text-[12.5px] font-semibold disabled:opacity-40">Cancel run</button>
      </>}
      {(cancelling || run.state === "succeeded") && <form className="flex w-full items-center gap-2" onSubmit={(event) => {
        event.preventDefault();
        if (cancelling) onCancel(reason); else if (reason.trim()) onRerun(reason.trim());
      }}>
        <input value={reason} onChange={(event) => setReason(event.target.value)} maxLength={500}
          placeholder={cancelling ? "Why (optional)" : "Why rerun it? (recorded)"} aria-label={cancelling ? "Reason to cancel" : "Reason to rerun"}
          className="border-hair-input bg-code text-ink placeholder:text-fainter min-w-0 flex-1 rounded-[6px] border px-2.5 py-1.5 text-[12.5px] outline-none" />
        {cancelling && <button type="button" onClick={() => { setCancelling(false); setReason(""); }} className="text-muted-text px-2 text-xs">Keep running</button>}
        <button type="submit" disabled={busy || (!cancelling && !reason.trim())}
          className={`${cancelling ? "bg-danger text-app" : "bg-teal text-primary-foreground"} rounded-[6px] px-3.5 py-1.5 text-[12.5px] font-semibold disabled:opacity-40`}>{cancelling ? "Cancel run" : "Rerun"}</button>
      </form>}
    </div>;
  return <div className="flex min-h-0 flex-1 flex-col">
    <div className="min-h-0 flex-1 overflow-y-auto px-[22px] pt-5 pb-8">
      {/* Run detail amendment (2026-11-02): the name and state, one muted line
          of facts, then the question with its answer before anything else. */}
      <div className="flex flex-wrap items-center gap-2.5">
        <span className="text-ink font-mono text-[15px] font-medium">{run.workflow}</span>
        <span className={`${text[tone]} inline-flex items-center gap-1.5 text-xs`}><span aria-hidden="true" className={`h-[7px] w-[7px] rounded-full ${dot[tone]}`} />{runStateLabel[run.state]}</span>
      </div>
      <p className="text-faint mt-1.5 flex flex-wrap gap-x-1.5 text-xs">
        <span>{runOrigin(run)}</span>
        {run.surface === "chat" && (run.session_id && onOpenSession
          ? <button type="button" onClick={() => onOpenSession(run.session_id!)} className="text-teal underline underline-offset-2">{sessionTitle || "its chat"}</button>
          : <span>a chat</span>)}
        <span>· v{run.version} · {runSummary(detail, steps, now)}</span>
      </p>
      {open.map((interruption) => <WaitingBox key={interruption.seq} run={run} steps={steps} interruption={interruption} busy={busy} onAnswer={onAnswer} />)}
      {run.state === "failed" && run.error && <p role="alert" className="border-danger-hair bg-danger-bg text-danger-ink-soft mt-[18px] rounded-[8px] border px-3.5 py-3 text-[12.5px] leading-[1.55]">{run.error}</p>}

      <h3 className="text-faint mt-7 mb-2 text-[11px] font-medium tracking-wide uppercase">Steps</h3>
      <ol>
        {steps.map((step) => {
          const live = step.state === "waiting" || step.state === "running" || step.state === "failed";
          const shown = openStep === step.id || (openStep === undefined && live);
          const fact = maps[step.id] ?? (step.state === "done" ? "" : stepWord[step.state]);
          return <li key={step.id}>
            <button type="button" aria-expanded={shown} onClick={() => setOpenStep(shown ? null : step.id)} className="hover:bg-hover -mx-2 flex w-[calc(100%_+_16px)] items-center gap-3 rounded-[6px] px-2 py-[6px] text-left">
              <span aria-hidden="true" className={`h-2 w-2 flex-none rounded-full border-2 ${timelineDot[step.state]}`} />
              <span className={`${step.state === "pending" || step.state === "skipped" ? "text-faint" : "text-ink"} w-[88px] flex-none truncate font-mono text-[12.5px]`}>{step.id}</span>
              <span className="text-fainter min-w-0 flex-1 truncate text-[11.5px]">{step.kind}{fact ? <span className={step.state === "waiting" ? "text-amber-ink" : step.state === "failed" ? "text-danger-ink" : ""}> · {fact}</span> : null}</span>
              {step.state === "done" && <span className="sr-only">{stepWord[step.state]}</span>}
            </button>
            {shown && step.description && <p className="text-muted-text mt-0.5 mb-1.5 pl-5 text-xs leading-normal">{step.description}</p>}
          </li>;
        })}
      </ol>

      {outputs.length > 0 && <>
        <h3 className="text-faint mt-3 mb-2.5 text-[11px] font-medium">Outputs</h3>
        <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1 font-mono text-xs">
          {outputs.map(([key, value]) => <div key={key} className="contents"><dt className="text-faint">{key}</dt><dd className="text-body break-words">{outputValue(value)}</dd></div>)}
        </dl>
      </>}

      <details className="mt-[22px]">
        <summary className="text-faint cursor-pointer text-[11px] font-medium">Provenance</summary>
        <dl className="mt-2.5 grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1 text-xs">
          {([["Run", run.id], ["Run key", run.run_key], ["Executable", run.executable_hash], ["Commit", run.commit], ["Pinned to", `Workspace revision ${run.workspace_revision}`]] as const)
            .map(([label, value]) => <div key={label} className="contents"><dt className="text-faint">{label}</dt><dd className="text-muted-text font-mono [overflow-wrap:anywhere]">{value}</dd></div>)}
        </dl>
      </details>
      {problem && <p role="alert" className="text-danger mt-4 text-xs">{problem}</p>}
    </div>
    {footer}
  </div>;
}

function WaitingBox({run, steps, interruption, busy, onAnswer}: {run: WorkflowRun; steps: RunStep[]; interruption: WorkflowInterruption; busy: boolean; onAnswer: (step: string, answer: Record<string, unknown>) => void}) {
  const question = interruption.kind === "question";
  const choices = question ? enumChoices(interruption.response_schema) : undefined;
  const acceptsEmpty = question && !choices && acceptsEmptyAnswer(interruption.response_schema);
  const list = question ? questionList(interruption) : undefined;
  // A review question's lines come as a deck of cards; any other list shows as its input.
  const review = acceptsEmpty ? reviewable(interruption) : undefined;
  const [showInput, setShowInput] = useState(false);
  const [writing, setWriting] = useState(false);
  const sinceText = new Date(interruption.opened_at).toLocaleTimeString("en-US", {hour: "numeric", minute: "2-digit"});
  return <section aria-label={question ? `steps.${interruption.step} needs your answer` : `steps.${interruption.step}'s effect needs settling`} className="border-amber-hair-strong bg-amber-bg mt-[18px] rounded-[10px] border px-4 py-3.5">
    <h3 className="text-ink text-[15px] leading-[1.35] font-semibold">{question ? runTask(run, steps, interruption) : `Did steps.${interruption.step}'s effect happen?`}</h3>
    {question && !writing && <p className="text-body-dim mt-1.5 text-[12.5px] leading-[1.5]">{firstSentence(interruption.message)}</p>}
    {choices ? <div className="mt-3 flex flex-wrap gap-2">{choices.values.map((value) => <button key={value} type="button" disabled={busy}
      onClick={() => onAnswer(interruption.step, {[choices.property]: value})}
      className="border-amber-hair-strong text-amber-ink hover:bg-amber-hair rounded-[6px] border px-3 py-[5px] text-[12.5px] font-medium disabled:opacity-40">{value}</button>)}</div>
    : acceptsEmpty && !writing ? <div className="mt-3.5 flex flex-wrap items-center gap-2.5">
      <button type="button" disabled={busy} onClick={() => onAnswer(interruption.step, {})} className="bg-teal text-primary-foreground rounded-[7px] px-3.5 py-2 text-[13px] font-semibold disabled:opacity-40">Accept as proposed</button>
      {list && <button type="button" aria-expanded={showInput} onClick={() => setShowInput(!showInput)} className="border-hair-input text-body hover:bg-hover rounded-[7px] border px-3 py-[7px] text-[12.5px]">{review ? (showInput ? "Hide" : "Review") : (showInput ? "Hide" : "Open")} the {list.count} {list.key}</button>}
      <button type="button" onClick={() => setWriting(true)} className="text-muted-text hover:text-body ml-auto text-xs">Write an answer</button>
    </div>
    : <div className="text-xs"><WorkflowInterruptionAnswer interruption={interruption} busy={busy} onAnswer={onAnswer} /></div>}
    {acceptsEmpty && !writing && showInput && (review
      ? <ReviewDeck key={interruption.seq} lines={review.lines} busy={busy} onAnswer={(answer) => onAnswer(interruption.step, answer)} onClose={() => setShowInput(false)} />
      : <pre className="bg-code text-body mt-3 max-h-80 overflow-auto rounded-[6px] p-3 font-mono text-[11.5px] leading-relaxed whitespace-pre-wrap">{JSON.stringify(interruption.request.input ?? interruption.request, null, 2)}</pre>)}
    <p className="text-muted-text mt-3 text-[11.5px]">Waiting since {sinceText} · You can also answer in any chat in this workspace.</p>
  </section>;
}

/** The Runs tab: loads the Workspace's runs, its open questions and each run version's steps, and keeps them current. */
export function WorkspaceRuns({workspace, selectedId, filter = "", sessionTitles = {}, caps, onSelect, onFilter, onOpenSession, onChanged, api = workflowRunAPI}: {
  workspace: Workspace;
  selectedId?: string;
  /** Today's workflow spend for the line beside the filters. */
  caps?: WorkflowCapStatus;
  /** The workflow the list is narrowed to; "" for all. */
  filter?: string;
  sessionTitles?: Record<string, string>;
  onSelect: (runId: string | undefined) => void;
  onFilter?: (workflow: string) => void;
  onOpenSession?: (sessionId: string) => void;
  /** A run changed because of the owner (answered, cancelled, rerun). */
  onChanged?: () => void;
  api?: WorkflowRunAPI;
}) {
  const [runs, setRuns] = useState<WorkflowRun[] | null>(null);
  const [questions, setQuestions] = useState<WorkflowInterruption[]>([]);
  const [outlines, setOutlines] = useState<Record<string, WorkflowStepOutline[]>>({});
  const [detail, setDetail] = useState<WorkflowRunDetail | null>(null);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const [now, setNow] = useState(() => new Date());

  const refresh = useCallback(async () => {
    const [page, open] = await Promise.all([api.runs(workspace.id), api.interruptions(workspace.id)]);
    setRuns(page.runs);
    setQuestions(open.interruptions);
    setNow(new Date());
  }, [api, workspace.id]);

  useEffect(() => {
    let live = true;
    const load = () => refresh().catch(() => { if (live) setProblem("Workflow runs could not be loaded."); });
    void load();
    const timer = window.setInterval(load, 15_000);
    return () => { live = false; window.clearInterval(timer); };
  }, [refresh]);

  // Each version's steps are read once, through any run pinned to it.
  useEffect(() => {
    if (!runs) return;
    const wanted = new Map<string, string>();
    for (const run of runs) if (!(versionKey(run) in outlines) && !wanted.has(versionKey(run))) wanted.set(versionKey(run), run.id);
    for (const [key, runId] of wanted) {
      api.definition(runId).then((view) => setOutlines((current) => ({...current, [key]: view.steps})), () => setOutlines((current) => ({...current, [key]: []})));
    }
  }, [api, runs, outlines]);

  useEffect(() => {
    if (!selectedId) { setDetail(null); return; }
    let live = true;
    api.inspect(selectedId).then((value) => { if (live) setDetail(value); }, () => { if (live) setProblem("The run could not be read."); });
    return () => { live = false; };
  }, [api, selectedId, runs]);

  useEffect(() => {
    if (!selectedId) return;
    const onEscape = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || document.querySelector("dialog[open]")) return;
      if (event.target instanceof HTMLElement && ["INPUT", "TEXTAREA"].includes(event.target.tagName)) return;
      onSelect(undefined);
    };
    window.addEventListener("keydown", onEscape);
    return () => window.removeEventListener("keydown", onEscape);
  }, [selectedId, onSelect]);

  const rows = useMemo(() => (runs ?? []).map((run) => {
    const steps = runSteps(run, outlines[versionKey(run)] ?? []);
    const question = questions.find((q) => q.run_id === run.id);
    return {run, steps, note: runNote(run, steps, question), question};
  }), [runs, outlines, questions]);
  const workflows = useMemo(() => [...new Set((runs ?? []).map((run) => run.workflow))].sort(), [runs]);

  const act = async (action: () => Promise<unknown>) => {
    setBusy(true); setProblem("");
    try { await action(); await refresh(); onChanged?.(); }
    catch (error) { setProblem(error instanceof Error ? error.message : "The workflow request failed. Refresh before trying again."); }
    finally { setBusy(false); }
  };
  const selectedSteps = detail ? runSteps(detail.run, outlines[versionKey(detail.run)] ?? [], latestStates(detail)) : [];

  return <div className="relative flex min-h-0 min-w-0 flex-1">
    <section aria-label="Workflow runs" className="min-w-0 flex-1 overflow-y-auto">
      <div className="mx-auto max-w-[1000px] px-6 pt-6 pb-16 sm:px-7">
        {runs === null ? <><div className="mb-5 flex items-center">{caps && <SpendLine caps={caps} />}</div><p className="text-muted-text text-xs">{problem || "Loading runs…"}</p></>
          : <RunList rows={rows} workflows={workflows} filter={filter} selectedId={selectedId} now={now} caps={caps} onFilter={(workflow) => onFilter?.(workflow)} onSelect={(run) => { setProblem(""); onSelect(run.id); }} />}
        {problem && runs !== null && !detail && <p role="alert" className="text-danger mt-2 text-xs">{problem}</p>}
      </div>
    </section>
    {selectedId && !(detail && detail.run.id === selectedId) && <RunPanelLoading runId={selectedId} problem={problem || undefined} onClose={() => onSelect(undefined)} />}
    {detail && detail.run.id === selectedId && <RunPanel key={detail.run.id} detail={detail} steps={selectedSteps} now={now} busy={busy} problem={problem}
      sessionTitle={detail.run.session_id ? sessionTitles[detail.run.session_id] : undefined}
      onClose={() => onSelect(undefined)} onOpenSession={onOpenSession}
      onAnswer={(step, answer) => void act(() => api.answer(detail.run.id, step, answer))}
      onCancel={(reason) => void act(() => api.cancel(detail.run.id, reason))}
      onRerun={(reason) => void act(async () => {
        const result = await api.rerun(workspace.id, detail.run.workflow, detail.run.id, reason);
        onSelect(result.run.run.id);
      })} />}
  </div>;
}
