// The owner's review inbox for memories Evie proposed without being asked
// (memory architecture Stage 2). Each item carries the exact prepared
// proposal, the same document an approval card shows; approving it applies
// exactly that proposal, or its "Always applies" alternative.

export type MemoryProposalReason = "not_requested" | "no_explicit_cue";

export type MemoryProposalItem = {
  proposal_id: string;
  session_id: string;
  session_title: string;
  scope_key: string;
  tool_name: string;
  reason: MemoryProposalReason;
  queued_at: string;
  revision: number;
  proposal: string;
  prepared_sha256: string;
  approvable: boolean;
  problem?: string;
};

export type MemoryProposalPage = { items: MemoryProposalItem[]; pending: number };
export type MemoryProposalResult = { proposal_id: string; operation_id: string; claim_id: string; standing: boolean };

export class MemoryProposalError extends Error {
  code: string;
  constructor(code: string, message: string) { super(message); this.code = code; }
}

async function postMemoryProposals<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(`/api/memory/proposals/${path}`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body), cache: "no-store" });
  const value = await response.json();
  if (!response.ok) throw new MemoryProposalError(value.code ?? "request_failed", value.error ?? `Memory inbox request failed (${response.status})`);
  return value as T;
}

export const memoryProposalAPI = {
  list: (): Promise<MemoryProposalPage> => postMemoryProposals("list", {}),
  count: (): Promise<{ pending: number }> => postMemoryProposals("count", {}),
  approve: (item: MemoryProposalItem, alwaysApplies: boolean): Promise<MemoryProposalResult> =>
    postMemoryProposals("approve", { proposal_id: item.proposal_id, prepared_sha256: item.prepared_sha256, always_applies: alwaysApplies }),
  decline: (id: string): Promise<{ proposal_id: string; declined: boolean }> => postMemoryProposals("decline", { proposal_id: id }),
};
export type MemoryProposalAPI = typeof memoryProposalAPI;
