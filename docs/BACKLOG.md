# Backlog

Status board for this project, updated by `/wrap` at session end. One line per story; details live in specs (`docs/active/`, `docs/done/` inside each tool's dir).

## Next session
- Last session: YouTube transcript library shipped end-to-end: SQLite/FTS, 6,185-file import, live fetch/channel tools, `ytscribe`, and read-only transcript queries.
- Mid-flight: memory remains a draft spec pending David's approval at `cmd/evie/docs/active/memory.spec.md`; no implementation started.
- Start with: review and approve or revise the memory spec, then choose its first implementation stage.

## In progress
- memory — local Letta-inspired memory design is drafted; pending approval, no code

## Next (ordered tool roadmap, decided 2026-07-29)
- budget analysis verdict — ask evie "how am I tracking against budget?"; transcript decides if step 5 needs a dedicated tool
- review flow live-fire — categorize awaiting-review transactions in chat via edit_db, mint rules from decisions

## Ideas
- OpenCode-inspired project-development backlog — documentation only; project
  runtime, agent profiles (`build`/`plan`/subagents), durable sessions,
  verification, recovery, and deferred extensions are mapped under
  `cmd/evie/docs/research/opencode-backlog/README.md`
- escape/cancel turns (web) — Esc×1 while thinking = scrub the user msg from
  s.messages + restore prompt to composer; Esc×2 (or Esc after content/tools
  start) = plain cancel. Needs: ctx plumbed Send→ChatStream, POST /api/cancel
  (disconnect ≠ cancel is deliberate), Session rollback of the lone user
  message. Harness capabilities in internal/agent, UX web-only. Deferred
  2026-08-08 for bigger fish.
- web frontend (`evie serve`) — rich output door (images/diffs/diagrams); decided over desktop/TUI, see decisions.md
- subagents — evie spawns scoped sub-conversations; design session when long tasks clog one context
- dynamic tool loading — deferred schemas (researched, design in task notes); trigger ~20 tools
- HTML budget report — monthly/on-demand visual; rides the web frontend arc
- self-hosted model (ollama sibling package) — the day the neutral-types refactor earns itself
- TUI polish (bubbletea + glamour) — markdown/spinners; only if terminal remains primary surface

## Recently done
<!-- newest first, max 10; older entries are deleted — git history is the archive -->
- 2026-08-13 YouTube transcripts: normalized SQLite/FTS library, 6,185 legacy imports, `ytscribe`, two Evie tools, read-only query_db integration
- 2026-08-03 web_search (Brave API, raw results); flushed out + fixed the never-loading .env bug in main.go
- 2026-08-03 web_fetch via full autopilot (spec review → independent tests → staged build → code review); 4 review bugs fixed
- 2026-08-03 glob+grep decided AGAINST — bash covers it; revisit only if token cost bites
- 2026-08-01 bash tool (ungated, persistent cwd, shell snapshot) + read_file/edit_file
- 2026-07-29 file-tools spec; budget_limits promoted to standing template; tool-description convention sharpened
- 2026-07-29 SSE streaming + smoothPrinter (first goroutine/channel/select in repo)
- 2026-07-28 categorize writes budget_entries; backfill minted 236 entries (4 human preserved); idempotency proven live
- 2026-07-28 budget_entries + budget_limits tables; budget spec rewritten (reference-not-copy model)
- 2026-07-28 query_db/edit_db general tools; approval gate (NeedsApproval + injected y/N callback)
