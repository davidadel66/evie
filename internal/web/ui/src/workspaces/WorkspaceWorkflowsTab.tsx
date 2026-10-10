import { useCallback, useEffect, useState } from "react";
import type { Workspace } from "../api/contextSessions";
import {
  parseRunInputs, pendingProposal, workflowAPI, workflowNotificationAPI, workflowRunAPI,
  type WorkflowAPI, type WorkflowCard, type WorkflowDefinition, type WorkflowDefinitionView, type WorkflowNotificationAPI, type WorkflowRun,
  type WorkflowRunAPI, type WorkflowScheduleStatus, type WorkflowStepOutline,
} from "../api/workflows";
import { ago } from "../ui/time";
import { WorkflowApprovalCard } from "./WorkspaceWorkflows";
import { WorkflowRunStart } from "./WorkspaceWorkflowRuns";
import { WorkspaceBuiltinWorkflows } from "./WorkspaceBuiltinWorkflows";
import { runStateLabel, runTone, runTrigger, type RunTone } from "./runView";
import {
  lineRange, needsReview, recentRuns, runDots, scheduleText, stepNotes, workflowStatus, yamlLine, type WorkflowStatusView,
} from "./workflowView";

// The Workflows tab (workspace-activity.spec.md story 3): every workflow of
// the Workspace with when it runs, its last runs and its status; one opens
// in a pop-up with its steps, its YAML, its approval, Start run, its
// schedule and its recent runs. Built-in workflows keep their own list.

const dot: Record<RunTone, string> = {amber: "bg-amber", teal: "bg-teal", ok: "bg-ok-gutter", danger: "bg-danger", ghost: "bg-ghost", faint: "bg-faint"};
const statusText: Record<WorkflowStatusView["tone"], string> = {amber: "text-amber", muted: "text-muted-text", danger: "text-danger", faint: "text-faint"};
const kindText: Record<string, string> = {agent: "text-ok", ai: "text-teal", map: "text-teal", interrupt: "text-amber", branch: "text-faint"};
const kindBorder: Record<string, string> = {agent: "border-ok", ai: "border-teal", map: "border-teal", interrupt: "border-amber", branch: "border-faint"};

/** One workflow in the table. */
export type WorkflowRow = { definition: WorkflowDefinition; description?: string; status: WorkflowStatusView; runs: WorkflowRun[] };

const columns = "grid grid-cols-[minmax(0,1fr)_110px_90px] gap-4 sm:grid-cols-[minmax(0,1fr)_150px_110px_120px]";

export function WorkflowTable({rows, onOpen}: {rows: WorkflowRow[]; onOpen: (name: string) => void}) {
  if (rows.length === 0) return <p className="border-hair text-muted-text border-t px-2.5 py-4 text-[12.5px]">No workflows have been proposed for this Workspace.</p>;
  return <>
    <div className={`${columns} text-fainter px-2.5 pb-2 text-[11px]`}><span>Workflow</span><span>Runs when</span><span className="hidden sm:block">Last runs</span><span className="text-right">Status</span></div>
    <ul className="border-hair border-t">
      {rows.map(({definition, description, status, runs}) => <li key={definition.name}>
        <button type="button" onClick={() => onOpen(definition.name)} className={`${columns} border-hair hover:bg-hover w-full items-center border-b px-2.5 py-3 text-left`}>
          <span className="min-w-0">
            <span className="flex items-baseline gap-2">
              <span className="text-ink font-mono text-[12.5px]">{definition.name}</span>
              {definition.current && <span className="text-fainter text-[11px]">v{definition.current.version}</span>}
            </span>
            {description && <span className="text-muted-text mt-[3px] block text-xs">{description}</span>}
          </span>
          <span className="text-muted-text text-xs">{scheduleText(definition.current?.schedule)}</span>
          <span className="hidden items-center gap-[3px] sm:flex" aria-label={runs.length ? `Last ${runs.length} runs` : "No runs yet"}>
            {runs.length === 0 ? <span className="text-fainter text-[11.5px]">None yet</span>
              : runDots(runs).map((tone, index) => <span key={index} className={`h-[7px] w-[7px] rounded-[2px] ${dot[tone]}`} />)}
          </span>
          <span className={`${statusText[status.tone]} text-right text-xs`}>{status.label}</span>
        </button>
      </li>)}
    </ul>
  </>;
}

