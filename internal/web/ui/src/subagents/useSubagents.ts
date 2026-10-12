import { createContext, useContext, useEffect, useState } from "react";
import { subagentAPI, type Subagent, type SubagentAPI } from "../api/subagents";
import type { ToolItem } from "../chat/activityModel";
import { subagentFinished } from "./subagentView";

/** The open conversation, so a chat row can read the children it started. */
export const SubagentSessionContext = createContext<string | undefined>(undefined);

export function useSubagentSession(): string | undefined {
  return useContext(SubagentSessionContext);
}

/**
 * The children one delegate call started, read again every intervalMs while
 * the call is unresolved or any child is unfinished, including after a failed
 * read. An undefined tool reads nothing.
 */
export function useSubagentList(sessionId: string | undefined, tool: ToolItem | undefined, live: boolean, api: SubagentAPI = subagentAPI, intervalMs = 2000) {
  const callId = tool?.id;
  const turnId = tool?.turn?.id ?? "";
  const [read, setRead] = useState<{callId: string; list: Subagent[]}>();
  const [problem, setProblem] = useState("");
  useEffect(() => {
    if (!sessionId || !callId) return;
    const controller = new AbortController();
    let timer: number | undefined;
    let unfinished = live;
    const tick = () => api.list(sessionId, turnId, callId, controller.signal).then((list) => {
      if (controller.signal.aborted) return;
      setRead({callId, list});
      setProblem("");
      unfinished = list.some((child) => !subagentFinished(child.state));
      if (live || unfinished) timer = window.setTimeout(tick, intervalMs);
    }, (error: unknown) => {
      if (controller.signal.aborted) return;
      setProblem(error instanceof Error ? error.message : "Subagents could not be read.");
      if (live || unfinished) timer = window.setTimeout(tick, intervalMs * 2);
    });
    void tick();
    return () => { controller.abort(); window.clearTimeout(timer); };
  }, [api, sessionId, turnId, callId, live, intervalMs]);
  // A list read for another call never shows under this one.
  return {list: read && read.callId === callId ? read.list : undefined, problem};
}
