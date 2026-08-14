# F15 - Source snapshots and session revert

Status: candidate, unapproved. Priority: before autonomous mutation.

## Purpose

Capture project state before model-driven mutation and support scoped undo
without resetting unrelated user work.

## How OpenCode does it

For Git projects, OpenCode creates a separate hidden Git object/index store. It
captures a tree before the model stream starts, then records changed files and a
later tree at step completion. Capturing before streaming is important because a
provider/tool runtime can execute an immediate tool before a later event hook.

Gitignored files and untracked files over 2 MiB are excluded. Unreachable
snapshot objects are pruned after seven days. Session revert restores files from
recorded patches and truncates/reconciles transcript state. OpenCode fixed a
regression specifically around instant tool calls mutating before snapshot.

Sources:

- [`snapshot/index.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/snapshot/index.ts)
- [`session/revert.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/session/revert.ts)

## EVIE today

Typed file edits have per-call stale previews but no multi-file checkpoint or
undo. Bash can mutate without any recorded before state. Git commands through
Bash can inspect or manually restore, but the session cannot identify exactly
which changes belong to one agent step.

## Proposed EVIE adaptation

- Git projects only in the first version; non-Git projects report snapshots
  unavailable rather than implying safety.
- Capture a hidden tree before any mutation-capable model step, not after the
  first tool event.
- Record the resulting changed paths/diff as session events.
- Revert only paths attributable to selected session steps.
- Detect files changed by David or another process since the recorded after
  state and refuse/preview conflicts instead of overwriting them.
- Revert transcript/tool projection consistently with filesystem restoration.
- Keep commit history and the user's index/branch untouched.

Snapshot is recovery, not permission. It must not justify removing approval or
verification by itself.

## Acceptance evidence

- An immediate first tool mutation is included in the snapshot boundary.
- New, modified, and deleted files restore correctly.
- User changes made after the agent step are never silently overwritten.
- `.gitignore`, large-file, symlink, and nested-worktree behavior is explicit.
- Snapshot cleanup does not prune still-referenced session state.

## Open decisions

1. Retention period and disk budget.
2. Include Gitignored small files or preserve OpenCode's exclusion?
3. Should every attended approved edit create a snapshot, or one per model step?
