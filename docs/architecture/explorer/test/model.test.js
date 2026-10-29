"use strict";

// Viewer model tests: they load the viewer under Node and call the pure
// functions it exports. Run with: node --test 'docs/architecture/explorer/test/*.test.js'

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const model = require("../explorer.js");

function pkgNode(pkgPath, group, extra) {
  return Object.assign(
    {
      id: "pkg:" + pkgPath,
      level: "component",
      kind: "package",
      title: pkgPath.replace(/^internal\//, ""),
      group: group,
      summary: { text: pkgPath + " does things.", refs: ["code:" + pkgPath] },
    },
    extra || {}
  );
}

function sampleObjects() {
  return [
    {
      format: 1,
      kind: "facts",
      packages: [
        { path: "cmd/tool", name: "main", command: true, doc: "Command tool runs.", testSupport: false },
        { path: "internal/alpha", name: "alpha", command: false, doc: "Package alpha serves.", testSupport: false },
        { path: "internal/beta", name: "beta", command: false, doc: "", testSupport: false },
        { path: "internal/gamma", name: "gamma", command: false, doc: "", testSupport: false },
      ],
      imports: [
        { from: "cmd/tool", to: "internal/alpha" },
        { from: "cmd/tool", to: "internal/beta" },
        { from: "internal/alpha", to: "internal/beta" },
        { from: "internal/alpha", to: "internal/gamma" },
      ],
      commands: [
        { binary: "tool", words: ["serve"], package: "cmd/tool", source: { file: "cmd/tool/main.go", function: "main" } },
      ],
      routes: [
        { method: "GET", pattern: "/api/alpha", package: "internal/alpha", guard: "requireOwner", source: { file: "internal/alpha/http.go", function: "routes" } },
      ],
      databases: [
        {
          id: "fixture",
          package: "internal/beta",
          tables: [
            {
              name: "widgets",
              columns: [
                { name: "id", type: "INTEGER", notNull: true, primaryKey: 1 },
                { name: "owner_id", type: "INTEGER", notNull: false, primaryKey: 0 },
              ],
              foreignKeys: [{ columns: ["owner_id"], table: "owners", references: ["id"] }],
            },
          ],
        },
      ],
      glossary: [
        { term: "Widget", definition: "A thing the fixture makes.", avoid: ["Gadget"] },
        { term: "Ledger Entry", definition: "One recorded line.", avoid: [] },
      ],
      adrs: [{ number: "0001", title: "Make the first decision", path: "docs/adr/0001-first-decision.md" }],
    },
    {
      format: 1,
      kind: "content",
      owner: "core",
      nodes: [
        pkgNode("internal/alpha", "runtime", {
          terms: ["Widget"],
          gotchas: [{ text: "Mind the ledger.", refs: ["term:Ledger Entry"] }],
          children: [{ id: "part:alpha-server", level: "part", title: "Server", summary: { text: "Serves.", refs: ["code:internal/alpha"] } }],
        }),
        pkgNode("internal/beta", "infrastructure", { subgroup: "Persistence", alsoIn: ["memory"] }),
      ],
    },
    {
      format: 1,
      kind: "content",
      owner: "edge",
      nodes: [
        pkgNode("internal/gamma", "intelligence"),
        pkgNode("cmd/tool", "infrastructure", { subgroup: "Surfaces", title: "cmd/tool" }),
      ],
      edges: [],
    },
    { format: 1, kind: "proposal", id: "sample", feature: "f", title: "Sample", status: "example", summary: { text: "s", refs: ["adr:0001"] } },
  ];
}

test("mergeData indexes nodes from every content file and normalises missing lists", () => {
  const data = model.mergeData(sampleObjects());

  assert.deepEqual(
    data.nodes.map((n) => n.id),
    ["pkg:internal/alpha", "pkg:internal/beta", "pkg:internal/gamma", "pkg:cmd/tool"]
  );
  assert.equal(data.nodeById["part:alpha-server"].title, "Server");
  assert.equal(data.nodeById["pkg:internal/gamma"].owner, "edge");
  assert.deepEqual(data.nodeById["pkg:internal/gamma"].does, []);
  assert.deepEqual(data.nodeById["pkg:internal/gamma"].children, []);
  assert.deepEqual(data.edges, []);
  assert.deepEqual(data.flows, []);
  assert.deepEqual(data.proposals.map((p) => p.id), ["sample"]);
  assert.equal(data.facts.packages.length, 4);
});

test("mergeData tolerates absent facts", () => {
  const data = model.mergeData([{ format: 1, kind: "content", owner: "x", nodes: [pkgNode("internal/x", "memory")] }]);
  assert.deepEqual(data.facts.imports, []);
  assert.deepEqual(data.facts.glossary, []);
  assert.equal(data.nodes.length, 1);
});

test("groupOverview places every package node under its group and subgroup", () => {
  const view = model.groupOverview(model.mergeData(sampleObjects()));

  assert.deepEqual(
    view.groups.map((g) => [g.id, g.label]),
    [
      ["memory", "Memory"],
      ["runtime", "Agent runtime"],
      ["intelligence", "Intelligence"],
      ["infrastructure", "Infrastructure"],
    ]
  );
  const infra = view.groups[3];
  assert.deepEqual(
    infra.subgroups.map((s) => [s.label, s.nodes.map((n) => n.id)]),
    [
      ["Persistence", ["pkg:internal/beta"]],
      ["Surfaces", ["pkg:cmd/tool"]],
    ]
  );
  assert.deepEqual(view.groups[0].subgroups, []);
  assert.deepEqual(view.groups[1].subgroups.map((s) => [s.label, s.nodes.map((n) => n.id)]), [["", ["pkg:internal/alpha"]]]);
});

test("groupOverview counts cross-group import edges by each node's own group", () => {
  const view = model.groupOverview(model.mergeData(sampleObjects()));

  // tool -> alpha (infrastructure -> runtime), tool -> beta stays inside
  // infrastructure, alpha -> beta (runtime -> infrastructure), alpha -> gamma
  // (runtime -> intelligence).
  assert.deepEqual(view.links, [
    { from: "runtime", to: "intelligence", count: 1 },
    { from: "runtime", to: "infrastructure", count: 1 },
    { from: "infrastructure", to: "runtime", count: 1 },
  ]);
});

function insideOrOn(p, r) {
  return p.x >= r.x - 0.5 && p.x <= r.x + r.width + 0.5 && p.y >= r.y - 0.5 && p.y <= r.y + r.height + 0.5;
}

function onBoundary(p, r) {
  const near = (a, b) => Math.abs(a - b) <= 0.5;
  return insideOrOn(p, r) && (near(p.x, r.x) || near(p.x, r.x + r.width) || near(p.y, r.y) || near(p.y, r.y + r.height));
}

function overlaps(a, b) {
  return a.x < b.x + b.width && b.x < a.x + a.width && a.y < b.y + b.height && b.y < a.y + a.height;
}

function strictlyInside(p, r) {
  return p.x > r.x + 0.5 && p.x < r.x + r.width - 0.5 && p.y > r.y + 0.5 && p.y < r.y + r.height - 0.5;
}

// manyPackages builds a system shaped like Evie's: groups of very different
// sizes, subgroups, and edges between every pair of groups.
function manyPackages() {
  const groups = { memory: 3, runtime: 13, intelligence: 3 };
  const nodes = [];
  const packages = [];
  for (const [group, count] of Object.entries(groups)) {
    for (let i = 0; i < count; i++) {
      packages.push({ path: `internal/${group}${i}`, name: `${group}${i}`, command: false, doc: "", testSupport: false });
      nodes.push(pkgNode(`internal/${group}${i}`, group));
    }
  }
  const subgroups = { Integrations: 6, "Local host": 5, Observability: 1, Persistence: 1, Surfaces: 5 };
  for (const [subgroup, count] of Object.entries(subgroups)) {
    for (let i = 0; i < count; i++) {
      const p = `internal/${subgroup.replace(" ", "").toLowerCase()}${i}`;
      packages.push({ path: p, name: p, command: false, doc: "", testSupport: false });
      nodes.push(pkgNode(p, "infrastructure", { subgroup }));
    }
  }
  // Every package imports the next one in its group, and the first two
  // packages of each group import the first two of every other group.
  const byGroup = {};
  nodes.forEach((n) => (byGroup[n.group] = byGroup[n.group] || []).push(n.id.slice(4)));
  const imports = [];
  for (const paths of Object.values(byGroup)) {
    for (let i = 0; i + 1 < paths.length; i++) imports.push({ from: paths[i], to: paths[i + 1] });
  }
  for (const [ga, pa] of Object.entries(byGroup)) {
    for (const [gb, pb] of Object.entries(byGroup)) {
      if (ga === gb) continue;
      for (const from of pa.slice(0, 2)) {
        for (const to of pb.slice(0, 2)) imports.push({ from, to });
      }
    }
  }
  return [
    { format: 1, kind: "facts", packages, imports },
    { format: 1, kind: "content", owner: "all", nodes },
  ];
}

test("group overview layout keeps panels and packages apart and packages inside their panel", () => {
  const layout = model.layoutGroupOverview(model.groupOverview(model.mergeData(manyPackages())));

  assert.equal(layout.panels.length, 4);
  assert.equal(layout.nodes.length, 3 + 13 + 3 + 18);
  for (let i = 0; i < layout.panels.length; i++) {
    for (let j = i + 1; j < layout.panels.length; j++) {
      assert.ok(!overlaps(layout.panels[i], layout.panels[j]), `panels ${layout.panels[i].id} and ${layout.panels[j].id} overlap`);
    }
  }
  for (let i = 0; i < layout.nodes.length; i++) {
    for (let j = i + 1; j < layout.nodes.length; j++) {
      assert.ok(!overlaps(layout.nodes[i], layout.nodes[j]), `nodes ${layout.nodes[i].id} and ${layout.nodes[j].id} overlap`);
    }
    const node = layout.nodes[i];
    const panel = layout.panels.find((p) => p.id === node.group);
    for (const corner of [
      { x: node.x, y: node.y },
      { x: node.x + node.width, y: node.y + node.height },
    ]) {
      assert.ok(insideOrOn(corner, panel), `node ${node.id} leaves panel ${panel.id}`);
    }
  }
  for (const node of layout.nodes) {
    for (const label of layout.panels.flatMap((p) => p.subgroups)) {
      assert.ok(!insideOrOn({ x: label.x, y: label.y }, node), `subgroup label ${label.label} sits on node ${node.id}`);
    }
  }
  assert.ok(layout.width >= Math.max(...layout.panels.map((p) => p.x + p.width)));
  assert.ok(layout.height >= Math.max(...layout.panels.map((p) => p.y + p.height)));
});

test("group overview layout starts and ends every dependency arrow on its panels and never crosses a panel", () => {
  const view = model.groupOverview(model.mergeData(manyPackages()));
  const layout = model.layoutGroupOverview(view);

  assert.equal(layout.edges.length, 12, "every ordered pair of the four groups has edges in this fixture");
  for (const edge of layout.edges) {
    const from = layout.panels.find((p) => p.id === edge.from);
    const to = layout.panels.find((p) => p.id === edge.to);
    assert.ok(onBoundary({ x: edge.x1, y: edge.y1 }, from), `edge ${edge.from}->${edge.to} does not start on ${edge.from}`);
    assert.ok(onBoundary({ x: edge.x2, y: edge.y2 }, to), `edge ${edge.from}->${edge.to} does not end on ${edge.to}`);
    assert.equal(edge.count, view.links.find((l) => l.from === edge.from && l.to === edge.to).count);
    for (let t = 0; t <= 1; t += 0.02) {
      const p = { x: edge.x1 + (edge.x2 - edge.x1) * t, y: edge.y1 + (edge.y2 - edge.y1) * t };
      for (const panel of layout.panels) {
        assert.ok(!strictlyInside(p, panel), `edge ${edge.from}->${edge.to} crosses panel ${panel.id}`);
      }
    }
  }
  for (const edge of layout.edges) {
    const back = layout.edges.find((e) => e.from === edge.to && e.to === edge.from);
    const apart = (a, b) => Math.hypot(a[0] - b[0], a[1] - b[1]) >= 10;
    assert.ok(
      apart([edge.x1, edge.y1], [back.x2, back.y2]) && apart([edge.x2, edge.y2], [back.x1, back.y1]),
      `edges ${edge.from}->${edge.to} and back share a line`
    );
  }
  const labels = layout.edges.map((e) => [e.labelX, e.labelY]);
  for (let i = 0; i < labels.length; i++) {
    for (let j = i + 1; j < labels.length; j++) {
      const d = Math.hypot(labels[i][0] - labels[j][0], labels[i][1] - labels[j][1]);
      assert.ok(d >= 20, `count labels ${i} and ${j} are ${d.toFixed(1)}px apart`);
    }
  }
});

test("group overview layout is deterministic and independent of input order", () => {
  const objects = manyPackages();
  const first = model.layoutGroupOverview(model.groupOverview(model.mergeData(objects)));
  const again = model.layoutGroupOverview(model.groupOverview(model.mergeData(manyPackages())));
  assert.deepEqual(again, first);

  const shuffled = manyPackages();
  shuffled[1].nodes.reverse();
  shuffled[0].imports.reverse();
  assert.deepEqual(model.layoutGroupOverview(model.groupOverview(model.mergeData(shuffled))), first);
});

test("packageDetails derives callers and dependencies from the import edges", () => {
  const data = model.mergeData(sampleObjects());
  const alpha = model.packageDetails(data, "pkg:internal/alpha");

  assert.equal(alpha.path, "internal/alpha");
  assert.equal(alpha.pkg.doc, "Package alpha serves.");
  assert.deepEqual(alpha.callers.map((c) => [c.id, c.title]), [["pkg:cmd/tool", "cmd/tool"]]);
  assert.deepEqual(alpha.dependencies.map((d) => d.id), ["pkg:internal/beta", "pkg:internal/gamma"]);
});

test("packageDetails lists glossary terms with their definitions, including terms its statements cite", () => {
  const data = model.mergeData(sampleObjects());
  const alpha = model.packageDetails(data, "pkg:internal/alpha");

  assert.deepEqual(alpha.terms, [
    { term: "Widget", definition: "A thing the fixture makes.", avoid: ["Gadget"] },
    { term: "Ledger Entry", definition: "One recorded line.", avoid: [] },
  ]);
});

test("packageDetails renders any extracted facts that name the package", () => {
  const data = model.mergeData(sampleObjects());

  assert.deepEqual(model.packageDetails(data, "pkg:cmd/tool").facts, [
    { key: "commands", title: "Commands", items: [{ title: "tool serve", detail: ["cmd/tool/main.go main"] }] },
  ]);
  assert.deepEqual(model.packageDetails(data, "pkg:internal/alpha").facts, [
    { key: "routes", title: "Routes", items: [{ title: "GET /api/alpha", detail: ["guard requireOwner", "internal/alpha/http.go routes"] }] },
  ]);
  assert.deepEqual(model.packageDetails(data, "pkg:internal/beta").facts, [
    {
      key: "databases",
      title: "Databases",
      items: [
        {
          title: "fixture.widgets",
          detail: ["id INTEGER not null, primary key 1", "owner_id INTEGER", "(owner_id) references owners (id)"],
        },
      ],
    },
  ]);
  assert.deepEqual(model.packageDetails(data, "pkg:internal/gamma").facts, []);
});

test("packageDetails renders facts lists it does not know generically", () => {
  const objects = sampleObjects();
  objects[0].plugins = [{ name: "research", package: "internal/gamma", enabled: true }];
  const facts = model.packageDetails(model.mergeData(objects), "pkg:internal/gamma").facts;

  assert.deepEqual(facts, [{ key: "plugins", title: "Plugins", items: [{ title: "research", detail: ["enabled: true"] }] }]);
});

test("resolveRef turns references into checkout links and in-page targets", () => {
  const data = model.mergeData(sampleObjects());

  assert.deepEqual(model.resolveRef("code:internal/alpha/alpha.go#Server.Start", data), {
    kind: "code",
    text: "internal/alpha/alpha.go#Server.Start",
    href: "../../../internal/alpha/alpha.go",
  });
  assert.deepEqual(model.resolveRef("doc:docs/guide.md#getting-started", data), {
    kind: "doc",
    text: "docs/guide.md#getting-started",
    href: "../../../docs/guide.md#getting-started",
  });
  assert.deepEqual(model.resolveRef("adr:0001", data), {
    kind: "adr",
    text: "ADR 0001: Make the first decision",
    href: "../../../docs/adr/0001-first-decision.md",
    title: "Make the first decision",
  });
  assert.deepEqual(model.resolveRef("term:Widget", data), {
    kind: "term",
    text: "Widget",
    title: "A thing the fixture makes.",
    definition: "A thing the fixture makes.",
    target: { type: "term", value: "Widget" },
  });
  assert.deepEqual(model.resolveRef("pkg:internal/alpha", data), {
    kind: "pkg",
    text: "internal/alpha",
    target: { type: "node", value: "pkg:internal/alpha" },
  });
  assert.deepEqual(model.resolveRef("node:pkg:internal/gamma", data), {
    kind: "node",
    text: "gamma",
    target: { type: "node", value: "pkg:internal/gamma" },
  });
  assert.deepEqual(model.resolveRef("route:GET /api/alpha", data), {
    kind: "route",
    text: "GET /api/alpha",
    target: { type: "route", value: "GET /api/alpha", node: "pkg:internal/alpha" },
  });
});

test("resolveRef reads route references in the check's one form only", () => {
  const data = model.mergeData(sampleObjects());
  const owner = (ref) => model.resolveRef(ref, data).target.node;

  assert.equal(owner("route:GET /api/alpha"), "pkg:internal/alpha");
  assert.equal(owner("route:/api/alpha"), "pkg:internal/alpha", "without a method, any method matches");
  for (const ref of ["route:GET  /api/alpha", "route:get /api/alpha", "route:GET\t/api/alpha", "route:api/alpha", "route:GET-X /api/alpha"]) {
    assert.equal(owner(ref), undefined, `${JSON.stringify(ref)} is not the check's form`);
  }
});

test("resolveRef encodes paths and refuses links that could leave the checkout", () => {
  const data = model.mergeData(sampleObjects());

  assert.equal(model.resolveRef("doc:docs/a file.md#x", data).href, "../../../docs/a%20file.md#x");
  assert.equal(model.resolveRef("doc:docs/notes?.md#heading one", data).href, "../../../docs/notes%3F.md#heading%20one");
  for (const ref of ["code:javascript:alert(1)", "code:../outside.go", "doc:/etc/passwd", "code:docs/../../x", "doc:https://example.com/x"]) {
    const link = model.resolveRef(ref, data);
    assert.equal(link.href, undefined, `${ref} produced ${link.href}`);
    assert.equal(link.text, ref.slice(ref.indexOf(":") + 1));
  }
  assert.deepEqual(model.resolveRef("mystery", data), { kind: "unknown", text: "mystery" });
  assert.deepEqual(model.resolveRef("adr:0099", data), { kind: "adr", text: "ADR 0099" });
});

test("the address fragment round-trips the view and the selected node", () => {
  const state = { view: "components", node: "pkg:internal/alpha" };
  const hash = model.formatHash(state);

  assert.equal(hash, "#view=components&node=pkg:internal/alpha");
  assert.deepEqual(model.parseHash(hash), state);
  assert.deepEqual(model.parseHash("#node=a%26b%3Dc&unknown=1"), { node: "a&b=c" });
  assert.deepEqual(model.parseHash(""), {});
  assert.deepEqual(model.parseHash(model.formatHash({ node: "x & y" })), { node: "x & y" });
});

// loadRealData evaluates the explorer's own data files the way the page
// does: the manifest lists them and each registers one object.
function loadRealData() {
  const dir = path.join(__dirname, "..", "data");
  const objects = [];
  let listed = [];
  const context = vm.createContext({
    evieExplorer: {
      manifest: (list) => {
        listed = list;
      },
      register: (object) => objects.push(object),
    },
  });
  vm.runInContext(fs.readFileSync(path.join(dir, "manifest.js"), "utf8"), context);
  for (const file of listed) {
    vm.runInContext(fs.readFileSync(path.join(dir, file), "utf8"), context);
  }
  // Objects built inside the vm context belong to another realm; a JSON
  // round trip gives plain objects of this one.
  return JSON.parse(JSON.stringify(objects));
}

test("the real explorer data draws every package exactly once in the group overview", () => {
  const data = model.mergeData(loadRealData());
  const layout = model.layoutGroupOverview(model.groupOverview(data));

  const drawn = layout.nodes.map((n) => n.id).sort();
  const packages = data.facts.packages.map((p) => "pkg:" + p.path).sort();
  assert.deepEqual(drawn, packages);
  for (let i = 0; i < layout.nodes.length; i++) {
    for (let j = i + 1; j < layout.nodes.length; j++) {
      assert.ok(!overlaps(layout.nodes[i], layout.nodes[j]), `nodes ${layout.nodes[i].id} and ${layout.nodes[j].id} overlap`);
    }
  }
});
