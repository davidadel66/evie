"use strict";

// Navigation: which view an address shows, what the breadcrumb offers, and
// which layout draws it.

const test = require("node:test");
const assert = require("node:assert/strict");
const model = require("../explorer.js");
const { fixtureObjects } = require("./fixture.js");

function data() {
  return model.mergeData(fixtureObjects());
}

test("an empty address opens the system context", () => {
  assert.deepEqual(model.resolveState(data(), {}), { view: "context", proposals: [] });
});

test("an address keeps a valid view, subject, selection, step, and proposals", () => {
  const raw = { view: "flow", of: "chat", node: "pkg:internal/alpha", step: 3, proposals: ["sample"], panel: "proposals" };
  assert.deepEqual(model.resolveState(data(), raw), raw);
  assert.deepEqual(model.resolveState(data(), { view: "group", of: "memory", term: "Widget" }), { view: "group", of: "memory", term: "Widget", proposals: [] });
});

test("an address naming something that does not exist falls back with a notice", () => {
  const d = data();
  const cases = [
    [{ view: "group", of: "nope" }, "components"],
    [{ view: "focus", of: "pkg:internal/ghost" }, "components"],
    [{ view: "inside", of: "container:ui" }, "containers"],
    [{ view: "flow", of: "ghost-flow" }, "flows"],
    [{ view: "diagram", of: "ghost-diagram" }, "components"],
    [{ view: "teleport" }, "context"],
  ];
  for (const [raw, view] of cases) {
    const state = model.resolveState(d, raw);
    assert.equal(state.view, view, JSON.stringify(raw));
    assert.equal(state.of, undefined);
    assert.ok(state.notice, `${JSON.stringify(raw)} explains the fallback`);
  }
  const quiet = model.resolveState(d, { view: "context", node: "ghost", term: "Ghost", proposals: ["ghost", "sample"], panel: "elsewhere" });
  assert.deepEqual(quiet, { view: "context", proposals: ["sample"] });
});

test("a flow step stays within the flow, and a step outside a stepped view is dropped", () => {
  const d = data();
  assert.equal(model.resolveState(d, { view: "flow", of: "chat" }).step, 1);
  assert.equal(model.resolveState(d, { view: "flow", of: "chat", step: 99 }).step, 7);
  assert.equal(model.resolveState(d, { view: "diagram", of: "delta-sequence", step: 2 }).step, 2, "a sequence diagram steps too");
  assert.equal(model.resolveState(d, { view: "diagram", of: "delta-states", step: 2 }).step, undefined);
  assert.equal(model.resolveState(d, { view: "context", step: 2 }).step, undefined);
});

test("a proposal's added flow and node can be addressed while the proposal is on", () => {
  const d = model.applyProposals(data(), ["sample"]);
  assert.equal(model.resolveState(d, { view: "flow", of: "theta-run", proposals: ["sample"] }).view, "flow");
  assert.equal(model.resolveState(d, { view: "focus", of: "pkg:internal/theta", proposals: ["sample"] }).of, "pkg:internal/theta");
});

function crumbs(state) {
  return model.breadcrumb(data(), model.resolveState(data(), state)).map((c) => [c.label, c.state ? c.state.view + (c.state.of ? ":" + c.state.of : "") : null]);
}

test("the breadcrumb leads back up through the levels", () => {
  assert.deepEqual(crumbs({ view: "context" }), [["Context", null]]);
  assert.deepEqual(crumbs({ view: "inside", of: "container:server" }), [
    ["Context", "context"],
    ["Containers", "containers"],
    ["server", null],
  ]);
  assert.deepEqual(crumbs({ view: "group", of: "runtime" }), [
    ["Components", "components"],
    ["Agent runtime", null],
  ]);
  assert.deepEqual(crumbs({ view: "focus", of: "pkg:internal/alpha" }), [
    ["Components", "components"],
    ["Agent runtime", "group:runtime"],
    ["alpha", null],
  ]);
  assert.deepEqual(crumbs({ view: "focus", of: "container:server" }), [
    ["Context", "context"],
    ["Containers", "containers"],
    ["server", null],
  ]);
  assert.deepEqual(crumbs({ view: "flow", of: "chat" }), [
    ["Flows", "flows"],
    ["Chat turn", null],
  ]);
  assert.deepEqual(crumbs({ view: "diagram", of: "delta-states" }), [
    ["Components", "components"],
    ["Agent runtime", "group:runtime"],
    ["delta", "focus:pkg:internal/delta"],
    ["Job lifecycle", null],
  ]);
});

test("each view belongs to one of the four level tabs", () => {
  const level = (view) => model.levelOf(view);
  assert.deepEqual(
    ["context", "containers", "inside", "components", "group", "focus", "flows", "flow", "diagram"].map(level),
    ["context", "containers", "containers", "components", "components", "components", "flows", "flows", "components"]
  );
  assert.deepEqual(model.LEVELS.map((l) => [l.id, l.label]), [
    ["context", "Context"],
    ["containers", "Containers"],
    ["components", "Components"],
    ["flows", "Flows"],
  ]);
});

test("layoutView draws whatever view the state names", () => {
  const d = data();
  const kinds = [
    [{ view: "context" }, "context"],
    [{ view: "containers" }, "containers"],
    [{ view: "inside", of: "container:server" }, "inside"],
    [{ view: "components" }, "overview"],
    [{ view: "group", of: "runtime" }, "group"],
    [{ view: "focus", of: "pkg:internal/alpha" }, "focus"],
    [{ view: "flows" }, "flows"],
    [{ view: "flow", of: "chat" }, "sequence"],
    [{ view: "diagram", of: "delta-er" }, "er"],
    [{ view: "diagram", of: "delta-lanes" }, "graph"],
  ];
  for (const [state, kind] of kinds) {
    assert.equal(model.layoutView(d, model.resolveState(d, state)).kind, kind, JSON.stringify(state));
  }
  assert.deepEqual(model.layoutView(d, { view: "group", of: "runtime" }), model.layoutGroup(d, "runtime"));
  const flows = model.layoutView(model.mergeData([]), { view: "flows" });
  assert.deepEqual(flows.items, []);
  assert.ok(flows.empty && flows.empty.title, "no flows yet shows an empty state");
});

test("the group overview shows a proposal's added and removed packages and counts only edges that remain", () => {
  const view = model.groupOverview(model.applyProposals(data(), ["sample"]));
  const runtime = view.groups.find((g) => g.id === "runtime");
  const nodes = runtime.subgroups.flatMap((s) => s.nodes);
  assert.equal(nodes.find((n) => n.id === "pkg:internal/theta").change, "added");
  assert.equal(nodes.find((n) => n.id === "pkg:internal/zeta").change, "removed");
  // zeta -> eta (runtime -> memory) is removed with zeta.
  assert.equal(view.links.find((l) => l.from === "runtime" && l.to === "memory"), undefined);
});
