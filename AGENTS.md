# Evie Agent Guide

## Project

Evie is a Go-native personal agent runtime with local SQLite persistence,
purpose-built tools, an OpenRouter conversational client, and a web interface.

Prefer small, inspectable changes over speculative abstractions. Enforce safety,
authorization, persistence, and recovery boundaries in code and deterministic
checks rather than relying on prompt instructions.

## Architecture map

Evie is the development spine. Use this conceptual map to place work and to
identify just-in-time learning needs:

```text
EVIE
|
|-- Memory
|   |-- graphs
|   |-- retrieval
|   |-- entity resolution
|   `-- temporal knowledge
|
|-- Agent runtime
|   |-- state machines
|   |-- tool execution
|   |-- planning
|   `-- evaluation
|
|-- Intelligence
|   |-- LLM fundamentals
|   |-- embeddings
|   |-- fine-tuning
|   `-- eventually research
|
`-- Infrastructure
    |-- databases
    |-- distributed systems
    |-- observability
    `-- deployment
```

Build from the active Evie feature outward: when implementation reaches a
knowledge boundary, learn enough to define and verify the next contract, then
return to the feature. Treat advanced research as a later lane driven by a
measured limitation in the working system. This map organizes thinking; it does
not authorize work beyond the current task, specification, and decisions.

For memory work, establish deterministic graph, retrieval, provenance,
temporal, and entity-resolution baselines before learned graph models. Learned
scores propose candidates; acceptance, authority, scope, and source rendering
remain explicit system boundaries.

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

## Package boundaries

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

## Git safety

- Preserve user-owned and pre-existing working-tree changes.
- Never reset, discard, overwrite, or broadly reformat unrelated work.
- Work on the branch or worktree assigned to the task, if one is. Otherwise
  create branches and worktrees as the work needs, for example one worktree
  per parallel subagent, and remove each worktree once its work is merged.
- Keep the owner's checkout on its current branch, with its working-tree
  changes in place.
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
and build, the drift check (`go run ./scripts/explorer check`), and staged and
unstaged whitespace checks. Format changed Go files with `gofmt` before
running it.

The drift check holds the facts (`docs/architecture/explorer/data/facts.js`),
the system map (`.vision/map.json`), and `CHANGELOG.md` to the code. Repair
stale facts with `go run ./scripts/explorer extract`; a new package needs a
component part with a cited summary in the map. After the map, the
changelog, the facts, or a design proposal in `.vision/proposals/` changes,
rebuild the committed page, `docs/architecture/explorer/system.html`, with the
vision skill (`python3 ~/.claude/skills/vision/scripts/vision.py build`) and
commit it with them; the check fails on a page older than any of them. Repair
everything else it reports by following
[`docs/architecture/explorer/README.md`](docs/architecture/explorer/README.md).
`/flow` updates the map, `CHANGELOG.md`, and the facts after each piece of
work.

For documentation-only changes, `git diff --check` and `go run
./scripts/explorer check` are sufficient locally, since glossary, ADR, and
heading edits can drift the facts and the map; CI still runs the full required
check.
Report exact commands, results, warnings, and every skipped check with its
reason. Model review never replaces these checks.

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

## Agent skills

### Issue tracker

Tickets are GitHub issues; specs are repository docs (see Sources of truth).
See `docs/agents/issue-tracker.md`.

### Triage labels

The five canonical triage roles, each label string equal to its name. See
`docs/agents/triage-labels.md`.

### Domain docs

Single-context: `GLOSSARY.md` and `docs/adr/` at the repo root. See
`docs/agents/domain.md`.
