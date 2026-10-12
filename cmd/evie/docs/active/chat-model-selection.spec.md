# Chat model selection

Status: implemented for David's 2026-09-29 request for a chat dropdown grouped by provider.

## Outcome and acceptance

- The composer offers a labeled, keyboard-accessible model dropdown grouped by
  the model author's provider namespace (OpenAI, Anthropic, Google, etc.). All
  models still run through OpenRouter. The catalog includes models advertising
  text input/output and tool support; selection still validates route context
  limits and the existing model-specific reasoning configuration.
- The selected model applies to subsequent foreground turns in this chat,
  including compaction. Existing history, scope and pinned capabilities stay
  intact. Save the model ID and a revision in SQLite; reopening the chat or
  restarting the server restores it. New chats use `EVIE_MODEL` or the existing
  application default. Delegated workers retain the supervisor's startup model.
- Model selection cannot overlap an active foreground turn or another session
  transition. The UI also disables it while messages are queued. Exact session
  identity and revision reject stale updates; chat sends carry the displayed
  model so another browser cannot silently change their model.
- Resolve a fresh immutable context profile before committing a selection.
  The default working ceiling may shrink to the route-safe hard window for UI
  chats; explicit environment limits remain strict. Output reserve and margin
  must still fit. Unsupported selections leave the current model unchanged.
- Catalog and selection errors appear beside the dropdown with Retry. A catalog
  provider failure still returns the current model so the chat remains usable.
  Model metadata fetches have time and byte bounds; credentials and raw provider
  errors never enter the browser. Existing same-origin management guards apply.

## Decision amendment

This request supersedes the startup-only/no-session-override clause in
[memory.decisions.md](memory.decisions.md) for web foreground chats. A profile
remains immutable within an agent runtime and turn; switching creates a new
runtime over the same durable history. CLI and delegated worker profile rules,
transport selection, memory authorization and context evidence are unchanged.

## Dependencies, risks and non-goals

Requires the existing OpenRouter account, catalog and route metadata. Catalog
capabilities do not guarantee account access, provider availability or response
quality. No provider credentials UI, automatic fallback, pricing display,
reasoning selector, global preference change, or worker-model switching.

## Verification and demonstration

Use HTTP fixtures for catalog filtering/bounds and smaller-model context limits,
SQLite tests for restart/stale/busy writes, controller tests for selecting and
resuming a model over existing history, guarded HTTP tests, UI API/render tests,
and a real-browser composer check. Run focused checks, the UI suite, Go race
checks for affected runtime boundaries, and `./scripts/verify-change.sh`.

Open a chat, choose a model under a provider heading, send a message, and reopen
the chat to confirm the selection. During a reply the selector is disabled.
Failed selection preserves both the draft and the previously selected model.

