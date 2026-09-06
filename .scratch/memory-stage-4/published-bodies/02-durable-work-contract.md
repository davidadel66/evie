## Parent

[Memory Stage 4: Compile sourced memory candidates for owner review](https://github.com/davidadel66/evie/issues/131)

## What to build

Resolve the durable work and Compilation Coverage prerequisite using the evidence and closure contract. Deliver state transitions and worked race/restart scenarios that dependent persistence and worker changes can implement deterministically. This ticket records policy and invariants; it does not introduce a generalized workflow engine or authorize cleanup.

## Acceptance criteria

- [ ] Define source units and ordering across sessions, material Compiler Generation identity, selected ranges, activation frontier semantics, bounded historical selection, and history outside selection.
- [ ] Define atomic event/scheduling boundaries and activation/reconciliation races, including existing eligible history, disabled/unconfigured extraction, queue saturation, and rediscovery of selected uncovered evidence.
- [ ] Specify job identity, staging and completion idempotency, exact completed ranges, unresolved gaps, successful empty results, and a contiguous frontier that cannot cross unresolved work. Later independent extraction cannot depend on earlier unaccepted candidates.
- [ ] Specify attempts, retryable/terminal failures, repair-attempt accounting, lease replacement and fencing, cancellation, shutdown, explicit skip behavior if supported, and recovery after each durable boundary. Preserve the existing ceiling of five attempts with exponential backoff from five seconds to a ten-minute cap.
- [ ] Define lifecycle hosting, cross-process capacity ownership and release, one active local inference request, bounded input/output/queued/staged work and database batches, and new-evidence priority with explicit backfill fairness.
- [ ] Distinguish prompt client cancellation, prohibition of stale durable completion, and eventual server capacity release; define what happens when the server cannot immediately release capacity.
- [ ] Freeze retention and equivalent-suggestion/re-presentation behavior across generations for accepted, edited, rejected, and unresolved candidates. Preserve review history and accepted operations; do not authorize unspecified erasure.
- [ ] Work through failure-gap/later-success, duplicate delivery, racing activation, restart after staging, late stale completion, and generation-upgrade examples. Every observable state has a specified meaning.
- [ ] Resolve material open choices with the owner, update the binding record, and pass documentation whitespace/local-link checks. State any unresolved prerequisite explicitly.

## Blocked by

- [Freeze evidence, source-window, and closure rules](https://github.com/davidadel66/evie/issues/132)

