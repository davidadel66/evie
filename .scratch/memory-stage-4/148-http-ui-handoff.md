# #148 guarded HTTP and isolated UI contribution

Engineering complete and frozen for root aggregation; no staging or commits performed. Root owns the final exact-tree full verification, two independent code-review axes, browser demonstration and one commit for ticket148.

Frozen manifest: `.scratch/memory-stage-4/148-http-ui-frozen-manifest.json` (11 paths, 75,657 bytes).
Frozen bytes: `.scratch/memory-stage-4/148-http-ui-frozen-files/`.
Original existing files: `.scratch/memory-stage-4/148-http-ui-originals/`, original hashes in `148-http-ui-original-manifest.json`.
Both existing-file baselines match exact #146 tree `97e9768e1d9ef26f44938f4b80de413a4ffda0cb`; one narrow registration line in candidate_review.go and three-view tab change in MemoryReviewTabs. No db/main/serve/Memory.tsx/App/graph/DataHub/theme/shell or user-file edits.

## Behavior

- Optional `CompilerDiagnosticsKernel` interface preserves existing review-only kernels. Guarded POST `/api/memory/compiler/sessions` and `/api/memory/compiler/diagnostics`, `{scope_key,input}` envelopes; strict known fields, duplicate/UTF8/framing checks and inclusive8KiB limit, current exact owner authority from server, every response no-store including shared-guard errors. Kernel output passes through unchanged; safe errors never reveal arbitrary DB/file/network details. Known invalid input/cursor/authorization/source errors retain distinct classifications.
- Separate Compiler health tab; accepted memory remains default. Eight views: jobs, candidates, activations, history, selection, selections and live_roots, foreground. Explicit scope/session selection and exact generation for selection. Every request limit32, replace-only pages, manual refresh, cursors reset on selection changes/refresh. Epochs fence stale scope/session/view/generation/results and unmount. Current authorization failures clear preceding diagnostic and discovered metadata.
- Counts labelled scope+session across generations; known counter labels only; legacy indexing explicitly partial. Distinct failed gaps/later success, selected/completed new-event counts versus bounding coordinates, live obligations before job materialization, frontiers/scanned positions are selection/discovery only. Global capacity includes no other scope identity. Retained generation is not runtime availability.
- Separate nullable queue/inference/validation/database/publication measurements, actual terminal commit versus response finalization and freshness only from Kernel observation. Null means unavailable/incomplete, not zero. Review revisions/outcomes/edits/lineage/timestamps are metadata only. Elapsed inbox age is only for current unresolved unsuppressed items; active review time unavailable here and approval rate is not accuracy. No acceptance/configuration mutation added.

## Verification

- `go test ./internal/web -run '^TestCompilerDiagnosticsHTTP' -count=1` PASS0.532s (`148-http-ui-go.log`).
- `go test ./internal/web -count=1` PASS3.175s (`148-http-ui-web-all.log`).
- `go test -race ./internal/web -run '^TestCompilerDiagnosticsHTTP' -count=1` PASS7.787s (`148-http-ui-race.log`).
- From internal/web/ui: `npx tsc -b` PASS (`148-http-ui-types.log`); focused `npx vitest run src/compilerDiagnostics src/api/compilerDiagnostics.test.ts` PASS3files18tests (`148-http-ui-vitest.log`). Final full `npx vitest run` result in `148-http-ui-vitest-all.log`.
- `npm run lint` PASS with existing pre-existing user Icon.tsx Fast Refresh warning; no contribution warning (`148-http-ui-lint.log`). `npm run build` PASS, existing >500KiB chunk warning (`148-http-ui-build.log`).
- `gofmt` changed Go files and exact owned-path `git diff --check` PASS (`148-http-ui-whitespace.log`). Frozen files match all live contribution hashes.
- Initial guard test caught shared jsonError clearing no-store. Fixed locally with bounded diagnostic response wrapper; original failure retained at `148-http-ui-go-first-failure.log`, subsequent normal/race pass. A shell invocation initially used the wrong working directory for read/test commands; no source changed by that failed invocation and rerun passed.
- `./scripts/verify-change.sh` and independent Standards/Spec reviews are intentionally assigned to root on the aggregate exact ticket tree; not claimed performed here. No browser run by this contributor; steps in `148-http-ui-browser-steps.md`. No active owner pilot or model-quality claim.

Real Store/HTTP tests cover closed-source metadata parity across all8views, current exact destination/source scope, guard/body bounds, forged/cross-view/cross-session/cross-scope/cross-generation cursors, recorded rejection timestamps, sanitized unknown errors, two-generation earlier terminal failure + later candidate success with actual selected/completed counts and postcommit/null timing, and a third generation through real localextractor transport to a closed loopback endpoint. The latter now reports endpoint_unavailable/retry_wait/due/recovery with no endpoint path leakage and capacity available because metadata failed before dispatch. Owner added the typed safe endpoint error to preserve this distinction.

Review entry points: compiler_diagnostics.go route authority and safe errors; compiler_diagnostics_test.go real observable seam; API types; controller.ts invalidation/paging; CompilerHealth.tsx metric semantics. No production dependency added.
