# Epic 1 - Restart-safe leased REPL sessions

Status: approved for planning by David on 2026-08-23

## Outcome

Allow an explicitly selected global or project REPL session to resume after an
Evie restart while ensuring that exactly one durable lease holder may run its
turn, start its tools, or append accepted turn evidence.

## Specification references

- [Memory invariants](../../memory.spec.md#invariants)
- [Scope and authority](../../memory.spec.md#scope-and-authority)
- [Session and event tables](../../memory.spec.md#session-and-event-tables)
- [Stage 1 - Session identity and append-only events](../../memory.spec.md#stage-1---session-identity-and-append-only-events)
- [Binding memory decisions](../../memory.decisions.md)

## Existing foundation

The repository already provides:

- global/project registration and immutable session scope;
- explicit confirmation before a new global or project REPL session is created;
- append-only provider-neutral event history and ordered history projection;
- before-action user, assistant, tool-intent, approval, and tool-outcome events;
- caller cancellation propagated through tool preparation, approval, execution,
  and blocking built-in boundaries;
- generic database/file fences around memory-owned storage; and
- durable turn-lease acquisition, heartbeat, release, expiry, and fencing
  primitives.

This epic completes the remaining Stage 1 integration. It does not rebuild the
implemented foundation.

## In scope

- Caller cancellation through tool preparation, approval, and execution.
- Durable lease ownership of live agent turns.
- Lease-fenced event appends and tool starts.
- Cancellation on lease loss and rejection of late provider output.
- Durable provider failure/interruption evidence while the turn remains
  authorized.
- Explicit selection and restart resume of existing REPL sessions.
- Provider-neutral persistence of available provider usage.

## Out of scope

- Working-context budgeting or compaction.
- Semantic graph state, extraction, indexing, or retrieval.
- Web-session selection and resume.
- Opaque provider continuation or reasoning payload persistence.
- Blocking later turns because an earlier tool intent has no terminal event.
- Synthesizing success or failure for an interrupted tool execution.
- Stage 0 reference-system documentation, which David excluded from this epic
  after confirming that he had already completed the Letta research.

## Stories

### Story 1 - Cancellable tool lifecycle

- [Planning summary](story_1/README.md)
- Status: delivered by [issue #55](https://github.com/davidadel66/evie/issues/55)
  and merged in [pull request #56](https://github.com/davidadel66/evie/pull/56)
  on 2026-08-23.
- Outcome: propagate the caller's context through tool preparation, approval,
  and execution so observed turn cancellation stops later turn-owned work.
- Depends on: existing caller-context support in the agent and provider.
- Acceptance summary: cancellation observed at an immediate lifecycle or
  side-effect check prevents that phase from starting; parent-turn cancellation
  aborts the lifecycle, while tool-local timeout behavior and existing
  tool-specific consistency cleanup are preserved.
- Verification summary: lifecycle cancellation matrix, REPL/web approval race,
  blocking built-in, finance/YouTube loop, cron cleanup, and shell-snapshot
  tests; focused package tests, `go test -race ./internal/web`, `go test ./...`,
  and `go vet ./...`.
- Proposed PR boundary: tool runtime contracts; agent, REPL, and web approval
  consumers; all built-in/lower context seams and mechanical CLI consumers;
  existing consistency cleanup; decision documentation; and tests. Durable
  lease integration remains deferred.

### Story 2 - Lease-owned and fenced turns

- [Planning summary](story_2/README.md)
- Status: dependency satisfied; interactive decision refinement required.
- Outcome: make a durable session lease own the complete live turn from the
  first event through provider and tool work.
- Depends on: Story 1 and the existing SQLite turn-lease primitives.
- Acceptance summary: acquisition precedes every turn action; heartbeats retain
  ownership; every append and tool start is fenced; lease loss cancels local
  work and discards late provider output; failures and interruptions are
  recorded only while the writer remains authorized.
- Verification summary: lease-race, stale-token, heartbeat-loss, late-response,
  append-failure, and tool-start tests plus repository Go checks.
- Proposed PR boundary: agent-owned turn-ownership interface, SQLite adapter,
  fenced history path, live agent integration, and tests; no resume chooser or
  usage accounting.

### Story 3 - Safe existing-session selection and resume

- [Planning summary](story_3/README.md)
- Outcome: explicitly select and resume an existing global or project REPL
  session without weakening its immutable scope or durable ownership.
- Depends on: Story 2.
- Acceptance summary: session selection never grants project scope silently,
  preserves the stored project/root snapshot, rebuilds ordered provider-neutral
  history, and reports a live lease conflict instead of opening a competing
  turn.
- Verification summary: global/project selection, restart, relocation-snapshot,
  and competing-process tests plus a manual REPL resume demonstration.
- Proposed PR boundary: active-session listing/query seams and REPL startup
  flow; no web UI, compaction, or graph work.

### Story 4 - Provider usage evidence

- [Planning summary](story_4/README.md)
- Status: implementation candidate for
  [issue #61](https://github.com/davidadel66/evie/issues/61); awaiting review and
  merge.
- Outcome: preserve usage reported by the provider as immutable,
  provider-neutral turn evidence.
- Depends on: Story 2's terminal provider-event path.
- Acceptance summary: all approved available usage fields survive persistence
  and restart; missing usage remains explicitly absent; opaque continuation,
  reasoning, and transport payloads do not become evidence.
- Verification summary: captured stream fixtures, event round trips, absent-
  usage cases, focused package tests, and repository Go checks.
- Proposed PR boundary: provider response mapping, provider-neutral usage event
  payload, persistence tests, and agent integration; no token-budget policy.

## Recommended order

1. Story 1 - Cancellable tool lifecycle.
2. Story 2 - Lease-owned and fenced turns.
3. Story 3 - Safe existing-session selection and resume.
4. Story 4 - Provider usage evidence.

Stories 3 and 4 may be refined independently after Story 2 lands, but the
default delivery order keeps the user-visible restart/resume path ahead of
accounting evidence.

## Epic completion evidence

- A deterministic two-store or two-process race proves that only one live lease
  holder can start and commit a turn for a session.
- Lease loss cancels an in-flight approval, provider request, or blocking tool;
  a late provider response cannot start tools or append events.
- One global and one project session each resume after restart with ordered
  provider-neutral history and their original immutable scope.
- Provider failure/interruption and available usage evidence survive restart
  without persisting opaque continuation or reasoning payloads.
- `go test ./...` and `go vet ./...` pass.

## Risks and open decisions

- The current REPL approval path synchronously owns its scanner. David approved
  fail-closed cancellation that may wait for one final input or EOF; afterward
  the answer is discarded and the same scanner remains the sole input owner.
- A durable lease fences accepted state and tool starts, but it cannot recall a
  provider request already transmitted remotely.
- Story 2's Story 1 dependency is satisfied, and its terminal-event,
  payload/redaction, stale-owner, lease-timing, streamed-output, fencing,
  cleanup, correlation, and incomplete-tool-group decisions are approved and
  recorded in `memory.decisions.md`. Follow-up decisions from the first contract
  challenge are also approved and recorded. The final independent closure
  challenge found no material issues; Story 2 is ready for issue materialization.
- Story 3's exact session-list presentation and selection details remain for
  interactive refinement; they may not weaken explicit scope confirmation.
- Story 4's exact six-counter provider-neutral allowlist, normalization rules,
  per-iteration association, persistence fate, and episodic-only exclusions were
  approved and materialized as issue #61 on 2026-08-25.

## Approval record

- 2026-08-23: David approved this one-epic Stage 1 boundary and its four-story
  decomposition.
- 2026-08-23: David directed the plan to disregard the Stage 0 note because he
  had already completed the Letta research.
- 2026-08-23: The retained specification and decisions were amended to record
  that prior research as satisfying Stage 0 without a new repository artifact.
- 2026-08-23: David approved Story 1's refined cancellation, timeout, approval-
  race, bounded cleanup, shell-snapshot, and explicit-exemption behavior after
  two independent read-only contract challenges.
- 2026-08-23: Story 1 was delivered by issue #55 and merged in pull request #56;
  David selected Story 2 as the next story for interactive refinement.
- 2026-08-23: David approved Story 2's full decision package covering terminal
  evidence, redaction, stale ownership, lease timing and conflicts, streaming,
  fencing points, cleanup, correlation, and incomplete tool groups.
- 2026-08-23: David approved every Story 2 follow-up from the first independent
  contract challenge: pre-root cancellation, lifecycle stages, heartbeat states,
  provider validation, web wire behavior, and release eligibility.
- 2026-08-23: David approved the second challenge's complete lifecycle-stage
  transition matrix and browser-visible discarded-response behavior.
- 2026-08-23: David approved `assistant_persistence_failed` as the local-only
  discarded-output cause for a non-lease assistant append failure.
- 2026-08-23: The final independent Story 2 contract challenge found no material
  issues; all acceptance criteria fit one coherent PR boundary with deterministic
  evidence and no later-story dependency.
- 2026-08-23: David authorized committing and pushing the approved planning
  records, creating exactly one Story 2 execution-contract issue, and beginning
  the reviewed delivery workflow immediately afterward.
- 2026-08-25: David approved Story 4's six-counter usage allowlist, independent
  malformed-field normalization, repeated-container and last-non-null stream
  rules, atomic per-assistant persistence, replay/compiler/policy exclusions,
  fixture provenance, and one-PR execution contract as issue #61.
