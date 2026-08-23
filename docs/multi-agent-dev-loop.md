# Durable reviewed story loop

Status: proposed design and GitHub execution-contract draft

This document defines a repository-local Codex SDK controller for delivering
one approved engineering story through implementation, deterministic
validation, independent review, and focused repair. It replaces the retired
design for an Evie-native general multi-agent runtime.

The first implementation story stops at one exact local candidate that is ready
for human review. Git push and draft-pull-request delivery remain a later,
independently reviewable story.

## Outcome

Replace manual back-and-forth between implementation and review tasks with a
resumable, auditable controller that can run unattended until one of two honest
outcomes:

- an exact candidate passes deterministic validation and fresh independent
  review; or
- the run stops with a precise decision, evidence, recovery, or convergence
  blocker.

The controller must make workflow state and agent ownership durable. Prompt
instructions alone are not a concurrency, recovery, or validation boundary.

## Why a controller in addition to skills

The repository skills define the workflow policy:

- `implement-story` creates one clean committed candidate;
- `review-story` defines exact-candidate validation and three independent review
  lenses; and
- `deliver-story` prototypes the persistent-worker, fresh-reviewer, bounded
  repair loop.

The controller provides mechanisms that a skill cannot guarantee:

- exclusive ownership of SDK threads and one active writer per thread;
- atomic state transitions and append-only receipts;
- restart-safe resumption without repeating completed phases;
- runtime schema validation rather than prose interpretation;
- exact candidate and source identity checks;
- process-tree cleanup for deterministic-check timeouts; and
- bounded unattended execution.

The controller uses the Codex SDK now. Evie may become an alternative execution
backend only after its own leases, recovery, permissions, structured outputs,
and observability are independently proven.

## Sources of truth

The implementation must freeze the exact Git commit and content identity of:

- `.agents/skills/implement-story/SKILL.md`;
- `.agents/skills/implement-story/references/candidate-result.schema.json`;
- `.agents/skills/review-story/SKILL.md`;
- `.agents/skills/review-story/references/lens-result.schema.json`;
- `.agents/skills/review-story/references/review-result.schema.json`;
- `.agents/skills/deliver-story/SKILL.md`;
- `AGENTS.md`; and
- the selected story, applicable specifications, and binding decisions.

Mutable filesystem paths are not sufficient audit evidence. A run records the
source commit and digests used for every prompt and schema that can affect its
verdict.

Historical evidence:

- GitHub issue `#53`, the original story-loop contract;
- pull request `#54`, the retired implementation; and
- the MEM-1.3 trial that exposed thread-writer conflicts, nested orchestration,
  schema drift, unsafe terminalization of retryable failures, and incomplete
  review recovery.

## Preserved principles

### Fresh context is a correctness property

The implementation worker persists across repairs because it owns the working
context. Reviewers never persist across candidate commits. Every changed
candidate receives three new lens contexts.

### Deterministic checks remain the ground truth

Model review does not replace commands such as tests, vetting, builds, and
whitespace checks. The controller runs configured checks independently and
binds their results to the exact candidate commit.

### Agent outputs are untrusted input

Every result is validated against a versioned JSON Schema and the current
workflow phase. Correct JSON with the wrong candidate, role, or thread identity
is invalid.

### Bounds do not imply success

Exhausting retries, passes, tokens, or time produces an incomplete or stalled
result. It never promotes the latest candidate to ready.

## Agent topology

The controller is the sole lifecycle owner. No SDK thread launches another
workflow thread.

```text
controller process
├── persistent implementation thread
│   ├── initial implementation
│   └── focused repair turns
└── review pass for exact candidate N
    ├── fresh contract lens thread       ┐
    ├── fresh correctness lens thread    ├─ concurrent
    ├── fresh maintainability lens thread┘
    └── fresh synthesis thread           ← runs after all lenses finish
```

The synthesis thread is necessary because application code cannot honestly
perform the semantic validation and deduplication currently performed by the
`review-story` coordinator. It receives the authoritative sources, exact
candidate, deterministic results, and the three structured lens results. It is
read-only, launches no children, and returns the overall review-result schema.

