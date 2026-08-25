# Evie Agent Guide

## Project

Evie is a Go-native personal agent runtime with local SQLite persistence,
purpose-built tools, an OpenRouter conversational client, and a web interface.

Prefer small, inspectable changes over speculative abstractions. Enforce safety,
authorization, persistence, and recovery boundaries in code and deterministic
checks rather than relying on prompt instructions.

## Sources of truth

- The current task defines the authorized scope of work.
- Active feature behavior lives in `cmd/*/docs/active/*.spec.md`.
- Binding feature decisions live in adjacent `*.decisions.md` files.
- Completed feature records live in `cmd/*/docs/done/`.
- Cross-cutting designs and decisions live in `docs/`.
- Files under `research/` are evidence and backlog material only; they do not
  authorize implementation.

Before changing a feature, read its task, specification, and decision record.
If they conflict in a way that affects behavior, security, persistence, or a
public interface, report the conflict instead of choosing silently.

## Repository map

- `cmd/evie/`: CLI, REPL, cron entry points, and Evie feature documents
- `cmd/finance/`, `cmd/todo/`, `cmd/ytscribe/`: supporting commands
- `internal/agent/`: conversation loop and agent-owned interfaces
- `internal/eviedb/`: SQLite setup and persistence implementations
- `internal/openrouter/`: OpenRouter transport
- `internal/tools/`: tool registry, execution, approvals, and safety fences
- `internal/web/`: HTTP server and approval flow
- `internal/web/ui/`: React and Vite frontend
- `docs/`: cross-cutting designs, decisions, and learning material

Interfaces should normally be owned by the package that consumes them.

## Scope

- Keep one change focused on one independently reviewable outcome.
- A ready story states its outcome, observable acceptance criteria, non-goals,
  dependencies, risks, and deterministic verification. It may live in a GitHub
  issue or a repository document; duplicate ceremony is not required.
- Split work when outcomes can be demonstrated independently or when one change
  combines multiple high-risk boundaries such as UI, persistence, migrations,
  authorization, or concurrency.
- Batch material product questions together. Do not invent a product decision
  that the task, specification, or decision record does not define.
- Work only on the assigned outcome. Avoid later stages, unrelated refactors,
  renames, formatting, and dependency updates.
- Add or update tests when behavior changes. Prefer existing seams and ask
  before adding a production dependency.

## Delivery loop

Use the thin loop in `docs/delivery-loop.md`:

1. Agree on one ready outcome.
2. Use one implementation task to make the smallest patch, running focused
   checks while developing.
3. Run `scripts/verify-change.sh` before handoff.
4. Open a draft pull request only when requested and let the required `Verify`
   check run.
5. Obtain one fresh, read-only review against the task, applicable specification,
   and exact diff. Use Codex `/review` when available.
6. The same implementer may make one bounded repair of blocking, in-scope
   findings, then reruns verification. Re-review only when the diff changed
   materially.
7. Hand the pull request to a human for approval and merge.

Do not use a multi-agent review swarm by default. Parallel work is appropriate
only for genuinely independent, dependency-ready changes or clearly distinct
specialist investigations, with isolated ownership and explicit user scope.

## Git safety

- Preserve user-owned and pre-existing working-tree changes.
- Never reset, discard, overwrite, or broadly reformat unrelated work.
- Use the branch or worktree assigned to the task.
- Do not create or switch branches or worktrees unless the task requests it.
- Commit, push, or open a pull request only when explicitly requested.
- Never merge a pull request on the user's behalf.

## Verification

Run focused checks while developing. Before handing off code or CI changes, run
from the repository root:

```sh
./scripts/verify-change.sh
```

Install UI dependencies first with `npm --prefix internal/web/ui ci` when
needed. The verification script runs the full Go test and vet suites, UI lint
and build, and staged and unstaged whitespace checks. Format changed Go files
with `gofmt` before running it.

For documentation-only changes, `git diff --check` is sufficient locally; CI
still runs the full required check. Report exact commands, results, warnings,
and every skipped check with its reason. Model review never replaces these
checks.

## Review priorities

Review only the task and changed behavior. Prioritize:

1. incorrect behavior or missing acceptance criteria;
2. authorization, secret-handling, and data-boundary violations;
3. persistence ordering, transaction, migration, recovery, and concurrency bugs;
4. cancellation or resource leaks;
5. missing regression and boundary tests;
6. unnecessary complexity that materially increases risk.

Do not block on unrelated cleanup or formatting handled by deterministic tools.

## Handoff

State what changed and why, the applicable story or specification, exact
verification and results, known risks and deferred work, manual demonstration
steps for user-visible behavior, and the best review entry points. Do not call
work complete while a required check is failing or skipped without an explicit
reason.
