# Harness improvements — TODO

The UI is a *surface*. This doc is about the thing underneath it: the loop in
`internal/agent/agent.go` that decides what the model sees, how much it costs,
and whether a long task survives to the end. Right now that loop is ~120 lines
and does the minimum. That's the correct starting point — everything below is
the list of things labs actually build on top of it, ordered by what buys the
most per hour of work.

Each item is scoped as something David builds: **what**, **why it matters**,
**how it lands in evie**, **done when**. No item is a refactor for its own sake.

Reference material this was drawn from is at the bottom — Anthropic's docs and
engineering posts, the `claude-code` source, Prime Intellect's Prime Agent
(open source, TypeScript), and Meta's Muse Code writeup.

---

## Where evie stands today (audit, 2026-08-06)

Facts, so the gaps below are concrete rather than aspirational:

| Concern | Today |
|---|---|
| System prompt | One sentence, hardcoded in `agent.New()` as `messages[0]` |
| Prompt caching | None. No `cache_control` anywhere in the repo |
| Token accounting | `ChatResponse` is `{Choices}` — the provider's `usage` block is parsed away and discarded |
| Context management | None. `s.messages` grows unbounded until the provider errors |
| Transcript persistence | None. The conversation dies with the process |
| Tool surface | 16 tools, flat list, all schemas sent every turn |
| Approval gate | **Exists and is good** — `NeedsApproval` + `Decision{Declined, Approved, Expired}`, fails closed |
| Frontend decoupling | **Exists and is good** — `Events` interface, REPL + web both drive the same `Session` |
| Evals | None |
| Provider | OpenRouter, `moonshotai/kimi-k3` |

The two "good" rows matter: the `Events` seam and the approval gate are the
parts that are hardest to retrofit, and they're already right. The rest of this
doc is filling in the parts that were correctly deferred.

**Provider caveat, applies throughout:** evie talks to OpenRouter, not the
Anthropic API. Anthropic-specific betas (`context-management-2025-06-27`,
`compact-2026-01-12`, `task-budgets-2026-03-13`) are **reference material for
how to think about the problem**, not drop-in features. Anything that must work
today has to be implemented client-side in Go. That's actually the more valuable
version to build — you learn the mechanism instead of setting a flag.

---

## Do these three first

