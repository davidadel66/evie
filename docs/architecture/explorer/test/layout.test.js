"use strict";

// Per-view layouts: every view draws its nodes apart, starts and ends every
// edge on its nodes, and places everything the same way for the same data,
// whatever order the data lists it in.

const test = require("node:test");
const assert = require("node:assert/strict");
const model = require("../explorer.js");
const { fixtureObjects } = require("./fixture.js");
const { assertNoOverlap, assertEdgesOnNodes, assertWithinLayout, assertEdgesAvoidNodes, crossings, center } = require("./geometry.js");

function data(objects) {
  return model.mergeData(objects || fixtureObjects());
}

// shuffled reverses every list in the content and facts, so a layout that
// depends on input order gives a different picture.
function shuffled() {
  const objects = fixtureObjects();
  for (const object of objects) {
    for (const key of ["nodes", "edges", "flows", "imports", "packages"]) {
      if (Array.isArray(object[key])) object[key].reverse();
    }
  }
  return [objects[0]].concat(objects.slice(1).reverse());
}

function nodeAt(layout, id) {
  const node = layout.nodes.find((n) => n.id === id);
  assert.ok(node, `the layout has ${id}`);
  return node;
}

test("context: the owner, the system, and outside systems sit in role columns, connected side to side", () => {
  const layout = model.layoutContext(data());

  assert.equal(layout.kind, "context");
  assert.deepEqual(layout.nodes.map((n) => n.id).sort(), ["external:api", "external:bank", "person:owner", "system:main"]);
  assertNoOverlap(layout.nodes, "context");
  assertWithinLayout(layout);
  assertEdgesOnNodes(layout);
  assert.equal(layout.edges.length, 3);
  const owner = center(nodeAt(layout, "person:owner"));
  const main = center(nodeAt(layout, "system:main"));
  assert.ok(owner.x < main.x, "people sit left of the system");
  for (const id of ["external:api", "external:bank"]) {
    assert.ok(center(nodeAt(layout, id)).x > main.x, `${id} sits right of the system`);
  }
  assert.equal(nodeAt(layout, "person:owner").ref, "person:owner", "a drawn node names the explorer node it stands for");
  assert.equal(layout.empty, null);
});

test("context: the layout is deterministic and independent of input order", () => {
  assert.deepEqual(model.layoutContext(data()), model.layoutContext(data()));
  assert.deepEqual(model.layoutContext(data(shuffled())), model.layoutContext(data()));
});

test("context: a system with no context nodes yet shows an empty state", () => {
  const layout = model.layoutContext(data([{ format: 1, kind: "content", owner: "x", nodes: [] }]));
  assert.deepEqual(layout.nodes, []);
  assert.deepEqual(layout.edges, []);
  assert.ok(layout.empty && layout.empty.title, "the empty state has a title");
});

test("containers: people, interfaces and CLIs, processes, stores, and outside services sit in lanes by kind", () => {
  const layout = model.layoutContainers(data());

  assert.equal(layout.kind, "containers");
  assert.deepEqual(
    layout.lanes.map((l) => [l.id, l.title, [...l.nodes].sort()]),
    [
      ["people", "People", ["person:owner"]],
      ["interfaces", "Interfaces and CLIs", ["container:cli", "container:ui"]],
      ["processes", "Processes", ["container:server"]],
      ["stores", "Stores", ["container:db", "container:files"]],
      ["outside", "Outside services", ["external:api", "external:bank"]],
    ]
  );
  assertNoOverlap(layout.nodes, "containers");
  assertNoOverlap(layout.lanes, "lanes");
  assertWithinLayout(layout);
  for (const lane of layout.lanes) {
    for (const id of lane.nodes) {
      const n = nodeAt(layout, id);
      assert.ok(n.x >= lane.x && n.x + n.width <= lane.x + lane.width && n.y >= lane.y && n.y + n.height <= lane.y + lane.height, `${id} leaves lane ${lane.id}`);
    }
  }
  for (let i = 1; i < layout.lanes.length; i++) {
    assert.ok(layout.lanes[i].x > layout.lanes[i - 1].x, "lanes run left to right");
  }
  assert.ok(!layout.nodes.some((n) => n.id === "system:main"), "the containers view opens the system up instead of drawing it");
  const b = layout.boundary;
  assert.equal(b.title, "Main system");
  for (const id of ["interfaces", "processes", "stores"]) {
    const lane = layout.lanes.find((l) => l.id === id);
    assert.ok(lane.x >= b.x && lane.x + lane.width <= b.x + b.width && lane.y >= b.y && lane.y + lane.height <= b.y + b.height, `lane ${id} sits inside the system boundary`);
  }
  for (const id of ["people", "outside"]) {
    const lane = layout.lanes.find((l) => l.id === id);
    assert.ok(lane.x + lane.width <= b.x || lane.x >= b.x + b.width, `lane ${id} sits outside the system boundary`);
  }
});

test("containers: edges start and end on their nodes and pass between the nodes of lanes they cross", () => {
  const layout = model.layoutContainers(data());

  assertEdgesOnNodes(layout);
  assert.equal(layout.edges.length, 7);
  assertEdgesAvoidNodes(layout);
  const skip = layout.edges.find((e) => e.from === "container:cli" && e.to === "container:db");
  assert.ok(skip, "the CLI's edge to the database skips the processes lane");
});

test("containers: the layout is deterministic and independent of input order", () => {
  assert.deepEqual(model.layoutContainers(data(shuffled())), model.layoutContainers(data()));
});

