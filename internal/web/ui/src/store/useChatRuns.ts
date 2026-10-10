import { useEffect, useState } from "react";
import { workflowRunAPI, type WorkflowRun, type WorkflowRunAPI, type WorkflowStepOutline } from "../api/workflows";
import { isFinished, versionKey } from "../workspaces/runView";

const pollMs = 10_000;

/** useChatRuns keeps the runs a chat started current for their cards: one
 *  read of the Workspace's runs list for all of them, again every 10
 *  seconds while any of them is unfinished and whenever refreshKey changes,
 *  plus each run version's step outline once. A run the list does not show
 *  keeps the state its workflow_start call reported. */
export function useChatRuns(workspaceId: string | undefined, runIds: string[], refreshKey?: unknown, api: Pick<WorkflowRunAPI, "runs" | "definition"> = workflowRunAPI) {
  const [runs, setRuns] = useState<Record<string, WorkflowRun>>({});
  const [outlines, setOutlines] = useState<Record<string, WorkflowStepOutline[]>>({});
  const ids = runIds.join(",");
  useEffect(() => {
    if (!workspaceId || ids === "") return;
    const wanted = ids.split(",");
    let live = true;
    let timer: number | undefined;
    const load = () => {
      api.runs(workspaceId).then((page) => {
        if (!live) return;
        const found = Object.fromEntries(page.runs.filter((run) => wanted.includes(run.id)).map((run) => [run.id, run]));
        setRuns(found);
        if (Object.values(found).some((run) => !isFinished(run))) timer = window.setTimeout(load, pollMs);
      }, () => undefined);
    };
    load();
    return () => { live = false; window.clearTimeout(timer); };
  }, [api, workspaceId, ids, refreshKey]);
  useEffect(() => {
    const seen = new Set<string>();
    for (const run of Object.values(runs)) {
      const key = versionKey(run);
      if (key in outlines || seen.has(key)) continue;
      seen.add(key);
      api.definition(run.id).then((view) => setOutlines((current) => ({...current, [key]: view.steps})), () => setOutlines((current) => ({...current, [key]: []})));
    }
  }, [api, runs, outlines]);
  return {runs, outlineOf: (run: WorkflowRun) => outlines[versionKey(run)] ?? []};
}
