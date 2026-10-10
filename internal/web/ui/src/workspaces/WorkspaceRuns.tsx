import { useCallback, useEffect, useMemo, useState } from "react";
import type { Workspace } from "../api/contextSessions";
import { workflowRunAPI, type WorkflowInterruption, type WorkflowRun, type WorkflowRunAPI, type WorkflowRunDetail, type WorkflowStepOutline } from "../api/workflows";
import { ago } from "../ui/time";
import { WorkflowInterruptionAnswer } from "./WorkflowInterruptionAnswer";
import {
  enumChoices, groupRuns, isFinished, latestStates, mapProgress, outputValue, versionKey, runNote, runOrigin, runStateLabel, runSteps, runSummary, runTone, runTrigger,
  type RunStep, type RunTone, type StepState,
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
export type RunRow = { run: WorkflowRun; steps: RunStep[]; note: string };

export function RunStepBar({steps}: {steps: RunStep[]}) {
  return <div role="img" aria-label={steps.map((step) => `${step.id} ${stepWord[step.state]}`).join(", ")} className="flex gap-0.5">
    {steps.map((step) => <span key={step.id} className={`h-1 min-w-1.5 flex-1 rounded-[2px] ${segment[step.state]}`} />)}
  </div>;
}

export function RunList({rows, workflows, filter, selectedId, now, onFilter, onSelect}: {
  rows: RunRow[];
  workflows: string[];
  filter: string;
  selectedId?: string;
  now: Date;
  onFilter: (workflow: string) => void;
  onSelect: (run: WorkflowRun) => void;
}) {
  const byRun = new Map(rows.map((row) => [row.run.id, row]));
  const groups = groupRuns(rows.map((row) => row.run).filter((run) => filter === "" || run.workflow === filter));
  const chip = (value: string, label: string, mono: boolean) => {
    const on = filter === value;
    return <button key={value || "all"} type="button" aria-pressed={on} onClick={() => onFilter(value)}
      className={`${on ? "bg-selected border-hair-input text-ink" : "border-hair text-muted-text hover:text-body"} ${mono ? "font-mono" : ""} rounded-[6px] border px-2.5 py-1 text-xs`}>{label}</button>;
  };
  const columns = "grid grid-cols-[minmax(0,1fr)_72px_80px_64px] gap-4 sm:grid-cols-[minmax(0,1fr)_72px_96px_72px]";
  return <>
    {workflows.length > 1 && <div className="mb-5 flex flex-wrap gap-1.5">{[chip("", "All workflows", false), ...workflows.map((name) => chip(name, name, true))]}</div>}
    <div className={`${columns} text-fainter px-2.5 pb-2 text-[11px]`}><span>Workflow</span><span>Steps</span><span>Trigger</span><span className="text-right">Started</span></div>
    {groups.length === 0 && <p className="border-hair text-muted-text border-t px-2.5 py-4 text-[12.5px]">No runs yet.</p>}
    {groups.map((group) => <section key={group.label} aria-label={group.label} className="mb-6">
      <h3 className={`${group.tone === "amber" ? "text-amber" : group.tone === "muted" ? "text-muted-text" : "text-faint"} flex items-baseline gap-2 px-2.5 pb-2 text-[11px] font-medium`}>
        {group.label}<span className="text-fainter font-normal">{group.runs.length}</span>
      </h3>
      <ul className="border-hair border-t">
        {group.runs.map((run) => {
          const row = byRun.get(run.id)!;
          const tone = runTone(run);
          return <li key={run.id}>
            <button type="button" onClick={() => onSelect(run)} aria-current={selectedId === run.id ? "true" : undefined}
              className={`${columns} ${selectedId === run.id ? "bg-hover" : ""} border-hair hover:bg-hover w-full items-center border-b px-2.5 py-[11px] text-left`}>
              <span className="flex min-w-0 flex-col gap-[3px]">
                <span className="flex items-center gap-2">
                  <span aria-hidden="true" className={`h-2 w-2 flex-none rounded-full ${dot[tone]}`} />
                  <span className="text-ink font-mono text-[12.5px]">{run.workflow}</span>
                  <span className="text-fainter text-[11px]">v{run.version}</span>
                  <span className="sr-only">{runStateLabel[run.state]}</span>
                </span>
                <span className={`${tone === "amber" || tone === "danger" ? text[tone] : "text-muted-text"} truncate pl-4 text-xs`}>{row.note}</span>
              </span>
              <RunStepBar steps={row.steps} />
              <span className="text-muted-text truncate text-xs">{runTrigger(run)}</span>
              <span className="text-faint text-right text-[11.5px]">{ago(run.created_at, now)}</span>
            </button>
          </li>;
        })}
      </ul>
    </section>)}
  </>;
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
      <div className="flex flex-wrap items-center gap-2.5">
        <span className="text-ink font-mono text-base font-medium">{run.workflow}</span>
        <span className="text-fainter text-xs">v{run.version}</span>
        <span className={`${text[tone]} inline-flex items-center gap-1.5 text-xs`}><span aria-hidden="true" className={`h-[7px] w-[7px] rounded-full ${dot[tone]}`} />{runStateLabel[run.state]}</span>
      </div>
      <p className="text-muted-text mt-2 flex flex-wrap gap-1.5 text-[12.5px]">
        <span>{runOrigin(run)}</span>
        {run.surface === "chat" && (run.session_id && onOpenSession
          ? <button type="button" onClick={() => onOpenSession(run.session_id!)} className="text-teal underline underline-offset-2">{sessionTitle || "its chat"}</button>
          : <span>a chat</span>)}
      </p>
      <p className="text-fainter mt-[3px] text-xs">{runSummary(detail, steps, now)}</p>

      {open.map((interruption) => <WaitingBox key={interruption.seq} interruption={interruption} busy={busy} onAnswer={onAnswer} />)}
      {run.state === "failed" && run.error && <p role="alert" className="border-danger-hair bg-danger-bg text-danger-ink-soft mt-[18px] rounded-[8px] border px-3.5 py-3 text-[12.5px] leading-[1.55]">{run.error}</p>}

      <h3 className="text-faint mt-6 mb-3 text-[11px] font-medium">Steps</h3>
      <ol>
        {steps.map((step, index) => <li key={step.id} className="grid grid-cols-[12px_minmax(0,1fr)] gap-x-3">
          <span className="flex flex-col items-center">
            <span aria-hidden="true" className={`mt-1 h-2.5 w-2.5 flex-none rounded-full border-2 ${timelineDot[step.state]}`} />
            {index < steps.length - 1 && <span className="bg-hair-strong my-1 w-px flex-1" />}
          </span>
          <span className="min-w-0 pb-4">
            <span className="flex flex-wrap items-baseline gap-2">
              <span className={`${step.state === "pending" || step.state === "skipped" ? "text-faint" : "text-ink"} font-mono text-[12.5px]`}>{step.id}</span>
              <span className="text-fainter text-[11px]">{step.kind}</span>
              <span className={`${step.state === "waiting" ? "text-amber" : step.state === "failed" ? "text-danger" : "text-fainter"} text-[11px]`}>{stepWord[step.state]}</span>
            </span>
            {step.description && <span className="text-muted-text mt-0.5 block text-xs leading-normal">{step.description}</span>}
            {maps[step.id] && <span className="text-body mt-0.5 block text-xs">{maps[step.id]}</span>}
          </span>
        </li>)}
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

function WaitingBox({interruption, busy, onAnswer}: {interruption: WorkflowInterruption; busy: boolean; onAnswer: (step: string, answer: Record<string, unknown>) => void}) {
  const choices = interruption.kind === "question" ? enumChoices(interruption.response_schema) : undefined;
  return <section aria-label={`steps.${interruption.step} needs your answer`} className="border-amber-hair-strong bg-amber-bg mt-[18px] rounded-[8px] border p-3.5">
    <h3 className="text-amber text-[11px] font-medium">{interruption.kind === "question" ? `steps.${interruption.step} needs your answer` : `steps.${interruption.step}'s effect needs settling`}</h3>
    {choices ? <>
      <p className="text-amber-ink mt-1.5 mb-3 text-[13px] leading-[1.55]">{interruption.message}</p>
      <div className="flex flex-wrap gap-2">{choices.values.map((value) => <button key={value} type="button" disabled={busy}
        onClick={() => onAnswer(interruption.step, {[choices.property]: value})}
        className="border-amber-hair-strong text-amber-ink hover:bg-amber-hair rounded-[6px] border px-3 py-[5px] text-[12.5px] font-medium disabled:opacity-40">{value}</button>)}</div>
    </> : <div className="text-xs"><WorkflowInterruptionAnswer interruption={interruption} busy={busy} onAnswer={onAnswer} /></div>}
    <p className="text-muted-text mt-2.5 text-[11.5px]">You can also answer in any chat in this workspace.</p>
  </section>;
}

/** The Runs tab: loads the Workspace's runs, its open questions and each run version's steps, and keeps them current. */
export function WorkspaceRuns({workspace, selectedId, filter = "", sessionTitles = {}, onSelect, onFilter, onOpenSession, onChanged, api = workflowRunAPI}: {
  workspace: Workspace;
  selectedId?: string;
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
    return {run, steps, note: runNote(run, steps, questions.find((q) => q.run_id === run.id))};
  }), [runs, outlines, questions]);
  const workflows = useMemo(() => [...new Set((runs ?? []).map((run) => run.workflow))].sort(), [runs]);

  const act = async (action: () => Promise<unknown>) => {
    setBusy(true); setProblem("");
    try { await action(); await refresh(); onChanged?.(); }
    catch (error) { setProblem(error instanceof Error ? error.message : "The workflow request failed. Refresh before trying again."); }
    finally { setBusy(false); }
  };
  const selectedSteps = detail ? runSteps(detail.run, outlines[versionKey(detail.run)] ?? [], latestStates(detail)) : [];

  return <div className="relative flex min-h-0 flex-1">
    <section aria-label="Workflow runs" className="min-w-0 flex-1 overflow-y-auto">
      <div className="mx-auto max-w-[1000px] px-6 pt-6 pb-16 sm:px-7">
        {runs === null ? <p className="text-muted-text text-xs">{problem || "Loading runs…"}</p>
          : <RunList rows={rows} workflows={workflows} filter={filter} selectedId={selectedId} now={now} onFilter={(workflow) => onFilter?.(workflow)} onSelect={(run) => { setProblem(""); onSelect(run.id); }} />}
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
