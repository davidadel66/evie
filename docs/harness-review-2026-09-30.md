# Harness review — 2026-09-30

Findings from six read-only reviews (turn loop, tools/approvals, durability,
memory, delegation, context management). "Verified" means re-checked in code
or re-run by the main session; the rest are reviewer-verified with file:line
evidence and are re-checked before each fix. Line numbers are as of `fb23624`.

What held up: lease fencing, approval handshake, prepare-then-execute tools,
scope binding for memory, receipts, and fail-closed egress. The problems sit
in budgets, relevance heuristics, and result contracts.

## Issues

### Turn loop (L)

| ID | Issue | Where | Sev | Verified |
|---|---|---|---|---|
| L1 | Model↔tool loop has no step cap; web turns can't be cancelled (runtime ctx, no cancel endpoint), so only a restart stops a runaway | `internal/agent/turn.go:187`, `internal/web/lifecycle.go:17` | High | yes |
| L2 | Chat Completions path ignores `finish_reason` (truncated answers commit as final) and never validates tool-call argument JSON | `internal/openrouter/client.go:210`, `internal/agent/turn.go:732` | Med-High | yes (finish_reason) |
| L3 | No stream idle timeout; a stalled SSE blocks forever while the heartbeat keeps the lease | `internal/openrouter/client.go:54` | Med | yes |
| L4 | No retry/backoff for transient provider errors (429/5xx/reset) before any output streamed | `internal/agent/turn.go:710` | Med | no |
| L5 | Tool-phase callbacks call `selectCause`, which waits on `toolDone` closed by the same goroutine → self-deadlock on caller cancel | `internal/agent/turn.go:533,556`, `ownership.go:176` | Med (rare) | no |
| L6 | Failed automatic compaction is retried on every loop iteration | `internal/agent/turn.go:265` | Low-Med | no |
| L7 | Raw provider error body (unbounded `ReadAll`) reaches the browser on the Chat path | `internal/openrouter/client.go:113` | Low | no |

### Tools and approvals (T)

| ID | Issue | Where | Sev | Verified |
|---|---|---|---|---|
| T1 | `query_db` on `finance` skips `validateSingleSelect`: multi-statement + `ATTACH` writes without approval; `sqlite_dbpage` leaks bank tokens | `internal/tools/db.go:207` | High | yes |
| T2 | Research worker's `web_fetch` can reach loopback/LAN/link-local; no dial-time IP check | `internal/tools/webfetch.go:41`, `internal/plugins/research.go:12` | Med | no |
| T3 | Untrusted-web-content wrapper not escaped; page can close the frame | `internal/tools/webfetch.go:444`, `webexcerpt.go:117` | Med-Low | no |
| T4 | `edit_file` on a symlink replaces the link with a regular file; target unchanged | `internal/tools/file.go:467` | Med | no |
| T5 | Bash buffers all output in memory until timeout (`CombinedOutput`) | `internal/tools/bash.go:222` | Med | no |
| T6 | Bash overflow file name is per-process, so calls overwrite each other; cut can split UTF-8 | `internal/tools/bash.go:272,283` | Low | no |
| T7 | Oversized web-fetch temp files are never deleted | `internal/tools/webfetch.go:382` | Low | no |

### Durability and sub-agent supervision (D)

| ID | Issue | Where | Sev | Verified |
|---|---|---|---|---|
| D1 | Parent-authority CTE stops at `depth<128`; delegation refused after ~43 tool rounds | `internal/eviedb/subagents.go:206` | Med | yes |
| D2 | `policy.Deadline` bounds the whole batch; queued children reported `parent_cancelled`, keys burned | `internal/subagents/supervisor.go:93` | Med | yes |
| D3 | Watchdog maps any store error (incl. `SQLITE_BUSY`) to `ErrAuthority`; 20 ms write-lock polling | `internal/subagents/supervisor.go:241` | Med | no |
| D4 | Recovery stops forever on first error; all attempts recovered in one transaction | `internal/subagents/recovery.go:19`, `eviedb/subagent_execution.go:257` | Med-Low | no |
| D5 | Capacity-wait path can strand an attempt in `admitted` | `internal/subagents/supervisor.go:200` | Low | no |
| D6 | No tests for recovery after reopen, transient store errors, deadline classification | — | Med | — |

### Delegation contract (G)

