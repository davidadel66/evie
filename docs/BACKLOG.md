# Backlog

Status board for this project, updated by `/wrap` at session end. One line per story; details live in specs (`docs/active/`, `docs/done/` inside each tool's dir).

## Next session
- Last session: budget pipeline shipped end-to-end (entries + limits + categorize migration), approval gate + general query_db/edit_db tools, SSE streaming with smooth pacing, file-tools spec written.
- Mid-flight: budget analysis never exercised — limits template is seeded (18 categories), 321 entries live, ~160 transactions awaiting review.
- Start with: /tutor the file-tools build from cmd/evie/docs/active/file-tools.spec.md — resolve its four open questions first, then David writes read_file/edit_file.

## In progress
- file-tools: read_file + edit_file — spec committed, build is David's next tutor session

## Next (ordered tool roadmap, decided 2026-07-29)
- cron — schedule recurring shell commands; first customer: daily finance sync + categorize
- write_file — only if file-tools session keeps creation separate
- budget analysis verdict — ask evie "how am I tracking against budget?"; transcript decides if step 5 needs a dedicated tool
- review flow live-fire — categorize awaiting-review transactions in chat via edit_db, mint rules from decisions

## Ideas
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
- 2026-08-03 web_search (Brave API, raw results); flushed out + fixed the never-loading .env bug in main.go
- 2026-08-03 web_fetch via full autopilot (spec review → independent tests → staged build → code review); 4 review bugs fixed
- 2026-08-03 glob+grep decided AGAINST — bash covers it; revisit only if token cost bites
- 2026-08-01 bash tool (ungated, persistent cwd, shell snapshot) + read_file/edit_file
- 2026-07-29 file-tools spec; budget_limits promoted to standing template; tool-description convention sharpened
- 2026-07-29 SSE streaming + smoothPrinter (first goroutine/channel/select in repo)
- 2026-07-28 categorize writes budget_entries; backfill minted 236 entries (4 human preserved); idempotency proven live
- 2026-07-28 budget_entries + budget_limits tables; budget spec rewritten (reference-not-copy model)
- 2026-07-28 query_db/edit_db general tools; approval gate (NeedsApproval + injected y/N callback)
- 2026-07-28 finance_query shipped, then generalized into query_db
- 2026-07-17 budget feature spec (categories, template+overrides, async review, netting refunds)
- 2026-07-16 cmd/agent → cmd/evie; finance_sync/rules/categorize tools; internal/tools package
- 2026-07-15 internal/todo + internal/finance extractions; repo on GitHub
