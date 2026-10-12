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

## finance-daily follow-ups (decided 2026-10-06, see cmd/evie/docs/active/finance-daily.*)
- finance-review (2026-10-07, cmd/evie/docs/active/finance-review.*): proposed in the Finance Workspace; needs David's approval, then the first Monday 09:30 run or a chat start; answer on the Workspace page or `evie workflows answer <run> review --answer '{}'`
- finance-daily approved 2026-10-06; first scheduled run 2026-10-07 09:00 America/Detroit succeeded (run 9cbf8363); cron job 3 removed the same day
- delete `cmd/finance` after one clean week and a parity check of Plaid link and rules seeding in Evie
- external delivery of workflow notifications (a failed finance-daily run is only visible in `evie workflows status` / `notifications`)
- `scripts/release.sh` (2026-10-07) verifies, installs and restarts serve in one step; use it for every change so `evie serve` never runs stale code
- categorize-patterns landed 2026-10-06 (name rules, `excluded`; 339 → 125 awaiting review). Left for chat review: 13 merchant-less rows (5× `TRANSFER CONSUMERS CU`, mobile deposits, Apple Pay, a hotel folio) and 112 one-off merchants; Venmo (48 rows, $5,942.52) is excluded as a transfer per the August decision and worth a second look
- move categorize-patterns.spec.md to cmd/finance/docs/done once David has eyeballed the excluded list

## Next (ordered tool roadmap, decided 2026-07-29)
- budget analysis verdict — ask evie "how am I tracking against budget?"; transcript decides if step 5 needs a dedicated tool
- review flow live-fire — categorize awaiting-review transactions in chat via edit_db, mint rules from decisions

## Ideas
- OpenCode-inspired project-development backlog — documentation only; project
  runtime, agent profiles (`build`/`plan`/subagents), durable sessions,
  verification, recovery, and deferred extensions are mapped under
  `cmd/evie/docs/research/opencode-backlog/README.md`
- escape-to-rewind (web) — Esc×1 while thinking = scrub the user msg from
  s.messages + restore prompt to composer; Esc×2 (or Esc after content/tools
  start) = plain cancel. Plain cancel shipped 2026-10-01 as the composer Stop
  button over POST /api/cancel (disconnect ≠ cancel still deliberate); what
  remains needs Session rollback of the lone user message (harness capability
  in internal/agent, UX web-only). Deferred 2026-08-08 for bigger fish.
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

- [ ] Workspace folder chooser does not let the owner pick a folder
  - ID: `CH-S1-BACKLOG-FOLDER-CHOOSER`
  - Discovered during: Changes story 1 setup, 2026-10-09 (creating the evie Workspace on `~/code/evie`)
  - Evidence: Choosing a folder in Create workspace did not yield a folder; typing the path in Settings → Folder worked. The chooser is `osascript` JXA (`NSOpenPanel`) run by the launchd-managed `evie serve` (`internal/web/native_folder_picker.go`), and its stderr is discarded, so `~/.evie/logs/serve.log` records nothing. Exact symptom (no dialog, dialog behind the browser, or a disabled choice) not yet confirmed.
  - Impact: Creating a Workspace with a folder needs a second step through Settings → Folder.
  - Why deferred: Pre-existing; outside Changes story 1.
  - Verification: Reproduce from the launchd-managed serve, log the picker's failure, then choose `~/code/evie` from Create workspace end to end.

- [ ] Self-sufficient tool results when a tool declines or does nothing
  - ID: `WA-S4-BACKLOG-SELF-SUFFICIENT-RESULTS`
  - Discovered during: the same demo
  - Evidence: `workflow_start` returned `existing: true` and "nothing new was started" within the first minute; Evie then spent several minutes establishing what the result left out (what the run did, that equal inputs are one run by its Run Key, how a rerun works) from the database, the procedural repo, ADRs and Evie's source.
  - Impact: Slow, costly turns that rediscover facts the tool already had.
  - Why deferred: A convention for every tool, not part of the run-card story.
  - Verification: Every declined, refused or no-op tool result states what happened, why, and what only the owner can do next (with the UI path); for `workflow_start`, the existing run's outcome, the Run Key rule and "rerun: your Rerun button on this run's card, with a reason". Tests per tool assert those fields.

- [ ] Per-turn budget with a check-in gate
  - ID: `WA-S4-BACKLOG-TURN-BUDGET`
  - Discovered during: the same demo
  - Evidence: The turn ran past 2m38s and 20+ tool calls with no limit; no per-turn time or tool-call budget exists in `internal/agent`.
  - Impact: Any over-deliberation runs unbounded until the owner presses Stop.
  - Why deferred: A runtime mechanism for every turn, not part of the run-card story.
  - Verification: Past a configured wall time or tool-call count, the turn pauses at a check-in card (Continue / Wrap up / Stop); Wrap up ends with a final answer from what is known; tests cover each choice and that approvals still gate as before.

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