The synthesis result is evidence, not a vote count. One retained material
finding blocks readiness.

## Workflow

```text
INITIALIZE
  → IMPLEMENT
  → VALIDATE_CANDIDATE
  → RUN_CHECKS
  → RUN_THREE_LENSES
  → SYNTHESIZE_REVIEW
      READY_FOR_HUMAN_REVIEW → COMPLETE_LOCAL
      DECISION_REQUIRED      → STOP_DECISION
      REVIEW_INCOMPLETE      → RETRY_MISSING_REVIEW_ONCE or STOP_INCOMPLETE
      CHANGES_REQUIRED       → REPAIR_WITH_SAME_WORKER
  → VALIDATE_REPAIRED_CANDIDATE
  → RUN_CHECKS
  → RUN_THREE_FRESH_LENSES
  → ...
```

Allow at most three review passes: the initial candidate and two repaired
candidates. An incomplete check or lens may be retried once while the candidate
head remains unchanged. A retry does not consume a repair pass.

## Candidate invariants

Before review or deterministic validation, require:

- the exact expected worktree and story branch;
- a clean worktree;
- a full candidate `HEAD` SHA;
- ancestry from the frozen base commit;
- a candidate different from the base for an implementation result;
- a repaired candidate that is a new descendant of the reviewed candidate; and
- a candidate-result object whose story, checks, coverage, and finding
  dispositions match the active phase.

Review and validation results are valid only for the recorded candidate SHA.
Any worktree change after a phase begins invalidates that phase's evidence.

## Durable state and receipts

Store controller state outside the candidate diff under the repository Git
common directory. Each run has:

- one atomically replaced current-state file;
- one append-only receipt per completed phase;
- one exclusive ownership lock;
- frozen configuration and source identities; and
- SDK thread identities and last reconciled turn identities.

Persist the completed structured model result before advancing the phase. On
resume, reconcile persisted state with SDK history and repository state before
starting another model turn. Never infer that a model turn must be repeated
merely because the candidate commit did not advance; decision and incomplete
results are completed turns too.

Retryable operational failures, including an active-thread writer conflict,
must remain retryable. They must not be converted into terminal workflow
verdicts by a catch-all exception path.

## Deterministic validation

Freeze validation commands at run creation. Execute them from the exact
candidate worktree, record command, exit code, bounded output, duration, and
candidate SHA, and check the worktree identity again afterward.

Run every command in its own process group. On timeout, terminate and reap the
complete process tree before repair or review can begin. A surviving child
process could mutate the candidate after its evidence was recorded.

Required check failure contributes to `CHANGES_REQUIRED`; an unavailable or
unsafe-to-run check contributes to an incomplete stop. Neither can produce
readiness.

## Stop conditions

Stop safely when:

- an approved source requires a product decision;
- any required schema, role, candidate, or source identity is invalid;
- required verification cannot complete honestly;
- a review remains incomplete after its one retry;
- the implementation worker reports a blocked or disputed material finding;
- a repair does not produce a clean new descendant commit;
- the same material failure survives the bounded repair attempts;
- the third review still requires changes;
- durable state cannot be reconciled; or
- another process owns the run.

Never start a replacement implementation worker or silently create another
worktree to recover from these conditions.

## Initial story boundary

### DEVX-2: Rebuild the durable reviewed story loop

#### Outcome and value

Provide a repository-local controller that moves one approved story through
implementation, deterministic validation, fresh independent review, focused
repair, and re-review until one exact local candidate is ready for human review
or the run reaches an explicit safe stop.

#### Dependencies

- The current `implement-story`, `review-story`, and `deliver-story` changes and
  their schemas are reviewed, committed, and merged.
- The retired `tools/story-loop/` implementation is removed from the active
  tree while remaining available through Git history.
- A locally authenticated Codex SDK runtime is available for a read-only smoke
  check.

#### In scope

