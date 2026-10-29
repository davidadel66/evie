evieExplorer.register(
{
  "format": 1,
  "kind": "content",
  "owner": "integrations",
  "nodes": [
    {
      "id": "pkg:cmd/finance",
      "level": "component",
      "kind": "package",
      "title": "cmd/finance",
      "group": "infrastructure",
      "subgroup": "Surfaces",
      "summary": {
        "text": "The command line over internal/finance: link banks through Plaid, sync transactions, seed categorization rules, and sanity-check the database.",
        "refs": [
          "code:cmd/finance/main.go#main"
        ]
      }
    },
    {
      "id": "pkg:cmd/ytscribe",
      "level": "component",
      "kind": "package",
      "title": "cmd/ytscribe",
      "group": "infrastructure",
      "subgroup": "Surfaces",
      "summary": {
        "text": "The command line for Evie's YouTube transcript library.",
        "refs": [
          "code:cmd/ytscribe/main.go#runYTScribe"
        ]
      }
    },
    {
      "id": "pkg:internal/mcpconn",
      "level": "component",
      "kind": "package",
      "title": "mcpconn",
      "group": "infrastructure",
      "subgroup": "Integrations",
      "summary": {
        "text": "Owns MCP Connections: the record of each hosted MCP server, the OAuth sign-in that connects it, the Keychain that holds its credentials, and the transport that carries its tokens.",
        "refs": [
          "code:internal/mcpconn/connection.go#Connection"
        ]
      }
    },
    {
      "id": "pkg:internal/mcpconn/mcptest",
      "level": "component",
      "kind": "package",
      "title": "mcpconn/mcptest",
      "group": "infrastructure",
      "subgroup": "Integrations",
      "summary": {
        "text": "Provides a fake hosted MCP server and OAuth authorization server for tests.",
        "refs": [
          "code:internal/mcpconn/mcptest/authserver.go#AuthServer"
        ]
      }
    },
    {
      "id": "pkg:internal/mcpchat",
      "level": "component",
      "kind": "package",
      "title": "mcpchat",
      "group": "infrastructure",
      "subgroup": "Integrations",
      "summary": {
        "text": "Decides which reviewed MCP server tools a Workspace chat may reach, and applies the owner's tool review to Workspace Access.",
        "refs": [
          "code:internal/mcpchat/service.go#Service"
        ]
      },
      "terms": [
        "Workspace",
        "Workspace Access"
      ]
    },
    {
      "id": "pkg:internal/github",
      "level": "component",
      "kind": "package",
      "title": "github",
      "group": "infrastructure",
      "subgroup": "Integrations",
      "summary": {
        "text": "The small GitHub REST client the Changes feature reads and writes through.",
        "refs": [
          "code:internal/github/client.go#Client",
          "code:internal/github/pulls.go#Client.CreatePullRequest"
        ]
      }
    },
    {
      "id": "pkg:internal/finance",
      "level": "component",
      "kind": "package",
      "title": "finance",
      "group": "infrastructure",
      "subgroup": "Integrations",
      "summary": {
        "text": "Owns the personal-finance domain: linking banks through Plaid, syncing transactions into ~/.finance/finance.db, and rule-based categorization.",
        "refs": [
          "code:internal/finance/db.go#OpenDB",
          "code:internal/finance/categorize.go#Categorize"
        ]
      }
    },
    {
      "id": "pkg:internal/youtube",
      "level": "component",
      "kind": "package",
      "title": "youtube",
      "group": "infrastructure",
      "subgroup": "Integrations",
      "summary": {
        "text": "Owns Evie's durable YouTube transcript library.",
        "refs": [
          "code:internal/youtube/db.go#OpenDB",
          "code:internal/youtube/service.go"
        ]
      }
    },
    {
      "id": "pkg:internal/changes",
      "level": "component",
      "kind": "package",
      "title": "changes",
      "group": "infrastructure",
      "subgroup": "Local host",
      "summary": {
        "text": "Models a Workspace's Repository and the Changes made in it.",
        "refs": [
          "code:internal/changes/repository.go#Repository"
        ]
      },
      "terms": [
        "Repository",
        "Change",
        "Workspace"
      ]
    },
    {
      "id": "pkg:internal/worktree",
      "level": "component",
      "kind": "package",
      "title": "worktree",
      "group": "infrastructure",
      "subgroup": "Local host",
      "summary": {
        "text": "Owns the lifecycle of a build worker's git worktree: plan it, create it on a fresh branch, commit the worker's changes, compare the branch, and clean it up.",
        "refs": [
          "code:internal/worktree/worktree.go#Plan"
        ]
      }
    },
    {
      "id": "pkg:internal/localfiles",
      "level": "component",
      "kind": "package",
      "title": "localfiles",
      "group": "infrastructure",
      "subgroup": "Local host",
      "summary": {
        "text": "Reads current disk contents beneath one attached folder.",
        "refs": [
          "code:internal/localfiles/files.go#Read"
        ]
      }
    },
    {
      "id": "pkg:internal/localterminal",
      "level": "component",
      "kind": "package",
      "title": "localterminal",
      "group": "infrastructure",
      "subgroup": "Local host",
      "summary": {
        "text": "Owns the owner's interactive shell and its PTY lifetime.",
        "refs": [
          "code:internal/localterminal/session.go#Session"
        ]
      }
    },
    {
      "id": "pkg:internal/workspacetabs",
      "level": "component",
      "kind": "package",
      "title": "workspacetabs",
      "group": "infrastructure",
      "subgroup": "Local host",
      "summary": {
        "text": "Reads a Workspace's own tabs from its folder.",
        "refs": [
          "code:internal/workspacetabs/tabs.go#Tab"
        ]
      },
      "terms": [
        "Workspace"
      ]
    }
  ],
  "edges": [],
  "flows": []
}
);
