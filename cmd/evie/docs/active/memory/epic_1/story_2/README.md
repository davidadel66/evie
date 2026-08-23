# Story 2 - Lease-owned and fenced turns

Status: proposed; dependency blocked by Story 1 and decision blocked

## Outcome

Require a durable session lease to own every live agent turn from its first
event through provider and tool work, with SQLite fencing at every accepted
write boundary.

## Sources

- [Epic 1](../README.md)
- [Memory invariants 6, 10, and 13](../../../memory.spec.md#invariants)
- [Session and event tables](../../../memory.spec.md#session-and-event-tables)
- [Stage 1](../../../memory.spec.md#stage-1---session-identity-and-append-only-events)
- [Binding memory decisions](../../../memory.decisions.md)

## Depends on

- Story 1 - Cancellable tool lifecycle.
- Existing durable turn-lease storage primitives.

## Acceptance summary

- The agent acquires a durable lease before the first event append or provider
  call and heartbeats it for the bounded turn lifetime.
- Immutable session scope and lease credentials are harness-owned rather than
  model arguments.
- Every turn event append and every tool start verifies the current fencing
  token at the storage boundary.
- Lease loss cancels local work; a late provider response is discarded and
  cannot append events or start tools.
- Provider failures and caller interruptions append durable evidence only while
  the process remains the authorized writer.
- Release and shutdown preserve the lease epoch invariants already defined by
  the store.

## Verification summary

- Agent/store integration tests for competing owners and stale tokens.
- Heartbeat-loss and lease-expiry cancellation tests.
- Late provider-response and tool-start fence tests.
- Fail-closed append and provider failure/interruption tests.
- `go test ./internal/agent ./internal/eviedb ./internal/memory ./internal/tools`
- `go test ./...`
- `go vet ./...`

## Proposed one-PR boundary

Add the agent-owned turn-ownership interface, its SQLite implementation, the
lease-bound history append path, heartbeat/cancellation integration, provider
failure evidence, and deterministic tests. Do not add the REPL resume chooser,
provider usage accounting, compaction, or semantic memory.

## Decisions required before readiness

Story 2 must not select these behaviors from implementation code. Interactive
refinement must obtain David's approval and record each result in
`memory.decisions.md`:

1. **Failure and interruption taxonomy:** define the durable event types and
   stable classifications for provider failure, caller cancellation, malformed
   provider response, and lease interruption.
2. **Payload and redaction:** define the provider-neutral fields that may be
   persisted, prohibit opaque reasoning/continuation state, and decide how raw
   provider error text is sanitized so secrets or transport details are not
   copied blindly into durable history.
3. **Stale-owner behavior:** define the observable result when lease loss makes
   the former owner unauthorized to append interruption evidence; a stale token
   must never be bypassed merely to record a failure.
4. **Streamed-output behavior:** define what the REPL may display before lease
   loss, what callbacks are suppressed afterward, and how the frontend indicates
   that already-rendered text was not durably accepted.
5. **Correlation and parentage:** define the turn/execution identifiers and
   parent event used by terminal provider evidence, including failures after a
   provider response but before durable assistant acceptance.

## Readiness

`DEPENDENCY_BLOCKED` until Story 1 lands. After that dependency, the contract
remains `DECISION_REQUIRED` until every question above is approved and recorded.
It then requires current-code refinement and an independent challenge before
David may select it for implementation.
