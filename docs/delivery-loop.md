# Evie delivery loop

Status: active repository workflow

## Purpose

Deliver one independently reviewable outcome with the fewest model turns that
preserve product clarity, deterministic evidence, independent review, and human
control. CI and repository code enforce mechanical gates; agent instructions
guide judgment.

The loop is deliberately not a Codex skill or controller. Ordinary Codex tasks,
one small verification script, GitHub Actions, and a concise `AGENTS.md` are the
mechanism. Add a narrow skill only after the same repeatable gap appears across
multiple stories and cannot be handled more reliably by code or CI.

## Define one ready outcome

Before implementation, write or agree on:

- one user-visible or operational outcome;
- observable acceptance criteria;
- explicit non-goals;
- applicable specifications and decision records;
- dependencies and material risks;
- exact deterministic verification;
- the intended one-pull-request boundary.

The brief may be a GitHub issue, an active feature story, or a concise statement
in the task. Small changes do not need an epic and a duplicated execution
contract. Broader requests should be decomposed, but only the next
dependency-ready story needs implementation detail.

Split a story when it has separately demonstrable outcomes or crosses several
high-risk boundaries that can be delivered independently. Diff size is a
warning rather than a contract: reconsider the boundary when a change is
difficult to explain, test, or review as one result.

## Execute the loop

1. **Scope.** Read the brief, relevant specification, decisions, and existing
   seams. Batch unresolved product questions into one request. Stop if a missing
   decision would materially change behavior or public interfaces.
2. **Implement.** One implementation task makes the smallest complete patch.
   Add a regression test first when practical and run focused checks while
   iterating.
3. **Verify.** Run `./scripts/verify-change.sh`. A failed required check blocks
   handoff. Record warnings and explain every intentionally skipped check.
4. **Open a draft PR.** Do this only when requested. Use the repository template
   to map acceptance criteria to evidence. The GitHub `Verify` workflow is the
   merge gate; configure branch protection to require it.
5. **Review once.** Use one fresh, read-only reviewer against the exact diff,
   story, specification, and the review priorities in `AGENTS.md`. Codex
   `/review` is the default when available. Add a specialist review only for a
   distinct high-risk domain that the general review cannot assess adequately.
6. **Repair once.** The same implementation task fixes blocking, in-scope
   findings. Do not widen the story or perform opportunistic cleanup. Rerun
   focused checks and full verification.
7. **Re-review when material.** Request a fresh review only if the repair changed
   behavior or risk materially. If a blocking issue remains after the bounded
   repair, stop and ask for human direction.
8. **Hand off.** A human reviews the evidence, approves, and merges. Record the
   outcome before selecting the next dependency-ready story.

## Hard guardrails

- `scripts/verify-change.sh` runs `go test ./...`, `go vet ./...`, UI lint, UI
  build, and local staged and unstaged whitespace checks.
- `.github/workflows/verify.yml` installs locked UI dependencies, runs the same
  script on pull requests and pushes to `master`, and checks the complete pull
  request diff for whitespace errors.
- The pull request template requires outcome, acceptance evidence, exact
  verification, risks, deferred work, and manual demonstration.
- Repository rules should require the `Verify` check and human approval before
  merge. A prompt or model verdict is never a substitute for either.

## When parallel agents help

Do not parallelize the default implementation-review loop. Parallel agents are
appropriate when tasks divide cleanly and do not write the same files, such as
independent codebase investigations or separate dependency-ready stories in
isolated worktrees. Each worker must have explicit ownership and must not depend
on unmerged behavior from another worker.

## Epic 2 trial

Use the first small dependency-ready Epic 2 story as the initial trial. For each
of the first three stories, record:

- acceptance criteria met and required checks passed;
- issue/task start to draft-PR wall time;
- number of implementation and review turns;
- repair rounds and material review findings;
- changed production files and diff size;
- human-requested rework after handoff.

After three stories, compare the results with similar Epic 1 work. Keep the new
loop if quality is maintained and coordination time falls materially. If a
repeated failure appears, fix the narrowest layer responsible: tests or CI
first, repository instructions second, and a small skill only as a last resort.
