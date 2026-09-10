## Parent

[Subagents: Preset-composed foreground research with Kernel supervision (#155)](https://github.com/davidadel66/evie/issues/155)

## What to build

Reopen durable foreground execution after process failure and reconstruct truthful execution state from the admitted attempt and accepted child evidence. Deliver retained findings to an authorized parent retry without starting a second child.

This slice implements restart behavior through the existing Kernel recovery and conversation seams. It does not add background execution, notifications, a scheduler, or production delegation rollout.

## Acceptance criteria

- [ ] Recovery distinguishes an abandoned original execution from another process's still-live parent/child ownership. It never adopts, cancels, or reclassifies a live execution solely because the local supervisor restarted.
- [ ] A failure before admission commits leaves no execution attempt or child. A committed admission whose child never started becomes interrupted after abandonment is proven, with no provider call.
- [ ] An attempt interrupted after child execution starts retains its original incomplete transcript and is classified at execution level. Recovery fabricates no missing parent/child message, capability outcome, or turn-terminal event.
- [ ] An authorized accepted final child answer reconstructs a missing completed execution/result projection. Completed findings survive failures between final answer, execution-result persistence, and parent delivery.
- [ ] Repeated identical parent requests after reopen resolve to the original attempt and retained outcome using the original parent-session identity and current applicable scope/access checks. Changed canonical arguments conflict; unrelated parents cannot inspect or attach.
- [ ] Interrupted or failed attempts never restart automatically. An explicitly fresh request requires a new idempotency key. Recovery itself issues no model calls, new research, or notifications.
- [ ] Replay returns retained results through the parent's current normal conversation fence. It does not write a synthetic outcome for the old incomplete parent invocation or duplicate an already accepted outcome.
- [ ] Deterministic crash/reopen tests cover every commit gap: before admission, after admission before child start, during execution, after final child acceptance before result projection, after result projection before parent delivery, and after parent delivery.
- [ ] Use multiple SQLite connections/store instances and deterministic ownership clocks to distinguish abandoned and live attempts. Run focused recovery/idempotency tests, applicable race checks, and full repository verification. Production registration remains absent.

## Blocked by

- [#172: Complete one bounded durable foreground research assignment](https://github.com/davidadel66/evie/issues/172)
