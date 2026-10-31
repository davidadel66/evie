evieExplorer.register(
{
  "format": 1,
  "kind": "content",
  "owner": "system",
  "nodes": [
    {
      "id": "owner",
      "level": "context",
      "kind": "person",
      "title": "Owner",
      "summary": {"text": "The owner runs the tool.", "refs": ["doc:docs/guide.md#getting-started"]}
    },
    {
      "id": "fixture",
      "level": "context",
      "kind": "system",
      "title": "Fixture",
      "summary": {"text": "The fixture system.", "refs": ["doc:docs/guide.md"]},
      "contains": ["tool-cli"]
    },
    {
      "id": "tool-cli",
      "level": "container",
      "kind": "cli",
      "title": "tool",
      "summary": {"text": "The tool command line.", "refs": ["code:cmd/tool/main.go#main"]},
      "contains": ["pkg:cmd/tool"]
    }
  ],
  "edges": [
    {"from": "owner", "to": "tool-cli", "label": "runs", "via": "process", "refs": ["doc:docs/guide.md#getting-started-1"]}
  ],
  "flows": [
    {
      "id": "run",
      "title": "Run the tool",
      "summary": {"text": "The tool asks alpha for a name.", "refs": ["code:cmd/tool/main.go#main"]},
      "trigger": {"text": "The owner runs the tool.", "refs": ["node:owner"]},
      "participants": ["pkg:cmd/tool", "pkg:internal/alpha"],
      "steps": [
        {"from": "pkg:cmd/tool", "to": "pkg:internal/alpha", "label": "Name()", "kind": "call", "via": "import", "refs": ["code:internal/alpha/alpha.go#Name"]},
        {"from": "pkg:internal/alpha", "to": "pkg:cmd/tool", "label": "name", "kind": "return", "via": "import", "refs": ["code:internal/alpha/alpha.go#Name"]},
        {"from": "pkg:cmd/tool", "to": "pkg:cmd/tool", "label": "prints it", "kind": "note", "refs": ["code:cmd/tool/main.go"]}
      ]
    }
  ]
}
);