- A repository-local Python CLI/controller under `tools/story-loop/`.
- One persistent implementation thread for initial work and repairs.
- Three concurrent fresh read-only lens threads for every candidate.
- One fresh read-only synthesis thread after every lens set.
- Direct controller ownership of every SDK thread.
- Candidate, lens, and review-result schema validation.
- Controller-owned deterministic validation bound to exact commits.
- Atomic state, append-only receipts, exclusive run ownership, and safe resume.
- Three review passes, two repair passes, and one retry for an incomplete review
  operation.
- A fake backend for deterministic tests without model or network access.
- A read-only Codex SDK authentication and startup smoke command.
- Operating documentation for start, status, resume, and safe stops.

#### Out of scope

- Push, pull-request creation or editing, comments, approval, readiness changes,
  and merge.
- GitHub issue planning or multi-story scheduling.
- A hosted service, CI runner, or generic workflow engine.
- Evie as the agent-execution backend.
- Product changes belonging to the live demonstration story.
- Changes to the workflow skills or schemas inside the controller pull request.

#### Acceptance criteria

- Starting a run durably records the exact story, frozen base, configured
  checks, source identities, and implementation thread before model work
  proceeds.
- Resuming continues the same implementation thread and does not repeat a
  durably completed model or validation phase.
- Every review pass starts three new direct lens threads against the same exact
  candidate and runs them concurrently.
- A fresh synthesis thread validates and deduplicates the lens evidence without
  launching children or editing repository state.
- Malformed output, incorrect roles, missing lenses, stale SHAs, and source
  mismatches stop safely.
- Deterministic commands run against the exact candidate, and a timeout
  terminates and reaps the complete process tree.
- `CHANGES_REQUIRED` sends only validated findings and failed-check evidence to
  the same implementation thread.
- A repaired candidate must be a clean new descendant and receives three fresh
  lens threads plus a fresh synthesis thread.
- `READY_FOR_HUMAN_REVIEW` is accepted only when the schema-enforced readiness
  conditions, deterministic checks, and repository identity checks all pass.
- State and receipts survive interruption between every phase without silently
  repeating completed work.
- A second process cannot own the same run concurrently.
- The controller never exceeds its bounds, resolves a product decision, pushes,
  opens a pull request, approves, or merges.

#### Verification

```sh
uvx ruff format --check tools/story-loop
uvx ruff check tools/story-loop
uv run --project tools/story-loop \
  python -m unittest discover -s tools/story-loop/tests -v
uv run --project tools/story-loop evie-story-loop smoke --repo .
git diff --check
go test ./...
go vet ./...
```

The smoke command must be read-only. All behavioral tests use the fake backend.

#### Manual demonstration

Run one separate dependency-ready backlog story through the controller and
show:

- the persistent implementation-thread identity;
- three fresh lens identities and one fresh synthesis identity per pass;
- exact candidate SHAs and deterministic results;
- at least one durable phase receipt; and
- the final ready or safe-stop result.

The demonstration story remains its own task, branch, and pull request.

#### Risks and approved decisions

- The Codex SDK is the initial execution backend; Evie is deferred.
- The controller, not an SDK thread or skill, owns agent lifecycle.
- Review synthesis uses a fresh fourth read-only thread after the three lens
  threads complete.
- Skills define policy; the controller enforces lifecycle, persistence, and
  validation.
- Provider work already sent remotely cannot be recalled, so stale or
  ownership-lost results must be discarded.
- Structured output is untrusted even when it validates syntactically.
- No review verdict survives a changed candidate SHA.

#### One-PR boundary

Controller state machine, Codex SDK backend, deterministic command runner,
state and receipts, schema integration, fake-backend tests, read-only smoke
command, and operating instructions. No GitHub delivery and no product-story
implementation.

## Follow-up boundary

After DEVX-2 is proven, a separate story may add non-force push and exact-SHA
draft-pull-request delivery. That story must not weaken the local readiness
gate, take ownership of planning, approve, mark ready, or merge.
