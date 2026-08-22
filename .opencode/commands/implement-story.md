---
description: Implement one ready story in a worktree and open a draft PR
agent: build
---

Use the `skill` tool to load `implement-story`, then apply it to this story:

$ARGUMENTS

Invoking this command explicitly authorizes creation or safe resumption of one
isolated story worktree and `codex/` branch, scoped edits, verification,
commits, a non-force push, and a draft pull request. Never merge the pull
request. Stop rather than guessing when the story is not ready or the worktree
cannot be prepared safely.