export type WorkflowView = "steps" | "yaml";

export function WorkflowDialog({definition, view, runs, schedule, card, mode, highlight, busy, problem, now,
  onMode, onStep, onClose, onReview, onApprove, onCloseCard, onStart, onSchedule, onDeactivate, onOpenRun, onAllRuns}: {
  definition: WorkflowDefinition;
  view?: WorkflowDefinitionView;
  runs: WorkflowRun[];
  schedule?: WorkflowScheduleStatus;
  card?: WorkflowCard;
  mode: WorkflowView;
  /** The step whose lines the YAML view highlights. */
  highlight?: WorkflowStepOutline;
  busy: boolean;
  problem?: string;
  now: Date;
  onMode: (mode: WorkflowView) => void;
  onStep: (step: WorkflowStepOutline) => void;
  onClose: () => void;
  onReview: () => void;
  onApprove: () => void;
  onCloseCard: () => void;
  onStart: (inputs: string) => void;
  onSchedule: (action: "pause" | "resume") => void;
  onDeactivate: () => void;
  onOpenRun: (runId: string) => void;
  onAllRuns: () => void;
}) {
  const [confirming, setConfirming] = useState(false);
  const status = workflowStatus(definition, schedule);
  const current = definition.current;
  const lines = view?.source.replace(/\n$/, "").split("\n") ?? [];
  const steps = view?.steps ?? [];
  const tab = (value: WorkflowView, label: string) => <button type="button" aria-pressed={mode === value} onClick={() => onMode(value)}
    className={`${mode === value ? "bg-selected text-ink" : "text-muted-text hover:text-body"} rounded-[5px] px-3 py-1 text-xs`}>{label}</button>;
  return <div className="fixed inset-0 z-40 flex items-center justify-center bg-black/65 p-4 sm:p-8" onClick={onClose}>
    <div role="dialog" aria-modal="true" aria-label={`Workflow ${definition.name}`} onClick={(event) => event.stopPropagation()}
      className="border-hair-bubble bg-inspector flex max-h-full w-full max-w-[920px] flex-col overflow-hidden rounded-[12px] border shadow-[0_24px_64px_rgba(0,0,0,0.5)]">
      <div className="border-hair flex h-12 flex-none items-center gap-2.5 border-b pr-3.5 pl-[22px]">
        <span className="text-faint text-xs">Workflow</span>
        <span className="text-muted-text truncate font-mono text-xs">{definition.name}/workflow.yaml</span>
        <span className="flex-1" />
        <button type="button" onClick={onClose} title="Close (Esc)" aria-label="Close workflow" className="text-muted-text hover:bg-hover hover:text-ink h-7 w-7 rounded-[6px] text-[17px] leading-none">×</button>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto px-7 pt-[22px] pb-[30px]">
        <div className="flex flex-wrap items-baseline gap-2.5">
          <span className="text-ink font-mono text-base font-medium">{definition.name}</span>
          {current && <span className="text-fainter text-xs">v{current.version}</span>}
          <span className={`${statusText[status.tone]} text-xs`}>{status.label}</span>
        </div>
        {view?.description && <p className="text-muted-text mt-1.5 text-[12.5px] leading-normal">{view.description}</p>}
        <p className="text-fainter mt-1 text-xs">{current?.schedule ? `Runs ${scheduleText(current.schedule)}` : "Runs on request"}{schedule?.next && !schedule.paused ? ` · next ${new Date(schedule.next.due).toLocaleString("en-US", {weekday: "short", hour: "numeric", minute: "2-digit"})}` : ""}</p>
        {definition.blocked?.map((reason) => <p key={reason} className="text-danger mt-2 text-xs leading-5">{reason}</p>)}

        {needsReview(definition) && <section aria-label="Approval" className="border-amber-hair-strong bg-amber-bg mt-4 rounded-[8px] border px-3.5 py-3">
          <p className="text-amber-ink text-[12.5px] leading-[1.55]">{definition.status === "proposed"
            ? "This workflow hasn't been approved yet, so it can't run."
            : definition.pending > 0 ? "A new version is waiting for your approval. The current version keeps running until you approve it."
              : "This workflow isn't running. Review its version to approve it again."}</p>
          {card ? <WorkflowApprovalCard card={card} busy={busy} onApprove={onApprove} onClose={onCloseCard} />
            : <button type="button" disabled={busy} onClick={onReview} className="border-amber-hair-strong text-amber-ink hover:bg-amber-hair mt-2.5 rounded-[6px] border px-3 py-[5px] text-[12.5px] font-medium disabled:opacity-40">Review and approve</button>}
        </section>}

        {definition.status === "active" && <div className="mt-4 flex flex-wrap items-start gap-2 text-xs">
          <details className="min-w-[260px] flex-1">
            <summary className="text-teal hover:text-teal-hover cursor-pointer">Start a run</summary>
            <div className="mt-2"><WorkflowRunStart workflows={[definition.name]} busy={busy} onStart={(_, inputs) => onStart(inputs)} /></div>
          </details>
          {current?.schedule && schedule && <button type="button" disabled={busy} onClick={() => onSchedule(schedule.paused ? "resume" : "pause")}
            className="text-muted-text hover:text-body disabled:opacity-40">{schedule.paused ? "Resume schedule" : "Pause schedule"}</button>}
          {current && !current.deactivated && (confirming
            ? <span className="flex items-center gap-2"><span className="text-muted-text">No new run will start.</span>
              <button type="button" disabled={busy} onClick={() => { setConfirming(false); onDeactivate(); }} className="text-danger font-medium disabled:opacity-40">Deactivate version {current.version}</button>
              <button type="button" onClick={() => setConfirming(false)} className="text-muted-text">Keep it</button></span>
            : <button type="button" disabled={busy} onClick={() => setConfirming(true)} className="text-muted-text hover:text-body disabled:opacity-40">Deactivate…</button>)}
        </div>}

        <div className="mt-[22px] mb-3 flex items-center gap-2.5">
          <div className="border-hair-bubble bg-card flex rounded-[7px] border p-0.5">{tab("steps", "Steps")}{tab("yaml", "YAML")}</div>
          <span className="flex-1" />
          <span className="text-fainter text-[11.5px]">{!view ? "Loading…" : mode === "steps" ? `${steps.length} ${steps.length === 1 ? "step" : "steps"} · click one to see its YAML` : `${lines.length} lines`}</span>
        </div>

        {view && mode === "steps" && <ol>
          {steps.map((step, index) => <li key={step.id} className="grid grid-cols-[12px_minmax(0,1fr)] gap-x-3">
            <span className="flex flex-col items-center">
              <span aria-hidden="true" className={`mt-3.5 h-2.5 w-2.5 flex-none rounded-[3px] border-2 ${kindBorder[step.kind] ?? "border-muted-text"}`} />
              {index < steps.length - 1 && <span className="bg-hair-bubble mt-1 w-px flex-1" />}
            </span>
            <button type="button" onClick={() => onStep(step)} title="Show in YAML"
              className="border-hair-bubble bg-card hover:border-teal-deeper mb-2 min-w-0 rounded-[8px] border px-3 py-[9px] text-left">
              <span className="flex items-baseline gap-2">
                <span className="text-ink font-mono text-[12.5px]">{step.id}</span>
                <span className={`${kindText[step.kind] ?? "text-muted-text"} text-[11px]`}>{step.kind}</span>
                <span className="flex-1" />
                <span className="text-fainter font-mono text-[11px]">{lineRange(step)}</span>
              </span>
              {step.description && <span className="text-muted-text mt-[3px] block text-xs leading-normal">{step.description}</span>}
              {stepNotes(step).length > 0 && <span className="mt-[7px] flex flex-wrap gap-1.5">
                {stepNotes(step).map((note) => <span key={note} className="bg-hover text-muted-text rounded-[4px] px-[7px] py-px font-mono text-[11px]">{note}</span>)}
              </span>}
            </button>
          </li>)}
        </ol>}

        {view && mode === "yaml" && <div className="border-hair bg-topbar rounded-[8px] border py-2.5 font-mono text-xs leading-[1.7]">
          {lines.map((raw, index) => {
            const n = index + 1;
            const on = !!highlight && n >= highlight.first_line && n <= highlight.last_line;
            const line = yamlLine(raw);
            return <div key={n} id={`wf-ln-${n}`} className={`${on ? "bg-teal-deep/40" : ""} grid grid-cols-[40px_minmax(0,1fr)]`}>
              <span className={`${on ? "text-teal" : "text-ghost"} pr-3.5 text-right select-none`}>{n}</span>
              <span className="text-body pr-3.5 whitespace-pre-wrap [overflow-wrap:anywhere]">{line.indent}{line.dash}<span className={line.depth === 0 ? "text-teal" : "text-muted-text"}>{line.key}</span>{line.rest}<span className="text-fainter">{line.comment}</span></span>
            </div>;
          })}
        </div>}

        <div className="mt-[26px] mb-1.5 flex items-baseline">
          <h3 className="text-faint text-[11px] font-medium">Recent runs</h3>
          <span className="flex-1" />
          {runs.length > 0 && <button type="button" onClick={onAllRuns} className="text-teal text-xs">All runs ›</button>}
        </div>
        {runs.length === 0 ? <p className="border-hair text-fainter border-t py-2 text-xs">No runs yet.</p>
          : <ul>{runs.map((run) => <li key={run.id}>
            <button type="button" onClick={() => onOpenRun(run.id)} className="border-hair hover:bg-hover flex w-full items-center gap-2.5 border-t px-1 py-2 text-left text-xs">
              <span aria-hidden="true" className={`h-[7px] w-[7px] flex-none rounded-full ${dot[runTone(run)]}`} />
              <span className="text-body font-mono">{run.id.slice(0, 8)}</span>
              <span className="text-muted-text">{runStateLabel[run.state]}</span>
              <span className="flex-1" />
              <span className="text-fainter">{runTrigger(run)} · {ago(run.created_at, now)}</span>
            </button>
          </li>)}</ul>}

        {(definition.versions?.length ?? 0) > 0 && <details className="mt-5">
          <summary className="text-faint cursor-pointer text-[11px] font-medium">Versions</summary>
          <ul className="mt-2 text-xs">{definition.versions!.map((version) => <li key={version.version} className="text-muted-text py-0.5">
            <span className="text-body font-mono">v{version.version}</span> approved {new Date(version.approved_at).toLocaleDateString("en-US", {month: "short", day: "numeric", year: "numeric"})} from the {version.approver.surface}
            {version.deactivated && <span className="text-faint"> · deactivated{version.deactivated.reason ? `: ${version.deactivated.reason}` : ""}</span>}
          </li>)}</ul>
        </details>}
        {problem && <p role="alert" className="text-danger mt-4 text-xs">{problem}</p>}
      </div>
    </div>
  </div>;
}