| ID | Issue | Where | Sev | Verified |
|---|---|---|---|---|
| G1 | Child doesn't know its budget (8 calls / 2 min / 1024 tokens); hitting a cap returns empty `failed`, discarding all work | `internal/subagents/budget.go:34`, `eviedb/subagent_execution.go:110` | High | yes |
| G2 | 2 KiB result cap; truncation drops sources first then tail of findings; full answer unreachable afterwards | `eviedb/subagent_execution.go:184` | Med-High | yes (cap) |
| G3 | "Sources" are regex-extracted URLs, never checked against fetch/search events | `eviedb/subagent_execution.go:167` | Med | no |
| G4 | Child findings reach the parent without untrusted framing | `internal/plugins/subagents.go:60` | Med | no |
| G5 | Replayed results carry no replay flag/timestamp; key conflicts don't name the key | `internal/delegation/types.go:88`, `plugins/subagents.go:68` | Med | no |
| G6 | No aggregate delegation cap per turn; failed children report null usage | `supervisor.go:108`, `subagent_execution.go:148` | Med | no |
| G7 | Opaque errors: one message for six validation failures; `policy_limit` ambiguous; one sibling error discards all results | `delegation/types.go:60`, `supervisor.go:151` | Low-Med | no |
| G8 | Running child sessions appear in `ListActiveSessions` / sidebar / REPL chooser | `eviedb/sessions.go:149` | Low | no |
| G9 | Parent delegation instructions give no effort scaling (how many workers, how much work each) or expected result format | `internal/agent/prompt.go:20` | Med | yes |
| G10 | A child that stops at a limit can't be continued; the parent can only start over with a new key | `internal/subagents/supervisor.go` | Med | yes |

### Memory (M)

| ID | Issue | Where | Sev | Verified |
|---|---|---|---|---|
| M1 | Automatic recall has no relevance floor and always adds earlier-topic terms ("thanks!" → searches unrelated past topics) | `internal/agent/retrieval_automatic.go:86,114`, `eviedb/retrieval.go:60` | High | yes (re-ran probe) |
| M2 | Corrected claims leave old source messages recallable as current owner statements; repeated unlinked statements bypass retirement | `eviedb/retrieval_temporal.go:38`, `semantic_correction.go:349` | High | no |
| M3 | Newer-statement discrepancy detection only fires when the new message reuses the predicate wording | `eviedb/retrieval_conflict.go:165` | High | no |
| M4 | Label/cardinality differences mint a new predicate version; conflicts keyed on predicate ID go silent | `eviedb/semantic.go:1166`, `semantic_conflicts.go:128` | High | yes (re-ran probe) |
| M5 | Model-proposed memories cite the whole root user message as `owner_statement`: web-injected values launder into owner authority; Global message text reaches Workspace sessions | `internal/agent/turn.go:576`, `eviedb/semantic.go:1233` | Med | no |
| M6 | Entity resolution reuses a single alias match silently; retrieval doesn't mark ambiguous aliases; approval card hides reuse | `eviedb/semantic_entity.go:218`, `retrieval_graph.go:146` | Med | no |
| M7 | Dense recall scans only the first 4,096 vectors (random-UUID order); truncation not surfaced | `eviedb/retrieval_dense.go:45` | Med | no |
| M8 | No evaluation signal at realistic scale (24-case corpus; Stage 9 spec-deferred) | `memory.spec.md:1449` | Med | — |

### Context management (C)

| ID | Issue | Where | Sev | Verified |
|---|---|---|---|---|
| C1 | Compaction plans against a worst-case ~97 KB summary, so it never runs below ~205k windows → session stuck on `context_overflow`; `/context` reports false headroom; no `/compact` in web | `internal/agent/automatic_compaction.go:122`, `context.go:450`, `cmd/evie/repl.go:732` | High | yes (mechanism) |
| C2 | Cumulative memory budget re-charges every re-send; long tool turns die with `context_overflow` at ~10% full | `internal/agent/retrieval_budget.go:33` | High | yes (mechanism) |
| C3 | Tokens estimated as bytes (≈4.5× over); recorded provider usage never used to calibrate | `internal/agent/context.go:57` | Med-High | no |
| C4 | Prompt-cache hostile: task `revision` in message 2, memory block before active root, projection churns mid-history; no `cache_control` for Anthropic | `eviedb/tasks_scope.go:342`, `context.go:208,237`, `openrouter/schema.go:13` | Med | no |
| C5 | Projected/compacted tool output can't be re-read (no read-by-event-id) | `internal/agent/tool_results.go:184` | Med | no |
| C6 | Rolling summary re-summarizes itself (drift) and is injected as a **system** message | `internal/agent/compaction.go:438`, `context.go:234` | Med | no |
| C7 | Metadata fallback covers only `kimi-k3`, not the default model; lookup failure `log.Fatalf`s at startup | `openrouter/context_profile.go:166`, `cmd/evie/main.go:168` | Low-Med | no |
| C8 | Provider 400 context-length errors not recovered by compact-and-retry | `internal/agent/turn.go:710` | Low | no |

