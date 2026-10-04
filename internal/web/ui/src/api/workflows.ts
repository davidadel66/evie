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
// workflow, which executes its deterministic and branch steps, and lists
// and inspects runs with every node attempt. Since sub-stage 4 a run is
// advanced under a fenced lease by one Evie process at a time; inspection
// shows the lease and the recovery events (takeovers, interrupted attempts,
// pauses, resumes, scheduled retries).

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
  /** No process advances the run before this time: a retry's backoff, or a paused run's next check. */
  not_before?: string;
};

export type WorkflowNodeAttempt = {
  seq: number;
  step: string;
  kind: string;
  attempt: number;
  input_checkpoint: number;
  input: unknown;
  state: "running" | "succeeded" | "failed" | "interrupted";
  output?: unknown;
  error?: string;
  next?: string;
  note?: string;
  started_at: string;
  finished_at?: string;
  lease_holder?: string;
  lease_token?: number;
};

export type WorkflowCheckpoint = { seq: number; after_attempt?: number; next: string; state: Record<string, unknown>; created_at: string };

export type WorkflowRunLease = {
  run_id: string;
  holder: string;
  token: number;
  acquired_at: string;
  heartbeat_at: string;
  expires_at: string;
  released_at?: string;
};

export type WorkflowRunEvent = {
  run_id: string;
  seq: number;
  kind: "lease_taken_over" | "attempt_interrupted" | "paused" | "resumed" | "retry_scheduled" | "lease_released";
  holder: string;
  token: number;
  attempt?: number;
  detail: string;
  at: string;
};

// Durable interruptions and AI steps (Stage 14 sub-stage 7): a run asks the
// owner a typed question, or presents an effect whose outcome is unknown,
// and waits; any later session answers it. AI attempts record their model
// call with its usage and cost.
export type WorkflowInterruptionResolution = {
  kind: "answered" | "reconciled" | "withdrawn";
  answer?: Record<string, unknown>;
  actor: string;
  surface?: string;
  session_id?: string;
  detail?: string;
  recorded_at: string;
};

export type WorkflowInterruption = {
  run_id: string;
  seq: number;
  workspace_id: string;
  workflow: string;
  kind: "question" | "effect_reconciliation";
  step: string;
  checkpoint: number;
  intent_seq?: number;
  message: string;
  request: Record<string, unknown>;
  response_schema: Record<string, unknown>;
  session_id?: string;
  opened_at: string;
  resolution?: WorkflowInterruptionResolution;
};

export type WorkflowModelCall = {
  attempt_seq: number;
  attempt_state: string;
  step: string;
  policy: string;
  policy_version: number;
  model: string;
  estimated_input_tokens: number;
  estimated_output_tokens: number;
  estimated_micro_usd: number;
  finished_at?: string;
  requests?: number;
  input_tokens?: number;
  output_tokens?: number;
  micro_usd?: number;
  pricing_source?: string;
  error?: string;
};

export type WorkflowRunDetail = {
  run: WorkflowRun;
  attempts: WorkflowNodeAttempt[];
  checkpoints: WorkflowCheckpoint[];
  lease?: WorkflowRunLease;
  events?: WorkflowRunEvent[];
  interruptions?: WorkflowInterruption[];
  model_calls?: WorkflowModelCall[];
};

export type WorkflowRunResult = { run: WorkflowRunDetail; existing: boolean };

export const workflowRunAPI = {
  runs: (workspaceId: string): Promise<{ runs: WorkflowRun[] }> => postWorkflows("runs", { workspace_id: workspaceId }),
  start: (workspaceId: string, name: string, inputs: Record<string, unknown>): Promise<WorkflowRunResult> =>
    postWorkflows("start", { workspace_id: workspaceId, name, inputs }),
  inspect: (runId: string): Promise<WorkflowRunDetail> => postWorkflows("inspect", { run_id: runId }),
  answer: (runId: string, step: string, answer: Record<string, unknown>): Promise<WorkflowRunDetail> =>
    postWorkflows("answer", { run_id: runId, step, answer }),
  interruptions: (workspaceId: string): Promise<{ interruptions: WorkflowInterruption[] }> => postWorkflows("interruptions", { workspace_id: workspaceId }),
};
export type WorkflowRunAPI = typeof workflowRunAPI;

/** A model call's tokens and cost: its usage once recorded, its estimate otherwise (it may have been billed). */
export function modelCallSpend(call: WorkflowModelCall): { tokens: number; microUSD: number } {
  return call.finished_at
    ? { tokens: (call.input_tokens ?? 0) + (call.output_tokens ?? 0), microUSD: call.micro_usd ?? 0 }
    : { tokens: call.estimated_input_tokens + call.estimated_output_tokens, microUSD: call.estimated_micro_usd };
}

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
