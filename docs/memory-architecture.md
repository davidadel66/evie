# Memory architecture — research and proposed plan

Status: research complete, plan awaiting David's decisions (2026-10-02).

Trigger: "keep answers concise" was saved as a memory but never reached the
model, because Evie's only path from memory into a request is relevance-based
recall, and no request shares words with a style preference.

## What the research says

### 1. Standing context is always loaded, never retrieved

Every system that handles preferences well keeps a small, bounded tier in
every request and selects it by rule, not relevance.

| System | Always in context | Retrieved on demand |
|---|---|---|
| MemGPT / Letta | Core memory: a fixed-size "working context" for "key facts, preferences" pinned to the system prompt [1][9] | Recall and archival storage, searched via tools |
| MemoryOS | User Profile, Agent Profile and User Traits "all utilized" [2] | User knowledge base, top-10 by relevance |
| Claude Code | CLAUDE.md files (scope-ordered) and the first 200 lines / 25 KB of MEMORY.md [3] | Topic memory files, read when needed |
| ChatGPT | Saved memories and a summary of ~15 recent conversations, plus custom instructions [5] (reverse-engineered) | Chat-history reference |
| Claude.ai | `<userMemories>`: "distilled, stable facts about the user" [6] (reverse-engineered) | `conversation_search`, `recent_chats` tools, called selectively |
| Cursor | Rules with `alwaysApply: true`; user, project and team scopes [7] | Rules attached by file glob, by description, or by @-mention |
| LangMem | Profiles: "a single document that represents the current state", read by key [8] | Collections, by semantic search plus importance and recency |

