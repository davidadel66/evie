# #148 compiler health and review diagnostics: engineering handoff

Engineering is frozen and its focused normal/race checks pass. Parent still owns the required isolated full verification, independent Standards/Spec reviews, browser demonstration and one #148 commit. No staging, commits, branch changes, production dependency additions, real model run or owner pilot were performed by this owner.

## Exact contribution

Use `148-engineering-checkpoint.json` with `148-frozen-files/`: **41 files**, comprising 26 owner files, 11 HTTP/UI files and four legacy-status files. Their saved originals and individual manifests remain in `148-originals/`, `148-owner-frozen-manifest.json`, `148-http-ui-frozen-manifest.json` and `148-legacy-frozen-manifest.json`. The owner's compiler_work.go before-hash is #147's frozen `6ee8b7e5ac6d51fac3bdf4e1eafba278664213e02e5af3a38bd6d4a74cec7367`, not the pre-#147 checkout. Its final bytes preserve #147 publication changes and add only #148 timing/reason hooks.

**Exclude live whole-file root hooks from this manifest.** Parent separately owns commit-only db.go/main.go/serve.go variants in `148-root-frozen-files/` and `148-root-frozen-manifest.json`; large pre-existing user changes remain in the shared working tree. Do not replace those protected files with a broad live copy.

## Observable outcome

The typed Kernel API, provider-independent `memory-health` CLI and guarded browser Compiler health tab expose the same exact-owner-scope/session projection. Sessions remain navigable after closure. Activation/history selection or job admission makes a destination navigable before a candidate is published; legacy selected scopes reconcile in bounded persisted pages. Current owner authority is checked on every read. Opaque cursors bind owner capability, destination, source session, view and optional generation. Unknown states/reasons and source/proposal text are never rendered, and the complete bounded JSON projection receives the secret-content check.

Eight views cover jobs, candidates, activations, history, selections, live roots, exact selection membership and foreground measurements. Failed or excluded units without jobs remain visible. Earlier gaps stay separate from later success, cancelled history keeps its original selection, completed-empty remains distinct from candidate output, and unavailable endpoints expose only a safe retry/recovery code. Scope capacity remains shared without identifying another scope's work.

Counters and timestamps are transactionally maintained with candidate/job state; partial-batch savepoints and exact-delivery retries preserve the same count/revision semantics as review outcomes. Operational attempts and review outcomes are not model quality scores. Null observations never become zero or success. Selected/completed event counts represent explicit newly covered source-unit members, including assistant/control events; they are not counts of factual support.

Actual extractor, deterministic validation, staging/publication, queue wait and post-COMMIT publication observations are separated. Successful foreground terminal measurement samples the final no-tool assistant Append; failure/interruption samples terminal events. The REPL/SSE host separately samples actual output finalization before the bounded 500ms telemetry persistence. No host participation means unavailable finalization, and telemetry failure cannot retroactively fail a committed response. Candidate freshness requires a real matching root/session terminal observation. Active human review time still requires David's independent start/stop/focus observations.

## Work bounds and restart behavior

Normal health reads use indexed keysets, at most 32 visible page members and at most five attempts per job. Candidate traversal visits at most two jobs/32 candidates and can return an empty continuation page. Legacy diagnostic counts reconcile at most 15 jobs / 61 ledger mutations per separate transaction. The 16-candidate publication group uses 55 ledger mutations including #147's 53 and two diagnostic updates, within the 64-mutation contract.

Old activation/history status entrypoints now use exact persisted incremental total projections: 128 event coordinates or 32 root records per request, returning ErrCompilerStatusIndexing until complete rather than capped totals or unbounded fallback scans. The separate existing detailed history receipt remains bounded by its explicit 10,000-event/64-interval contract. Counter continuation survives reopen/concurrency/VACUUM and rechecks current authority. Interval membership uses one indexed predecessor and endpoint guard; regressions cover 3,000 retained units and equal activation frontiers.

Initial CREATE INDEX installation builds SQLite indexes over retained metadata once; that migration cost is explicitly documented. Startup does not reconstruct all counters. No retained-source/candidate/accepted-operation rewrite, total-retention bound, deletion policy, force-release bypass or model-quality inference is introduced.

