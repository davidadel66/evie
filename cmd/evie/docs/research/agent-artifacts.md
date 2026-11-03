# Agent artifacts: outputs for people and for other agents

**Research date:** 2026-11-02
**Status:** Evidence and backlog material only. This note does not authorize
implementation, change any active specification, or open any issue. Online
claims cite first-party documentation, specifications, source repositories, or
first-party announcements; secondary coverage is labeled. Inferences are marked
**Inference:**. Skills are covered by the sibling note `agent-skills.md` and
appear here only where they touch artifacts.

## Finding

Across products, an **artifact** is a durable, addressable output kept apart
from the conversation that produced it. It has an identity, usually versions,
and a renderer. The products differ in where it lives and what it may do:

- **Document-like artifacts**: ChatGPT canvas, Gemini Canvas docs and
  Microsoft Copilot Pages.
- **Sandboxed web pages**: Claude artifacts, Gemini Canvas apps, LibreChat,
  Open WebUI.
- **Tool-attached UI templates** that a server declares ahead of time: MCP
  Apps and the OpenAI Apps SDK.

The recent movement is toward pages with **declared runtime capabilities**, such
as storage, model calls, connectors and viewer identity. The host grants these
per page and brokers each call; the page itself has no network access.
([Claude Code artifacts](https://code.claude.com/docs/en/artifacts),
[MCP Apps spec](https://github.com/modelcontextprotocol/ext-apps/blob/main/specification/2026-01-26/apps.mdx))

Between agents, the protocols that name the concept separate it from messages.
A2A says plainly that "Messages SHOULD NOT be used to deliver task outputs";
results "SHOULD BE returned using Artifacts associated with a Task"
([A2A §3.7](https://a2a-protocol.org/latest/specification)). Google ADK,
Anthropic's research system and MCP resource links all keep large outputs in a
store and pass a reference.

Evie already has two of the three pieces, in separate places:

- **A sealed renderer**: Workspace tabs.
- **A durable, referenced, injection-framed agent output**: the stored subagent
  report.

It lacks a general identity, a version history and a human view for outputs
that sit outside a Workspace folder.

**Inference:** the smallest useful step is to treat the stored worker report as
Evie's first artifact and give the owner a view of it, before building any new
store.

## Direct answer: can an artifact be a message from one agent to another?

**It should be something else, but the two travel together.**

A **message** is addressed. It names a sender and a recipient, and it asks,
answers, reports status or hands over control. It lives in the recipient's
history and is read once.

An **artifact** is the work product. It is owned by a task or a scope, kept
after the turn, versioned, and fetched by anyone authorized to read it.

The usual pattern is a short message that **carries or points to** one or more
artifacts:

- A2A puts artifacts on a Task. Messages refer to earlier work through
  `referenceTaskIds`.
- MCP tool results can return a `resource_link` instead of embedding the
  content.
- Anthropic's research agents "store their work in external systems, then pass
  lightweight references back to the coordinator"
  ([Anthropic](https://www.anthropic.com/engineering/multi-agent-research-system)).

Keeping them apart matters for safety as well as size. A message from another
agent may carry a request, but it must carry no authority. Claude Code marks a
subagent's report as the subagent's words, which "carry no authority from you"
([sub-agents](https://code.claude.com/docs/en/sub-agents)). An artifact should
never carry authority at all. It is data with provenance.

Evie's delegation result already has this split:

- **The message**: status, reason, identities, sources and usage, written by the
  harness outside the untrusted frame.
- **The artifact in all but name**: the stored full report, paged on demand with
  `read_subagent_report`.

## Evie today: what already behaves like an artifact

| Evie concept | What it is | Artifact-like properties | Gap |
| --- | --- | --- | --- |
| Workspace tab ([spec](../active/workspace-tabs.spec.md), [decisions](../active/workspace-tabs.decisions.md)) | Evie-written page in `.evie/tabs/<id>/` of a Workspace folder | Durable, versioned by the folder's git, sealed renderer, owner-approved read-only data channel | Workspace with a folder only; a live **view** over data, not a snapshot of a turn's output; agents read its problems, not its content |
| Subagent report ([spec](../active/subagents.spec.md), [decisions](../active/subagents.decisions.md)) | A worker's full final report, stored durably | Stable ID (execution attempt), paged by reference, framed as untrusted, cited sources checked by the harness | No version or kind of its own; readable only by the parent session; no document view in the UI |
| Workflow run outputs ([spec](../active/workflow-runtime.spec.md)) | `outputs` JSON object per run in `workflow_runs` ([schema](../../../../internal/eviedb/workflow_runs.go)) | Durable, structured, readable by tabs through `evie.runs()` | Run-scoped JSON, not a renderable document |
| Inspector ([ui-chat spec](../active/ui-chat.spec.md), [`Panel.tsx`](../../../../internal/web/ui/src/artifacts/Panel.tsx)) | The right-hand pane: files, diffs, tools, memory, runs, builds, subagents | Already the planned home: "Files, diffs, previews, tables, and future artifacts use the same surface" | No artifact kind; "Artifact cards… Nothing emits artifacts until the whiteboard feature" |
| System explorer ([README](../../../../docs/architecture/explorer/README.md)) | A fixed viewer over agent-written data files | "Agents edit the data, never the page", plus a drift check | Repository documentation only |

Details that matter for any design:

- **Tabs are sealed.** The route sends `sandbox allow-scripts; default-src
  'none'; … connect-src 'none'; … frame-ancestors 'self'`
  ([`workspace_tabs.go`](../../../../internal/web/workspace_tabs.go)), and the
  frame is `sandbox="allow-scripts"`
  ([`WorkspaceTabFrame.tsx`](../../../../internal/web/ui/src/workspaces/WorkspaceTabFrame.tsx)).
  A headless-browser check exercises a hostile tab
  ([`workspace-tabs-browser.mjs`](../../../../scripts/workspace-tabs-browser.mjs)).
  Data approvals pin the exact data list
  ([`eviedb/workspace_tabs.go`](../../../../internal/eviedb/workspace_tabs.go),
  [`web_context_sessions.go`](../../../../cmd/evie/web_context_sessions.go)).
  WebRTC is an accepted residual risk.
- **The prompt already routes "reports" to tabs.** A session with
  `workspace_tabs` is told: "When David wants a view of this Workspace's own (a
  report, dashboard, metrics or analysis), build it as one of the Workspace's
  tabs" ([`prompt.go`](../../../../internal/agent/prompt.go)). Any artifact
  concept must say where that line falls.
- **Worker results are bounded and framed.** The inline summary defaults to
  12,000 bytes, eight results fit a 96 KiB envelope, and report pages default to
  16 KiB inside a 64 KiB envelope
  ([`subagent_result.go`](../../../../internal/eviedb/subagent_result.go),
  [decisions](../active/subagents.decisions.md)). Child-written text arrives
  inside an escaped `[begin untrusted research child output …]` frame. Harness
  fields stay outside it.
- **The UI does not show the report as a document.** The Inspector shows a
  worker's objective, the context it was given, and its activity trace
  ([`SubagentInspection.tsx`](../../../../internal/web/ui/src/subagents/SubagentInspection.tsx)).
- **"Receipts" are not outputs.** In the subagent code, receipts are Composition
  Receipts: the pinned tools and capabilities a worker had
  ([`receipt.go`](../../../../internal/subagents/receipt.go),
  [GLOSSARY](../../../../GLOSSARY.md)). **Inference:** they are the right
  provenance input. A report written by a worker that held web tools is
  web-derived.

## Products

### Claude (claude.ai and Claude Code)

**Types and rendering.** The current Help Center calls an artifact "almost
anything": documents, code, single-page websites, images, diagrams, dashboards
and small tools. It adds templates: Docs, Slides and Design, plus Dashboards
and Motion in beta. Artifacts made before 2026-09-16 are "legacy". They keep
working but cannot be newly created
([Help Center](https://support.claude.com/en/articles/17153992-what-are-artifacts-and-how-do-i-use-them)).
A 2024 account lists the classic kinds as "interactive HTML apps, Markdown,
HTML, SVG, Mermaid diagrams and React Components", rendered in "iFrame
sandboxes with full-site process isolation" with "strict Content Security
Policies" (secondary:
[Willison](https://simonwillison.net/2024/Aug/28/how-anthropic-built-artifacts/)).

**Storage, model calls and connectors.**

- Artifacts can call Claude, and each user's usage counts against their own plan
  ([announcement, 2025-07-25](https://claude.com/blog/claude-powered-artifacts)).
- Persistent storage is personal or shared, 20 MB per artifact and text only.
  The Help Center warns users to check for shared storage "before entering
  sensitive information".
- Artifacts can call the user's connectors, and Claude asks the first time.
  Connector tools that need approval for each action are unavailable
  ([Help Center](https://support.claude.com/en/articles/17153992-what-are-artifacts-and-how-do-i-use-them)).

**Claude Code's `Artifact` tool**
([docs](https://code.claude.com/docs/en/artifacts)):

- **Publishing.** The tool publishes a local `.html` or `.md` file as a page on
  claude.ai that is private by default, with a 16 MiB limit. Each publish is a
  version, and the owner chooses which version viewers see. An editor's newer
  version causes a stale publish to be refused and merged.
- **Isolation.** The page loads from a sandboxed `*.claudeusercontent.com`
  origin under a CSP. Scripts may come only from five CDN hosts. `fetch`
  reaches only the page's own origin and Google Fonts.
- **Connectors.** Calls run "through the viewing account's own connection", and
  the page cannot call connectors outside its declaration.
- **Reading other people's pages.** Reading someone else's artifact returns a
  summary that "reports instructions written into the page instead of relaying
  them".
- **Governance.** Comments sent to Claude reach a watching session, and
  publishing, sharing and deletion are audited.

**Observed in a Claude Code session, 2026-11-02:**

- The tool also accepts supporting files and an asset store. The docs' "Single
  page: relative links do not resolve" may lag the tool.
- Pages declare capabilities under runtime contract 0.2.96: `artifact`
  (self-republish), `assets`, `comments`, `db`, `downloads`, `files`, `mcp`,
  `room`, `sample` (ask Claude) and `user` (viewer identity), with
  `permissions` built in.
- `db` is a shared document store with access levels `view < interact < admin
  < owner`. The page and the agent, through an `ArtifactData` tool, read and
  write the same rows. The guidance says "No secrets; shared data is
  untrusted."
- A design skill must be loaded before writing a page.

### OpenAI

**Canvas.** The help article describes canvas as rendering React and HTML "in
a sandbox environment", with version history and "Show changes". OpenAI's
model release notes for 2026-05-28 say canvas "will no longer be available in
GPT-5.5 Instant or GPT-5.5 Thinking". Writing and coding moved to in-chat
writing and code blocks
([canvas](https://help.openai.com/en/articles/9930697-what-is-the-canvas-feature-in-chatgpt-and-how-do-i-use-it),
[release notes](https://help.openai.com/en/articles/9624314-model-release-notes),
[blocks](https://help.openai.com/en/articles/20001246-working-with-writing-blocks-and-code-blocks-in-chatgpt);
search-index text, because direct fetches returned 403).

**Apps SDK.** Components "run inside an iframe in ChatGPT" and speak the MCP
Apps bridge. ChatGPT-only extensions sit under `window.openai`. Widget state
splits `modelContent`, which the model sees, from `privateContent`, which is
"UI-only state the model should not see"
([Apps SDK UI](https://developers.openai.com/apps-sdk/build/chatgpt-ui)). The
security guide says "Assume prompt injection and malicious inputs will reach
your server" and asks for human confirmation of irreversible operations
([security](https://developers.openai.com/apps-sdk/guides/security-privacy)).

### MCP Apps (formerly MCP-UI)

SEP-1865 is Final. MCP Apps became "the first official MCP extension" on
2026-01-26
([blog](https://blog.modelcontextprotocol.io/posts/2026-01-26-mcp-apps/)).

- **Templates.** Servers predeclare `ui://` resources of type
  `text/html;profile=mcp-app`. Tools point to them with `_meta.ui.resourceUri`,
  so hosts can prefetch and review templates. Embedding UI directly in tool
  results, MCP-UI's original approach, was deferred
  ([SEP-1865](https://modelcontextprotocol.io/seps/1865-mcp-apps-interactive-user-interfaces-for-mcp)).
- **Isolation.** A web host "MUST wrap the View … through an intermediate
  Sandbox proxy" on a different origin. The CSP is built from declared
  `connectDomains` and `resourceDomains`, and defaults to no network.
- **Two audiences.** A tool result's `content` is for the model, while
  `structuredContent` is "optimized for UI rendering (not added to model
  context)".
- **Model context.** `ui/update-model-context` overwrites the View's previous
  context
  ([spec 2026-01-26](https://github.com/modelcontextprotocol/ext-apps/blob/main/specification/2026-01-26/apps.mdx)).
- **Support.** Supported hosts include Claude, VS Code GitHub Copilot, Microsoft
  365 Copilot and Goose
  ([overview](https://modelcontextprotocol.io/extensions/apps/overview)).

OpenAI's guide says "The model receives `structuredContent` from the data
tool", which contradicts the MCP Apps text above. Hosts evidently differ.

### Gemini and Microsoft

**Gemini Canvas** creates docs, apps, slides and code, and has a preview with a
console. Docs keep versions. A shared app link carries the warning "Anyone with
that public link can view and edit data associated with the app"
([Gemini Help](https://support.google.com/gemini/answer/16047321?hl=en&co=GENIE.Platform%3DDesktop)).

**Copilot Pages** is "a dynamic, persistent canvas"
([how it works](https://support.microsoft.com/en-us/microsoft-365-copilot/how-microsoft-365-copilot-pages-works)).
It is "saved as a .page file in a new user-owned SharePoint Embedded container"
([FAQ](https://support.microsoft.com/en-us/microsoft-365-copilot/frequently-asked-questions-about-microsoft-365-copilot-pages)).
A copied component stays live: "Changes made to the component within Microsoft
365 apps will update the page in real time"
([share](https://support.microsoft.com/en-US/Microsoft-365-Copilot/share-a-microsoft-365-copilot-page)).

### Open-source chat UIs

- **LibreChat** renders React, HTML, SVG, Markdown and Mermaid. The preview
  "runs in a sandboxed iframe using CodeSandbox's Sandpack", and the bundler
  can be self-hosted. Artifacts are an agent capability with an
  `:::artifact{…}` wrapper syntax and a version menu
  ([docs](https://www.librechat.ai/docs/features/artifacts)).
- **Open WebUI** renders HTML and SVG in a `srcdoc` iframe. Same-origin access
  is off by default. An optional `IFRAME_CSP` is prepended as a meta tag, and
  "the first policy wins". Each edit creates a version
  ([docs](https://docs.openwebui.com/features/chat-conversations/chat-features/code-execution/artifacts)).
  Its advisories show the failure mode, each a high-severity stored XSS from
  `allow-scripts` plus `allow-same-origin`:
  - the HTML view, fixed in 0.6.5
    ([GHSA-4vrc-m9ch-6m3r](https://github.com/advisories/GHSA-4vrc-m9ch-6m3r));
  - message `embeds`, which "ignor[ed] the … Allow Same Origin configuration"
    and spread through shared chats, fixed in 0.6.44
    ([GHSA-vjm7-m4xh-7wrc](https://github.com/advisories/GHSA-vjm7-m4xh-7wrc));
  - citation previews, fixed in 0.7.0
    ([GHSA-xc8p-9rr6-97r2](https://github.com/advisories/GHSA-xc8p-9rr6-97r2));
  - a file preview served from the app's own origin, "so the sandbox provided
    no isolation at all", fixed in 0.11.0
    ([GHSA-3xpf-xq7r-v8c5](https://github.com/advisories/GHSA-3xpf-xq7r-v8c5)).
- **LobeHub's** `HtmlPreview` component defaults to `allow-scripts allow-forms
  allow-modals` without same-origin, and calls overriding it "Strongly
  discouraged"
  ([component docs](https://ui.lobehub.com/components/html-preview)). I did not
  confirm that LobeChat's artifact panel uses this component.

### Comparison

| Product | Unit | Isolation | Versions | Persistence and sharing | Calls model or tools |
| --- | --- | --- | --- | --- | --- |
| Claude artifacts | Page, doc or template | Sandboxed separate origin, CSP, CDN allowlist | Per publish; owner picks the viewed version | Private by default; org or public; 20 MB storage; Claude Code `db` | Yes: Claude, viewer's connectors, declared |
| ChatGPT canvas, then blocks | Doc or code | "Sandbox" (no details) | Version arrows, diff | In chat | No |
| OpenAI Apps SDK | Tool UI component | Iframe, strict CSP | n/a | Widget state per instance, not durable | `callTool`, follow-up message |
| MCP Apps | `ui://` template plus tool data | Sandbox proxy on another origin; declared CSP | Template reviewable | Host-defined | `tools/call`, `ui/message`, model context |
| Gemini Canvas | Doc, app or slides | Preview (no details) | Doc versions | Public link; app data editable by link holders | Gemini features |
| Copilot Pages | `.page` / Loop | Microsoft 365 app | Not documented | SharePoint Embedded; live components | Copilot edits |
| LibreChat / Open WebUI | Code block rendered | Sandpack iframe / `srcdoc` sandbox | Version menu | In chat | No |

## Artifacts between agents

- **A2A 1.0.0.**
  - A Message has `role`, `messageId`, `parts`, and optional `contextId`,
    `taskId` and `referenceTaskIds`.
  - An Artifact has `artifactId` ("unique within a task"), `name`, `parts` and
    `metadata`.
  - A Part holds exactly one of `text`, `raw`, `url` or `data`, so a payload can
    go by value or by reference.
  - Artifacts stream through `TaskArtifactUpdateEvent` with `append` and
    `lastChunk`
    ([spec](https://a2a-protocol.org/latest/specification)).
  - A finished task "cannot restart". A refinement is a new task in the same
    `contextId` that references the original, and agents "reuse a consistent
    `artifact-name`" for refined versions. That linkage is client-managed
    ([life of a task](https://a2a-protocol.org/latest/topics/life-of-a-task/)).
- **MCP 2026-07-28.** A tool result can embed a resource or return a
  `resource_link`, and links "are not guaranteed to appear" in
  `resources/list`. Annotations carry `audience` (`user`, `assistant`). The
  spec's non-normative guidance on stateful tools says "a handle is a name, not
  a capability", so the server should check authorization against the handle
  on every call
  ([tools](https://modelcontextprotocol.io/specification/2026-07-28/server/tools)).
- **Google ADK** defines artifacts as "named, versioned binary data". Saving
  under an existing name adds a version. A plain filename is session-scoped,
  and a `user:` prefix makes the artifact visible across that user's sessions.
  Session state is "not optimized for storing large amounts of data".
  `LoadArtifactsTool` lists artifacts and loads one into a single request
  without saving it into history
  ([ADK](https://adk.dev/artifacts/)).
- **OpenAI Agents SDK.** A handoff transfers control. By default the receiver
  "gets to see the entire previous conversation history", which input filters
  can trim. `Agent.as_tool` passes structured input without a transfer
  ([handoffs](https://openai.github.io/openai-agents-python/handoffs/)).
- **LangGraph** uses shared graph state. With handoffs "you must explicitly
  decide what messages pass between agents", or agents get "malformed
  conversation history or bloated context"
  ([handoffs](https://docs.langchain.com/oss/python/langchain/multi-agent/handoffs)).
- **Claude Code.** Subagents return "only the summary". Reports are scanned and
  arrive under a header that marks them as subagent output
  ([sub-agents](https://code.claude.com/docs/en/sub-agents)). Experimental
  agent teams add a shared task list and JSON-file mailboxes. "A teammate can't
  approve a permission prompt or supply consent on your behalf"
  ([agent teams](https://code.claude.com/docs/en/agent-teams)).
- **Blackboards.** Research systems post to a shared board and pick the next
  agent from its contents
  ([Han and Zhang](https://arxiv.org/abs/2507.01701),
  [Salemi et al.](https://arxiv.org/abs/2510.01285)). The prior note found
  coordination cost dominates when agents mutate shared artifacts
  ([delegation efficiency](multi-agent-delegation-efficiency.md)).

### The documented distinction

| Dimension | Message | Artifact |
| --- | --- | --- |
| Purpose | Coordinate: ask, clarify, report status, transfer control | Deliver the output (A2A §3.7) |
| Addressing | To a recipient (A2A `role` and `taskId`; Claude Code `SendMessage` `to`) | Owned by a task or scope; fetched by ID or URI |
| Lifetime | Part of one history; may be compacted | Outlives the turn; listed and re-read (ADK, Claude) |
| Versioning | None; a new message supersedes | Versions (ADK auto-increment, Claude publishes, A2A refined name) |
| Size and delivery | Small, inline | Chunked (A2A `append`), paged, or linked (`resource_link`) |
| References | Points at work (`referenceTaskIds`) | Is the thing pointed at; the handle is a name, not a capability (MCP) |
| Authority | Another agent's message carries none of the user's | Never any; data with provenance |

**Pass by value** when the content is small, immutable and needed in the very
next model call: a status or a bounded summary. **Pass by reference** when it
is large, read selectively, shared with a human view, versioned, or derived from
untrusted input. A reference keeps the content out of prompts until it is
asked for, which reduces both token cost and the surface for injection
(Anthropic; ADK). Evie's worker result already follows this rule: the summary
goes by value and the report by reference.

## Security

**Rendering model-written HTML/JS.**

- **Opaque or separate origin.** MDN says combining `allow-scripts` with
  `allow-same-origin` on a same-origin document "lets the embedded document
  remove the `sandbox` attribute"
  ([MDN](https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Elements/iframe)).
  The Open WebUI advisories are that failure in practice. Claude uses a
  separate origin. MCP Apps add a proxy on a second origin.
- **CSP that denies network by default.** OWASP: "Employ strict Content Security
  Policies (CSP) to mitigate the risk of XSS attacks from LLM-generated
  content", and "Treat the model as any other user"
  ([LLM05](https://genai.owasp.org/llmrisk/llm052025-improper-output-handling/)).
- **No direct capabilities.** Data and actions go through a host-brokered,
  declared, consented channel. Examples are Claude's capabilities and
  connector declarations, MCP Apps' tool visibility and Evie's tab helper.
- **Residual exfiltration** is real: Evie documents WebRTC. A page that can both
  read private data and reach out completes the "lethal trifecta" of private
  data, untrusted content and external communication (independent researcher:
  [Willison](https://simonwillison.net/2025/Jun/16/the-lethal-trifecta/)).

**Agents reading other agents' artifacts.**

- Prompt injection propagates between agents. "Prompt Infection" demonstrates
  self-replicating LLM-to-LLM injection and proposes tagging the source of
  inter-agent content ([Lee and Tiwari](https://arxiv.org/abs/2410.07283)).
- Documented mitigations:
  - **Frame and label the content.** Claude Code uses a report header. Evie uses
    its untrusted frame.
  - **Summarize rather than relay** embedded instructions, as Claude Code does
    when reading someone else's artifact.
  - **Never accept relayed approvals** (agent teams).
  - **Validate structured parts against a schema.** A2A requires that "Content
    MUST be validated against the A2A protocol schema" and that file references
    be checked for SSRF.
  - **Keep untrusted data away from privileged planning.** The "Dual LLM"
    pattern ("A privileged LLM has access to tools but never processes
    untrusted data") and "LLM Map-Reduce" isolate third-party data
    ([design patterns](https://arxiv.org/abs/2506.08837)).
- No source claims injection is solved.

## How Evie could adapt artifacts

Everything in this section is **Inference**, grounded in the Evie code above.

### Principles

1. **One stored object, two views.** An artifact version holds:
   - a body for people, rendered in the Inspector or a tab;
   - a bounded model-facing summary;
   - optional structured JSON.

   Agents get the summary plus a reference by value, and page the body on
   demand. This is the subagent result contract generalized. It also matches
   `content` vs `structuredContent` and `modelContent` vs `privateContent`.
2. **Immutable versions with a single writer.** Each version is append-only
   and content-hashed. Only the producing session, attempt or run writes new
   versions. Other agents create derived artifacts that reference it. This
   avoids blackboard write races and keeps provenance linear.
3. **Provenance written by the harness, outside the frame:**
   - the producer: session, execution or run;
   - its Composition Receipt digest;
   - the Context Scope;
   - the sources the harness verified;
   - a taint flag (web-derived, Workspace data, owner-authored);
   - the time.
4. **Scope follows Context Scope, enforced in code.** A session artifact is
   readable by that session and its parent. A Workspace artifact is readable by
   that Workspace's sessions. A Global Conversation artifact is readable by
   Global Conversations. Every read checks the reference against the reader's
   scope, because the handle is a name, not a capability.
5. **Rendering reuses the tab seal.**
   - Markdown, tables and JSON render natively and inert, with no raw HTML.
   - SVG and HTML render only in the existing `sandbox allow-scripts`,
     `connect-src 'none'` frame with the WebRTC prelude, through a route like
     the tab route.
   - An HTML artifact gets no data channel unless the owner approves an exact
     list, as with tabs.
   - Mermaid is omitted, given the owner's stated preference for hand-built SVG.
6. **No authority in artifacts.** Reading an artifact never satisfies an
   approval. Its text reaches a model only inside the untrusted frame.

### Options

| Option | Shape | Pros | Cons |
| --- | --- | --- | --- |
| **A. Files only** | Artifacts are files under a Workspace folder, for example `.evie/artifacts/<id>/` with a manifest; the Inspector renders them through the tab seal | No schema; git gives versions and review; reuses file tools and tabs | Needs a folder, so no Global Conversations, folderless Workspaces or research workers; provenance and scope live in files that agents can edit; overlaps tabs |
| **B. SQLite artifact store** | `artifacts` plus immutable `artifact_versions` rows (media type, hash, bytes, producer, scope, taint, summary), one general `artifact` tool (publish, read, list) gated by scope in code, Inspector `artifact` kind | Works in every scope; provenance and authorization in code; one reference format for people and agents; matches A2A and ADK | New persistence, migration, retention and erasure; must not become a second tab system or a memory side door |
| **C. Unified references first** | No new store. A reference format over existing stores (worker report, run outputs, Workspace file, tab) that the Inspector and a read tool understand | Smallest; no duplication; tests the "same object, two views" contract on real data | Adds no home for standalone model-authored documents; versions differ by source |

**Recommendation:** start with C and grow into B when a measured need appears,
such as "I want Evie's analysis kept, cited later, outside a folder". Keep A as
the tab mechanism it already is.

**What not to duplicate.**

- **Tabs stay the live views over data.** An artifact is a snapshot output. It
  could later be "promoted" into a tab, or committed to a folder, by an explicit
  owner action.
- **Worker reports are not copied into a new table.** In B they would *become*
  artifact versions, and `read_subagent_report` would read them, with pinned
  sessions kept byte-identical.
- **The Inspector stays the only surface.** No second rail.

### Recommended first slice

**Open a worker's stored report as a document in the Inspector.** It is one
outcome on the UI and a read-only route, with no new table, no new model tool
and no prompt change.

- **Outcome:** a finished or partial research, explore or build worker shows a
  **Report** view.
  - The view renders the exact stored report as inert Markdown.
  - The harness-written provenance appears outside the body: worker kind,
    status, verified and unverified sources, usage, and the producing session.
  - The parent model keeps reading the same bytes through
    `read_subagent_report`.
- **Acceptance:**
  - The view shows the full stored bytes; the trace's cut text is not used.
  - Raw HTML and script in a report render as text.
  - Links are http(s) only.
  - Another session's attempt is refused.
  - A pre-Stage 10 result renders.
- **Verification:**
  - Go route test for the scope refusal.
  - UI test with a hostile report.
  - `./scripts/verify-change.sh`.
- **Risk:** low. It is read-only. The only new boundary is that the report
  route must enforce scope as `ReadSubagentReport` does
  ([`subagent_result.go`](../../../../internal/eviedb/subagent_result.go)).
- **What it proves:** whether "one stored object, two views" is worth
  generalizing, tested on real reports before any schema exists.

**Possible next slices**, each separately reviewable:

1. An `ArtifactRef` shape shared by the Inspector and tool results.
2. Option B's store, with Markdown and JSON only.
3. HTML artifacts through the tab seal.
4. Promotion to a tab or a folder.

## Open questions for the owner

1. Should artifacts exist outside a Workspace folder, in Global Conversations
   and folderless Workspaces? Yes implies B; no implies A.
2. Where is the line between a **tab** (a live view) and an **artifact** (a
   kept output)? Should the `workspace_tabs` prompt line keep sending "reports"
   to tabs?
3. Who may read an artifact: the producing session only, its parent, the whole
   Workspace? May a Global Conversation cite a Workspace artifact?
4. May artifacts feed Memory or Automatic Recall? Web-derived ones would bring
   untrusted text into memory.
5. Do artifacts follow Workspace retention, export and Hard Erasure, like run
   records (workflow-runtime story 64)?
6. Single writer with derived versions, or a shared board that several agents
   edit?
7. Are model-written HTML artifacts wanted at all at first, or Markdown, tables
   and SVG only?

## Not verified

- help.openai.com returned HTTP 403. The canvas, release-note and blocks claims
  come from search-index text.
- The Claude Code capability list, `db` access levels and supporting files were
  observed in this session's tool and skill text, not in public docs.
- LobeChat's artifact renderer was not confirmed to use `HtmlPreview`.
- The classic Claude artifact MIME types appear only in unofficial prompt leaks,
  so they are omitted.
- The blackboard papers were read at abstract level.
- No product was exercised live.

## Local verification

- `git diff --check`: passed with no diagnostics. The command does not inspect
  this untracked file.
- `git diff --no-index --check /dev/null cmd/evie/docs/research/agent-artifacts.md`:
  no whitespace diagnostics; exit 1 reflects the new file differing from
  `/dev/null`.
- A Python `pathlib` check resolved every local Markdown link.
- `go run ./scripts/explorer check`: "the explorer data matches the code".
- `./scripts/verify-change.sh` was not run: this change is research
  documentation only.
