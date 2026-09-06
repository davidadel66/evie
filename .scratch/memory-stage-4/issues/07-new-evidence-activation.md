## Parent

[Memory Stage 4: Compile sourced memory candidates for owner review](https://github.com/davidadel66/evie/issues/131)

## What to build

Connect the recoverable compiler to foreground event commitment and runtime lifecycle. Add explicit activation for new evidence under one pinned generation and reconcile only selected uncovered evidence according to the closure contract. Keep historical selection explicit and preserve normal conversation and explicit memory without a configured extractor.

## Acceptance criteria

- [ ] Activation records the selected scopes, pinned generation, and captured new-evidence frontier. Racing activation and event commitment neither lose selected eligible evidence nor silently select older history.
- [ ] Selected terminal-event commitment and its idempotent scheduling record are atomic. Failed, cancelled/interrupted, crashed, and command-only histories follow the frozen eligibility/closure rules without synthetic terminal events after lease loss.
- [ ] Reconciliation finds selected uncovered durable evidence after process failure. History outside selection is inspectably outside selection, not complete or automatically queued.
- [ ] A blocked or unavailable extractor does not become a foreground model-call dependency. With extraction disabled or unconfigured, episodes continue committing without creating permanently pending extraction work.
- [ ] Use the frozen hosting/startup/shutdown and priority behavior through the actual runtime entry points; retain cross-process capacity and worker fencing.
- [ ] Provide a minimal CLI activation/status demonstration. Record observable foreground-finalization and scheduling timings needed for later pilot measurement without inventing a budget here.
- [ ] Test foreground completion while scripted extraction stalls, activation/reconciliation races, restart, each closure class, and disabled/unavailable extraction through the Kernel and runtime adapters. Run repository-required full change verification.

## Blocked by

- Draft 06: Recover unfinished compilation safely across processes

