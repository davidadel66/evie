# Story 4 - Provider usage evidence

Status: proposed; dependency blocked by Story 2

## Outcome

Persist usage reported by the conversational provider as immutable,
provider-neutral turn evidence for later context and evaluation diagnostics.

## Sources

- [Epic 1](../README.md)
- [Session and event tables](../../../memory.spec.md#session-and-event-tables)
- [Stage 1](../../../memory.spec.md#stage-1---session-identity-and-append-only-events)
- [Binding memory decisions](../../../memory.decisions.md)

## Depends on

- Story 2's fenced terminal provider-event path.

## Acceptance summary

- Approved provider-neutral usage fields returned by a successful provider turn
  are persisted with immutable terminal-turn evidence.
- Available usage survives event round trips and restart.
- Provider responses without usage remain explicitly absent rather than
  receiving invented zero values.
- Opaque continuation payloads, reasoning details, and raw transport state are
  not stored as semantic evidence.
- Usage persistence remains fenced by the active turn lease.

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

`DEPENDENCY_BLOCKED` until Story 2 lands. Refinement must confirm the exact
provider-neutral field set against captured fixtures before the story can be
declared ready.
