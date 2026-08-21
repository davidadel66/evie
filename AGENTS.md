# Evie Agent Guide

## Project

Evie is a Go-native personal agent runtime with local SQLite persistence,
purpose-built tools, an OpenRouter conversational client, and a web interface.

Prefer small, inspectable changes over speculative abstractions. Enforce safety,
authorization, persistence, and recovery boundaries in code rather than relying
on prompt instructions.

## Sources of truth

- The current task defines the authorized scope of work.
- Active feature behavior lives in `cmd/*/docs/active/*.spec.md`.
- Binding feature decisions live in adjacent `*.decisions.md` files.
- Completed feature records live in `cmd/*/docs/done/`.
- Cross-cutting designs and decisions live in `docs/`.
- Files under a `research/` directory are evidence and backlog material only;
  they do not authorize implementation.

Before changing a feature, read the applicable task, specification, and decision
record. If they conflict in a way that affects behavior, security, persistence,
or a public interface, report the conflict instead of silently choosing an
interpretation.

## Repository map

- `cmd/evie/`: Evie CLI, REPL, cron entry points, and Evie feature documents
- `cmd/finance/`, `cmd/todo/`, `cmd/ytscribe/`: supporting commands
- `internal/agent/`: conversation loop and agent-owned interfaces
- `internal/eviedb/`: SQLite setup and persistence implementations
- `internal/openrouter/`: OpenRouter transport
- `internal/tools/`: tool registry, execution, approvals, and safety fences
- `internal/web/`: HTTP server and approval flow
- `internal/web/ui/`: React and Vite frontend
- `docs/`: cross-cutting designs, decisions, and learning material

Interfaces should normally be owned by the package that consumes them.

## Working agreement

- Keep one change focused on one independently reviewable outcome.
- Work only on the assigned acceptance criteria; do not implement later stages
  or adjacent backlog items opportunistically.
- Avoid unrelated refactors, renames, formatting, and dependency updates.
- Add or update tests when behavior changes.
- Prefer existing seams unless the task requires a new abstraction.
- Ask before adding a production dependency.
- If a change grows across multiple concerns or becomes difficult to review,
  stop and propose a smaller sequence of changes.
- When implementation is requested, complete the scoped work autonomously.
  Use tutor-style, line-by-line development only when the user asks for it.

## Git safety

- Preserve user-owned and pre-existing working-tree changes.
- Never reset, discard, overwrite, or broadly reformat unrelated work.
- Use the branch or worktree assigned to the task.
- Do not create or switch branches or worktrees unless the task requests it.
- Commit, push, or open a pull request only when explicitly requested.
- Never merge a pull request on the user's behalf.

## Verification

Run focused checks while developing. Before handing off Go changes, run from the
repository root:

```sh
go test ./...
go vet ./...
```

Format changed Go files with `gofmt`.

If `internal/web/ui/` changed, also run from that directory:

```sh
npm run lint
npm run build
```

For documentation-only changes, run `git diff --check` and verify affected links
or references when practical.

Report exact commands, results, relevant warnings, and every skipped check with
its reason. A model review does not replace deterministic verification.

## Code review rules

Review the change against its task and applicable specification. Prioritize:

1. incorrect behavior or missing acceptance criteria;
2. authorization, secret-handling, and data-boundary violations;
3. persistence ordering, transaction, recovery, and concurrency bugs;
4. cancellation or resource leaks;
5. missing regression and boundary tests;
6. unnecessary complexity that materially increases risk.

Keep findings scoped to the change. Do not block on unrelated cleanup or
subjective formatting that deterministic tooling can enforce.

## Handoff

Every completed implementation should state:

- what changed and why;
- the applicable story or specification section;
- verification performed and its results;
- known gaps, risks, and deliberately deferred work;
- manual demonstration steps when behavior is user-visible;
- the best files or entry points for a reviewer to read first.

Do not call work complete while required verification is failing or skipped
without an explicit, recorded reason.
