evieExplorer.register(
{
  "format": 1,
  "kind": "proposal",
  "id": "sample",
  "feature": "epsilon",
  "title": "Add epsilon",
  "status": "proposed",
  "spec": "doc:docs/guide.md",
  "summary": {"text": "Epsilon joins alpha.", "refs": ["doc:docs/guide.md#getting-started"]},
  "nodes": [
    {
      "op": "add",
      "id": "pkg:internal/epsilon",
      "level": "component",
      "kind": "package",
      "title": "epsilon",
      "group": "runtime",
      "summary": {"text": "Epsilon will help alpha.", "refs": ["doc:docs/guide.md", "node:pkg:internal/epsilon"]}
    },
    {"op": "change", "id": "pkg:internal/alpha", "summary": {"text": "Alpha will use epsilon.", "refs": ["doc:docs/guide.md"]}},
    {"op": "remove", "id": "pkg:internal/delta"}
  ],
  "edges": [
    {"op": "add", "from": "pkg:internal/alpha", "to": "pkg:internal/epsilon", "label": "uses", "via": "import"},
    {"op": "change", "from": "pkg:internal/alpha", "to": "pkg:internal/gamma", "via": "call", "label": "reads a value everywhere"},
    {"op": "remove", "from": "pkg:cmd/tool", "to": "pkg:internal/alpha", "via": "import"}
  ],
  "flows": [
    {"op": "change", "id": "run", "title": "Run the tool with epsilon"}
  ],
  "requirements": [{"text": "Alpha must reach epsilon.", "refs": ["doc:docs/guide.md"]}],
  "decisions": [{"text": "Epsilon is a new package.", "refs": ["adr:0001"]}]
}
);
