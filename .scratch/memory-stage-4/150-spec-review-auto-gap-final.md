# Ticket 150 final automatic-gap Spec recheck

**PASS: no outstanding actionable finding in the corrected engineering delta.** Final tree `4b29071dc69018a9fedb2464315453baab0c9025`; exact snapshot `/var/folders/nv/59bkjyks06gdx0l5k1v5pqk40000gn/T/evie-150-auto-gap-checkpoint-skk5hi0c`. This concludes the recheck of the confirmed interleaved historical-ownership defect; it does not approve unproduced study results.

The reviewer independently reproduced the original live-first failure on `413601a7` (0.263s), then the history-first ordering on the initial live-only repair `52842eb7` (0.281s). Both manufactured a failed interval3..4 with zero selected events and zero attempts. Raw logs and scratch-only reproduction overlays remain retained.

Final production code converts only a successful bounded capture with the exact `failed:empty_selection` result and zero root members. The two automatic reconcilers are the only callers setting `AwaitClosure`; direct QueueCandidateUnit errors remain unchanged. Other source errors, impossible ancestry, invalid sources and inspection/size limits cannot enter this branch.

The coordinate-only selection records `excluded:no_root_members`, zero selected events and no job, attempt or coverage row. Existing immutable ownership lets subsequent reconciliation advance. History inspection joins each event through its actual root, so a foreign root's events cannot borrow this exclusion. No new source authority, promotion, accepted semantic effect or scheduling lane is introduced.

Independent focused verification from the exact final snapshot:

`go test ./scripts/memory-stage4-pilot -run '^TestPilot(LaterRoot|PreFrontierRoot|SealedRoot|ReviewedHistoricalSuffix|ExplicitEmpty)' -count=1`

PASS, 0.672s. This covers both live-first/history-first orderings, zero-job/coverage diagnostics, three database reopen cycles, unchanged historical job IDs and lanes, no false history frontier or foreign-event exclusion, genuine later-root progress, and direct explicit failure preservation. The earlier sparse/closure boundary cases also pass. `git diff --check 413601a7c6088b59fb5dcd7ced53f46193590ffe 4b29071dc69018a9fedb2464315453baab0c9025` passes.

No production edits or broad tests were performed by this reviewer. Fresh source-bound full conformance and the corrected matrix remain required before accepting the measured study. Actual model quality, David's review sessions, adopted numerical gates and release readiness remain pending.
