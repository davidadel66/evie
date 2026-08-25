# Backlog

Status board for this project, updated by `/wrap` at session end. Keep planned
stories to one line; delivery-discovered issues use the structured form below.
Details live in active plans/specs and completed feature records.

## Next session
- Last session: Memory Epic 1 Stories 3 and 4 were delivered by issues #59/#61
  and merged in pull requests #60/#62, completing Epic 1 and Stage 1.
- Mid-flight: no memory story is active. Epic 2 has not yet been decomposed or
  selected for implementation.
- Start with: work with David in one AI-assisted session to select and refine
  the first dependency-ready, independently verifiable Epic 2 story.

## In progress
- memory — [approved specification](../cmd/evie/docs/active/memory.spec.md);
  Epic 1 complete, with no Epic 2 story currently selected

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

## Discovered issues
<!--
Validated, pre-existing issues discovered during story delivery. Add only
material, independently actionable work that is outside the delivered story.
Do not use this section for speculative ideas, style cleanup, or work required
by the current story's acceptance criteria.
-->

- [ ] Update the UI build chain past vulnerable nanoid 3.3.17
  - ID: `E1-S4-BACKLOG-NANOID-3.3.17`
  - Discovered during: issue #61 at final candidate `247d6688dadf1f62823ae84c31978e61903ae87e`
  - Evidence: The reviewed base and final candidate package lock pin `nanoid` 3.3.17 through the existing Vite/PostCSS chain; `npm audit --json` reports GHSA-2v37-7h3g-55p8 with a fix available.
  - Impact: The pre-existing UI build chain remains exposed to an infinite-loop availability advisory when a custom generator receives size zero.
  - Why deferred: The issue predates Story 4, does not affect its acceptance criteria, and dependency updates are outside the approved provider-usage story.
  - Verification: Complete an approved dependency update, then require `npm audit --json` to omit GHSA-2v37-7h3g-55p8, `npm run build` to pass, and `go test ./...` to pass.

## Recently done
<!-- newest first, max 10; older entries are deleted — git history is the archive -->
- 2026-08-25 memory Story 4: provider-neutral usage evidence, issue #61 / PR #62; Epic 1 complete
- 2026-08-24 memory Story 3: safe titled REPL session selection and restart resume, issue #59 / PR #60
- 2026-08-24 memory Story 2: lease-owned and fenced live turns, issue #57 / PR #58
- 2026-08-13 YouTube transcripts: normalized SQLite/FTS library, 6,185 legacy imports, `ytscribe`, two Evie tools, read-only query_db integration
- 2026-08-03 web_search (Brave API, raw results); flushed out + fixed the never-loading .env bug in main.go
- 2026-08-03 web_fetch via full autopilot (spec review → independent tests → staged build → code review); 4 review bugs fixed
- 2026-08-03 glob+grep decided AGAINST — bash covers it; revisit only if token cost bites
- 2026-08-01 bash tool (ungated, persistent cwd, shell snapshot) + read_file/edit_file
- 2026-07-29 file-tools spec; budget_limits promoted to standing template; tool-description convention sharpened
- 2026-07-29 SSE streaming + smoothPrinter (first goroutine/channel/select in repo)