test("containers: no containers yet shows an empty state", () => {
  const layout = model.layoutContainers(data([{ format: 1, kind: "content", owner: "x", nodes: [] }]));
  assert.deepEqual(layout.nodes, []);
  assert.ok(layout.empty && layout.empty.title);
});

function assertSound(layout, what) {
  assertNoOverlap(layout.nodes, what);
  assertWithinLayout(layout);
  assertEdgesOnNodes(layout);
  assertEdgesAvoidNodes(layout);
}

test("group view: packages are layered by import depth, importers above what they import", () => {
  const layout = model.layoutGroup(data(), "runtime");

  assert.equal(layout.kind, "group");
  assert.equal(layout.title, "Agent runtime");
  const members = layout.nodes.filter((n) => !n.boundary).map((n) => n.id).sort();
  assert.deepEqual(members, ["pkg:internal/alpha", "pkg:internal/delta", "pkg:internal/epsilon", "pkg:internal/zeta"]);
  assertSound(layout, "group");
  for (const e of layout.edges) {
    const from = nodeAt(layout, e.from);
    const to = nodeAt(layout, e.to);
    assert.ok(from.y + from.height <= to.y, `${e.from} sits above ${e.to}`);
  }
  // alpha -> epsilon -> delta -> zeta, and alpha -> zeta directly: the
  // longest path puts zeta three layers below alpha, and the long edge
  // goes around the layers between.
  const ys = ["alpha", "epsilon", "delta", "zeta"].map((p) => nodeAt(layout, "pkg:internal/" + p).y);
  assert.ok(ys[0] < ys[1] && ys[1] < ys[2] && ys[2] < ys[3], "each import sits one layer lower");
  assert.ok(layout.edges.some((e) => e.from === "pkg:internal/alpha" && e.to === "pkg:internal/zeta"));
});

test("group view: out-of-group dependencies are faded boundary chips below the group", () => {
  const layout = model.layoutGroup(data(), "runtime");
  const chips = layout.nodes.filter((n) => n.boundary);

  assert.deepEqual(chips.map((n) => [n.id, n.group]).sort(), [
    ["pkg:internal/beta", "infrastructure"],
    ["pkg:internal/eta", "memory"],
    ["pkg:internal/gamma", "intelligence"],
  ]);
  const lowest = Math.max(...layout.nodes.filter((n) => !n.boundary).map((n) => n.y + n.height));
  for (const chip of chips) {
    assert.ok(chip.faded, `${chip.id} is faded`);
    assert.ok(chip.y >= lowest, `${chip.id} sits below the group`);
  }
  for (const e of layout.edges.filter((e) => chips.some((c) => c.id === e.to))) {
    assert.ok(e.faded, `the edge to ${e.to} is faded`);
  }
});

test("group view: a node shown in other groups joins their view, and an empty group shows an empty state", () => {
  const memory = model.layoutGroup(data(), "memory");
  const beta = nodeAt(memory, "pkg:internal/beta");
  assert.ok(!beta.boundary, "beta is shown in the memory group too");
  assert.equal(beta.group, "infrastructure", "it keeps its own group's colour");

  const empty = model.layoutGroup(data([{ format: 1, kind: "content", owner: "x", nodes: [] }]), "memory");
  assert.deepEqual(empty.nodes, []);
  assert.ok(empty.empty && empty.empty.title);
});

test("group view: ordering removes crossings the input order would cause", () => {
  const pkgs = ["a1", "a2", "b1", "b2", "c1", "c2"];
  const objects = [
    {
      format: 1,
      kind: "facts",
      packages: pkgs.map((p) => ({ path: "internal/" + p, name: p, command: false, doc: "", testSupport: false })),
      imports: [
        { from: "internal/a1", to: "internal/b2" },
        { from: "internal/a2", to: "internal/b1" },
        { from: "internal/b1", to: "internal/c2" },
        { from: "internal/b2", to: "internal/c1" },
      ],
    },
    {
      format: 1,
      kind: "content",
      owner: "x",
      nodes: pkgs.map((p) => ({ id: "pkg:internal/" + p, level: "component", kind: "package", title: p, group: "runtime", summary: { text: p, refs: ["code:x"] } })),
    },
  ];
  const layout = model.layoutGroup(model.mergeData(objects), "runtime");
  assertSound(layout, "crossing fixture");
  assert.equal(crossings(layout), 0);
});

test("group view: a proposal's added, changed, and removed parts carry their change", () => {
  const layout = model.layoutGroup(model.applyProposals(data(), ["sample"]), "runtime");

  assert.equal(nodeAt(layout, "pkg:internal/theta").change, "added");
  assert.equal(nodeAt(layout, "pkg:internal/alpha").change, "changed");
  assert.equal(nodeAt(layout, "pkg:internal/zeta").change, "removed");
  const edge = (from, to) => layout.edges.find((e) => e.from === "pkg:internal/" + from && e.to === "pkg:internal/" + to);
  assert.equal(edge("alpha", "theta").change, "added");
  assert.equal(edge("alpha", "epsilon").change, "changed");
  assert.equal(edge("delta", "zeta").change, "removed");
  assertSound(layout, "group with proposal");
});

test("group view: the layout is deterministic and independent of input order", () => {
  assert.deepEqual(model.layoutGroup(data(shuffled()), "runtime"), model.layoutGroup(data(), "runtime"));
});
