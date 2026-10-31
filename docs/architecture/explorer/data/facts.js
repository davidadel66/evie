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
      "pattern": "/api/subagents/list",
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
      "date": "2026-10-29",
      "title": "System explorer",
      "branch": "feat/system-explorer",
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
    }
  ]
}
);
