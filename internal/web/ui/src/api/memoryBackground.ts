// The always-on background memory pass (memory architecture Stage 4): it
// reads only the owner's new messages after a conversation goes idle (or
// every N messages) and queues what it finds in the review inbox. This is
// its content-free status: on or off and why, model, triggers, caps,
// today's spend and the latest run.

export type MemoryBackgroundRun = {
  run_id: string;
  session_id: string;
  trigger: "idle" | "turns" | "compaction" | "manual";
  status: "succeeded" | "failed" | "skipped" | "deferred" | "superseded";
  from_sequence: number;
  through_sequence: number;
  model: string;
  input_tokens: number;
  output_tokens: number;
  cost_micro_usd: number;
  messages: number;
  candidates: number;
  queued: number;
  duplicates: number;
  noops: number;
  skipped: number;
  malformed?: boolean;
  error?: string;
  started_at: string;
  finished_at: string;
};

export type MemoryBackgroundSession = {
  session_id: string;
  title: string;
  scope_key: string;
  cursor: number;
  pending_owner_messages: number;
  failures: number;
  next_attempt_at?: string;
  last_status?: MemoryBackgroundRun["status"];
  last_run_at?: string;
};

export type MemoryBackgroundStatus = {
  enabled: boolean;
  state: "on" | "disabled" | "paused";
  reason?: string;
  provider?: string;
  model?: string;
  pricing_source?: string;
  idle_minutes: number;
  turn_trigger: number;
  pass_token_budget: number;
  daily_token_cap: number;
  daily_micro_usd_cap: number;
  remaining_tokens: number;
  remaining_micro_usd: number;
  progress: {
    activated_at?: string;
    last_run?: MemoryBackgroundRun;
    last_error?: MemoryBackgroundRun;
    today: { since: string; input_tokens: number; output_tokens: number; cost_micro_usd: number };
    pending_proposals: number;
    sessions: MemoryBackgroundSession[];
    skipped_batches: number;
    runs_today: number;
    proposals_queued_today: number;
  };
};

export const memoryBackgroundAPI = {
  async status(): Promise<MemoryBackgroundStatus> {
    const response = await fetch("/api/memory/background/status", { method: "POST", headers: { "Content-Type": "application/json" }, body: "{}", cache: "no-store" });
    const value = await response.json();
    if (!response.ok) throw new Error(value.error ?? `Background pass status failed (${response.status})`);
    return value as MemoryBackgroundStatus;
  },
};
export type MemoryBackgroundAPI = typeof memoryBackgroundAPI;
