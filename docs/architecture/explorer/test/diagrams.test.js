"use strict";

// A node's own diagrams: graphs (with and without lanes), state diagrams,
// and er diagrams drawn from the facts.

const test = require("node:test");
const assert = require("node:assert/strict");
const model = require("../explorer.js");
const { fixtureObjects } = require("./fixture.js");
const { assertNoOverlap, assertEdgesOnNodes, assertWithinLayout, assertEdgesAvoidNodes, onBoundary } = require("./geometry.js");

function data() {
  return model.mergeData(fixtureObjects());
}

function diagram(id, d) {
  return (d || data()).nodeById["pkg:internal/delta"].diagrams.find((g) => g.id === id);
}

function sound(layout, what) {
  assertNoOverlap(layout.nodes, what);
  assertWithinLayout(layout);
  assertEdgesOnNodes(layout);
  assertEdgesAvoidNodes(layout);
}

function nodeAt(layout, id) {
  const n = layout.nodes.find((x) => x.id === id);
  assert.ok(n, `the layout has ${id}`);
  return n;
}

test("a plain graph diagram is layered top to bottom with its labels", () => {
  const layout = model.layoutDiagram(data(), diagram("delta-plain"));

  assert.equal(layout.kind, "graph");
  assert.deepEqual(layout.nodes.map((n) => [n.id, n.title]).sort(), [
    ["w", "Side"],
    ["x", "Front"],
    ["y", "Middle"],
    ["z", "Back"],
  ]);
  sound(layout, "plain graph");
  assert.ok(nodeAt(layout, "x").y < nodeAt(layout, "y").y && nodeAt(layout, "y").y < nodeAt(layout, "z").y);
  assert.deepEqual(layout.edges.map((e) => e.label).sort(), ["", "", "", "shortcut"]);
});

test("a graph diagram with lanes puts each node in its lane's band, flowing left to right", () => {
  const layout = model.layoutDiagram(data(), diagram("delta-lanes"));

  assert.deepEqual(layout.lanes.map((l) => [l.id, l.title, [...l.nodes].sort()]), [
    ["ingest", "Ingest", ["parse", "read"]],
    ["store", "Store", ["index", "save"]],
  ]);
  assertNoOverlap(layout.lanes, "lanes");
  for (const lane of layout.lanes) {
    for (const id of lane.nodes) {
      const n = nodeAt(layout, id);
      assert.ok(n.y >= lane.y && n.y + n.height <= lane.y + lane.height && n.x >= lane.x && n.x + n.width <= lane.x + lane.width, `${id} sits in lane ${lane.id}`);
    }
  }
  sound(layout, "lane graph");
  assert.ok(nodeAt(layout, "read").x < nodeAt(layout, "parse").x && nodeAt(layout, "parse").x < nodeAt(layout, "save").x);
  assert.equal(nodeAt(layout, "save").ref, "pkg:internal/beta", "a diagram node can stand for an explorer node");
  assert.equal(nodeAt(layout, "read").ref, null);
  assert.equal(nodeAt(layout, "parse").title, "Parse <widget> & check", "labels are kept as text, never as markup");
});

test("a state diagram marks its initial state and draws every transition, including loops and returns", () => {
  const layout = model.layoutDiagram(data(), diagram("delta-states"));

  assert.equal(layout.kind, "state");
  const initial = layout.nodes.find((n) => n.shape === "initial");
  assert.ok(initial, "the initial marker is drawn");
  const queued = nodeAt(layout, "queued");
  assert.ok(initial.y + initial.height <= queued.y, "the initial marker sits above the initial state");
  assert.equal(nodeAt(layout, "done").title, "done", "a state without a label shows its id");
  assert.equal(layout.edges.length, 6, "five transitions and the initial arrow");
  sound(layout, "state");
  const loop = layout.edges.find((e) => e.from === "running" && e.to === "running");
  assert.equal(loop.label, "heartbeat");
  assert.ok(loop.x1 === loop.x2 && loop.y2 > loop.y1, "a self transition loops on the state's side");
  const back = layout.edges.find((e) => e.from === "failed" && e.to === "queued");
  assert.ok(back.y1 > back.y2, "a transition back to an earlier state runs upward");
  assert.deepEqual(back.refs, ["code:internal/delta"]);
});

