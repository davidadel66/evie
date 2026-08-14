# A04 - General subagent

Status: deferred, unapproved. Kind: mutable subagent.

## Purpose

Handle a broad independent unit of work that does not fit a specialized child
profile.

## How OpenCode does it

OpenCode's `general` agent is a normal tool-using subagent. It inherits the
caller's model unless configured otherwise, retains broad default permissions,
and disables todo writing. The task tool describes it as suitable for complex
research and parallel multi-step work.

Source: [`Agent` registry, `general`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/agent/agent.ts#L182-L195).

## EVIE assessment

Do not make this an early profile. A second unrestricted EVIE adds cost,
coordination, approval, and file-conflict problems without adding a clear
specialty. `explore`, spec review, blind tests, and code review each have a
reason to exist and can be constrained. `general` is mostly an escape hatch.

## Conditions that could earn it

- Durable child sessions and cancellation already work.
- Parent and child have isolated worktrees when either can mutate.
- Effective parent permission denials are inherited.
- The task is independent and has a concrete artifact or verdict contract.
- The caller can explain why `build` should not do the work directly.

## Proposed constraints if promoted

- Depth one by default.
- No task-ledger mutation in the child.
- Explicit file/worktree ownership.
- Per-child turn, token, time, and cost budgets.
- Return final artifact paths and a concise verdict, never the full trajectory.
- No Git commit, push, release, or external publication without separate
  authority.

## Acceptance evidence

- Two children cannot unknowingly edit the same worktree.
- A parent denial cannot be restored by choosing `general`.
- Budget exhaustion is observable to the parent.
- Child approval requests have a defined attended/unattended policy.

## Open decision

Is there a real recurring workload that specialized roles cannot cover? If not,
keep this deferred.
