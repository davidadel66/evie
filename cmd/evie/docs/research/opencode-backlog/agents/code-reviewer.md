# A10 - Code reviewer

Status: EVIE-specific candidate, unapproved. Kind: read-only subagent.

## Purpose

Adversarially inspect the final diff against the approved spec and identify
bugs, regressions, security issues, and missing tests without sharing the
executor's accepted assumptions.

## Origin

This is not an OpenCode built-in. Fresh-context review found four real bugs in
EVIE's `web_fetch` work, including a bug faithfully implementing a flawed spec.
OpenCode's profile and child-session mechanisms provide a runtime shape for the
role.

## Inputs

- Approved spec and hash.
- Binding project instructions.
- Base-to-candidate diff and changed-file contents.
- Deterministic gate report.
- No executor reasoning or conversational transcript.

## Capabilities

- Read-only access to candidate and necessary surrounding source/tests.
- Read-only Git diff/status operations.
- Optional bounded verification commands that cannot write project files.
- No edit/write, unrestricted Bash, task spawning, commit, or publication.

## Output contract

Findings first, ordered by severity. Each finding includes exact file/line,
failure scenario, impact, and why existing tests/gates did not catch it. A clean
review must state residual risks and what was not inspected.

The reviewer does not vote the build into correctness. Deterministic gates and
human live-fire remain separate evidence.

## Acceptance evidence

- The reviewer cannot alter the candidate tree.
- It receives no executor transcript.
- Findings cite concrete code, not general style preferences.
- Review fixes trigger gates and a new review/diff boundary.
- A clean verdict does not erase skipped verification or known gaps.

## Open decisions

1. Should a reviewer run tests, or only inspect already recorded gate evidence?
2. Does every review fix require a fresh reviewer session to avoid inherited
   assumptions?
