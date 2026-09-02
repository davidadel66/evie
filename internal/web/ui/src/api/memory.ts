export type ScopeRevision = { scope_key: string; revision: number };
export type ExactReadMetadata = {
  valid_at: string;
  as_known_at: string;
  selected_scope?: string;
  allowed_scopes: string[];
  scope_revisions: ScopeRevision[];
};
export type SemanticScope = {
  scope_id: string;
  scope_key: string;
  registry_id?: string;
  revision: number;
  quarantined?: boolean;
  quarantine_reason?: string;
};
export type SemanticEntity = {
  entity_id: string;
  scope_key: string;
  canonical_name: string;
  entity_type: string;
  anchor_kind?: string;
};
export type SemanticPredicate = {
  predicate_id: string;
  token: string;
  version: number;
  label: string;
  object_constraint: string;
  cardinality: string;
};
export type SemanticClaim = {
  claim_id: string;
  scope_key: string;
  subject_entity_id: string;
  predicate: SemanticPredicate;
  object: { entity_id?: string; literal?: { kind: string; value: string } };
  polarity: string;
  valid_time: { from?: string | null; to?: string | null };
  created_operation_id: string;
  transaction_time: string;
};
export type SemanticObjectSummary = {
  object_kind: "entity" | "alias" | "claim" | "source_link" | "graph_link";
  object_id: string;
  scope_key: string;
  status: string;
  entity?: SemanticEntity;
  claim?: SemanticClaim;
};
export type SemanticObjectPage = {
  metadata: ExactReadMetadata;
  objects: SemanticObjectSummary[];
  next_cursor?: string;
};
export type SemanticScopePage = {
  metadata: ExactReadMetadata;
  scopes: SemanticScope[];
  next_cursor?: string;
};
export type SemanticSourceInspection = {
  source: {
    source_link_id?: string;
    operation_id?: string;
    event_id: string;
    session_id: string;
    source_scope_key: string;
    authority: string;
    observed_at: string;
    evidence_sha256: string;
    evidence: string;
    eligibility: string;
  };
  lifecycle: SemanticState[];
};
export type SemanticState = {
  state: string;
  operation_id: string;
  scope_revision: number;
  transaction_time: string;
};
export type SemanticOperation = {
  operation_id: string;
  schema_version: number;
  kind: string;
  source_event_id: string;
  proposal_sha256: string;
  effect_sha256: string;
  transaction_time: string;
  prior_revisions: ScopeRevision[];
  resulting_revisions: ScopeRevision[];
};
export type SemanticObjectInspection = {
  object_kind: SemanticObjectSummary["object_kind"];
  object_id: string;
  scope: SemanticScope;
  status: string;
  entity?: SemanticEntity;
  claim?: SemanticClaim;
  sources: SemanticSourceInspection[];
  lifecycle: SemanticState[];
  operations: SemanticOperation[];
  conflicts: { code: string; predicate_token: string; claim_ids: string[] }[];
  metadata: ExactReadMetadata;
};

export type MemoryTimeFilter = { history?: boolean; validAt?: string; asKnownAt?: string };
export type MemoryObjectQuery = MemoryTimeFilter & {
  scopeKey: string;
  kinds: ("entity" | "claim")[];
  pageSize?: number;
  cursor?: string;
};

async function postJSON<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const value = (await response.json()) as T & { error?: string };
  if (!response.ok) throw new Error(value.error ?? `Request failed (${response.status})`);
  return value;
}

export function listMemoryScopes(): Promise<SemanticScopePage> {
  return postJSON("/api/memory/scopes", {});
}

export function listMemoryObjects(query: MemoryObjectQuery): Promise<SemanticObjectPage> {
  return postJSON("/api/memory/objects", query);
}

export function inspectMemoryObject(
  scopeKey: string,
  kind: SemanticObjectSummary["object_kind"],
  id: string,
  filter: MemoryTimeFilter,
): Promise<SemanticObjectInspection> {
  return postJSON("/api/memory/inspect", { scopeKey, kind, id, ...filter });
}
