## Parent

[Subagents: Preset-composed foreground research with Kernel supervision (#155)](https://github.com/davidadel66/evie/issues/155)

## What to build

Make a foreground assignment's authority end with the originating parent execution, including races while the child has its own live lease. Demonstrate the behavior through the same composed delegation flow using deterministic execution barriers.

This ticket owns supervisor lifecycle correctness. Production plugin registration and mapping Plugin Manager lifecycle events into this behavior remain in the rollout ticket.

## Acceptance criteria

- [ ] Parent cancellation, deadline, heartbeat failure, lease expiry/replacement, or relevant current access revocation prevents further child activity and triggers bounded cancellation/cleanup.
- [ ] The child must retain its own valid fence and current authorization from the original parent invocation. A still-active child Session or renewable child lease cannot sustain authority after the originating parent execution ends.
- [ ] Child lease loss is handled through the normal fenced runtime. Stale parent or child owners cannot accept new execution state or write substitute conversational outcomes.
- [ ] Cancellation while admitted and while running closes further work, joins admitted execution, releases child ownership, and reclaims capacity without leaking callbacks or resources.
- [ ] The supervisor exposes a live admission/lifecycle operation that closes admission and cancels owned foreground assignments on stop/disable/shutdown. Previously resolved delegation closures cannot bypass the closed state.
- [ ] The final-acceptance contract established by the foreground ticket is preserved: an authorized committed final child answer wins over later cancellation, while cancellation that wins before final acceptance prevents stale success.
- [ ] A committed child result survives parent-result persistence failure or parent cancellation before delivery. Parent writes remain subject to the parent's fence and no replacement result event is fabricated.
- [ ] Deterministic tests race cancellation, parent/child lease replacement, heartbeat failure, revocation, and supervisor stop against provider calls, final child acceptance, and parent delivery. Use real SQLite and multiple store connections where required.
- [ ] No production delegation exposure is added by this ticket. Run focused ownership/lifecycle tests with the Go race detector and the full repository verification.

## Blocked by

- [#172: Complete one bounded durable foreground research assignment](https://github.com/davidadel66/evie/issues/172)
