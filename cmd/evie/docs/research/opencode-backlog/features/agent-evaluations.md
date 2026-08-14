# F27 - Coding-agent evaluations

Status: candidate, unapproved. Priority: fixtures early, paid evals deliberately.

## Purpose

Measure whether profile, prompt, tool, context, and model changes improve real
development behavior instead of trusting one impressive transcript.

## OpenCode comparison

OpenCode has extensive deterministic unit/integration coverage and provider
fixtures, but no dedicated SWE-bench-style agent-quality suite was found at the
inspected revision. EVIE should copy the deterministic testing discipline and
build a smaller real-task evaluation set suited to its goals.

## Proposed EVIE adaptation

Use three layers:

### Deterministic runtime tests

- permission ceilings and hidden schemas;
- cancellation and process cleanup;
- retry and doom-loop behavior;
- execution-state crash recovery;
- path/symlink boundaries;
- compaction projection and tool pairing;
- snapshot/revert conflicts;
- parent/child depth and cancellation.

### Scripted trajectory fixtures

A fake provider emits known streams/tool calls. Assert exact event sequences,
tool availability, context composition, profile transitions, and final stage
state without paying for a model.

### Model-backed coding scenarios

10-20 tasks from real EVIE work, run deliberately with a build tag/command.
Score observable behavior:

- correct project instructions followed;
- appropriate tool/profile selection;
- no unauthorized mutation;
- gate outcome;
- spec-derived tests fail old/pass new;
- grounded reviewer findings;
- token/cost/latency and intervention count.

Do not exact-match prose. Preserve transcripts, model/profile versions, tool
schemas, project fixture revision, and pass criteria.

## Acceptance evidence

- `go test ./...` covers deterministic invariants without remote calls.
- A deliberate eval command reports each scenario and configuration.
- Changing model, prompt, or profile yields a comparable before/after report.
- Prompt-injection fixtures test repository/web/tool text as untrusted data.
- Failures retain enough evidence to diagnose the trajectory.

## Open decisions

1. First five real coding scenarios.
2. Which scores are fully mechanical versus human rubric?
3. Cost ceiling and model versions for deliberate eval runs.
