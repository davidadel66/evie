import { useEffect, useState } from "react";
import { workflowNotificationAPI, workflowRunAPI, type WorkflowCapStatus, type WorkflowNotificationAPI, type WorkflowRunAPI } from "../api/workflows";
import { needsByWorkspace } from "../workspaces/workspaceActivity";

const pollMs = 30_000;

/** useWorkspaceNeeds counts, per Workspace, the runs waiting on the owner
 *  now, for the sidebar and tab badges: on mount, whenever refreshKey
 *  changes, on window focus and every 30 seconds. A failed read keeps the
 *  last counts. */
export function useWorkspaceNeeds(refreshKey?: unknown, api: Pick<WorkflowRunAPI, "openInterruptions"> = workflowRunAPI): Record<string, number> {
  const [needs, setNeeds] = useState<Record<string, number>>({});
  useEffect(() => {
    let live = true;
    const load = () => {
      api.openInterruptions().then((value) => { if (live) setNeeds(needsByWorkspace(value.interruptions)); }, () => undefined);
    };
    load();
    const timer = window.setInterval(load, pollMs);
    window.addEventListener("focus", load);
    return () => {
      live = false;
      window.clearInterval(timer);
      window.removeEventListener("focus", load);
    };
  }, [api, refreshKey]);
  return needs;
}

/** useWorkflowCaps reads today's workflow spend and the daily cap for the
 *  Workspace header, on mount and every 30 seconds. */
export function useWorkflowCaps(workspaceId: string | undefined, api: Pick<WorkflowNotificationAPI, "status"> = workflowNotificationAPI): WorkflowCapStatus | undefined {
  const [caps, setCaps] = useState<WorkflowCapStatus>();
  useEffect(() => {
    if (workspaceId === undefined) return;
    let live = true;
    const load = () => { api.status(workspaceId).then((value) => { if (live) setCaps(value.caps); }, () => undefined); };
    load();
    const timer = window.setInterval(load, pollMs);
    return () => {
      live = false;
      window.clearInterval(timer);
    };
  }, [api, workspaceId]);
  return caps;
}
