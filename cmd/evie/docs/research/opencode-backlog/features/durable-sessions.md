# F08 - Durable sessions and execution events

Status: candidate, unapproved. Priority: first.

## Purpose

Make conversations, tool activity, approvals, and child relationships survive
process restarts without replaying uncertain side effects.

## How OpenCode does it

OpenCode stores sessions, messages, and typed parts in SQLite. Text, reasoning,
tool state, step boundaries, snapshots, patches, compaction, and subtasks are
represented explicitly rather than flattened into prose. Session runners keep
one active owner and cancellation/status events drive frontends.

The repository currently contains both legacy and newer persistence/event paths;
that migration complexity is not a pattern EVIE should reproduce.

Sources:

- [`core/session/sql.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/core/src/session/sql.ts)
- [`schema session types`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/schema/src/v1/session.ts)

## EVIE today

Conversation history is a process-only `[]openrouter.Message`. `evie.db` stores
cron state but no sessions or executions. Browser reload loses the visible
transcript while the server retains hidden in-memory history. The current
`memory.spec.md` already defines SQLite `sessions` and append-only `events` and
supersedes the older JSONL proposal.

## Proposed EVIE adaptation

Use the memory draft's append-only SQLite event model as the single source of
truth. Do not create a second coding-session store.

Persist:

- session identity, project context, parent/branch, profile, model, status;
- user admission and committed turn boundaries;
- assistant text/reasoning metadata needed for replay;
- tool call arguments and execution state before and after side effects;
- approval requests/replies;
- usage, finish reason, errors, compaction, snapshots, and verification events;
- child session/task relationships.

On restart, `running` tool execution becomes `unknown`. EVIE must not replay it.
The session remains blocked until David resolves it as succeeded, failed, or
left unknown. The model may receive only facts justified by that resolution.

## First slice

- Create/list/resume one session from REPL.
- Append-only events and deterministic message projection.
- Durable execution IDs and unknown-side-effect recovery.
- One active runner per session.
- Format versioning and provider-payload redaction.

## Later

- Web session picker and reload recovery.
- Branching from an earlier event.
- Background child jobs.
- Retention/hard deletion policy.

## Acceptance evidence

- Restart loses at most explicitly in-flight data and never silently replays a
  side effect.
- Event sequence remains unique and ordered under competing local processes.
- REPL and web project the same persisted session.
- Full history remains available after compaction.
- A torn/failed event transaction cannot produce an orphan model-visible tool
  result.

## Open decisions

1. Approve or split Stage 1 of `memory.spec.md` as the shared session feature.
2. How much opaque provider reasoning/continuation state is safe to persist?
3. What explicit user flow resolves an unknown execution?
