evieExplorer.register(
{
  "format": 1,
  "kind": "content",
  "owner": "platform",
  "nodes": [
    {
      "id": "pkg:internal/eviedb",
      "level": "component",
      "kind": "package",
      "title": "eviedb",
      "group": "infrastructure",
      "subgroup": "Persistence",
      "alsoIn": [
        "memory"
      ],
      "summary": {
        "text": "Opens Evie's SQLite database at ~/.evie/evie.db and is the Store through which other packages persist their records.",
        "refs": [
          "code:internal/eviedb/db.go#OpenDBContext",
          "code:internal/eviedb/store.go#Store"
        ]
      }
    },
    {
      "id": "pkg:internal/web",
      "level": "component",
      "kind": "package",
      "title": "web",
      "group": "infrastructure",
      "subgroup": "Surfaces",
      "summary": {
        "text": "Serves Evie's web frontend: its open conversations and the approvals waiting for a browser answer.",
        "refs": [
          "code:internal/web/serve.go#Server"
        ]
      }
    },
    {
      "id": "pkg:cmd/evie",
      "level": "component",
      "kind": "package",
      "title": "cmd/evie",
      "group": "infrastructure",
      "subgroup": "Surfaces",
      "summary": {
        "text": "The evie binary: it dispatches on its subcommand, wires up the provider client, and hands the session to a frontend.",
        "refs": [
          "code:cmd/evie/main.go#main"
        ]
      }
    },
    {
      "id": "pkg:cmd/todo",
      "level": "component",
      "kind": "package",
      "title": "cmd/todo",
      "group": "infrastructure",
      "subgroup": "Surfaces",
      "summary": {
        "text": "The owner's command line for Evie's durable SQLite Task Trees.",
        "refs": [
          "code:cmd/todo/main.go#run"
        ]
      },
      "terms": [
        "Task Tree"
      ]
    },
    {
      "id": "pkg:internal/usage",
      "level": "component",
      "kind": "package",
      "title": "usage",
      "group": "infrastructure",
      "subgroup": "Observability",
      "summary": {
        "text": "Provides content-free owner diagnostics that never become conversation evidence, billing authority, or runtime budget state.",
        "refs": [
          "code:internal/usage/usage.go",
          "code:internal/usage/service.go#Service"
        ]
      }
    }
  ],
  "edges": [],
  "flows": []
}
);
