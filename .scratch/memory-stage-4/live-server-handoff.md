# Local Stage 4 server handoff

Merged [PR #152](https://github.com/davidadel66/evie/pull/152) and [startup fix #153](https://github.com/davidadel66/evie/pull/153). Master is `8d45aa04d133215e7e73da1d7b5ad48ea943617f`. The original 20 ticket commits remain intact; the separate 26-line startup fix commits the existing two-file empty-list repair and its regression.

Installed binary: [/Users/davidboktor/go/bin/evie](/Users/davidboktor/go/bin/evie). Its clean source revision is `f4bcc21cf38653b1ddacbef8d3d3e370af5ec237`, whose tree is identical to final merged master. Binary SHA-256: `130666d0eaf86eabc5e98c066c6c8d43d01d56d9c5826af45a88911e01f77173`.

The server is running at [http://127.0.0.1:6687](http://127.0.0.1:6687) on this Mac, process `58065`. A fresh browser panel displays the chooser. Choose a Context Scope to chat; use Memory → Review candidates → Global memory to inspect the currently empty inbox. Compiler health is also available. No chat message or memory decision was submitted during the smoke test.

Automatic extraction is disabled (`EVIE_COMPILER_CONFIG` empty); there is no approved adequate extractor configuration yet. Thus this installation enables the web interface, not automatic candidate generation or release-quality acceptance. Actual memory evaluation still requires model selection, owner review, pilot-derived gates and an untouched holdout. The server is a detached process for this session, not an installed auto-start service. After it stops, `evie serve` starts the installed binary again using the existing local configuration.

## Verification

- `npm --prefix internal/web/ui ci` succeeded in an isolated committed checkout.
- `./scripts/verify-change.sh` passed for both the initial delivery and the startup fix. Full Go tests/vet, UI lint/build, and whitespace checks ran. Logs: [initial](live-release-verify.log), [fixed](live-startup-fix-verify.log).
- `go test ./internal/web -run '^TestContextSessionHTTPEncodesEmptyCollectionsAsArrays$' -count=1` failed on the old handler with three null collections, then passed with the fix.
- Independent Standards and Spec reviews both passed the two-file startup fix.
- GitHub Verify passed for [PR 152](https://github.com/davidadel66/evie/actions/runs/33981529982) and [PR 153](https://github.com/davidadel66/evie/actions/runs/33982073628) before merge.
- `go build -trimpath -o /Users/davidboktor/code/evie/.scratch/memory-stage-4/evie-stage4-server-fixed ./cmd/evie` passed in the clean fixed snapshot. Build metadata verified the exact source and `vcs.modified=false`.
- Actual Chrome against the installed server and unmodified API: chooser renders, Review candidates opens, global listing returns HTTP 200 with zero candidates, Compiler health renders, no browser errors. [Receipt](live-server-browser-smoke.json). A separate visible in-app browser also displays the chooser.
- Final HTTP 200, listening loopback process, binary hash, merged-tree equality, and SQLite `PRAGMA integrity_check` all verified. No required check was skipped.

Existing warnings: Icon.tsx fast-refresh lint warning; Vite large bundle warning; npm audit reports one existing high-severity transitive nanoid advisory (GHSA-2v37-7h3g-55p8). Dependency manifests/lockfiles are unchanged by this work; no dependency update was bundled. [Audit](live-release-npm-audit.json).

## Preservation and evaluation note

Before startup, SQLite's backup API created a consistent database backup (integrity check OK), and the previous installed binary was copied to [/Users/davidboktor/.evie/backups/before-memory-stage4-20260905T173832Z](/Users/davidboktor/.evie/backups/before-memory-stage4-20260905T173832Z). Server log: [/Users/davidboktor/.evie/logs/serve-stage4-fixed-20260905T174924Z.log](/Users/davidboktor/.evie/logs/serve-stage4-fixed-20260905T174924Z.log). Private database and log contents were not published.

All 221 recorded working-tree file contents remain byte-identical to this turn's starting state. The two existing startup-fix files are now committed; other drafts remain local. The real index is empty. The working branch remains `codex/memory-stage-4`; no checkout or reset discarded drafts.

David's C13 judgment approving decorative emoji omission, and preference for memory entries without emojis, is recorded in [c13-human-judgment.json](c13-human-judgment.json). Other judgments remain pending. No automatic emoji-removal policy was added and no original evidence or frozen output was rewritten.
