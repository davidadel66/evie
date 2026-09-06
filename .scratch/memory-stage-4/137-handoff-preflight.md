# Ticket #137 implementation handoff

Root planning checkpoint, before #136 public API is frozen. This is a seam map, not a replacement for the approved ticket or binding work contract. User authorizes deterministic implementation while model evaluation continues; acceptance and actual runtime observations are not waived.

## Ownership and dependencies

Read published-bodies/06-worker-recovery-and-capacity.md and cmd/evie/docs/active/memory-stage-4-work-contract.decisions.md plus fixtures W01–W14. #136 owner currently owns internal/memory/compiler.go, internal/eviedb/compiler_{schema,source,validation,work}.go, internal/localextractor, and initial CLI dispatch. #140 owner will own review files and accepted operation/replay extensions. Root coordinates exact handoff; do not edit another active owner's files. New worker files can contain #137 lifecycle logic once #136 claim/stage/publish methods are stable. Root stages/commits one ticket at a time.

## Required extension

Keep SQLite authoritative. A supervisor may wake work but channels cannot be the work ledger. Start in configured long-lived hosts, not short commands that must drain the queue. Stop new claims, cancel clients, and use a five-second cleanup context at shutdown. Never hold a write transaction across inference.

Claim increments durable attempt and monotonically increasing fence before dispatch and reserves one database-wide request plus bounded stage/inbox capacity. Crash before HTTP write still consumes attempt. Leases use database time, 30-second expiry and ten-second renewal. All renewal, stage, publication, retry, and resource mutations require current unexpired holder/fence; cancellation increments fence before signalling a client.

Retry transient endpoint, timeout, disconnect, overload and malformed/truncated output only. Preserve sealed request and accepted context. Five attempts maximum across processes/restarts; delays 5,10,20,40 seconds. Unsafe config/model identity/source/scope/effect/source size is terminal. Resource/config pause changes no attempts. Resume cannot reset budget or re-extract a completed job. An intact fifth-attempt stage can be adopted and published after explicit resume without request6.

Unknown server release retains the global slot as release_pending. Neither lease expiry, client return, process death, connection close nor a timer permits another request. Release evidence must bind request/server; if runtime has no contracted status/cancel endpoint, expose capacity_blocked and require verified controlled restart. A stale acknowledgement cannot release a replacement's reservation. Client cancellation target is under one second, separately from actual server completion.

Adopt valid durable stages with a new fence and no model call; revalidate full envelope/hash/current eligibility before atomic group publication, completion and exact coverage. Cancellation prevents automatic adoption. All errors leave earlier coverage gaps visible while later independent units may complete.

## Verification matrix

Use real temporary SQLite, independent Store handles, and subprocess tests at public seams. Inject exits after claim/before dispatch, after response/before stage, after full stage/before publish, and after commit/before response. Assert exact attempts/group/completion counts on reopen. Use stalled local HTTP fixtures to prove prompt cancellation, unknown release blocks a second store, stale bytes never stage, verified release allows progress, and stale release never frees newer capacity.

Exercise both race orders for cancellation vs staging/publication and lease loss vs late output. Verify five-attempt backoff with durable database clock/controlled due times rather than minute-long sleeps. Oversized input is zero-call failure; partial/malformed output never successful empty. Queue1024/stages128+16MiB/inbox2048 reserve16 are enforced transactionally. Background transactions inspect<=128 events and mutate<=64 candidate/ledger rows. Full new-vs-historical fairness is #138/#139, but preserve the global counter seam.

Do focused tests, then root-coordinated ./scripts/verify-change.sh and independent Standards/Spec review. Do not claim actual selected-model coverage from scripted fixtures. No production dependencies, pushes, PRs or parent issue updates.
