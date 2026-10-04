// Workflow activation (Stage 14 sub-stage 2): the owner reviews a proposed
// Workflow Definition's approval card and approves exactly the executable
// hash, authority hash and reviewed version it showed. Proposals come from
// `evie workflows propose`; nothing here starts a run.

export type WorkflowStatus = "proposed" | "active" | "blocked" | "inactive";

export type AuthorityOperation = { capability: string; connection: string; resources: string[]; max_calls_per_run: number };
export type WorkflowSchedule = { cron: string; timezone: string; catch_up: string; max_runs?: number; inputs: string[] };

export type WorkflowActivation = {
  workspace_id: string;
  name: string;
  version: number;
  proposal_id: string;
  commit: string;
  executable_hash: string;
  authority_hash: string;
  authority: AuthorityOperation[];
  workers: string[];
  schedule?: WorkflowSchedule;
  approver: { actor: string; surface: string };
  approved_at: string;
  deactivated?: { reason: string; surface: string; at: string };
};

export type WorkflowProposalSummary = {
  id: string;
  executable_hash: string;
  simulation_status: string;
  documents_version?: number;
  approved_versions?: number[];
  pending: boolean;
  proposed_at: string;
};

export type WorkflowDefinition = {
  workspace_id: string;
  name: string;
  status: WorkflowStatus;
  blocked?: string[];
  current?: WorkflowActivation;
  pending: number;
  versions?: WorkflowActivation[];
  proposals?: WorkflowProposalSummary[];
};

export type SectionDiff = { section: string; removed?: string[]; added?: string[] };

export type WorkflowCard = {
  proposal: {
    id: string;
    workspace_id: string;
    name: string;
    commit: string;
    executable_hash: string;
    authority_hash: string;
    documents_version?: number;
    simulation?: { status: string; error?: string; attempts: number; tokens: number; previews: unknown[]; notice: string };
  };
  current?: WorkflowActivation;
  current_status?: WorkflowStatus;
  expected_version: number;
  needs_approval: boolean;
  diff: { from_hash?: string; to_hash: string; executable_changed: boolean; executable: SectionDiff[]; authority: string[]; documentation_changed: boolean };
  requirements: { authority: AuthorityOperation[]; authority_hash: string; workers: string[]; schedule?: WorkflowSchedule };
  refusals?: string[];
  patch: string;
};

export class WorkflowError extends Error {
  code: string;
  constructor(code: string, message: string) { super(message); this.code = code; }
}

async function postWorkflows<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(`/api/workflows/${path}`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body), cache: "no-store" });
  const value = await response.json();
  if (!response.ok) throw new WorkflowError(value.code ?? "request_failed", value.error ?? `Workflow request failed (${response.status})`);
  return value as T;
}

export const workflowAPI = {
  list: (workspaceId: string): Promise<{ workflows: WorkflowDefinition[] }> => postWorkflows("list", { workspace_id: workspaceId }),
  show: (workspaceId: string, name: string): Promise<WorkflowDefinition> => postWorkflows("show", { workspace_id: workspaceId, name }),
  diff: (proposalId: string): Promise<WorkflowCard> => postWorkflows("diff", { proposal_id: proposalId }),
  approve: (card: WorkflowCard): Promise<WorkflowActivation> => postWorkflows("approve", {
    proposal_id: card.proposal.id, executable_hash: card.proposal.executable_hash,
    authority_hash: card.proposal.authority_hash, expected_version: card.expected_version,
  }),
  deactivate: (workspaceId: string, name: string, version: number): Promise<WorkflowActivation> =>
    postWorkflows("deactivate", { workspace_id: workspaceId, name, version }),
};
export type WorkflowAPI = typeof workflowAPI;

/** The newest proposal still waiting for approval, if any. */
export function pendingProposal(definition: WorkflowDefinition): WorkflowProposalSummary | undefined {
  return definition.proposals?.find((proposal) => proposal.pending);
}

// Workflow runs (Stage 14 sub-stage 3): the owner starts a run of an active
// workflow, which executes its deterministic and branch steps before the
// response, and lists and inspects runs with every node attempt.

export type WorkflowRunState =
  | "queued" | "running" | "waiting_for_human" | "waiting_for_dependency" | "needs_reconciliation"
  | "cancelling" | "cancelled" | "failed" | "succeeded";

export type WorkflowRun = {
  id: string;
  workspace_id: string;
  workflow: string;
  version: number;
  commit: string;
  executable_hash: string;
  authority_hash: string;
  workspace_revision: string;
  access_revision: number;
  run_key: string;
  inputs: Record<string, unknown>;
  state: WorkflowRunState;
  outputs?: Record<string, unknown>;
  error?: string;
  surface: string;
  session_id?: string;
  checkpoint: number;
  next_step?: string;
  created_at: string;
  started_at?: string;
  finished_at?: string;
};

export type WorkflowNodeAttempt = {
  seq: number;
  step: string;
  kind: string;
  attempt: number;
  input_checkpoint: number;
  input: unknown;
  state: "running" | "succeeded" | "failed";
  output?: unknown;
  error?: string;
  next?: string;
  note?: string;
  started_at: string;
  finished_at?: string;
};

export type WorkflowCheckpoint = { seq: number; after_attempt?: number; next: string; state: Record<string, unknown>; created_at: string };

export type WorkflowRunDetail = { run: WorkflowRun; attempts: WorkflowNodeAttempt[]; checkpoints: WorkflowCheckpoint[] };

export type WorkflowRunResult = { run: WorkflowRunDetail; existing: boolean };

export const workflowRunAPI = {
  runs: (workspaceId: string): Promise<{ runs: WorkflowRun[] }> => postWorkflows("runs", { workspace_id: workspaceId }),
  start: (workspaceId: string, name: string, inputs: Record<string, unknown>): Promise<WorkflowRunResult> =>
    postWorkflows("start", { workspace_id: workspaceId, name, inputs }),
  inspect: (runId: string): Promise<WorkflowRunDetail> => postWorkflows("inspect", { run_id: runId }),
};
export type WorkflowRunAPI = typeof workflowRunAPI;

/** The start form's inputs: one JSON object, or undefined when it is not one. */
export function parseRunInputs(text: string): Record<string, unknown> | undefined {
  if (text.trim() === "") return {};
  try {
    const value: unknown = JSON.parse(text);
    return value !== null && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : undefined;
  } catch {
    return undefined;
  }
}
