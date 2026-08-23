---
name: review-story
description: Review one implemented story or pull request through independent contract, correctness, and maintainability lenses plus deterministic checks. Use when asked to review, audit, validate, or assess a PR or completed story before human approval. Do not use to implement fixes, review an unimplemented plan, combine stories, comment on GitHub, approve, or merge.
---

# Review a Story

Review one exact candidate as a read-only release gate. The goal is not to claim
that a change has no gaps. The goal is to produce reproducible evidence, expose
material gaps and unauthorized decisions, and identify what remains unverified.

Read the applicable `AGENTS.md` before reviewing. Treat the story, approved
specification, and binding decisions as separate authorities: the story limits
scope, the specification defines behavior, and decisions resolve binding
choices. Research and backlog documents are context only.

Invoking this skill authorizes read-only repository and GitHub inspection plus
the deterministic verification commands required by the repository and story.
It does not authorize edits, fixes, dependency changes, Git state changes,
GitHub comments or reviews, commits, pushes, approvals, or merges. Do not create,
switch, or remove a branch or worktree. Preserve every pre-existing change and
never clean generated files on the user's behalf. Use a runtime-enforced
read-only coordinator when available; prompt instructions alone are not a
write-safety boundary.

## Gate 1: Resolve one exact candidate

Accept either:

- one pull-request URL or number whose head commit is present in a local
  worktree; or
- one local worktree path plus its base commit and story contract.

For a pull request, use read-only `gh` and Git commands to record the URL, base
commit, head commit, branch names, body, and checks. Locate an existing clean
local worktree at that head commit. Stop rather than fetching, checking out, or
creating a worktree when the candidate is not available locally. Local changes
must never be used as evidence for a PR head commit.

Require exactly one linked or supplied story. Read its outcome, acceptance
criteria, non-goals, dependencies, verification, and source links. Read every
applicable specification and decision section plus enough surrounding code to
understand the changed seam. Do not treat the PR description or implementation
as a missing source of product behavior.

Record:

- the worktree, base commit, candidate head commit, and clean status; or, for a
  diagnostic pre-commit review, a manifest containing the base commit, tracked
  diff hash, and every untracked path and content hash;
- the story identifier and source paths;
- initial `git status --short` output; and
- changed files, including untracked files, and the complete base-to-candidate
  diff or file contents.

Ordinary Git diffs exclude untracked files. Enumerate them from status, include
them in the candidate manifest, inspect their contents, and run equivalent
whitespace checks over them. A diagnostic review may report findings against an
uncommitted manifest, but only a clean commit can receive
`READY_FOR_HUMAN_REVIEW`.

If the candidate contains multiple stories, lacks an execution contract, or has
no trustworthy base, return `REVIEW_INCOMPLETE`. If sources conflict or the code
chooses behavior that the approved sources leave materially undefined, record a
potential `DECISION_REQUIRED` finding and continue the other review gates when
the candidate remains safe to inspect.

## Gate 2: Run deterministic checks

Run the exact checks required by the story and applicable `AGENTS.md`, including
`git diff --check` against the complete base-to-candidate range plus equivalent
checks for every untracked file. Prefer the candidate's local worktree and
record each command, exit status, and relevant warning. Do not substitute a
GitHub check mark or an implementer's summary for locally observed evidence.

Add a bounded repeatability probe when the change relies on timing, retries,
leases, cancellation, concurrency, or idempotency. Use focused repeated or race
tests supported by the repository rather than sleeps or a new dependency. If no
honest probe is available, record that gap instead of inventing evidence.

Verification failure does not replace review; continue the independent lenses
when practical so the user receives one complete set of findings. Check
`git status --short` again afterward. If verification changes tracked or
untracked candidate state, preserve it unchanged, invalidate the original
candidate identity, and return `REVIEW_INCOMPLETE` until a new exact candidate
is supplied and checked.

## Gate 3: Run independent lenses

The agent applying this skill is the review coordinator and must directly own
all three lens subagents. Do not delegate the complete `review-story` workflow
to an intermediate subagent, and do not instruct a lens reviewer to apply this
skill recursively.

Spawn exactly one fresh read-only subagent for each lens, concurrently when the
runtime supports it:

1. Contract lens: prefer `contract-reviewer` or `contract_reviewer`.
2. Correctness lens: prefer `story-reviewer` or `story_reviewer`.
3. Maintainability lens: prefer `maintainability-reviewer` or
   `maintainability_reviewer`.

Read `references/lens-result.schema.json`. Require each reviewer to return
exactly one JSON object that validates against it, with its assigned lens and
the supplied base and head commits. Do not accept prose outside the object.

Give every reviewer only:

