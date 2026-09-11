import type { SemanticClaim, SemanticObjectInspection } from "./memory";

export type MemoryActivityData = {
  snapshotId: string;
  status: string;
  acceptedCount: number;
  excerptCount: number;
  historicalCount?: number;
  retiredCount?: number;
  conflictCount?: number;
};

export type MemorySourceReference = {
  source_link_id?: string;
  event_id: string;
  event_part: string;
  session_id: string;
  scope_key: string;
  authority: string;
  observed_at: string;
  locator_kind: string;
  locator_value: string;
  evidence_sha256: string;
};

export type MemoryReference = {
  id: string;
  kind: string;
  claim_id?: string;
  claim_operation_id?: string;
  as_known_at: string;
  valid_at: string;
  scope_key: string;
  status: string;
  current_status?: string;
  intent?: string;
  valid_at_constrained?: boolean;
  correction_mode?: string;
  current_correction_mode?: string;
  conflicts?: SemanticObjectInspection["conflicts"];
  related_claim_ids?: string[];
  paths: string[];
  sources: MemorySourceReference[];
};

export type MemoryEvidenceReceipt = {
  sessionId: string;
  snapshotId: string;
  version: string;
  status: string;
  evidence: {
    reference: MemoryReference;
    available: boolean;
    current_status: string;
    evidence?: {
      text: string;
      claim?: Pick<SemanticClaim, "transaction_time" | "valid_time">;
      effective_valid_time?: SemanticClaim["valid_time"];
      current_status?: string;
      correction_mode?: string;
      current_correction_mode?: string;
      conflicts?: SemanticObjectInspection["conflicts"];
      related_claim_ids?: string[];
      sources: {
        source_link_id?: string;
        event_id: string;
        session_id: string;
        source_scope_key: string;
        authority: string;
        actor?: string;
        observed_at: string;
        evidence: string;
        locator_kind: string;
        locator_value: string;
        evidence_sha256: string;
      }[];
    };
  }[];
};

export async function inspectMemoryEvidence(sessionId: string, snapshotId: string, signal?: AbortSignal): Promise<MemoryEvidenceReceipt> {
  const response = await fetch("/api/memory/evidence", {
    method: "POST", headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ sessionId, snapshotId }), cache: "no-store", signal,
  });
  const value = await response.json() as MemoryEvidenceReceipt & { error?: string };
  if (!response.ok) throw new Error(value.error ?? "Original memory evidence is unavailable.");
  if (value.sessionId !== sessionId || value.snapshotId !== snapshotId) throw new Error("The selected conversation changed. Open its sources again.");
  return { ...value, evidence: value.evidence ?? [] };
}
