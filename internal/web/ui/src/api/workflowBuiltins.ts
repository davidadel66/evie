// Built-in workflows (owner decision 2026-10-04): reviewed workflows built
// into Evie, which every Workspace runs without a proposal or approval
// because they hold no Standing Authority (only read-only research and
// explore workers, which the Workspace must allow). The owner turns one off
// or on for every Workspace; an approved copy of the same name (this
// Workspace's, then the Global one) overrides it.

export type WorkflowBuiltinControl = {
  seq: number;
  name: string;
  enabled: boolean;
  reason?: string;
  surface: string;
  recorded_at: string;
};

export type WorkflowBuiltinVersion = {
  version: number;
  executable_hash: string;
  commit: string;
  recorded_at: string;
  current?: boolean;
};

/** What a new run of the name in one Workspace would pin there, or why it cannot start. */
export type WorkflowBuiltinInWorkspace = {
  workspace_id: string;
  source: "builtin" | "global" | "workspace" | string;
  version?: number;
  blocked?: string[];
};

export type WorkflowBuiltin = {
  name: string;
  description: string;
  inputs: string[];
  workers: string[];
  max_tokens: number;
  executable_hash?: string;
  /** Its recorded version number, once a run pinned this Evie's version. */
  version?: number;
  problem?: string;
  enabled: boolean;
  control?: WorkflowBuiltinControl;
  versions?: WorkflowBuiltinVersion[];
  global?: { status: string; current?: { version: number } };
  workspace?: WorkflowBuiltinInWorkspace;
};

export class WorkflowBuiltinError extends Error {
  code: string;
  constructor(code: string, message: string) { super(message); this.code = code; }
}

async function postBuiltins<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(`/api/workflows/${path}`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body), cache: "no-store" });
  const value = await response.json();
  if (!response.ok) throw new WorkflowBuiltinError(value.code ?? "request_failed", value.error ?? `Workflow request failed (${response.status})`);
  return value as T;
}

export const workflowBuiltinAPI = {
  list: (workspaceId: string): Promise<{ builtins: WorkflowBuiltin[] }> => postBuiltins("builtins", { workspace_id: workspaceId }),
  setEnabled: (name: string, enabled: boolean): Promise<WorkflowBuiltinControl> => postBuiltins("builtins/control", { name, enabled }),
};
export type WorkflowBuiltinAPI = typeof workflowBuiltinAPI;