## Decisions (David, 2026-09-30)

- **Memory sources (M5):** bind each model-proposed memory to the exact span
  of the owner's message that contains the value. A value not in the owner's
  own words is stored as Evie-proposed, not `owner_statement`, and the
  approval card says so.
- **Compaction summary (C6):** send it as a user-role block labelled as a
  summary of past conversation, not as a system message.
- **Private network (T2):** delegated workers cannot fetch loopback, private,
  or link-local addresses (checked at dial time). The main chat keeps access.
- **Sub-agents (G1, G2, G10):** follow the pattern Anthropic's research
  system, Claude Code, and the OpenAI Agents SDK share:
  - budget by wall-clock time (default 15 min) and tokens, not request count;
    the child compacts its own context like the main chat;
  - when the budget runs low, one final tool-free call produces the report,
    returned as `partial`; the parent can continue the child instead of
    restarting;
  - the full report is stored; the parent gets the summary section inline
    (asked for at 1–2k tokens) plus a read tool for the rest; sources come
    from pages the child actually fetched.

## Defaults chosen (redirect if wrong)

- L1: runaway guard of 100 model calls per turn (env-configurable); at the
  cap the model gets one final tool-free call to answer with what it has.
- L4: retry at most 2 times with backoff, only before any output streamed.
- C3: calibrate bytes-per-token per model from recorded `input_tokens`, with
  a conservative floor of 3 bytes/token until enough samples exist.
- Sub-agent token budget: 1M tokens per child (input + output); wrap-up at
  90% of time or tokens. At most 16 children per parent turn.
- M1: earlier-topic terms join the query only when they overlap the current
  message or the current message is a short follow-up; injected items must
  match at least one distinctive current-message term.
- M2: corrected sources stay retrievable but are labelled historical and
  linked to the correction, per memory decision 10.
- M6: add an ambiguity marker and show entity reuse on the approval card;
  merge/split operations stay out of scope.

## Plan

**Progress:** stages 1, 7, 11 merged (2026-10-01). Stage 11's baseline is in
`cmd/evie/docs/active/memory-scale-eval.md`; stages 12–13 re-record it.

Each stage is one reviewable change: focused tests for every fixed issue
(failing first), `gofmt`, then `./scripts/verify-change.sh`. A stage that
changes specified behavior updates its spec or decisions file in the same
change. After each stage: a real input/output demo on offer.

### 1. Tool fences — T1–T7
- **Outcome:** ungated tools can't write, leak, or escape their fences.
- **Accept:** finance `query_db` rejects multi-statement, `ATTACH`, and
  `sqlite_dbpage`; a worker fetch to `127.0.0.1`, `192.168.x`, `169.254.x`,
  or a DNS name resolving there fails, including via redirect; page text
  containing the end marker can't close the wrapper; `edit_file` through a
  symlink edits the target and keeps the link; bash holds at most the cap in
  memory, overflow files are unique per call, and cuts are UTF-8 safe;
  oversized fetch temp files are removed.
- **Docs:** web-fetch decisions record the worker exception.
- **Risk:** dial-time check must not break the main chat's localhost access.

### 2. Provider robustness — L2, L3, L4, L7, C7
- **Outcome:** provider hiccups don't corrupt history or hang sessions.
- **Accept:** `finish_reason` `length`/`error` is not committed as a final
  answer; invalid tool-argument JSON is rejected before durable commit; a
  stream silent past the idle timeout fails the turn and releases the lease;
  429/502/503/504/reset before output retries up to 2 times; raw error
  bodies never reach the browser and are read with a cap; the default model
  has a context fallback and startup survives a metadata failure.
- **Risk:** retries must never repeat a call after output or tool effects.

### 3. Loop control — L1, L5, L6
- **Outcome:** no turn can run unbounded or deadlock.
- **Accept:** a model that always calls tools stops at the cap with a final
  tool-free answer; caller cancellation during a tool returns instead of
  hanging; a failed compaction is not retried within the same turn.
- **Depends on:** 2.

