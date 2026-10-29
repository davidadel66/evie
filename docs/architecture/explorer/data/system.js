evieExplorer.register(
{
  "format": 1,
  "kind": "content",
  "owner": "system",
  "nodes": [
    {
      "id": "person:owner",
      "level": "context",
      "kind": "person",
      "title": "Owner",
      "summary": {
        "text": "The single owner Evie works for, who drives it from the web UI and the terminal.",
        "refs": [
          "doc:GLOSSARY.md#evie",
          "code:cmd/evie/main.go#main"
        ]
      }
    },
    {
      "id": "system:evie",
      "level": "context",
      "kind": "system",
      "title": "Evie",
      "summary": {
        "text": "A Go-native personal agent runtime with local SQLite persistence, purpose-built tools, an OpenRouter conversational client, and a web interface.",
        "refs": [
          "doc:AGENTS.md#project"
        ]
      }
    },
    {
      "id": "external:openrouter",
      "level": "context",
      "kind": "external-system",
      "title": "OpenRouter",
      "summary": {
        "text": "The remote model API Evie calls through its Chat Completions and Responses endpoints.",
        "refs": [
          "pkg:internal/openrouter",
          "code:internal/openrouter/client.go#NewClient"
        ]
      }
    },
    {
      "id": "external:ollama",
      "level": "context",
      "kind": "external-system",
      "title": "Ollama",
      "summary": {
        "text": "A local Ollama server, used for local embeddings and by the explicitly configured local compiler transport.",
        "refs": [
          "code:internal/localembedding/client.go#Client",
          "code:internal/localextractor/ollama.go"
        ]
      }
    },
    {
      "id": "external:mcp-servers",
      "level": "context",
      "kind": "external-system",
      "title": "Hosted MCP servers",
      "summary": {
        "text": "Hosted MCP servers the owner connects through an OAuth sign-in, whose reviewed tools a chat may reach.",
        "refs": [
          "pkg:internal/mcpconn",
          "pkg:internal/mcpchat"
        ]
      }
    },
    {
      "id": "external:github",
      "level": "context",
      "kind": "external-system",
      "title": "GitHub",
      "summary": {
        "text": "GitHub's REST API, which the Changes feature talks to through a small client.",
        "refs": [
          "code:internal/github/client.go#Client"
        ]
      }
    },
    {
      "id": "external:plaid",
      "level": "context",
      "kind": "external-system",
      "title": "Plaid",
      "summary": {
        "text": "The service through which the finance domain links banks and syncs transactions.",
        "refs": [
          "pkg:internal/finance",
          "code:internal/finance/client.go"
        ]
      }
    },
    {
      "id": "external:youtube",
      "level": "context",
      "kind": "external-system",
      "title": "YouTube",
      "summary": {
        "text": "The video site the YouTube transcript library reads from.",
        "refs": [
          "code:internal/youtube/client.go#Client"
        ]
      }
    },
    {
      "id": "container:serve",
      "level": "container",
      "kind": "process",
      "title": "evie serve",
      "summary": {
        "text": "The `evie serve` process: the web server, the runtime behind it, and the background hosts it starts.",
        "refs": [
          "code:cmd/evie/main.go#main",
          "code:internal/web/lifecycle.go#ServeWithContext"
        ]
      }
    },
    {
      "id": "container:repl",
      "level": "container",
      "kind": "cli",
      "title": "evie REPL",
      "summary": {
        "text": "The interactive terminal REPL that `evie` with no command starts.",
        "refs": [
          "code:cmd/evie/main.go#main",
          "code:cmd/evie/repl.go#runREPLContextIOWithMemory"
        ]
      }
    },
    {
      "id": "container:management-cli",
      "level": "container",
      "kind": "cli",
      "title": "evie management commands",
      "summary": {
        "text": "One-shot `evie` subcommands that inspect or manage Evie's state and then exit.",
        "refs": [
          "code:cmd/evie/main.go#main"
        ]
      }
    },
    {
      "id": "container:web-ui",
      "level": "container",
      "kind": "user-interface",
      "title": "Web UI",
      "summary": {
        "text": "The React single-page app the owner uses in a browser.",
        "refs": [
          "code:internal/web/ui/src/App.tsx",
          "code:internal/web/ui/package.json"
        ]
      }
    },
    {
      "id": "container:evie-db",
      "level": "container",
      "kind": "store",
      "title": "Evie database",
      "summary": {
        "text": "Evie's main SQLite database.",
        "refs": [
          "code:internal/eviedb/db.go#OpenDBAt"
        ]
      }
    },
    {
      "id": "container:finance-db",
      "level": "container",
      "kind": "store",
      "title": "Finance database",
      "summary": {
        "text": "The finance domain's SQLite database at ~/.finance/finance.db.",
        "refs": [
          "code:internal/finance/db.go#OpenDB"
        ]
      }
    },
    {
      "id": "container:youtube-db",
      "level": "container",
      "kind": "store",
      "title": "YouTube library database",
      "summary": {
        "text": "The YouTube transcript library's SQLite database.",
        "refs": [
          "code:internal/youtube/db.go#OpenDB"
        ]
      }
    },
    {
      "id": "container:procedural-repo",
      "level": "container",
      "kind": "store",
      "title": "Procedural Git repository",
      "summary": {
        "text": "Evie's procedural Git repository, the reviewed, versioned home of procedural assets such as Workflow Definitions.",
        "refs": [
          "code:internal/procedural/repo.go#Repo"
        ]
      }
    },
    {
      "id": "container:keychain",
      "level": "container",
      "kind": "store",
      "title": "Keychain",
      "summary": {
        "text": "The Keychain entries that hold MCP connection credentials.",
        "refs": [
          "code:internal/mcpconn/keychain.go#Keychain"
        ]
      }
    },
    {
      "id": "container:todo-cli",
      "level": "container",
      "kind": "cli",
      "title": "todo",
      "summary": {
        "text": "The `todo` command, the owner's CLI for durable Task Trees.",
        "refs": [
          "code:cmd/todo/main.go#main"
        ]
      }
    },
    {
      "id": "container:finance-cli",
      "level": "container",
      "kind": "cli",
      "title": "finance",
      "summary": {
        "text": "The `finance` command, the CLI over the finance domain.",
        "refs": [
          "code:cmd/finance/main.go#main"
        ]
      }
    },
    {
      "id": "container:ytscribe-cli",
      "level": "container",
      "kind": "cli",
      "title": "ytscribe",
      "summary": {
        "text": "The `ytscribe` command, the CLI for the YouTube transcript library.",
        "refs": [
          "code:cmd/ytscribe/main.go#main"
        ]
      }
    }
  ],
  "edges": [],
  "flows": []
}
);