Catalog source: [OpenRouter Models API](https://openrouter.ai/docs/api/api-reference/models/list-all-models-and-their-properties).

## Verified 2026-09-29

- `go test ./internal/openrouter ./internal/eviedb -run 'Test(ChatModel|SelectedChat|SessionModel)'` — passed.
- `go test ./internal/web ./cmd/evie ./internal/eviedb -run 'Test(ChatModel|SessionModel)'` — passed, including actual request model, retained history, restart, and stale-runtime fencing.
- `./internal/web/ui/node_modules/.bin/vitest run --root internal/web/ui` — 370 tests passed in 62 files. The affected model API/selector tests also passed after final cleanup.
- `go test -race ./internal/web ./internal/eviedb ./cmd/evie -run 'Test(ChatModel|SessionModel|SelectedChat)'` — passed.
- `./scripts/verify-change.sh` — passed on the final implementation: Go tests/vet,
  UI lint/build, and staged/unstaged whitespace checks. Five existing Fast Refresh
  export warnings and the existing Vite large-chunk warning remain; no new lint warnings.
- `go test -c -o .scratch/chat-model-selection/browser.test ./cmd/evie` followed by
  `EVIE_PLAYWRIGHT_MODULE=/Users/davidboktor/code/evie/.scratch/memory-stage-4/browser-driver/node_modules/playwright-core node scripts/chat-model-selection-browser.cjs`
  — seven Chrome scenarios passed with zero page errors: grouping/keyboard focus,
  draft/reload preservation, failed selection, busy reply, stale-browser recovery,
  returning parked queued messages to the draft, and 390px layout. The fixture
  uses a temporary database and synthetic catalog/provider. Reports, screenshots,
  and verification logs are under `.scratch/chat-model-selection/`.
- Parallel Standards and Spec reviews: no remaining actionable findings.

No required checks were skipped. Live provider calls were not run: these checks
prove local selection, persistence, request wiring and UI behavior, not account
access or interoperability for every model. The running installed server was not
replaced or restarted; rebuild/restart `evie serve` to load this checkout's changes.

Review entry points: `internal/web/models.go` (guarded selection),
`cmd/evie/web_context_sessions.go` (runtime and persistence ordering),
`internal/eviedb/session_models.go` and `turn_leases.go` (revision/turn fencing),
and `internal/web/ui/src/chat/ModelSelector.tsx` (grouped control).

## Model control amendment — October 3, 2026

David asked for no always-visible model control: `/model` opens a picker,
following T3 Code. This supersedes the labeled dropdown above; catalog,
selection, persistence, fencing and busy rules are unchanged.

- No model control is shown in the composer. Exact `/model` (trimmed), typed
  and sent or chosen from the composer's command menu, opens a searchable
  picker above the composer, grouped by provider, with the current model
  marked. It is never sent as a message, including while a reply runs;
  `/model <anything>` reports `Usage: /model`.
- The search field takes focus on open. Arrow keys move through models, Enter
  in the search field chooses the first match, Escape or a choice closes the
  picker and returns focus to the composer. Clicking elsewhere closes it.
- While a turn runs, messages are queued, a selection is saving, or the session
  is changing, models are disabled and the picker says the model can change
  once the reply ends. Selection and catalog errors appear below the composer
  with Retry. The picker closes when the active session changes.
- Typing a bare `/` prefix opens a command menu listing matching commands
  (`/model`, `/compact`) with descriptions. Arrow keys move the highlight, Enter
  or a click runs the highlighted command exactly as if it were sent, Tab
  completes it into the draft, and Escape dismisses the menu until the draft
  changes. Any whitespace, argument or other text closes it.

Verification on October 3, 2026: UI suite with composer command, command menu
and picker tests; the updated `scripts/chat-model-selection-browser.cjs` passed
seven Chrome scenarios with zero page errors (command menu filtering,
navigation, dismissal and completion; keyboard picker; reload; failed
selection; `/model` during a reply; stale-browser recovery; parked queue;
390px menu and picker fit).

## Selectable catalog amendment — October 11, 2026

David asked that `/model` list only models Evie can select. Picking OpenRouter's
"Claude Opus Latest" failed, and a live check found 125 of 390 catalog chat
models failing selection. The catalog still requires text input/output and
tool support, and now also excludes, using only the catalog list's metadata:

- Variant IDs with a `:` suffix such as `:batch` or `:free`. Batch variants run
  only through OpenRouter's batch API; other variants fail the route identity
  check.
- Router models, whose architecture tokenizer is `Router` (for example
  `~anthropic/claude-opus-latest` and `openrouter/auto`). They list no
  endpoints, so no route-safe window exists.
- Models that support neither `max_tokens` nor, for the Responses transport,
  `max_completion_tokens` or `max_output_tokens`.
- Models whose top provider's maximum completion tokens are missing or below
  the configured output reserve (`EVIE_CONTEXT_OUTPUT_RESERVE_TOKENS`, default
  16,384).

Selection still resolves the route context profile before committing. A listed
model can still fail there, because per-endpoint status and limits are not in
the catalog list: on October 11, 2026, 2 of 255 listed models failed
consistently and up to 4 more failed while their providers were down. Such a
selection fails as before and leaves the current model unchanged. Catalog fixture tests cover each exclusion and a
kept model.