CoALA treats the retrieval method as a per-memory-type choice ("rule-based,
sparse, or dense") [10]. Rule-based always-include is the standard answer for
preferences and identity.

**Evie today:** no such tier. Repository instructions are always loaded, but
memory claims only arrive via lexical/dense recall. That is the gap.

### 2. Scopes combine deterministically

Claude Code concatenates policy, user, project and local instructions from
broadest to most specific, so specific ones come later and win by position [3].
Cursor orders team, then project, then user [7]. This maps directly onto
Evie's Global, Workspace and session scopes.

### 3. How memories get written

- **Hot path, explicit:** the user says "remember X" (every system).
- **Hot path, model-proposed:** the Claude API memory tool lets the model
  issue file commands that the application executes [4]. ChatGPT proposes and
  the user confirms in conversation [5].
- **Background:** Letta sleep-time agents consolidate "without interrupting
  your active work", triggered after N steps or at compaction [9]. LangMem's
  "subconscious" formation runs after a conversation goes idle [8]. Claude.ai
  "periodically" updates memories in the background [6].
- **Update operations:** Mem0 extracts candidate facts (using a conversation
  summary plus the 10 most recent messages), retrieves the 10 most similar
  existing memories, and has the LLM choose ADD / UPDATE / DELETE / NOOP. Its
  graph variant marks conflicting relations invalid instead of deleting them
  "to enable temporal reasoning" [11].

### 4. Contradictions: invalidate, don't delete

Zep/Graphiti stores four timestamps per fact (created/expired, valid/invalid).
When facts conflict, it sets the old fact's invalid time and keeps it [12].
Evie's supersession and valid-time model already does this; keep it.

### 5. Retrieval quality (LongMemEval [13])

- Commercial assistants lost 30–64% accuracy versus reading the full history.
- Indexing at round granularity beats session granularity.
- Fact-augmented keys gave +9.4% recall@k and +5.4% accuracy.
- Time-aware query expansion gave +6.8–11.3% recall on temporal questions.
- Chain-of-note reading with JSON structure gave up to +10 points.
- Mem0 on LoCoMo: 91% lower p95 latency than full context (1.44 s vs ~17 s)
  and over 90% fewer tokens, for a few points of judge accuracy [11].

### What not to adopt

- A-MEM's "memory evolution" rewrites existing memories in place [14]. This
  conflicts with Evie's provenance and approval model.
- Mem0's physical DELETE. Use invalidation, as Mem0's own graph variant does.
- The Claude API memory tool's pull-only design [4]. A preference arrives only
  if the model obeys "view memory first", which is a behavioral guarantee, not
  a structural one. AGENTS.md asks for boundaries enforced in code.

## Proposed architecture for Evie

Four tiers, each with a named precedent.

1. **Standing context (new).** Approved standing memories (preferences,
   instructions, core profile facts) are rendered in full on every request.
   - Order: Global, then Workspace, then session.
   - Bounded size, placed in the stable cached prefix.
   - Selected by rule, never by relevance.
   - Precedents: MemGPT core memory, MemoryOS traits, CLAUDE.md, ChatGPT saved
     memories.
2. **Relevance recall (existing, improved).** Facts and conversation evidence
   via automatic recall. Add LongMemEval's fact-augmented keys and time-aware
   query expansion, measured on `internal/memoryeval`.
3. **On-demand tools (existing).** `memory_search`, `memory_query_claims` and
   the others, like Claude's `conversation_search`. Add one tool that lists the
   standing set and the owner's profile, so "what do you remember about me" is
   one call.
4. **Background consolidation (existing compiler, promoted).**
   - After a session goes idle or at compaction (Letta, LangMem), extract
     candidates, compare them with similar existing claims, and propose
     ADD / UPDATE / INVALIDATE / NOOP (Mem0).
   - Proposals land in the existing review inbox; nothing is accepted without
     approval.

Unchanged: approval-gated writes, provenance and source binding, scope rules,
supersession by invalidation.

## Plan (stages, each one reviewable change)

1. **Standing tier.**
   - Data: standing kind (by type, with an override), Global → Workspace →
     session order, hard budget with "memory full".
   - Rendering: every request, in the cached prefix, labelled as the owner's
     standing context.
   - Eval: a standing preference reaches every request, including ones that
     share no words with it.
   - Migrate David's two existing preferences.
2. **Profile and Workspace memory responsibilities.**
   - The system prompt states both duties.
   - Proactive proposals route to the review inbox; an explicit request still
     shows the approval card.
   - The card and memory UI show "Always applies" with the toggle; the Workspace
     brief renders in its Workspace.
3. **"What do you know about me / this project."** One read tool returning the
   standing set plus profile or Workspace facts.
4. **Always-on background pass.** Upgrade the memory compiler as in decision
   4, with the DeepSeek Flash default and an eval on the scale corpus for
   proposal precision.
5. **Episodic session summaries.** One stored summary per session, searchable
   on demand.
6. **Recall quality.** Fact-augmented keys and time-aware query expansion,
   gated on `internal/memoryeval` gains.

Context-management stages (after the memory stages; rationale and sources
in `docs/context-management-comparison.md`):

7. **General delegation.** Worker presets beyond web research: a read-only
   explore worker, optionally a task worker for long tool-heavy jobs.
8. **In-turn compaction.** Compact completed tool groups inside the active
   turn when clearing isn't enough.
9. **Tighter tool output.** Lower per-result admission and protect memory,
   todo and task results from clearing.
10. **Memory flush before compaction.** Trigger the Stage 4 background pass
    just before a compaction.

Order: 1 and 5 in parallel; then 2 and 3; then 4; then 6; then 7–10 (7 first;
8 and 9 in parallel; 10 once Stage 4 has landed).

### Acceptance criteria

**1. Standing tier**
- Claims carry a deterministic kind: preference, instruction, profile fact,
  Workspace brief, or other fact. The model proposes it, the card shows it,
  and validation enforces it. Standing defaults to true for preference,
  instruction, profile fact and Workspace brief, and David can toggle it
  before approving.
- Every request renders active, approved, standing claims visible in the
  current scope, in Global → Workspace → session order, as a labelled data
  block in the stable prefix.
  - It changes only when the standing set changes, so cache hits survive.
  - It is never subject to relevance gating.
  - Retired, superseded and out-of-scope claims never render.
- Budgets: about 500 tokens for the profile (Global plus session) and about
  500 per Workspace brief, using the calibrated estimator. Approving a claim
  past the budget fails with "memory full", naming the current items and
  their sizes. Nothing renders truncated.
- A test proves a standing preference reaches a request that shares no words
  with it, in a new session, in another Workspace (for Global), and after
  compaction.
- David's two existing General-workspace preferences can be marked standing
  through the normal approval path; nothing is rewritten silently.
- Spec and decisions are amended, and old sessions' receipts still resume.

**5. Episodic session summaries**
- When a session is compacted, or goes idle after N turns, its summary is
  stored as one episode per session, with scope, time range and source event
  IDs.
- Episodes are searchable on demand through a read-only memory tool, under
  the same scope and egress rules as conversation search. They are never
  standing.
- Provenance points back to the session's events. The summary text is framed
  as data.
- No model call happens just to create an episode: it reuses the existing
  compaction summary or the background pass's output.

**7. General delegation**
- The orchestrator can delegate local, read-heavy work (reading many files,
  querying databases, investigating a repository) to an explore worker with a
  clean context.
- The worker returns the Stage 8 sub-agent result contract: summary inline,
  full report through `read_subagent_report`, `continue_research`-style
  continuation.
- Workers get only read-only capabilities. Writes, approvals and memory writes
  stay with the orchestrator.
- The same budgets, wrap-up, per-turn cap, framing and authority checks apply
  as for research workers.
- Prompt guidance: delegate when a task needs many tool calls or large outputs
  the orchestrator doesn't need to see.
- **Decided (David, 2026-10-03):** the explore worker gets `read_file`,
  read-only `query_db`, and a bash restricted to vetted read-only commands
  (for example `ls`, `rg`, `cat`, `git log`/`diff`/`show`,
  `sqlite3 -readonly`), enforced in code. The allowlist must reject
  write-capable flags and option-embedded execution (`git -c`, `--output`,
  pipes into writers, redirections).

**8. In-turn compaction**
- When tool-result clearing can't bring the active turn under budget, the
  completed tool groups of the active turn are summarized. The turn's user
  message stays verbatim.
- The cut never splits a tool call from its result.
- Persistence validators accept the new shape. Recovery and the C8 retry still
  hold.
- A test: one long agentic turn on a 128k model keeps working past the point
  where clearing alone would overflow.

**9. Tighter tool output**
- The per-result admission limit drops from 100 KiB (target set by
  measurement). Oversized outputs go to files with an excerpt plus paging,
  as bash and web_fetch already do.
- Results from memory, todo/task and sub-agent report tools are never cleared
  by projection (an `exclude_tools` equivalent).
- Cache stability is unchanged.

**10. Memory flush before compaction**
- Just before an automatic or manual compaction, the Stage 4 background pass
  runs over the turns about to be summarized.
- Its proposals land in the review inbox before the summary replaces those
  turns.
- It doesn't block the turn: it runs asynchronously over stored events, which
  remain in SQLite.

## Decisions (David, 2026-10-03)

1. **Standing by type, visible on approval.** Preferences and instructions
   about David become standing by default (the MemoryOS / LangMem profile
   rule). The approval card shows "Always applies", and David can switch it
   off before approving.
2. **Hard cap.** The standing tier has a fixed budget (about 500 tokens).
   Saving past it reports "memory full" and asks David to drop or merge first
   (ChatGPT style).
3. **Two memory responsibilities, stated in the system prompt.** The model
   proactively proposes what is worth remembering without being told
   (MemGPT, Claude Code auto-memory, ChatGPT):
   - **Profile:** David's preferences and facts about him, standing by type.
   - **Workspace memory:** the active Workspace's brief, decisions and
     conventions. The brief is standing in that Workspace; topic facts go
     to relevance recall.

   Proactive proposals go to the review inbox, not a blocking approval card.
   An explicit "remember this" still shows the card. Approval, scope and
   source binding stay enforced in code.
4. **Background pass: propose, David approves.** Upgrade the existing memory
   compiler:
   - from per-session opt-in to always-on, triggered at idle or every N turns
     across all conversations;
   - comparing candidates with similar existing claims and proposing
     add / update / invalidate / no-op (Mem0);
   - routing each proposal to profile or Workspace scope;
   - landing in the review inbox.

   It keeps reading only owner messages, so third-party text can't plant
   facts about David. Default model: `deepseek/deepseek-v4.1-flash`, pinned
   rather than a floating `-latest` alias. Optionally use its `:batch` variant
   (about 60% cheaper) because background latency doesn't matter. Local Ollama
   stays available.
5. **No recent-conversations digest.** Past conversations stay reachable on
   demand through recall and search tools (Claude.ai style).
6. **Project history.** Store one summary per session (reusing compaction
   summaries) as searchable episodic memory (Zep episode layer, LongMemEval
   session indexing). It is fetched on demand, never standing.

## Sources

1. MemGPT — https://arxiv.org/abs/2310.08560
2. MemoryOS — https://arxiv.org/abs/2506.06326
3. Claude Code memory — https://code.claude.com/docs/en/memory
4. Claude API memory tool — https://platform.claude.com/docs/en/agents-and-tools/tool-use/memory-tool
5. ChatGPT memory, reverse-engineered — https://llmrefs.com/blog/reverse-engineering-chatgpt-memory
6. Claude.ai memory, reverse-engineered — https://manthanguptaa.in/posts/claude_memory
7. Cursor rules — https://cursor.com/docs/context/rules
8. LangMem conceptual guide — https://langchain-ai.github.io/langmem/concepts/conceptual_guide/
9. Letta memory and sleep-time — https://docs.letta.com/guides/agents/memory, https://docs.letta.com/guides/agents/architectures/sleeptime
10. CoALA — https://arxiv.org/abs/2309.02427
11. Mem0 — https://arxiv.org/html/2504.19413v1
12. Zep/Graphiti — https://arxiv.org/html/2501.13956v1
13. LongMemEval — https://arxiv.org/html/2410.10813v2
14. A-MEM — https://arxiv.org/abs/2502.12110

Caveats: the ChatGPT and Claude.ai descriptions are reverse-engineered, not
official. OpenAI's own memory FAQ could not be fetched (HTTP 403). Benchmark
gains are self-reported by each paper.
