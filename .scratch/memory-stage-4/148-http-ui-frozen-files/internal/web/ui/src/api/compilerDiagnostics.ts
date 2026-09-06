import { candidateReviewAPI, CandidateReviewError, type CandidateRef } from "./candidateReview";

export type DiagnosticView = "jobs" | "candidates" | "activations" | "history" | "selection" | "selections" | "live_roots" | "foreground";
export type DiagnosticQuery = { session_id: string; view: DiagnosticView; generation_id?: string; cursor?: string; limit: number };
export type DiagnosticSessions = { session_ids: string[]; next_cursor?: string };
export type AttemptMeasurement = { attempt: number; fence: number; claimed_at_unix_ms: number; queue_wait_nanos: number | null; inference_nanos: number | null; validation_nanos: number | null; database_completion_nanos: number | null; observed_outcome: string };
export type DiagnosticJob = { job_id: string; generation_id: string; session_id: string; first_sequence: number; last_sequence: number; state: string; reason?: string; attempts: number; retry_at?: number; pause_reason?: string; lane: string; recovery?: string; queued_at_unix_ms: number | null; published_at_unix_ms: number | null; selected_new_events: number; completed_new_events: number; candidate_freshness_nanos: number | null; publication_nanos: number | null; measurements: AttemptMeasurement[] };
export type DiagnosticCandidate = { ref: CandidateRef; job_id: string; generation_id: string; review_state: string; equivalent_to?: string; published_at_unix_ms: number | null; decided_at_unix_ms: number | null; edited: boolean };
export type DiagnosticActivation = { activation_id: string; selector: { source_scope: string; session_id?: string; destination: string }; generation_id: string; revision: number; after_position: number; through_position?: number; work_paused: boolean };
export type DiagnosticHistory = { request_id: string; range_index: number; generation_id: string; first_sequence: number; last_sequence: number; scanned_sequence: number; revision: number; cancelled: boolean };
export type DiagnosticUnit = { selection_id: string; generation_id: string; job_id?: string; first_sequence: number; last_sequence: number; state: string; reason?: string; selected_new_events: number };
export type DiagnosticRoot = { activation_id: string; root_id: string; first_sequence: number; last_sequence: number; state: string; reason?: string; selection_id?: string };
export type DiagnosticSelection = { event_id: string; sequence: number; membership: string };
export type ForegroundMeasurement = { root_id: string; started_at_unix_ms: number; terminal_committed_at_unix_ms: number | null; terminal_commit_nanos: number | null; response_finalized_at_unix_ms: number | null; response_finalization_nanos: number | null; outcome: string };
export type CompilerDiagnostics = { scope_key: string; session_id: string; view: DiagnosticView; as_of_unix_ms: number; revision: number; indexing: boolean; counts: Record<string, number>; capacity_state: string; jobs: DiagnosticJob[]; candidates: DiagnosticCandidate[]; activations: DiagnosticActivation[]; history: DiagnosticHistory[]; selection: DiagnosticSelection[]; foreground: ForegroundMeasurement[]; selections: DiagnosticUnit[]; live_roots: DiagnosticRoot[]; next_cursor?: string };

async function postDiagnostics<T>(path: string, scope_key: string, input: unknown): Promise<T> {
 const response = await fetch(`/api/memory/compiler/${path}`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ scope_key, input }), cache: "no-store" });
 const value = await response.json();
 if (!response.ok) throw new CandidateReviewError(value.code ?? "diagnostics_retryable", value.error ?? "Compiler diagnostics are unavailable. Refresh to try again.");
 return value as T;
}
export const compilerDiagnosticsAPI = {
 scopes: candidateReviewAPI.scopes,
 sessions: (scope: string, cursor = ""): Promise<DiagnosticSessions> => postDiagnostics("sessions", scope, { limit: 32, cursor }),
 inspect: (scope: string, query: DiagnosticQuery): Promise<CompilerDiagnostics> => postDiagnostics("diagnostics", scope, query),
};
export type CompilerDiagnosticsAPI = typeof compilerDiagnosticsAPI;
