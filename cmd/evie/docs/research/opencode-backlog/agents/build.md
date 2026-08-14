# A01 - Build agent

Status: candidate, unapproved. Kind: visible primary agent.

## Purpose

Own an implementation task end to end: inspect, change files, run commands,
verify the result, and report what actually passed.

## How OpenCode does it

`build` is OpenCode's default visible primary agent. It uses the selected model
unless configured otherwise and receives the model-family system prompt. Its
default permission set broadly allows tools, asks for external-directory access
and secret-like reads, asks on repeated identical tool calls, and enables the
question and plan-entry controls.

OpenCode does not treat `build` as a separate executor process. It is a profile
on the same persisted session loop.

Source: [`Agent` registry, `build`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/agent/agent.ts#L119-L155).

## EVIE today

The one EVIE session already behaves roughly like a build agent: it has
`read_file`, approval-gated `edit_file`, unrestricted Bash, and web research.
What is missing is project scope, durable state, bounded execution, recovery,
project instructions, and a mechanical definition of done.

## Proposed EVIE adaptation

`build` should remain the primary conversational owner, not a catch-all bypass:

- bind it to one canonical project root and session cwd;
- load trusted project instructions before acting;
- require explicit implementation authority for source changes;
- snapshot before mutation once F15 exists;
- maintain a visible task ledger for non-trivial work;
- run the project's configured verification gate before claiming completion;
- report skipped checks and deliberately accepted gaps;
- stop on exhausted turn, token, time, or cost budgets.

For the EVIE repository itself, the loaded `CLAUDE.md` tutor rule remains in
force. `build` may inspect and verify, but it does not gain permission to write
learning-relevant Go unless David explicitly authorizes that task.

## Tool posture

- Read/search/project inspection: allow.
- Existing-file edits and file creation: according to session write policy;
  retain human preview while attended.
- Bash: allow only under the shell policy and project scope selected for the
  session.
- Git commit/push/release: separate consequential permissions, never implied by
  ordinary file-write authority.
- Subagents: allow only after F12, with parent denials inherited.

## Completion contract

The agent may say the task is complete only when:

- requested artifacts exist;
- relevant deterministic checks ran and passed, or their omission is explicit;
- the final diff contains no unexplained unrelated changes;
- approval-gated changes actually executed;
- no tool execution remains pending or unknown.

## Acceptance evidence

- The profile cannot override project instructions.
- A failed verification command prevents an unqualified success claim.
- Budget exhaustion produces a failed/incomplete state, not a polished success
  summary.
- Existing user changes are preserved and distinguished from agent changes.

## Open decisions

1. What command or file defines a project's verification gate?
2. Is attended `build` allowed to write after one session-level grant, or must
   each prepared mutation remain approval-gated?
3. Which Git operations require their own approval class?
