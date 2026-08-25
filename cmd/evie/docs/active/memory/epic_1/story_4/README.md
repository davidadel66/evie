# Story 4 - Provider usage evidence

Status: implementation candidate for issue #61; awaiting review and merge

## Outcome

Persist usage reported by the conversational provider as immutable,
provider-neutral turn evidence for later context and evaluation diagnostics.

## Sources

- [Epic 1](../README.md)
- [Session and event tables](../../../memory.spec.md#session-and-event-tables)
- [Stage 1](../../../memory.spec.md#stage-1---session-identity-and-append-only-events)
- [Binding memory decisions](../../../memory.decisions.md)

## Depends on

- Story 2's fenced terminal provider-event path, delivered by issue #57 and
  pull request #58.

## Acceptance summary

- Six approved optional provider counters are normalized identically for
  streaming and non-streaming responses, with missing, zero, malformed,
  duplicate, overflow, excluded, and repeated-container cases preserving their
  approved meanings.
- The final non-null stream usage occurrence replaces rather than merges earlier
  occurrences, including trailing usage-only chunks.
- Every successful provider iteration stores its own usage on the corresponding
  assistant payload in the same fenced append, including tool-calling
  iterations.
- Available, partial, zero, and absent usage survive event round trips and
  restart without changing provider history projection or incomplete-tool-group
  omission.
- Usage remains immutable episodic diagnostics. Provider-specific metadata,
  opaque continuation payloads, reasoning details, raw transport state,
  semantic compilation, policy, aggregation, and frontend representation remain
  excluded.

## Verification summary

- Captured streaming-response fixtures with complete, partial, and absent usage.
- Provider mapping and event persistence round-trip tests.
- Restart/history tests proving usage does not alter provider replay messages.
- `go test ./internal/openrouter ./internal/agent ./internal/eviedb ./internal/memory`
- `go test ./...`
- `go vet ./...`

## Proposed one-PR boundary

Add provider response usage mapping, the approved provider-neutral event
payload, fenced persistence, and deterministic tests. Do not implement token
budgets, compaction, retrieval diagnostics, billing policy, or opaque provider
state.

## Readiness

David approved the complete refined execution contract on 2026-08-25 and it was
materialized as [issue #61](https://github.com/davidadel66/evie/issues/61).
Implementation is prepared from reviewed base
`5593518cf64abae9558d9dc91f21188dd5d66b3f` for independent review.
