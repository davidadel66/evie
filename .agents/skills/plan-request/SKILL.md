---
name: plan-request
description: Convert a product or engineering request or large active spec into PR-sized delivery work, write approved initiative or epic plans, and materialize one selected story as a ready GitHub execution-contract issue. Use when asked to plan, decompose, scope, stage, select, or prepare work for implementation. Do not use for implementation or pull-request review.
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

### Gate 3: Select one story and create its execution contract

Enter this gate only when the user explicitly selects exactly one story from an
approved plan and authorizes creation of its GitHub issue.

- Read the approved story summary, applicable specification and decisions,
  dependencies, current code seams, and existing GitHub issues.
- Refuse a duplicate issue. Return the existing issue when it already represents
  the selected story.
- Expand the summary into the complete contract defined by
  `docs/request-planning.md`; do not add behavior absent from approved sources.
- Check the contract against the definition of ready before creating the issue.
- Stop without creating an issue when a missing choice materially affects
  behavior, security, persistence, recovery, concurrency, or a public interface.
- Create one GitHub issue only after the story is ready. Do not assume labels,
  milestones, or project-board fields that the repository does not define.
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
8. Check every proposed story against the repository's definition of ready.
9. Present the proposal for approval and recommend the smallest useful first
   story.
10. If the user explicitly approves writing the planning artifacts, create only
    the approved initiative and epic files, verify them, and stop.
11. When the user later selects exactly one approved story and authorizes its
    GitHub issue, build and validate its full execution contract.
12. Create or return that one issue and stop. Implementation belongs to
    `$implement-story`.

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

- the selected story and authoritative sources;
- the GitHub issue URL, or the readiness blocker;
- the outcome, non-goals, acceptance criteria, verification, dependencies, and
  one-PR boundary recorded in the issue; and
- the exact `$implement-story` invocation to use next when the issue is ready.
