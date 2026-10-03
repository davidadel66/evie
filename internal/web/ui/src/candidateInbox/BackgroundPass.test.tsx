import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { MemoryBackgroundStatus } from "../api/memoryBackground";
import type { MemoryProposalItem } from "../api/memoryProposals";
import { MemoryPresentationProvider } from "../memory/presentation";
import { BackgroundPassStatusView } from "./BackgroundPassStatus";
import { ProposalInboxView, type ProposalInboxState } from "./ProposalInbox";

const on: MemoryBackgroundStatus = {
  enabled: true, state: "on", provider: "openrouter", model: "deepseek/deepseek-v4.1-flash", idle_minutes: 10, turn_trigger: 20,
  pass_token_budget: 32000, daily_token_cap: 300000, daily_micro_usd_cap: 250000, remaining_tokens: 287650, remaining_micro_usd: 246100,
  progress: {
    today: { since: "2026-10-03T00:00:00Z", input_tokens: 11000, output_tokens: 1350, cost_micro_usd: 3900 },
    pending_proposals: 2, sessions: [], skipped_batches: 0, runs_today: 3, proposals_queued_today: 4,
    last_run: { run_id: "r", session_id: "s", trigger: "idle", status: "succeeded", from_sequence: 1, through_sequence: 9, model: "openrouter:deepseek/deepseek-v4.1-flash@memory-background-v1",
      input_tokens: 900, output_tokens: 120, cost_micro_usd: 414, messages: 2, candidates: 2, queued: 1, duplicates: 0, noops: 1, skipped: 0,
      started_at: "2026-10-03T09:00:00Z", finished_at: "2026-10-03T09:00:03Z" },
  },
};

function status(value: MemoryBackgroundStatus) {
  return renderToStaticMarkup(<BackgroundPassStatusView status={value} problem="" />);
}

describe("background pass status", () => {
  it("shows the pass on, its model, today's spend against the caps and the last run", () => {
    const html = status(on);
    for (const text of ["Background memory pass", "On", "deepseek/deepseek-v4.1-flash", "12,350 of 300,000 tokens", "$0.0039 of $0.25",
      "after 10 minutes idle or every 20 messages", "Last run", "1 proposal"]) expect(html).toContain(text);
  });

  it("says why it is off, and when it is paused at the cap", () => {
    const off = status({ ...on, enabled: false, state: "disabled", reason: "needs EVIE_REMOTE_MEMORY=on to send your messages to deepseek/deepseek-v4.1-flash through OpenRouter", model: undefined });
    expect(off).toContain("Off");
    expect(off).toContain("needs EVIE_REMOTE_MEMORY=on");
    const paused = status({ ...on, state: "paused", reason: "today's cap is reached; the pass resumes after midnight UTC", remaining_tokens: 0 });
    expect(paused).toContain("Paused");
    expect(paused).toContain("cap is reached; the pass resumes after midnight UTC");
  });

  it("lists each conversation's cursor and pending owner messages", () => {
    const html = status({ ...on, progress: { ...on.progress, sessions: [
      { session_id: "a", title: "Trip planning", scope_key: "global", cursor: 42, pending_owner_messages: 3, failures: 0, last_status: "succeeded" },
      { session_id: "b", title: "", scope_key: "global", cursor: 7, pending_owner_messages: 1, failures: 2, next_attempt_at: "2026-10-03T10:00:00Z", last_status: "failed" },
    ] } });
    for (const text of ["Conversations", "Trip planning · read through event 42 · 3 new messages · last run succeeded", "Untitled conversation", "2 failures, next try"]) expect(html).toContain(text);
  });

  it("shows the latest problem", () => {
    const failing = status({ ...on, progress: { ...on.progress, last_error: { ...on.progress.last_run!, status: "failed", malformed: true, error: "malformed background model output: extraction: unexpected EOF" } } });
    expect(failing).toContain("Last problem");
    expect(failing).toContain("malformed background model output");
  });
});

const background = (overrides: Partial<MemoryProposalItem>): MemoryProposalItem => ({
  proposal_id: "p", session_id: "s", session_title: "Moving notes", scope_key: "global", tool_name: "memory_correct_claim", reason: "background",
  queued_at: "2026-10-03T09:00:00Z", revision: 1, proposal: JSON.stringify({ kind: "correct_claim", classification: { kind: "profile_fact", standing: true, bytes_per_token_milli: 0 } }),
  prepared_sha256: "sha256:x", approvable: true, ...overrides,
});

function inbox(items: MemoryProposalItem[]) {
  const state: ProposalInboxState = { page: { items, pending: items.length }, busy: false, problem: "", notice: "", alwaysApplies: {} };
  return renderToStaticMarkup(<MemoryPresentationProvider><ProposalInboxView state={state} onAlwaysApplies={() => undefined}
    onApprove={() => undefined} onDecline={() => undefined} onRefresh={() => undefined} /></MemoryPresentationProvider>);
}

describe("background proposals in the inbox", () => {
  it("labels them as found in your conversations and shows an update as current → new", () => {
    const html = inbox([background({ change: { action: "update", current: "owner — home city: Boston", replacement: "owner — home city: Chicago", evidence: "I moved to Chicago last month." } })]);
    for (const text of ["Found in your conversations", "Update", "You — home city: Boston", "You — home city: Chicago", "Your words", "I moved to Chicago last month.", "Always applies", "Approve", "Decline"]) expect(html).toContain(text);
  });

  it("shows a retirement plainly, with no Always applies switch", () => {
    const html = inbox([background({ tool_name: "memory_retire", proposal: JSON.stringify({ kind: "retire_memory_object" }),
      change: { action: "retire", current: "owner — morning drink: coffee", evidence: "I quit coffee for good." } })]);
    expect(html).toContain("Retire");
    expect(html).toContain("You — morning drink: coffee");
    expect(html).toContain("I quit coffee for good.");
    expect(html).not.toContain("Always applies");
  });

  it("marks a value the owner's words don't state", () => {
    const html = inbox([background({ change: { action: "update", current: "owner — music: rock", replacement: "owner — music: jazz", evie_proposed: true } })]);
    expect(html).toContain("your words don’t state this value");
  });
});
