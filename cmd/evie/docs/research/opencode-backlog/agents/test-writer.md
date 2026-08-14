# A09 - Blind test writer

Status: EVIE-specific candidate, unapproved. Kind: isolated mutable subagent.

## Purpose

Derive executable expectations from the approved spec without seeing the new
implementation, then prove the tests detect its absence.

## Origin

This is not an OpenCode built-in. EVIE's existing design identifies blindness
plus fail-old/pass-new as the highest-value model-assisted verification step.
OpenCode contributes child sessions, profiles, tool state, and worktree ideas.

## Mechanical blindness

Prompting "do not read the implementation" is not enough. The child works in a
Git worktree at the pre-change base revision. If the new implementation file did
not exist there, it cannot inspect it. Existing implementation relevant to a
modification remains visible; the child is blind to the proposed diff, not to
the entire system it must test.

## Inputs

- Approved spec and hash.
- Binding project instructions.
- Pre-change source tree.
- Existing test conventions and verification contract.

## Capabilities

- Read/search within the old-tree worktree.
- Create/edit only designated test files and fixtures.
- Run bounded test/build commands in that worktree.
- No network by default, no parent/new-tree access, no commit/push, no subagents.

## Output contract

- Paths of new/changed tests.
- Exact old-tree command and failing output.
- Explanation of which required behavior each failure exercises.
- Any spec gap discovered while writing tests.
- No claim that tests pass on the implementation; that is the pipeline's job.

## Acceptance evidence

- New tests fail against the old tree for the intended reason.
- Empty tests, zero assertions, and unrelated compile failures do not satisfy
  the gate.
- The child cannot resolve paths into the implementation worktree.
- The test artifact applies cleanly to the implementation tree or reports a
  conflict for review.

## Open decisions

1. How does the gate distinguish an intended behavioral failure from a broken
   test or fixture?
2. Are modifications to existing tests permitted, or only new files?
