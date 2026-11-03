import type { Item } from "../store/reducer";

// Delegated children as the owner watches them from the parent conversation
// (chat-activity.spec.md, subagent inspector amendment). Read-only.

export type SubagentState = "admitted" | "running" | "succeeded" | "partial" | "failed" | "cancelled" | "interrupted";
export type SubagentAction = { name: string; args: string; done: boolean };
export type Subagent = {
  executionId: string;
  worker: string;
  key: string;
  objective: string;
  context?: string;
  state: SubagentState;
  continues?: string;
  createdAt: string;
  startedAt?: string;
  endedAt?: string;
  deadlineMs?: number;
  actions: number;
  latest?: SubagentAction;
};
export type SubagentTrace = { subagent: Subagent; items: Item[] };
/** A child's stored final report, the one the parent model pages through
 *  read_subagent_report (artifacts.spec.md). text is absent when the child
 *  has no final report; omittedBytes is what a report over 1 MiB left out. */
export type SubagentReport = {
  executionId: string;
  worker: string;
  status: SubagentState;
  reason?: string;
  text?: string;
  totalBytes: number;
  omittedBytes: number;
};

async function postSubagents<T>(path: string, body: unknown, signal?: AbortSignal): Promise<T> {
  const response = await fetch(`/api/subagents/${path}`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body), cache: "no-store", signal });
  const value = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(value.error ?? "Subagents could not be read.");
  return value as T;
}

export const subagentAPI = {
  /** turnId (the call's turn) tells apart calls that reuse a provider call ID. */
  list: (sessionId: string, turnId: string, callId: string, signal?: AbortSignal) =>
    postSubagents<{ subagents: Subagent[] }>("list", { sessionId, turnId, callId }, signal).then((value) => value.subagents),
  trace: (sessionId: string, executionId: string, signal?: AbortSignal) =>
    postSubagents<SubagentTrace>("trace", { sessionId, executionId }, signal),
  report: (sessionId: string, executionId: string, signal?: AbortSignal) =>
    postSubagents<{ report: SubagentReport }>("report", { sessionId, executionId }, signal).then((value) => value.report),
};
export type SubagentAPI = typeof subagentAPI;
