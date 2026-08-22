# Evie story loop

`evie-story-loop` is an executable controller for one approved engineering
story. It automates the task handoff that otherwise happens manually:

1. prepare an isolated Git worktree and start one persistent, workspace-write
   implementation task with a controller-owned worker contract;
2. review its exact commit in a brand-new, read-only task with `review-story`;
3. run operator-supplied deterministic checks outside both model tasks;
4. resume the original implementation task with the combined review and
   validation delta; and
5. repeat until the candidate is ready or a bounded stop condition fires.

The controller is deliberately not a skill. The external `review-story` skill
defines the review task; a small embedded worker contract avoids invoking the
high-level `implement-story` workflow and then conflicting with its own review
and delivery gates. The process owns transitions, persistence, candidate
identity, validation, locking, convergence, and delivery.

## Install

The tool needs Python 3.10 or newer. Its project pins the stable Python Codex
SDK and the matching bundled Codex runtime:

```sh
uv sync --project tools/story-loop
```

The bundled runtime uses the same local Codex authentication as the app and
CLI. A ChatGPT Pro login is sufficient for an interactive local run; an API
key is not required. API-key authentication remains appropriate for an
unattended environment and is billed separately.

Confirm runtime startup, authentication, the read-only sandbox, and structured
output before starting a story:

```sh
uv run --project tools/story-loop evie-story-loop smoke --repo .
```

The smoke command starts an ephemeral read-only task that does not inspect or
modify repository files. If authentication is missing, sign in through the
Codex app or a working Codex CLI, then rerun it.

## Start one story

Start from any worktree in the target repository; the CLI resolves the primary
worktree, loads the approved issue contract with `gh`, and prepares
`.worktrees/<run-id>` itself before granting an agent write access. The primary
worktree is never the implementation task's writable root.

`review-story` is currently an explicit external dependency because its skill
files are not yet committed on `master`. Pass the absolute path to the reviewed
skill contract. Once that separate skill work lands, the CLI can gain a
repository default without coupling this controller PR to those changes.

```sh
uv run --project tools/story-loop evie-story-loop start \
  --story https://github.com/davidadel66/evie/issues/54 \
  --review-skill /absolute/path/to/review-story/SKILL.md \
  --base master \
  --check 'git diff --check master...HEAD' \
  --check 'go test ./...' \
  --check 'go vet ./...' \
  --max-review-passes 3 \
  --draft-pr
```

Important controls:

- `--check` is repeatable and required. These shell commands are trusted
  operator input saved in immutable run configuration. The controller never
  executes commands proposed by a model.
- `--review-skill` is required and may point to a repository, personal, or
  installed `review-story/SKILL.md`. Startup fails before creating a run if the
  file is absent.
- The story URL is resolved with `gh issue view` before either sandboxed task
  starts. Use `--story-contract-file` for an approved local execution contract
  or an environment without GitHub access.
- `--draft-pr` is explicit delivery authorization. Without it, a passing run
  stops with a local candidate ready for human handoff. The controller never
  approves or merges.
- `--max-review-passes` defaults to three. A repeated semantic delta stops
  earlier as `STALLED`.
- `--run-id` overrides the stable ID derived from the story. An issue URL such
  as issue 54 becomes `issue-54`.
- `--model` defaults to `gpt-5.6-terra`, which is supported with ChatGPT-backed
  Codex authentication. Override it explicitly when another available model is
  required.

The implementation task is scoped to the controller-created worktree and told
to commit locally but not push or create a pull request. Its thread ID is
persisted before the first write-producing turn. On resume, a clean advanced
commit from an interrupted implementation or repair turn is reconciled as a
candidate and still has to pass fresh review and deterministic validation.
After both pass, the controller performs delivery only when `--draft-pr` was
set and verifies that the resulting PR is open, draft, on the configured base
and head branches, and bound to the exact candidate SHA.

## Inspect and resume

State is outside every candidate diff under the repository's Git common
directory:

```text
.git/story-loop/<run-id>/
  state.json
  run.lock
  iterations/
    001.json
    002.json
```

`state.json` is replaced atomically after each completed phase. Each iteration
receipt is immutable and contains the exact candidate SHA, fresh reviewer task
ID, structured findings, deterministic command results, remaining delta, and
controller decision.

Inspect a run without starting an agent:

```sh
uv run --project tools/story-loop evie-story-loop status \
  --repo . \
  --run-id issue-54
```

Resume after an interrupted process:

```sh
uv run --project tools/story-loop evie-story-loop resume \
  --repo . \
  --run-id issue-54
```

One non-blocking file lock prevents two controllers from advancing the same
run. A resume begins at the last durable phase boundary rather than restarting
the whole story.

## Stop outcomes

- `READY_FOR_HUMAN_REVIEW`: fresh review has no findings and every configured
  check passed; a draft PR is also available when authorized.
- `DECISION_REQUIRED`: implementation or review found a product/specification
  choice that the controller must not guess.
- `REVIEW_INCOMPLETE`: reviewer output or required review evidence was
  incomplete.
- `IMPLEMENTATION_INCOMPLETE` / `IMPLEMENTATION_FAILED`: the persistent worker
  could not produce a valid committed candidate.
- `MAX_PASSES`: the configured review bound was reached.
- `STALLED`: the semantic delta did not change after repair.
- `INVALID_CANDIDATE`: a worktree, branch, cleanliness check, or exact SHA no
  longer matched the structured handoff.
- `DELIVERY_FAILED` / `LOOP_FAILED`: a controller-owned operation failed after
  prior state had been preserved.

Controlled non-ready outcomes return exit code 2. Configuration or startup
errors return 1, and an interrupt returns 130.

## Development verification

The deterministic suite uses a fake backend and makes no network or model
calls:

```sh
PYTHONPATH=tools/story-loop/src \
  python3 -m unittest discover -s tools/story-loop/tests -v
```

The suite covers success, repair feedback, changing and repeated validation
failures, human-decision and incomplete-review stops, convergence bounds,
stale candidates, contradictory or invalid structured output, isolated
worktree preparation, locking, atomic receipts, exact draft-PR metadata, SDK
thread/sandbox selection, and interruption recovery after implementation and
repair commits.
