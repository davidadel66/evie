# Workspace research — implementation and verification

Implemented the September 29 owner request in
[the accepted story](../../cmd/evie/docs/active/workspace-research.spec.md).

- Workers default to **1,048,576 serialized request bytes (1 MiB)** and use the
  invoking chat's selected model. Each has an independent working budget, capped
  by that model's route-safe context window, output reserve, and estimation
  margin. Existing operator overrides remain supported. Parent budgets and the
  2,048-byte per-worker result cap are unchanged.
- Standard Workspaces have an explicit **Allow research delegation** option at
  creation and on the Workspace page. Enabling affects new chats. Disabling
  stops active research and permanently revokes affected old revisions;
  re-enabling grants only new chats. Expected revisions reject stale edits.
- New Web 1.1 chats can fetch relevant sections by literal query and continue
  through UTF-8 byte offsets. Default excerpts are 16 KiB, maximum 32 KiB;
  escaping can reduce them further. Successful results, including redirects,
  fit a 64 KiB serialized envelope. Continuations verify the full text hash.
- Workers receive only their assignment, selected context, and their own
  history. They return concise findings, sources, and limitations. They retain
  only Web search/fetch capabilities. Old Web 1.0 receipts retain their original
  contract; children inherit the parent's pinned version and record compatibility
  evidence before executing.

## Verification

All final required checks passed. No required check was skipped.

| Command | Final result |
| --- | --- |
| `./scripts/verify-change.sh` | Passed: UI lint/build, complete Go suite, Go vet, staged and unstaged whitespace checks. See `verify.log`. |
| `internal/web/ui/node_modules/.bin/vitest run --root internal/web/ui` | 63 files, 375 tests passed. See `ui-tests.log`. |
| `go test -race ./internal/subagents ./internal/tools ./internal/eviedb ./internal/openrouter ./internal/plugins ./internal/web -run 'TestWorkspaceResearch\|TestWorkspaceWorker\|TestWorker\|TestLegacyParent\|TestComposedParent\|TestWebFetchExcerpt\|TestWebExcerptUpgrade\|TestWorkspaceAdmission' -count=1 -timeout 180s` | All six packages passed. See `race.log`. |
| `go test ./internal/tools -run '^TestWebFetchExcerpt' -count=1` | Passed after the final redirect fix. |
| `go test -race ./internal/tools -run '^TestWebFetchExcerpt' -count=1` | Passed after the final redirect fix. |
| `go test ./internal/subagents -run '^TestWorkspaceWorkerReads' -count=1 -v` | Real HTTP excerpts plus SQLite and scripted provider passed; accumulated worker requests exceeded the old 64 KiB budget and the parent received only bounded findings. |
| `go test ./internal/subagents -run 'TestLegacyParent\|TestComposedParent' -count=1 -v` | Legacy compatibility audit and selected-model propagation through the real parent tool invocation passed. |
| `go test ./internal/eviedb -run '^TestWorkspaceResearch' -count=1` | Allowance, stale revision, immutable pins, and revocation persistence across database reopen passed. |
| `node .scratch/workspace-research/verify-workspace-ui.cjs` | Seven browser checks passed, zero page errors, clean fixture shutdown. |

[Browser report](browser-validation-1790699532811/browser-report.json),
[desktop screenshot](browser-validation-1790699532811/desktop-settings.png),
[mobile screenshot](browser-validation-1790699532811/mobile-settings.png).
Browser checks used disposable SQLite and the real HTTP/UI stack. They cover
creation defaults and opt-in, preset changes, keyboard focus, old/new chat
revision pins, toggles and reload, stale edits, successful saves followed by
failed list refresh, and 390px layout. Screenshots were visually inspected.

Existing warnings remain: five React Fast Refresh lint warnings and Vite's
chunk-size warning. One full rerun encountered a transient lease error in
`TestConversationExpansionWindowMeasurements`; that test passed three isolated
runs (`go test ./internal/agent -run '^TestConversationExpansionWindowMeasurements$' -count=3`)
and the final complete verification passed. The failed run is retained in
`verify-transient-lease-failure.log`. Initial Web-version fixture failures were
corrected to exercise current and frozen contracts separately.

## Review

Standards: one redirect-envelope bypass was reproduced with a failing test,
fixed at the new excerpt boundary, and independently rechecked. No remaining
findings. Spec: no missing acceptance criteria, incorrect behavior, or scope
creep identified in the scoped feature changes. Unrelated worktree changes were
excluded from review and preserved.

Review entry points: `internal/openrouter/worker_context.go`,
`internal/subagents/receipt.go`, `internal/subagents/supervisor.go`,
`internal/eviedb/workspace_research.go`, `internal/tools/webexcerpt.go`,
`internal/plugins/web.go`, and
`internal/web/ui/src/workspaces/WorkspaceResearchSettings.tsx`.

## Manual demonstration and boundaries

Rebuild/restart Evie from this checkout. Ensure the Web and Subagents plugins
are enabled, open a Standard Workspace, enable **Allow research delegation**,
then start a new chat. Ask Evie to research a long HTML/text source through
workers using relevant sections, and inspect the concise sourced result.
Disable the Workspace permission during a separate active research run to
observe interruption. Existing chats do not acquire newly added permissions
or Web schemas.

The installed server was not rebuilt/restarted and personal Workspace settings
were not changed. No live model provider was contacted. PDF extraction remains
unsupported; the excerpt reader supports text, HTML, JSON, and XML. Increasing
the worker allowance cannot exceed a smaller selected model's context limit.
No dependency, commit, push, or PR was added.
