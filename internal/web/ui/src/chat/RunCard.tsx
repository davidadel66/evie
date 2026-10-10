import type { WorkflowRun } from "../api/workflows";
import { duration } from "../ui/time";
import { isFinished, runNote, runStateLabel, runTone, stepProgress, type RunStep, type RunTone } from "../workspaces/runView";
import type { ChatRunStart } from "./chatRuns";

// A run Evie started in this chat (workspace-activity.spec.md story 4): a
// live card under her reply, opening the run in the chat Inspector.

const dot: Record<RunTone, string> = {amber: "bg-amber", teal: "bg-teal", ok: "bg-ok", danger: "bg-danger", ghost: "bg-ghost", faint: "bg-faint"};
const text: Record<RunTone, string> = {amber: "text-amber", teal: "text-teal", ok: "text-ok", danger: "text-danger", ghost: "text-muted-text", faint: "text-muted-text"};

export function RunCard({start, run, steps, onOpen}: {
  start: ChatRunStart;
  /** The run as the runs list last read it; until then, what workflow_start reported. */
  run?: WorkflowRun;
  steps: RunStep[];
  onOpen?: (trigger: HTMLButtonElement) => void;
}) {
  const current = run ?? ({state: start.state} as WorkflowRun);
  const tone = runTone(current);
  const meta = !run ? "" : isFinished(run) && run.finished_at
    ? duration(new Date(run.finished_at).getTime() - new Date(run.started_at ?? run.created_at).getTime())
    : steps.length > 0 ? stepProgress(run, steps) : "";
  return <button type="button" onClick={(event) => onOpen?.(event.currentTarget)} aria-label={`Run of ${start.workflow}, ${runStateLabel[current.state]}`}
    className="border-hair-bubble bg-card hover:bg-hover focus-visible:ring-teal flex w-full max-w-[560px] flex-none items-center gap-2.5 rounded-[8px] border px-3 py-2.5 text-left focus-visible:ring-1 focus-visible:outline-none">
    <span aria-hidden="true" className={`h-2 w-2 flex-none rounded-full ${dot[tone]}`} />
    <span className="text-faint flex-none text-[11px]">Run</span>
    <span className="text-ink flex-none font-mono text-[12.5px]">{start.workflow}</span>
    <span className="text-muted-text min-w-0 flex-1 truncate text-[12.5px]">{run ? runNote(run, steps) : ""}</span>
    <span className={`${text[tone]} flex-none text-xs`}>{runStateLabel[current.state]}</span>
    {meta && <span className="text-fainter flex-none text-[11.5px]">{meta}</span>}
    <span aria-hidden="true" className="text-faint flex-none">›</span>
  </button>;
}
