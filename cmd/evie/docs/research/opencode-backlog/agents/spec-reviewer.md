# A08 - Spec reviewer

Status: EVIE-specific candidate, unapproved. Kind: read-only subagent.

## Purpose

Try to make an executor's hidden guesses visible before implementation begins.

## Origin

This is not an OpenCode built-in. It comes from EVIE's successful `web_fetch`
and `web_search` workflow, where fresh-context spec review found ambiguities
before code existed. OpenCode's profile/permission/subagent machinery shows how
to make that role a real constrained session.

## Inputs

- Frozen spec content and hash.
- Relevant binding project instructions.
- Explicit product constraints supplied by David.
- No implementation diff or executor transcript.

## Capabilities

- Read only the assigned spec and explicitly approved reference documents.
- Ask focused clarification questions through the parent.
- No Bash, project-wide read by default, writes, tests, or child agents.

Scoping only by tool name is insufficient: unrestricted `read_file` could inspect
implementation and unrelated files. The role needs a resource allowlist or a
prepared input bundle.

## Output contract

Each finding contains:

```text
severity: blocking | important | minor
spec location
ambiguous or missing behavior
why an executor would have to guess
one recommended resolution
```

The final verdict is `READY`, `READY WITH NON-BLOCKING FINDINGS`, or `BLOCKED`.
The parent stores full findings as an artifact and receives a bounded verdict.

## Acceptance evidence

- The reviewer cannot inspect implementation files.
- A clean verdict identifies the exact spec revision reviewed.
- Findings distinguish product ambiguity from implementation preference.
- Blocking findings stop the automated pipeline until resolved or explicitly
  waived.

## Open decisions

1. Can a model reliably assign blocking severity, or must David make the final
   block/non-block choice?
2. Which reference documents may the reviewer request beyond the spec?
