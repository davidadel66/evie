# F23 - File and Git watching

Status: deferred, unapproved.

## Purpose

Detect project changes made outside EVIE so caches, diffs, instructions, and
diagnostics do not silently use stale state.

## How OpenCode does it

OpenCode has an optional native filesystem watcher that publishes file changes
and Git `HEAD` changes into its event system. It uses that infrastructure for
responsive project state and service invalidation.

Source: [`core/filesystem/watcher.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/core/src/filesystem/watcher.ts).

## EVIE assessment

Defer. EVIE currently has little cached project state, so re-reading at safe
boundaries is simpler and more reliable. Watchers add platform dependencies,
event coalescing, rename semantics, ignored-directory policy, and overflow
recovery.

## Trigger to promote

- Instruction/LSP/index caches make repeated full validation expensive.
- External edits routinely invalidate prepared changes or displayed diffs.
- Measurements show polling/revalidation is a bottleneck.

## Proposed first slice if promoted

- Watch one canonical project root.
- Publish coarse "project changed" and Git `HEAD` changed events.
- Invalidate caches; do not infer exact semantic changes from watcher events.
- Always retain safe-boundary revalidation before mutation.
- Recover from dropped/overflowed events with a full rescan.

## Acceptance evidence

- External edits invalidate prepared previews and stale context.
- Ignore rules prevent dependency/build directories from flooding the session.
- Watcher failure degrades to explicit revalidation, not stale trust.
