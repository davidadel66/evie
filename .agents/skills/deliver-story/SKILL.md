---
name: deliver-story
description: Deliver one approved engineering story through initial implementation, independent three-lens review, bounded repair by the same persistent implementation worker, and a draft pull request for human review. Use when asked to deliver or run the reviewed implementation workflow for a single ready story. Do not use to plan work, combine stories, make product decisions, approve a pull request, or merge changes.
---

# Deliver a Story

Coordinate execution; do not edit product files yourself. Keep implementation
and review in separate agent contexts, preserve one exact candidate identity at
each review pass, and never merge a pull request.

Read `references/delivery-result.schema.json` before beginning. Every exit from
this skill must return exactly one JSON object that validates against it, with
no prose or Markdown outside the object. Keep every required field present; use
empty strings, arrays, or `null` only where the schema permits them.

Explicit invocation of `$deliver-story` authorizes one non-force push of the
reviewed story branch and creation of one draft pull request for that story. It
does not authorize force-pushes, edits to an existing pull request, comments,
review submission, approval, readiness changes, or merge.

## Own the agent topology

Remain the sole workflow coordinator.

- Spawn exactly one direct implementation worker and retain its agent identity
  for the entire story.
- Instruct that worker to apply the project `implement-story` skill for the
  initial candidate.
- Apply the project `review-story` skill yourself. Directly spawn its contract,
  correctness, and maintainability reviewers; do not insert an intermediate
  review coordinator.
- Never allow the implementation worker or a lens reviewer to spawn or replace
  the workflow coordinator.

Run review only while the implementation worker is idle. Run repair only after
all lens reviewers for the prior candidate have finished or been stopped.

## Obtain the initial candidate

Give the implementation worker exactly one approved story contract and its
authoritative sources. Require one result matching
`../implement-story/references/candidate-result.schema.json`. Reject malformed
or incomplete output instead of inferring omitted values.

Accept `CANDIDATE_READY` only with a clean committed candidate containing its
worktree, branch, base commit, full head commit, acceptance coverage, and
deterministic check evidence. Stop on `DECISION_REQUIRED` or
`IMPLEMENTATION_INCOMPLETE`; do not begin review.

Reject a candidate whose worktree is dirty, whose reported head is not the
actual full `HEAD`, or whose branch and base do not match the prepared story
workspace. Do not repair or reinterpret an invalid handoff on the worker's
behalf.

## Review and route the verdict

Review the exact candidate using `review-story` and its shared result schema.
Route its verdict as follows:

- `READY_FOR_HUMAN_REVIEW`: stop the repair loop and preserve the reviewed head.
- `DECISION_REQUIRED`: stop and request an authoritative user decision. Never
  turn the ambiguity into an implementation instruction.
- `REVIEW_INCOMPLETE`: stop or retry only the unavailable review operation. Do
  not ask the implementation worker to change code without a validated finding.
- `CHANGES_REQUIRED`: send the validated findings to the same implementation
  worker as a follow-up task.

## Repair with the persistent worker

For `CHANGES_REQUIRED`, provide the worker only:

- the exact reviewed base and head commits;
- each validated finding with its ID, evidence, impact, and required change;
- the unchanged story, specification, decisions, and non-goals; and
- the deterministic checks that must be rerun.

Require the worker to verify it still owns the same worktree and branch, repair
only in-scope findings, rerun affected and required checks, and create a new
commit without amending the reviewed commit. Require a disposition for every
finding: `ADDRESSED`, `DISPUTED`, or `BLOCKED`, with evidence.

Require the repair follow-up to use the same candidate-result schema. Reject
`CANDIDATE_READY` unless `finding_dispositions` contains exactly one entry for
every routed finding and no unrelated finding IDs.

Accept the repair only when the worktree is clean and the returned full head
commit is a descendant of and different from the reviewed head. Do not invoke
`implement-story` from scratch, create a replacement worker, or prepare another
worktree for a repair pass.

Send every accepted repaired candidate through a new `review-story` pass with
three fresh lens reviewers. No verdict carries across a changed head commit.

## Bound the loop

Allow at most three review passes: the initial candidate plus at most two
repair-and-review cycles. Never extend the limit automatically or restart the
workflow with a new implementation worker.

For each pass, record its number, exact head commit, review verdict, retained
finding IDs, repair dispositions, and resulting commit. Stop before the limit
when:

- the worker returns the reviewed head again, a dirty worktree, or a commit that
  is not its clean descendant;
- the same material failure remains after two consecutive repair attempts;
- the worker reports a finding as `BLOCKED` or identifies a required product
  decision; or
- required deterministic verification cannot complete honestly.

Identify a repeated finding by the same violated authoritative rule and failure
scenario, not by reviewer wording, title, line number, or generated ID alone.

For `REVIEW_INCOMPLETE`, retry the unavailable check or lens at most once while
the candidate head remains unchanged. Use a fresh context for a retried lens.
If the retry is incomplete, stop and return the missing evidence; do not consume
a repair cycle or change code.

If the third review pass still returns `CHANGES_REQUIRED`, stop with the exact
candidate, remaining findings, completed checks, and repair history for human
direction.

## Deliver the exact reviewed candidate

Proceed only on `READY_FOR_HUMAN_REVIEW`. Immediately before delivery, verify
that the worktree is clean, its branch is the recorded story branch, and its
full `HEAD` is the exact reviewed head. Verify that the head descends from the
recorded base commit and that the pull-request base still matches the prepared
story workspace. Any mismatch invalidates the verdict; stop without pushing.

Check for an existing open or draft pull request from the story branch. If its
base, head branch, and remote head already match the reviewed candidate, return
it unchanged and do not create a duplicate. If a pull request exists but does
not match, stop without updating it. Otherwise:

1. Push the recorded story branch to its configured remote without force.
2. Stop if the push is not a fast-forward or the remote, authentication, or
   pull-request base is unavailable.
3. Create one draft pull request against the recorded pull-request base.
4. Include the story reference and outcome, exact base and head commits,
   acceptance coverage, deterministic checks, review-pass history, resolved and
   deferred findings, residual risks, and the best files to inspect first.
5. Confirm the returned pull-request URL and that its remote head commit equals
   the reviewed head. Do not modify the candidate after delivery.

Never convert the draft to ready, submit a GitHub review, approve, or merge.

## Handoff

Return `DELIVERED` only with a verified pull-request URL whose remote head is the
exact reviewed candidate. Embed the final review result and include every
review and repair pass in `review_history`.

Return `DECISION_REQUIRED` only for an authoritative product choice and identify
the decision phase and question in the blocker. Return `DELIVERY_INCOMPLETE` for
every other honest stop, including loop exhaustion and operational delivery
failures, with the precise phase, reason, and safe continuation commands when
available. Never claim delivery through the summary field.
