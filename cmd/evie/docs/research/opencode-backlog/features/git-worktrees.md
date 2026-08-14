# F16 - Git worktrees for isolated agents

Status: candidate, unapproved. Priority: before parallel mutable agents.

## Purpose

Prevent agents from corrupting one another's files and mechanically enforce
blind test-writing against a pre-change tree.

## How OpenCode does it

OpenCode has managed worktree support in addition to snapshots. Worktrees create
separate checkouts tied to one repository, allowing isolated sessions to operate
without sharing a filesystem view.

Sources:

- [`worktree/index.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/worktree/index.ts)
- [`git/index.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/git/index.ts)

## EVIE adaptation

Use worktrees for two specific cases, not every chat:

1. **Blind test writer:** checkout the pre-change revision so the implementation
   literally is not present. New tests must fail there.
2. **Concurrent mutable children:** one worktree per child, with explicit merge
   or patch handoff to the parent.

Each worktree has:

- owning session/task ID;
- source repository and base revision;
- canonical root used by every child tool;
- lifecycle status and cleanup record;
- changed-file/diff artifact;
- no direct permission to push or mutate the parent's checkout.

Worktree isolation is not an OS sandbox. An unrestricted shell can leave the
directory, so F05/F06 policy still matters.

## Acceptance evidence

- Two children can edit the same relative path without sharing bytes.
- Blind tests run against a tree that excludes the implementation.
- Cleanup refuses a dirty/unreported worktree rather than deleting evidence.
- Crash recovery lists orphaned worktrees and can reconcile them.
- Applying a child result detects conflicts with parent/user changes.

## Open decisions

1. Patch application, cherry-pick, or human-reviewed manual merge as the first
   handoff mechanism?
2. Where are temporary worktrees stored and how long are failed ones retained?
3. Does an isolated worktree justify auto-approval of file writes for a child?
   Recommendation: only after a separate explicit policy decision.
