---
name: plan-request
description: Interactively plan, decompose, scope, audit, refine, or repair product and engineering work into PR-sized delivery stories, write approved initiative or epic plans, and materialize one selected story as a ready GitHub execution-contract issue. Use for new requests and existing backlog stories that must be validated before implementation. Do not use for implementation or pull-request review.
---

# Plan a Request

Read `docs/request-planning.md` completely from the repository root, then follow
it as the authoritative planning method. Also read the applicable specification,
decision record, Git status, and only the code needed to understand existing
behavior and honest change boundaries.

## Operating gates

### Gate 1: Propose

Use this gate by default, including the first time the skill is invoked.

- Inspect the repository without changing files.
- Present the proposed classification, hierarchy, and story boundaries.
- Stop for approval before creating or updating planning artifacts.

### Gate 2: Write approved planning artifacts

Enter this gate only after the user explicitly approves the proposal and asks
to create or update its planning files.

- Create only the approved structure defined by `docs/request-planning.md`.
- For a multi-epic initiative, create its index and epic files; keep proposed
  story summaries inside their epic files.
- Preserve approved specifications and decisions rather than moving, splitting,
  or rewriting them merely to fit the plan.
- Run documentation verification and report the files written.
- Stop after the planning artifacts are complete.

### Gate 3: Interactively refine one selected story

Enter this gate when the user selects exactly one proposed or existing story for
readiness work. Existing issues begin read-only.

- Read the approved story summary, applicable specification and decisions,
  dependencies, current code seams, and existing GitHub issues.
- Expand or audit the full contract using the interactive story-refinement
  method in `docs/request-planning.md`.
- Build the acceptance-to-boundary matrix and change-surface forecast from
  approved behavior and current code.
- Ask one focused material question at a time, lead with a recommendation, and
  revise the proposal from the user's answers.
- Use a fresh read-only contract challenger after the draft is resolved when
  independent contexts are available. Validate its evidence yourself.
- Return `STORY_READY`, `REVISION_REQUIRED`, `DECISION_REQUIRED`, or
  `DEPENDENCY_BLOCKED`. Do not create or update an issue in this gate.

### Gate 4: Materialize one approved ready story

Enter this gate only after the user explicitly approves the refined contract
and authorizes creating or updating exactly one GitHub issue.

- Refuse a duplicate issue. Audit and reuse the existing issue when it owns the
  selected story.
- Record the complete approved contract and readiness evidence defined by
  `docs/request-planning.md`; do not add behavior absent from approved sources.
- Reconfirm that no source, dependency, code seam, or base commit changed since
  refinement. Return to Gate 3 when readiness evidence is stale.
- Create or update one issue only after the story is `STORY_READY`. Do not close,
  replace, or split an existing issue without explicit approval.
- Do not assume labels, milestones, or project-board fields that the repository
  does not define.
- Return the issue URL and readiness evidence, then stop before implementation.

In all gates:

- Do not implement product code.
- Do not create or switch branches or worktrees.
- Do not commit, push, create GitHub issues, or open pull requests unless the
  active gate and user authorization explicitly permit the action.
- Do not bulk-create execution-contract issues for future or unselected stories.
- Treat working-tree changes as user-owned and preserve them.
- Ask only about unresolved choices that materially affect behavior, security,
  persistence, recovery, concurrency, public interfaces, or story boundaries.
  Ask one focused question per turn unless the user requests a batch.

## Workflow

1. Restate the requested outcome and identify explicit non-goals.
2. Locate applicable specifications, decisions, prior plans, and current
   implementation evidence.
3. Classify the request as a story, epic, initiative, or research spike. Do not
   force hierarchy onto small work.
4. Surface blocking product decisions or evidence gaps. Use a bounded research
   spike when implementation would otherwise depend on guessing.
5. Decompose the work according to `docs/request-planning.md`:
   - keep one independently verifiable outcome per story and pull request;
   - group related stories under one coherent epic;
   - use a feature directory and epic files for a multi-epic initiative;
   - keep proposed story summaries in epic files;
   - reserve the full execution contract for the selected GitHub story issue.
6. Reference approved specification sections instead of duplicating their
   behavior in delivery artifacts.
7. Order work by dependency and risk, accounting for work already present in the
   repository.
8. Check every proposed story provisionally against the repository's definition
   of ready.
9. Present the proposal for approval and recommend the smallest useful first
   story.
10. If the user explicitly approves writing the planning artifacts, create only
    the approved initiative and epic files, verify them, and stop.
11. When the user later selects one proposed or existing story, run the
    interactive refinement audit and obtain a fresh contract challenge.
12. Resolve decisions and revisions with the user until the selected story is
    `STORY_READY` or honestly blocked.
13. Only after separate approval to materialize it, create, update, or return
    that one issue and stop. Implementation belongs to `$implement-story`.

## Output

In Gate 1, return a concise planning proposal with:

- classification and rationale;
- outcome and non-goals;
- authoritative sources inspected;
- proposed initiative, epics, and story summaries at the appropriate depth;
- dependencies and recommended order;
- blocking decisions and research spikes;
- stories that are ready versus not ready, with reasons;
- recommended first story and its proposed one-PR boundary.

When the request is too ambiguous to decompose honestly, return the smallest set
of focused questions instead of manufacturing a detailed backlog.

In Gate 2, return the paths created or updated, documentation checks and their
results, and any approved items that could not be recorded.

In Gate 3, return:

- the readiness outcome and reasons;
- the acceptance-to-boundary matrix and change-surface forecast;
- blocking questions, proposed revisions or split, and a recommended choice;
- dependency, decision, and independent-challenge evidence; and
- the exact proposed GitHub contract when ready.

In Gate 4, return:

- the selected story and authoritative sources;
- the GitHub issue URL, or the readiness blocker;
- the outcome, non-goals, acceptance criteria, verification, dependencies, and
  one-PR boundary recorded in the issue;
- its reviewed base commit, change-surface forecast, boundary rationale,
  resolved decisions, and contract-challenge evidence; and
- the exact `$implement-story` invocation to use next when the issue is ready.