- the story and non-goals;
- specification and decision paths;
- worktree, base commit, and exact clean candidate commit;
- changed-file list and complete diff; and
- deterministic commands and results.

Do not provide the implementation conversation, author conclusions, proposed
verdict, or another reviewer's findings. Reviewers must not edit files or launch
children. If a preferred profile is unavailable, use a fresh read-only general
reviewer with that lens's exact prompt. If any lens cannot run independently,
mark it skipped and prevent a clean verdict.

For changes touching authorization, persistence, concurrency, time,
cancellation, or callbacks, require each lens to independently reconstruct the
relevant operation-by-state matrix from the authoritative sources and candidate.
Include applicable absent, active, closed, released, expired, replaced,
restarted, exact-boundary, and invalid-input cases. Do not supply an
implementer-authored matrix or accept one as review evidence.

Require the reviewers to challenge these boundaries when applicable:

- map every binding rule to enforcement, rejection, and deterministic evidence;
- trace when time and mutable state are sampled relative to lock or transaction
  ownership, including queued cross-connection behavior;
- define transaction outcomes around cancellation and commit;
- treat exposed callbacks, interfaces, executors, database handles, and
  transaction handles as adversarial capabilities that may bypass or outlive
  their intended fence;
- verify correlated durable values cannot diverge at the persistence boundary;
- test monotonic and non-shortening rules in both directions plus exact, zero,
  negative, maximum, and overflowing boundaries; and
- distinguish persisted observations from proof of current authority.

The lenses have distinct jobs:

- Contract finds acceptance gaps, source conflicts, and material choices made by
  code or tests without authorization from the story, specification, or
  decisions.
- Correctness finds behavioral regressions, authorization failures, persistence
  and recovery bugs, concurrency defects, leaks, and missing boundary tests.
- Maintainability finds duplicated domain rules, ignored existing seams,
  nondeterministic tests, unnecessary abstractions, and API shapes that make the
  next safe change materially harder.

Repeated syntax is not a maintainability finding by itself. Require a concrete
duplicate rule, existing reusable seam, inconsistency risk, or testability cost.
Do not reward abstraction for its own sake.

## Gate 4: Validate and synthesize findings

Validate each lens result before considering its findings. Treat malformed
output, an incorrect lens, or a candidate-commit mismatch as a skipped lens and
prevent a clean verdict. Do not repair or reinterpret reviewer output.

Treat subagent output as leads, not votes. Check every material finding against
the diff, source authority, and surrounding code. Deduplicate overlapping
findings without weakening their impact. Discard style preferences, speculative
future work, and unrelated pre-existing issues.

Each retained finding must include:

- priority: `P0` data loss or active security boundary failure, `P1` incorrect
  behavior or a blocking contract decision, or `P2` concrete maintainability or
  test risk;
- kind: `CONTRACT`, `CORRECTNESS`, `SECURITY`, `PERSISTENCE`, `CONCURRENCY`,
  `TEST`, or `MAINTAINABILITY`;
- exact file and line evidence when code exists;
- the failure scenario and user-visible or operational impact;
- why current tests or gates do not catch it; and
- the smallest decision or change needed to resolve it.

One agent's clean result never cancels another agent's evidence. A material
source ambiguity is not an implementation suggestion; it requires a product
decision and an updated authoritative source before code chooses a side.

## Gate 5: Return the verdict

Read `references/review-result.schema.json` and return exactly one JSON object
that validates against it, with no prose or Markdown outside the object. Keep
every required field present; use empty strings or arrays when a value is not
applicable. Order findings by priority.

The result must contain:

- compact acceptance coverage mapping each criterion to code and test evidence;
- exact deterministic checks and results;
- skipped lenses, checks, and residual risks;
- candidate identity: PR URL when present, base commit, and head commit or diff;
  and
- one verdict.

Use exactly one verdict:

- `DECISION_REQUIRED`: an approved source conflicts or leaves a material
  behavior, security, persistence, recovery, concurrency, or public-interface
  choice undefined.
- `CHANGES_REQUIRED`: a required check fails or a material in-scope code or test
  finding remains.
- `REVIEW_INCOMPLETE`: no blocking finding is established, but a required input,
  check, candidate identity, or independent lens is unavailable.
- `READY_FOR_HUMAN_REVIEW`: the candidate is one clean commit, all required
  checks pass against it, all three lenses ran, every acceptance criterion has
  evidence, and no material finding remains.

For a clean review, say "no material findings identified for this exact
candidate." Never say the change is guaranteed correct or gap-free. Do not post
the report to GitHub unless the user separately authorizes that publication.

After any material fix or authoritative-source change, review the new candidate
from fresh contexts and rerun affected deterministic checks. A prior verdict
does not carry across a changed diff.