1. **Capture `usage`** (#1) — an afternoon, and it's the prerequisite for
   honestly evaluating #3, #5, and #6. You cannot tune what you don't measure.
2. **Extract the system prompt to a file** (#2) — an afternoon, immediately
   changes behavior quality, and makes caching viable by giving the prefix
   enough tokens to be worth caching.
3. **Persist the transcript as JSONL** (#4) — a session, and it unlocks resume,
   compaction, debugging, and eventually evals. Every serious harness surveyed
   has this; evie is the only one that doesn't.

---

## 1. Capture and expose token usage

**What.** Parse the `usage` object from provider responses and thread it through
to frontends.

**Why.** Everything downstream is guesswork without it. Whether caching works is
a question `cache_read_input_tokens > 0` answers definitively. Whether
compaction is needed is a question `input_tokens` answers. Cost per turn is a
question you currently cannot answer at all. This is the instrumentation that
makes the rest of the list empirical instead of vibes-based.

**How it lands.** `openrouter.ChatResponse` grows a `Usage` field.
OpenRouter's chat-completions shape gives `prompt_tokens`, `completion_tokens`,
`total_tokens`, and a `prompt_tokens_details.cached_tokens` when the upstream
provider reports cache hits. The struct is roughly:

```go
// Usage is the provider's accounting for one request. Cached is the
// portion of Prompt that was served from a prompt cache — zero when
// nothing cached, which is how you tell caching isn't working.
type Usage struct {
    Prompt     int `json:"prompt_tokens"`
    Completion int `json:"completion_tokens"`
    Total      int `json:"total_tokens"`
    Details    struct {
        Cached int `json:"cached_tokens"`
    } `json:"prompt_tokens_details"`
}
```

Then a decision: does `Events` grow a method (`Usage(u openrouter.Usage)`), or
does `Session` accumulate a running total that frontends read? Prefer the
`Events` method — it matches the existing "everything worth rendering flows
through Events" contract, and a cumulative total is trivially derived by the
frontend that wants one. Note the streaming path needs care: in SSE mode the
usage block arrives on the final chunk (OpenRouter needs
`stream_options.include_usage` or equivalent), so `ChatStream` has to look for
it there rather than in a non-streaming body.

**Done when.** The REPL can print `~2,431 tok in / 180 out / 0 cached` after a
turn, and the web UI has the numbers available even if it doesn't render them
yet.

---

## 2. A real system prompt, in a file, in sections

**What.** Replace the one-line string in `agent.New()` with a sectioned prompt
loaded from a file, plus the ambient context a personal assistant needs.

**Why.** The current prompt is *"You're name is Evie - A helpful assistant to
David with the intent to be his personal 'jarvis'"* (typo included). It tells
the model nothing about: what tools exist and when to prefer which, that it's
talking to David specifically, what today's date is, what machine it's on, how
verbose to be, or what "done" looks like. Every capability gap you've noticed in
chat is at least partly a prompt gap.

Anthropic's context-engineering guidance is explicit about the failure modes on
both ends: hardcoded brittle if-then logic on one side, vague
"be helpful" gestures on the other. The recommendation is distinct sections —
background, instructions, tool guidance, output format — at the **minimum
information sufficient** to get the behavior you want, then iterate by
examining actual failures.

**How it lands.** A few sub-decisions worth making deliberately:

- **Where does it live?** `internal/agent/prompt.md` embedded with `go:embed`
  is the clean answer: it ships with the binary (no runtime file dependency, no
  "works on my machine"), it's diffable in git, and it doesn't need path
  resolution. This is worth doing as your first `go:embed` — it's a small, very
  idiomatic corner of the stdlib.
- **What's static vs. dynamic?** Critical for #3. The static body is one text
  block. Dynamic facts (date, cwd, hostname, git branch) go in a *separate,
  later* block so they don't invalidate the cached prefix. See #3.
- **Sections to include:** who Evie is and who David is; the tool inventory with
  *when to prefer which* (bash vs. read_file, query_db vs. finance_*); response
  style; the approval-gate contract (some tools require confirmation — don't
  apologize for it, don't try to route around it with bash); how to handle
  ambiguity.
- **Personal-assistant context:** the fact that David has a `todo` list, a
  `finance` db with a budget, and cron jobs is *knowledge*, not just tool
  availability. The model should know these exist without having to probe.

One trap worth naming: the tool-preference guidance duplicates information
that's already in tool descriptions. Keep the *when to choose between tools*
guidance in the system prompt and the *how to call this tool* guidance in the
description. Don't write the same sentence twice — you'll change one and not the
other.

**Done when.** `prompt.md` exists with named sections, `New()` loads it, and
you've run at least three real tasks you previously found awkward to see whether
the prompt fixed them. Record what changed in `docs/decisions.md`.

---

## 3. Prompt caching

**What.** Get the static prefix (tools + system prompt) served from cache
instead of re-billed every turn.

**Why.** In a tool-using loop, the same prefix is re-sent on *every iteration*
of the inner `for` loop, not just every user turn. A turn with six tool calls
sends the prefix seven times. Cache reads run ~0.1× input price on Anthropic;
the effect on a long agentic session is large. Latency drops too.

**How it lands.** Three parts, in order:

**(a) Get the render order right.** The cacheable prefix is
`tools` → `system` → `messages`, and *any* byte change at one level invalidates
that level and everything after it. Today `agent.go` builds
`Tools: tools.SchemasWith(extra)` — and `extra` is per-turn (the web frontend
passes `show`, the REPL passes none). **Tools appended conditionally at the
front of the render order means the cache breaks whenever the frontend differs.**
Worse, `SchemasWith` appends extras *after* the base list, which is the right
choice for cache stability (base prefix stays byte-identical) — but only if the
base list itself is stable and deterministically ordered. It is today, because
`all` is a package-level slice literal. Keep it that way; don't ever build it
from a map.

**(b) Split static from dynamic.** The moment you put "today is 2026-08-06" in
the system prompt (and you should — #2), you've created a value that changes and
sits in front of everything. The fix is structural, not clever: static body in
one block with the cache breakpoint on it, dynamic facts in a *later* block.
This is the single most common caching bug, and the Anthropic docs lead with it.
The same principle kills the other silent invalidators: unsorted JSON map
iteration in a tool schema, a timestamp in a tool description, per-request IDs.

**(c) Send the breakpoints — provider-dependent.** This is where OpenRouter
complicates things. Anthropic's API takes `cache_control: {"type": "ephemeral"}`
on a content block, max 4 breakpoints, with an optional `"ttl": "1h"` (2× write
cost, same 0.1× read). OpenRouter *passes these through* for Anthropic-backed
models but the mechanism differs per upstream provider — some cache
automatically with no breakpoints at all, some don't cache at all.
`moonshotai/kimi-k3` is not an Anthropic model, so the Anthropic parameter shape
may be ignored entirely.

The honest sequencing: do (a) and (b) now — they're free, they're correct
regardless of provider, and they're prerequisites. Then measure with #1's
`Details.Cached` field to find out whether your provider caches at all. Only
build the breakpoint plumbing if the measurement says it'll pay.

**Model minimums, for reference** (a prefix shorter than this silently doesn't
cache, no error): Opus 5 / Fable 5 / Mythos 5 = 512 tokens; Opus 4.8 / Sonnet 5
/ Sonnet 4.6 = 1,024; Opus 4.7 = 2,048; Opus 4.6 / Haiku 4.5 = 4,096. Evie's
current one-sentence prompt is ~25 tokens — nowhere near cacheable on any model.
Doing #2 first is what makes #3 possible.

**A trick worth knowing:** pre-warming. Fire one request with `max_tokens: 0`
at startup to load tools+system into cache before the first real user message,
so the first turn doesn't eat the write penalty.

**Done when.** `Details.Cached > 0` on the second turn of a session, logged and
verified — or a decision recorded in `docs/decisions.md` that the current
provider doesn't support it, with the render-order work (a, b) done anyway.

---

## 4. Persist the transcript as append-only JSONL

**What.** Write every message to `~/.evie/sessions/<id>.jsonl` as it happens.
Reload from it on `evie --resume`.

**Why.** This is the highest-leverage structural item on the list, and it's the
one thing *every* harness surveyed converged on independently:

- **Prime Agent:** sessions are append-only JSONL at
  `~/.prime/agent/sessions/<id>.jsonl`, entries linked by `id`/`parentId` into a
  *tree* rather than a list. The daemon recovers crashed workers from the session
  log. Branching happens in-place without new files.
- **Muse Code:** "a local event log in which every model call, tool run,
  approval, and edit is appended," which they describe as making the runtime
  "replay-exact and restart-safe: after a crash, the agent can resume precisely
  where it stopped." They cite this as what enables multi-day tasks.
- **claude-code:** transcript files per session, which is what `--resume` and
  `--continue` read.

Convergent design across three independent teams is a strong signal. And the
reasons compound:

1. **Resume.** A long task survives a crash, a laptop sleep, a `Ctrl-C`.
2. **Debugging.** When evie does something dumb, you can read exactly what it
   saw. Today that information is gone the instant the process exits.
3. **Compaction needs it** (#5). Compaction replaces messages in the *sent*
   context while the full history stays on disk — that's the whole trick, and
   it requires a durable full history.
4. **Evals need it** (#9). Real transcripts are the raw material for a test set.

**How it lands.** The design decisions, roughly in order of how much they matter:

- **Append-only, one JSON object per line.** Never rewrite the file. Crash-safe
  by construction: a torn last line is detectable and discardable, and
  everything before it is intact. This is why all three harnesses chose JSONL
  over a single JSON document.
- **Entry envelope, not raw messages.** Each line wants `type`, `id`,
  `parentId`, `timestamp`, and a payload. Storing bare `openrouter.Message`
  values couples your on-disk format to a provider's wire format — a bad trade
  when swapping providers is already on the roadmap (see BACKLOG "self-hosted
  model"). The envelope is the seam.
- **`parentId` even if you don't branch yet.** It costs one field now and is
  painful to retrofit. Prime Agent uses it for `/tree` navigation — jumping back
  to an earlier point and taking a different path. You may never build that UI,
  but the field is cheap insurance.
- **Where does the writing happen?** `Session` gains a writer. Not in `Events` —
  persistence isn't rendering, and a frontend must not be able to skip it by
  implementing `Events` badly. Note the `Events` interface currently gets
  `AssistantDone`/`ToolResult` at exactly the points you'd want to persist,
  which suggests the write belongs right next to those calls in the loop.
- **Versioning from day one.** A `version` field in the header entry. Prime
  Agent is on v3 and migrates v1 and v2 on load. You will change this format.

**Done when.** `evie` writes a session file, `evie --resume` (or a `sessions`
subcommand) reloads the last one and continues the conversation with full
history, and killing the process mid-turn loses at most the in-flight message.

---

## 5. Compaction

**What.** When context approaches the window limit, summarize the old portion
and send the summary in place of it.

**Why.** Today `s.messages` grows until the provider rejects the request. That's
a hard ceiling on task length, and it arrives without warning mid-task. Every
long-horizon result in the surveyed systems — Prime Agent's Factorio run, Muse
Code's multi-day tasks — depends on some form of this.

**How it lands.** Anthropic has server-side compaction
(`compact_20260112`, beta `compact-2026-01-12`, min trigger 50k tokens, returns
a `compaction` block you pass back). Not available through OpenRouter, so build
it client-side. Prime Agent's algorithm is the one to copy — it's simple,
well-documented, and provider-agnostic:

1. **Trigger:** `contextTokens > contextWindow - reserveTokens`, with
   `reserveTokens` default 16,384 — room for the response. Plus a manual
   `/compact [instructions]`.
2. **Find the cut point:** walk backwards from the newest message accumulating
   token estimates until you've kept `keepRecentTokens` (default 20,000).
3. **Cut only at legal boundaries:** user messages, assistant messages, bash
   executions. **Never cut at a tool result** — it must stay attached to its
   tool call, or you send the model an orphaned result and it gets confused.
   This is the constraint that bites; write the test first.
4. **Summarize the span** with an LLM call, in a structured format, passing the
   *previous* summary as context so repeated compactions stay coherent.
5. **Append a compaction entry** holding the summary plus `firstKeptEntryId`,
   and reload: what the model now sees is `system + summary + messages from
   firstKeptEntryId`.

The full history stays on disk (#4) — you're changing what's *sent*, not what
happened.

**The hard case, worth reading before you start:** a *split turn*. One turn
whose tool calls alone exceed `keepRecentTokens`. The cut lands mid-turn at an
assistant message, so there are no complete turns before it to summarize.
Prime Agent handles this by generating two summaries — one for prior history,
one for the early part of the split turn — and merging them. Design for it
early; a single 200-file bash output will produce it.

**Cheaper thing to do first:** tool-result clearing. Anthropic's
`clear_tool_uses_20250919` drops old tool results (keeping the last 3 by
default) and leaves a placeholder so the model knows something was removed. No
LLM call, no summary, much less code — and for a tool-heavy loop like evie's
it recovers most of the tokens. This is the 80/20; consider shipping it before
full compaction. If you do, note the caching interaction: clearing content
invalidates the cache from that point, which is exactly why Anthropic's API has
a `clear_at_least` knob — don't burn a cache rewrite to reclaim 200 tokens.

**Done when.** A session can run past the context window without erroring, and
`/compact` works manually. Test with a deliberately huge bash output.

---

## 6. Context accounting — a `/context` view

**What.** Attribute the current context window to its components: system prompt,
tool schemas, each message, tool results.

**Why.** Two reasons. First, when you're deciding what to cut, you need to know
what's big — and the answer is routinely surprising (16 tool schemas is probably
several thousand tokens you've never counted). Second, `claude-code` ships this
as a user-facing `/context` command precisely because *the human* tuning the
harness needs it.

**How it lands.** A function that walks the current request and returns a
per-component token breakdown, plus a REPL command that renders it. The
measurement question: with Anthropic you'd use the `count_tokens` endpoint
(never tiktoken — wrong tokenizer, wrong numbers). Through OpenRouter you likely
don't have that, so a heuristic (chars/4) is the pragmatic answer — but label it
as an estimate in the output so you don't later trust it as truth. #1's real
`usage` numbers give you a calibration point to check the heuristic against.

**Done when.** `/context` in the REPL prints a breakdown that sums to
approximately the `prompt_tokens` the provider reported on the last turn.

---

## 7. Tool surface work

**What.** Treat the 16 tool schemas as a designed interface rather than an
accumulated one.

**Why.** Anthropic's SWE-bench team reported spending *more* time optimizing
tool ergonomics than optimizing prompts. The framing that stuck: invest in the
agent-computer interface the way you'd invest in a human one. Concretely, from
`writing-tools-for-agents`:

- **Consolidate.** Prefer one `schedule_event` over `list_users` + `list_events`
  + `create_event`. Evie's `cron_add`/`cron_list`/`cron_remove` and
  `todo_list`/`todo_add` are worth a look — though note these mirror the CLI
  surface deliberately, which is a real countervailing reason. Judgment call,
  not an obvious win.
- **Namespace consistently.** Evie mostly does this (`finance_*`, `cron_*`) but
  not uniformly — `get_time` vs. `query_db` vs. `read_file`. Anthropic notes
  naming has "non-trivial effects on tool-use evaluations."
- **Errors as instructions.** A tool error is a prompt. `"invalid input"`
  teaches nothing; `"category must be one of: groceries, rent, ... (got
  'food')"` gets the retry right. This is the single cheapest quality win on
  this list — go read the error paths in `internal/tools/*.go` and rewrite the
  bad ones. An afternoon, no design required.
- **Token-efficient responses.** Pagination, truncation, and filtering with
  sensible defaults. Return semantic names, not UUIDs. Consider a
  `response_format: "concise" | "detailed"` enum so the model can choose
  verbosity — cheap to add, and it moves the decision to the party that knows
  the task.

**The bigger structural item — deferred tool loading.** Already in BACKLOG under
"dynamic tool loading" with a ~20-tool trigger; evie is at 16. The mechanism
worth knowing, from `claude-code`'s `ToolSearchTool`: schemas are **appended**
when fetched rather than swapped in place. That's not incidental — appending
preserves the cached prefix, whereas swapping the tool list invalidates *all
three* cache levels (tools sit first in render order). If you build deferred
loading, build it as append-only for exactly this reason. Note this puts #7 and
#3 in tension if done naively, and in harmony if done right.

**Done when.** Error messages across `internal/tools/` are instructional, naming
is consistent, and a decision on consolidation is recorded either way.

---

## 8. Memory

**What.** Facts that survive across sessions.

**Why.** Evie's whole premise is *personal* assistant. An assistant that
relearns your budget categories, your preferences, and your project state every
session isn't one. This is also where "context engineering" stops being about
cost and starts being about capability: the recommendation from Anthropic's
context-engineering post is **structured note-taking** — let the agent write
notes outside the context window and retrieve them later, which is external
memory that doesn't consume the window.

**How it lands.** Ordered by cost, and the first is genuinely enough for a while:

1. **A file the model can read and write.** `~/.evie/memory.md`, loaded into the
   system prompt at session start (in the *dynamic* block — #3), edited via the
   existing `edit_file` tool. Zero new tools. Enough for months.
2. **Structured memory with retrieval.** Files per fact with descriptions,
   loaded on relevance rather than wholesale. Matters once memory exceeds what
   you'd want to paste into every request.
3. **The compelling version — a self-editing harness.** Prime Agent's *continual
   harness*: harness state is `H = (ρ, G, K, M)` — supplemental prompt notes,
   sub-agents, skills, memories — with unified CRUD the model calls itself
   (`create_prompt_note()`, `create_memory()`, `create_skill()`,
   `update_*`, `delete_*`, `list(kind)`). Then a `/refine` command reads the
   session's trajectory and applies minimal evidence-backed edits to that state.
   The base system prompt stays **immutable**; only supplemental state is
   writable, with snapshots for rollback. That immutability boundary is the
   design insight worth stealing — self-modification without the failure mode of
   the agent corrupting its own foundation.

**Note on interaction with #5:** Anthropic's docs specifically recommend pairing
memory with context clearing, so the agent can save what matters *before* the
clearing happens. Ordering matters — a memory write triggered after compaction
has already dropped the content is too late.

**Done when.** Evie remembers something across a restart that you only told it
once. Start with option 1 and don't over-build.

---

## 9. Evals

**What.** A fixed set of tasks, re-run whenever the prompt, model, or tool
surface changes.

**Why.** Every item above changes behavior, and right now the only way you'd
know whether a change helped is that it felt better in one conversation. That's
not a signal. Anthropic's tool-writing post is blunt about the method that
actually works: build evals from real tasks, then use Claude itself to analyze
the transcripts and propose refinements — and they report measurable accuracy
gains on their internal Slack and Asana tools from exactly that loop.

**How it lands.** This one is deliberately unglamorous and should stay small:

- **10–20 tasks, from your actual usage.** "How am I tracking against budget?"
  "Add a cron job to sync finances nightly." "What did I spend on groceries last
  month?" The BACKLOG's unexercised budget-analysis path is a natural first
  case.
- **A pass criterion per task** — often "called the right tools in a reasonable
  order," not "produced exact text." Exact-match scoring on prose will drive you
  toward the wrong optimizations.
- **Run it as a Go test** with a `-tags eval` build tag so it doesn't run on
  every `go test ./...`. It costs money and takes time; it should be deliberate.
- **Depends on #4.** Real transcripts are where the task set comes from.

**Done when.** `go test -tags eval ./...` runs the set and reports pass/fail per
task, and you've used it once to decide between two versions of the system
prompt.

---

## 10. Long-running autonomy

**What.** Let evie keep working when you're not watching.

**Why.** This is the frontier item — least urgent, most interesting, and evie is
unusually well-positioned because **cron already exists**. The three surveyed
systems all landed on roughly the same primitives:

- **Goals** — a persistent objective that outlives a single turn, with an
  optional token budget. Prime Agent's `/goal`; Muse Code's `/goal`.
- **Heartbeats** — cron-style scheduled prompts that re-enter the loop.
  Evie's `cron_add` is already 80% of this mechanism; what's missing is
  cron firing a *prompt* into a session rather than a shell command.
- **Autonomous mode with bounds and a gate.** Prime Agent's shape:
  `--autonomous --autonomous-gate "npm run check" --autonomous-max-turns 20`,
  plus `--autonomous-max-tokens` and `--autonomous-timeout-ms`. The **gate** is
  the load-bearing part: a command that must pass for the agent to consider
  itself done. For evie that's `go build ./... && go test ./...`. Bounds without
  a gate is just a runaway loop with a timer.

**Also worth knowing about, though probably not for evie:** both Prime Agent and
Muse Code keep **persistent background sub-agents** rather than spawning one per
task, on the grounds that a long-lived sub-agent doesn't re-gather the same
context every time. And Prime Agent's daemon architecture — a background process
owning live sessions over a local socket, so closing the terminal detaches the
client without stopping the work — is the piece that makes multi-day tasks
practical. Evie's `Session` + `Events` split is already the right shape for
this: the daemon would own `Session`, and REPL/web become clients. Not a
rewrite. Just not this month.

**Done when.** Nothing here is committed. Revisit after #4 and #5, which are
prerequisites for any of it working.

---

## Deliberately not doing

Worth recording so these don't get re-litigated:

- **Programmatic tool calling via a persistent REPL kernel.** Prime Agent's
  central bet: a persistent IPython kernel is the *only* tool interface, and the
  model writes Python that calls tools as functions — so intermediate results
  live in kernel variables instead of the context window. Genuinely clever, and
  the wrong shape for evie. It requires a Jupyter/ZeroMQ transport, kernel
  lifecycle management, a Python runtime dependency, and namespace snapshotting
  for revival. Evie's `bash` tool with a persistent cwd already captures a slice
  of the same idea at a fraction of the cost.
- **Sub-agents.** Already in BACKLOG as "design session when long tasks clog one
  context." Compaction (#5) addresses the same pressure for less complexity.
  Do #5 first, then reassess whether the need is still there.
- **Multi-agent messaging.** Prime Agent restricts agent-to-agent messaging to
  the "nuclear family" (parent, sibling, child) to prevent cross-session chatter
  — a nice constraint, and irrelevant until sub-agents exist.

---

## Sources

**Anthropic documentation** (fetched 2026-08-06):
- Prompt caching — `platform.claude.com/docs/en/build-with-claude/prompt-caching`
  — render order, breakpoint placement, per-model minimums, TTL pricing,
  invalidation table, pre-warming with `max_tokens: 0`
- Context editing — `.../context-editing` — `clear_tool_uses_20250919`,
  `clear_thinking_20251015`, `trigger`/`keep`/`clear_at_least`/`exclude_tools`,
  cache interaction
- Compaction — `.../compaction` — `compact_20260112`, trigger minimums,
  `pause_after_compaction`, per-iteration usage accounting

**Anthropic engineering posts:**
- *Effective context engineering for AI agents* — "the smallest possible set of
  high-signal tokens that maximize the likelihood of some desired outcome";
  system prompt sectioning, just-in-time retrieval, structured note-taking,
  sub-agents returning 1–2k-token summaries
- *Writing tools for agents* — consolidation, namespacing, token efficiency,
  response-format enums, instructional errors, eval-driven iteration
- *Building effective agents* — workflow vs. agent, ACI investment, simplicity
  and transparency; the SWE-bench "more time on tools than prompts" datapoint

**`claude-code` source** (local, `~/code/claude-code`):
- `src/services/api/claude.ts` — `getCacheControl({querySource})`,
  `should1hCacheTTL`, "exactly one message-level cache_control marker per
  request", `ephemeral_1h`/`ephemeral_5m` accounting
- `src/services/api/promptCacheBreakDetection.ts` — per-request hashing of
  system blocks *and each tool schema* to attribute which component broke the
  cache; writes a `cache-break-*.diff` for inspection. Overkill for evie, but
  the idea — make cache breaks *diagnosable* rather than mysterious — is the
  transferable part
- `ToolSearchTool` — deferred tool schemas, appended not swapped
- `src/utils/tokenBudget.ts`, `analyzeContext.ts` — the `/context` view

**Prime Agent** (Prime Intellect, open source, TypeScript,
`github.com/PrimeIntellect-ai/prime-agent`, ~3.3k stars) — the most directly
useful source here, because it's a complete harness you can read:
- `packages/coding-agent/docs/compaction.md` — the compaction algorithm, cut
  point rules, split-turn handling, `CompactionEntry` shape
- `.../session-format.md` — JSONL entry types, `id`/`parentId` tree, version
  migration
- `.../architecture.md`, `.../rlm-runtime.md` — daemon/worker/session split,
  IPython host-request bridge
- `.../long-running-agents.md` — goals, heartbeats, autonomous mode, agent
  messaging
- Blog: `primeintellect.ai/blog/prime-agent` — the RLM and continual-harness
  framing, `/refine`, `H = (ρ, G, K, M)`

**Meta Muse Code / Muse Spark 1.2** (announced 2026-08-05,
`research.meta.ai/blog/introducing-muse-code-and-muse-spark-1-2`) — beta, and
**neither the harness nor the weights are open-sourced**, so this is
architecture-by-description only. The transferable ideas: an append-only event
log covering every model call, tool run, approval, and edit, making the runtime
"replay-exact and restart-safe"; persistent background sub-agents rather than
per-task spawning; bundled `/plan`, `/grill` (stress-test a plan), and `/goal`
skills. The `/grill` idea — an adversarial pass over a plan before executing it
— is cheap to adopt and pairs well with the repo's existing spec-review habit.
