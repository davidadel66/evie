# A02 - Plan agent

Status: candidate, unapproved. Kind: visible primary agent.

## Purpose

Investigate a requested change and produce a reviewable implementation plan
without modifying the project.

## How OpenCode does it

OpenCode's `plan` profile denies edit-class tools except for one plan-file path,
denies delegation to the mutable `general` subagent, allows questions, and can
exit back to build mode. A long plan-mode reminder defines four phases:
exploration, design, review, and writing the plan file.

The pinned source is internally inconsistent: the design phase tells the model
to launch a `general` agent while the profile's permission rules deny that exact
delegation. EVIE should not reproduce this impossible workflow.

Important limitation: OpenCode still permits Bash in plan mode. Its reminder
says not to mutate, but the shell can write anywhere. Therefore OpenCode's plan
mode is behaviorally read-only, not mechanically read-only.

Sources:

- [`Agent` registry, `plan`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/agent/agent.ts#L156-L181)
- [`plan-mode.txt`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/session/prompt/plan-mode.txt)

## Proposed EVIE adaptation

EVIE should only call this profile read-only if the harness can prove it:

- hide and reject every file/database/external mutator;
- do not expose unrestricted Bash;
- expose typed read/search/Git-inspection tools, or a separately constrained
  read-only command runner;
- permit writing only one harness-owned plan artifact outside the project tree;
- require an explicit user decision before transitioning to `build`;
- create a new build turn or profile transition event so the authority change is
  visible in history.

The plan artifact should contain scope, relevant files, ordered changes,
verification, risks, and open decisions. It must not pretend unresolved product
choices are implementation details.

## Workflow

1. Restate the outcome and identify ambiguity.
2. Explore the code and existing tests without mutation.
3. Ask only decisions that materially change the implementation.
4. Produce one recommended plan, not an unranked option dump.
5. Persist the plan with project revision/hash metadata.
6. Wait for approval; never transition itself into write authority.

## Acceptance evidence

- Attempts to call `edit_file`, `write_file`, mutable Bash, or write-capable
  subagents fail mechanically.
- The only writable path is the assigned plan artifact.
- The plan records the source revision it was based on and warns if stale when
  build begins.
- Plan approval is distinct from approval of every future side effect.

## Open decisions

1. Can a useful cross-language read-only shell be constrained safely enough, or
   should plan wait for typed glob/grep/directory-read/Git tools?
2. Should plans live under `~/.evie/plans`, inside the project, or in SQLite?
3. Does plan approval start a new child session or continue the same transcript
   with a profile-transition event?
