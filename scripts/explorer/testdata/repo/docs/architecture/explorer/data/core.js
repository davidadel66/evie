evieExplorer.register(
{
  "format": 1,
  "kind": "content",
  "owner": "core",
  "nodes": [
    {
      "id": "pkg:cmd/tool",
      "level": "component",
      "kind": "package",
      "title": "cmd/tool",
      "group": "infrastructure",
      "subgroup": "Surfaces",
      "summary": {"text": "Command tool prints the fixture name.", "refs": ["code:cmd/tool/main.go#main"]}
    },
    {
      "id": "pkg:internal/alpha",
      "level": "component",
      "kind": "package",
      "title": "alpha",
      "group": "runtime",
      "summary": {"text": "Alpha serves the fixture.", "refs": ["code:internal/alpha/alpha.go#Server", "pkg:internal/alpha"], "detail": "Reserved for the depth pass."},
      "does": [{"text": "Starts a server.", "refs": ["code:internal/alpha/alpha.go#Server.Start"]}],
      "owns": [{"text": "The listen address.", "refs": ["code:internal/alpha/alpha.go#Server.Addr"]}],
      "interfaces": [{"text": "Handlers serve requests.", "refs": ["code:internal/alpha/alpha.go#Handler.Serve"]}],
      "gotchas": [{"text": "The limit is fixed.", "refs": ["code:internal/alpha/alpha.go#Limit", "adr:0002", "term:Widget"]}],
      "terms": ["Widget", "Ledger Entry"],
      "links": ["code:internal/alpha", "doc:docs/guide.md#using-alpha-with-links", "adr:0001"],
      "diagrams": [
        {
          "id": "alpha-graph",
          "title": "Alpha and friends",
          "kind": "graph",
          "summary": {"text": "Alpha calls beta.", "refs": ["code:internal/alpha/alpha.go#Name"]},
          "lanes": [{"id": "core", "title": "Core"}],
          "nodes": [
            {"id": "a", "label": "alpha", "node": "pkg:internal/alpha", "lane": "core"},
            {"id": "b", "label": "beta", "node": "pkg:internal/beta"}
          ],
          "edges": [{"from": "a", "to": "b", "label": "reads Name", "refs": ["code:internal/beta/beta.go#Name"]}]
        },
        {
          "id": "alpha-sequence",
          "title": "Asking alpha",
          "kind": "sequence",
          "summary": {"text": "The tool asks alpha.", "refs": ["code:cmd/tool/main.go#main"]},
          "participants": ["pkg:cmd/tool", "pkg:internal/alpha"],
          "steps": [
            {"from": "pkg:cmd/tool", "to": "pkg:internal/alpha", "label": "Name()", "kind": "call", "refs": ["code:internal/alpha/alpha.go#Name"]}
          ]
        },
        {
          "id": "alpha-states",
          "title": "Server states",
          "kind": "state",
          "summary": {"text": "A server starts once.", "refs": ["code:internal/alpha/alpha.go#Server.Start"]},
          "states": [
            {"id": "idle", "label": "Idle", "refs": ["code:internal/alpha/alpha.go#Server"]},
            {"id": "running", "label": "Running", "refs": ["code:internal/alpha/alpha.go#Server.Start"]}
          ],
          "initial": "idle",
          "transitions": [
            {"from": "idle", "to": "running", "label": "Start", "refs": ["code:internal/alpha/alpha.go#Server.Start"]}
          ]
        }
      ],
      "children": [
        {
          "id": "part:alpha-server",
          "level": "part",
          "title": "Server",
          "summary": {"text": "The server type.", "refs": ["code:internal/alpha/alpha.go#Server"]}
        }
      ]
    },
    {
      "id": "pkg:internal/alpha/alphatest",
      "level": "component",
      "kind": "package",
      "title": "alpha/alphatest",
      "group": "runtime",
      "summary": {"text": "Alphatest fakes alpha.", "refs": ["code:internal/alpha/alphatest/fake.go#Fake"]}
    },
    {
      "id": "pkg:internal/beta",
      "level": "component",
      "kind": "package",
      "title": "beta",
      "group": "memory",
      "alsoIn": ["runtime"],
      "summary": {"text": "Beta names the fixture.", "refs": ["code:internal/beta/beta.go#Name"]}
    },
    {
      "id": "pkg:internal/delta",
      "level": "component",
      "kind": "package",
      "title": "delta",
      "group": "intelligence",
      "summary": {"text": "Delta holds a value.", "refs": ["code:internal/delta/delta.go#Value"]}
    },
    {
      "id": "pkg:internal/gamma",
      "level": "component",
      "kind": "package",
      "title": "gamma",
      "group": "intelligence",
      "summary": {"text": "Gamma holds a value.", "refs": ["code:internal/gamma/gamma.go#Value"]}
    }
  ],
  "edges": [
    {"from": "pkg:internal/alpha", "to": "pkg:internal/beta", "label": "names", "via": "import", "refs": []},
    {"from": "pkg:internal/alpha", "to": "pkg:internal/gamma", "label": "reads a value on windows", "via": "call", "refs": ["code:internal/alpha/alpha_windows.go"]}
  ],
  "flows": []
}
);
