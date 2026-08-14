# F12 - Foreground and background subagents

Status: candidate, unapproved. Priority: after durable sessions and profiles.

## Purpose

Delegate bounded, specialized work into isolated contexts while keeping the
parent agent responsible for the final outcome.

## How OpenCode does it

OpenCode's `task` tool selects a subagent profile, creates or resumes a child
session, inherits the caller's model unless overridden, and enforces a configured
depth limit (default one). Foreground calls wait for the child's final text.
Experimental background calls return immediately and later inject a synthetic
completion/error message into the parent.

The task tool records parent/child session IDs and supports `task_id` resume.
Cancellation propagates to children. At the inspected revision, background job
coordination is process-local, so it is not a restart-safe design EVIE should
copy directly.

The pinned resume path loads a supplied session ID without an explicit check
that it belongs to the requesting parent, project, or requested profile. It also
reuses persisted child permissions rather than visibly intersecting them with a
new parent policy. EVIE must treat this as a gap, not a pattern.

Source: [`tool/task.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/tool/task.ts).

## Proposed EVIE adaptation

Ship in two separate stages.

### Stage 1 - Foreground explore

- Only the mechanically read-only `explore` profile.
- Depth one; the child receives no spawn capability.
- Parent project root and hard permission denials are inherited.
- Child has its own transcript, context budget, cwd, and task ID.
- Parent receives a bounded final finding, not the full trajectory.
- Parent cancellation cancels the child.
- Resume accepts only a task found in the current parent's durable child
  registry, with the same project and profile.

### Stage 2 - Durable background work

- Requires a SQLite job/outbox, leases, restart recovery, and notifications.
- Spawn returns an admitted task handle immediately.
- Parent works only on non-overlapping scope.
- Results arrive as explicit verdict/artifact events.
- Child registry survives restart and compaction.

Mutable children wait for F16 worktree isolation and an approval policy. A child
must not use its profile to bypass parent denials, and a background child must
not block forever on an approval no human can see.

On every resume, EVIE revalidates parent/family ownership, canonical project ID,
profile, worktree, depth, active-runner ownership, and the current effective
permission ceiling. A stale persisted grant cannot restore authority the parent
has since lost. Cross-family, cross-project, and wrong-profile IDs fail without
revealing whether an unrelated session exists.

## Communication contract

- Parent, child, and siblings in one family only.
- Child returns verdicts, findings, and artifact paths, not reasoning or full
  transcript.
- Parent owns task ledger and final user response.
- Resuming a task continues its existing child session.

## Acceptance evidence

- Child context and tool output do not inflate parent history beyond the final
  result.
- Parent cancellation reaches all descendants.
- Depth limit cannot be bypassed.
- Parent denies remain hard child ceilings.
- Cross-family, cross-project, wrong-profile, and stale-permission resume
  attempts are rejected.
- Background work survives process restart before being called durable.
- Concurrent mutable children never share a worktree.

## Open decisions

1. What approval policy applies to an unattended child in an isolated worktree?
2. May siblings message each other directly, or only through the parent?
3. What result-size budget prevents subagent output from recreating the context
   problem it was meant to solve?