test("an er diagram draws each table with its columns from the facts and joins foreign keys column to column", () => {
  const layout = model.layoutDiagram(data(), diagram("delta-er"));

  assert.equal(layout.kind, "er");
  const widgets = nodeAt(layout, "widgets");
  assert.deepEqual(
    widgets.columns.map((c) => [c.name, c.type, c.primaryKey, c.foreignKey, c.notNull]),
    [
      ["id", "INTEGER", 1, false, false],
      ["owner_id", "INTEGER", 0, true, true],
      ["parent_id", "INTEGER", 0, true, false],
      ["kind", "TEXT", 0, false, true],
    ]
  );
  assert.equal(widgets.ref, "pkg:internal/beta", "a table selects the package that owns its database");
  const owners = nodeAt(layout, "owners");
  assert.ok(owners.stub && owners.faded, "a referenced table the diagram does not list is a faded stub");
  assert.deepEqual(owners.columns, []);
  assertNoOverlap(layout.nodes, "er");
  assertWithinLayout(layout);
  assert.equal(layout.edges.length, 4);
  for (const e of layout.edges) {
    const child = nodeAt(layout, e.from);
    const parent = nodeAt(layout, e.to);
    assert.ok(onBoundary({ x: e.x1, y: e.y1 }, child), `${e.from}.${e.fromColumn} starts on its table`);
    assert.ok(onBoundary({ x: e.x2, y: e.y2 }, parent), `${e.to} ends on its table`);
    const row = child.columns.find((c) => c.name === e.fromColumn);
    assert.ok(e.y1 >= child.y + row.y && e.y1 <= child.y + row.y + row.height, `${e.from}.${e.fromColumn} starts at its row`);
    if (!parent.stub) {
      const target = parent.columns.find((c) => c.name === e.toColumn);
      assert.ok(e.y2 >= parent.y + target.y && e.y2 <= parent.y + target.y + target.height, `${e.to}.${e.toColumn} ends at its row`);
    }
  }
  const self = layout.edges.find((e) => e.from === "widgets" && e.to === "widgets");
  assert.equal(self.fromColumn, "parent_id");
  assert.equal(self.toColumn, "id");
});

test("an er diagram for a database the facts do not have yet shows an empty state, and missing tables say so", () => {
  const d = data();
  const missingDb = model.layoutDiagram(d, { id: "x", title: "X", kind: "er", database: "nope", tables: ["a"] });
  assert.deepEqual(missingDb.nodes, []);
  assert.ok(missingDb.empty && missingDb.empty.title);

  const missingTable = model.layoutDiagram(d, { id: "y", title: "Y", kind: "er", database: "fixture", tables: ["audit", "ghost"] });
  assert.ok(nodeAt(missingTable, "ghost").missing);
});

test("diagram layouts are deterministic", () => {
  for (const id of ["delta-lanes", "delta-plain", "delta-states", "delta-er", "delta-sequence"]) {
    assert.deepEqual(model.layoutDiagram(data(), diagram(id)), model.layoutDiagram(data(), diagram(id)), id);
  }
  assert.equal(model.layoutDiagram(data(), diagram("delta-sequence")).kind, "sequence");
});

test("findDiagram finds a diagram and the node that carries it", () => {
  const found = model.findDiagram(data(), "delta-states");
  assert.equal(found.node.id, "pkg:internal/delta");
  assert.equal(found.diagram.title, "Job lifecycle");
  assert.equal(model.findDiagram(data(), "nope"), null);
});

test("transition labels never cover one another", () => {
  const { labelBoxes } = require("./geometry.js");
  const objects = require("./fixture.js").fixtureObjects();
  const delta = objects[2].nodes.find((n) => n.id === "pkg:internal/delta");
  const states = delta.diagrams.find((g) => g.id === "delta-states");
  // Four transitions leave "running" for nearby states, all labelled.
  states.states.push({ id: "paused", label: "Paused", refs: ["code:internal/delta"] }, { id: "lost", label: "Lost", refs: ["code:internal/delta"] });
  states.transitions.push(
    { from: "running", to: "paused", label: "pause requested by the owner", refs: ["code:internal/delta"] },
    { from: "running", to: "lost", label: "heartbeat expired", refs: ["code:internal/delta"] },
    { from: "paused", to: "running", label: "resume", refs: ["code:internal/delta"] }
  );
  const d = model.mergeData(objects);
  const layout = model.layoutDiagram(d, model.findDiagram(d, "delta-states").diagram);
  assert.ok(labelBoxes(layout).length >= 7);
  assertNoOverlap(labelBoxes(layout), "transition labels");
});