### 4. Context budgeting — C1, C2, C3, C8
- **Outcome:** long sessions keep working on 128k and 200k models.
- **Accept:** compaction plans against a realistic summary bound and runs on
  a 128k working ceiling; `/context` reports what the turn path actually
  sends; memory budget charges each memory byte once per turn, not per
  resend; token estimates use calibrated ratios; a provider context-length
  400 triggers one compact-and-retry.
- **Docs:** context/compaction decisions updated (estimation no longer
  deferred).
- **Depends on:** 3.

### 5. Web turn controls — L1 (cancel), C1 (`/compact`)
- **Outcome:** you can stop a turn and compact from the browser.
- **Accept:** a Stop button cancels the active turn through
  `POST /api/cancel` (tab close still doesn't cancel); `/compact` works in
  the web composer.
- **Non-goals:** Esc-to-rewind from the backlog idea.
- **Depends on:** 3, 4.

### 6. Context shape and caching — C4, C5, C6
- **Outcome:** stable, cacheable prefixes and recoverable tool output.
- **Accept:** task focus changes don't alter the prefix before history; the
  summary is a labelled user-role data block; projection changes only at
  compaction points; Anthropic models get `cache_control` breakpoints; the
  model can re-read a projected tool result by event ID.
- **Docs:** `memory.decisions.md:405` updated for the summary role.
- **Depends on:** 4.

### 7. Sub-agent durability — D1–D6
- **Outcome:** supervision is correct under load and after crashes.
- **Accept:** delegation works after 100+ parent tool rounds; queued
  children at the deadline are reported as deadline, not parent-cancelled;
  `SQLITE_BUSY` is retried, not treated as authority loss; recovery retries
  with backoff and isolates bad records; no attempt is left `admitted`.
  Each has a regression test.

### 8. Sub-agent budget and results — G1, G2, G3, G6
- **Outcome:** a research child always returns its work.
- **Accept:** budget is time + tokens and stated to the child; at 90% the
  child gets a tool-free wrap-up call and returns `partial` with findings;
  the full report is stored and readable via `read_subagent_report`; the
  inline result is the summary section; sources are fetched URLs marked
  cited or not; usage is reported for every outcome; at most 16 children
  per parent turn.
- **Docs:** subagents spec (story 32, limits sections) and decisions.
- **Depends on:** 3, 4, 7.

### 9. Sub-agent continuation — G10
- **Outcome:** the parent can extend a partial child instead of restarting.
- **Accept:** `continue_research(execution_id, message)` resumes the same
  child session with a fresh budget slice, under the same authority checks.
- **Depends on:** 8.

### 10. Sub-agent contract polish — G4, G5, G7, G8, G9
- **Outcome:** the parent model gets clear, safe, honest results.
- **Accept:** child findings arrive in an untrusted-data frame; replayed
  results carry a replay flag and timestamp; errors name the field, key, or
  limit; one failed sibling doesn't discard the others; child sessions are
  hidden from session lists; the delegation prompt includes effort scaling
  and a result format.
- **Depends on:** 8.

### 11. Memory evaluation at scale — M8
- **Outcome:** recall changes are measured, not guessed.
- **Accept:** a replay corpus built from real-history shape with many
  distractors reports precision, unwanted-item rate, and stale-fact misses;
  runs deterministically in tests.
- **Non-goals:** learned rankers.

### 12. Memory recall relevance — M1, M7
- **Outcome:** recall injects only material relevant to the current message.
- **Accept:** "thanks!" injects nothing unrelated; the reviewer's privacy
  scenario injects neither private message; unwanted rate drops on the
  stage-11 corpus with no recall loss on the existing held-out set; dense
  recall covers all vectors or reports truncation.
- **Depends on:** 11.

### 13. Memory currency and conflicts — M2, M3, M4
- **Outcome:** corrected and contradicted facts never surface as current.
- **Accept:** after a correction, the old source is labelled historical and
  linked; a retired fact restated without a link is flagged; a later
  statement with different wording is detected; label or cardinality drift
  still produces conflict warnings.
- **Depends on:** 11.

### 14. Memory authority and entities — M5, M6
- **Outcome:** owner authority means the owner actually said it.
- **Accept:** a proposed memory's source is the exact quoted span; a value
  absent from the owner's words is saved as Evie-proposed and the card says
  so; Global text doesn't reach Workspace sessions; alias reuse is shown on
  the card and ambiguous aliases are marked in recall.
- **Docs:** memory spec invariants for source binding.