## Verification

Exact commands, results and remaining gates: `148-focused-verification.json`. Final broad normal passed all six packages: Store 43.886s, agent 1.374s, localextractor 0.641s, CLI 8.836s, memory 0.427s and web 3.886s. Final focused race passed Store 15.952s, agent 1.290s, localextractor 2.456s, CLI 2.666s and web 22.249s. The later backward-clock regression additionally passed normal 0.280s / race 2.269s. Legacy final broad race passed 115.592s. All prior 37 aggregate live/frozen hashes and whitespace check passed.

HTTP/UI contribution independently passed normal/race endpoint parity, actual closed-loopback endpoint failure, current guards and exact cursor bindings; UI TypeScript/lint/build and all 30 files/173 tests passed. Existing Icon.tsx lint and chunk-size warnings are documented in `148-http-ui-verification.json`. No browser clicks, real provider evaluation or David review-time measurements are claimed. Parent must run the repository-required full verify and two independent review axes against the ticket-isolated tree.

## Review and manual demonstration

Read `cmd/evie/docs/active/memory-stage-4-diagnostics.implementation.md` for command examples, typed semantics, bounds and the repeatable CPU/RSS/DB-WAL observation procedure. Start code review at internal/memory/compiler_diagnostics.go, internal/eviedb/compiler_diagnostics_schema.go, compiler_diagnostics_views.go, compiler_diagnostics_measurement.go, compiler_status_projection.go and internal/agent/foreground_measurement.go. Public seam tests live in compiler_diagnostics_public_test.go, HTTP compiler_diagnostics_test.go and CLI compiler_diagnostics_test.go.

Follow `148-http-ui-browser-steps.md` with a throwaway fixture: open Memory > Compiler health, select an authorized scope and closed source session, traverse each view, inspect a failed earlier gap plus later completion, then an unavailable local endpoint and its retry/capacity state. Change scope/session/view during a request to verify stale-response fencing; use Continue indexing scopes for a legacy selected destination. Confirm no prompt, endpoint, proposal or source text appears. Use the fixture's existing accepted/review flow to inspect one independent success and one failed batch group; compare its JSON projection with the CLI/Kernel projection. Preserve source text and process arguments outside exported diagnostics.

## Spec review correction: failed host output

The independent review found that SSE writes/flushes and REPL writes could fail while the host still claimed finalization. Final correction freezes 41 aggregate files (393,822 bytes). BeginResponseMeasurement now returns func(outputErr error) error: a non-nil host I/O error preserves terminal commit/outcome and persists unavailable finalization fields. A repeated finalizer cannot convert that failed observation into success. No raw output error is persisted or logged.

SSE retains its first event write/short-write/FlushError failure and returns it from TurnDone; the root-owned serve hook passes it to the finalizer. Standard ResponseController flushing detects FlushError where supported. REPL captures per-turn errors from its synchronous and smooth-printer output, flushes a buffered host writer before sampling, and resets the error for the next turn. The existing Send result and committed terminal events remain unchanged. Successful host I/O is not an acknowledgement that a remote client consumed the response.

Actual Send regression cases cover early/final writes, short writes, early/final SSE flushes, REPL smooth-printer and buffered flush failures, provider-failure error output, successful output and next-turn recovery. The corrected HTTP tests fail against a Go overlay of the saved pre-fix code; the REPL red run also reproduced incorrect finalization. Final focused normal passed all three packages and race passed agent 1.309s/web 1.615s/CLI 13.714s. Broad final agent/web pass; live CLI encountered an unrelated uncommitted #149 history-conformance assertion reported to root. The #148 CLI suite passes 7.745s with only the three uncommitted #149 test files excluded through a Go overlay. Full root verification and review remain required. Exact commands and preserved results are in 148-output-fix-verification.json.

The prior aggregate snapshot is retained at 148-before-output-fix-frozen-files/ with 148-engineering-checkpoint-pre-output-fix.json; narrow originals are in 148-output-fix-originals/. Root separately preserved and updated its protected serve.go variant.
