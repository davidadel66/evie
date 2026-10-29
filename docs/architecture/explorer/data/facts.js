evieExplorer.register(
{
  "format": 1,
  "kind": "facts",
  "packages": [
    {
      "path": "cmd/evie",
      "name": "main",
      "command": true,
      "doc": "Command evie is the agent harness — the nucleus of this repo.",
      "testSupport": false
    },
    {
      "path": "cmd/finance",
      "name": "main",
      "command": true,
      "doc": "Command finance is the CLI frontend over internal/finance: link banks via Plaid, sync transactions, seed categorization rules, and sanity-check the database.",
      "testSupport": false
    },
    {
      "path": "cmd/todo",
      "name": "main",
      "command": true,
      "doc": "Command todo is the owner CLI for Evie's durable SQLite Task Trees.",
      "testSupport": false
    },
    {
      "path": "cmd/ytscribe",
      "name": "main",
      "command": true,
      "doc": "Command ytscribe is the CLI frontend for Evie's YouTube transcript library.",
      "testSupport": false
    },
    {
      "path": "internal/agent",
      "name": "agent",
      "command": false,
      "doc": "Package agent is the harness core: the model↔tools loop, extracted from any particular frontend.",
      "testSupport": false
    },
    {
      "path": "internal/changes",
      "name": "changes",
      "command": false,
      "doc": "Package changes is a Workspace's Repository and the Changes made in it (changes.spec.md).",
      "testSupport": false
    },
    {
      "path": "internal/composition",
      "name": "composition",
      "command": false,
      "doc": "Package composition owns the Kernel's content-free durable identity for one session composition.",
      "testSupport": false
    },
    {
      "path": "internal/delegation",
      "name": "delegation",
      "command": false,
      "doc": "Package delegation defines durable Kernel execution records, separate from Tasks.",
      "testSupport": false
    },
    {
      "path": "internal/eviedb",
      "name": "eviedb",
      "command": false,
      "doc": "",
      "testSupport": false
    },
    {
      "path": "internal/finance",
      "name": "finance",
      "command": false,
      "doc": "Package finance owns the personal-finance domain: linking banks through Plaid, syncing transactions into a local SQLite database (~/.finance/finance.db), and rule-based categorization.",
      "testSupport": false
    },
    {
      "path": "internal/github",
      "name": "github",
      "command": false,
      "doc": "Package github is the small GitHub REST client the Changes feature reads and later writes through (changes.spec.md).",
      "testSupport": false
    },
    {
      "path": "internal/localembedding",
      "name": "localembedding",
      "command": false,
      "doc": "Package localembedding implements the selected local-only Ollama embedding protocol.",
      "testSupport": false
    },
    {
      "path": "internal/localextractor",
      "name": "localextractor",
      "command": false,
      "doc": "Package localextractor implements the explicitly configured, pinned local compiler transport.",
      "testSupport": false
    },
    {
      "path": "internal/localfiles",
      "name": "localfiles",
      "command": false,
      "doc": "Package localfiles reads current disk contents beneath one attached folder.",
      "testSupport": false
    },
    {
      "path": "internal/localterminal",
      "name": "localterminal",
      "command": false,
      "doc": "Package localterminal owns an owner's interactive shell and its PTY lifetime.",
      "testSupport": false
    },
    {
      "path": "internal/mcpchat",
      "name": "mcpchat",
      "command": false,
      "doc": "Package mcpchat is the Workspace side of MCP connections (workspace-mcp-connections.spec.md): which reviewed server tools a chat may reach, and the owner's tool review that changes Workspace Access.",
      "testSupport": false
    },
    {
      "path": "internal/mcpconn",
      "name": "mcpconn",
      "command": false,
      "doc": "Package mcpconn owns MCP Connections (workspace-mcp-connections.spec.md): the Kernel's record of each hosted MCP server, the OAuth sign-in that connects it, the Keychain that holds its credentials, and the HTTP transport that carries its tokens.",
      "testSupport": false
    },
    {
      "path": "internal/mcpconn/mcptest",
      "name": "mcptest",
      "command": false,
      "doc": "Package mcptest provides a fake hosted MCP server and OAuth authorization server for tests, shaped like Square's (workspace-mcp-connections story 0): the MCP endpoint answers 401 with resource_metadata, Dynamic Client Registration issues public clients, PKCE is S256, and every refresh rotates the refresh token.",
      "testSupport": true
    },
    {
      "path": "internal/memory",
      "name": "memory",
      "command": false,
      "doc": "",
      "testSupport": false
    },
    {
      "path": "internal/memorybackground",
      "name": "memorybackground",
      "command": false,
      "doc": "Package memorybackground is the always-on background memory pass (docs/memory-architecture.md, Stage 4 and decision 4): after a root conversation goes idle, or every N new owner messages, it reads only the owner's new messages, extracts candidate memories, compares each with the most similar existing memories, and queues ADD / UPDATE / INVALIDATE proposals (Mem0's operations; NOOP queues nothing) in the owner's review inbox.",
      "testSupport": false
    },
    {
      "path": "internal/memoryeval",
      "name": "memoryeval",
      "command": false,
      "doc": "Package memoryeval defines the versioned fixture and report contracts shared by Semantic Memory evaluation stages.",
      "testSupport": false
    },
    {
      "path": "internal/openrouter",
      "name": "openrouter",
      "command": false,
      "doc": "Package openrouter is a minimal client for OpenRouter's Chat Completions and Responses APIs.",
      "testSupport": false
    },
    {
      "path": "internal/plugins",
      "name": "plugins",
      "command": false,
      "doc": "Package plugins contains the Kernel-owned composition seam for compiled first-party plugins.",
      "testSupport": false
    },
    {
      "path": "internal/procedural",
      "name": "procedural",
      "command": false,
      "doc": "Package procedural is Evie's procedural Git repository: the reviewed, versioned home of procedural assets such as Workflow Definitions (Stage 14 sub-stage 2 of docs/memory-architecture.md; ADR 0007, 0012).",
      "testSupport": false
    },
    {
      "path": "internal/repoinstructions",
      "name": "repoinstructions",
      "command": false,
      "doc": "Package repoinstructions loads only designated root repository guidance.",
      "testSupport": false
    },
    {
      "path": "internal/subagents",
      "name": "subagents",
      "command": false,
      "doc": "Package subagents supervises bounded foreground children in the Kernel.",
      "testSupport": false
    },
    {
      "path": "internal/task",
      "name": "task",
      "command": false,
      "doc": "Package task defines the Kernel-owned durable Task contract.",
      "testSupport": false
    },
    {
      "path": "internal/taskeval",
      "name": "taskeval",
      "command": false,
      "doc": "Package taskeval derives a stable, content-free evidence baseline for durable Task evaluation.",
      "testSupport": false
    },
    {
      "path": "internal/tools",
      "name": "tools",
      "command": false,
      "doc": "Package tools is the agent's toolbox: every capability the model can invoke, one file per tool, plus the registry and dispatcher that wire them into the chat loop.",
      "testSupport": false
    },
    {
      "path": "internal/untrusted",
      "name": "untrusted",
      "command": false,
      "doc": "Package untrusted fences text Evie did not author — web pages, transcripts, stored tool output, a research child's report — as data, so the text can neither close its frame nor read as instructions.",
      "testSupport": false
    },
    {
      "path": "internal/usage",
      "name": "usage",
      "command": false,
      "doc": "Package usage provides content-free owner diagnostics.",
      "testSupport": false
    },
    {
      "path": "internal/web",
      "name": "web",
      "command": false,
      "doc": "The approval handshake: a gated tool call pauses the turn mid-Send while the answer arrives as a separate HTTP request.",
      "testSupport": false
    },
    {
      "path": "internal/workflow",
      "name": "workflow",
      "command": false,
      "doc": "Package workflow loads, validates, hashes, reviews and simulates reviewed Workflow Definitions (workflow-runtime.spec.md, Stage 14 sub-stage 1).",
      "testSupport": false
    },
    {
      "path": "internal/workflowruntime",
      "name": "workflowruntime",
      "command": false,
      "doc": "Package workflowruntime is the Workflow Runtime's single entry point (workflow-runtime.spec.md).",
      "testSupport": false
    },
    {
      "path": "internal/workspacetabs",
      "name": "workspacetabs",
      "command": false,
      "doc": "Package workspacetabs reads a Workspace's own tabs from its folder (workspace-tabs.spec.md).",
      "testSupport": false
    },
    {
      "path": "internal/worktree",
      "name": "worktree",
      "command": false,
      "doc": "Package worktree is the harness-owned lifecycle of a build worker's git worktree (Stage 11 of docs/memory-architecture.md, 2026-10-03): planning it before admission, creating it on a fresh branch, committing the worker's changes as the harness, reporting the branch against its base, and cleaning it up.",
      "testSupport": false
    },
    {
      "path": "internal/youtube",
      "name": "youtube",
      "command": false,
      "doc": "Package youtube owns Evie's durable YouTube transcript library.",
      "testSupport": false
    }
  ],
  "imports": [
    {
      "from": "cmd/evie",
      "to": "internal/agent"
    },
    {
      "from": "cmd/evie",
      "to": "internal/changes"
    },
    {
      "from": "cmd/evie",
      "to": "internal/composition"
    },
    {
      "from": "cmd/evie",
      "to": "internal/delegation"
    },
    {
      "from": "cmd/evie",
      "to": "internal/eviedb"
    },
    {
      "from": "cmd/evie",
      "to": "internal/finance"
    },
    {
      "from": "cmd/evie",
      "to": "internal/github"
    },
    {
      "from": "cmd/evie",
      "to": "internal/localextractor"
    },
    {
      "from": "cmd/evie",
      "to": "internal/mcpchat"
    },
    {
      "from": "cmd/evie",
      "to": "internal/mcpconn"
    },
    {
      "from": "cmd/evie",
      "to": "internal/memory"
    },
    {
      "from": "cmd/evie",
      "to": "internal/memorybackground"
    },
    {
      "from": "cmd/evie",
      "to": "internal/openrouter"
    },
    {
      "from": "cmd/evie",
      "to": "internal/plugins"
    },
    {
      "from": "cmd/evie",
      "to": "internal/procedural"
    },
    {
      "from": "cmd/evie",
      "to": "internal/subagents"
    },
    {
      "from": "cmd/evie",
      "to": "internal/task"
    },
    {
      "from": "cmd/evie",
      "to": "internal/tools"
    },
    {
      "from": "cmd/evie",
      "to": "internal/usage"
    },
    {
      "from": "cmd/evie",
      "to": "internal/web"
    },
    {
      "from": "cmd/evie",
      "to": "internal/workflow"
    },
    {
      "from": "cmd/evie",
      "to": "internal/workflowruntime"
    },
    {
      "from": "cmd/evie",
      "to": "internal/workspacetabs"
    },
    {
      "from": "cmd/evie",
      "to": "internal/worktree"
    },
    {
      "from": "cmd/finance",
      "to": "internal/finance"
    },
    {
      "from": "cmd/todo",
      "to": "internal/delegation"
    },
    {
      "from": "cmd/todo",
      "to": "internal/eviedb"
    },
    {
      "from": "cmd/todo",
      "to": "internal/memory"
    },
    {
      "from": "cmd/todo",
      "to": "internal/task"
    },
    {
      "from": "cmd/ytscribe",
      "to": "internal/youtube"
    },
    {
      "from": "internal/agent",
      "to": "internal/delegation"
    },
    {
      "from": "internal/agent",
      "to": "internal/memory"
    },
    {
      "from": "internal/agent",
      "to": "internal/openrouter"
    },
    {
      "from": "internal/agent",
      "to": "internal/repoinstructions"
    },
    {
      "from": "internal/agent",
      "to": "internal/task"
    },
    {
      "from": "internal/agent",
      "to": "internal/tools"
    },
    {
      "from": "internal/changes",
      "to": "internal/delegation"
    },
    {
      "from": "internal/changes",
      "to": "internal/eviedb"
    },
    {
      "from": "internal/changes",
      "to": "internal/github"
    },
    {
      "from": "internal/changes",
      "to": "internal/localfiles"
    },
    {
      "from": "internal/changes",
      "to": "internal/memory"
    },
    {
      "from": "internal/changes",
      "to": "internal/worktree"
    },
    {
      "from": "internal/delegation",
      "to": "internal/composition"
    },
    {
      "from": "internal/delegation",
      "to": "internal/memory"
    },
    {
      "from": "internal/delegation",
      "to": "internal/openrouter"
    },
    {
      "from": "internal/delegation",
      "to": "internal/untrusted"
    },
    {
      "from": "internal/eviedb",
      "to": "internal/composition"
    },
    {
      "from": "internal/eviedb",
      "to": "internal/delegation"
    },
    {
      "from": "internal/eviedb",
      "to": "internal/localembedding"
    },
    {
      "from": "internal/eviedb",
      "to": "internal/mcpconn"
    },
    {
      "from": "internal/eviedb",
      "to": "internal/memory"
    },
    {
      "from": "internal/eviedb",
      "to": "internal/repoinstructions"
    },
    {
      "from": "internal/eviedb",
      "to": "internal/task"
    },
    {
      "from": "internal/eviedb",
      "to": "internal/usage"
    },
    {
      "from": "internal/eviedb",
      "to": "internal/workflow"
    },
    {
      "from": "internal/eviedb",
      "to": "internal/workspacetabs"
    },
    {
      "from": "internal/localextractor",
      "to": "internal/eviedb"
    },
    {
      "from": "internal/localextractor",
      "to": "internal/memory"
    },
    {
      "from": "internal/localfiles",
      "to": "internal/memory"
    },
    {
      "from": "internal/localfiles",
      "to": "internal/tools"
    },
    {
      "from": "internal/localterminal",
      "to": "internal/memory"
    },
    {
      "from": "internal/mcpchat",
      "to": "internal/eviedb"
    },
    {
      "from": "internal/mcpchat",
      "to": "internal/mcpconn"
    },
    {
      "from": "internal/mcpchat",
      "to": "internal/memory"
    },
    {
      "from": "internal/mcpchat",
      "to": "internal/workflow"
    },
    {
      "from": "internal/memorybackground",
      "to": "internal/localextractor"
    },
    {
      "from": "internal/memorybackground",
      "to": "internal/memory"
    },
    {
      "from": "internal/memorybackground",
      "to": "internal/openrouter"
    },
    {
      "from": "internal/plugins",
      "to": "internal/composition"
    },
    {
      "from": "internal/plugins",
      "to": "internal/delegation"
    },
    {
      "from": "internal/plugins",
      "to": "internal/finance"
    },
    {
      "from": "internal/plugins",
      "to": "internal/mcpconn"
    },
    {
      "from": "internal/plugins",
      "to": "internal/memory"
    },
    {
      "from": "internal/plugins",
      "to": "internal/openrouter"
    },
    {
      "from": "internal/plugins",
      "to": "internal/task"
    },
    {
      "from": "internal/plugins",
      "to": "internal/tools"
    },
    {
      "from": "internal/plugins",
      "to": "internal/untrusted"
    },
    {
      "from": "internal/plugins",
      "to": "internal/workflow"
    },
    {
      "from": "internal/plugins",
      "to": "internal/workspacetabs"
    },
    {
      "from": "internal/repoinstructions",
      "to": "internal/memory"
    },
    {
      "from": "internal/subagents",
      "to": "internal/agent"
    },
    {
      "from": "internal/subagents",
      "to": "internal/composition"
    },
    {
      "from": "internal/subagents",
      "to": "internal/delegation"
    },
    {
      "from": "internal/subagents",
      "to": "internal/eviedb"
    },
    {
      "from": "internal/subagents",
      "to": "internal/memory"
    },
    {
      "from": "internal/subagents",
      "to": "internal/openrouter"
    },
    {
      "from": "internal/subagents",
      "to": "internal/tools"
    },
    {
      "from": "internal/subagents",
      "to": "internal/untrusted"
    },
    {
      "from": "internal/subagents",
      "to": "internal/worktree"
    },
    {
      "from": "internal/taskeval",
      "to": "internal/memory"
    },
    {
      "from": "internal/taskeval",
      "to": "internal/plugins"
    },
    {
      "from": "internal/taskeval",
      "to": "internal/task"
    },
    {
      "from": "internal/tools",
      "to": "internal/eviedb"
    },
    {
      "from": "internal/tools",
      "to": "internal/finance"
    },
    {
      "from": "internal/tools",
      "to": "internal/memory"
    },
    {
      "from": "internal/tools",
      "to": "internal/openrouter"
    },
    {
      "from": "internal/tools",
      "to": "internal/untrusted"
    },
    {
      "from": "internal/tools",
      "to": "internal/youtube"
    },
    {
      "from": "internal/web",
      "to": "internal/agent"
    },
    {
      "from": "internal/web",
      "to": "internal/changes"
    },
    {
      "from": "internal/web",
      "to": "internal/eviedb"
    },
    {
      "from": "internal/web",
      "to": "internal/finance"
    },
    {
      "from": "internal/web",
      "to": "internal/github"
    },
    {
      "from": "internal/web",
      "to": "internal/localfiles"
    },
    {
      "from": "internal/web",
      "to": "internal/localterminal"
    },
    {
      "from": "internal/web",
      "to": "internal/mcpchat"
    },
    {
      "from": "internal/web",
      "to": "internal/mcpconn"
    },
    {
      "from": "internal/web",
      "to": "internal/memory"
    },
    {
      "from": "internal/web",
      "to": "internal/openrouter"
    },
    {
      "from": "internal/web",
      "to": "internal/plugins"
    },
    {
      "from": "internal/web",
      "to": "internal/tools"
    },
    {
      "from": "internal/web",
      "to": "internal/usage"
    },
    {
      "from": "internal/web",
      "to": "internal/workflow"
    },
    {
      "from": "internal/web",
      "to": "internal/workflowruntime"
    },
    {
      "from": "internal/web",
      "to": "internal/workspacetabs"
    },
    {
      "from": "internal/web",
      "to": "internal/worktree"
    },
    {
      "from": "internal/workflowruntime",
      "to": "internal/delegation"
    },
    {
      "from": "internal/workflowruntime",
      "to": "internal/eviedb"
    },
    {
      "from": "internal/workflowruntime",
      "to": "internal/memory"
    },
    {
      "from": "internal/workflowruntime",
      "to": "internal/openrouter"
    },
    {
      "from": "internal/workflowruntime",
      "to": "internal/plugins"
    },
    {
      "from": "internal/workflowruntime",
      "to": "internal/procedural"
    },
    {
      "from": "internal/workflowruntime",
      "to": "internal/workflow"
    },
    {
      "from": "internal/worktree",
      "to": "internal/delegation"
    },
    {
      "from": "internal/worktree",
      "to": "internal/tools"
    }
  ],
  "commands": [],
  "routes": [],
  "databases": [],
  "glossary": [
    {
      "term": "AI Node",
      "definition": "A workflow step that uses a model to produce typed proposed data for later deterministic validation. It cannot directly perform an external effect.",
      "avoid": [
        "Agent",
        "capability call"
      ]
    },
    {
      "term": "Account Connection",
      "definition": "The established credentials and consent that allow a capability to access an external account.",
      "avoid": [
        "Plugin loading",
        "action approval"
      ]
    },
    {
      "term": "Action Approval",
      "definition": "The owner's explicit permission for a particular consequential action to execute when Standing Authority does not cover it or the Workflow Definition requires an interruption. It is independent of whether the providing plugin is loaded.",
      "avoid": [
        "Plugin approval"
      ]
    },
    {
      "term": "Agent Preset",
      "definition": "A named and versioned composition of plugins and capabilities assigned when a session is created. The session remains pinned to that exact approved version, and the preset does not grant authority to execute consequential actions.",
      "avoid": [
        "Profile",
        "session type"
      ]
    },
    {
      "term": "Alias",
      "definition": "An explicitly accepted name or identifier for an Entity that does not merge identities merely because their text is equal.",
      "avoid": [
        "Entity",
        "display name",
        "fuzzy match"
      ]
    },
    {
      "term": "Automatic Recall",
      "definition": "The selection of relevant accepted memories and attributed Conversation Excerpts for Evie's current request without requiring the owner to explicitly ask for recall.",
      "avoid": [
        "Automatic memory acceptance",
        "memory compilation"
      ]
    },
    {
      "term": "Awaiting Review",
      "definition": "A posted Transaction that has no Budget Entry. Pending Transactions are never Awaiting Review because no decision about them can be made yet.",
      "avoid": [
        "Uncategorized",
        "unmatched",
        "pending"
      ]
    },
    {
      "term": "Background Run",
      "definition": "A Workflow Run that advances independently of an active session and can report progress or outcomes back to one.",
      "avoid": [
        "Cron job",
        "detached session"
      ]
    },
    {
      "term": "Budget Entry",
      "definition": "The recorded decision that one posted Transaction counts toward a Category at a given amount, whether made by a rule or by the owner.",
      "avoid": [
        "Categorization",
        "label",
        "tag"
      ]
    },
    {
      "term": "Capability",
      "definition": "A named ability Evie can use to fulfill a request. A capability may include tools, instructions, connectors, or workflows supplied by a plugin.",
      "avoid": [
        "Plugin",
        "tool"
      ]
    },
    {
      "term": "Capability Contract",
      "definition": "The versioned behavioral and schema boundary a Capability promises to its consumers independently of the implementation version of its provider.",
      "avoid": [
        "Plugin version",
        "tool description"
      ]
    },
    {
      "term": "Capability ID",
      "definition": "The canonical plugin-namespaced identity of a Capability, used by presets and workflows without relying on display names or plugin load order.",
      "avoid": [
        "Tool label",
        "capability schema"
      ]
    },
    {
      "term": "Capability Provider",
      "definition": "A plugin role that contributes one specific family of capabilities through its own focused interface, such as tools, models, sandboxes, or subagents.",
      "avoid": [
        "Plugin",
        "universal provider"
      ]
    },
    {
      "term": "Catch-up Policy",
      "definition": "The reviewed rule for whether and how a Workflow Schedule creates runs for occurrences missed while Evie was unavailable.",
      "avoid": [
        "Retry policy",
        "run deduplication"
      ]
    },
    {
      "term": "Change",
      "definition": "One branch of work in a Workspace's Repository, followed from draft through review to merged or closed. On GitHub an open Change has a pull request.",
      "avoid": [
        "Pull request",
        "PR",
        "patch",
        "diff",
        "proposal"
      ]
    },
    {
      "term": "Claim",
      "definition": "An immutable accepted proposition relating a subject Entity to another Entity or typed literal together with scope, provenance, temporal meaning, and lifecycle history.",
      "avoid": [
        "Event",
        "objective fact",
        "graph link"
      ]
    },
    {
      "term": "Claim Polarity",
      "definition": "Whether an exact Claim proposition is affirmed or denied. Missing knowledge is unknown rather than an implicit denial.",
      "avoid": [
        "Predicate negation",
        "confidence",
        "absence"
      ]
    },
    {
      "term": "Compatibility Resolution",
      "definition": "A durable record that a newer provider implementation may continue work pinned to an older, unchanged Capability Contract, including the exact replacement version and evidence used.",
      "avoid": [
        "Silent upgrade",
        "workflow migration"
      ]
    },
    {
      "term": "Compilation Coverage",
      "definition": "The record of which selected episodic ranges a Compiler Generation has processed and which gaps remain. Its contiguous frontier does not cross an unresolved gap.",
      "avoid": [
        "Accepted memory",
        "candidate count",
        "all retained history"
      ]
    },
    {
      "term": "Compiler Generation",
      "definition": "One fixed extraction configuration under which Episodic Memory produces Memory Candidates, kept distinct from accepted Semantic Memory and other generations.",
      "avoid": [
        "Semantic revision",
        "model name",
        "extraction attempt"
      ]
    },
    {
      "term": "Composition Receipt",
      "definition": "A durable, content-free identity of the exact Agent Preset, plugins, capabilities, schemas, instructions, and non-secret configuration references a session received.",
      "avoid": [
        "Preset name",
        "credentials"
      ]
    },
    {
      "term": "Connection ID",
      "definition": "An opaque non-secret reference to one Account Connection. Presets, workflows, and receipts may contain the reference but never the credentials it represents.",
      "avoid": [
        "Access token",
        "account credential"
      ]
    },
    {
      "term": "Connection Readiness",
      "definition": "Whether the Account Connections and external resources required for a capability are currently usable.",
      "avoid": [
        "Plugin health"
      ]
    },
    {
      "term": "Connector Plugin",
      "definition": "A plugin that contributes reusable capabilities backed by a particular external system, independently of any one personal or business workflow.",
      "avoid": [
        "Feature plugin"
      ]
    },
    {
      "term": "Context Entity",
      "definition": "The canonical scope-local Entity that represents one registered Workspace, project, or session inside Semantic Memory.",
      "avoid": [
        "Context Scope",
        "duplicate project entity",
        "global promotion"
      ]
    },
    {
      "term": "Context Scope",
      "definition": "The one durable area, either a Workspace or a filesystem project, whose scoped memory and configuration a session may use in addition to global and session memory.",
      "avoid": [
        "Agent preset",
        "account connection"
      ]
    },
    {
      "term": "Conversation Excerpt",
      "definition": "An attributed portion of Episodic Memory that establishes what was recorded, without by itself establishing an accepted current Claim.",
      "avoid": [
        "Accepted memory",
        "confirmed fact"
      ]
    },
    {
      "term": "Durable Notification",
      "definition": "A persisted, deduplicated notice to the owner from any part of Evie, such as a Workflow Run that completed, failed, or needs attention. External channels may mirror it but are not its source of truth.",
      "avoid": [
        "Workflow event",
        "transient message"
      ]
    },
    {
      "term": "Effect Intent",
      "definition": "A durable declaration recorded before a Workflow Run attempts an external effect, including the stable identity needed to reconcile or deduplicate it.",
      "avoid": [
        "Effect receipt",
        "planned action"
      ]
    },
    {
      "term": "Effect Receipt",
      "definition": "Durable evidence of the accepted response or observed outcome of an external effect.",
      "avoid": [
        "Effect intent",
        "checkpoint"
      ]
    },
    {
      "term": "Enabled Plugin",
      "definition": "An installed plugin that the owner has made eligible for automatic loading.",
      "avoid": [
        "Active plugin"
      ]
    },
    {
      "term": "Entity",
      "definition": "A stable scope-aware identity for a person, organization, place, object, or concept referenced by Semantic Memory.",
      "avoid": [
        "Name",
        "alias",
        "claim"
      ]
    },
    {
      "term": "Episodic Memory",
      "definition": "The immutable chronological evidence of sessions, turns, tool activity, and their recorded outcomes.",
      "avoid": [
        "Working memory",
        "semantic memory"
      ]
    },
    {
      "term": "Execution Composition",
      "definition": "The exact capability providers, versions, Connection IDs, schemas, and non-secret configuration pinned for one Workflow Run independently of any session's Agent Preset.",
      "avoid": [
        "Agent preset",
        "standing authority"
      ]
    },
    {
      "term": "Feature Plugin",
      "definition": "A plugin that contributes the language, rules, and procedures for a particular area of the owner's life or work by composing reusable capabilities.",
      "avoid": [
        "Connector plugin",
        "workflow"
      ]
    },
    {
      "term": "First-party Plugin",
      "definition": "A plugin shipped with Evie or developed as part of the owner's local Evie environment, rather than obtained from an outside publisher.",
      "avoid": [
        "Third-party plugin"
      ]
    },
    {
      "term": "Foreground Run",
      "definition": "A Workflow Run whose progress and interaction are presented through an active session.",
      "avoid": [
        "Chat session workflow"
      ]
    },
    {
      "term": "Graph Link",
      "definition": "An explicitly accepted typed relationship between semantic objects for structure or provenance when the relationship is not an ordinary Claim.",
      "avoid": [
        "Claim",
        "inferred similarity",
        "cross-scope import"
      ]
    },
    {
      "term": "Hard Erasure",
      "definition": "Permanent coordinated removal of information and its retained representations across Evie's memory layers and projections.",
      "avoid": [
        "Retirement",
        "ordinary deletion"
      ]
    },
    {
      "term": "Kernel",
      "definition": "The non-removable part of Evie that supervises plugins and preserves the runtime's safety, authority, durability, and recovery guarantees.",
      "avoid": [
        "Core plugin"
      ]
    },
    {
      "term": "Loaded Plugin",
      "definition": "An enabled plugin whose contributions are registered with Evie's Plugin Manager. Loading does not mean its capabilities are visible to the model.",
      "avoid": [
        "Active plugin",
        "selected capability"
      ]
    },
    {
      "term": "Memory Candidate",
      "definition": "A source-linked interpretation proposed from episodic evidence that has not been accepted into Semantic Memory. It is excluded from ordinary graph traversal and retrieval until an approved rule emits a Semantic Operation.",
      "avoid": [
        "Claim",
        "remembered fact",
        "Memory Operation Proposal"
      ]
    },
    {
      "term": "Memory Capability",
      "definition": "A session-bound ability exposed through plugin composition for inspecting, querying, or explicitly changing memory without owning memory truth or scope.",
      "avoid": [
        "Memory store",
        "memory provider"
      ]
    },
    {
      "term": "Memory Inspection",
      "definition": "A read-only scoped view of entities, Claims, provenance, temporal history, and lifecycle state intended for owner understanding rather than model retrieval.",
      "avoid": [
        "Retrieval",
        "direct database access",
        "graph export"
      ]
    },
    {
      "term": "Memory Operation Proposal",
      "definition": "An unaccepted typed Semantic Memory change prepared in direct response to an explicit owner request and shown for approval before it becomes an operation.",
      "avoid": [
        "Semantic operation",
        "automatically extracted candidate"
      ]
    },
    {
      "term": "Model Policy",
      "definition": "A named and versioned mapping from an AI Node to an exact model provider, model, parameters, and output schema. A Workflow Run pins the policy version it uses.",
      "avoid": [
        "Agent preset",
        "model preference"
      ]
    },
    {
      "term": "Outcome Unknown",
      "definition": "A Workflow Run state in which an Effect Intent exists but Evie cannot yet prove whether the external effect occurred.",
      "avoid": [
        "Failed",
        "retryable"
      ]
    },
    {
      "term": "Pending Fence",
      "definition": "The boundary that keeps Pending Transactions out of categorization until the provider posts them, so a Budget Entry is only ever minted at the final amount.",
      "avoid": [
        "Pending filter",
        "skip rule"
      ]
    },
    {
      "term": "Plugin",
      "definition": "An installed extension that contributes one or more capabilities to Evie.",
      "avoid": [
        "Tool",
        "skill"
      ]
    },
    {
      "term": "Plugin Health",
      "definition": "Whether a plugin's code and required dependencies loaded and operate correctly. It is independent of whether an external account is connected.",
      "avoid": [
        "Connection readiness"
      ]
    },
    {
      "term": "Plugin Lifecycle",
      "definition": "The reversible progression through which the Plugin Manager enables, waits for, loads, operates, fails, and stops a plugin and its contributions.",
      "avoid": [
        "Workflow run"
      ]
    },
    {
      "term": "Plugin Manager",
      "definition": "The trusted part of Evie that discovers enabled plugins and manages their loading, dependencies, health, and cleanup.",
      "avoid": [
        "Agent",
        "approver"
      ]
    },
    {
      "term": "Plugin Manifest",
      "definition": "A plugin's declarative identity, version, Kernel compatibility, dependencies, and contributed Capability Contracts.",
      "avoid": [
        "Agent preset",
        "plugin configuration"
      ]
    },
    {
      "term": "Predicate",
      "definition": "The canonical validated relationship token of a Claim, kept distinct from its human-readable wording and from automatically inferred synonyms.",
      "avoid": [
        "Display label",
        "free-form relation text"
      ]
    },
    {
      "term": "Predicate Cardinality",
      "definition": "Whether a Predicate normally relates one subject to one or many simultaneous objects. It identifies potential conflicts without deleting conflicting Claims.",
      "avoid": [
        "Database uniqueness",
        "Claim Polarity"
      ]
    },
    {
      "term": "Preset Validity",
      "definition": "Whether an Agent Preset's required plugins and capabilities can be resolved. It is independent of whether external accounts are connected.",
      "avoid": [
        "Connection readiness",
        "plugin health"
      ]
    },
    {
      "term": "Procedural Memory",
      "definition": "Reviewed and versioned definitions and instructions for how Evie should work, kept separate from factual knowledge and changing execution state.",
      "avoid": [
        "Semantic memory",
        "workflow run"
      ]
    },
    {
      "term": "Procedural Workflow",
      "definition": "Reviewed repeatable behavior represented by a Workflow Definition and performed through Workflow Runs.",
      "avoid": [
        "Skill",
        "ad hoc tool sequence"
      ]
    },
    {
      "term": "Promotion",
      "definition": "An accepted Semantic Operation that creates broader-scoped knowledge linked to its narrower-scoped evidence while leaving the original knowledge unchanged.",
      "avoid": [
        "Scope mutation",
        "cross-scope import"
      ]
    },
    {
      "term": "Ready Set",
      "definition": "The Tasks of a Task Tree that can be dispatched to workers now, in batches whose members' expected paths do not conflict, sized to the worker limits. It is computed on read and schedules nothing; the orchestrator dispatches it inside its own turn.",
      "avoid": [
        "Queue",
        "scheduler",
        "workflow run"
      ]
    },
    {
      "term": "Repository",
      "definition": "The one version-controlled codebase a Workspace may own, kept only on the owner's machine or also on GitHub. A Workspace has no Repository or exactly one.",
      "avoid": [
        "Project",
        "codebase",
        "filesystem project"
      ]
    },
    {
      "term": "Resource Conflict Key",
      "definition": "A stable identity declared by an external-effect capability so Evie can prevent concurrent writes that could interfere with the same external resource.",
      "avoid": [
        "Run key",
        "effect idempotency key"
      ]
    },
    {
      "term": "Retirement",
      "definition": "A reversible lifecycle transition that excludes knowledge and its corresponding conversation evidence from ordinary recall while preserving them for explicit historical inspection.",
      "avoid": [
        "Hard erasure",
        "supersession"
      ]
    },
    {
      "term": "Retry Policy",
      "definition": "The bounded, node-specific rules governing whether and how a failed attempt may run again. Outcome Unknown is never retryable by policy.",
      "avoid": [
        "Recovery",
        "replay"
      ]
    },
    {
      "term": "Run Key",
      "definition": "A deterministic, workflow-specific identity for the logical execution that Evie uses to prevent duplicate manual or scheduled starts.",
      "avoid": [
        "Workflow run ID",
        "effect idempotency key"
      ]
    },
    {
      "term": "Scope Revision",
      "definition": "A monotonic version of accepted Semantic Memory within one scope used for ordering, concurrency checks, and reproducible projections.",
      "avoid": [
        "Workspace revision",
        "database timestamp"
      ]
    },
    {
      "term": "Semantic Memory",
      "definition": "Accepted source-linked entities, temporal claims, and relationships that represent what Evie knows without replacing their Episodic Memory evidence.",
      "avoid": [
        "Event history",
        "procedural memory",
        "objective truth"
      ]
    },
    {
      "term": "Semantic Operation",
      "definition": "An accepted append-only change that creates or transitions Semantic Memory state together with its provenance.",
      "avoid": [
        "Event",
        "extraction candidate",
        "workflow run"
      ]
    },
    {
      "term": "Skill",
      "definition": "Reviewed instructions and resources loaded into the model's working context for the model to interpret. A skill does not own durable execution state.",
      "avoid": [
        "Workflow definition"
      ]
    },
    {
      "term": "Standing Authority",
      "definition": "The bounded operations, accounts, resources, schedules, recipients, and limits that an approved Workflow Definition may exercise across runs without repeated Action Approval.",
      "avoid": [
        "Account connection",
        "unrestricted access"
      ]
    },
    {
      "term": "Subtask",
      "definition": "A Task whose parent is another Task in the same scope. Subtasks use the same model as Tasks and may themselves have Subtasks.",
      "avoid": [
        "Checklist text",
        "agent run",
        "separate subtask type"
      ]
    },
    {
      "term": "Successor Run",
      "definition": "A new Workflow Run created by an explicit migration from an unfinished older run, linked to its predecessor and carrying only validated state and durable effect evidence.",
      "avoid": [
        "Rewritten run",
        "retry"
      ]
    },
    {
      "term": "Task",
      "definition": "A durable, owner-visible unit of intended work that may be decomposed and updated by the owner or an authorized agent. Evie may create a top-level Task when durable tracking is useful; incidental agent planning is not a Task.",
      "avoid": [
        "Todo item",
        "transient agent plan",
        "workflow run"
      ]
    },
    {
      "term": "Task Access Grant",
      "definition": "Bounded authority for a session or subagent to read, contribute to, or manage one Task and all its current and future descendants, without revealing its ancestors or siblings. Effective authority is the intersection of the grant and the agent's available capabilities; Task Focus is not a grant. In the initial model, only Evie issues grants and each grant lasts for its authorized subagent session or run rather than becoming a standing access-control entry.",
      "avoid": [
        "Agent preset",
        "Task Focus",
        "prompt context"
      ]
    },
    {
      "term": "Task Claim",
      "definition": "A session-scoped indication that an agent is actively working on one Task. A claim remains valid while its owning session or run is active and is released when that execution ends or recovery finds it is no longer running. It coordinates concurrent work without replacing the agent-run record or permanently locking the Task. Progress, result, and completion updates require an active claim; child creation and metadata corrections do not.",
      "avoid": [
        "Task ownership",
        "access grant",
        "agent execution"
      ]
    },
    {
      "term": "Task Dependency",
      "definition": "A Task of the same Task Tree that must be completed, and its build merged, before another Task is dispatched. A subtask also waits for its ancestors' dependencies, so a Task never depends on its ancestor or descendant. Only the orchestrating conversation plans dependencies.",
      "avoid": [
        "Sibling order",
        "ordering hint"
      ]
    },
    {
      "term": "Task Focus",
      "definition": "The one explicitly selected Task whose relevant open descendants are supplied to a session as working context. Focus does not itself grant Task access.",
      "avoid": [
        "Entire backlog",
        "inferred task",
        "Task grant"
      ]
    },
    {
      "term": "Task Tree",
      "definition": "One top-level Task and its recursively nested Subtasks. Evie creates top-level Task Trees; authorized subagents may contribute beneath a granted root.",
      "avoid": [
        "Flat task list",
        "project",
        "workflow run"
      ]
    },
    {
      "term": "Transaction Time",
      "definition": "The time at which Evie accepted a Claim or one of its lifecycle transitions.",
      "avoid": [
        "Valid time",
        "event time"
      ]
    },
    {
      "term": "Typed Literal",
      "definition": "A non-Entity Claim object represented by one closed value kind with canonical encoding and equality rules.",
      "avoid": [
        "Arbitrary JSON",
        "floating-point value",
        "entity"
      ]
    },
    {
      "term": "Unsupported Claim",
      "definition": "An accepted Claim with no eligible active Source Link. It remains historical but is excluded from normal current retrieval until eligible support is restored.",
      "avoid": [
        "Retired claim",
        "deleted claim",
        "rejected candidate"
      ]
    },
    {
      "term": "Valid Time",
      "definition": "The interval in the represented world during which a Claim applies.",
      "avoid": [
        "Recorded time",
        "acceptance time"
      ]
    },
    {
      "term": "Workflow Approval",
      "definition": "The owner's acceptance of one exact Workflow Definition version and its requested Standing Authority for future runs.",
      "avoid": [
        "Action approval",
        "account connection"
      ]
    },
    {
      "term": "Workflow Definition",
      "definition": "An approved and versioned declarative graph describing the state, steps, and transitions of a Procedural Workflow together with its requested Standing Authority. A proposal is not a definition until the owner accepts it.",
      "avoid": [
        "Workflow run",
        "skill"
      ]
    },
    {
      "term": "Workflow Run",
      "definition": "One durable execution of a pinned Workflow Definition, including its current state, checkpoints, interruptions, attempts, and outcomes.",
      "avoid": [
        "Session",
        "workflow definition"
      ]
    },
    {
      "term": "Workflow Schedule",
      "definition": "A reviewed trigger included in a Workflow Definition that may create Workflow Runs according to its declared timing and catch-up policy.",
      "avoid": [
        "Background run",
        "cron entry"
      ]
    },
    {
      "term": "Working Memory",
      "definition": "The disposable, request-specific projection Evie assembles for a model call from selected history, summaries, instructions, and retrieved knowledge.",
      "avoid": [
        "Working context",
        "event history"
      ]
    },
    {
      "term": "Workspace",
      "definition": "A durable, owner-defined scope for an ongoing area of life or work. It groups that area's memory, Procedural Workflows, allowed Account Connections, business configuration, and default Agent Preset without pretending the area is a code project or a plugin.",
      "avoid": [
        "Filesystem project",
        "session",
        "feature plugin"
      ]
    },
    {
      "term": "Workspace Access",
      "definition": "The outer boundary of Account Connections, resources, and memory that a Workspace permits its sessions and workflows to reference. It does not itself authorize an action.",
      "avoid": [
        "Standing authority",
        "account connection"
      ]
    },
    {
      "term": "Workspace Revision",
      "definition": "One reviewed and versioned state of a Workspace's configuration and access boundary. Sessions pin the revision from which they started, subject to later safety revocations.",
      "avoid": [
        "Workflow definition",
        "memory snapshot"
      ]
    }
  ],
  "adrs": [
    {
      "number": "0001",
      "title": "Load enabled plugins automatically and select capabilities per request",
      "path": "docs/adr/0001-activate-enabled-plugins-automatically.md"
    },
    {
      "number": "0002",
      "title": "Keep supervision in the kernel",
      "path": "docs/adr/0002-keep-supervision-in-the-kernel.md"
    },
    {
      "number": "0003",
      "title": "Compose feature plugins from reusable connector plugins",
      "path": "docs/adr/0003-compose-features-from-reusable-connectors.md"
    },
    {
      "number": "0004",
      "title": "Start with compiled first-party plugins",
      "path": "docs/adr/0004-start-with-compiled-first-party-plugins.md"
    },
    {
      "number": "0005",
      "title": "Use reviewed workflows for feature operations",
      "path": "docs/adr/0005-use-reviewed-workflows-for-feature-operations.md"
    },
    {
      "number": "0006",
      "title": "Use one Agent Preset per session",
      "path": "docs/adr/0006-use-one-agent-preset-per-session.md"
    },
    {
      "number": "0007",
      "title": "Separate workflow definitions from durable execution",
      "path": "docs/adr/0007-separate-workflow-definitions-from-execution.md"
    },
    {
      "number": "0008",
      "title": "Workflow approval grants bounded standing authority",
      "path": "docs/adr/0008-workflow-approval-grants-standing-authority.md"
    },
    {
      "number": "0009",
      "title": "Reconcile uncertain workflow effects before continuing",
      "path": "docs/adr/0009-reconcile-uncertain-workflow-effects.md"
    },
    {
      "number": "0010",
      "title": "Keep AI nodes away from external effects",
      "path": "docs/adr/0010-keep-ai-nodes-away-from-effects.md"
    },
    {
      "number": "0011",
      "title": "Validate and simulate workflows before approval",
      "path": "docs/adr/0011-validate-and-simulate-workflows-before-approval.md"
    },
    {
      "number": "0012",
      "title": "Version user Agent Presets in procedural Git",
      "path": "docs/adr/0012-version-user-agent-presets-in-procedural-git.md"
    },
    {
      "number": "0013",
      "title": "Pin immutable plugin compositions per session",
      "path": "docs/adr/0013-pin-session-plugin-compositions.md"
    },
    {
      "number": "0014",
      "title": "Make plugin lifecycle reversible and dependency-aware",
      "path": "docs/adr/0014-make-plugin-lifecycle-reversible-and-dependency-aware.md"
    },
    {
      "number": "0015",
      "title": "Use focused capability-provider interfaces",
      "path": "docs/adr/0015-use-focused-capability-provider-interfaces.md"
    },
    {
      "number": "0016",
      "title": "Pin Workflow Runs to their own execution compositions",
      "path": "docs/adr/0016-pin-workflow-execution-compositions.md"
    },
    {
      "number": "0017",
      "title": "Use bounded node-specific retries and safe cancellation",
      "path": "docs/adr/0017-use-bounded-node-specific-retries-and-safe-cancellation.md"
    },
    {
      "number": "0018",
      "title": "Use closed declarative Workflow Definitions",
      "path": "docs/adr/0018-use-closed-declarative-workflow-definitions.md"
    },
    {
      "number": "0019",
      "title": "Isolate and pin AI Node context",
      "path": "docs/adr/0019-isolate-and-pin-ai-node-context.md"
    },
    {
      "number": "0020",
      "title": "Activate reviewed workflow schedules",
      "path": "docs/adr/0020-activate-reviewed-workflow-schedules.md"
    },
    {
      "number": "0021",
      "title": "Deduplicate Workflow Runs with Run Keys",
      "path": "docs/adr/0021-deduplicate-workflow-runs-with-run-keys.md"
    },
    {
      "number": "0022",
      "title": "Add Workspaces for ongoing life and business areas",
      "path": "docs/adr/0022-add-workspaces-for-ongoing-life-and-business-areas.md"
    },
    {
      "number": "0023",
      "title": "Make schedule catch-up and overlap explicit",
      "path": "docs/adr/0023-make-schedule-catch-up-explicit.md"
    },
    {
      "number": "0024",
      "title": "Distinguish credential rotation from resource changes",
      "path": "docs/adr/0024-distinguish-credential-rotation-from-resource-changes.md"
    },
    {
      "number": "0025",
      "title": "Never silently migrate pinned Workflow Runs",
      "path": "docs/adr/0025-never-silently-migrate-pinned-workflow-runs.md"
    },
    {
      "number": "0026",
      "title": "Bind each session to at most one Workspace",
      "path": "docs/adr/0026-bind-each-session-to-at-most-one-workspace.md"
    },
    {
      "number": "0027",
      "title": "Treat Workspace Access as an authority ceiling",
      "path": "docs/adr/0027-treat-workspace-access-as-an-authority-ceiling.md"
    },
    {
      "number": "0028",
      "title": "Separate Workspace configuration, secrets, and state",
      "path": "docs/adr/0028-separate-workspace-configuration-secrets-and-state.md"
    },
    {
      "number": "0029",
      "title": "Resume runs only after safe dependency recovery",
      "path": "docs/adr/0029-resume-runs-only-after-safe-dependency-recovery.md"
    },
    {
      "number": "0030",
      "title": "Migrate unfinished work with Successor Runs",
      "path": "docs/adr/0030-migrate-unfinished-work-with-successor-runs.md"
    },
    {
      "number": "0031",
      "title": "Require explicit Context Scope selection",
      "path": "docs/adr/0031-require-explicit-context-scope-selection.md"
    },
    {
      "number": "0032",
      "title": "Use one Context Scope per session",
      "path": "docs/adr/0032-use-one-context-scope-per-session.md"
    },
    {
      "number": "0033",
      "title": "Pin Workspace Revisions but apply revocations immediately",
      "path": "docs/adr/0033-pin-workspace-revisions-but-apply-revocations-immediately.md"
    },
    {
      "number": "0034",
      "title": "Isolate Context Scope memory and require explicit promotion",
      "path": "docs/adr/0034-isolate-context-memory-and-require-explicit-promotion.md"
    },
    {
      "number": "0035",
      "title": "Namespace and pin Capability IDs",
      "path": "docs/adr/0035-namespace-and-pin-capability-identities.md"
    },
    {
      "number": "0036",
      "title": "Eagerly expose preset tools and lazily load skills",
      "path": "docs/adr/0036-eagerly-expose-preset-tools-and-lazily-load-skills.md"
    },
    {
      "number": "0037",
      "title": "Declare and check plugin compatibility contracts",
      "path": "docs/adr/0037-declare-and-check-plugin-compatibility-contracts.md"
    },
    {
      "number": "0038",
      "title": "Fail invalid presets without conflating Account Connections",
      "path": "docs/adr/0038-fail-invalid-presets-without-conflating-connections.md"
    },
    {
      "number": "0039",
      "title": "Resolve capability providers explicitly",
      "path": "docs/adr/0039-resolve-capability-providers-explicitly.md"
    },
    {
      "number": "0040",
      "title": "Bound Workspaces to reviewed Agent Presets",
      "path": "docs/adr/0040-bound-workspaces-to-reviewed-agent-presets.md"
    },
    {
      "number": "0041",
      "title": "Audit compatible provider substitutions",
      "path": "docs/adr/0041-audit-compatible-provider-substitutions.md"
    },
    {
      "number": "0042",
      "title": "Share connections with Workspace-specific resource bounds",
      "path": "docs/adr/0042-share-connections-with-workspace-specific-resource-bounds.md"
    },
    {
      "number": "0043",
      "title": "Serialize conflicting external effects",
      "path": "docs/adr/0043-serialize-conflicting-external-effects.md"
    },
    {
      "number": "0044",
      "title": "Preserve proposals and record human corrections",
      "path": "docs/adr/0044-preserve-proposals-and-record-human-corrections.md"
    },
    {
      "number": "0045",
      "title": "Retain redacted workflow audit history",
      "path": "docs/adr/0045-retain-redacted-workflow-audit-history.md"
    },
    {
      "number": "0046",
      "title": "Use durable workflow notifications",
      "path": "docs/adr/0046-use-durable-workflow-notifications.md"
    },
    {
      "number": "0047",
      "title": "Keep semantic-memory truth in the Kernel",
      "path": "docs/adr/0047-keep-semantic-memory-truth-in-the-kernel.md"
    },
    {
      "number": "0048",
      "title": "Replay Semantic Memory from accepted operations",
      "path": "docs/adr/0048-replay-semantic-memory-from-accepted-operations.md"
    },
    {
      "number": "0049",
      "title": "Separate semantic state, extraction, and retrieval",
      "path": "docs/adr/0049-separate-semantic-state-extraction-and-retrieval.md"
    },
    {
      "number": "0050",
      "title": "Require explicit approval for model-structured semantic changes",
      "path": "docs/adr/0050-require-explicit-semantic-change-approval.md"
    },
    {
      "number": "0051",
      "title": "Share one semantic interface across user surfaces",
      "path": "docs/adr/0051-share-one-semantic-interface-across-user-surfaces.md"
    },
    {
      "number": "0052",
      "title": "Use canonical semantic identities and values",
      "path": "docs/adr/0052-use-canonical-semantic-identities-and-values.md"
    },
    {
      "number": "0053",
      "title": "Evaluate memory as separate systems",
      "path": "docs/adr/0053-evaluate-memory-as-separate-systems.md"
    },
    {
      "number": "0054",
      "title": "Keep accepted semantic state model-independent",
      "path": "docs/adr/0054-keep-accepted-semantic-state-model-independent.md"
    },
    {
      "number": "0055",
      "title": "Separate world Claims from structural Graph Links",
      "path": "docs/adr/0055-separate-world-claims-from-structural-links.md"
    },
    {
      "number": "0056",
      "title": "Preserve semantic history through lifecycle changes",
      "path": "docs/adr/0056-preserve-semantic-history-through-lifecycle-changes.md"
    },
    {
      "number": "0057",
      "title": "Separate negation from Predicate cardinality",
      "path": "docs/adr/0057-separate-negation-from-predicate-cardinality.md"
    },
    {
      "number": "0058",
      "title": "Version global Predicate definitions",
      "path": "docs/adr/0058-version-global-predicate-definitions.md"
    },
    {
      "number": "0059",
      "title": "Pin exact memory reads to Scope Revisions",
      "path": "docs/adr/0059-pin-exact-memory-reads-to-scope-revisions.md"
    },
    {
      "number": "0060",
      "title": "Quarantine and shadow-rebuild semantic projections",
      "path": "docs/adr/0060-quarantine-and-shadow-rebuild-semantic-projections.md"
    },
    {
      "number": "0061",
      "title": "Use canonical anchors and atomic sourced operations",
      "path": "docs/adr/0061-use-canonical-anchors-and-atomic-sourced-operations.md"
    },
    {
      "number": "0062",
      "title": "Distinguish candidate support from interpretation context",
      "path": "docs/adr/0062-distinguish-candidate-support-from-interpretation-context.md"
    },
    {
      "number": "0063",
      "title": "Separate candidate progress from contiguous coverage",
      "path": "docs/adr/0063-separate-candidate-progress-from-contiguous-coverage.md"
    },
    {
      "number": "0064",
      "title": "Let owner candidate review outlive source sessions",
      "path": "docs/adr/0064-let-owner-candidate-review-outlive-source-sessions.md"
    },
    {
      "number": "0065",
      "title": "Separate compiler activation from historical backfill",
      "path": "docs/adr/0065-separate-compiler-activation-from-historical-backfill.md"
    },
    {
      "number": "0066",
      "title": "Allow scoped recall across conversations",
      "path": "docs/adr/0066-allow-conversation-recall-within-one-context-scope.md"
    },
    {
      "number": "0067",
      "title": "Run the daily finance job as a scheduled workflow, not a cron job",
      "path": "docs/adr/0067-run-finance-daily-as-a-scheduled-workflow.md"
    }
  ]
}
);
