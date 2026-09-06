# Ticket 149 final review and verification

Base: 9d697bf57c7835d7522da391c44163e15f416189. Final isolated tree: ebb84af94f37b5bb5278d54208dc1d9f2dba73c7, 15 files. The checkpoint excludes pre-existing user work and tickets150/151.

## Standards

Independent full review and final test-isolation delta review: no documented-standard breaches or material smell findings. Details:149-standards-review.md. Typed physical SQLite startup retry and the trusted read-only query-tool constructor are narrow seams required by observed conformance failures.

## Spec

Independent review initially found two verification defects: closed-source public graph reads required active observers, and generic SQL containment accepted unrelated errors. Both are resolved. Browser operation IDs and exact hashes now bind owner inspection; active observers inspect context scopes, and exact closed-session destinations use temporary projection assertions plus canonical replay without reopening sources. Two real allowed query controls and308 exact policy denials across77 protected tables/views prove blocked queries never open a database. A deliberate allowlist mutation passes the old test and fails the new one. Final Spec review and the test-only Git isolation delta have no findings. Details:149-spec-review.md. Final delta axes ran independently in parallel.

## Verification

The complete final command ran in the isolated checkpoint with its explicit Git directory/worktree/index:

`python3 scripts/memory-stage-4-conformance.py --browser-receipt /Users/davidboktor/code/evie/.scratch/memory-stage-4/149-browser-verified/browser.json --output-dir /Users/davidboktor/code/evie/.scratch/memory-stage-4/149-conformance-verified`

PASS: six report-boundary tests, eleven integrated receipts in normal4.203s/race55.076s, `./scripts/verify-change.sh`51.891s, and `npx --no-install vitest run`23files/155tests. Full verification includes all Go tests/vet, UI lint/build and staged/unstaged whitespace. Zero failures or skipped checks. Existing Icon.tsx fast-refresh lint warning and Vite chunk-size warning remain. Exact commands/logs, runtime versions, source hashes, observations and warnings are retained in149-conformance-verified/report.json; source SHA2563a0f7d3ca21c90f3aab7c55d3d97d77dc3f88b7bda1f738b878b8d521c62446c is unchanged after verification.

`EVIE_PLAYWRIGHT_MODULE=/Users/davidboktor/code/evie/.scratch/memory-stage-4/browser-driver/node_modules/playwright-core node scripts/memory-stage-4-browser.cjs /Users/davidboktor/code/evie/.scratch/memory-stage-4/149-browser-verified`: PASS. Actual Chrome/React/HTTP inspection, preview, acceptance, recorded operation and reload run in global, two Workspace, two project and two session destinations. Global accepted memory/provenance is browsed; separate Kernel/projection checks retain all seven closed sources, prevent implicit Promotion, and verify canonical replay with zero model calls. No browser JavaScript errors; fixture process exits0 and temporary database is removed. Screenshot inspected.

Earlier failed runs are retained. The first browser run exposed a primary-candidate lineage assumption in the helper. A subsequent full run passed all Go/UI checks but failed the Python source-binding fixture because inherited alternate-tree Git variables targeted the caller repository. The fixture now binds all four Git paths within a scoped environment patch; normal and inherited-environment sentinel tests pass independently. The old fixture installed a core.worktree value in caller config; root saved the config, removed only that exact root-owned value, and verified the original repository top-level. No user file or branch state was discarded.

## Outcome and limits

Ticket149's repeatable scripted conformance proves foreground completion during stalled extraction, recovery and process fencing, exact cross-adapter review, explicit historical selection/gaps/empty results, immutable generations, atomic dependent acceptance, current source-policy redaction, accepted provenance and replay. A reproduced physical SQLite WAL-open race is fixed with typed BUSY startup retry, without replaying schema or application writes. Driver-open cancellation still obeys the pinned driver's existing SQLite busy timeout; the retry budget is not a strict wall-clock interruption guarantee.

This does not establish learned model quality, actual human review time, resource capacity or release readiness. The generic tool fence excludes the documented privileged local shell. Manual reproduction and exact browser commands are in cmd/evie/docs/fixtures/memory-stage-4-conformance/v1/README.md. Review entry points: sqlite_startup.go, the Stage4 conformance tests, query-tool constructor, and source-bound runner/browser driver.
