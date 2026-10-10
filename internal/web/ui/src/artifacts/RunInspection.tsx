import { useCallback, useEffect, useState } from "react";
import { workflowRunAPI, type WorkflowRunAPI, type WorkflowRunDetail, type WorkflowStepOutline } from "../api/workflows";
import { RunDetail } from "../workspaces/WorkspaceRuns";
import { isFinished, latestStates, runSteps } from "../workspaces/runView";

// A run in the chat Inspector (workspace-activity.spec.md story 4): the
// same detail as the Runs tab's panel, kept current while the run is
// unfinished. A rerun shows the new run in its place.
export function RunInspection({runId, api = workflowRunAPI}: {runId: string; api?: WorkflowRunAPI}) {
  const [current, setCurrent] = useState(runId);
  const [detail, setDetail] = useState<WorkflowRunDetail>();
  const [outline, setOutline] = useState<WorkflowStepOutline[]>([]);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const [now, setNow] = useState(() => new Date());

  const load = useCallback(() => api.inspect(current).then((value) => { setDetail(value); setNow(new Date()); return value; }), [api, current]);

  useEffect(() => {
    let live = true;
    let timer: number | undefined;
    const tick = () => load().then((value) => {
      if (live && !isFinished(value.run)) timer = window.setTimeout(tick, 10_000);
    }, () => { if (live) setProblem("The run could not be read."); });
    void tick();
    api.definition(current).then((view) => { if (live) setOutline(view.steps); }, () => undefined);
    return () => { live = false; window.clearTimeout(timer); };
  }, [api, current, load]);

  const act = async (action: () => Promise<unknown>) => {
    setBusy(true); setProblem("");
    try { await action(); await load(); }
    catch (error) { setProblem(error instanceof Error ? error.message : "The workflow request failed. Refresh before trying again."); }
    finally { setBusy(false); }
  };

  if (!detail || detail.run.id !== current) {
    return <div className="px-[22px] pt-5">{problem ? <p role="alert" className="text-danger text-xs">{problem}</p> : <p role="status" className="text-muted-text text-xs">Loading run…</p>}</div>;
  }
  return <RunDetail detail={detail} steps={runSteps(detail.run, outline, latestStates(detail))} now={now} busy={busy} problem={problem || undefined}
    onAnswer={(step, answer) => void act(() => api.answer(detail.run.id, step, answer))}
    onCancel={(reason) => void act(() => api.cancel(detail.run.id, reason))}
    onRerun={(reason) => void act(async () => {
      const result = await api.rerun(detail.run.workspace_id, detail.run.workflow, detail.run.id, reason);
      setCurrent(result.run.run.id);
    })} />;
}
