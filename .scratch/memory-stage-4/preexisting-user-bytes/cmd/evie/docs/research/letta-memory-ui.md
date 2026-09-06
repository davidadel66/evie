# Letta memory UI/UX reference for Evie Stage 3

Research snapshot: 2026-09-01. Sources are limited to first-party Letta documentation, GitHub repositories, and screenshots. The code snapshot is Letta Code `0.31.10` at commit [`c3dc1de`](https://github.com/letta-ai/letta-code/tree/c3dc1de3c131e28493a612ef7434cc7dbf2e1e1e) (2026-09-01); the documentation snapshot is [`fe934f5`](https://github.com/letta-ai/letta-docs-md/tree/fe934f500abf046bf74f426dd99991bedf08f09e) (2026-08-27).

## Summary

Letta's current memory experience is primarily a visual and textual editor for a Git-backed Markdown filesystem (MemFS), not a semantic knowledge graph. Its strongest transferable ideas are a single memory destination, progressive disclosure from overview to record to history, visible context status, and inspectable change provenance. Its files, folders, Markdown rendering, reference edges, and Git diffs are storage-model-specific and should not define Evie's SQL temporal graph UI.

Letta also retains block and archival-passage APIs from its earlier memory model. These are useful API references, but they should not be conflated with the current MemFS interface or with semantic entities and temporally versioned claims.

## What users inspect and edit today

### Desktop and web

The current Desktop **Memory** page provides graph and list views. The graph represents memory files and references between them; selecting a node opens its description and content for editing and saving. Letta's [Desktop documentation](https://docs.letta.com/platform/desktop-app) states this directly, and the first-party [memory-editing screenshot](https://docs.letta.com/images/desktop/memory-editing.png) shows:

- groups such as `SYSTEM`, `SKILLS`, `SECRETS`, `REFERENCE`, and project/user-specific groups;
- node colors for core, external, and skill files, node size based on file length, and edges labeled as references;
- a selected file's description and Markdown body;
- history and document controls plus **Reset**, **Save**, and **Recompile** actions.

The graph is therefore a file/reference map, not a graph of people, concepts, claims, or evidence. The screenshot shows a conventional 2D node-link visualization; there is no evidence that this UI uses Three.js or that 3D interaction is part of Letta's design.

Letta describes the original Agent Development Environment as deprecated and replaced by the new ADE inside `chat.letta.com` and the Desktop app ([V1 ADE notice](https://docs.letta.com/v1-sdk/ade)). Letta Code links its terminal memory viewer to a browser route with `view=memory` ([source](https://github.com/letta-ai/letta-code/blob/c3dc1de3c131e28493a612ef7434cc7dbf2e1e1e/src/cli/components/MemfsTreeViewer.tsx#L48-L144)).

**Unresolved:** the authenticated `chat.letta.com` editor could not be inspected from public first-party material beyond the documented Desktop view and route. This report does not assume that every Desktop control is identical on the web.

### CLI and local browser views

Letta Code exposes the following memory commands in its current [slash-command registry](https://github.com/letta-ai/letta-code/blob/c3dc1de3c131e28493a612ef7434cc7dbf2e1e1e/src/cli/commands/registry.ts#L55-L132):

| Surface | Current behavior |
| --- | --- |
| `/memory` | Read-only split tree/preview browser for MemFS files. Keyboard navigation opens a fuller preview; `O` opens the current memory view in the ADE/browser. The TUI labels this handoff **Edit in ADE** ([source](https://github.com/letta-ai/letta-code/blob/c3dc1de3c131e28493a612ef7434cc7dbf2e1e1e/src/cli/components/MemfsTreeViewer.tsx#L347-L503)). |
| `/palace` | Generates and opens a local, static, read-only **Memory Palace** HTML report from the MemFS Git repository ([handler](https://github.com/letta-ai/letta-code/blob/c3dc1de3c131e28493a612ef7434cc7dbf2e1e1e/src/cli/app/use-submit-handler.ts#L1439-L1485), [generator](https://github.com/letta-ai/letta-code/blob/c3dc1de3c131e28493a612ef7434cc7dbf2e1e1e/src/web/generate-memory-viewer.ts#L1-L46)). |
| `/remember <text>` | Asks the agent to decide what should be remembered, choose or create a memory location, avoid duplication, and commit the update. Placement is model judgment, not a deterministic memory operation ([prompt](https://github.com/letta-ai/letta-code/blob/c3dc1de3c131e28493a612ef7434cc7dbf2e1e1e/src/agent/prompts/remember.md#L1-L29)). |
| `/doctor` | Audits memory placement, duplication, and system-prompt token usage ([docs](https://docs.letta.com/configuration/memory)). |
| Direct files | Users can inspect or edit `$MEMORY_DIR` with ordinary filesystem tools; edits become durable when committed/pushed through MemFS ([MemFS docs](https://docs.letta.com/concepts/memfs)). |

Memory Palace has four tabs: **Context**, **Core Memory**, **External Memory**, and **History**. Core and external views expose a file tree, character counts, in-context/out-of-context labels, Markdown/raw rendering, frontmatter fields, and the latest commit. History supports search and displays commit hash, author, relative date, message/body, statistics, and diffs ([template source](https://github.com/letta-ai/letta-code/blob/c3dc1de3c131e28493a612ef7434cc7dbf2e1e1e/src/web/memory-viewer-template.txt#L897-L950), [file panels](https://github.com/letta-ai/letta-code/blob/c3dc1de3c131e28493a612ef7434cc7dbf2e1e1e/src/web/memory-viewer-template.txt#L1292-L1454), [history](https://github.com/letta-ai/letta-code/blob/c3dc1de3c131e28493a612ef7434cc7dbf2e1e1e/src/web/memory-viewer-template.txt#L1668-L1802)). Its Context view visualizes token allocation across system instructions, core memory, tools, messages, summaries, and free space ([source](https://github.com/letta-ai/letta-code/blob/c3dc1de3c131e28493a612ef7434cc7dbf2e1e1e/src/web/memory-viewer-template.txt#L1075-L1222)).

### SDK and API

The current Agent SDK accepts named memory entries when creating an agent. Each entry becomes a Markdown file; entries created this way live under `system/` and are loaded into context ([Agent SDK memory docs](https://docs.letta.com/agent-sdk/memory)). Shared memory is represented by sharing a MemFS repository rather than by a semantic entity graph.

The generated remote-client API also exposes two older-shaped resources:

- **Blocks:** retrieve/list/update/attach/detach core memory blocks. A block has a label, value, description, character limit, tags, metadata, creator/updater IDs, and project association ([generated API source](https://github.com/letta-ai/letta-docs-md/blob/fe934f500abf046bf74f426dd99991bedf08f09e/api/python/resources/agents/subresources/blocks/index.md#L3-L97)).
- **Archival passages:** list text passages with embeddings, tags, file/source references, deletion state, creator/updater IDs, and `created_at`/`updated_at` timestamps ([generated API source](https://github.com/letta-ai/letta-docs-md/blob/fe934f500abf046bf74f426dd99991bedf08f09e/api/python/resources/agents/subresources/passages/index.md#L1-L140)). Semantic search supports tags, `top_k`, and filters on passage creation time ([search API source](https://github.com/letta-ai/letta-docs-md/blob/fe934f500abf046bf74f426dd99991bedf08f09e/api/python/resources/agents/subresources/passages/methods/search/index.md#L1-L73)).

The V1 documentation describes blocks as always-visible structured prompt sections and archival memory as embedding-backed, on-demand storage ([blocks](https://docs.letta.com/v1-sdk/memory/memory-blocks), [archival memory](https://docs.letta.com/v1-sdk/memory/archival-memory)). These pages belong to the legacy V1 SDK. They document surviving concepts and endpoints, not the primary current MemFS UX.

## Information hierarchy

The current MemFS hierarchy is:

1. **Agent / MemFS repository** — each agent owns a Git-backed memory repository; repositories can be shared.
2. **Context class** — files under `system/` are loaded every turn; files elsewhere remain out of context while their tree remains visible.
3. **Path and file** — memory is addressed by path and stored as Markdown.
4. **File metadata and body** — YAML frontmatter provides fields such as description; the remainder is Markdown content.
5. **References and groups** — the Desktop graph derives edges from file references and visually groups paths/categories.
6. **Git history** — commits record file changes, author, date, message/reason, and diff.

This hierarchy follows Letta's [MemFS specification](https://docs.letta.com/concepts/memfs). Files under `system/` are the current analogue of core memory; other files are external/on-demand memory. Semantic/vector search is not enabled by default for MemFS: normal file search/read is the baseline, with semantic or hybrid search available through an optional mod.

## Entities, time, and provenance

- **Semantic entities:** no current first-party UI or MemFS contract found in this snapshot exposes people/concepts as canonical entities with aliases, claims, or graph relationships. Desktop graph nodes are files. The API's deprecated block `entity_id` refers to an entity within a template, not a semantic-memory entity. Agent identities are attachment records, also not semantic entities.
- **Timestamps:** MemFS exposes Git commit dates. Archival passages expose creation/update timestamps, and search can filter by passage creation time. No first-party source found a distinction between represented-world **valid time** and database **transaction time**.
- **Provenance:** MemFS provides file-level Git author, commit message/reason, date, and diff. The write tool requires a reason and commits changes with an agent author ([write implementation](https://github.com/letta-ai/letta-code/blob/c3dc1de3c131e28493a612ef7434cc7dbf2e1e1e/src/tools/impl/memory.ts#L35-L152)). Passage APIs expose creator/updater IDs and optional source/file references. No current UI evidence was found for claim-level evidence spans, source-event lineage, or operation-level temporal corrections.

## Markdown-specific versus transferable

| Letta design | Tied to Markdown/MemFS | Transferable form for Evie's SQL temporal graph |
| --- | --- | --- |
| Graph overview | Nodes are files; edges are textual references; groups reflect directories/categories. | Overview of entities, claims, sources, and typed relationships, with scope and lifecycle filters. |
| Core vs external memory | Determined by whether a file is under `system/`. | Clearly show what is eligible for working context versus stored semantic memory, without making a filesystem path the policy boundary. |
| File detail editor | YAML frontmatter, Markdown body, raw/rendered toggle, character count. | Typed detail panel for identity, aliases, claims, confidence/status, scope, valid time, transaction time, and source evidence. |
| Save and history | Git commit is the durable change unit; history is a file diff. | Explicit semantic operations such as add, correct, supersede, retire, restore, link, and scope change; show the accepted operation log and resulting revisions. |
| `/remember` | The model decides which file/block to alter and writes prose. | Keep model extraction/proposals separate from deterministic acceptance and storage operations; Stage 3 can inspect and operate without requiring model judgment. |
| Memory Palace | Static read-only report with context, file collections, and Git history. | A read-only CLI/web inspector with overview, selected-record details, provenance, and history can use the same underlying query model. |
| Context visualization | Token allocation is derived from rendered prompt/files. | Useful later for working-context observability, but not part of establishing semantic graph correctness. |

## Implications for Evie Stage 3

Stage 3 can reasonably include a small `/memory` inspector and a web Memory tab if both remain views over deterministic graph operations rather than introducing extraction, ranking, or model acceptance policy. The first useful web hierarchy is overview/list, scoped filters (`global`, workspace/project, session where applicable), selected entity or claim, provenance, and operation history. A 2D graph may complement the exact list/detail view; a Three.js experience should remain optional because spatial presentation must not obscure source, scope, time, or lifecycle state.

Evie should borrow Letta's progressive disclosure and visible history, but not its free-form Markdown editing contract. The graph should survive restart and rebuild from accepted operations, while the UI exposes typed corrections and temporal revisions rather than mutable prose or file diffs.
