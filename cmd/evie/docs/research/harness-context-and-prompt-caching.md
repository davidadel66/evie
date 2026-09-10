# Harness context construction and prompt caching

**Research date:** 2026-09-05

**Authority:** official Anthropic, Claude Code, LangGraph, and Letta documentation.
This note explains documented patterns; it is not an approved implementation
specification. Product documentation describes behavior, not every internal
implementation detail.

## Main finding

Appending messages is a normal foundation for an agent harness. Claude Code
explicitly documents resending context and appending new exchanges. Retrieval
and compaction complement this: retrieval decides what additional information
enters the conversation; compaction replaces accumulated history when a smaller
working context is needed. These are separate choices from whether new messages
are appended ([Claude Code caching](https://code.claude.com/docs/en/prompt-caching),
[Anthropic context engineering](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)).

## Documented examples

### Claude Code: stable startup context, on-demand reads, occasional compaction

Claude Code orders requests as system instructions/tools, project context, then
conversation. Root and user CLAUDE.md contents stay at their session-start
versions. Reading a changed file appends a new result; an old read is not rewritten.
Compaction replaces conversation history with a summary, reuses the system
layer, and reloads project context. The summary request itself appends a
summarization instruction to existing context, allowing cache reuse
([caching behavior](https://code.claude.com/docs/en/prompt-caching)).

Its memory has a small startup index and larger files available on demand.
The first 200 lines or 25KB of MEMORY.md, whichever comes first, load at startup.
Topic files are read with ordinary file tools when needed. This is a concrete
example of keeping discoverability in context without loading every stored fact
([memory documentation](https://code.claude.com/docs/en/memory)).

Anthropic describes Claude Code as combining upfront project instructions with
file discovery through tools such as glob and grep. It also describes persistent
notes and compaction for long tasks. Retrieval has a tradeoff: exploration takes
extra calls and can pursue unhelpful paths, whereas automatic upfront retrieval
can surface useful information sooner
([context engineering](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)).

### LangGraph: the application chooses the context policy

LangGraph is a framework, so its examples are not one universal harness policy.
Its memory guide demonstrates searching a user-scoped store using the latest
message, joining retrieved facts into a system message, then prepending that
message to stored conversation history. The same guide offers message trimming,
deletion, and summarization strategies
([memory guide](https://docs.langchain.com/oss/python/langgraph/add-memory)).

Inference: this example can preserve the message history while changing an early
prefix whenever retrieval results change. That is a real illustration of why
an append-only history does not guarantee an unchanged full request.

### Letta: a small always-visible memory area plus files read on demand

Letta's current MemFS documentation describes memory as a git-backed filesystem.
Files under `system/` load into the system prompt each turn; other files remain
outside context until needed. A directory tree stays in the prompt so the agent
can discover those files. Default retrieval uses ordinary file search and reads,
without a semantic or vector index; an optional search mod adds those modes
([MemFS](https://docs.letta.com/concepts/memfs)).

Inference: this separates always-needed memory from deeper references. Updating
the system files or directory tree can change early context; precise cache
effects still depend on request rendering and provider behavior.

## Evie in the current working tree

These observations include existing uncommitted work in the checkout. They
describe code paths, not a verified running process or its environment settings.

Before every provider iteration, including iterations after tool execution, Evie
loads durable events, its compaction chain, working context, and tool schemas
([turn.go](../../../../internal/agent/turn.go#L159)). Its model message order is:

```text
[stable system instructions]
[optional conversation summary, system role]
[optional working context, user role]
[projected conversation history]
```

That order comes from
[context.go](../../../../internal/agent/context.go#L201). Tool schemas are fixed
for the session by the registry
([registry.go](../../../../internal/tools/registry.go#L34)).

The name `WorkingContext` could suggest general memory retrieval, but its current
database implementation delegates to the focused-task context renderer
([history.go](../../../../internal/eviedb/history.go#L44),
[tasks_scope.go](../../../../internal/eviedb/tasks_scope.go#L296)). It emits task
information such as title, status, and revision for open, in-progress, and
blocked task nodes, bounded to 64 nodes and 16KiB. It does not perform semantic
memory retrieval.

Memory reads are available through the memory plugin when the remote-memory
capability is enabled with `EVIE_REMOTE_MEMORY=on`
([capability gating](../../../../internal/plugins/memory.go#L110)). Available
operations include exact claim queries, alias lookup, and bounded one- or
two-hop traversal
([queries](../../../../internal/plugins/memory.go#L462),
[lookup](../../../../internal/plugins/memory.go#L509),
[traversal](../../../../internal/plugins/memory.go#L531)). When called, their
results enter tool history. The Stage 3 specification explicitly leaves
full-text/vector search, relevance ranking, and context injection to Stage 5
([Stage 3 specification](../active/semantic-memory-stage-3.spec.md#L426)).

Under context pressure, Evie can shorten older tool results
([context.go](../../../../internal/agent/context.go#L228)) and automatically
compact at an 80% threshold toward a 60% target. This budget uses conservative
serialized-byte estimation rather than the provider's measured input tokens
([automatic_compaction.go](../../../../internal/agent/automatic_compaction.go#L17)).

Inference: a task status or revision change can alter the working-context block
before unchanged conversation history, reducing prefix reuse on the next call.
Memory tool retrieval itself can append evidence without that rewrite. Neither
observation establishes the actual cache-hit rate; that needs provider-reported
usage from requests.

## Design implications

These are design inferences, not implementation recommendations or authorization:

- Separate persisted history, the selected model context, and provider cache
  reuse. Having a fact in a database does not mean the model has seen it.
- Small stable context can orient the model; tools or automatic retrieval can
  supply detailed evidence for the current question.
- Appending new retrieved evidence can preserve an existing prefix. Replacing
  an early retrieval block can reduce prefix reuse while keeping context smaller
  and more current. Measure relevance and cost rather than assuming either wins.
- Compaction intentionally trades history detail and immediate prefix reuse for
  a smaller working context. Cache hits alone are not an answer-quality metric.

Even tool availability can be updated without rewriting the prefix when the
provider supports deferred definitions: Anthropic documents placing discovered
tool references in conversation history. Ordinary changes to upfront tool
definitions invalidate the corresponding prefix
([tool caching](https://platform.claude.com/docs/en/agents-and-tools/tool-use/tool-use-with-prompt-caching)).

## Follow-up: three iteration principles (2026-09-06)

This is a targeted update from primary lab sources, not an exhaustive survey.
The principles below are design inferences for future evaluation after Evie's
approved memory work, not additional implementation scope.

Local clarification: a fresh Evie session has no summary. A successful manual
`/compact` requires at least three completed remaining turns and preserves the
newest two ([manual compaction](../../../../internal/agent/compaction.go#L264));
automatic compaction uses the pressure thresholds described above. Accepted
summaries are persisted and reconstructed for subsequent calls; raw conversation
history is not rewritten. Individual tool results are capped at 100KiB and a
tool group at 128KiB
([tool result bounds](../../../../internal/agent/tool_results.go#L13)). Context
projection can shorten older results under pressure while protecting the latest
three groups after the group cap
([context projection](../../../../internal/agent/context.go#L228)).

Initial assessment: keep the focused-task snapshot for correctness and recovery;
measure its size and change frequency before changing its placement or replacing
it with incremental updates. This is a design judgment, not a finding that the
current placement minimizes cache costs.

1. **Retrieve narrowly and filter before information enters context.** The
   on-demand reads described above remain compatible with append-only history.
   Keep a small orientation block, then fetch evidence needed for the question.
   OpenAI's August 13, 2026 builder guide specifically recommends processing,
   filtering, and aggregating tool results in code outside the model's context
   ([guide](https://openai.com/index/builders-guide-to-gpt-5-6/)). For Evie, the
   inference is to test bounded evidence with source references and a way to
   expand it; avoid making the model inspect a large raw result just to discard
   most of it. This principle does not require adopting a new execution runtime.

2. **Treat output clearing and summarization as different controls.** Anthropic's
   current documentation distinguishes removing selected older tool results from
   replacing history with a generated summary. It offers controls to retain
   recent results, exclude important tools, and clear enough at once to justify
   breaking the prompt cache
   ([context editing, accessed September 6, 2026](https://platform.claude.com/docs/en/build-with-claude/context-editing)).
   For Evie, the inference is to preserve compact conclusions and source handles
   while allowing bulky, reproducible results to leave the working context.
   Summaries still need to retain unresolved work and relevant decisions.

3. **Choose policies using a small repeatable evaluation.** Anthropic's
   January 9, 2026 evaluation guide recommends starting with 20–50 cases from
   real failures or manual checks, inspecting actual outcomes, and tracking
   cost and latency alongside quality
   ([guide](https://www.anthropic.com/engineering/demystifying-evals-for-ai-agents)).
   For Evie, test missing evidence, irrelevant memories, corrected facts, and
   continuation after compaction. Compare one policy change at a time using
   answer correctness, supporting evidence found, and provider-reported token
   usage. A smaller prompt or higher cache-hit rate is useful only if the agent
   still answers and acts correctly.
