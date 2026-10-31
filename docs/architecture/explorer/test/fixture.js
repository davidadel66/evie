"use strict";

// A small explorer data set shaped like Evie's: context and container nodes,
// packages in every group, content edges, flows, a node carrying one diagram
// of every kind, facts with commands, routes, tables, and History entries
// (one touching a removed package), and two design proposals that add,
// change, and remove nodes, edges, and flows.
//
// Not a test file: the *.test.js glob does not pick it up.

function st(text, ...refs) {
  return { text, refs: refs.length ? refs : ["code:internal/alpha"] };
}

function pkg(path, group, extra) {
  return Object.assign(
    {
      id: "pkg:" + path,
      level: "component",
      kind: "package",
      title: path.replace(/^internal\//, ""),
      group,
      summary: st(path + " does things.", "code:" + path),
    },
    extra || {}
  );
}

function facts() {
  const packages = [
    ["cmd/tool", "main", true],
    ["internal/alpha", "alpha", false],
    ["internal/beta", "beta", false],
    ["internal/gamma", "gamma", false],
    ["internal/delta", "delta", false],
    ["internal/epsilon", "epsilon", false],
    ["internal/zeta", "zeta", false],
    ["internal/eta", "eta", false],
    ["internal/alphatest", "alphatest", false],
  ].map(([path, name, command]) => ({
    path,
    name,
    command,
    doc: name === "alpha" ? "Package alpha serves." : "",
    testSupport: name.endsWith("test"),
  }));
  return {
    format: 1,
    kind: "facts",
    packages,
    imports: [
      { from: "cmd/tool", to: "internal/alpha" },
      { from: "cmd/tool", to: "internal/beta" },
      { from: "internal/alpha", to: "internal/epsilon" },
      { from: "internal/alpha", to: "internal/gamma" },
      { from: "internal/alpha", to: "internal/zeta" },
      { from: "internal/alphatest", to: "internal/alpha" },
      { from: "internal/delta", to: "internal/beta" },
      { from: "internal/delta", to: "internal/zeta" },
      { from: "internal/epsilon", to: "internal/delta" },
      { from: "internal/eta", to: "internal/beta" },
      { from: "internal/zeta", to: "internal/eta" },
    ],
    commands: [
      { binary: "tool", words: ["serve"], package: "cmd/tool", source: { file: "cmd/tool/main.go", function: "main" } },
      { binary: "tool", words: ["migrate", "up"], package: "cmd/tool", source: { file: "cmd/tool/main.go", function: "migrate" } },
    ],
    routes: [
      { method: "GET", pattern: "/api/alpha", package: "internal/alpha", guard: "requireOwner", source: { file: "internal/alpha/http.go", function: "routes" } },
      { pattern: "/healthz", package: "internal/alpha", source: { file: "internal/alpha/http.go", function: "routes" } },
    ],
    databases: [
      {
        id: "fixture",
        package: "internal/beta",
        tables: [
          {
            name: "audit",
            columns: [
              { name: "id", type: "INTEGER", notNull: false, primaryKey: 1 },
              { name: "note", type: "TEXT", notNull: true, primaryKey: 0 },
            ],
            foreignKeys: [],
          },
          {
            name: "owners",
            columns: [
              { name: "id", type: "INTEGER", notNull: false, primaryKey: 1 },
              { name: "name", type: "TEXT", notNull: true, primaryKey: 0 },
            ],
            foreignKeys: [],
          },
          {
            name: "widget_tags",
            columns: [
              { name: "widget_id", type: "INTEGER", notNull: true, primaryKey: 1 },
              { name: "tag", type: "TEXT", notNull: true, primaryKey: 2 },
              { name: "tagged_by", type: "INTEGER", notNull: false, primaryKey: 0 },
            ],
            foreignKeys: [
              { columns: ["widget_id"], table: "widgets", references: ["id"] },
              { columns: ["tagged_by"], table: "owners", references: ["id"] },
            ],
          },
          {
            name: "widgets",
            columns: [
              { name: "id", type: "INTEGER", notNull: false, primaryKey: 1 },
              { name: "owner_id", type: "INTEGER", notNull: true, primaryKey: 0 },
              { name: "parent_id", type: "INTEGER", notNull: false, primaryKey: 0 },
              { name: "kind", type: "TEXT", notNull: true, primaryKey: 0 },
            ],
            foreignKeys: [
              { columns: ["owner_id"], table: "owners", references: ["id"] },
              { columns: ["parent_id"], table: "widgets", references: ["id"] },
            ],
          },
        ],
      },
    ],
    glossary: [
      { term: "Widget", definition: "A thing the fixture makes.", avoid: ["Gadget"] },
      { term: "Ledger Entry", definition: "One recorded line.", avoid: [] },
      { term: "Plugin", definition: "An installable bundle of capabilities.", avoid: ["skill pack"] },
    ],
    adrs: [{ number: "0001", title: "Make the first decision", path: "docs/adr/0001-first-decision.md" }],
    history: [
      {
        date: "2026-10-20",
        title: "Teach the tool to export",
        branch: "feat/export",
        merge: "abc1234",
        touches: ["cmd/tool", "internal/beta"],
        sources: ["doc:docs/export.spec.md", "#12"],
        text: ["The tool writes its widgets as files.", "Owners asked to keep copies."],
      },
      {
        date: "2026-10-20",
        title: "Retire the old store",
        branch: "chore/retire-store",
        touches: ["internal/gone", "internal/beta"],
        sources: ["#11"],
        text: ["The old store is removed."],
      },
      {
        date: "2026-09-30",
        title: "Start the fixture",
        branch: "master",
        commits: { from: "1111111", to: "2222222" },
        touches: ["internal/alpha", "scripts/build"],
        sources: ["code:internal/alpha", "#1"],
        text: ["The first commits."],
      },
    ],
  };
}

function diagrams() {
  return [
    {
      id: "delta-lanes",
      title: "Delta pipeline",
      kind: "graph",
      summary: st("How delta moves widgets."),
      lanes: [
        { id: "ingest", title: "Ingest" },
        { id: "store", title: "Store" },
      ],
      nodes: [
        { id: "read", label: "Read input", lane: "ingest" },
        { id: "parse", label: "Parse <widget> & check", lane: "ingest" },
        { id: "save", label: "Save", lane: "store", node: "pkg:internal/beta" },
        { id: "index", label: "Index", lane: "store" },
      ],
      edges: [
        { from: "read", to: "parse", label: "bytes" },
        { from: "parse", to: "save", label: "widget", refs: ["code:internal/delta"] },
        { from: "save", to: "index" },
        { from: "index", to: "read", label: "retry" },
      ],
    },
    {
      id: "delta-plain",
      title: "Delta parts",
      kind: "graph",
      summary: st("Delta's parts."),
      nodes: [
        { id: "x", label: "Front" },
        { id: "y", label: "Middle" },
        { id: "z", label: "Back" },
        { id: "w", label: "Side" },
      ],
      edges: [
        { from: "x", to: "y" },
        { from: "y", to: "z" },
        { from: "x", to: "z", label: "shortcut" },
        { from: "x", to: "w" },
      ],
    },
    {
      id: "delta-sequence",
      title: "Delta saves",
      kind: "sequence",
      summary: st("Delta saves a widget."),
      participants: ["pkg:internal/delta", "pkg:internal/beta"],
      steps: [
        { from: "pkg:internal/delta", to: "pkg:internal/beta", label: "Save", kind: "call", via: "import", refs: ["code:internal/delta"] },
        { from: "pkg:internal/beta", to: "pkg:internal/delta", label: "Saved", kind: "return", refs: ["code:internal/beta"] },
      ],
    },
    {
      id: "delta-states",
      title: "Job lifecycle",
      kind: "state",
      summary: st("A job's states."),
      initial: "queued",
      states: [
        { id: "queued", label: "Queued", refs: ["code:internal/delta"] },
        { id: "running", label: "Running", refs: ["code:internal/delta"] },
        { id: "done", refs: ["code:internal/delta"] },
        { id: "failed", label: "Failed", refs: ["code:internal/delta"] },
      ],
      transitions: [
        { from: "queued", to: "running", label: "claim", refs: ["code:internal/delta"] },
        { from: "running", to: "done", label: "finish", refs: ["code:internal/delta"] },
        { from: "running", to: "failed", label: "error", refs: ["code:internal/delta"] },
        { from: "failed", to: "queued", label: "retry", refs: ["code:internal/delta"] },
        { from: "running", to: "running", label: "heartbeat", refs: ["code:internal/delta"] },
      ],
    },
    {
      id: "delta-er",
      title: "Widget tables",
      kind: "er",
      summary: st("Where widgets live.", "table:fixture.widgets"),
      database: "fixture",
      tables: ["widgets", "widget_tags", "audit"],
    },
  ];
}

function content() {
  return [
    {
      format: 1,
      kind: "content",
      owner: "system",
      nodes: [
        { id: "person:owner", level: "context", kind: "person", title: "Owner", summary: st("The owner.") },
        { id: "system:main", level: "context", kind: "system", title: "Main system", summary: st("The system.") },
        { id: "external:api", level: "context", kind: "external-system", title: "Model API", summary: st("A remote API.") },
        { id: "external:bank", level: "context", kind: "external-system", title: "Bank", summary: st("A bank.") },
        {
          id: "container:server",
          level: "container",
          kind: "process",
          title: "server",
          summary: st("The server process."),
          contains: ["pkg:internal/alpha", "pkg:internal/epsilon", "pkg:internal/delta"],
        },
        { id: "container:cli", level: "container", kind: "cli", title: "tool", summary: st("The CLI."), contains: ["pkg:cmd/tool"] },
        { id: "container:ui", level: "container", kind: "user-interface", title: "Web UI", summary: st("The UI.") },
        { id: "container:db", level: "container", kind: "store", title: "Database", summary: st("The database.") },
        { id: "container:files", level: "container", kind: "store", title: "Files", summary: st("Files on disk.") },
      ],
      edges: [
        { from: "person:owner", to: "system:main", label: "uses", via: "call", refs: ["code:internal/alpha"] },
        { from: "system:main", to: "external:api", label: "asks", via: "network", refs: ["code:internal/gamma"] },
        { from: "system:main", to: "external:bank", label: "syncs", via: "network", refs: ["code:internal/beta"] },
        { from: "person:owner", to: "container:ui", label: "browses", via: "http", refs: ["code:internal/alpha"] },
        { from: "person:owner", to: "container:cli", label: "types", via: "process", refs: ["code:cmd/tool"] },
        { from: "container:ui", to: "container:server", label: "calls", via: "http", refs: ["code:internal/alpha"] },
        { from: "container:server", to: "container:db", label: "reads and writes", via: "sql", refs: ["code:internal/beta"] },
        { from: "container:server", to: "external:api", label: "asks", via: "network", refs: ["code:internal/gamma"] },
        { from: "container:cli", to: "container:db", label: "migrates", via: "sql", refs: ["code:cmd/tool"] },
        { from: "container:cli", to: "container:files", label: "exports", via: "file", refs: ["code:cmd/tool"] },
        { from: "pkg:internal/alpha", to: "pkg:internal/epsilon", label: "starts work", via: "import" },
        { from: "pkg:internal/delta", to: "external:bank", label: "syncs", via: "network", refs: ["code:internal/delta"] },
      ],
      flows: [
        {
          id: "chat",
          title: "Chat turn",
          summary: st("One turn of chat."),
          trigger: st("The owner sends a message."),
          participants: ["person:owner", "pkg:internal/alpha", "pkg:internal/epsilon", "pkg:internal/delta"],
          steps: [
            { from: "person:owner", to: "pkg:internal/alpha", label: "Send a message", kind: "call", via: "http", refs: ["route:GET /api/alpha"] },
            { from: "pkg:internal/alpha", to: "pkg:internal/epsilon", label: "Start the turn with the full transcript and every tool", kind: "call", via: "import", refs: ["code:internal/alpha"] },
            { from: "pkg:internal/epsilon", to: "pkg:internal/epsilon", label: "Plan", kind: "call", refs: ["code:internal/epsilon"] },
            { from: "pkg:internal/epsilon", to: "pkg:internal/delta", label: "Queue work", kind: "async", refs: ["code:internal/delta"] },
            { from: "pkg:internal/alpha", to: "pkg:internal/delta", label: "Work runs in the background", kind: "note", refs: ["code:internal/delta"] },
            { from: "pkg:internal/delta", to: "pkg:internal/epsilon", label: "Done", kind: "return", via: "import", refs: ["code:internal/delta"] },
            { from: "pkg:internal/alpha", to: "person:owner", label: "Reply", kind: "return", refs: ["code:internal/alpha"] },
          ],
        },
        {
          id: "legacy",
          title: "Legacy export",
          summary: st("An old export."),
          trigger: st("A nightly timer."),
          participants: ["pkg:cmd/tool", "pkg:internal/beta"],
          steps: [{ from: "pkg:cmd/tool", to: "pkg:internal/beta", label: "Export", kind: "call", via: "import", refs: ["code:cmd/tool"] }],
        },
      ],
    },
    {
      format: 1,
      kind: "content",
      owner: "core",
      nodes: [
        pkg("internal/alpha", "runtime", {
          terms: ["Widget"],
          does: [st("Serves widgets.", "code:internal/alpha", "term:Widget")],
          gotchas: [st("Mind the ledger.", "term:Ledger Entry")],
          links: ["adr:0001"],
        }),
        pkg("internal/epsilon", "runtime"),
        pkg("internal/delta", "runtime", { diagrams: diagrams() }),
        pkg("internal/zeta", "runtime"),
        pkg("internal/beta", "infrastructure", { subgroup: "Persistence", alsoIn: ["memory"] }),
        pkg("cmd/tool", "infrastructure", { subgroup: "Surfaces", title: "cmd/tool" }),
        pkg("internal/alphatest", "infrastructure", { subgroup: "Surfaces" }),
        pkg("internal/gamma", "intelligence"),
        pkg("internal/eta", "memory"),
      ],
      edges: [],
      flows: [],
    },
  ];
}

function proposals() {
  return [
    {
      format: 1,
      kind: "proposal",
      id: "sample",
      feature: "theta",
      title: "Add theta",
      status: "example",
      spec: "doc:docs/theta.spec.md",
      summary: st("Theta replaces zeta.", "doc:docs/theta.spec.md"),
      nodes: [
        { op: "add", id: "pkg:internal/theta", level: "component", kind: "package", title: "theta", group: "runtime", summary: st("Theta runs.", "doc:docs/theta.spec.md") },
        { op: "change", id: "pkg:internal/alpha", summary: st("Alpha starts theta.", "doc:docs/theta.spec.md"), terms: ["Plugin"] },
        { op: "remove", id: "pkg:internal/zeta" },
        { op: "change", id: "container:server", summary: st("The server also runs theta.", "doc:docs/theta.spec.md") },
      ],
      edges: [
        { op: "add", from: "pkg:internal/alpha", to: "pkg:internal/theta", via: "import", label: "starts" },
        { op: "add", from: "pkg:internal/theta", to: "pkg:internal/delta", via: "import" },
        { op: "change", from: "pkg:internal/alpha", to: "pkg:internal/epsilon", via: "import", label: "hands over" },
        { op: "remove", from: "pkg:internal/delta", to: "pkg:internal/zeta", via: "import" },
        { op: "change", from: "container:server", to: "external:api", via: "network", label: "streams" },
      ],
      flows: [
        {
          op: "add",
          id: "theta-run",
          title: "Theta run",
          summary: st("Theta runs once.", "doc:docs/theta.spec.md"),
          trigger: st("Alpha asks.", "doc:docs/theta.spec.md"),
          participants: ["pkg:internal/alpha", "pkg:internal/theta"],
          steps: [{ from: "pkg:internal/alpha", to: "pkg:internal/theta", label: "Run", kind: "call", refs: ["node:pkg:internal/theta"] }],
        },
        { op: "change", id: "chat", title: "Chat turn with theta" },
        { op: "remove", id: "legacy" },
      ],
      requirements: [st("Theta must run nightly.", "doc:docs/theta.spec.md#user-stories")],
      decisions: [st("Theta replaces zeta.", "doc:docs/theta.decisions.md", "adr:0001")],
    },
    {
      format: 1,
      kind: "proposal",
      id: "second",
      feature: "iota",
      title: "Add iota",
      status: "proposed",
      summary: st("Iota remembers.", "doc:docs/iota.spec.md"),
      nodes: [{ op: "add", id: "pkg:internal/iota", level: "component", kind: "package", title: "iota", group: "memory", summary: st("Iota.", "doc:docs/iota.spec.md") }],
      edges: [{ op: "add", from: "pkg:internal/iota", to: "pkg:internal/eta", via: "import" }],
      flows: [],
      requirements: [],
      decisions: [],
    },
  ];
}

function fixtureObjects() {
  return [facts()].concat(content(), proposals());
}

module.exports = { fixtureObjects, diagrams };
