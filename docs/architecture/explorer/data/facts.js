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
      "path": "docs/architecture/explorer",
      "name": "explorer",
      "command": false,
      "doc": "Package explorer carries the system map page into the evie binary: the page the vision skill builds from .vision/map.json, exactly as it was when the binary was built (system-page.spec.md).",
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
      "doc": "Package memorybackground is the background memory pass (docs/memory-architecture.md, Stage 4 and decision 4).",
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
      "path": "internal/skill",
      "name": "skill",
      "command": false,
      "doc": "Package skill checks one skill in the Agent Skills format against Evie's rules (cmd/evie/docs/active/skills.spec.md): a SKILL.md with YAML frontmatter and Markdown instructions, plus optional text reference files, with no scripts, executables or links, only UTF-8 text, and within Evie's size limits.",
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
      "to": "internal/skill"
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
      "to": "internal/skill"
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
      "to": "internal/skill"
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
      "to": "internal/skill"
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
      "to": "docs/architecture/explorer"
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
  "commands": [
    {
      "binary": "evie",
      "words": [
        "changes"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/changes.go",
        "function": "runChangeManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "changes",
        "close"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/changes.go",
        "function": "runChangeManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "changes",
        "disarm"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/changes.go",
        "function": "runChangeManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "changes",
        "list"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/changes.go",
        "function": "runChangeManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "changes",
        "merge"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/changes.go",
        "function": "runChangeManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "changes",
        "push"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/changes.go",
        "function": "runChangeManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "connections"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/connections.go",
        "function": "runConnectionManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "connections",
        "add"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/connections.go",
        "function": "runConnectionManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "connections",
        "connect"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/connections.go",
        "function": "runConnectionManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "connections",
        "disconnect"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/connections.go",
        "function": "runConnectionManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "connections",
        "list"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/connections.go",
        "function": "runConnectionManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "cron-exec"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/main.go",
        "function": "main"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-backfill"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/compiler_history.go",
        "function": "runCompilerHistoryManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-backfill",
        "cancel"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/compiler_history.go",
        "function": "runCompilerHistoryManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-backfill",
        "resume"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/compiler_history.go",
        "function": "runCompilerHistoryManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-backfill",
        "select"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/compiler_history.go",
        "function": "runCompilerHistoryManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-backfill",
        "status"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/compiler_history.go",
        "function": "runCompilerHistoryManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-background"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/memory_background.go",
        "function": "runMemoryBackgroundManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-background",
        "status"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/memory_background.go",
        "function": "runMemoryBackgroundManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-candidates"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/compiler_management.go",
        "function": "runCompilerManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-candidates",
        "cancel"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/compiler_management.go",
        "function": "runCompilerManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-candidates",
        "inspect"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/compiler_management.go",
        "function": "runCompilerManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-candidates",
        "resume"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/compiler_management.go",
        "function": "runCompilerManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-candidates",
        "status"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/compiler_management.go",
        "function": "runCompilerManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-compile"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/compiler_management.go",
        "function": "runCompilerManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-compiler"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/compiler_activation.go",
        "function": "runCompilerActivationManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-compiler",
        "activate"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/compiler_activation.go",
        "function": "runCompilerActivationManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-compiler",
        "disable"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/compiler_activation.go",
        "function": "runCompilerActivationManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-compiler",
        "resume"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/compiler_activation.go",
        "function": "runCompilerActivationManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-compiler",
        "status"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/compiler_activation.go",
        "function": "runCompilerActivationManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-health"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/compiler_diagnostics.go",
        "function": "runCompilerDiagnostics"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-review"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/candidate_review.go",
        "function": "runOwnerReviewManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-review",
        "alternatives"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/candidate_review.go",
        "function": "runOwnerReviewManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-review",
        "batch-inspect"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/candidate_batch.go",
        "function": "runOwnerBatchReviewManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-review",
        "batch-prepare"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/candidate_batch.go",
        "function": "runOwnerBatchReviewManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-review",
        "batch-resolve"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/candidate_batch.go",
        "function": "runOwnerBatchReviewManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-review",
        "choose"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/candidate_review.go",
        "function": "runOwnerReviewManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-review",
        "edit"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/candidate_batch.go",
        "function": "runOwnerBatchReviewManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-review",
        "edit-revision"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/candidate_batch.go",
        "function": "runOwnerBatchReviewManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-review",
        "identity-revision"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/candidate_review.go",
        "function": "runOwnerReviewManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-review",
        "inbox"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/candidate_review.go",
        "function": "runOwnerReviewManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-review",
        "inspect"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/candidate_review.go",
        "function": "runOwnerReviewManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-review",
        "lineage"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/candidate_review.go",
        "function": "runOwnerReviewManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-review",
        "operation"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/candidate_review.go",
        "function": "runOwnerReviewManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-review",
        "prepare"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/candidate_review.go",
        "function": "runOwnerReviewManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-review",
        "resolve"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/candidate_review.go",
        "function": "runOwnerReviewManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-review",
        "temporal-choose"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/candidate_review.go",
        "function": "runOwnerReviewManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-review",
        "temporal-options"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/candidate_review.go",
        "function": "runOwnerReviewManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "memory-review",
        "temporal-revision"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/candidate_review.go",
        "function": "runOwnerReviewManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "plugins"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/main.go",
        "function": "main"
      }
    },
    {
      "binary": "evie",
      "words": [
        "plugins",
        "disable"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/management.go",
        "function": "runManagementCommand"
      }
    },
    {
      "binary": "evie",
      "words": [
        "plugins",
        "enable"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/management.go",
        "function": "runManagementCommand"
      }
    },
    {
      "binary": "evie",
      "words": [
        "plugins",
        "list"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/management.go",
        "function": "runManagementCommand"
      }
    },
    {
      "binary": "evie",
      "words": [
        "presets"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/main.go",
        "function": "main"
      }
    },
    {
      "binary": "evie",
      "words": [
        "presets",
        "list"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/management.go",
        "function": "runManagementCommand"
      }
    },
    {
      "binary": "evie",
      "words": [
        "presets",
        "validate"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/management.go",
        "function": "runManagementCommand"
      }
    },
    {
      "binary": "evie",
      "words": [
        "projects"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/build_verify.go",
        "function": "runProjectManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "projects",
        "list"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/build_verify.go",
        "function": "runProjectManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "projects",
        "verify"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/build_verify.go",
        "function": "runProjectManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "serve"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/main.go",
        "function": "main"
      }
    },
    {
      "binary": "evie",
      "words": [
        "sessions"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/main.go",
        "function": "main"
      }
    },
    {
      "binary": "evie",
      "words": [
        "sessions",
        "inspect"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/management.go",
        "function": "runManagementCommand"
      }
    },
    {
      "binary": "evie",
      "words": [
        "skills"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/main.go",
        "function": "main"
      }
    },
    {
      "binary": "evie",
      "words": [
        "skills",
        "approve"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/skills.go",
        "function": "runSkillManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "skills",
        "deactivate"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/skills.go",
        "function": "runSkillManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "skills",
        "import-claude"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/skills.go",
        "function": "runSkillManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "skills",
        "list"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/skills.go",
        "function": "runSkillManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "skills",
        "propose"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/skills.go",
        "function": "runSkillManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "skills",
        "show"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/skills.go",
        "function": "runSkillManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/main.go",
        "function": "main"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "answer"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_activation.go",
        "function": "runWorkflowActivation"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "approve"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_activation.go",
        "function": "runWorkflowActivation"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "builtin"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_activation.go",
        "function": "runWorkflowActivation"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "builtin",
        "disable"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_builtins.go",
        "function": "workflowBuiltinCommand"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "builtin",
        "enable"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_builtins.go",
        "function": "workflowBuiltinCommand"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "builtin",
        "list"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_builtins.go",
        "function": "workflowBuiltinCommand"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "cancel"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_activation.go",
        "function": "runWorkflowActivation"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "customize"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_activation.go",
        "function": "runWorkflowActivation"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "deactivate"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_activation.go",
        "function": "runWorkflowActivation"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "diff"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_activation.go",
        "function": "runWorkflowActivation"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "global"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_activation.go",
        "function": "runWorkflowActivation"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "global",
        "deactivate"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_builtins.go",
        "function": "globalWorkflowCommand"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "global",
        "list"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_builtins.go",
        "function": "globalWorkflowCommand"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "global",
        "show"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_builtins.go",
        "function": "globalWorkflowCommand"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "inspect"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_activation.go",
        "function": "runWorkflowActivation"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "interruptions"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_activation.go",
        "function": "runWorkflowActivation"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "list"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_activation.go",
        "function": "runWorkflowActivation"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "migrate"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_activation.go",
        "function": "runWorkflowActivation"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "notifications"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_activation.go",
        "function": "runWorkflowActivation"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "propose"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_activation.go",
        "function": "runWorkflowActivation"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "reconcile"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_activation.go",
        "function": "runWorkflowActivation"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "report"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_activation.go",
        "function": "runWorkflowActivation"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "retention"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_activation.go",
        "function": "runWorkflowActivation"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "retention",
        "status"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_migrations.go",
        "function": "workflowRetentionCommand"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "retention",
        "sweep"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_migrations.go",
        "function": "workflowRetentionCommand"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "run"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_activation.go",
        "function": "runWorkflowActivation"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "runs"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_activation.go",
        "function": "runWorkflowActivation"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "schedule"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_activation.go",
        "function": "runWorkflowActivation"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "schedule",
        "pause"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_schedules.go",
        "function": "controlWorkflowSchedule"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "schedule",
        "resume"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_schedules.go",
        "function": "controlWorkflowSchedule"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "schedules"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_activation.go",
        "function": "runWorkflowActivation"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "show"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_activation.go",
        "function": "runWorkflowActivation"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "simulate"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflows.go",
        "function": "runWorkflowManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "status"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_activation.go",
        "function": "runWorkflowActivation"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "substitute"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflow_activation.go",
        "function": "runWorkflowActivation"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "templates"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflows.go",
        "function": "runWorkflowManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workflows",
        "validate"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workflows.go",
        "function": "runWorkflowManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workspaces"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/connections.go",
        "function": "runConnectionManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workspaces",
        "access"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workspace_explore.go",
        "function": "runWorkspaceManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workspaces",
        "build"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workspace_explore.go",
        "function": "runWorkspaceManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workspaces",
        "explore"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workspace_explore.go",
        "function": "runWorkspaceManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workspaces",
        "list"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workspace_explore.go",
        "function": "runWorkspaceManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workspaces",
        "mcp-tools"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/connections.go",
        "function": "runConnectionManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "workspaces",
        "verify"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/workspace_explore.go",
        "function": "runWorkspaceManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "worktrees"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/worktrees.go",
        "function": "runWorktreeManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "worktrees",
        "clean"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/worktrees.go",
        "function": "runWorktreeManagement"
      }
    },
    {
      "binary": "evie",
      "words": [
        "worktrees",
        "list"
      ],
      "package": "cmd/evie",
      "source": {
        "file": "cmd/evie/worktrees.go",
        "function": "runWorktreeManagement"
      }
    },
    {
      "binary": "finance",
      "words": [
        "categorize"
      ],
      "package": "cmd/finance",
      "source": {
        "file": "cmd/finance/main.go",
        "function": "main"
      }
    },
    {
      "binary": "finance",
      "words": [
        "db"
      ],
      "package": "cmd/finance",
      "source": {
        "file": "cmd/finance/main.go",
        "function": "main"
      }
    },
    {
      "binary": "finance",
      "words": [
        "help"
      ],
      "package": "cmd/finance",
      "source": {
        "file": "cmd/finance/main.go",
        "function": "main"
      }
    },
    {
      "binary": "finance",
      "words": [
        "link"
      ],
      "package": "cmd/finance",
      "source": {
        "file": "cmd/finance/main.go",
        "function": "main"
      }
    },
    {
      "binary": "finance",
      "words": [
        "rules"
      ],
      "package": "cmd/finance",
      "source": {
        "file": "cmd/finance/main.go",
        "function": "main"
      }
    },
    {
      "binary": "finance",
      "words": [
        "sync"
      ],
      "package": "cmd/finance",
      "source": {
        "file": "cmd/finance/main.go",
        "function": "main"
      }
    },
    {
      "binary": "todo",
      "words": [
        "add"
      ],
      "package": "cmd/todo",
      "source": {
        "file": "cmd/todo/main.go",
        "function": "run"
      }
    },
    {
      "binary": "todo",
      "words": [
        "block"
      ],
      "package": "cmd/todo",
      "source": {
        "file": "cmd/todo/main.go",
        "function": "run"
      }
    },
    {
      "binary": "todo",
      "words": [
        "cancel"
      ],
      "package": "cmd/todo",
      "source": {
        "file": "cmd/todo/main.go",
        "function": "run"
      }
    },
    {
      "binary": "todo",
      "words": [
        "delete"
      ],
      "package": "cmd/todo",
      "source": {
        "file": "cmd/todo/main.go",
        "function": "run"
      }
    },
    {
      "binary": "todo",
      "words": [
        "done"
      ],
      "package": "cmd/todo",
      "source": {
        "file": "cmd/todo/main.go",
        "function": "run"
      }
    },
    {
      "binary": "todo",
      "words": [
        "get"
      ],
      "package": "cmd/todo",
      "source": {
        "file": "cmd/todo/main.go",
        "function": "run"
      }
    },
    {
      "binary": "todo",
      "words": [
        "help"
      ],
      "package": "cmd/todo",
      "source": {
        "file": "cmd/todo/main.go",
        "function": "run"
      }
    },
    {
      "binary": "todo",
      "words": [
        "list"
      ],
      "package": "cmd/todo",
      "source": {
        "file": "cmd/todo/main.go",
        "function": "run"
      }
    },
    {
      "binary": "todo",
      "words": [
        "release-claim"
      ],
      "package": "cmd/todo",
      "source": {
        "file": "cmd/todo/main.go",
        "function": "run"
      }
    },
    {
      "binary": "todo",
      "words": [
        "reopen"
      ],
      "package": "cmd/todo",
      "source": {
        "file": "cmd/todo/main.go",
        "function": "run"
      }
    },
    {
      "binary": "todo",
      "words": [
        "show"
      ],
      "package": "cmd/todo",
      "source": {
        "file": "cmd/todo/main.go",
        "function": "run"
      }
    },
    {
      "binary": "todo",
      "words": [
        "start"
      ],
      "package": "cmd/todo",
      "source": {
        "file": "cmd/todo/main.go",
        "function": "run"
      }
    },
    {
      "binary": "todo",
      "words": [
        "update"
      ],
      "package": "cmd/todo",
      "source": {
        "file": "cmd/todo/main.go",
        "function": "run"
      }
    },
    {
      "binary": "ytscribe",
      "words": [
        "fetch"
      ],
      "package": "cmd/ytscribe",
      "source": {
        "file": "cmd/ytscribe/main.go",
        "function": "runYTScribe"
      }
    },
    {
      "binary": "ytscribe",
      "words": [
        "help"
      ],
      "package": "cmd/ytscribe",
      "source": {
        "file": "cmd/ytscribe/main.go",
        "function": "runYTScribe"
      }
    },
    {
      "binary": "ytscribe",
      "words": [
        "import"
      ],
      "package": "cmd/ytscribe",
      "source": {
        "file": "cmd/ytscribe/main.go",
        "function": "runYTScribe"
      }
    },
    {
      "binary": "ytscribe",
      "words": [
        "scrape"
      ],
      "package": "cmd/ytscribe",
      "source": {
        "file": "cmd/ytscribe/main.go",
        "function": "runYTScribe"
      }
    }
  ],
  "routes": [
    {
      "pattern": "/",
      "package": "internal/web",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "method": "POST",
      "pattern": "/api/approve",
      "package": "internal/web",
      "guard": "guard",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/cancel",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/changes/close",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/changes_merge.go",
        "function": "Server.registerChangeMergeRoutes"
      }
    },
    {
      "pattern": "/api/changes/disarm",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/changes_merge.go",
        "function": "Server.registerChangeMergeRoutes"
      }
    },
    {
      "pattern": "/api/changes/list",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/changes.go",
        "function": "Server.registerChangeRoutes"
      }
    },
    {
      "pattern": "/api/changes/merge",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/changes_merge.go",
        "function": "Server.registerChangeMergeRoutes"
      }
    },
    {
      "pattern": "/api/changes/push",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/changes_push.go",
        "function": "Server.registerChangePushRoutes"
      }
    },
    {
      "pattern": "/api/changes/refresh",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/changes_sync.go",
        "function": "Server.registerChangeSyncRoutes"
      }
    },
    {
      "pattern": "/api/changes/show",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/changes.go",
        "function": "Server.registerChangeRoutes"
      }
    },
    {
      "method": "POST",
      "pattern": "/api/chat",
      "package": "internal/web",
      "guard": "guard",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/chats/follow",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/chats/status",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/compact",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/connections/add",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/connections.go",
        "function": "Server.registerConnectionRoutes"
      }
    },
    {
      "pattern": "/api/connections/connect",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/connections.go",
        "function": "Server.registerConnectionRoutes"
      }
    },
    {
      "pattern": "/api/connections/disconnect",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/connections.go",
        "function": "Server.registerConnectionRoutes"
      }
    },
    {
      "pattern": "/api/connections/list",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/connections.go",
        "function": "Server.registerConnectionRoutes"
      }
    },
    {
      "pattern": "/api/connections/oauth/callback",
      "package": "internal/web",
      "source": {
        "file": "internal/web/connections.go",
        "function": "Server.registerConnectionRoutes"
      }
    },
    {
      "pattern": "/api/connections/tools",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/connection_tools.go",
        "function": "Server.registerConnectionToolRoutes"
      }
    },
    {
      "pattern": "/api/connections/tools/policy",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/connection_tools.go",
        "function": "Server.registerConnectionToolRoutes"
      }
    },
    {
      "pattern": "/api/connections/tools/set",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/connection_tools.go",
        "function": "Server.registerConnectionToolRoutes"
      }
    },
    {
      "pattern": "/api/connections/wait",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/connections.go",
        "function": "Server.registerConnectionRoutes"
      }
    },
    {
      "pattern": "/api/context-sessions/archive",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/context-sessions/history",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/context-sessions/list",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/context-sessions/restore",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/context-sessions/select",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/data/database/rows",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/data/database/schema",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/data/spending/accounts",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/spending_accounts.go",
        "function": "Server.registerSpendingAccountRoutes"
      }
    },
    {
      "pattern": "/api/data/spending/accounts/refresh",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/spending_accounts.go",
        "function": "Server.registerSpendingAccountRoutes"
      }
    },
    {
      "pattern": "/api/data/spending/cash-flow",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/data/spending/category",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/data/spending/day",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/data/spending/link/cancel",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/spending_accounts.go",
        "function": "Server.registerSpendingAccountRoutes"
      }
    },
    {
      "pattern": "/api/data/spending/link/start",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/spending_accounts.go",
        "function": "Server.registerSpendingAccountRoutes"
      }
    },
    {
      "pattern": "/api/data/spending/link/status",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/spending_accounts.go",
        "function": "Server.registerSpendingAccountRoutes"
      }
    },
    {
      "pattern": "/api/data/spending/refresh",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/data/spending/summary",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/data/spending/transactions",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/data/usage/summary",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/folder/changes",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/local_folder.go",
        "function": "Server.registerFolderRoutes"
      }
    },
    {
      "pattern": "/api/folder/diff",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/local_folder.go",
        "function": "Server.registerFolderRoutes"
      }
    },
    {
      "pattern": "/api/folder/list",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/local_folder.go",
        "function": "Server.registerFolderRoutes"
      }
    },
    {
      "pattern": "/api/folder/read",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/local_folder.go",
        "function": "Server.registerFolderRoutes"
      }
    },
    {
      "pattern": "/api/github/repositories",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/repository.go",
        "function": "Server.registerGitHubRoutes"
      }
    },
    {
      "pattern": "/api/github/status",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/repository.go",
        "function": "Server.registerGitHubRoutes"
      }
    },
    {
      "pattern": "/api/memory/background/status",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/memory_background.go",
        "function": "Server.registerMemoryBackgroundRoutes"
      }
    },
    {
      "pattern": "/api/memory/candidates/batch/inspect",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/candidate_review_advanced.go",
        "function": "Server.registerAdvancedCandidateReviewRoutes"
      }
    },
    {
      "pattern": "/api/memory/candidates/batch/prepare",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/candidate_review_advanced.go",
        "function": "Server.registerAdvancedCandidateReviewRoutes"
      }
    },
    {
      "pattern": "/api/memory/candidates/batch/resolve",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/candidate_review_advanced.go",
        "function": "Server.registerAdvancedCandidateReviewRoutes"
      }
    },
    {
      "pattern": "/api/memory/candidates/edit",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/candidate_review_advanced.go",
        "function": "Server.registerAdvancedCandidateReviewRoutes"
      }
    },
    {
      "pattern": "/api/memory/candidates/edit/revision",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/candidate_review_advanced.go",
        "function": "Server.registerAdvancedCandidateReviewRoutes"
      }
    },
    {
      "pattern": "/api/memory/candidates/identity/choose",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/candidate_review_advanced.go",
        "function": "Server.registerAdvancedCandidateReviewRoutes"
      }
    },
    {
      "pattern": "/api/memory/candidates/identity/options",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/candidate_review_advanced.go",
        "function": "Server.registerAdvancedCandidateReviewRoutes"
      }
    },
    {
      "pattern": "/api/memory/candidates/identity/revision",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/candidate_review_advanced.go",
        "function": "Server.registerAdvancedCandidateReviewRoutes"
      }
    },
    {
      "pattern": "/api/memory/candidates/inspect",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/candidate_review.go",
        "function": "Server.registerCandidateReviewRoutes"
      }
    },
    {
      "pattern": "/api/memory/candidates/list",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/candidate_review.go",
        "function": "Server.registerCandidateReviewRoutes"
      }
    },
    {
      "pattern": "/api/memory/candidates/operation",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/candidate_review.go",
        "function": "Server.registerCandidateReviewRoutes"
      }
    },
    {
      "pattern": "/api/memory/candidates/prepare",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/candidate_review.go",
        "function": "Server.registerCandidateReviewRoutes"
      }
    },
    {
      "pattern": "/api/memory/candidates/resolve",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/candidate_review.go",
        "function": "Server.registerCandidateReviewRoutes"
      }
    },
    {
      "pattern": "/api/memory/candidates/scopes",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/candidate_review.go",
        "function": "Server.registerCandidateReviewRoutes"
      }
    },
    {
      "pattern": "/api/memory/candidates/temporal/choose",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/candidate_review_advanced.go",
        "function": "Server.registerAdvancedCandidateReviewRoutes"
      }
    },
    {
      "pattern": "/api/memory/candidates/temporal/options",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/candidate_review_advanced.go",
        "function": "Server.registerAdvancedCandidateReviewRoutes"
      }
    },
    {
      "pattern": "/api/memory/candidates/temporal/revision",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/candidate_review_advanced.go",
        "function": "Server.registerAdvancedCandidateReviewRoutes"
      }
    },
    {
      "pattern": "/api/memory/compiler/diagnostics",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/compiler_diagnostics.go",
        "function": "Server.registerCompilerDiagnosticRoutes"
      }
    },
    {
      "pattern": "/api/memory/compiler/sessions",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/compiler_diagnostics.go",
        "function": "Server.registerCompilerDiagnosticRoutes"
      }
    },
    {
      "pattern": "/api/memory/evidence",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/memory/inspect",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/memory/objects",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/memory/proposals/approve",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/memory_proposals.go",
        "function": "Server.registerMemoryProposalRoutes"
      }
    },
    {
      "pattern": "/api/memory/proposals/count",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/memory_proposals.go",
        "function": "Server.registerMemoryProposalRoutes"
      }
    },
    {
      "pattern": "/api/memory/proposals/decline",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/memory_proposals.go",
        "function": "Server.registerMemoryProposalRoutes"
      }
    },
    {
      "pattern": "/api/memory/proposals/list",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/memory_proposals.go",
        "function": "Server.registerMemoryProposalRoutes"
      }
    },
    {
      "pattern": "/api/memory/scopes",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/models/list",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/models/select",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/notifications",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/notifications.go",
        "function": "Server.registerNotificationRoutes"
      }
    },
    {
      "pattern": "/api/notifications/read",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/notifications.go",
        "function": "Server.registerNotificationRoutes"
      }
    },
    {
      "pattern": "/api/plugins/lifecycle",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/plugins/list",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/presets/list",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/presets/validate",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/repository-instructions/preview",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/repository_instructions.go",
        "function": "Server.registerRepositoryInstructionRoutes"
      }
    },
    {
      "pattern": "/api/repository-instructions/settings",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/repository_instructions.go",
        "function": "Server.registerRepositoryInstructionRoutes"
      }
    },
    {
      "pattern": "/api/repository-instructions/snapshot",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/repository_instructions.go",
        "function": "Server.registerRepositoryInstructionRoutes"
      }
    },
    {
      "pattern": "/api/sessions/inspect",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/skills/list",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/subagents/list",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/subagents.go",
        "function": "Server.registerSubagentRoutes"
      }
    },
    {
      "pattern": "/api/subagents/report",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/subagents.go",
        "function": "Server.registerSubagentRoutes"
      }
    },
    {
      "pattern": "/api/subagents/trace",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/subagents.go",
        "function": "Server.registerSubagentRoutes"
      }
    },
    {
      "pattern": "/api/terminal/close",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/terminal.go",
        "function": "Server.registerTerminalRoutes"
      }
    },
    {
      "pattern": "/api/terminal/input",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/terminal.go",
        "function": "Server.registerTerminalRoutes"
      }
    },
    {
      "pattern": "/api/terminal/open",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/terminal.go",
        "function": "Server.registerTerminalRoutes"
      }
    },
    {
      "pattern": "/api/terminal/resize",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/terminal.go",
        "function": "Server.registerTerminalRoutes"
      }
    },
    {
      "pattern": "/api/workflows/answer",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workflows.go",
        "function": "Server.registerWorkflowRoutes"
      }
    },
    {
      "pattern": "/api/workflows/approve",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workflows.go",
        "function": "Server.registerWorkflowRoutes"
      }
    },
    {
      "pattern": "/api/workflows/builtins",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workflows.go",
        "function": "Server.registerWorkflowRoutes"
      }
    },
    {
      "pattern": "/api/workflows/builtins/control",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workflows.go",
        "function": "Server.registerWorkflowRoutes"
      }
    },
    {
      "pattern": "/api/workflows/cancel",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workflows.go",
        "function": "Server.registerWorkflowRoutes"
      }
    },
    {
      "pattern": "/api/workflows/deactivate",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workflows.go",
        "function": "Server.registerWorkflowRoutes"
      }
    },
    {
      "pattern": "/api/workflows/definition",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workflows.go",
        "function": "Server.registerWorkflowRoutes"
      }
    },
    {
      "pattern": "/api/workflows/diff",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workflows.go",
        "function": "Server.registerWorkflowRoutes"
      }
    },
    {
      "pattern": "/api/workflows/inspect",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workflows.go",
        "function": "Server.registerWorkflowRoutes"
      }
    },
    {
      "pattern": "/api/workflows/interruptions",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workflows.go",
        "function": "Server.registerWorkflowRoutes"
      }
    },
    {
      "pattern": "/api/workflows/list",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workflows.go",
        "function": "Server.registerWorkflowRoutes"
      }
    },
    {
      "pattern": "/api/workflows/migrate",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workflows.go",
        "function": "Server.registerWorkflowRoutes"
      }
    },
    {
      "pattern": "/api/workflows/notifications",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workflows.go",
        "function": "Server.registerWorkflowRoutes"
      }
    },
    {
      "pattern": "/api/workflows/notifications/read",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workflows.go",
        "function": "Server.registerWorkflowRoutes"
      }
    },
    {
      "pattern": "/api/workflows/reconcile",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workflows.go",
        "function": "Server.registerWorkflowRoutes"
      }
    },
    {
      "pattern": "/api/workflows/retention",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workflows.go",
        "function": "Server.registerWorkflowRoutes"
      }
    },
    {
      "pattern": "/api/workflows/retention/sweep",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workflows.go",
        "function": "Server.registerWorkflowRoutes"
      }
    },
    {
      "pattern": "/api/workflows/runs",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workflows.go",
        "function": "Server.registerWorkflowRoutes"
      }
    },
    {
      "pattern": "/api/workflows/schedule",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workflows.go",
        "function": "Server.registerWorkflowRoutes"
      }
    },
    {
      "pattern": "/api/workflows/schedules",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workflows.go",
        "function": "Server.registerWorkflowRoutes"
      }
    },
    {
      "pattern": "/api/workflows/show",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workflows.go",
        "function": "Server.registerWorkflowRoutes"
      }
    },
    {
      "pattern": "/api/workflows/start",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workflows.go",
        "function": "Server.registerWorkflowRoutes"
      }
    },
    {
      "pattern": "/api/workflows/status",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workflows.go",
        "function": "Server.registerWorkflowRoutes"
      }
    },
    {
      "pattern": "/api/workspaces/archive",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/workspaces/build",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/workspaces/choose-folder",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/workspaces/explore",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/workspaces/folder",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/local_folder.go",
        "function": "Server.registerFolderRoutes"
      }
    },
    {
      "pattern": "/api/workspaces/register",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/workspaces/repository",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/repository.go",
        "function": "Server.registerRepositoryRoutes"
      }
    },
    {
      "pattern": "/api/workspaces/repository/publish",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/repository.go",
        "function": "Server.registerRepositoryRoutes"
      }
    },
    {
      "pattern": "/api/workspaces/research",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/workspaces/restore",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/workspaces/tabs",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workspace_tabs.go",
        "function": "Server.registerWorkspaceTabRoutes"
      }
    },
    {
      "pattern": "/api/workspaces/tabs/approve",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workspace_tabs.go",
        "function": "Server.registerWorkspaceTabDataRoutes"
      }
    },
    {
      "pattern": "/api/workspaces/tabs/data",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workspace_tabs.go",
        "function": "Server.registerWorkspaceTabDataRoutes"
      }
    },
    {
      "pattern": "/api/workspaces/tabs/problems",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workspace_tabs.go",
        "function": "Server.registerWorkspaceTabProblemRoutes"
      }
    },
    {
      "pattern": "/api/workspaces/tabs/withdraw",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/workspace_tabs.go",
        "function": "Server.registerWorkspaceTabDataRoutes"
      }
    },
    {
      "pattern": "/api/workspaces/verify",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/api/workspaces/verify/propose",
      "package": "internal/web",
      "guard": "managementRoute",
      "source": {
        "file": "internal/web/serve.go",
        "function": "Server.Handler"
      }
    },
    {
      "pattern": "/explorer",
      "package": "internal/web",
      "source": {
        "file": "internal/web/explorer.go",
        "function": "Server.registerExplorerRoutes"
      }
    },
    {
      "pattern": "/explorer/",
      "package": "internal/web",
      "source": {
        "file": "internal/web/explorer.go",
        "function": "Server.registerExplorerRoutes"
      }
    },
    {
      "method": "GET",
      "pattern": "/workspace-tabs/{workspace}/{tab}",
      "package": "internal/web",
      "source": {
        "file": "internal/web/workspace_tabs.go",
        "function": "Server.registerWorkspaceTabRoutes"
      }
    },
    {
      "method": "GET",
      "pattern": "/workspace-tabs/{workspace}/{tab}/{path...}",
      "package": "internal/web",
      "source": {
        "file": "internal/web/workspace_tabs.go",
        "function": "Server.registerWorkspaceTabRoutes"
      }
    }
  ],
  "databases": [
    {
      "id": "evie",
      "package": "internal/eviedb",
      "tables": [
        {
          "name": "build_verify_revisions",
          "columns": [
            {
              "name": "scope_kind",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "scope_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 3
            },
            {
              "name": "setup_command",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "verify_command",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "change_merge_arms",
          "columns": [
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "number",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "armed_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "workspace_id"
              ],
              "table": "workspaces",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "change_pull_requests",
          "columns": [
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "number",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "owner",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "repository",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "branch",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "base",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "title",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "body",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "author",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "state",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "draft",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "head_sha",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "mergeable",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "url",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "checks_json",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "github_updated_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "synced_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "workspace_id"
              ],
              "table": "workspaces",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "events",
          "columns": [
            {
              "name": "id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "sequence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "project_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "parent_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "event_type",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "role",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "execution_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "content",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "payload_json",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "format_version",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "parent_id",
                "session_id"
              ],
              "table": "events",
              "references": [
                "id",
                "session_id"
              ]
            },
            {
              "columns": [
                "project_id"
              ],
              "table": "projects",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "workspace_id"
              ],
              "table": "workspaces",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "job_runs",
          "columns": [
            {
              "name": "id",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "job_id",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "started_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "finished_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "exit_code",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "output",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "jobs",
          "columns": [
            {
              "name": "id",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "name",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "schedule",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "command",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "created_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "enabled",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "mcp_connections",
          "columns": [
            {
              "name": "id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "name",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "url",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "client_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "status",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "status_detail",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "oauth_json",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "created_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "updated_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_background_cursors",
          "columns": [
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "cursor_sequence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "failures",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "malformed",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "next_attempt_at",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "updated_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "memory_background_runs",
          "columns": [
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "trigger",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "status",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "from_sequence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "through_sequence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "model",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "input_tokens",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "output_tokens",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "cost_micro_usd",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "messages",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "candidates",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "queued",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "duplicates",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "noops",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "skipped",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "malformed",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "error",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "started_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "finished_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "memory_background_state",
          "columns": [
            {
              "name": "singleton",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "activated_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "runner_holder",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "runner_expires_at",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_compiler_activation_claims",
          "columns": [
            {
              "name": "job_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "fence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "work_epoch",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "job_id"
              ],
              "table": "memory_compiler_jobs",
              "references": [
                "job_id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_activation_dirty",
          "columns": [
            {
              "name": "activation_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "high_position",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "scanned_position",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "scan_order",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "activation_id"
              ],
              "table": "memory_compiler_activations",
              "references": [
                "activation_id"
              ]
            },
            {
              "columns": [
                "session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_activation_jobs",
          "columns": [
            {
              "name": "job_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "activation_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "activation_id"
              ],
              "table": "memory_compiler_activations",
              "references": [
                "activation_id"
              ]
            },
            {
              "columns": [
                "job_id"
              ],
              "table": "memory_compiler_jobs",
              "references": [
                "job_id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_activation_requests",
          "columns": [
            {
              "name": "request_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "request_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "response",
              "type": "BLOB",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_compiler_activation_roots",
          "columns": [
            {
              "name": "activation_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "root_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "first_sequence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "last_sequence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "position",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "state",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "reason",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "selection_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "checked_order",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "root_id"
              ],
              "table": "events",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "activation_id"
              ],
              "table": "memory_compiler_activations",
              "references": [
                "activation_id"
              ]
            },
            {
              "columns": [
                "session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_activations",
          "columns": [
            {
              "name": "activation_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "selector_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "source_scope",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "source_session",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "destination",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "generation_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "after_position",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "through_position",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "work_paused",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "work_epoch",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "generation_id"
              ],
              "table": "memory_compiler_generations",
              "references": [
                "generation_id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_candidate_groups",
          "columns": [
            {
              "name": "job_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "envelope_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "job_id"
              ],
              "table": "memory_compiler_jobs",
              "references": [
                "job_id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_candidates",
          "columns": [
            {
              "name": "candidate_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "job_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "ordinal",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "envelope",
              "type": "BLOB",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "equivalence_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "review_state",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "review_revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "equivalent_to",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "job_id"
              ],
              "table": "memory_compiler_candidate_groups",
              "references": [
                "job_id"
              ]
            },
            {
              "columns": [
                "equivalent_to"
              ],
              "table": "memory_compiler_candidates",
              "references": [
                "candidate_id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_capacity",
          "columns": [
            {
              "name": "singleton",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "request_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "job_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "fence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "holder",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "server_identity",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "state",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "job_id"
              ],
              "table": "memory_compiler_jobs",
              "references": [
                "job_id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_coverage",
          "columns": [
            {
              "name": "job_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "outcome",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "event_ids",
              "type": "BLOB",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "job_id"
              ],
              "table": "memory_compiler_jobs",
              "references": [
                "job_id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_diagnostic_attempts",
          "columns": [
            {
              "name": "job_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "attempt",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "fence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "claimed_at",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "queue_wait_ns",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "inference_ns",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "validation_ns",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "database_ns",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "outcome",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "job_id"
              ],
              "table": "memory_compiler_jobs",
              "references": [
                "job_id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_diagnostic_decisions",
          "columns": [
            {
              "name": "candidate_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "decided_at",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "candidate_id"
              ],
              "table": "memory_compiler_candidates",
              "references": [
                "candidate_id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_diagnostic_foreground",
          "columns": [
            {
              "name": "root_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "started_at",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "terminal_at",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "terminal_ns",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "finalized_at",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "finalization_ns",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "outcome",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "root_id"
              ],
              "table": "events",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_diagnostic_jobs",
          "columns": [
            {
              "name": "job_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "destination",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "state",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "attempts",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "cancellations",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "queued_at",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "ready_at",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "published_at",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "publication_ns",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "unresolved",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "accepted",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "rejected",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "suppressed",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "job_id"
              ],
              "table": "memory_compiler_jobs",
              "references": [
                "job_id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_diagnostic_navigation",
          "columns": [
            {
              "name": "singleton",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "cohort",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "last_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "last_ordinal",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "activation_cutoff",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "history_cutoff",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "job_cutoff",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_compiler_diagnostic_reconcile",
          "columns": [
            {
              "name": "singleton",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "last_job",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "cutoff",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "complete",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_compiler_diagnostic_sessions",
          "columns": [
            {
              "name": "destination",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "counts",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_event_positions",
          "columns": [
            {
              "name": "event_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "commit_position",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "event_id"
              ],
              "table": "events",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_generations",
          "columns": [
            {
              "name": "generation_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "manifest",
              "type": "BLOB",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_compiler_history_changes",
          "columns": [
            {
              "name": "operation_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "request_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "response",
              "type": "BLOB",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "change_order",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_compiler_history_events",
          "columns": [
            {
              "name": "request_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "range_ordinal",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "event_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 3
            },
            {
              "name": "sequence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "root_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "event_id"
              ],
              "table": "events",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "root_id"
              ],
              "table": "events",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "request_id",
                "range_ordinal"
              ],
              "table": "memory_compiler_history_ranges",
              "references": [
                "request_id",
                "ordinal"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_history_jobs",
          "columns": [
            {
              "name": "job_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "cancel_order",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "job_id"
              ],
              "table": "memory_compiler_jobs",
              "references": [
                "job_id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_history_ranges",
          "columns": [
            {
              "name": "request_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "ordinal",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "source_scope",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "destination",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "first_sequence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "last_sequence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "first_event_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "last_event_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "scanned_sequence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "checked_order",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "first_event_id"
              ],
              "table": "events",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "last_event_id"
              ],
              "table": "events",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "request_id"
              ],
              "table": "memory_compiler_history_requests",
              "references": [
                "request_id"
              ]
            },
            {
              "columns": [
                "session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_history_requests",
          "columns": [
            {
              "name": "request_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "request_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "owner_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "generation_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "selection_order",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "receipt",
              "type": "BLOB",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "pending_ranges",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "pending_roots",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "range_checked",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "root_checked",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "resume_order",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "cancelled",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "generation_id"
              ],
              "table": "memory_compiler_generations",
              "references": [
                "generation_id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_history_resume_queue",
          "columns": [
            {
              "name": "request_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "change_order",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "checked_order",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "request_id"
              ],
              "table": "memory_compiler_history_requests",
              "references": [
                "request_id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_history_roots",
          "columns": [
            {
              "name": "request_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "range_ordinal",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "root_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 3
            },
            {
              "name": "first_sequence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "last_sequence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "state",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "reason",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "selection_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "checked_order",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "root_id"
              ],
              "table": "events",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "request_id",
                "range_ordinal"
              ],
              "table": "memory_compiler_history_ranges",
              "references": [
                "request_id",
                "ordinal"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_history_schedule_counter",
          "columns": [
            {
              "name": "singleton",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "value",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_compiler_history_selection_refs",
          "columns": [
            {
              "name": "generation_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "destination",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 3
            },
            {
              "name": "event_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 4
            },
            {
              "name": "active_requests",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "event_id"
              ],
              "table": "events",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "generation_id"
              ],
              "table": "memory_compiler_generations",
              "references": [
                "generation_id"
              ]
            },
            {
              "columns": [
                "session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_job_schedule",
          "columns": [
            {
              "name": "job_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "lane",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "position",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "historical_order",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "pause_reason",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "job_id"
              ],
              "table": "memory_compiler_jobs",
              "references": [
                "job_id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_jobs",
          "columns": [
            {
              "name": "job_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "generation_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "destination",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "root_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "first_sequence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "last_sequence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "window_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "request",
              "type": "BLOB",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "state",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "reason",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "attempts",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "fence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "holder",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "lease_until",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "retry_at",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "root_id"
              ],
              "table": "events",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "generation_id"
              ],
              "table": "memory_compiler_generations",
              "references": [
                "generation_id"
              ]
            },
            {
              "columns": [
                "session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_position_counter",
          "columns": [
            {
              "name": "singleton",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "value",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_compiler_recurrence",
          "columns": [
            {
              "name": "publication_order",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "candidate_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "encoding_version",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "exact_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "related_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "presentation_epoch",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "exact_encoding",
              "type": "BLOB",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "related_encoding",
              "type": "BLOB",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "primary_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "relationship",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "suppressed",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "checked_interpretation",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "checked_review",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "checked_state",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "candidate_id"
              ],
              "table": "memory_compiler_candidates",
              "references": [
                "candidate_id"
              ]
            },
            {
              "columns": [
                "primary_id"
              ],
              "table": "memory_compiler_candidates",
              "references": [
                "candidate_id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_recurrence_migration",
          "columns": [
            {
              "name": "singleton",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "last_rowid",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_compiler_release_receipts",
          "columns": [
            {
              "name": "request_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "job_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "fence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "holder",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "server_identity",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "kind",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_compiler_resources",
          "columns": [
            {
              "name": "job_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "fence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "stage_bytes",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "candidate_positions",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "job_id"
              ],
              "table": "memory_compiler_jobs",
              "references": [
                "job_id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_scheduler",
          "columns": [
            {
              "name": "singleton",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "new_attempts",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_compiler_selections",
          "columns": [
            {
              "name": "selection_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "generation_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "destination",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "root_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "first_sequence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "last_sequence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "state",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "reason",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "job_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "window",
              "type": "BLOB",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "root_id"
              ],
              "table": "events",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "generation_id"
              ],
              "table": "memory_compiler_generations",
              "references": [
                "generation_id"
              ]
            },
            {
              "columns": [
                "session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_stages",
          "columns": [
            {
              "name": "job_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "fence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "envelope",
              "type": "BLOB",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "envelope_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "consumed",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "job_id"
              ],
              "table": "memory_compiler_jobs",
              "references": [
                "job_id"
              ]
            }
          ]
        },
        {
          "name": "memory_compiler_status_event_revision",
          "columns": [
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_compiler_status_events",
          "columns": [
            {
              "name": "source_scope",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "generation_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 3
            },
            {
              "name": "destination",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 4
            },
            {
              "name": "event_revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "history_revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "after_sequence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "total",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "selected",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_compiler_status_history_revision",
          "columns": [
            {
              "name": "generation_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "destination",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 3
            },
            {
              "name": "revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_compiler_status_roots",
          "columns": [
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "after_activation",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "after_root",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "pending",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "failed",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_compiler_worker_installation",
          "columns": [
            {
              "name": "singleton",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_dense_dirty",
          "columns": [
            {
              "name": "generation",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "claim_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "generation"
              ],
              "table": "memory_dense_generations",
              "references": [
                "generation"
              ]
            }
          ]
        },
        {
          "name": "memory_dense_event_dirty",
          "columns": [
            {
              "name": "generation",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "event_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "generation"
              ],
              "table": "memory_dense_generations",
              "references": [
                "generation"
              ]
            }
          ]
        },
        {
          "name": "memory_dense_event_vectors",
          "columns": [
            {
              "name": "generation",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "event_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "scope_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "byte_start",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 3
            },
            {
              "name": "byte_end",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 4
            },
            {
              "name": "source_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "revision_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "content_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "vector",
              "type": "BLOB",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "generation"
              ],
              "table": "memory_dense_generations",
              "references": [
                "generation"
              ]
            }
          ]
        },
        {
          "name": "memory_dense_generations",
          "columns": [
            {
              "name": "generation",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "configuration",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "configuration_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "state",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "claim_checkpoint",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "event_checkpoint",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_dense_selection",
          "columns": [
            {
              "name": "singleton",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "generation",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "enabled",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "generation"
              ],
              "table": "memory_dense_generations",
              "references": [
                "generation"
              ]
            }
          ]
        },
        {
          "name": "memory_dense_vectors",
          "columns": [
            {
              "name": "generation",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "claim_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "scope_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "byte_start",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 3
            },
            {
              "name": "byte_end",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 4
            },
            {
              "name": "source_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "revision_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "content_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "document_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "vector",
              "type": "BLOB",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "generation"
              ],
              "table": "memory_dense_generations",
              "references": [
                "generation"
              ]
            }
          ]
        },
        {
          "name": "memory_proposal_inbox",
          "columns": [
            {
              "name": "proposal_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "scope_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "tool_name",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "reason",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "idempotency_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "source_event_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "intent_event_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "target_claim_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "dedupe_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "proposal_json",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "proposal_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "prepared_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "alternative_proposal_sha256",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "alternative_prepared_sha256",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "queued_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "refreshed_at",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "everywhere_proposal_json",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "everywhere_proposal_sha256",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "everywhere_prepared_sha256",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "everywhere_alternative_proposal_sha256",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "everywhere_alternative_prepared_sha256",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "memory_proposal_inbox_declines",
          "columns": [
            {
              "name": "proposal_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "declined_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "proposal_id"
              ],
              "table": "memory_proposal_inbox",
              "references": [
                "proposal_id"
              ]
            }
          ]
        },
        {
          "name": "memory_retrieval_dirty",
          "columns": [
            {
              "name": "claim_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_retrieval_event_dirty",
          "columns": [
            {
              "name": "event_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_retrieval_event_fts_v4",
          "columns": [
            {
              "name": "generation",
              "type": "",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "event_id",
              "type": "",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "scope_key",
              "type": "",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "body",
              "type": "",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "keys",
              "type": "",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_retrieval_event_fts_v4_rows",
          "columns": [
            {
              "name": "fts_rowid",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "event_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_retrieval_fts_v3",
          "columns": [
            {
              "name": "generation",
              "type": "",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "claim_id",
              "type": "",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "scope_key",
              "type": "",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "body",
              "type": "",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_retrieval_fts_v3_rows",
          "columns": [
            {
              "name": "fts_rowid",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "claim_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_retrieval_generations",
          "columns": [
            {
              "name": "generation",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "configuration",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "state",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "checkpoint",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_review_audits",
          "columns": [
            {
              "name": "audit_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "scope_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "preview_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "action",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "envelope",
              "type": "BLOB",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "preview_id"
              ],
              "table": "memory_review_previews",
              "references": [
                "preview_id"
              ]
            }
          ]
        },
        {
          "name": "memory_review_authorization",
          "columns": [
            {
              "name": "singleton",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "authentication_key",
              "type": "BLOB",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "source_policy",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_review_batch_deliveries",
          "columns": [
            {
              "name": "owner_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "delivery_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "scope_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "request_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "result",
              "type": "BLOB",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_review_batch_previews",
          "columns": [
            {
              "name": "preview_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "scope_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "preview_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "envelope",
              "type": "BLOB",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_review_deliveries",
          "columns": [
            {
              "name": "owner_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "delivery_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "scope_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "request_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "result",
              "type": "BLOB",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_review_edit_revisions",
          "columns": [
            {
              "name": "candidate_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "envelope",
              "type": "BLOB",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "candidate_id"
              ],
              "table": "memory_compiler_candidates",
              "references": [
                "candidate_id"
              ]
            }
          ]
        },
        {
          "name": "memory_review_identity_revisions",
          "columns": [
            {
              "name": "candidate_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "envelope",
              "type": "BLOB",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "candidate_id"
              ],
              "table": "memory_compiler_candidates",
              "references": [
                "candidate_id"
              ]
            }
          ]
        },
        {
          "name": "memory_review_inbox_revisions",
          "columns": [
            {
              "name": "scope_key",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_review_navigation_reconcile",
          "columns": [
            {
              "name": "singleton",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "last_candidate",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "cutoff",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "complete",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_review_previews",
          "columns": [
            {
              "name": "preview_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "scope_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "preview_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "envelope",
              "type": "BLOB",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_review_resolutions",
          "columns": [
            {
              "name": "candidate_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "owner_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "delivery_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "audit_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "candidate_id"
              ],
              "table": "memory_compiler_candidates",
              "references": [
                "candidate_id"
              ]
            },
            {
              "columns": [
                "audit_id"
              ],
              "table": "memory_review_audits",
              "references": [
                "audit_id"
              ]
            }
          ]
        },
        {
          "name": "memory_review_temporal_revisions",
          "columns": [
            {
              "name": "candidate_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "envelope",
              "type": "BLOB",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "candidate_id"
              ],
              "table": "memory_compiler_candidates",
              "references": [
                "candidate_id"
              ]
            }
          ]
        },
        {
          "name": "memory_session_episode_fts",
          "columns": [
            {
              "name": "body",
              "type": "",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "memory_session_episodes",
          "columns": [
            {
              "name": "episode_rowid",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "scope_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "summary_event_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "generation",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "covered_first_event_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "covered_first_sequence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "covered_last_event_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "covered_last_sequence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "started_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "ended_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "summarized_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "covered_first_event_id"
              ],
              "table": "events",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "covered_last_event_id"
              ],
              "table": "events",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "summary_event_id"
              ],
              "table": "events",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "plugin_enabled_configuration",
          "columns": [
            {
              "name": "plugin_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "enabled",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "updated_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "projects",
          "columns": [
            {
              "name": "id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "display_name",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "canonical_root",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "archived",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "created_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "updated_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "repository_instruction_snapshots",
          "columns": [
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "turn_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "snapshot_json",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "turn_id",
                "session_id"
              ],
              "table": "events",
              "references": [
                "id",
                "session_id"
              ]
            }
          ]
        },
        {
          "name": "semantic_aliases",
          "columns": [
            {
              "name": "alias_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "entity_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "scope_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "value",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "normalized_value",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "lifecycle",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "source_event_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "created_operation_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "source_event_id"
              ],
              "table": "events",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "entity_id"
              ],
              "table": "semantic_entities",
              "references": [
                "entity_id"
              ]
            },
            {
              "columns": [
                "created_operation_id"
              ],
              "table": "semantic_operations",
              "references": [
                "operation_id"
              ]
            },
            {
              "columns": [
                "scope_id"
              ],
              "table": "semantic_scopes",
              "references": [
                "scope_id"
              ]
            }
          ]
        },
        {
          "name": "semantic_claim_classifications",
          "columns": [
            {
              "name": "claim_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "scope_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "kind",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "standing",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "operation_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "scope_revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "transaction_time",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "claim_id"
              ],
              "table": "semantic_claims",
              "references": [
                "claim_id"
              ]
            },
            {
              "columns": [
                "operation_id"
              ],
              "table": "semantic_operations",
              "references": [
                "operation_id"
              ]
            },
            {
              "columns": [
                "scope_id"
              ],
              "table": "semantic_scopes",
              "references": [
                "scope_id"
              ]
            }
          ]
        },
        {
          "name": "semantic_claim_corrections",
          "columns": [
            {
              "name": "operation_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "scope_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "old_claim_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "replacement_claim_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "mode",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "effective_time",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "old_valid_from",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "old_valid_to",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "old_effective_from",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "old_effective_to",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "replacement_from",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "replacement_to",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "scope_revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "transaction_time",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "old_claim_id"
              ],
              "table": "semantic_claims",
              "references": [
                "claim_id"
              ]
            },
            {
              "columns": [
                "replacement_claim_id"
              ],
              "table": "semantic_claims",
              "references": [
                "claim_id"
              ]
            },
            {
              "columns": [
                "operation_id"
              ],
              "table": "semantic_operations",
              "references": [
                "operation_id"
              ]
            },
            {
              "columns": [
                "scope_id"
              ],
              "table": "semantic_scopes",
              "references": [
                "scope_id"
              ]
            }
          ]
        },
        {
          "name": "semantic_claims",
          "columns": [
            {
              "name": "claim_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "scope_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "subject_entity_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "predicate_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "predicate_token",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "predicate_version",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "object_kind",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "object_entity_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "literal_kind",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "literal_value",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "polarity",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "valid_from",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "valid_to",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "lifecycle",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "created_operation_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "transaction_time",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "object_entity_id"
              ],
              "table": "semantic_entities",
              "references": [
                "entity_id"
              ]
            },
            {
              "columns": [
                "subject_entity_id"
              ],
              "table": "semantic_entities",
              "references": [
                "entity_id"
              ]
            },
            {
              "columns": [
                "created_operation_id"
              ],
              "table": "semantic_operations",
              "references": [
                "operation_id"
              ]
            },
            {
              "columns": [
                "predicate_id"
              ],
              "table": "semantic_predicates",
              "references": [
                "predicate_id"
              ]
            },
            {
              "columns": [
                "scope_id"
              ],
              "table": "semantic_scopes",
              "references": [
                "scope_id"
              ]
            }
          ]
        },
        {
          "name": "semantic_cursor_auth",
          "columns": [
            {
              "name": "singleton",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "hmac_key",
              "type": "BLOB",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "semantic_entities",
          "columns": [
            {
              "name": "entity_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "scope_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "canonical_name",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "entity_type",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "anchor_kind",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "lifecycle",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "created_operation_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "created_operation_id"
              ],
              "table": "semantic_operations",
              "references": [
                "operation_id"
              ]
            },
            {
              "columns": [
                "scope_id"
              ],
              "table": "semantic_scopes",
              "references": [
                "scope_id"
              ]
            }
          ]
        },
        {
          "name": "semantic_graph_links",
          "columns": [
            {
              "name": "graph_link_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "scope_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "relation",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "source_kind",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "source_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "target_kind",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "target_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "lifecycle",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "created_operation_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "transaction_time",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "created_operation_id"
              ],
              "table": "semantic_operations",
              "references": [
                "operation_id"
              ]
            },
            {
              "columns": [
                "scope_id"
              ],
              "table": "semantic_scopes",
              "references": [
                "scope_id"
              ]
            }
          ]
        },
        {
          "name": "semantic_maintenance_lock",
          "columns": [
            {
              "name": "singleton",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "fencing_token",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "holder_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "expires_at",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "semantic_operation_scopes",
          "columns": [
            {
              "name": "operation_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "scope_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "prior_revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "resulting_revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "written",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "operation_id"
              ],
              "table": "semantic_operations",
              "references": [
                "operation_id"
              ]
            },
            {
              "columns": [
                "scope_id"
              ],
              "table": "semantic_scopes",
              "references": [
                "scope_id"
              ]
            }
          ]
        },
        {
          "name": "semantic_operations",
          "columns": [
            {
              "name": "operation_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "schema_version",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "operation_kind",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "idempotency_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "actor",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "target_scope_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "source_event_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "proposal_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "effect_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "proposal_json",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "prepared_proposal_json",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "result_json",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "transaction_time",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "source_event_id"
              ],
              "table": "events",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "target_scope_id"
              ],
              "table": "semantic_scopes",
              "references": [
                "scope_id"
              ]
            },
            {
              "columns": [
                "session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "semantic_predicates",
          "columns": [
            {
              "name": "predicate_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "scope_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "token",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "version",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "label",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "object_constraint",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "cardinality",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "created_operation_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "created_operation_id"
              ],
              "table": "semantic_operations",
              "references": [
                "operation_id"
              ]
            },
            {
              "columns": [
                "scope_id"
              ],
              "table": "semantic_scopes",
              "references": [
                "scope_id"
              ]
            }
          ]
        },
        {
          "name": "semantic_projection_quarantine",
          "columns": [
            {
              "name": "scope_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "reason",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "operation_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "verified_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "scope_id"
              ],
              "table": "semantic_scopes",
              "references": [
                "scope_id"
              ]
            }
          ]
        },
        {
          "name": "semantic_promotion_entities",
          "columns": [
            {
              "name": "operation_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "source_entity_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "destination_entity_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "destination_entity_id"
              ],
              "table": "semantic_entities",
              "references": [
                "entity_id"
              ]
            },
            {
              "columns": [
                "source_entity_id"
              ],
              "table": "semantic_entities",
              "references": [
                "entity_id"
              ]
            },
            {
              "columns": [
                "operation_id"
              ],
              "table": "semantic_promotions",
              "references": [
                "operation_id"
              ]
            }
          ]
        },
        {
          "name": "semantic_promotions",
          "columns": [
            {
              "name": "operation_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "source_scope_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "destination_scope_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "source_claim_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "destination_claim_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "destination_claim_id"
              ],
              "table": "semantic_claims",
              "references": [
                "claim_id"
              ]
            },
            {
              "columns": [
                "source_claim_id"
              ],
              "table": "semantic_claims",
              "references": [
                "claim_id"
              ]
            },
            {
              "columns": [
                "operation_id"
              ],
              "table": "semantic_operations",
              "references": [
                "operation_id"
              ]
            },
            {
              "columns": [
                "destination_scope_id"
              ],
              "table": "semantic_scopes",
              "references": [
                "scope_id"
              ]
            },
            {
              "columns": [
                "source_scope_id"
              ],
              "table": "semantic_scopes",
              "references": [
                "scope_id"
              ]
            }
          ]
        },
        {
          "name": "semantic_scopes",
          "columns": [
            {
              "name": "scope_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "scope_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "scope_kind",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "registry_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "semantic_source_links",
          "columns": [
            {
              "name": "source_link_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "scope_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "claim_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "event_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "source_session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "source_scope_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "event_part",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "locator_kind",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "locator_value",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "evidence_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "source_actor",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "source_type",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "authority",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "observed_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "eligibility",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "created_operation_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "event_id"
              ],
              "table": "events",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "claim_id"
              ],
              "table": "semantic_claims",
              "references": [
                "claim_id"
              ]
            },
            {
              "columns": [
                "created_operation_id"
              ],
              "table": "semantic_operations",
              "references": [
                "operation_id"
              ]
            },
            {
              "columns": [
                "scope_id"
              ],
              "table": "semantic_scopes",
              "references": [
                "scope_id"
              ]
            },
            {
              "columns": [
                "source_session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "semantic_state_events",
          "columns": [
            {
              "name": "scope_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "object_kind",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "object_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 3
            },
            {
              "name": "state",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 4
            },
            {
              "name": "operation_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "scope_revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "transaction_time",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "operation_id"
              ],
              "table": "semantic_operations",
              "references": [
                "operation_id"
              ]
            },
            {
              "columns": [
                "scope_id"
              ],
              "table": "semantic_scopes",
              "references": [
                "scope_id"
              ]
            }
          ]
        },
        {
          "name": "session_compatibility_resolutions",
          "columns": [
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "resolution_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "resolution_json",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "resolved_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "session_id"
              ],
              "table": "session_composition_receipts",
              "references": [
                "session_id"
              ]
            }
          ]
        },
        {
          "name": "session_composition_receipts",
          "columns": [
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "receipt_json",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "session_model_settings",
          "columns": [
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "model_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "session_task_focus",
          "columns": [
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "task_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "selected_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "task_id"
              ],
              "table": "tasks",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "session_turn_leases",
          "columns": [
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "holder_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "fencing_token",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "lease_generation",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "expires_at",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "session_workspace_access_pins",
          "columns": [
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "access_revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "workspace_id"
              ],
              "table": "workspaces",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "sessions",
          "columns": [
            {
              "name": "id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "project_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "project_root_snapshot",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "parent_session_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "title",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "status",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "created_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "updated_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "workspace_revision_snapshot",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "project_id"
              ],
              "table": "projects",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "parent_session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "workspace_id"
              ],
              "table": "workspaces",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "skill_activations",
          "columns": [
            {
              "name": "name",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "version",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "proposal_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "commit_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "tree_path",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "version_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "manifest",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "metadata",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "actor",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "surface",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "approved_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "proposal_id"
              ],
              "table": "skill_proposals",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "skill_deactivations",
          "columns": [
            {
              "name": "name",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "version",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "actor",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "surface",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "deactivated_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "name",
                "version"
              ],
              "table": "skill_activations",
              "references": [
                "name",
                "version"
              ]
            }
          ]
        },
        {
          "name": "skill_imports",
          "columns": [
            {
              "name": "id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "report",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "actor",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "surface",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "imported_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "skill_proposals",
          "columns": [
            {
              "name": "id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "import_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "name",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "commit_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "tree_path",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "version_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "manifest",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "metadata",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "source",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "warnings",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "actor",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "surface",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "proposed_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "import_id"
              ],
              "table": "skill_imports",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "subagent_executions",
          "columns": [
            {
              "name": "id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "parent_session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "idempotency_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "request_digest",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "child_session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "state",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "record_json",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "child_session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "parent_session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "task_access_grants",
          "columns": [
            {
              "name": "id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "grantee_session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "root_task_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "access_level",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "issuer_actor_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "issuer_session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "issuer_run_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "issued_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "ended_at",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "end_reason",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "ended_by_actor_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "ended_by_session_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "ended_by_run_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "grantee_session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "issuer_session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "root_task_id"
              ],
              "table": "tasks",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "task_claim_events",
          "columns": [
            {
              "name": "id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "task_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "sequence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "operation",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "actor_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "previous_revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "resulting_revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "outcome",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "diagnostic_code",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "identity_sha256",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "claim_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "claim_reason",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "management_override",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "management_reason",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "task_id"
              ],
              "table": "tasks",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "task_claims",
          "columns": [
            {
              "name": "task_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "claim_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "actor_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "lease_holder_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "lease_token",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "lease_generation",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "acquired_run_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "claimed_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "task_id"
              ],
              "table": "tasks",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "task_coordination_conflicts",
          "columns": [
            {
              "name": "id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "actor_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "identity_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "original_request_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "attempted_request_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "operation",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "task_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "task_coordination_results",
          "columns": [
            {
              "name": "actor_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "identity_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 3
            },
            {
              "name": "request_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "operation",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "task_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "event_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "outcome_code",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "claim_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "claimed_at",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "released_at",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "release_reason",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "from_status",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "event_id"
              ],
              "table": "task_claim_events",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "task_decomposition_children",
          "columns": [
            {
              "name": "actor_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "identity_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 3
            },
            {
              "name": "child_order",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 4
            },
            {
              "name": "child_task_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "child_revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "actor_id",
                "session_id",
                "identity_sha256"
              ],
              "table": "task_decomposition_results",
              "references": [
                "actor_id",
                "session_id",
                "identity_sha256"
              ]
            },
            {
              "columns": [
                "child_task_id",
                "child_revision"
              ],
              "table": "task_revisions",
              "references": [
                "task_id",
                "revision"
              ]
            }
          ]
        },
        {
          "name": "task_decomposition_conflicts",
          "columns": [
            {
              "name": "id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "actor_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "identity_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "original_request_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "attempted_request_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "parent_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "task_decomposition_results",
          "columns": [
            {
              "name": "actor_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "identity_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 3
            },
            {
              "name": "parent_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "event_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "outcome_code",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "diagnostic_field",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "previous_revision",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "resulting_revision",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "from_status",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "to_status",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "event_id"
              ],
              "table": "task_hierarchy_events",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "actor_id",
                "session_id",
                "identity_sha256"
              ],
              "table": "task_idempotency_claims",
              "references": [
                "actor_id",
                "session_id",
                "identity_sha256"
              ]
            }
          ]
        },
        {
          "name": "task_event_grants",
          "columns": [
            {
              "name": "event_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "grant_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "grant_id"
              ],
              "table": "task_access_grants",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "task_event_idempotency",
          "columns": [
            {
              "name": "event_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "identity_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "task_events",
          "columns": [
            {
              "name": "id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "task_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "sequence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "operation",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "actor_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "previous_revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "resulting_revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "outcome",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "diagnostic_code",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "task_id"
              ],
              "table": "tasks",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "task_hierarchy",
          "columns": [
            {
              "name": "task_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "parent_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "root_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "sibling_order",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "parent_id"
              ],
              "table": "tasks",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "root_id"
              ],
              "table": "tasks",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "task_id"
              ],
              "table": "tasks",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "task_hierarchy_events",
          "columns": [
            {
              "name": "id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "task_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "sequence",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "actor_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "previous_revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "resulting_revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "operation",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "outcome",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "diagnostic_code",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "task_id"
              ],
              "table": "tasks",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "task_idempotency_claims",
          "columns": [
            {
              "name": "actor_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "identity_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 3
            },
            {
              "name": "request_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "operation",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "task_idempotency_conflicts",
          "columns": [
            {
              "name": "id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "actor_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "identity_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "original_request_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "attempted_request_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "operation",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "task_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "task_legacy_todo_imports",
          "columns": [
            {
              "name": "migration_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "source_list",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "source_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "item_count",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "applied_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "task_legacy_todo_provenance",
          "columns": [
            {
              "name": "migration_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "source_list",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "legacy_id",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 3
            },
            {
              "name": "source_index",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "task_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "migration_id"
              ],
              "table": "task_legacy_todo_imports",
              "references": [
                "migration_id"
              ]
            },
            {
              "columns": [
                "task_id"
              ],
              "table": "tasks",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "task_mutation_results",
          "columns": [
            {
              "name": "actor_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "identity_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 3
            },
            {
              "name": "request_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "operation",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "task_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "event_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "outcome_code",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "diagnostic_field",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "previous_revision",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "resulting_revision",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "from_status",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "to_status",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "event_id"
              ],
              "table": "task_events",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "task_revisions",
          "columns": [
            {
              "name": "task_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "scope",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "title",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "description",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "priority",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "due_date",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "result_summary",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "status",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "created_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "updated_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "depends_on",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "paths",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "task_id"
              ],
              "table": "tasks",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "tasks",
          "columns": [
            {
              "name": "id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "scope",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "title",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "description",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "priority",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "due_date",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "result_summary",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "status",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "created_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "updated_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "depends_on",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "paths",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "workflow_activations",
          "columns": [
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "name",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "version",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 3
            },
            {
              "name": "proposal_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "commit_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "tree_path",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "executable_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "authority_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "authority",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "workers",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "schedule",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "workspace_revision_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "access_revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "approver",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "approval_surface",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "approval_detail",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "approved_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "proposal_id"
              ],
              "table": "workflow_proposals",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "workspace_id"
              ],
              "table": "workspaces",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workflow_builtin_controls",
          "columns": [
            {
              "name": "seq",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "name",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "enabled",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "reason",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "actor",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "surface",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "workflow_builtin_versions",
          "columns": [
            {
              "name": "name",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "version",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "commit_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "tree_path",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "executable_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "authority_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "authority",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "workers",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "report",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "recorded_by",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "workflow_deactivations",
          "columns": [
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "name",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "version",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 3
            },
            {
              "name": "reason",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "surface",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "deactivated_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "workspace_id",
                "name",
                "version"
              ],
              "table": "workflow_activations",
              "references": [
                "workspace_id",
                "name",
                "version"
              ]
            }
          ]
        },
        {
          "name": "workflow_effect_conflicts",
          "columns": [
            {
              "name": "conflict_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 3
            },
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "intent_seq",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "acquired_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "released_at",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "run_id",
                "intent_seq"
              ],
              "table": "workflow_effect_intents",
              "references": [
                "run_id",
                "seq"
              ]
            }
          ]
        },
        {
          "name": "workflow_effect_intents",
          "columns": [
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "seq",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "attempt_seq",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "attempt",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "step_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "checkpoint",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "capability",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "contract",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "schema_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "plugin",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "plugin_version",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "connection",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "resource",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "idempotency_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "idempotent",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "reconcile",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "conflict_keys",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "input",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "authority",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "lease_holder",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "lease_token",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "created_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "run_id"
              ],
              "table": "workflow_runs",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workflow_effect_receipts",
          "columns": [
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "intent_seq",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "outcome",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "response",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "error",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "source",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "evidence",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "actor",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "surface",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "lease_holder",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "lease_token",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "run_id",
                "intent_seq"
              ],
              "table": "workflow_effect_intents",
              "references": [
                "run_id",
                "seq"
              ]
            }
          ]
        },
        {
          "name": "workflow_effect_unknowns",
          "columns": [
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "intent_seq",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "reason",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "response",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "lease_holder",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "lease_token",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "run_id",
                "intent_seq"
              ],
              "table": "workflow_effect_intents",
              "references": [
                "run_id",
                "seq"
              ]
            }
          ]
        },
        {
          "name": "workflow_global_activations",
          "columns": [
            {
              "name": "name",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "version",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "proposal_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "commit_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "tree_path",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "executable_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "authority_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "authority",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "workers",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "approver",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "approval_surface",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "approval_detail",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "approved_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "proposal_id"
              ],
              "table": "workflow_global_proposals",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workflow_global_deactivations",
          "columns": [
            {
              "name": "name",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "version",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "reason",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "surface",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "deactivated_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "name",
                "version"
              ],
              "table": "workflow_global_activations",
              "references": [
                "name",
                "version"
              ]
            }
          ]
        },
        {
          "name": "workflow_global_proposals",
          "columns": [
            {
              "name": "id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "name",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "commit_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "tree_path",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "executable_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "authority_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "report",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "simulation",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "simulation_status",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "proposed_by",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "surface",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "documents_version",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "proposed_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "workflow_interruption_resolutions",
          "columns": [
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "interruption_seq",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "resolution",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "answer",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "actor",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "surface",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "detail",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "lease_holder",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "lease_token",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "run_id",
                "interruption_seq"
              ],
              "table": "workflow_interruptions",
              "references": [
                "run_id",
                "seq"
              ]
            }
          ]
        },
        {
          "name": "workflow_interruptions",
          "columns": [
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "seq",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "kind",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "step_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "checkpoint",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "intent_seq",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "message",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "request",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "response_schema",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "lease_holder",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "lease_token",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "opened_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "run_id",
                "intent_seq"
              ],
              "table": "workflow_effect_intents",
              "references": [
                "run_id",
                "seq"
              ]
            },
            {
              "columns": [
                "run_id",
                "checkpoint"
              ],
              "table": "workflow_run_checkpoints",
              "references": [
                "run_id",
                "seq"
              ]
            },
            {
              "columns": [
                "run_id"
              ],
              "table": "workflow_runs",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workflow_model_calls",
          "columns": [
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "attempt_seq",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "step_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "policy",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "policy_version",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "provider",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "model",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "parameters",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "prompt_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "schema_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "request_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "estimated_input_tokens",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "estimated_output_tokens",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "estimated_micro_usd",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "lease_holder",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "lease_token",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "started_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "finished_at",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "requests",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "input_tokens",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "output_tokens",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "usage_reported",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "micro_usd",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "pricing_source",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "response",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "error",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "run_id"
              ],
              "table": "workflow_runs",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workflow_node_attempts",
          "columns": [
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "seq",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "step_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "kind",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "attempt",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "input_checkpoint",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "input",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "state",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "output",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "error",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "next_step",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "note",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "started_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "finished_at",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "lease_holder",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "lease_token",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "item_index",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "map_attempt",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "run_id",
                "input_checkpoint"
              ],
              "table": "workflow_run_checkpoints",
              "references": [
                "run_id",
                "seq"
              ]
            },
            {
              "columns": [
                "run_id"
              ],
              "table": "workflow_runs",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workflow_notifications",
          "columns": [
            {
              "name": "id",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "dedup_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "workflow",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "kind",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "run_state",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "detail",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "origin_surface",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "created_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "read_at",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "change_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "run_id"
              ],
              "table": "workflow_runs",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workflow_proposals",
          "columns": [
            {
              "name": "id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "name",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "commit_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "tree_path",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "executable_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "authority_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "report",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "simulation",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "simulation_status",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "proposed_by",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "surface",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "workspace_revision_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "access_revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "documents_version",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "proposed_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "workspace_id"
              ],
              "table": "workspaces",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workflow_retention_guard",
          "columns": [
            {
              "name": "singleton",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "purpose",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "workflow_retention_state",
          "columns": [
            {
              "name": "singleton",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "runner_holder",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "runner_expires_at",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "last_sweep_at",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "last_summary",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "workflow_retention_tombstones",
          "columns": [
            {
              "name": "id",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "sweep_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "action",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "data_class",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "classification",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "workflow",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "records",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "bytes",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "fields",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "oldest_at",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "newest_at",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "policy",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "reason",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "actor",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "surface",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "run_id"
              ],
              "table": "workflow_runs",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workflow_run_cancellations",
          "columns": [
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "reason",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "actor",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "surface",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "from_state",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "requested_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "run_id"
              ],
              "table": "workflow_runs",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workflow_run_checkpoints",
          "columns": [
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "seq",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "after_attempt",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "next_step",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "state",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "created_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "compacted_at",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "run_id"
              ],
              "table": "workflow_runs",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workflow_run_events",
          "columns": [
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "seq",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "kind",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "holder_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "fencing_token",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "attempt_seq",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "detail",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "created_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "run_id"
              ],
              "table": "workflow_runs",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workflow_run_leases",
          "columns": [
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "holder_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "fencing_token",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "acquired_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "heartbeat_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "expires_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "released_at",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "run_id"
              ],
              "table": "workflow_runs",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workflow_run_migrations",
          "columns": [
            {
              "name": "successor_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "predecessor_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "workflow",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "from_version",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "to_version",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "checkpoint",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "resume_step",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "logical_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "plan",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "effects",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "carried_elapsed_ms",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "carried_attempts",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "carried_tokens",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "carried_micro_usd",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "slot_timezone",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "slot_key",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "reason",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "actor",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "surface",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "predecessor_id"
              ],
              "table": "workflow_runs",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "successor_id"
              ],
              "table": "workflow_runs",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workflow_run_overrides",
          "columns": [
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "rerun_of",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "logical_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "reason",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "actor",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "surface",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "rerun_of"
              ],
              "table": "workflow_runs",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "run_id"
              ],
              "table": "workflow_runs",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workflow_run_substitutions",
          "columns": [
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "seq",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "capability",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "plugin",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "pinned_version",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "replacement_version",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "contract",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "schema_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "evidence",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "approver",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "surface",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "approved_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "run_id"
              ],
              "table": "workflow_runs",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workflow_runs",
          "columns": [
            {
              "name": "id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "workflow",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "version",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "commit_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "tree_path",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "executable_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "authority_hash",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "workspace_revision_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "access_revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "composition",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "run_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "inputs",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "state",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "outputs",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "error",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "origin_surface",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "created_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "started_at",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "finished_at",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "updated_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "not_before",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "source",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "workspace_id"
              ],
              "table": "workspaces",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workflow_schedule_controls",
          "columns": [
            {
              "name": "seq",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "workflow",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "version",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "action",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "reason",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "actor",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "surface",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "workspace_id",
                "workflow",
                "version"
              ],
              "table": "workflow_activations",
              "references": [
                "workspace_id",
                "name",
                "version"
              ]
            }
          ]
        },
        {
          "name": "workflow_schedule_slots",
          "columns": [
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "workflow",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "version",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "timezone",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 3
            },
            {
              "name": "slot_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 4
            },
            {
              "name": "due_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "shifted",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "outcome",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "detail",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "decided_by",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "decided_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "workspace_id",
                "workflow",
                "version"
              ],
              "table": "workflow_activations",
              "references": [
                "workspace_id",
                "name",
                "version"
              ]
            },
            {
              "columns": [
                "run_id"
              ],
              "table": "workflow_runs",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workflow_scheduler_state",
          "columns": [
            {
              "name": "singleton",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "runner_holder",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "runner_expires_at",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "workflow_worker_calls",
          "columns": [
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "attempt_seq",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "step_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "item_index",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "worker",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "policy",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "policy_version",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "provider",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "model",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "objective_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "schema_sha256",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "budget_ms",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "budget_tokens",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "input_per_million",
              "type": "REAL",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "output_per_million",
              "type": "REAL",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "pricing_source",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "estimated_micro_usd",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "adopted_from",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "continues",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "execution_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "child_session_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "lease_holder",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "lease_token",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "started_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "finished_at",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "status",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "reason",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "input_tokens",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "output_tokens",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "usage_complete",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "counted_tokens",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "micro_usd",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "child_session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "run_id"
              ],
              "table": "workflow_runs",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workflow_worker_sessions",
          "columns": [
            {
              "name": "run_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "session_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "created_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "session_id"
              ],
              "table": "sessions",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "run_id"
              ],
              "table": "workflow_runs",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workspace_access_revisions",
          "columns": [
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "access",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "workspace_id"
              ],
              "table": "workspaces",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workspace_build_settings",
          "columns": [
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "revision_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "setting",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "workspace_id",
                "revision_id"
              ],
              "table": "workspace_preset_revisions",
              "references": [
                "workspace_id",
                "revision_id"
              ]
            }
          ]
        },
        {
          "name": "workspace_explore_settings",
          "columns": [
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "revision_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "setting",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "workspace_id",
                "revision_id"
              ],
              "table": "workspace_preset_revisions",
              "references": [
                "workspace_id",
                "revision_id"
              ]
            }
          ]
        },
        {
          "name": "workspace_folder_revisions",
          "columns": [
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "path",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "workspace_id"
              ],
              "table": "workspaces",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workspace_instruction_settings",
          "columns": [
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "enabled",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "workspace_id"
              ],
              "table": "workspaces",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workspace_mcp_tool_policies",
          "columns": [
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "revision",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "policies",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "workspace_id"
              ],
              "table": "workspaces",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workspace_preset_revisions",
          "columns": [
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "revision_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "default_preset_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "allowed_preset_ids",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "workspace_id"
              ],
              "table": "workspaces",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workspace_preset_revocations",
          "columns": [
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "revision_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "preset_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 3
            },
            {
              "name": "revoked_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "workspace_id",
                "revision_id"
              ],
              "table": "workspace_preset_revisions",
              "references": [
                "workspace_id",
                "revision_id"
              ]
            }
          ]
        },
        {
          "name": "workspace_tab_data_approvals",
          "columns": [
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "tab_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "data_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "approved_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "workspace_id"
              ],
              "table": "workspaces",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workspace_tab_problems",
          "columns": [
            {
              "name": "id",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "workspace_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "tab_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "kind",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "message",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "workspace_id"
              ],
              "table": "workspaces",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "workspaces",
          "columns": [
            {
              "name": "id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "display_name",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "lifecycle_state",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "current_revision_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "created_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "updated_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        }
      ]
    },
    {
      "id": "finance",
      "package": "internal/finance",
      "tables": [
        {
          "name": "budget_entries",
          "columns": [
            {
              "name": "id",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "transaction_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "category",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "amount_cents",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "source",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "tags",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "category"
              ],
              "table": "categories",
              "references": [
                "name"
              ]
            },
            {
              "columns": [
                "transaction_id"
              ],
              "table": "transactions",
              "references": [
                "transaction_id"
              ]
            }
          ]
        },
        {
          "name": "budget_limits",
          "columns": [
            {
              "name": "category",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "month",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "limit_cents",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "category"
              ],
              "table": "categories",
              "references": [
                "name"
              ]
            }
          ]
        },
        {
          "name": "categories",
          "columns": [
            {
              "name": "name",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "effect_receipts",
          "columns": [
            {
              "name": "idempotency_key",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "effect",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "output",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "recorded_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "finance_account_snapshots",
          "columns": [
            {
              "name": "item_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "updated_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "item_id"
              ],
              "table": "items",
              "references": [
                "item_id"
              ]
            }
          ]
        },
        {
          "name": "finance_accounts",
          "columns": [
            {
              "name": "item_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "account_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "name",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "mask",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "type",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "subtype",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "item_id"
              ],
              "table": "items",
              "references": [
                "item_id"
              ]
            }
          ]
        },
        {
          "name": "finance_link_sessions",
          "columns": [
            {
              "name": "id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "link_token",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "hosted_url",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "expires_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "state",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "claim_expires_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "created_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "items",
          "columns": [
            {
              "name": "item_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "access_token",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "cursor",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "institution",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "linked_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "name_rules",
          "columns": [
            {
              "name": "id",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "pattern",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "category",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "priority",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "category"
              ],
              "table": "categories",
              "references": [
                "name"
              ]
            }
          ]
        },
        {
          "name": "review_decisions",
          "columns": [
            {
              "name": "transaction_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "decision",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "category",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "reason",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "run_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "decided_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "transaction_id"
              ],
              "table": "transactions",
              "references": [
                "transaction_id"
              ]
            }
          ]
        },
        {
          "name": "rules",
          "columns": [
            {
              "name": "id",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "merchant",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "category",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "category"
              ],
              "table": "categories",
              "references": [
                "name"
              ]
            }
          ]
        },
        {
          "name": "transactions",
          "columns": [
            {
              "name": "transaction_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "item_id",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "account_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "date",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "name",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "merchant_name",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "amount_cents",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "plaid_category",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "category",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "category_source",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "reviewed",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "pending",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "tags",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "item_id"
              ],
              "table": "items",
              "references": [
                "item_id"
              ]
            }
          ]
        }
      ]
    },
    {
      "id": "youtube",
      "package": "internal/youtube",
      "tables": [
        {
          "name": "channels",
          "columns": [
            {
              "name": "id",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "youtube_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "name",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "handle",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "url",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "legacy_name",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "legacy_key",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "created_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "updated_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "transcript_fts",
          "columns": [
            {
              "name": "transcript_id",
              "type": "",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "channel",
              "type": "",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "title",
              "type": "",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "text",
              "type": "",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": []
        },
        {
          "name": "transcript_states",
          "columns": [
            {
              "name": "video_id",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 1
            },
            {
              "name": "language_code",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 2
            },
            {
              "name": "status",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "detail",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "checked_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "video_id"
              ],
              "table": "videos",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "transcripts",
          "columns": [
            {
              "name": "id",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "artifact_key",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "video_id",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "legacy_channel_id",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "language_code",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "language_name",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "source",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "text",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "word_count",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "retrieved_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "source_path",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "source_sha256",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "legacy_channel_id"
              ],
              "table": "channels",
              "references": [
                "id"
              ]
            },
            {
              "columns": [
                "video_id"
              ],
              "table": "videos",
              "references": [
                "id"
              ]
            }
          ]
        },
        {
          "name": "videos",
          "columns": [
            {
              "name": "id",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 1
            },
            {
              "name": "youtube_id",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "legacy_key",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "channel_id",
              "type": "INTEGER",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "title",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "url",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "published_at",
              "type": "TEXT",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "duration_seconds",
              "type": "INTEGER",
              "notNull": false,
              "primaryKey": 0
            },
            {
              "name": "created_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            },
            {
              "name": "updated_at",
              "type": "TEXT",
              "notNull": true,
              "primaryKey": 0
            }
          ],
          "foreignKeys": [
            {
              "columns": [
                "channel_id"
              ],
              "table": "channels",
              "references": [
                "id"
              ]
            }
          ]
        }
      ]
    }
  ],
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
      "term": "Global Conversation",
      "definition": "A conversation started outside any Context Scope, with global memory, its own session memory, and the earlier Global Conversations it may recall.",
      "avoid": [
        "Unscoped session",
        "plain chat"
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
  ],
  "history": [
    {
      "date": "2026-11-05",
      "title": "Architecture and History move into the System map",
      "branch": "feat/system-architecture",
      "touches": [
        ".vision",
        "docs/architecture/explorer",
        "internal/web",
        "internal/web/ui",
        "scripts/explorer",
        "scripts/verify-change.sh",
        "AGENTS.md"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/system-page.decisions.md",
        "doc:cmd/evie/docs/active/system-page.spec.md",
        "doc:docs/architecture/explorer/README.md"
      ],
      "text": [
        "The System view showed the map beside the old explorer, a second page with its own look that answered \"where is it?\" and drew History. David asked for both to live in the vision skill instead, and its page now has three tabs: Map; Architecture, the renamed \"Where is it?\", with Context, Containers, Components and Flows, where each part lists the map steps that cite it; and History, read straight from this file. Evie's System view is that one page in one frame, and its Map, Architecture and History tabs only move the page's fragment, so switching never reloads it.",
        "Evie's written explorer content moved into `.vision/map.json` as parts, edges, sequences and groups, so the explorer viewer, its Node tests, the example design proposal, the content files and the manifest are gone; the fact extractor and `facts.js` stay. The drift check now holds the map's architecture to the code the way it held the content files: every package needs a part, every part, edge and sequence reference must resolve, import edges and steps must be real imports, and every sequence step must cite code. Evie serves only the page, and `build.js` is gone, since the page shows no code links and History's GitHub links come from the map. Design proposals did not carry over; they come back as part of the vision skill."
      ]
    },
    {
      "date": "2026-11-04",
      "title": "The System view opens on a map of Evie's flows",
      "branch": "feat/system-map",
      "merge": "0f5de61",
      "touches": [
        ".vision",
        "docs/architecture/explorer",
        "internal/web",
        "internal/web/ui",
        "internal/memorybackground",
        "scripts/explorer",
        "AGENTS.md"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/system-page.decisions.md",
        "doc:cmd/evie/docs/active/system-page.spec.md"
      ],
      "text": [
        "The explorer shows Evie as components, which says where code lives but not what happens when David sends a message. David approved a prototype that draws Evie the vision way instead, and the System view now opens on it: a subway map whose lines are one chat turn and the flows it starts (Memory, Workflows, Subagents, Changes & merge, MCP sign-in, Serve startup), each station opening into the steps inside it. It is always dark inside Evie and shows no code. History and Where is it? sit beside it as tabs, opening the explorer on its History and on its components in one shared frame, so switching between them does not reload it.",
        "The map lives in `.vision/map.json` and points every step at its code and every route, table, package and command chip at the extracted facts. The vision skill builds it into `docs/architecture/explorer/system.html`, which is committed and built into the binary like the facts. The skill lives outside the repository, so the drift check now also fails when a reference in the map no longer resolves, when a fact chip no longer says what its reference points at (matched as the skill's check matches it), or when the page was not rebuilt after the map changed, comparing the page's map hash with one computed the skill's way. It reads only the forms Evie's map uses and refuses the skill's others. Drawing the map also corrected two claims elsewhere: the background memory pass is off unless `EVIE_REMOTE_MEMORY=on` or a local extractor enables it, and Automatic Recall can run two more searches for exact selectors such as `task-12`."
      ]
    },
    {
      "date": "2026-11-03",
      "title": "Skills: owner-approved procedures Evie loads on demand",
      "branch": "feat/skills",
      "merge": "e07bb02",
      "touches": [
        "internal/skill",
        "internal/eviedb",
        "internal/plugins",
        "internal/agent",
        "internal/web",
        "internal/web/ui",
        "internal/memory",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/skills.spec.md",
        "#201",
        "#202",
        "#203",
        "#204"
      ],
      "text": [
        "David relied on dozens of Claude Code skills, but Evie had none, so a procedure had to be explained in the chat each time or hard-coded into the prompt. Skills in the Agent Skills format now come into Evie from the command line: `evie skills propose <folder>`, or `evie skills import-claude` for David's personal, synced and `~/.agents` skills (and installed plugin skills with `--plugins`). Each is checked, committed to the procedural repository and recorded as a proposal; nothing is active until David approves an exact version, one skill or a whole import at a time. Skills that bundle scripts or executables are refused.",
        "A chat created after an approval pins the active skills in its Composition Receipt for its whole life; older chats resume unchanged on the frozen pre-skills Standard preset. Evie sees a short framed catalog of the chat's skills and loads one with the `skill` tool when it fits. David can force one with `/name` in the composer, which sends the skill with the message and labels it. A skill guides how Evie works and never grants tools, approvals or memory scope."
      ]
    },
    {
      "date": "2026-11-03",
      "title": "Read a worker's report in the Inspector",
      "branch": "feat/artifacts-report-view",
      "merge": "7fe6b17",
      "touches": [
        "internal/eviedb",
        "internal/web",
        "internal/web/ui",
        "internal/delegation",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/artifacts.spec.md",
        "#199",
        "#200"
      ],
      "text": [
        "When a chat delegated research, exploration or a build, the worker's full report was stored and the parent model could page it, but David could only see the worker's activity. Opening a worker in the Inspector now offers Report and Activity tabs, with Report first once the worker has finished. Report shows the exact stored report, the same bytes `read_subagent_report` pages, as an inert document: raw HTML stays text, only http(s) links are links and open outside Evie, and images never load.",
        "Evie's own facts sit above the worker's text: kind, status and reason, tokens and model responses, the sources Evie verified and the cited URLs it couldn't, and its notes. The worker's text and limitations are labelled as the worker's. The report answers only for the open chat and the workers it delegated. Older records still open, and a report over 1 MiB says how much is cut. This is artifacts stage 1: the first stored output with a view of its own, before any artifact store."
      ]
    },
    {
      "date": "2026-11-02",
      "title": "Review proposed lines as a deck of cards",
      "branch": "feat/proposal-deck",
      "merge": "261aa8f",
      "touches": [
        "internal/web/ui",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/workspace-activity.spec.md",
        "doc:cmd/evie/docs/active/finance-review.spec.md"
      ],
      "text": [
        "A review question, such as finance-review's, showed its proposed lines as raw JSON. David asked for cards he can swipe through like the memory inbox. \"Review the N lines\" now opens a deck: one line at a time with its merchant, amount, date, category and confidence, reason and merchant rule. Swipe right or → keeps it, left or ← rejects it, ↑ skips it to the back, the category pill changes it, and the rule chip drops the rule. The deck ends on a summary whose Answer sends only the exceptions in the step's own schema. It applies to any question whose input lists objects with ids and whose schema names reject, change or no_rule lists."
      ]
    },
    {
      "date": "2026-11-02",
      "title": "Workspace tabs read at a glance",
      "branch": "feat/workspace-tabs-ui",
      "merge": "206c61e",
      "touches": [
        "internal/web/ui",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/workspace-activity.spec.md",
        "doc:cmd/evie/docs/active/workspace-tabs.spec.md"
      ],
      "text": [
        "David found the Runs and Workflows tabs cluttered and a Workspace's own tabs indistinguishable from the built-in ones. After an approved canvas prototype: the strip puts a divider and a page icon before the Workspace's own tabs, pins Settings right as an icon, and moves the spend line into Runs. Runs reads one line per run: a waiting run is a task with an Answer control, such as \"Review 12 lines\", and finished runs sit under their workflow with a summary, a dot per run and dated rows on demand, so thirty days of finance-daily is one row. The run panel leads with the task and its answer, \"Accept as proposed\" when an empty answer is valid, with the input and the JSON form on demand, and steps are one line each. Built-in workflows are rows of the Workflows table, and the CLI how-to folds into one disclosure."
      ]
    },
    {
      "date": "2026-11-02",
      "title": "Steadier chat status, and General leaves the prompt",
      "branch": "fix/general-followups",
      "merge": "2e4e792",
      "touches": [
        "internal/web",
        "internal/agent"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/multi-chat.spec.md",
        "doc:cmd/evie/docs/active/session-history-and-memory-scope.spec.md",
        "term:Global Conversation"
      ],
      "text": [
        "Follow-ups to retiring the General workspace. The chat status stream sent an unchanged status again when two changes woke it at once; computed just before an approval and delivered after it, that repeat showed the chat as still waiting and made a status test fail about once in 200 to 600 runs. The stream now skips a status equal to the last one it sent. The chat prompt no longer tells the model that \"The General workspace is a workspace\"; the memory compiler's frozen copy of that line stays under its versioned policy. The glossary names a chat outside any workspace a Global Conversation."
      ]
    },
    {
      "date": "2026-11-02",
      "title": "Chats start without a workspace",
      "branch": "feat/plain-chats",
      "merge": "8eae958",
      "touches": [
        "internal/web",
        "internal/web/ui"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/ui-chat.spec.md",
        "doc:cmd/evie/docs/active/multi-chat.decisions.md"
      ],
      "text": [
        "David kept a \"General\" workspace only because a chat without one was hard to start. The sidebar now opens with New chat, which starts a chat with no workspace, and lists those chats under Chats below the Workspaces: the ten latest, with Show all for the rest. With no chat open, Start a conversation offers New chat, New workspace and Manage workspaces in place of the scope chooser. New chat reopens an open chat that is still empty, and a chat with no workspace reads \"No workspace\" instead of \"Unscoped\". General was archived the same day."
      ]
    },
    {
      "date": "2026-11-02",
      "title": "New workspaces are Standard with research on",
      "branch": "feat/workspace-create-simplify",
      "merge": "e95adb1",
      "touches": [
        "internal/web",
        "internal/web/ui",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/workspace-research.spec.md",
        "doc:cmd/evie/docs/active/workspaces.decisions.md",
        "#198"
      ],
      "text": [
        "David found the Agent Preset choice over-engineered and wanted research delegation always on. Creating a workspace now asks only for a name, a repository and a folder; the server creates it on the Standard preset with research delegation on, unless a request sends `allowResearchDelegation: false`, and the REPL does the same. Workspace settings keep the research switch. Presets are checked before a repository is set up, so a refused workspace leaves no GitHub repository behind. Revisiting presets as an owner choice is #198."
      ]
    },
    {
      "date": "2026-11-02",
      "title": "Readable chat column and a one-strip inspector",
      "branch": "feat/chat-readability",
      "merge": "7723418",
      "touches": [
        "internal/web/ui",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/ui-chat.spec.md",
        "doc:cmd/evie/docs/active/chat-activity.spec.md"
      ],
      "text": [
        "David found long answers hard to read. Playwright screenshots of the live app showed why: the 768px column at 15px ran about 100 characters a line, an answer ran straight into the next user bubble, and every bold phrase flashed white. The transcript and composer now share a 680px column at 16px, a user turn after the first sits clear of the answer above it with the time it was sent beneath the bubble, and emphasis keeps the body color. A Claude Design canvas prototype was approved first; David declined the serif and warm palette directions shown beside it, so teal and IBM Plex stay.",
        "The inspector's header stacked view buttons, tabs and a title line. It is now one strip: the open tabs, a plus whose menu offers Files, Working tree and Terminal, then focus and close. Tabs name what they hold, such as the tool's action and subject, and the pane uses the app's own inspector surface instead of the editor gray."
      ]
    },
    {
      "date": "2026-11-02",
      "title": "Model picker opens on the current model",
      "branch": "fix/model-picker-current",
      "merge": "f654bbf",
      "touches": [
        "internal/web/ui",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/chat-model-selection.spec.md"
      ],
      "text": [
        "`/model` marked the current model, but far down a list of about 250 models sorted by provider, so David couldn't tell which model a chat used. The picker now names it above the list, such as \"Current: DeepSeek V4.1 Flash\", opens scrolled to its row once the catalog fills, and arrow-down from the search field starts there."
      ]
    },
    {
      "date": "2026-11-02",
      "title": "One live thinking line with titled thoughts",
      "branch": "feat/thinking-line",
      "merge": "46a8c45",
      "touches": [
        "internal/web/ui",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/chat-activity.spec.md",
        "doc:cmd/evie/docs/active/chat-activity.decisions.md"
      ],
      "text": [
        "While Evie works, the activity header is now the live line, after David's Claude.ai screenshot: the current title with the clock running, a soft shimmer, and a fade between titles held two seconds each. A thought's title is its heading, else its latest intent rewritten as an action (\"Let me check the fees.\" becomes \"Checking the fees\"), else its latest sentence. A short streamed progress message shows as the line's title instead of flashing below it. A finished turn names its successful actions before its time, such as \"Searched the web, read a page · 1m 12s\", and the expanded trace is a bordered card of titled thoughts that open to their text.",
        "This is stage 1, screen only. Reasoning is still not saved, so thought rows disappear on reload; saving it (stage 2) and a small title-model call (stage 3) are agreed and deferred."
      ]
    },
    {
      "date": "2026-11-01",
      "title": "Test suite passes on CI's Linux runners",
      "branch": "fix/ci-linux-failures",
      "merge": "b262252",
      "touches": [
        "cmd/evie",
        "internal/eviedb",
        "internal/localterminal",
        "internal/subagents",
        "internal/tools",
        "internal/worktree"
      ],
      "sources": [
        "code:internal/worktree/worktree_test.go",
        "code:internal/localterminal/session_test.go",
        "code:internal/subagents/build_test.go"
      ],
      "text": [
        "CI's Ubuntu runner gained bubblewrap, so the sandbox tests ran on Linux for the first time, and its bash differs from macOS's. The worktree, build worker, terminal, relevance-scale and verify-before-push tests now expect Linux's sandbox and bash behaviour, and the worktree commit test checks that a `.GIT` is refused at commit. The fix was made on 2026-10-26 on the remote and reached local master in this merge."
      ]
    },
    {
      "date": "2026-11-01",
      "title": "Dark explorer inside Evie",
      "branch": "feat/explorer-dark",
      "touches": [
        "docs/architecture/explorer"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/system-page.decisions.md",
        "doc:docs/architecture/explorer/README.md"
      ],
      "text": [
        "The explorer in Evie's System view is now always dark, whatever the system preference; opened from disk it still follows the system. Each colour token now holds its light and dark values in `light-dark()`, so the frame forces dark through `color-scheme` without a second copy of the palette."
      ]
    },
    {
      "date": "2026-10-31",
      "title": "System page and History",
      "branch": "feat/system-page",
      "merge": "38c7f63",
      "touches": [
        "internal/web",
        "docs/architecture/explorer",
        "scripts/explorer",
        "internal/web/ui"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/system-page.spec.md",
        "#195",
        "#196",
        "#197"
      ],
      "text": [
        "Brings the system explorer into Evie. A System entry under Data in the sidebar opens the explorer full-height in a sandboxed frame. `evie serve` serves a snapshot of the explorer built into the binary under `/explorer/`, with a `build.js` naming the repository, the commit, and whether the build had uncommitted changes. Code links in the served explorer open GitHub at the build's commit, or master when there is no commit stamp or the build was modified; opened from disk, they stay local file links.",
        "The explorer gains a fifth tab, History, drawn from this file: every merge to master and every run of direct-to-master commits, newest first and grouped by month, and each package's panel lists the entries that touched it. `extract` parses `CHANGELOG.md` into the facts, and the drift check validates each entry's format, date, order, sources, and commits. The changelog is backfilled for master's whole first-parent history."
      ]
    },
    {
      "date": "2026-10-31",
      "title": "System explorer",
      "branch": "feat/system-explorer",
      "merge": "394b77f",
      "touches": [
        "scripts/explorer",
        "docs/architecture/explorer"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/system-explorer.spec.md",
        "doc:cmd/evie/docs/active/system-explorer.decisions.md",
        "doc:docs/architecture/explorer/README.md",
        "#186",
        "#187",
        "#188",
        "#189",
        "#190",
        "#191",
        "#192",
        "#193",
        "#194"
      ],
      "text": [
        "Adds the system explorer, a page opened straight from disk that shows what Evie does: every package under `cmd/` and `internal/` as a component, grouped under the architecture map in `AGENTS.md`, with levels from the system context down to one package's focus view, key runtime flows as sequence diagrams to step through, and design proposals toggled on top of the built system.",
        "Written content cites its sources, and the facts beneath it are extracted from the code: packages, imports, commands, routes, database tables, glossary terms, and ADRs. The drift check, run by `./scripts/verify-change.sh`, fails when the data and the code disagree, so the map cannot drift quietly."
      ]
    },
    {
      "date": "2026-10-28",
      "title": "Repository aligned with the current skills setup",
      "branch": "master",
      "commits": {
        "from": "7bc0936",
        "to": "7bc0936"
      },
      "touches": [
        "GLOSSARY.md",
        "AGENTS.md",
        "docs/agents",
        "cmd/evie/docs"
      ],
      "sources": [
        "doc:GLOSSARY.md",
        "doc:docs/agents/domain.md",
        "doc:docs/agents/triage-labels.md",
        "doc:docs/agents/issue-tracker.md"
      ],
      "text": [
        "Renames `CONTEXT.md` to `GLOSSARY.md`, the name the skills now read, and points the four docs that referenced it at the new name. Adds the domain and triage-label docs the skills expect, and an Agent skills section in `AGENTS.md` that points at them. The issue-tracker doc now says tickets are GitHub issues and specs are repository docs, matching `AGENTS.md`."
      ]
    },
    {
      "date": "2026-10-26",
      "title": "Tool results projected in batches, and lighter turns",
      "branch": "cache/batched-projection",
      "merge": "cc69b4d",
      "touches": [
        "internal/agent",
        "internal/eviedb"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/memory.decisions.md",
        "code:internal/agent/request_history.go",
        "code:internal/agent/turn_decodes.go"
      ],
      "text": [
        "While a request fits usable input, older tool-result groups past the three retained ones now become eligible for pressure projection in whole batches of four, so the projected history changes once a batch instead of on almost every tool call. Cached-token counts from the owner's history showed that rolling projection caused most avoidable provider cache misses; in a replay of a long chat, the previous request's bytes left unreused fell from 1.70 MB to 1.37 MB for 1.6 percent more bytes sent. The composer version becomes `context-composer-v6`.",
        "The branch also cuts the work each model request repeats within a turn: the turn reads only the events committed since its previous request (`EventsAfter`), derives history checks once per request, decodes each stored event once, and sizes pressure projection without re-encoding the whole request. On a 70-turn chat, overhead per model request fell from 68 ms to 23 ms; requests are unchanged."
      ]
    },
    {
      "date": "2026-10-26",
      "title": "Opening the database checks only semantic foreign keys",
      "branch": "perf/startup-fk-check",
      "merge": "c114768",
      "touches": [
        "internal/eviedb"
      ],
      "sources": [
        "code:internal/eviedb/semantic_replay.go"
      ],
      "text": [
        "Every open of Evie's database ran an unscoped `PRAGMA foreign_key_check`, reading every row of every table, and then kept only the `semantic_*` failures: 43 ms of a 55 ms open at 50,000 messages, paid by every `evie` command. The check now runs on each `semantic_*` table in name order (0.16 ms) and finds the same failures. Its rows are now closed on a scan error, which had left the connection checked out."
      ]
    },
    {
      "date": "2026-10-26",
      "title": "Idle retrieval ticks read before they write",
      "branch": "perf/dense-idle-no-write",
      "merge": "adf24f5",
      "touches": [
        "internal/eviedb"
      ],
      "sources": [
        "code:internal/eviedb/retrieval_dense_index.go#Store.ensureDenseGeneration"
      ],
      "text": [
        "The once-a-second retrieval maintenance tick took SQLite's write lock even when nothing changed: an UPDATE disabling dense retrieval (the default) or, with it enabled, a `BEGIN IMMEDIATE` around an unchanged selection. While a turn held the lock, an idle tick waited out the five-second busy timeout and failed. `ensureDenseGeneration` now reads the selection first and writes only when it must change."
      ]
    },
    {
      "date": "2026-10-26",
      "title": "Due workflow runs found without reading finished ones",
      "branch": "perf/due-workflow-runs-index",
      "merge": "df8b10e",
      "touches": [
        "internal/eviedb"
      ],
      "sources": [
        "code:internal/eviedb/workflow_run_leases.go"
      ],
      "text": [
        "The scheduler asks for due runs once a second, and its query ORed a condition per due state, so SQLite could use no index and scanned every run retention kept: 6.24 ms with 20,000 finished runs. The query now starts with one `IN` term over the five due states, read through a new partial index holding just those runs (0.068 ms for the same history). The `workflow_runs_unfinished` index, which the query never used, is dropped on open."
      ]
    },
    {
      "date": "2026-10-26",
      "title": "An idle compiler host stops taking the write lock",
      "branch": "perf/compiler-host-idle",
      "merge": "999d61e",
      "touches": [
        "internal/eviedb",
        "cmd/evie"
      ],
      "sources": [
        "code:internal/eviedb/compiler_reconciliation.go#Store.RunCompilerHost",
        "code:cmd/evie/compiler_host.go"
      ],
      "text": [
        "`RunCompilerHost` ran four `BEGIN IMMEDIATE` transactions every 25 ms whether or not there was work, re-checked a turn still in progress 40 times a second, and dropped reconciliation errors without a word. Each tick now looks for work read-only first: ready work runs every 25 ms until it is gone, work that only time or a released turn lease can move runs once a second, and with nothing pending the host takes no write lock and looks every 250 ms. Failures retry once a second and reach a new `ReconcileHealth` callback, which the runtime logs. Idle CPU over 3 s drops from 116 ms to 12 ms with nothing pending, and from 369 ms to 24 ms with a turn in progress."
      ]
    },
    {
      "date": "2026-10-26",
      "title": "Search-index refreshes and coverage without index scans",
      "branch": "perf/index-coverage-count",
      "merge": "5382545",
      "touches": [
        "internal/eviedb"
      ],
      "sources": [
        "code:internal/eviedb/retrieval_index_rows.go",
        "code:internal/eviedb/retrieval_index.go"
      ],
      "text": [
        "The conversation and accepted-memory FTS tables keep their event or Claim id in an unindexed column, so every refresh deleted a row by scanning the whole index inside the write lock: 4.16 s for a 256-message batch at 50,000 messages. Each index now has a lookup from id to FTS rowid, written in the same transaction as the row, and refreshes delete through it (0.10 s); opening the database rebuilds a lookup that does not match its index.",
        "Coverage, read by every search and maintenance tick, now counts indexed rows through those lookups and keeps the dirty queue as the outer loop. At 50,000 messages the coverage read drops from 19.9 ms to 0.34 ms, and an idle maintenance tick from 40.2 ms to 0.71 ms."
      ]
    },
    {
      "date": "2026-10-26",
      "title": "Dead code removed and scratch output untracked",
      "branch": "cleanup/dead-code-and-junk",
      "merge": "e919e80",
      "touches": [
        "internal/web",
        "internal/finance",
        "internal/tools",
        "internal/eviedb",
        "internal/plugins",
        "internal/openrouter",
        "internal/subagents",
        "cmd/evie",
        ".gitignore"
      ],
      "sources": [
        "code:internal/delegation/types.go#ExploreToolNames"
      ],
      "text": [
        "Deletes the functions nothing calls, not even tests: the legacy `web.Serve` entry points, which duplicated `ServeWithContext` without shutdown, unused REPL and file-edit wrappers, `finance.Query` and `OpenDBReadOnly`, and a handful of orphaned helpers. The explore test now checks the toolset against `delegation.ExploreToolNames`, the list the supervisor enforces, instead of a second copy. `.scratch/`, `tmp/` and `__pycache__/`, about 460 MB of local run output, binaries and rendered drafts, are no longer tracked; curated evidence stays under `cmd/*/docs/fixtures/`."
      ]
    },
    {
      "date": "2026-10-13",
      "title": "Workspace memories stay in their Workspace, reviewed as a deck",
      "branch": "master",
      "commits": {
        "from": "bda9f7d",
        "to": "0f1f386"
      },
      "touches": [
        "internal/memorybackground",
        "internal/eviedb",
        "internal/memory",
        "internal/web",
        "internal/web/ui"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/memory.decisions.md",
        "doc:cmd/evie/docs/active/memory.spec.md#background-memory-pass"
      ],
      "text": [
        "The background pass had routed any memory the model called \"profile\" to Global, so a Finance chat's merchant-rule request became a Global profile fact. In a Workspace or project chat every memory now targets that scope; one that Evie suggests holds everywhere also carries a prepared Global version, and the review card's \"Save everywhere instead\" approves that exact version. Global chats and in-turn proposals are unchanged.",
        "The review then becomes a deck showing one proposal at a time. A card's face holds the memory, with pills for where it applies, its kind and Evie's wording, and the Always applies and Everywhere instead toggles; flipping it shows the message Evie read and the exact change. Swipes or the arrow keys save, decline or skip, and \"Decline all\" clears a batch from one message. The background pass becomes a status dot, and an empty queue says All caught up."
      ]
    },
    {
      "date": "2026-10-12",
      "title": "Workspace tabs read approved data and can show built-in views",
      "branch": "master",
      "commits": {
        "from": "e517a4f",
        "to": "a3fa82e"
      },
      "touches": [
        "internal/workspacetabs",
        "internal/web",
        "internal/web/ui",
        "internal/plugins",
        "internal/tools",
        "internal/eviedb",
        "internal/agent",
        "internal/workflowruntime",
        "cmd/evie",
        "scripts"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/workspace-tabs.spec.md",
        "doc:cmd/evie/docs/active/workspace-tabs.decisions.md"
      ],
      "text": [
        "A tab's `tab.json` may declare data (a registered database, files in the Workspace folder matching a glob, or the Workspace's workflow runs) and reads nothing until the owner approves that exact list in Settings → Tabs. Tabs read through Evie's helper over a private `MessageChannel` opened with a one-time key, and the server checks each request against the approved list. A review then bounded each query to 4 seconds and each value to 2 MB, capped concurrent tab reads, and refused secret-looking paths, SQLite files and hard links.",
        "Evie's helper also reports a tab's script errors, failed loads and policy blocks, each tab keeps its latest 20 problems, and a new `workspace_tabs` tool gives Workspace chats the tab contract, each tab's status, data list and approval, and its problems. Tabs can load Evie's own stylesheet at `_evie/evie.css`, and query rows arrive as objects keyed by column. David accepted WebRTC as a residual risk and asked for a partial block, so every tab page now starts with a script that removes the WebRTC constructors. Finally, `tab.json` may name `\"builtin\": \"spending\"` to show the real Spending view in the Workspace."
      ]
    },
    {
      "date": "2026-10-12",
      "title": "Workspace tabs: a Workspace's own pages, sealed",
      "branch": "master",
      "commits": {
        "from": "0c4f91d",
        "to": "d529e8a"
      },
      "touches": [
        "internal/workspacetabs",
        "internal/web",
        "internal/web/ui",
        "internal/eviedb",
        "cmd/evie",
        "scripts"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/workspace-tabs.spec.md",
        "doc:cmd/evie/docs/active/workspace-tabs.decisions.md"
      ],
      "text": [
        "A Workspace can have its own tabs, written by Evie and kept in its folder under `.evie/tabs/<tab-id>/` with a `tab.json` and an `index.html`. `internal/workspacetabs` lists and validates them within fixed limits and opens files only through an `os.Root` on the tab folder, and `evie serve` serves them with a sandboxing content-security policy: scripts, no same-origin, no network. A review then hardened serving: redirects built only from checked IDs, only regular files reached without a link, loading only from the tab's own folder URL, and an app shell that frames only Evie's own pages, so a tab cannot navigate its frame to another site.",
        "Tabs show in the Workspace header after Runs, Changes and Workflows, with a More menu for those that do not fit, each in an iframe sandboxed to scripts. Settings → Tabs lists every tab folder and what is wrong with it. A browser check proves the seal in Chrome with a hostile tab."
      ]
    },
    {
      "date": "2026-10-12",
      "title": "Several chats at once",
      "branch": "master",
      "commits": {
        "from": "7d1613a",
        "to": "af14f41"
      },
      "touches": [
        "internal/web",
        "internal/web/ui",
        "internal/openrouter",
        "internal/agent",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/multi-chat.spec.md",
        "doc:cmd/evie/docs/active/multi-chat.decisions.md"
      ],
      "text": [
        "The server held one chat and refused to open another while any turn ran. It now keeps up to 8 open chats, each with its own agent session and model, closing the least recently used idle one at the limit, and a turn in one chat no longer blocks another. Each chat publishes its turn events to a feed the browser follows, so leaving a running chat or reloading no longer loses the turn, and an approval in a detached turn waits while any Evie tab is connected. The sidebar marks chats that are running, waiting on an approval, or finished since they were last viewed. Opening a chat also reuses a model's resolved context profile for 10 minutes instead of asking OpenRouter twice every time."
      ]
    },
    {
      "date": "2026-10-12",
      "title": "MCP connections",
      "branch": "mcp-connections",
      "merge": "5ca729c",
      "touches": [
        "internal/mcpconn",
        "internal/mcpchat",
        "internal/plugins",
        "internal/eviedb",
        "internal/web",
        "internal/web/ui",
        "internal/tools",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/workspace-mcp-connections.spec.md",
        "doc:cmd/evie/docs/active/workspace-mcp-connections.decisions.md",
        "#179"
      ],
      "text": [
        "`evie serve` connects to hosted MCP servers: OAuth sign-in with PKCE, refresh with rotation saved before use, and tokens kept only in the macOS Keychain. A spike found that the MCP Go SDK re-sends on 401 and 403 and cannot tell unsent calls from unanswered ones, so Evie owns tokens, refresh and sending once, and classifies a failed call as not sent, rejected or unknown.",
        "Workspace chats reach the MCP tools David reviewed for that Workspace through two general tools, `mcp_list_tools` and `mcp_call`, which frames every result as untrusted. A call asks first unless David sets that exact tool version to Allow, and Settings → Connections adds servers, signs in, and enables tools and sets their policy per Workspace. The work reached master as pull request #179, merged here from `origin/master`."
      ]
    },
    {
      "date": "2026-10-12",
      "title": "Watch a chat's subagents",
      "branch": "master",
      "commits": {
        "from": "36c6087",
        "to": "ee2111a"
      },
      "touches": [
        "internal/web",
        "internal/web/ui",
        "internal/eviedb",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/subagents.decisions.md",
        "doc:cmd/evie/docs/active/chat-activity.spec.md"
      ],
      "text": [
        "Two read-only routes, `/api/subagents/list` and `/api/subagents/trace`, return the children one delegation call admitted, continued or replayed, and one child's events for that attempt. Both answer only for the open conversation, and child sessions still cannot be opened as chats. In the chat, delegate rows say what they do, such as Research with subagents, and report a live call's children (\"Researching · 2 of 4 done\"); inspecting the row lists each child's state, assignment, elapsed time and latest action, and a child opens to its assignment, its context, and its own trace and report."
      ]
    },
    {
      "date": "2026-10-12",
      "title": "Collapsible Workspaces, selectable models, and the model's current thought",
      "branch": "master",
      "commits": {
        "from": "38e6ba3",
        "to": "a50ee99"
      },
      "touches": [
        "internal/web/ui",
        "internal/openrouter"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/chat-model-selection.spec.md",
        "doc:cmd/evie/docs/active/chat-activity.spec.md",
        "doc:cmd/evie/docs/active/chat-activity.decisions.md"
      ],
      "text": [
        "A chevron collapses a Workspace in the sidebar to hide its chats, remembered per browser. The `/model` picker lists only models that can be selected: 125 of 390 catalog models had failed selection, so variants, routers, and models without a usable output limit or below the reply reserve are skipped. While Evie works with its activity collapsed, the status line shows the latest heading or complete sentence of the streamed reasoning, held at least two seconds. Turns no longer show a Repository instructions action, memory activity hides retrievals that found nothing, and the Workspace tab strip no longer scrolls."
      ]
    },
    {
      "date": "2026-10-11",
      "title": "No memory proposals for archived Workspaces",
      "branch": "master",
      "commits": {
        "from": "6a3c787",
        "to": "6a3c787"
      },
      "touches": [
        "internal/eviedb",
        "internal/memorybackground"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/workspaces.decisions.md"
      ],
      "text": [
        "A pending proposal that targets an archived Workspace, or came from one of its conversations, leaves the review inbox and its count, and approving it is refused; restoring the Workspace brings it back. The background pass stops reading an archived Workspace's conversations, and queueing a new proposal for one is refused."
      ]
    },
    {
      "date": "2026-10-11",
      "title": "Design-sync setup and backlog notes",
      "branch": "master",
      "commits": {
        "from": "93d2227",
        "to": "0ea2e46"
      },
      "touches": [
        "internal/web/ui",
        ".design-sync",
        "docs"
      ],
      "sources": [
        "doc:docs/BACKLOG.md"
      ],
      "text": [
        "`designSystem.ts` exports the UI's props-driven components as a library entry, and `.design-sync` holds the sync configuration, conventions, notes and component previews for Evie's design system. The backlog records the finance-review status and items for the folder chooser, tool results and the turn budget."
      ]
    },
    {
      "date": "2026-10-11",
      "title": "Finance review workflow",
      "branch": "master",
      "commits": {
        "from": "c1e4fc6",
        "to": "c1e4fc6"
      },
      "touches": [
        "internal/finance",
        "internal/plugins",
        "internal/workflow",
        "internal/workflowruntime",
        "internal/tools",
        "internal/agent",
        "internal/web/ui",
        "cmd/evie",
        "cmd/evie/workflows"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/finance-review.spec.md",
        "doc:cmd/evie/docs/active/finance-review.decisions.md"
      ],
      "text": [
        "A workflow for unclassified posted transactions: a model proposes a category for each as typed data, the deterministic `gate_proposals` step drops anything that breaks the rules, and the run pauses for the owner's answer per line. One approved effect, `finance.apply_review`, writes the accepted entries with an exact merchant rule each, and `review_decisions` records every answer so a rejected transaction is never proposed again. The finance plugin moves to 1.3.0; its tools are unchanged."
      ]
    },
    {
      "date": "2026-10-11",
      "title": "Delete a Workspace by archiving it",
      "branch": "master",
      "commits": {
        "from": "c06ac3e",
        "to": "c06ac3e"
      },
      "touches": [
        "internal/eviedb",
        "internal/web",
        "internal/web/ui",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/workspaces.decisions.md"
      ],
      "text": [
        "The sidebar's \"Delete workspace\", behind a confirmation that says what is kept, archives the Workspace: it leaves the sidebar and the Workspaces page and takes no new sessions or runs, but keeps its sessions, memory and history. Settings → Archived workspaces lists each one with Restore, through new `/api/workspaces/archive` and `/restore` routes that are refused while a turn runs."
      ]
    },
    {
      "date": "2026-10-11",
      "title": "Review cards show the message Evie read",
      "branch": "master",
      "commits": {
        "from": "eae9e11",
        "to": "eae9e11"
      },
      "touches": [
        "internal/eviedb",
        "internal/memory",
        "internal/web/ui"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/memory.decisions.md"
      ],
      "text": [
        "Each inbox proposal now carries the owner message it was prepared from, clipped to 2,000 bytes, and cards are grouped under that message. A card leads with the memory in Claim-text form and folds kind, scope and the Always applies switch into one line; the repeated Evie-proposed warning becomes an \"Evie's wording\" tag explained once."
      ]
    },
    {
      "date": "2026-10-10",
      "title": "Changes: the Changes tab, the review page, and builds in the Inspector",
      "branch": "master",
      "commits": {
        "from": "06115fc",
        "to": "3ae24bb"
      },
      "touches": [
        "internal/changes",
        "internal/localfiles",
        "internal/web",
        "internal/web/ui",
        "internal/plugins",
        "internal/subagents",
        "internal/agent",
        "internal/workflowruntime"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/changes.spec.md#delivery-stories",
        "#177"
      ],
      "text": [
        "A Workspace with a Repository gets a Changes tab that groups Changes into \"Needs your review\", \"In progress\" and \"Merged and closed\". A Change opens on a review page with the action its state calls for and Close, a rail of files with \"Viewed\" marks, checks and origin, and Evie's notes beside a side-by-side or unified diff read through `localfiles.Compare`. A chat's build rows open the Inspector on the Change they made, and in a Workspace chat `propose_build_merge` shows no merge card: the Change waits in the Changes tab, where the owner merges it. The Inspector's folder \"Changes\" tab is renamed \"Working tree\". Still open under #177: when the owner's checkout can't be fast-forwarded, the \"N commits behind\" count is computed but not yet shown."
      ]
    },
    {
      "date": "2026-10-10",
      "title": "Changes: merge, close, and \"Merge when checks pass\"",
      "branch": "master",
      "commits": {
        "from": "de5b0fa",
        "to": "e95629e"
      },
      "touches": [
        "internal/changes",
        "internal/github",
        "internal/eviedb",
        "internal/web",
        "internal/web/ui",
        "internal/subagents",
        "internal/delegation",
        "internal/workflowruntime",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/changes.spec.md#delivery-stories",
        "#177"
      ],
      "text": [
        "`evie changes merge|close` and their routes let the owner act on Changes without a chat. In a local Repository a merge goes through the merge gate, which verifies, rebases and verifies a draft again first; on GitHub it reads the pull request, refuses a draft or a head other than the one Evie last showed, and uses GitHub's \"Rebase and merge\". After any merge Evie deletes its own branch and worktree and fast-forwards the owner's base branch only when the checkout is on it with no uncommitted edits; the owner's own branches are never removed.",
        "\"Merge when checks pass\" arms a pull request so a sync merges it once every check has passed, and clears it on a failing check or a conflict. To carry its notices, the workflow notifications table is rebuilt into one store for workflow and Change notices."
      ]
    },
    {
      "date": "2026-10-10",
      "title": "Changes: push and open pull requests",
      "branch": "master",
      "commits": {
        "from": "6fd68f4",
        "to": "ef007f8"
      },
      "touches": [
        "internal/changes",
        "internal/github",
        "internal/plugins",
        "internal/subagents",
        "internal/delegation",
        "internal/workflow",
        "internal/workflowruntime",
        "internal/web",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/changes.spec.md#delivery-stories",
        "doc:cmd/evie/docs/active/workflow-runtime.decisions.md",
        "#177"
      ],
      "text": [
        "`evie changes push` and `POST /api/changes/push` push one of Evie's draft Changes in a GitHub Repository: the Workspace's Verify runs first in the build's worktree, sandboxed without network, the branch goes to origin with the token as a per-command header, and the pull request opens with Evie's notes and how Verify ended. A live test opened a pull request titled with the build's truncated objective and described with the worker's whole report, so `propose_build_merge` now requires a reviewer-facing title and description that name no local folder.",
        "A new GitHub plugin adds the workflow effect `github.open_change`, which does the same for a workflow's build but requires Verify to pass. Because Repositories differ per Workspace, an effect may now declare one resource prefix (`repo:`) instead of a list."
      ]
    },
    {
      "date": "2026-10-10",
      "title": "Changes: the Change list and GitHub sync",
      "branch": "master",
      "commits": {
        "from": "32c474d",
        "to": "3c88e43"
      },
      "touches": [
        "internal/changes",
        "internal/github",
        "internal/eviedb",
        "internal/web",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/changes.spec.md#delivery-stories",
        "#177"
      ],
      "text": [
        "A Change is one branch of work with its lifecycle state, author, origin, notes, pull request and checks. Evie's own Changes are read from its build records, GitHub pull requests and their checks are stored by a 60-second sync with a refresh route, and a pull request on one of Evie's build branches is that Change on GitHub. `evie changes list` and the list route read both, and a folder guard protects a Workspace whose Changes are unfinished. A live test found that GitHub's Checks API refuses fine-grained tokens, so a pull request's checks are now the newest run of each GitHub Actions workflow on its head commit plus its commit statuses."
      ]
    },
    {
      "date": "2026-10-10",
      "title": "Agents may create branches and worktrees",
      "branch": "master",
      "commits": {
        "from": "6cb75cc",
        "to": "6cb75cc"
      },
      "touches": [
        "AGENTS.md"
      ],
      "sources": [
        "doc:AGENTS.md#git-safety"
      ],
      "text": [
        "`AGENTS.md` lets agents create branches and worktrees as the work needs, such as one worktree per parallel subagent, and remove each once its work is merged. The owner's checkout stays on its branch with its changes in place."
      ]
    },
    {
      "date": "2026-10-10",
      "title": "Changes: a Workspace's Repository",
      "branch": "master",
      "commits": {
        "from": "6cac07f",
        "to": "3e7b57b"
      },
      "touches": [
        "internal/changes",
        "internal/github",
        "internal/worktree",
        "internal/web",
        "internal/web/ui",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/changes.spec.md",
        "doc:cmd/evie/docs/active/changes.decisions.md",
        "#177"
      ],
      "text": [
        "A Workspace's Repository is read from its folder on each request, never stored: local at the top of a git work tree, GitHub when its origin names a github.com repository. With `GITHUB_TOKEN` set, a small GitHub REST client checks the token's access, and Settings → Repository shows the Repository, its base branch and the token status. Settings can publish a local Repository to GitHub, and creating a Workspace offers no repository, a new local one, a new one on GitHub, or connecting an existing GitHub repository. Git commands that reach GitHub run hardened, with the token only as a per-command auth header, never in arguments, git config, the remote URL or output."
      ]
    },
    {
      "date": "2026-10-09",
      "title": "Chat shell reads of Evie's own state accepted",
      "branch": "master",
      "commits": {
        "from": "997bc3f",
        "to": "997bc3f"
      },
      "touches": [
        "cmd/evie/docs"
      ],
      "sources": [
        "doc:cmd/evie/docs/done/bash.decisions.md"
      ],
      "text": [
        "On 2026-10-09 a Finance Workspace chat read `~/.evie/evie.db` and Evie's source through the ungated chat shell. David kept the shell ungated, and the bash decision record names the real alternative, the OS sandbox with an approval gate to leave it, if this is ever revisited."
      ]
    },
    {
      "date": "2026-10-09",
      "title": "Workspace activity: Runs, Workflows, notifications, and run cards",
      "branch": "master",
      "commits": {
        "from": "8cc5ea7",
        "to": "c721272"
      },
      "touches": [
        "internal/web",
        "internal/web/ui",
        "internal/workflow",
        "internal/workflowruntime",
        "internal/eviedb"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/workspace-activity.spec.md",
        "doc:cmd/evie/docs/active/workspace-activity.decisions.md",
        "doc:cmd/evie/docs/active/changes.spec.md",
        "#177",
        "#178"
      ],
      "text": [
        "Specifies Changes (#177) and Workspace activity (#178), then builds the activity side. A Workspace opens on a header with Runs, Workflows and Settings tabs, a badge for runs waiting on the owner, and today's workflow spend against the daily cap, and a notifications icon on every view lists notices from every Workspace. The Runs tab groups runs as Waiting for you, Running and Earlier, each with a bar of its steps, and opens a run in a panel with its question, error, steps, outputs, and Cancel or Rerun. The Workflows tab opens each workflow in a pop-up with its Steps and YAML views, approval and recent runs, and a chat's `workflow_start` call shows a live run card that opens the run in the Inspector. Still open under #178: opening on the Changes tab when something needs review, and counting Changes that need review in the sidebar badge."
      ]
    },
    {
      "date": "2026-10-07",
      "title": "Finance: a scheduled daily workflow, name rules, and transfers out of spending",
      "branch": "master",
      "commits": {
        "from": "8573641",
        "to": "fa59262"
      },
      "touches": [
        "internal/finance",
        "internal/plugins",
        "internal/tools",
        "internal/workflow",
        "internal/workflowruntime",
        "internal/agent",
        "internal/web",
        "internal/web/ui",
        "cmd/evie",
        "cmd/evie/workflows",
        "cmd/finance",
        "scripts"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/finance-daily.spec.md",
        "doc:cmd/evie/docs/active/finance-daily.decisions.md",
        "adr:0067",
        "doc:cmd/finance/docs/active/categorize-patterns.spec.md",
        "doc:cmd/evie/docs/active/spending.decisions.md"
      ],
      "text": [
        "The daily finance job was a launchd cron job running a separately installed `finance` binary that had drifted two months behind source, so two banks wedged on foreign-key failures and categorize was skipped on those days. Finance becomes the first production workflow effect provider: `finance.sync_ledger` and `finance.categorize_ledger` record a receipt per idempotency key, and the `finance-daily` workflow syncs, categorizes and checks for failed banks at 09:00 America/Detroit. `evie serve` runs as a KeepAlive LaunchAgent so the scheduler is up with no terminal open, the old cron job's removal is recorded, and `scripts/release.sh` verifies, installs and restarts `evie serve` in one step.",
        "Transactions with no merchant name, such as card payments, transfers and payroll, could never match a merchant rule, so `name_rules` adds prioritized SQL `LIKE` patterns on the bank string, and `excluded` becomes a real category for money that deliberately does not count as spending; on the live database, transactions awaiting review fell from 339 to 125. The Spending overview then leaves excluded allocations out of every total, and card payments get their own Credit Card Payments category, which the overview omits on both legs."
      ]
    },
    {
      "date": "2026-10-04",
      "title": "Built-in deep research v2",
      "branch": "master",
      "commits": {
        "from": "023f21a",
        "to": "023f21a"
      },
      "touches": [
        "internal/workflow",
        "internal/workflowruntime",
        "internal/eviedb",
        "internal/web",
        "internal/web/ui",
        "internal/agent",
        "internal/plugins",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/workflow-runtime.decisions.md",
        "doc:docs/memory-architecture.md"
      ],
      "text": [
        "Deep research is built into Evie and runs in every Workspace without a proposal or approval, because a built-in holds no Standing Authority: no capability, schedule or build worker, enforced by the validator and the database. Each built-in version is recorded in procedural Git before a run pins it; a name resolves to the Workspace's approved copy, then the Global one, then the built-in; and the owner can disable built-ins or customize them. Deep research v2 searches several angles under a 15-source budget, extracts quoted claims, and puts each claim to three adversarial verifiers that count only evidence the harness saw, with honest inconclusive outcomes. The Subagents plugin is on by default for new databases."
      ]
    },
    {
      "date": "2026-10-04",
      "title": "The /model picker and collapsed activity",
      "branch": "master",
      "commits": {
        "from": "a223a4f",
        "to": "a223a4f"
      },
      "touches": [
        "internal/web/ui",
        "scripts"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/chat-model-selection.spec.md",
        "doc:cmd/evie/docs/active/chat-activity.spec.md"
      ],
      "text": [
        "The always-visible model dropdown becomes a searchable `/model` picker opened from the composer and grouped by provider. The working trace collapses into a live status line with a single summary for settled tool problems."
      ]
    },
    {
      "date": "2026-10-04",
      "title": "Workflow runtime: agent and map steps, deep research, schedules, and retention",
      "branch": "master",
      "commits": {
        "from": "4d58f14",
        "to": "fbe2c73"
      },
      "touches": [
        "internal/workflowruntime",
        "internal/workflow",
        "internal/eviedb",
        "internal/subagents",
        "internal/delegation",
        "internal/plugins",
        "internal/agent",
        "internal/web",
        "internal/web/ui",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/workflow-runtime.spec.md",
        "doc:cmd/evie/docs/active/workflow-runtime.decisions.md",
        "adr:0020",
        "adr:0023",
        "adr:0030",
        "adr:0045",
        "adr:0046"
      ],
      "text": [
        "Stage 14 sub-stages 9 to 12 of the memory architecture plan. Agent steps run research, explore and build workers owned by the run, and map steps fan out over a list with bounded parallelism and per-item recovery. The deep-research template ships with verifiers that must fetch their evidence and a deterministic citation check, and a Workflows plugin gives chats `workflow_start` and `workflow_status`; nothing model-facing proposes, approves or cancels a run.",
        "Approved schedules start runs with bounded catch-up after downtime, a daily cap limits all workflow runs' AI calls and workers (by default $5 and 10 million tokens), and run transitions record durable notifications. Unfinished runs can migrate to successor runs, and retention policies apply audited redaction, deletion and checkpoint compaction to finished runs."
      ]
    },
    {
      "date": "2026-10-04",
      "title": "Workflow runtime: capability reads and effects, AI steps, and cancellation",
      "branch": "master",
      "commits": {
        "from": "1a98fe0",
        "to": "d01b83f"
      },
      "touches": [
        "internal/workflowruntime",
        "internal/workflow",
        "internal/eviedb",
        "internal/plugins",
        "internal/web",
        "internal/web/ui",
        "internal/agent",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/workflow-runtime.decisions.md",
        "adr:0009",
        "adr:0010",
        "adr:0015",
        "adr:0017",
        "adr:0041"
      ],
      "text": [
        "Stage 14 sub-stages 5 to 8. Plugins contribute workflow capability reads through a focused provider role, starting with fixed reads of the local finance database; runs pin each provider and pause when one is missing or changed until the owner approves an audited substitution. Effects run through a durable effect protocol, where an interrupted effect's outcome is unknown and moves the run to reconciliation rather than a rerun. Interrupt steps wait for the owner's typed answer from any surface, and AI steps send only the reviewed prompt and declared input to the exact model of their Model Policy. The owner can cancel runs, revoked runs pause at node boundaries, and a succeeded Run Key reruns only under an audited override."
      ]
    },
    {
      "date": "2026-10-04",
      "title": "Workflow runtime: validate, approve, run, and recover",
      "branch": "master",
      "commits": {
        "from": "5166b3a",
        "to": "9c7dcdb"
      },
      "touches": [
        "internal/workflow",
        "internal/workflowruntime",
        "internal/procedural",
        "internal/eviedb",
        "internal/web",
        "internal/web/ui",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/workflow-runtime.spec.md",
        "doc:cmd/evie/docs/active/workflow-runtime.decisions.md",
        "doc:docs/memory-architecture.md",
        "adr:0007",
        "adr:0008",
        "adr:0011",
        "adr:0018",
        "adr:0021"
      ],
      "text": [
        "Stage 14 sub-stages 1 to 4 of the memory architecture plan. `workflow.yaml` definitions are parsed strictly, validated, hashed and simulated with effects suppressed (`evie workflows validate|simulate`). Definitions are proposed into a private procedural Git repository, and one owner approval activates one exact executable and authority hash within the Workspace's access, granting standing authority without starting a run. Runs of a Workspace's active workflow pin their version and record a checkpoint before each node. They advance under a fenced lease, so the REPL, `evie serve` and CLI starts share one database, and a background scheduler resumes a dead holder's run from its checkpoint."
      ]
    },
    {
      "date": "2026-10-04",
      "title": "Build workers, the merge gate, and Task Tree orchestration",
      "branch": "master",
      "commits": {
        "from": "b754c92",
        "to": "a796c99"
      },
      "touches": [
        "internal/subagents",
        "internal/delegation",
        "internal/worktree",
        "internal/plugins",
        "internal/tools",
        "internal/eviedb",
        "internal/agent",
        "internal/task",
        "internal/memory",
        "internal/web",
        "internal/web/ui",
        "cmd/evie",
        "cmd/todo"
      ],
      "sources": [
        "doc:docs/memory-architecture.md",
        "doc:cmd/evie/docs/active/subagents.spec.md",
        "doc:cmd/evie/docs/active/subagents.decisions.md",
        "term:Task Tree"
      ],
      "text": [
        "Stages 11 to 13 of the memory architecture plan. `delegate_build` gives a build worker one bounded code change on one Task, with free edits inside a harness-created git worktree on a fresh `evie/` branch: file tools are confined through `os.Root`, and bash runs only under Seatbelt or bubblewrap with no network. Workspaces need an explicit build setting, off by default. `propose_build_merge` verifies the worker's branch with the owner's verify command, rebases it onto its target, verifies again, and only then shows an approval card; on approval the harness fast-forwards the target without stashing, resetting or pushing. Tasks gain dependencies and paths, and `todo_ready` returns a tree's dispatchable Tasks in conflict-free batches sized to the worker caps."
      ]
    },
    {
      "date": "2026-10-03",
      "title": "Memory architecture Stages 6 to 10: recall quality and context management",
      "branch": "master",
      "commits": {
        "from": "a82bb8f",
        "to": "3f41a54"
      },
      "touches": [
        "internal/agent",
        "internal/eviedb",
        "internal/memory",
        "internal/memoryeval",
        "internal/memorybackground",
        "internal/subagents",
        "internal/delegation",
        "internal/plugins",
        "internal/tools",
        "internal/web",
        "internal/web/ui",
        "cmd/evie",
        "cmd/todo"
      ],
      "sources": [
        "doc:docs/memory-architecture.md",
        "doc:docs/context-management-comparison.md",
        "doc:cmd/evie/docs/active/memory.spec.md#memory-flush-before-compaction",
        "doc:cmd/evie/docs/active/subagents.spec.md"
      ],
      "text": [
        "Read-only explore workers (`delegate_explore`) get `read_file` confined to a pinned folder, the read-only `query_db`, and a bash that parses each command line itself and runs only allowlisted read-only commands without a shell. By default a Workspace allows explore exactly while it has a local folder, confined to that folder.",
        "When a request cannot fit and no whole-turn prefix reaches the compaction target, the active turn's oldest completed tool groups are summarized into a labelled block, keeping the newest two verbatim. Requests that offer `read_tool_result` show at most 32 KiB per tool result, as a head-and-tail excerpt that names the omitted range, and memory, todo and delegation results are protected from shortening. Every committed compaction offers its covered owner messages to the background memory pass, owed durably across restarts.",
        "Messages that are the source of an accepted Claim are indexed under that Claim's words, and a deterministic parser reads one explicit past time (\"last week\", \"in March\") so search reads that window first. Automatic Recall now holds accepted Claims, their companions and dense-only hits to the same relevance floor as conversation excerpts."
      ]
    },
    {
      "date": "2026-10-03",
      "title": "Memory architecture plan and Stages 1 to 5",
      "branch": "master",
      "commits": {
        "from": "d531b17",
        "to": "5ec2a41"
      },
      "touches": [
        "internal/memory",
        "internal/memorybackground",
        "internal/eviedb",
        "internal/agent",
        "internal/plugins",
        "internal/tools",
        "internal/localextractor",
        "internal/openrouter",
        "internal/web",
        "internal/web/ui",
        "cmd/evie",
        "docs"
      ],
      "sources": [
        "doc:docs/memory-architecture.md",
        "doc:cmd/evie/docs/active/memory.spec.md#standing-memory",
        "doc:cmd/evie/docs/active/memory.spec.md#proactive-memory",
        "doc:cmd/evie/docs/active/memory.spec.md#background-memory-pass",
        "doc:cmd/evie/docs/active/memory.decisions.md"
      ],
      "text": [
        "Writes down memory architecture research and a staged plan in `docs/memory-architecture.md`, then extends it with context-management and orchestration stages. Each accepted compaction now writes or advances its session's episode, which `memory.search_episodes` searches on demand. Approved standing memories reach every request by rule under a hard budget; every kind but plain facts is standing by default, and the owner can switch that before approving.",
        "The system prompt states Evie's profile and Workspace memory duties. A `remember` call shows the approval card only when the owner explicitly asked; other proposals queue in a review inbox, and `memory_overview` returns the standing set with current profile and Workspace memories in one read. A background pass, off unless `EVIE_REMOTE_MEMORY=on` or a local extractor is configured, reads the owner's messages after a conversation goes idle or every 20 owner messages, decides whether each candidate adds, updates or invalidates a memory, and queues every change for review; nothing applies without approval."
      ]
    },
    {
      "date": "2026-10-01",
      "title": "Harness review: final verification pass",
      "branch": "master",
      "commits": {
        "from": "2e46764",
        "to": "cf636a6"
      },
      "touches": [
        "internal/agent",
        "internal/eviedb",
        "internal/subagents",
        "internal/tools",
        "internal/memoryeval",
        "internal/openrouter",
        "internal/memory",
        "internal/web",
        "internal/web/ui",
        "cmd/evie"
      ],
      "sources": [
        "doc:docs/harness-review-2026-09-30.md",
        "doc:cmd/evie/docs/active/memory-scale-eval.md"
      ],
      "text": [
        "The harness review's final verification pass closes the gaps it found. Tool fences frame cross-host redirect targets, cap bash spill files at 64 MiB, back the evie database's table fence with an EXPLAIN-based check, and judge NAT64 and 6to4 addresses by their embedded IPv4. Memory binds owner spans to the sentence that states the Claim, through one shared clause and subject analysis that fails safe, narrows free text in operation history outside its scope, and treats conversational filler and acknowledgements as low content.",
        "A context-length rejection after a same-iteration compaction now ends the turn as `context_overflow`, and turns that ended without terminal evidence no longer block compaction. Subagents retry terminal writes on lock contention and refuse opening child sessions, and web tool errors use fixed harness wording instead of server-controlled text."
      ]
    },
    {
      "date": "2026-10-01",
      "title": "Harness review: context budgeting, web turn controls, and the delegation contract",
      "branch": "master",
      "commits": {
        "from": "06e8b72",
        "to": "b1a4bd3"
      },
      "touches": [
        "internal/agent",
        "internal/subagents",
        "internal/delegation",
        "internal/eviedb",
        "internal/plugins",
        "internal/openrouter",
        "internal/untrusted",
        "internal/tools",
        "internal/memory",
        "internal/web",
        "internal/web/ui",
        "cmd/evie"
      ],
      "sources": [
        "doc:docs/harness-review-2026-09-30.md",
        "doc:cmd/evie/docs/active/subagents.spec.md",
        "doc:cmd/evie/docs/active/subagents.implementation.md"
      ],
      "text": [
        "Automatic compaction plans against a 20 KiB summary reserve and measures the real summary before committing it, token budgets convert to bytes with a per-model ratio calibrated from recorded input tokens, and a provider context-length rejection triggers one compact-and-retry per turn. Requests are ordered from most to least stable, with cache breakpoints for Anthropic and Qwen routes; `read_tool_result` rereads the session's durable tool outcomes; and the rolling summary is sent as a labelled data block. The browser gains a Stop button and a `/compact` command.",
        "Subagents are budgeted by time (15 minutes) and tokens (1M), end with a tool-free wrap-up report at 90 percent of either or under context pressure, and return their summary inline with the rest paged through `read_subagent_report`. `continue_research` extends a finished child under the current turn. Child-written text reaches the parent inside the escaped untrusted-data frame, now shared from `internal/untrusted`, and delegated child sessions are no longer listed as owner sessions."
      ]
    },
    {
      "date": "2026-10-01",
      "title": "Harness review: stale facts, the tool loop, and memory authority",
      "branch": "master",
      "commits": {
        "from": "f80bd74",
        "to": "f67dead"
      },
      "touches": [
        "internal/eviedb",
        "internal/agent",
        "internal/memory",
        "internal/memoryeval",
        "internal/openrouter",
        "internal/subagents",
        "internal/finance",
        "internal/web/ui",
        "cmd/evie"
      ],
      "sources": [
        "doc:docs/harness-review-2026-09-30.md",
        "doc:cmd/evie/docs/active/memory-scale-eval.md",
        "doc:cmd/evie/docs/active/memory.decisions.md"
      ],
      "text": [
        "Conversation excerpts now carry historical claims recomputed on each read, so a corrected source stays retrievable but is marked superseded, and newer owner statements are linked by deterministic wording rules; in the scale evaluation, stale misses fell from 20 to 2. Each turn is capped at `EVIE_TURN_STEP_LIMIT` model responses (default 100), the last requested without tools, and a cancellation during the tool phase can no longer deadlock. Proposed memories bind to the quoted owner span that states them, and correction sources and legacy whole-message sources are narrowed outside their scope. Concurrent finance writes now wait on lock contention instead of failing."
      ]
    },
    {
      "date": "2026-10-01",
      "title": "Harness review: findings, scale evaluation, provider hardening, and recall relevance",
      "branch": "master",
      "commits": {
        "from": "346bed5",
        "to": "83c8043"
      },
      "touches": [
        "internal/memoryeval",
        "internal/openrouter",
        "internal/agent",
        "internal/eviedb",
        "internal/memory",
        "cmd/evie"
      ],
      "sources": [
        "doc:docs/harness-review-2026-09-30.md",
        "doc:cmd/evie/docs/active/memory-scale-eval.md"
      ],
      "text": [
        "Records the 2026-09-30 harness review's findings and staged plan. A seeded synthetic history of over 2,000 messages replays through real agent turns, SQLite and the memory tools to score recall precision, unwanted items and stale facts, and its baselines record that every target is unmet. Provider streaming rejects truncated or errored completions before they are committed, gains idle and connection timeouts, and retries no-response, 429, 502, 503 and 504 failures up to twice before any output. Automatic Recall no longer searches on low-content messages, requires excerpts to match a distinctive request term, and stops re-injecting messages still in the request."
      ]
    },
    {
      "date": "2026-10-01",
      "title": "Harness review: tool fences and subagent supervision",
      "branch": "master",
      "commits": {
        "from": "9a4f75d",
        "to": "ca12abe"
      },
      "touches": [
        "internal/tools",
        "internal/subagents",
        "internal/eviedb",
        "cmd/evie"
      ],
      "sources": [
        "doc:docs/harness-review-2026-09-30.md",
        "doc:cmd/evie/docs/done/bash.decisions.md",
        "doc:cmd/evie/docs/done/file-tools.decisions.md"
      ],
      "text": [
        "The first fixes from the harness review harden the tool fences: finance `query_db` accepts a single SELECT on a connection that cannot attach or write files, research workers' `web_fetch` refuses loopback and private addresses after DNS resolution, third-party text has its frame markers escaped, `edit_file` through a symlink edits the target, and bash spills output past its cap to a file. Subagent supervision proves parent authority by the intent's place in the current turn, applies each child's deadline from its own start, retries SQLite lock contention instead of ending authority, and recovers each attempt in its own transaction, so unrecoverable records no longer stop startup."
      ]
    },
    {
      "date": "2026-09-29",
      "title": "Spending accounts, the chat model selector, and Workspace research",
      "branch": "master",
      "commits": {
        "from": "688c786",
        "to": "fb23624"
      },
      "touches": [
        "internal/finance",
        "internal/web",
        "internal/web/ui",
        "internal/eviedb",
        "internal/openrouter",
        "internal/subagents",
        "internal/tools",
        "internal/plugins",
        "internal/agent",
        "internal/delegation",
        "cmd/evie",
        "AGENTS.md"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/spending-accounts-and-navigation.spec.md",
        "doc:cmd/evie/docs/active/spending.spec.md",
        "doc:cmd/evie/docs/active/chat-model-selection.spec.md",
        "doc:cmd/evie/docs/active/workspace-research.spec.md"
      ],
      "text": [
        "Data → Spending shows connected banks and their authorized accounts, refreshed from Plaid only on request, and clicking a calendar day opens its transactions, whose categories can be edited in place. The sidebar drops its top-level New chat and Workspaces buttons. The composer gains a model dropdown grouped by provider, and a chat's model is saved in SQLite for its later turns. Standard Workspaces get an explicit research-delegation setting, off by default, and research workers get a 1 MiB request budget and bounded section and excerpt reading of fetched pages. `AGENTS.md` drops its derivable repository map."
      ]
    },
    {
      "date": "2026-09-21",
      "title": "Workspace setup and session archive",
      "branch": "master",
      "commits": {
        "from": "ea9fa97",
        "to": "ea9fa97"
      },
      "touches": [
        "internal/web",
        "internal/web/ui",
        "internal/eviedb",
        "internal/memory",
        "cmd/evie",
        "cmd/todo"
      ],
      "sources": [
        "doc:cmd/evie/docs/done/workspace-setup-and-session-archive.spec.md"
      ],
      "text": [
        "Workspaces are created in a modal with a persisted agent preset and the native macOS folder picker. Settings gains session archive and restore controls and font preferences."
      ]
    },
    {
      "date": "2026-09-21",
      "title": "Spending dashboard and folder-backed Workspace tools",
      "branch": "master",
      "commits": {
        "from": "70c4c8a",
        "to": "70c4c8a"
      },
      "touches": [
        "internal/finance",
        "internal/localfiles",
        "internal/localterminal",
        "internal/repoinstructions",
        "internal/agent",
        "internal/tools",
        "internal/memory",
        "internal/openrouter",
        "internal/eviedb",
        "internal/web",
        "internal/web/ui",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/spending.spec.md",
        "doc:cmd/evie/docs/active/workspace-local-folder.spec.md",
        "doc:cmd/evie/docs/active/repository-instructions.spec.md"
      ],
      "text": [
        "Data gains a Spending dashboard: monthly cash-flow charts, daily calendar navigation, saved transaction classification views, and an explicit bank refresh. The same commit brings the Workspace Inspector, repository instructions, local file and terminal tools, model compatibility, and subagent context updates."
      ]
    },
    {
      "date": "2026-09-11",
      "title": "Tool details and memory results move to the Inspector",
      "branch": "master",
      "commits": {
        "from": "19a57cd",
        "to": "f1a1a25"
      },
      "touches": [
        "internal/web/ui",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/chat-activity.spec.md",
        "doc:cmd/evie/docs/active/chat-activity.decisions.md"
      ],
      "text": [
        "Generic tool arguments and results move from the chat to the selected action's Inspector, which shows useful details first and the exact strings behind Debug details, closed by default; the chat keeps compact action rows. Recorded memory results are decoded for display, so values, scope, status and search outcomes appear before Debug details."
      ]
    },
    {
      "date": "2026-09-11",
      "title": "Memory Stage 5, merged into the subagents work",
      "branch": "codex/memory-stage-5",
      "merge": "d426061",
      "touches": [
        "internal/agent",
        "internal/eviedb",
        "internal/memory",
        "internal/localembedding",
        "internal/plugins",
        "internal/subagents",
        "internal/tools",
        "internal/web",
        "internal/web/ui",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/memory-stage-5-retrieval.spec.md",
        "doc:cmd/evie/docs/active/memory-stage-5-release-assessment.md",
        "doc:cmd/evie/docs/active/subagents.implementation.md",
        "#176",
        "#154",
        "#156",
        "#157",
        "#158",
        "#159",
        "#160",
        "#161",
        "#162",
        "#163",
        "#164",
        "#165",
        "#166",
        "#167",
        "#168"
      ],
      "text": [
        "Brings Memory Stage 5 (pull request #176, bounded recall and a failed release assessment) into the subagents work. Ordinary messages now receive bounded recall: accepted evidence with its original sources, scoped conversation excerpts with safely expanded context, historical and conflicting evidence, contextual references resolved to eligible identities, reusable turn investigations, source-supported relationships, and a measured local semantic retrieval, and each answer's original requests can be inspected. The frozen held-out assessment then failed its release gates (1,051 of 1,077 passing), and the failure is recorded rather than tuned away.",
        "The merge keeps delegated sessions out of memory: child sessions get no automatic or model-directed retrieval, and conversation indexing excludes delegated lineage, so assignments never become owner statements."
      ]
    },
    {
      "date": "2026-09-11",
      "title": "Bounded parallel research subagents",
      "branch": "master",
      "commits": {
        "from": "a24b45a",
        "to": "a24b45a"
      },
      "touches": [
        "internal/subagents",
        "internal/delegation",
        "internal/eviedb",
        "internal/plugins",
        "internal/agent",
        "internal/memory",
        "internal/openrouter",
        "internal/tools",
        "cmd/evie",
        "cmd/todo"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/subagents.spec.md",
        "doc:cmd/evie/docs/active/subagents.decisions.md",
        "doc:cmd/evie/docs/active/subagents.implementation.md",
        "#155",
        "#169",
        "#170",
        "#171",
        "#172",
        "#173",
        "#174",
        "#175"
      ],
      "text": [
        "The foundation of subagents: an optional Subagents plugin gives the chat `delegate_research`, whose assignments run in parallel as research workers limited to web search and fetch, each with its own history and trusted worker instructions. Each assignment is one durable attempt per parent and key, admitted only for the committed invocation, fenced together with the parent, and recovered without being run again. The memory compiler rejects delegated lineage, so a child's text never becomes owner evidence."
      ]
    },
    {
      "date": "2026-09-10",
      "title": "GPT-6 Astra, the activity timeline, and file inspection",
      "branch": "master",
      "commits": {
        "from": "4098721",
        "to": "f27546d"
      },
      "touches": [
        "internal/openrouter",
        "internal/agent",
        "internal/eviedb",
        "internal/memory",
        "internal/web",
        "internal/web/ui",
        "cmd/evie",
        "cmd/todo"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/gpt-6-astra.spec.md",
        "doc:cmd/evie/docs/active/gpt-6-astra.decisions.md",
        "doc:docs/gpt-6-astra-migration-plan.md",
        "doc:cmd/evie/docs/active/chat-activity.spec.md",
        "adr:0066"
      ],
      "text": [
        "Evie's default model becomes `openai/gpt-6-astra`, through OpenRouter's stateless Responses endpoint, keeping OpenRouter and preserving response phases and compaction boundaries; Kimi stays available through Chat Completions. The chat replaces raw tool cards with a collapsible activity timeline, file actions open in a side Inspector with highlighted source and side-by-side changes, and the file viewer and Workspace navigation are simplified. The range also adds the memory Stage 5 retrieval and subagents specifications and an agent orchestration research draft."
      ]
    },
    {
      "date": "2026-09-06",
      "title": "Usage, restored conversation history, and the architecture record",
      "branch": "master",
      "commits": {
        "from": "7323826",
        "to": "7323826"
      },
      "touches": [
        "internal/usage",
        "internal/eviedb",
        "internal/memory",
        "internal/web",
        "internal/web/ui",
        "internal/plugins",
        "internal/agent",
        "internal/localextractor",
        "cmd/evie",
        "docs/adr",
        "docs/architecture",
        "CONTEXT.md",
        "AGENTS.md"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/usage.spec.md",
        "doc:cmd/evie/docs/active/session-history-and-memory-scope.spec.md",
        "doc:cmd/evie/docs/active/workspaces.spec.md",
        "doc:cmd/evie/docs/active/workflow-runtime.spec.md",
        "adr:0022",
        "adr:0047"
      ],
      "text": [
        "A large snapshot commit. Data gains Usage, showing Codex account activity and Evie's recorded conversation token usage with explicit coverage. Reopening a saved session restores its messages and recorded tool outcomes, and Evie recommends where a memory applies (everywhere, this Workspace, or this conversation), resolved by the harness and confirmed on approval.",
        "It also adds the `CONTEXT.md` glossary, 50 architecture decision records, specifications for Workspaces, the Workflow Runtime and semantic memory Stages 3 and 4, memory architecture diagrams and research notes, and commits local run output under `.scratch/` and `tmp/`."
      ]
    },
    {
      "date": "2026-09-05",
      "title": "Workspace shell and the Data hub",
      "branch": "master",
      "commits": {
        "from": "2aad5a7",
        "to": "2aad5a7"
      },
      "touches": [
        "internal/web",
        "internal/web/ui",
        "internal/eviedb",
        "internal/memory",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/ui-chat.spec.md",
        "doc:cmd/evie/docs/active/serve.decisions.md"
      ],
      "text": [
        "The browser becomes a scope-aware three-pane workbench: left navigation for New chat, Data and Workspaces, center tabs for chat and Workspace pages, and one collapsible right Inspector. Data is a typed hub whose first sources are Database, with live schema topology and records for an allowlist of tables, and Memory, a graph of current Claims and Entities in one exact scope and pinned revision."
      ]
    },
    {
      "date": "2026-09-05",
      "title": "Web chooser no longer crashes on empty lists",
      "branch": "codex/memory-stage-4",
      "merge": "8d45aa0",
      "touches": [
        "internal/web"
      ],
      "sources": [
        "code:internal/web/context_sessions.go",
        "#153"
      ],
      "text": [
        "The context-session list route returned `null` for an empty list of Workspaces, projects or sessions, which crashed the web chooser at startup. It now returns empty arrays, and a test holds the route to that."
      ]
    },
    {
      "date": "2026-09-05",
      "title": "Memory Stage 4: durable memory compilation and owner review",
      "branch": "codex/memory-stage-4",
      "merge": "fac58ff",
      "touches": [
        "internal/eviedb",
        "internal/memory",
        "internal/localextractor",
        "internal/memoryeval",
        "internal/agent",
        "internal/tools",
        "internal/web",
        "internal/web/ui",
        "cmd/evie",
        "scripts"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/semantic-memory-stage-4.spec.md",
        "doc:cmd/evie/docs/active/memory.spec.md#stage-4---durable-asynchronous-memory-compiler",
        "doc:cmd/evie/docs/active/memory-stage-4-evidence-contract.decisions.md",
        "doc:cmd/evie/docs/active/memory-stage-4-work-contract.decisions.md",
        "doc:cmd/evie/docs/active/memory-stage-4-review-contract.decisions.md",
        "adr:0062",
        "adr:0063",
        "adr:0064",
        "adr:0065",
        "#152",
        "#132",
        "#133",
        "#134",
        "#135",
        "#136",
        "#137",
        "#138",
        "#139",
        "#140",
        "#141",
        "#142",
        "#143",
        "#144",
        "#145",
        "#146",
        "#147",
        "#148",
        "#149",
        "#150",
        "#151"
      ],
      "text": [
        "Adds the durable memory compiler. After defining Stage 4's evidence, durable work and owner review contracts, the branch persists bounded owner-assertion candidates, recovers fenced compiler work across processes, and lets background compilation be activated at explicit frontiers, with bounded historical backfill and coverage inspection.",
        "The owner reviews candidates in a web inbox independently of their source sessions: simple candidates, sourced identities and Predicate definitions, temporal changes, local clock observations, and edited interpretations with dependent batches, and review decisions are preserved across compiler generations. Compiler health and measured progress are exposed, and tooling measures the local extractor, proves deterministic conformance, measures the workload, and assesses release evidence.",
        "Experimental acceptance stayed open (#135, #136, #150, #151 and the parent #131): no adequate extractor was selected, and model-output adjudication, the owner pilot and the numerical release gates are still pending, so the merge did not activate compilation (#152)."
      ]
    },
    {
      "date": "2026-09-04",
      "title": "Todo and YouTube plugins, and durable Task Trees",
      "branch": "codex/todo-youtube-task-trees",
      "merge": "9ef0771",
      "touches": [
        "internal/task",
        "internal/taskeval",
        "internal/plugins",
        "internal/eviedb",
        "internal/agent",
        "internal/tools",
        "internal/todo",
        "internal/openrouter",
        "internal/web",
        "cmd/todo",
        "cmd/evie"
      ],
      "sources": [
        "term:Task Tree",
        "adr:0004",
        "#130",
        "#117",
        "#118",
        "#119",
        "#120",
        "#121",
        "#122",
        "#123",
        "#124",
        "#125",
        "#126",
        "#127",
        "#128",
        "#129"
      ],
      "text": [
        "YouTube and Todo become first-party plugins, and Todo gains durable Global Tasks with a retained lifecycle, idempotent mutations, and recursive Task Trees. Work on a Task is claimed by its active execution, the legacy Todo list is imported exactly once, Task Trees are scoped and focused, and delegated sessions can be granted a Task subtree. The `todo` CLI moves onto the shared Task store, and Evie can focus and delegate Task Trees."
      ]
    },
    {
      "date": "2026-09-02",
      "title": "Memory Stage 3: deterministic semantic memory",
      "branch": "codex/memory-stage-3",
      "merge": "5e5e43d",
      "touches": [
        "internal/eviedb",
        "internal/memory",
        "internal/memoryeval",
        "internal/agent",
        "internal/plugins",
        "internal/tools",
        "internal/web",
        "internal/web/ui",
        "cmd/evie",
        "scripts"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/semantic-memory-stage-3.spec.md",
        "doc:cmd/evie/docs/active/semantic-memory-encodings.spec.md",
        "doc:cmd/evie/docs/active/memory.spec.md#stage-3---temporal-graph-domain-and-explicit-memory",
        "adr:0047",
        "adr:0052",
        "adr:0056",
        "#115",
        "#83",
        "#85",
        "#103",
        "#104",
        "#105",
        "#106",
        "#107",
        "#108",
        "#109",
        "#110",
        "#111",
        "#112",
        "#113",
        "#114"
      ],
      "text": [
        "Implements deterministic semantic memory on frozen encodings. Evie remembers one sourced literal Claim at a time, creates Entities and resolves aliases, enforces exact Claim semantics, corrects Claims across time, and retires and restores them through a reversible lifecycle, with semantic scope isolation and exact neighborhood links. Semantic projections can be recovered, focused memory tools and owner inspection surfaces expose the state, and evaluation baselines and an acceptance test prove the stage. The branch also adds creating and entering Workspace sessions."
      ]
    },
    {
      "date": "2026-09-01",
      "title": "Plugin system Phase 1",
      "branch": "codex/plugin-system-phase-1-core",
      "merge": "751e19e",
      "touches": [
        "internal/plugins",
        "internal/composition",
        "internal/eviedb",
        "internal/tools",
        "internal/agent",
        "internal/web",
        "internal/web/ui",
        "cmd/evie",
        "docs/adr"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/plugins.spec.md",
        "adr:0001",
        "adr:0004",
        "adr:0006",
        "adr:0013",
        "adr:0014",
        "#101",
        "#70",
        "#74",
        "#75",
        "#76",
        "#77",
        "#78",
        "#79",
        "#80"
      ],
      "text": [
        "Specifies the plugin system's first phase and records its decisions as the first architecture decision records. Session toolsets are pinned; Web and Finance load as compiled first-party plugins with a reversible lifecycle; sessions are composed from a standard preset and keep a composition receipt; and compatible session resumes are audited. The owner manages plugins and built-in presets from a management page."
      ]
    },
    {
      "date": "2026-08-30",
      "title": "GitHub issue tracker configured",
      "branch": "master",
      "commits": {
        "from": "9740c71",
        "to": "9740c71"
      },
      "touches": [
        "docs/agents"
      ],
      "sources": [
        "doc:docs/agents/issue-tracker.md"
      ],
      "text": [
        "Adds `docs/agents/issue-tracker.md`: issues for this repository live on GitHub and are handled with the `gh` CLI."
      ]
    },
    {
      "date": "2026-08-30",
      "title": "Memory Stage 2: bounded context and durable compaction",
      "branch": "codex/stage-2-context-management",
      "merge": "d5c1f1a",
      "touches": [
        "internal/agent",
        "internal/eviedb",
        "internal/memory",
        "internal/openrouter",
        "internal/web",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/memory.spec.md#stage-2---working-context-and-compaction",
        "#69",
        "#64",
        "#65",
        "#66",
        "#67",
        "#68"
      ],
      "text": [
        "Separates durable history from what the model sees. Each request's conversational context is composed within a bound, tool results enter it within their own bound, and eligible turns compact into a durable rolling summary: manually first, then automatically before a request, with compaction chains that resume after a restart."
      ]
    },
    {
      "date": "2026-08-30",
      "title": "Route-safe context profile",
      "branch": "master",
      "commits": {
        "from": "ae4cda3",
        "to": "02a7545"
      },
      "touches": [
        "internal/openrouter",
        "internal/agent",
        "internal/web",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/memory.decisions.md",
        "#63"
      ],
      "text": [
        "Each process resolves one immutable context profile before opening or resuming a session, and the profile bounds every conversational request. Without an explicit window override, startup looks up the model and its endpoints on OpenRouter and takes the smallest context length among active endpoints that can honor the output reserve as the route-safe window. Malformed metadata fails discovery, and other discovery failures fall back to a built-in window only for Evie's built-in model."
      ]
    },
    {
      "date": "2026-08-25",
      "title": "Delivery loop taken out of AGENTS.md",
      "branch": "master",
      "commits": {
        "from": "0be3ba0",
        "to": "0be3ba0"
      },
      "touches": [
        "AGENTS.md",
        "docs",
        "cmd/evie/docs"
      ],
      "sources": [
        "doc:AGENTS.md"
      ],
      "text": [
        "Removes the delivery loop the same day it arrived: its section in `AGENTS.md` and `docs/delivery-loop.md` go, and the backlog and research notes stop referring to it."
      ]
    },
    {
      "date": "2026-08-25",
      "title": "Simplified delivery workflow",
      "branch": "codex/simplify-delivery-loop",
      "merge": "edee820",
      "touches": [
        "AGENTS.md",
        "scripts",
        ".github/workflows",
        ".agents/skills",
        "docs"
      ],
      "sources": [
        "doc:scripts/verify-change.sh",
        "doc:.github/workflows/verify.yml"
      ],
      "text": [
        "Replaces most of the custom story skills, reviewer agents and planning documents with a lighter loop: a concise `AGENTS.md`, `docs/delivery-loop.md`, a pull request template, a GitHub Actions Verify workflow, and `scripts/verify-change.sh` as the deterministic check."
      ]
    },
    {
      "date": "2026-08-25",
      "title": "Memory Epic 1 complete",
      "branch": "master",
      "commits": {
        "from": "af74b05",
        "to": "af74b05"
      },
      "touches": [
        "cmd/evie/docs",
        "docs"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/memory/epic_1/README.md"
      ],
      "text": [
        "Records Epic 1, restart-safe leased REPL sessions, as completed on 2026-08-25, updating its stories, the memory spec and decisions, and the backlog."
      ]
    },
    {
      "date": "2026-08-25",
      "title": "Provider token usage evidence",
      "branch": "codex/61-persist-provider-usage",
      "merge": "dcbbe57",
      "touches": [
        "internal/agent",
        "internal/openrouter",
        "internal/memory",
        "internal/eviedb"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/memory/epic_1/story_4/README.md",
        "#62",
        "#61"
      ],
      "text": [
        "Epic 1 story 4: usage reported by the provider is persisted as immutable, provider-neutral evidence on each accepted assistant message, for later context and evaluation diagnostics. Follow-up fixes clarify usage boundaries, preserve negative zero, and normalize authoritative absence."
      ]
    },
    {
      "date": "2026-08-24",
      "title": "Select and resume titled REPL sessions",
      "branch": "codex/59-select-resume-titled-repl-sessions",
      "merge": "5593518",
      "touches": [
        "cmd/evie",
        "internal/agent",
        "internal/eviedb",
        "internal/memory",
        "internal/web"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/memory/epic_1/story_3/README.md",
        "#60",
        "#59"
      ],
      "text": [
        "Epic 1 story 3: after a restart the REPL can create or resume an active global or project session from one combined list, with deterministic session titles, immutable scope and durable turn ownership preserved. Follow-up fixes fence stale chooser state and harden its recovery."
      ]
    },
    {
      "date": "2026-08-24",
      "title": "Lease-own and fence every live turn",
      "branch": "codex/57-lease-own-fence-live-turn",
      "merge": "02360c5",
      "touches": [
        "internal/agent",
        "internal/eviedb",
        "internal/memory",
        "internal/openrouter",
        "internal/tools",
        "internal/web",
        "internal/web/ui",
        "cmd/evie"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/memory/epic_1/story_2/README.md",
        "#58",
        "#57"
      ],
      "text": [
        "Epic 1 story 2: a durable session lease owns every live agent turn from its first event through provider and tool work, with SQLite fencing at every accepted write. Review rounds closed terminal-boundary races, made turn boundaries cancellation-safe, gated approvals and isolated provider transport, and preserved the turn stage and committed output."
      ]
    },
    {
      "date": "2026-08-23",
      "title": "Lease-owned turns story readied",
      "branch": "master",
      "commits": {
        "from": "02d5cc8",
        "to": "02d5cc8"
      },
      "touches": [
        "cmd/evie/docs"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/memory/epic_1/story_2/README.md",
        "doc:cmd/evie/docs/active/memory.decisions.md"
      ],
      "text": [
        "Readies Epic 1 story 2, lease-owned turns, before implementation, recording its decisions in the memory decisions and amending the memory, serve and chat UI specs."
      ]
    },
    {
      "date": "2026-08-23",
      "title": "Cancellable tool lifecycle",
      "branch": "codex/55-cancellable-tool-lifecycle",
      "merge": "d48c0ac",
      "touches": [
        "internal/tools",
        "internal/agent",
        "internal/eviedb",
        "internal/finance",
        "internal/youtube",
        "internal/web",
        "cmd/evie",
        "cmd/finance"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/memory/epic_1/story_1/README.md",
        "#56",
        "#55"
      ],
      "text": [
        "Epic 1 story 1: the turn's caller-owned context flows through tool preparation, approval and execution, so a cancellation seen at a lifecycle boundary stops later turn-owned work and aborts the turn. Follow-ups closed cancellation races, kept the cron tool's ordinary cleanup, and filled recovery decision gaps."
      ]
    },
    {
      "date": "2026-08-23",
      "title": "Story delivery skills and the Epic 1 plan",
      "branch": "master",
      "commits": {
        "from": "dc76f99",
        "to": "adbb2d8"
      },
      "touches": [
        ".agents/skills",
        ".codex/agents",
        ".opencode/agents",
        "tools/story-loop",
        "cmd/evie/docs",
        "docs"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/memory/epic_1/README.md"
      ],
      "text": [
        "Reworks the agent skills for story work: a review-story skill with contract and maintainability reviewers, then a deliver-story skill with result schemas, while the `tools/story-loop` controller is removed. The memory plan is rewritten around Epic 1, restart-safe leased sessions, with a README per story."
      ]
    },
    {
      "date": "2026-08-22",
      "title": "Reviewed story repair loop",
      "branch": "codex/devx-1-story-loop",
      "merge": "51a0387",
      "touches": [
        "tools/story-loop",
        ".gitignore"
      ],
      "sources": [
        "#54"
      ],
      "text": [
        "Adds `tools/story-loop`, a Python controller for one approved story. It starts an implementation task in an isolated worktree, reviews the exact commit in a fresh read-only task, runs deterministic checks outside both, and resumes the implementation with the findings until the candidate is ready or a bounded stop condition fires."
      ]
    },
    {
      "date": "2026-08-22",
      "title": "Durable turn-lease storage",
      "branch": "codex/3-mem-1-2-durable-turn-lease-storage",
      "merge": "e57868f",
      "touches": [
        "internal/eviedb",
        "internal/memory"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/memory/epic_1/README.md",
        "#52"
      ],
      "text": [
        "MEM-1.2 adds durable turn-lease storage in SQLite: acquiring, renewing, releasing and expiring a session's turn lease, with fencing whose epoch invariants are pinned by tests."
      ]
    },
    {
      "date": "2026-08-21",
      "title": "Explicit REPL scope selection",
      "branch": "codex/2-mem-1-1-explicit-repl-scope-selection",
      "merge": "3b7d96e",
      "touches": [
        "cmd/evie",
        "internal/eviedb"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/memory/epic_1/README.md",
        "#51"
      ],
      "text": [
        "MEM-1.1: the REPL selects a global or project scope explicitly and confirms before creating a new session in it."
      ]
    },
    {
      "date": "2026-08-21",
      "title": "Durable sessions, AGENTS.md, and the memory epics",
      "branch": "master",
      "commits": {
        "from": "4b1f113",
        "to": "b5e9d0e"
      },
      "touches": [
        "internal/agent",
        "internal/eviedb",
        "internal/memory",
        "internal/tools",
        "internal/openrouter",
        "internal/web",
        "cmd/evie",
        "AGENTS.md",
        ".agents/skills",
        "cmd/evie/docs",
        "docs"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/memory.spec.md#stage-1---session-identity-and-append-only-events",
        "doc:AGENTS.md"
      ],
      "text": [
        "Conversations become durable: Evie's database gains projects, sessions and an append-only event log, and the agent rebuilds its history from the stored events. `query_db` can read the evie database, fenced to the cron tables. `AGENTS.md` arrives, with the workflow for implementing stories, and the memory plan is broken into nine epics with stories, planning and implementation skills, and execution contracts."
      ]
    },
    {
      "date": "2026-08-17",
      "title": "Backlog research and a revised memory spec",
      "branch": "master",
      "commits": {
        "from": "362943a",
        "to": "9a6faa6"
      },
      "touches": [
        "cmd/evie/docs",
        "docs",
        "CLAUDE.md"
      ],
      "sources": [
        "doc:cmd/evie/docs/research/opencode-backlog/README.md",
        "doc:cmd/evie/docs/active/memory.spec.md",
        "doc:cmd/evie/docs/active/memory.decisions.md"
      ],
      "text": [
        "Adds a feature backlog researched from opencode's agents and features, and substantially revises the memory spec and its decisions. `CLAUDE.md` is deleted."
      ]
    },
    {
      "date": "2026-08-13",
      "title": "YouTube transcript library",
      "branch": "master",
      "commits": {
        "from": "7e206b7",
        "to": "7e206b7"
      },
      "touches": [
        "internal/youtube",
        "internal/tools",
        "cmd/ytscribe"
      ],
      "sources": [
        "doc:cmd/evie/docs/done/youtube-transcripts.spec.md",
        "doc:cmd/evie/docs/done/youtube-transcripts.decisions.md"
      ],
      "text": [
        "Gives Evie a durable, searchable YouTube transcript library in `~/.evie/transcripts/transcripts.db`. Two ungated tools fetch and cache one public video's transcript (`youtube_transcript`) and refresh a channel's newest videos (`youtube_scrape_channel`), and the same package backs a `ytscribe` CLI for full channel crawls and for importing an existing scraper corpus. `query_db` can also read the transcripts database."
      ]
    },
    {
      "date": "2026-08-12",
      "title": "System prompt and memory research",
      "branch": "master",
      "commits": {
        "from": "31439fe",
        "to": "31439fe"
      },
      "touches": [
        "internal/agent",
        "cmd/evie/docs"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/memory.spec.md",
        "doc:cmd/evie/docs/active/memory.decisions.md"
      ],
      "text": [
        "Gives Evie a stable system prompt covering identity, task ownership, trust and approval, tool use and communication, and drafts a local, Letta-inspired memory spec with its decisions."
      ]
    },
    {
      "date": "2026-08-09",
      "title": "Evie: one agent loop, evie serve, reasoning, cron, and web tools",
      "branch": "master",
      "commits": {
        "from": "19f5904",
        "to": "07344e3"
      },
      "touches": [
        "internal/agent",
        "internal/web",
        "internal/web/ui",
        "internal/tools",
        "internal/eviedb",
        "internal/openrouter",
        "internal/finance",
        "cmd/evie",
        "cmd/moussa"
      ],
      "sources": [
        "doc:cmd/evie/docs/active/serve.spec.md",
        "doc:cmd/evie/docs/done/agent-extract.spec.md",
        "doc:cmd/evie/docs/done/serve-core.spec.md",
        "doc:cmd/evie/docs/done/reasoning.spec.md",
        "doc:cmd/evie/docs/done/reasoning.decisions.md",
        "doc:cmd/evie/docs/done/cron.spec.md",
        "doc:cmd/evie/docs/done/cron.decisions.md",
        "doc:cmd/evie/docs/done/web-fetch.spec.md",
        "doc:cmd/evie/docs/done/web-fetch.decisions.md",
        "doc:cmd/evie/docs/done/web-search.spec.md",
        "doc:cmd/evie/docs/done/web-search.decisions.md",
        "doc:cmd/evie/docs/active/ui-chat.spec.md"
      ],
      "text": [
        "The harness becomes Evie: `cmd/moussa` becomes `cmd/evie`, in the module `github.com/davidadel66/evie`. The agent loop moves into `internal/agent` behind an events interface, so the terminal REPL and the new `evie serve` web UI are two faces of one loop; `internal/web` streams turns over SSE and round-trips approvals, and a React chat shows streamed reasoning in a collapsible block.",
        "New tools: `cron_add`, `cron_list` and `cron_remove` schedule shell commands that launchd fires, stored in `~/.evie/evie.db`; `web_fetch` returns a page as lightweight Markdown; and `web_search` returns raw Brave Search results. The web chat is then polished, with a text-size control and code themes, and file approvals show full diffs."
      ]
    },
    {
      "date": "2026-08-01",
      "title": "File tools and bash",
      "branch": "master",
      "commits": {
        "from": "252cb4d",
        "to": "b29b53c"
      },
      "touches": [
        "internal/tools",
        "cmd/moussa",
        "docs"
      ],
      "sources": [
        "doc:cmd/evie/docs/done/file-tools.spec.md",
        "doc:cmd/evie/docs/done/file-tools.decisions.md",
        "doc:cmd/evie/docs/done/file-tools.build.md",
        "doc:cmd/evie/docs/done/bash.decisions.md"
      ],
      "text": [
        "`read_file` returns numbered lines capped at 100 KB, fenced by a path denylist inside path resolution, and `edit_file` makes exact-string replacements, approval-gated and written atomically. `bash` runs a real login shell with no approval gate or denylist, since a shell walks around any fence: the working directory persists between calls, interactive aliases come from a snapshot taken at startup, and its timeouts and 30k output cap follow Claude Code's BashTool. `docs/BACKLOG.md` and `docs/decisions.md` start here, through `/wrap`."
      ]
    },
    {
      "date": "2026-07-29",
      "title": "Streamed replies",
      "branch": "master",
      "commits": {
        "from": "5909731",
        "to": "5c83ef1"
      },
      "touches": [
        "internal/openrouter",
        "cmd/moussa",
        "internal/tools"
      ],
      "sources": [
        "pkg:internal/openrouter"
      ],
      "text": [
        "Replies stream from OpenRouter over SSE and print to the terminal at a smooth pace. The database tool descriptions now say recurring budget limits always go in the standing template, with a monthly row only for a one-month exception."
      ]
    },
    {
      "date": "2026-07-28",
      "title": "Budget tables and general database tools",
      "branch": "master",
      "commits": {
        "from": "aa87fb5",
        "to": "c85bd6d"
      },
      "touches": [
        "internal/finance",
        "internal/tools",
        "cmd/moussa",
        "cmd/finance/docs"
      ],
      "sources": [
        "doc:cmd/finance/docs/active/budget.spec.md"
      ],
      "text": [
        "Specifies a monthly budget with per-category limits, kept current by scheduled sync and rule categorization, with uncertain transactions reviewed in chat. The agent first gets `finance_query` for read-only SQL, then general `query_db` and `edit_db` tools that replace it, with writes behind an approval gate. `budget_entries` and `budget_limits` tables follow, and categorize now writes a budget entry per transaction instead of the legacy category column."
      ]
    },
    {
      "date": "2026-07-16",
      "title": "Domain packages and the agent's finance tools",
      "branch": "master",
      "commits": {
        "from": "cdcb0f7",
        "to": "0175449"
      },
      "touches": [
        "internal/finance",
        "internal/todo",
        "internal/tools",
        "internal/openrouter",
        "cmd/agent",
        "cmd/moussa",
        "cmd/finance",
        "cmd/todo"
      ],
      "sources": [
        "pkg:internal/finance",
        "pkg:internal/tools"
      ],
      "text": [
        "The finance and todo domains move out of their commands into `internal/finance`, split into client, link and sync, and `internal/todo`, with every package and function documented and environment loading kept out of domain code. The merchant lookup file moves to `~/.finance`. Agent tools move into `internal/tools` and gain `finance_sync`, `finance_rules` and `finance_categorize`, and `cmd/agent` is renamed `cmd/moussa`."
      ]
    },
    {
      "date": "2026-07-14",
      "title": "First commit",
      "branch": "master",
      "commits": {
        "from": "ef694f5",
        "to": "ef694f5"
      },
      "touches": [
        "cmd/todo",
        "cmd/finance",
        "cmd/agent",
        "internal/openrouter"
      ],
      "sources": [
        "doc:cmd/finance/docs/done/sync.spec.md",
        "doc:cmd/finance/docs/done/sync.decisions.md",
        "doc:cmd/finance/docs/active/categorize.spec.md"
      ],
      "text": [
        "The repository starts as a personal collection of Go command-line tools, built partly to learn Go: `todo`, a task manager with JSON persistence; `finance`, which links banks through Plaid, syncs transactions into SQLite and categorizes them by merchant rules; and `agent`, a terminal chat loop through OpenRouter with time and todo tools."
      ]
    }
  ]
}
);
