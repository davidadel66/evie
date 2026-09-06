# Conversation history and memory applicability — September 5, 2026

Implemented the user-authorized contract in cmd/evie/docs/active/session-history-and-memory-scope.spec.md, including the confirmed direct and background-candidate paths. Existing Memory UI simplification remains intact.

Reopening and reloading conversations restores persisted messages and recorded tool outcomes in 100-item pages. Historical approvals cannot execute. Delayed history, stream and approval responses are isolated to their session; earlier-page retry preserves its cursor and existing messages. Both reads and chat sends bind to the displayed session ID. Selection failures are visible.

New conversations expose a closed memory destination recommendation: Everywhere, Workspace, This conversation. The prompt chooses applicability from meaning and qualifiers. Exact approval includes subject, predicate, value, polarity, destination and source evidence. Sources remain in their original scope; Global memory does not disclose workspace evidence or private entity references. Candidate inbox scope stays separate from its approved effect. Identity, temporal correction, compound and batch effects preserve the destination and revision boundaries. Mixed-destination groups are reviewed separately. Existing General memories remain workspace-scoped.

Older sessions pin their original memory tool schemas. Memory implementation 1.1.0 provides frozen 1.0.0 variants through existing compatibility resolutions; receipts remain unchanged, original schemas match their frozen hashes, and legacy tools reject destination keys case-insensitively. Start a new conversation to test direct applicability recommendations. The scope-aware background compiler policy and compatible schema are implemented and tested, but the local compiler remains unconfigured/disabled. No model-quality score is claimed; the spec includes twelve human review cases.

## Verification

- `./scripts/verify-change.sh` PASS after compatibility fixes: all Go tests/vet, UI lint/build and staged/unstaged whitespace. Log: verify-final-compatibility.log.
- `npx vitest run` from internal/web/ui PASS: 32 files, 185 tests. Log: ui-compatibility.log.
- `go test ./internal/plugins -run 'TestOriginalMemory|TestMemoryPlugin' -count=1` PASS: original receipt/schema compatibility and mixed-case destination rejection.
- `go build -trimpath -o .scratch/session-history/evie ./cmd/evie` PASS. Installed atomically at /Users/davidboktor/go/bin/evie after consistent private SQLite, binary and environment backup. Runtime PID/log/hash/backup: runtime.json.
- `node .scratch/session-history/browser-fixtures.cjs` PASS: delayed A response after B, older-page failure/retry, retained current items, session-bound send and zero page errors. All API mutations were intercepted synthetic fixtures.
- `node .scratch/session-history/repro.cjs` PASS against installed server: actual General saved message restores. Log: live-repro-final.log.
- `node .scratch/session-history/live-browser.cjs` PASS: actual General reopen/reload, historical approvals inert, Memory list/detail/David/graph navigation, mobile no overflow and zero page errors. Log: live-browser-final.log. Screenshots: live-history.png, live-memory.png, live-mobile.png.
- All 16 semantic tables match the pre-install database backup; all 180 pre-existing file hashes preserved. No memory acceptance, new real chat message or external model call was submitted during QA. See preservation.json.
- Read-only Standards/Spec reviews completed; all findings fixed. Final compatibility review found a case-insensitive JSON boundary issue, fixed and regression-tested.

Warnings: non-failing mixed-export Fast Refresh warnings in Icon.tsx and presentation.tsx; existing large Vite bundle warning. No required checks skipped. Earlier failures included a managed-chat test requiring its new session binding, an accidentally repository-root Vitest run that included scratch fixtures (correct UI-root run passed; generated tracked cache restored), and live pinned-schema resume failure (fixed via explicit compatibility). These failures are not final verification results.

Initial history paging bounds response item count, but still reads the full session event log before projection; indexed database paging is deferred. Other UI areas and compiler activation are deferred. Changes remain uncommitted on master; no push, PR or merge performed.

## Manual demonstration and review entry points

Refresh http://127.0.0.1:6687, open General → hows it going, and reload. Open Data → Memory, select General, then open either memory, David or its graph value. Start a new General conversation and ask to remember a general preference, workspace-only instruction or conversation-only instruction; check the exact applicability before approving.

Review: internal/web/history.go and history_test.go; internal/web/ui/src/store/useSession.ts and chat/ApprovalCard.tsx; internal/plugins/memory.go and memory_test.go; internal/eviedb/semantic_destination.go, candidate_destination.go and memory_destination_public_test.go; internal/memory/scope_instructions.go; internal/agent/prompt.go.