/** The Workflows tab: loads the Workspace's workflows, their schedules, runs and YAML, and acts on one in the pop-up. */
export function WorkspaceWorkflowsTab({workspace, onOpenRun, onAllRuns, api = workflowAPI, runAPI = workflowRunAPI, statusAPI = workflowNotificationAPI}: {
  workspace: Workspace;
  onOpenRun: (runId: string) => void;
  onAllRuns: (workflow: string) => void;
  api?: WorkflowAPI;
  runAPI?: WorkflowRunAPI;
  statusAPI?: Pick<WorkflowNotificationAPI, "status">;
}) {
  const [definitions, setDefinitions] = useState<WorkflowDefinition[] | null>(null);
  const [schedules, setSchedules] = useState<WorkflowScheduleStatus[]>([]);
  const [runs, setRuns] = useState<WorkflowRun[]>([]);
  const [views, setViews] = useState<Record<string, WorkflowDefinitionView>>({});
  const [open, setOpen] = useState<string>();
  const [detail, setDetail] = useState<WorkflowDefinition>();
  const [card, setCard] = useState<WorkflowCard>();
  const [mode, setMode] = useState<WorkflowView>("steps");
  const [highlight, setHighlight] = useState<WorkflowStepOutline>();
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const [now, setNow] = useState(() => new Date());

  const refresh = useCallback(async () => {
    const [list, status, page] = await Promise.all([api.list(workspace.id), statusAPI.status(workspace.id), runAPI.runs(workspace.id)]);
    setDefinitions(list.workflows);
    setSchedules(status.schedules);
    setRuns(page.runs);
    setNow(new Date());
    return list.workflows;
  }, [api, runAPI, statusAPI, workspace.id]);

  // Each workflow's YAML is read for its description, and again after a
  // change of version.
  const loadViews = useCallback((list: WorkflowDefinition[]) => {
    for (const definition of list) {
      api.source(workspace.id, definition.name).then((value) => setViews((current) => ({...current, [definition.name]: value})), () => undefined);
    }
  }, [api, workspace.id]);

  useEffect(() => {
    refresh().then(loadViews, () => setProblem("Workflows could not be loaded."));
  }, [refresh, loadViews]);

  const show = useCallback((name: string) => {
    setOpen(name); setCard(undefined); setMode("steps"); setHighlight(undefined); setProblem("");
    api.show(workspace.id, name).then(setDetail, () => setProblem("The workflow could not be read."));
  }, [api, workspace.id]);
  const close = useCallback(() => { setOpen(undefined); setDetail(undefined); setCard(undefined); }, []);

  useEffect(() => {
    if (!open) return;
    const onEscape = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      if (event.target instanceof HTMLElement && ["INPUT", "TEXTAREA", "SELECT"].includes(event.target.tagName)) return;
      close();
    };
    window.addEventListener("keydown", onEscape);
    return () => window.removeEventListener("keydown", onEscape);
  }, [open, close]);

  useEffect(() => {
    if (mode === "yaml" && highlight) document.getElementById(`wf-ln-${highlight.first_line}`)?.scrollIntoView({block: "center", behavior: "smooth"});
  }, [mode, highlight]);

  const act = async (action: () => Promise<unknown>) => {
    setBusy(true); setProblem("");
    try {
      await action();
      const list = await refresh();
      loadViews(list);
      if (open) setDetail(await api.show(workspace.id, open));
    } catch (error) {
      setProblem(error instanceof Error ? error.message : "The workflow request failed. Refresh before trying again.");
    } finally { setBusy(false); }
  };

  const definition = detail ?? definitions?.find((d) => d.name === open);
  const rows: WorkflowRow[] = (definitions ?? []).map((d) => ({definition: d, description: views[d.name]?.description,
    status: workflowStatus(d, schedules.find((s) => s.workflow === d.name)), runs: recentRuns(runs, d.name)}));

  return <section aria-label="Workflows" className="min-h-0 flex-1 overflow-y-auto">
    <div className="mx-auto max-w-[1000px] px-6 pt-6 pb-16 sm:px-7">
      {definitions === null ? <p className="text-muted-text text-xs">{problem || "Loading workflows…"}</p> : <WorkflowTable rows={rows} onOpen={show} />}
      <p className="text-fainter mt-4 max-w-[640px] px-2.5 text-xs leading-5">Propose a workflow with <code className="font-mono">evie workflows propose {workspace.id} &lt;folder&gt; --fixtures &lt;file&gt;</code>. Approving a version grants exactly the Standing Authority it shows and starts no run.</p>
      <div className="px-2.5"><WorkspaceBuiltinWorkflows workspace={workspace} /></div>
    </div>
    {open && definition && <WorkflowDialog definition={definition} view={views[open]} runs={recentRuns(runs, open)} schedule={schedules.find((s) => s.workflow === open)}
      card={card} mode={mode} highlight={highlight} busy={busy || workspace.state !== "active"} problem={problem || undefined} now={now}
      onMode={(next) => { setMode(next); if (next === "steps") setHighlight(undefined); }}
      onStep={(step) => { setHighlight(step); setMode("yaml"); }}
      onClose={close}
      onReview={() => void act(async () => {
        const shown = await api.show(workspace.id, definition.name);
        // A waiting proposal, or, for a blocked or inactive definition, the
        // approved version's own proposal, which can be approved again.
        const id = pendingProposal(shown)?.id ?? shown.current?.proposal_id ?? shown.proposals?.[0]?.id;
        if (!id) throw new Error("There is no proposal to review.");
        setCard(await api.diff(id));
      })}
      onApprove={() => void act(async () => { if (card) await api.approve(card); setCard(undefined); })}
      onCloseCard={() => setCard(undefined)}
      onStart={(text) => void act(async () => {
        const inputs = parseRunInputs(text);
        if (!inputs) throw new Error("Inputs must be one JSON object of input names to values.");
        const result = await runAPI.start(workspace.id, definition.name, inputs, true);
        close();
        onOpenRun(result.run.run.id);
      })}
      onSchedule={(action) => void act(() => api.schedule(workspace.id, definition.name, action))}
      onDeactivate={() => void act(() => definition.current ? api.deactivate(workspace.id, definition.name, definition.current.version) : Promise.resolve())}
      onOpenRun={(runId) => { close(); onOpenRun(runId); }}
      onAllRuns={() => { close(); onAllRuns(definition.name); }} />}
  </section>;
}
