# Story 2 - Lease-owned and fenced turns

Status: delivered by issue #57 and merged in pull request #58 on 2026-08-24

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
- [Serve behavior](../../../serve.spec.md)
- [Serve decisions](../../../serve.decisions.md)
- [Browser chat behavior](../../../ui-chat.spec.md)

## Depends on

- Story 1 - Cancellable tool lifecycle, delivered by issue #55 and merged in
  pull request #56 on 2026-08-23.
- Existing durable turn-lease storage primitives.

## Acceptance summary

- The agent acquires a durable lease before the first event append or provider
  call and heartbeats it for the bounded turn lifetime.
- Immutable session scope and lease credentials are harness-owned rather than
  model arguments.
- Every turn event append and every tool start verifies the current fencing
  token at the storage boundary.
- Every provider iteration verifies ownership immediately before it starts;
  gated tools verify before preparation and again after approval immediately
  before execution.
- Lease loss cancels local work; a late provider response is discarded and
  cannot append events or start tools.
- Provider failures and caller interruptions append durable evidence only while
  the process remains the authorized writer.
- Safe terminal payloads distinguish provider failure, invalid response, caller
  cancellation, and caller deadline without persisting raw provider or Go error
  details.
- Already-rendered response text that cannot commit is marked locally as
  discarded in the REPL, web stream, and browser transcript; partial text stays
  visible but is explicitly labeled interrupted and not saved.
- Incomplete tool-call groups remain durable evidence but are omitted as a
  whole from later provider-history projection, without synthesizing outcomes or
  blocking a later turn.
- Release and shutdown preserve the lease epoch invariants already defined by
  the store.

## Verification summary

- Agent/store integration tests for competing owners and stale tokens.
- Heartbeat-loss and lease-expiry cancellation tests.
- Late provider-response and tool-start fence tests.
- Fail-closed append and provider failure/interruption tests.
- Redaction, terminal parentage, discarded-stream, bounded-cleanup, and
  incomplete-tool-group projection tests.
- Content-only and reasoning-only assistant-persistence failures proving exact
  REPL/SSE/browser discarded behavior and no recursive durable event.
- Browser render assertions for partial-text inline and reasoning-only
  standalone warnings, including the exact message and persistence after
  `turn_done`.
- `go test ./internal/agent ./internal/eviedb ./internal/memory ./internal/tools`
- `go test ./cmd/evie ./internal/web`
- `go test -race ./internal/agent ./internal/eviedb ./internal/web`
- `npx vitest run`, `npm run lint`, and `npm run build` from
  `internal/web/ui`
- `go test ./...`
- `go vet ./...`

## Proposed one-PR boundary

Add the agent-owned turn-ownership interface, its SQLite implementation, the
lease-bound history append path, heartbeat/cancellation integration, provider
and tool-start authorization, provider failure/interruption evidence, incomplete
tool-group history projection, REPL/web discarded-stream notification, bounded
cleanup, browser discarded-state rendering, and deterministic tests. Do not add
the REPL resume chooser, provider usage accounting, compaction, or semantic
memory.

## Approved decisions

David approved the following decisions on 2026-08-23; the binding detail is in
`memory.decisions.md`:

1. Separate `turn_failed` and `turn_interrupted` taxonomies with stable
   classifications and first-cause cancellation semantics.
2. Allowlisted, redacted payloads rooted at the user-event turn ID and parented
   only to accepted evidence.
3. Fixed 30-second leases, 10-second heartbeats, immediate fail-closed heartbeat
   cancellation, and fail-fast ownership conflicts.
4. No stale-owner evidence bypass or later synthesized lease-loss event.
5. Live streaming with callback suppression and a local `response_discarded`
   marker when rendered text did not commit.
6. Durable authorization before each provider and tool lifecycle boundary.
7. One independent five-second terminal-evidence attempt and one independent
   five-second release attempt.
8. Preservation but whole-group provider-projection omission of incomplete tool
   executions.
9. No terminal evidence before the root user event; acquired pre-root turns
   perform release only and retain the committed root ID thereafter.
10. A closed seven-value lifecycle-stage vocabulary with pre-phase sampling and
    optional HTTP status only for non-2xx responses.
11. Expected heartbeat shutdown is normal; definitive loss and other heartbeat
    failures are distinct local-only causes in first-terminal-cause arbitration.
12. Exact provider failure/invalid-response mapping and structural tool-call
    validity, while tool argument validation remains model-visible tool failure.
13. Exact REPL/SSE discarded-response payload and ordering, with pre-stream
    durable lease conflicts returning HTTP 409.
14. Exactly one release attempt per successfully acquired turn and none after
    acquisition failure or conflict.
15. A complete seven-stage entry/exit transition matrix covering post-commit
    callbacks, approval waiting, tool execution, and multi-tool/provider loops.
16. Browser-visible discarded state that preserves partial text, annotates it
    inline, creates a standalone warning for reasoning-only output, and survives
    `turn_done`.
17. Local `assistant_persistence_failed` discarded state when rendered output's
    non-lease assistant append fails, without a recursive durable event.

## Readiness

The Story 1 dependency was complete. David approved and recorded the initial and
follow-up decision packages. A fresh independent closure challenge against base
`d48c0ac1b9309bbaa73b37c23fdae7fa0a7e603d` found no material issues. Story 2
was delivered by issue #57 and merged in pull request #58 on 2026-08-24.
