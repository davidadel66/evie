# F17 - Bounded autonomous builds

Status: deferred, unapproved. Priority: much later.

## Purpose

Allow EVIE to implement a project task without continuous supervision while
remaining restart-safe, bounded, recoverable, and mechanically judged.

## OpenCode comparison

OpenCode supplies many required primitives: profiles, tasks, cancellation,
permissions, persisted tool states, snapshots, compaction, and worktrees. The
important lesson is that autonomy is the composition of these controls, not one
`--auto` flag.

The current `docs/delivery-loop.md` keeps the repository's coding workflow
small and mechanism-driven before making EVIE an execution backend:

```text
scope -> one implementation task -> deterministic checks
-> one fresh review -> bounded repair or safe stop -> human merge
```

## Preconditions

- F01-F09 foundation complete.
- `build`, review profiles, and subagent runtime complete.
- Project verification contract approved.
- Snapshots and worktree isolation complete.
- Durable jobs and restart recovery complete.
- Explicit autonomous write authority and cost limits.

## Proposed first workflow

`evie build <spec> --auto` runs a Go-owned stage machine, not an agent deciding
its own process:

1. Freeze spec hash, base revision, project instructions, permissions, budgets.
2. Run spec review; block on unresolved high-impact findings.
3. Write blind tests in the old-tree worktree; prove they fail.
4. Run build agent in an isolated implementation worktree.
5. Run required gates; permit bounded repair attempts.
6. Run fresh-context code review against spec and diff.
7. Re-run gates after review fixes.
8. Produce a report and await human demo/merge decision.

Bounds include model steps, repair attempts, wall time, tokens, cost, child
count, recursion depth, and disk use. Exhaustion is a failed/incomplete build,
not a reason to silently raise limits.

## Acceptance evidence

- Killing EVIE mid-build resumes or blocks safely without replaying unknown
  side effects.
- No stage can bypass the deterministic gate.
- New tests fail old tree and pass new tree.
- Final report includes exact evidence, skipped stages, and known gaps.
- Nothing commits, pushes, publishes, or merges without separately granted
  authority.

## Open decisions

1. Keep David as default implementation stage for learning projects and require
   `--auto` explicitly? Recommendation: yes.
2. Which spec findings block automatically versus require human triage?
3. How many repair rounds are permitted before failure?
