The memory compiler now turns selected conversation evidence into durable, reviewable candidates while chat continues. Owners can inspect exact sources, preview and edit changes, and accept or reject candidates in the CLI or web inbox, including after the source session closes. Accepted memory remains approval-gated.

This branch preserves one commit for each ticket #132–#151. It adds explicit live activation and historical backfill, restart/concurrency recovery, generation handling, review UI and compiler diagnostics, plus deterministic conformance and evaluation tooling. The workload study exposed and fixed an interleaved-root reconciliation bug that created empty failed jobs.

Validation:
- Fresh `./scripts/verify-change.sh` passed on an isolated checkout of delivery commit `10dc6759680f4b0fc23a54f0d9ff8595073eb501`: full Go tests/vet, UI lint/build, and whitespace checks.
- Source-bound conformance, seven browser scope checks, normal/race tests, and independent Standards/Spec reviews passed.
- 99 scripted workload trials up to one million retained events: zero unexpected jobs; 33 deliberately injected failures retained as expected fixtures.
- The complete branch whitespace check passes. Existing unrelated local drafts are excluded.

Experimental acceptance remains open for #135, #136, #150 and #151, and parent #131. No adequate extractor is selected; actual model-output adjudication, owner pilot, numerical release gates and untouched holdout acceptance remain pending. The infrastructure measurements do not establish learned quality or sustained capacity. Merging does not activate compilation or make Stage 4 release-ready.

Review entry points:
- `internal/eviedb/compiler_reconciliation.go` and the compiler/review Kernel implementations.
- `internal/web/ui/src/candidateInbox/` and `internal/web/candidate_review.go`.
- `cmd/evie/docs/fixtures/memory-stage4-pilot/v1/infrastructure-results.md`.
- `scripts/memory-stage4-release/README.md`.

Existing warnings: Icon.tsx fast-refresh lint warning, Vite bundle-size warning, and npm audit reports one high-severity transitive nanoid advisory. UI dependency manifests/lockfiles are unchanged from master.
