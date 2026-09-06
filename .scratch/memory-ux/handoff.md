# Memory UI handoff — September 5, 2026

Implemented the presentation contract in cmd/evie/docs/active/memory-ui.spec.md. Memory defaults to a readable list in the active workspace, with Graph alternate. Global and workspace names lead the single scope selector; conversation/project scopes remain in Other scopes. Memories and Review are primary; background activity and technical history are secondary. Literal graph nodes and global owner inspection now open a main detail page. Sources, polarity, conflict notices, exact review effects and approval safeguards remain intact. The HTTP display setting EVIE_OWNER_NAME supplies David without changing the canonical owner.

Actual General claims verified unchanged: 9a98e35a-642f-4b13-96d3-cd97515d9279 and f083fca4-b627-412d-b847-c2daaa5a975b, attached to canonical owner 639e6a85-52c8-47ac-82d0-f5fb3e073fbd. Both are active and workspace-scoped. API null collections are normalized at the frontend boundary to prevent empty-list/detail crashes.

## Verification

- `./scripts/verify-change.sh` PASS: full Go tests/vet, UI lint/build, staged/unstaged whitespace. Log: verify.log.
- `npx vitest run` from internal/web/ui PASS: 31 files, 180 tests. Log: ui-tests-final.log.
- `go build -trimpath -o .scratch/memory-ux/evie ./cmd/evie` PASS. Installed at /Users/davidboktor/go/bin/evie; source is current master plus uncommitted UI changes (not a clean committed release).
- Actual installed-server Chrome checks PASS: General list, empty Global, list detail with exact evidence, David person link, literal graph detail, back, Review empty state and secondary Background activity. Desktop and 390px mobile render without horizontal page overflow or page errors.
- Delayed Global object responses released after switching back to General did not replace General's listing. This used deferred genuine responses; final UI checks used unmodified APIs.
- Both read-only Standards and Spec re-reviews PASS after fixes.
- All 180 baseline pre-existing file hashes preserved. No commit, push or merge performed.

Warnings: existing Icon.tsx and new presentation.tsx mixed-export Fast Refresh lint warnings; existing Vite large-bundle warning. These are non-failing development/build warnings. No required checks skipped. Earlier verification failures (trailing whitespace and a temporary test edit error) were corrected; final runs pass. Live smoke testing caught null collections in details and empty scopes, fixed and retested.

## Runtime and review

Server: http://127.0.0.1:6687, PID 80063. Consistent SQLite, prior binary and .env backups are recorded in runtime.json, along with private server log path. Local EVIE_OWNER_NAME=David is configured. Original memory values/source evidence and canonical identity are preserved; no memory decision or chat message was submitted during QA. Automatic compiler configuration remains unchanged.

Refresh the browser, open Data → Memory → General, click either memory to see the original source, then David to inspect the person. Try Graph or Review. Compiler activity remains a secondary diagnostics page; its configuration has not been redesigned in this task. Other application UI is deferred as requested.

Review entry points: internal/web/ui/src/memory/Memory.tsx, presentation.tsx, api/memory.ts, candidateInbox/CandidateInbox.tsx, internal/web/context_sessions.go.
