---
name: implement-story
description: Implement one ready engineering story in an isolated Git worktree, iterate against its acceptance criteria and deterministic checks, obtain a fresh three-lens read-only review, and deliver a draft pull request for human review. Use when asked to implement, execute, or deliver a single approved story or issue. Do not use to plan an epic, combine multiple stories, review an existing pull request, or merge changes.
---

# Implement a Story

Treat one ready story as the complete execution contract. Read the applicable
`AGENTS.md`, story, specification, decision record, and only the code needed to
understand the existing behavior before editing.

Invoking `$implement-story` explicitly, or invoking a wrapper command that says
it uses this skill, authorizes creation of one story worktree and branch. Commit,
push, and draft-pull-request creation are authorized only when that invocation
also states that delivery contract. Never merge a pull request.

## Gate 1: Confirm readiness

Require:

- one independently reviewable outcome;
- explicit acceptance criteria and non-goals;
- applicable specification and decision references;
- enough evidence to identify honest implementation and test boundaries; and
- a known base ref for the change.

Stop before creating a worktree when a missing choice materially affects
behavior, security, persistence, recovery, concurrency, or a public interface.
Do not silently expand or reinterpret the story.

## Gate 2: Prepare the isolated worktree

Run the bundled helper from the current repository checkout:

```sh
bash .agents/skills/implement-story/scripts/prepare-worktree.sh \
  --story <story-id-or-short-name> \
  --base <base-ref>
```

Use the story ID plus a short outcome when available. Omit `--base` only when
the current committed `HEAD` is the intended base.

The helper either prepares the current linked worktree or creates
`.worktrees/<story-slug>` with branch `codex/<story-slug>`. Record its
`worktree`, `branch`, `base_ref`, `base_commit`, and `pr_base` output.

After preparation:

- target every product file operation at the returned worktree;
- set that path as the working directory for every shell command;
- use absolute paths under that worktree for patch operations;
- verify `git -C <worktree> status --short` before and after edits; and
- never edit product files in the checkout that launched the skill.

Treat an unexpected existing path, branch, or registration as a blocker. Never
delete, reset, clean, or overwrite it to make preparation succeed.

## Gate 3: Establish the baseline

Inspect the relevant code paths and run the smallest meaningful pre-change
checks. Record failures that already exist. When practical, add or identify a
test that demonstrates the missing behavior before implementation.

Do not change an approved test merely to make the implementation pass unless
the story explicitly requires changing that test contract.

## Gate 4: Implement and verify

Repeat this bounded loop:

1. Make the smallest change that advances one acceptance criterion.
2. Format affected files and run targeted tests or checks.
3. Inspect the diff against the story, specification, decisions, and non-goals.
4. Fix actionable failures without widening the story.

Exit the loop when all acceptance criteria pass, or stop with evidence when:

- the same blocking failure survives three materially different attempts;
- required verification cannot run;
- the needed change exceeds the story boundary; or
- a new product decision is required.

### Pre-review safety audit

When the change touches authorization, persistence, concurrency, time,
cancellation, or callbacks, complete this audit before the first review:

- Map every applicable binding rule to an enforcing code boundary, a rejection
  path, and deterministic test evidence.
- Build an operation-by-state matrix using every relevant state, including
  absent, active, closed, released, expired, replaced, restarted, exact-boundary,
  and invalid-input cases.
- Identify the serialization point. Sample time or mutable state used for an
  authorization decision only after acquiring the applicable lock or
  transaction ownership, and exercise queued cross-connection behavior when
  concurrency is in scope.
- Treat every callback, interface, executor, database handle, and transaction
  handle as an adversarial capability. Ensure callers cannot bypass fencing,
  control the transaction, escape the intended resource scope, or reuse the
  capability afterward. Prefer typed operations over raw execution surfaces.
- Define cancellation behavior immediately before and during commit. Test both
  rollback-before-commit and definitive commit outcomes when applicable.
- Enforce correlated durable invariants in the schema or persistence boundary,
  not only in constructors, and test restart or reload behavior.
- Test monotonic and non-shortening rules in both directions plus zero,
  negative, maximum, and overflowing inputs when the domain permits them.
- Distinguish persisted observations from proof of current authority in API
  names, documentation, and tests.

Do not advance until each applicable rule has enforcement plus negative or
boundary-test evidence, or a recorded reason deterministic coverage is not
possible.

Before review, run every check required by the applicable `AGENTS.md`. Do not
substitute model judgment for deterministic verification.

## Gate 5: Obtain an independent review

After deterministic checks pass, confirm the diff contains only this story and
commit the candidate with a descriptive message. Then load and apply the project
`review-story` skill to that clean commit, the base commit, story, specification,
decisions, and recorded verification. That workflow must run fresh contract,
correctness, and maintainability lenses without receiving the implementer's
conclusions.

Do not proceed on `DECISION_REQUIRED`, `CHANGES_REQUIRED`, or
`REVIEW_INCOMPLETE`. Fix valid in-scope findings, rerun affected deterministic
checks, create a new commit without amending the reviewed commit, and request
one follow-up review only when the fixes materially changed the candidate. Limit
the independent review loop to two passes. Never resolve a contract gap in code;
return it for an authoritative decision.

## Gate 6: Deliver the draft pull request

Proceed only while all required checks pass and no material review finding
remains.

1. Confirm the worktree is clean and `HEAD` is the exact reviewed commit.
2. Push the story branch without force.
3. Open a draft pull request against `pr_base`.
4. Include the story, outcome, verification evidence, risks, and deferred work
   in the pull-request body.

If GitHub access, authentication, a remote, or `pr_base` is unavailable, stop
after the safest completed local step and give exact continuation commands. Do
not claim a pull request exists without its URL.

## Handoff

Return:

- the draft pull-request URL, or the precise delivery blocker;
- the story and specification sections implemented;
- the worktree, branch, and base commit;
- what changed and why;
- exact verification and independent-review results;
- known gaps and deliberately deferred work; and
- the best files for the human reviewer to read first.
