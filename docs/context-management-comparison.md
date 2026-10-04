# Context management — Evie vs current harnesses

Status: comparison for planning (2026-10-03). In-turn compaction, tighter
tool output and the memory flush before compaction (memory architecture
Stages 8, 9 and 10) are implemented; see their 2026-10-03 entries in
`cmd/evie/docs/active/memory.decisions.md`. Explore workers (Stage 7) are
implemented too; see `cmd/evie/docs/active/subagents.decisions.md`.

## Techniques the leading harnesses use

- **Orchestrator–worker delegation.** Workers run in clean context windows
  and return "a condensed, distilled summary of its work (often 1,000–2,000
  tokens)", so the orchestrator stays focused on synthesis [1]. Claude Code
  ships Explore, Plan and general-purpose sub-agents; only the final result
  reaches the main conversation [2].
- **Tool-result clearing.** Old tool results are replaced with placeholders.
  This is "the safest form of aggressive pruning" [1]. The Claude API's
  `clear_tool_uses` triggers at 100k input tokens by default, keeps the last
  3 tool uses, and supports `exclude_tools` and `clear_at_least` (so a clear
  is worth the cache invalidation it causes) [3]. Research finds simple
  observation masking about as efficient as LLM summarization [6].
- **Compaction, including mid-task.** OpenAI's server-side compaction fires
  during a response when context crosses `compact_threshold`. Prior user
  messages stay verbatim; assistant turns, tool calls and results become one
  compaction item [4]. OpenHands chains condensers: drop browser noise, then
  mask observations, then summarize [5].
- **Note-taking before forgetting.** Claude is warned as context nears the
  clearing threshold so it can save important information to memory first
  [3]. Agents also keep structured notes outside the window [1].
- **Just-in-time context.** Keep lightweight identifiers and load data with
  tools when needed instead of preloading [1].

## Where Evie stands

| Technique | Evie today | Gap |
|---|---|---|
| Delegation | Research workers only (`web_search`, `web_fetch`), with budgets, wrap-up, pageable reports and continuation | **Large.** Local work (reading files, querying databases, long bash investigations) can't be delegated, so its output floods the orchestrator. |
| Tool-result clearing | Above 60% of the budget, older groups shrink to 1 KiB head/tail placeholders in bands, keeping the newest 3; `read_tool_result` re-reads them. Since Stage 9, a request that offers `read_tool_result` shows at most 32 KiB per result and 96 KiB per group, with the omitted range named for paging. Memory, Task, delegation and reread results are excluded from clearing. A clear that frees under 8 KiB while the request fits is skipped. | Small. Workers and older presets keep the 100 KiB per-result bound, and the 1 KiB projection is not frame-aware for very long delimiter lines. |
| Compaction | Rolling summary of whole completed turns, carry-forward sections, calibrated budget, retry when the provider rejects for context length; since Stage 8, the active turn's oldest completed tool groups are summarized when a request cannot fit | Small. Retained tool-call arguments are never shortened. |
| Save before forgetting | Since Stage 10, every committed compaction (whole-turn, in-turn or manual) owes the background memory pass its summarized owner messages; the pass runs them as soon as the conversation is free, and a lost request is recovered from the committed compaction | Small. The flush runs just *after* the compaction commits (the events stay in SQLite), never inside the live turn; an automatic flush waits up to a minute after the turn ends. |
| Structured notes | Task Trees and Task Focus; summary sections carried forward | Covered. |
| Just-in-time | Memory tools, conversation and episode search, `read_tool_result`, `read_subagent_report` | Covered. |
| Cache stability | Stable-first ordering, projection bands, Anthropic breakpoints | Covered. |

## Recommended improvements, in order

1. **General delegation.** Add worker presets beyond research:
   - a read-only **explore** worker (`read_file`, read-only `query_db`,
     repository search) for "go read and summarize";
   - optionally a **task** worker for long tool-heavy jobs.

   Workers return the existing Stage 8 result contract (summary inline, full
   report pageable). Writes and approvals stay with the orchestrator. Needs
   design decisions on which tools each worker gets; bash is ungated today, so
   giving it to workers needs care.
2. **In-turn compaction.** Allow compacting completed tool groups inside the
   active turn when clearing isn't enough. Keep the user message verbatim and
   summarize the older tool groups, like OpenAI's server-side compaction.
3. **Tighter tool output.** *Implemented (Stage 9).*
   - Lower the per-result admission (100 KiB today) and rely on files plus
     paging for large outputs.
   - Add `exclude_tools`-style protection so memory, todo and task results are
     never cleared.
4. **Memory flush before compaction.** Trigger memory Stage 4's background pass
   just *before* a compaction, so what's about to be summarized is offered as
   memory first. Implemented (Stage 10) as a flush owed by every committed
   compaction and run once the conversation is free, since the pass may not
   commit during a live turn and must not block it.

## Sources

1. Anthropic, Effective context engineering for AI agents — https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents
2. Claude Code sub-agents — https://code.claude.com/docs/en/sub-agents
3. Claude API context editing — https://platform.claude.com/docs/en/build-with-claude/context-editing
4. OpenAI compaction — https://developers.openai.com/api/docs/guides/compaction
5. OpenHands condenser — https://docs.openhands.dev/sdk/guides/context-condenser
6. The Complexity Trap: observation masking vs LLM summarization — https://arxiv.org/pdf/2508.21433
