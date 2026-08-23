# Story 1 - Cancellable tool lifecycle

Status: refined; pending final execution-contract approval

## Outcome

Propagate the turn's caller-owned `context.Context` through tool preparation,
approval, and execution so cancellation observed at a lifecycle boundary stops
later turn-owned work and aborts the agent turn.

## Sources

- [Epic 1](../README.md)
- [Memory invariant 13](../../../memory.spec.md#invariants)
- [Stage 1](../../../memory.spec.md#stage-1---session-identity-and-append-only-events)
- [Binding memory decisions](../../../memory.decisions.md)
- [Serve behavior](../../../serve.spec.md)
- [Serve decisions](../../../serve.decisions.md)
- [Cron consistency contract](../../../../done/cron.spec.md)

## Depends on

- Existing context-aware agent and provider calls.

## Acceptance summary

- Tool preparation, approval, approval observation, prepared execution, direct
  execution, and registered/per-turn-extra tool paths receive the parent turn
  context.
- Parent cancellation observed at an immediate pre-phase or pre-side-effect
  check prevents that phase from starting. The check and invocation are not an
  atomic fence; cancellation after the check follows the approved uncertainty
  behavior until Story 2 adds durable tool-start fencing.
- Parent-turn cancellation aborts the agent lifecycle rather than becoming a
  model-visible tool result. A tool-local child timeout remains an ordinary
  tool failure while the parent context is live.
- The REPL retains its one synchronous scanner. Cancellation while it waits may
  require one final input or EOF; afterward the answer is discarded and no
  approval observation or execution occurs.
- Web request disconnect affects approval visibility only. Approval and
  disconnect use first-atomic-claimant-wins; approval-first honors the decision,
  while disconnect-first expires it and makes a later approval return `404`.
- Turn-owned subprocess, HTTP, database open/setup/query, service, and shell-
  snapshot work honors the parent context. Local deadlines are child contexts,
  and the earlier parent deadline wins.
- Once an effect starts, cancellation is best effort and starts no automatic
  retry. No new generic rollback protocol is added; existing tool-specific
  atomicity and compensating cleanup remain mandatory.
- An already-started cron consistency sequence may use a 10-second bounded
  cleanup context. Parent cancellation remains the lifecycle result, and any
  cleanup failure is joined so neither condition is hidden.
- Finance and YouTube cancellation stops later banks, pages, retries, videos,
  and delays instead of being downgraded to a per-item failure.
- Startup shell warming may continue independently. An interactive waiter may
  cancel; a cancelled lazy capture leaves the cache retryable, with only one
  capture active and independently cancellable waiters.
- `tools.Warm()` startup work and `tools.RunScheduled()` headless cron execution
  remain explicit non-turn exemptions. Existing whole-web-turn disconnect
  behavior remains unchanged.

## Verification summary

- Registry tests for cancellation before every lifecycle phase, after approval
  observation, extra tools, dispatcher wrappers, and parent-versus-child
  deadline classification.
- Agent tests proving lifecycle cancellation starts no later tool or provider
  iteration.
- REPL single-reader/late-input tests and barrier-driven web tests for both
  atomic approval/disconnect claimant orders.
- Blocking built-in, database-open, shell-snapshot, finance/YouTube loop, and
  cron consistency-cleanup cancellation tests.
- Regression tests for the two exemptions and existing non-cancellation tool
  error behavior.
- `go test ./internal/tools ./internal/agent ./cmd/evie ./internal/web ./internal/finance ./internal/youtube ./cmd/finance`
- `go test -race ./internal/web`
- `go test ./...`
- `go vet ./...`

## Proposed one-PR boundary

Change the shared tool lifecycle contracts; their agent, REPL, and web approval
consumers; all registered built-ins and necessary lower context-aware seams;
mechanical non-agent finance CLI callers; existing tool-specific consistency
cleanup; the web approval-race decision record; and deterministic tests. Do not
add durable lease acquisition or fencing, event-format changes, unfinished-turn
resume projection, whole-web-turn cancellation, provider usage, generic
recovery/rollback, or Stage 2 behavior.

## Readiness

The current-code change surface and cancellation matrix have been audited, all
material behavior decisions were approved by David, and two fresh read-only
contract challenges were resolved. The story remains pending David's approval
of the exact GitHub execution contract and separate authorization to materialize
one issue.
