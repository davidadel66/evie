---
name: implement-story
description: Implement one ready engineering story in an isolated Git worktree, verify it against its acceptance criteria and deterministic checks, and return one clean committed candidate for independent review. Use when asked to implement or execute a single approved story or issue. Do not use to plan an epic, combine multiple stories, review a candidate, repair review findings, create a pull request, or merge changes.
---

# Implement a Story

Treat one ready story as the complete execution contract. Read the applicable
`AGENTS.md`, story, specification, decision record, and only the code needed to
understand the existing behavior before editing.

Read `references/candidate-result.schema.json` before beginning. Every exit from
this skill must return exactly one JSON object that validates against it, with
no prose or Markdown outside the object. Keep every required field present; use
empty strings or arrays when a value is not applicable.

Invoking `$implement-story` explicitly, or invoking a wrapper command that says
it uses this skill, authorizes creation of one story worktree and branch. Commit,
but do not push or create, edit, approve, or merge a pull request. Independent
review, review repair, and delivery belong to separate workflows.

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

## Gate 5: Finalize the candidate

After deterministic checks pass, confirm the diff contains only this story and
commit the candidate with a descriptive message. Confirm the worktree is clean
and record the full `HEAD` SHA. Do not amend the candidate after recording it.

Return the exact candidate identity and evidence so a fresh reviewer can assess
it without relying on the implementation conversation. Do not invoke
`review-story`, spawn reviewers, repair anticipated findings, push the branch,
or deliver the change from this skill.

## Handoff

Return `CANDIDATE_READY` only for a clean committed candidate whose reported
identity and checks you observed directly. Map every acceptance criterion to
code and test evidence. For the initial implementation, return an empty
`finding_dispositions` array.

Return `DECISION_REQUIRED` when an authoritative choice is needed, including
the exact question, context, and known options. Return
`IMPLEMENTATION_INCOMPLETE` for every other honest stop, with the completed
checks and known gaps. Never claim readiness through the summary field.
