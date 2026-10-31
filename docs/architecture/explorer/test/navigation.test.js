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

test("each view belongs to one of the five level tabs", () => {
  const level = (view) => model.levelOf(view);
  assert.deepEqual(
    ["context", "containers", "inside", "components", "group", "focus", "flows", "flow", "diagram", "history"].map(level),
    ["context", "containers", "containers", "components", "components", "components", "flows", "flows", "components", "history"]
  );
  assert.deepEqual(model.LEVELS.map((l) => [l.id, l.label]), [
    ["context", "Context"],
    ["containers", "Containers"],
    ["components", "Components"],
    ["flows", "Flows"],
    ["history", "History"],
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

test("an address naming an inherited object key resolves to nothing, without crashing", () => {
  const d = data();
  for (const name of ["constructor", "toString", "__proto__", "hasOwnProperty", "valueOf"]) {
    for (const hash of [`#view=inside&of=${name}`, `#view=focus&of=${name}`, `#view=group&of=${name}`, `#view=flow&of=${name}`, `#view=diagram&of=${name}`, `#view=${name}`]) {
      const state = model.resolveState(d, model.parseHash(hash));
      assert.equal(state.of, undefined, hash);
      assert.ok(state.notice, `${hash} explains the fallback`);
      model.layoutView(d, state);
      model.breadcrumb(d, state);
    }
    const quiet = model.resolveState(d, model.parseHash(`#node=${name}&term=${name}&proposals=${name}`));
    assert.deepEqual(quiet, { view: "context", proposals: [] }, name);
    const details = model.nodeDetails(d, name);
    assert.equal(details.node, null, name);
    assert.equal(details.pkg, null, name);
    for (const kind of ["node", "term", "adr", "table", "route", "command", "pkg"]) {
      const link = model.resolveRef(kind + ":" + name, d);
      assert.equal(link.title, undefined, `${kind}:${name} has no title`);
      assert.equal(link.definition, undefined, `${kind}:${name} has no definition`);
      assert.equal(link.href, undefined, `${kind}:${name} has no link`);
      if (link.target) assert.equal(link.target.node, undefined, `${kind}:${name} has no owner`);
    }
    assert.equal(model.resolveRef("node:" + name, d).text, name);
    assert.equal(model.resolveRef("adr:" + name, d).text, "ADR " + name);
    assert.equal(model.levelOf(name), "context");
    assert.deepEqual(model.search(d, name).filter((r) => r.id === name), []);
  }
  const prefixed = model.nodeDetails(d, "pkg:constructor");
  assert.equal(prefixed.pkg, null, "a package path is looked up as an own key too");
});

test("a node or term whose id is an inherited object key is found like any other", () => {
  const objects = require("./fixture.js").fixtureObjects();
  const facts = objects.find((o) => o.kind === "facts");
  facts.glossary.push({ term: "constructor", definition: "Builds things.", avoid: [] });
  const content = objects.find((o) => o.kind === "content");
  content.nodes.push({ id: "toString", level: "container", kind: "process", title: "Stringer", summary: { text: "Turns things into text.", refs: ["code:internal/alpha"] }, contains: ["pkg:internal/alpha"] });
  const d = model.mergeData(objects);

  assert.equal(model.resolveState(d, model.parseHash("#view=inside&of=toString")).of, "toString");
  assert.equal(model.resolveState(d, model.parseHash("#term=constructor")).term, "constructor");
  assert.equal(model.nodeDetails(d, "toString").node.title, "Stringer");
  assert.equal(model.resolveRef("term:constructor", d).definition, "Builds things.");
  assert.equal(model.layoutView(d, { view: "inside", of: "toString" }).kind, "inside");
});
