"use strict";

// Design proposal overlay: what a proposal adds, changes, and removes, and
// how each kind of change is styled.

const test = require("node:test");
const assert = require("node:assert/strict");
const model = require("../explorer.js");
const { fixtureObjects } = require("./fixture.js");

function data() {
  return model.mergeData(fixtureObjects());
}

test("an active proposal adds, changes, and removes nodes without touching the built data", () => {
  const built = data();
  const shown = model.applyProposals(built, ["sample"]);

  const theta = shown.nodeById["pkg:internal/theta"];
  assert.equal(theta.change, "added");
  assert.equal(theta.proposal, "sample");
  assert.equal(theta.group, "runtime");
  assert.deepEqual(theta.does, []);

  const alpha = shown.nodeById["pkg:internal/alpha"];
  assert.equal(alpha.change, "changed");
  assert.equal(alpha.summary.text, "Alpha starts theta.");
  assert.deepEqual(alpha.changed, ["summary", "terms"]);
  assert.deepEqual(alpha.terms, ["Plugin"]);
  assert.equal(alpha.does.length, 1, "fields the change leaves out stay as built");
  assert.equal(alpha.was.summary.text, "internal/alpha does things.");

  assert.equal(shown.nodeById["pkg:internal/zeta"].change, "removed");
  assert.ok(shown.nodes.some((n) => n.id === "pkg:internal/zeta"), "a removed node is still drawn, struck through");

  assert.equal(built.nodeById["pkg:internal/theta"], undefined);
  assert.equal(built.nodeById["pkg:internal/alpha"].change, undefined);
  assert.equal(built.nodeById["pkg:internal/alpha"].summary.text, "internal/alpha does things.");
  assert.deepEqual(shown.active, ["sample"]);
});

test("no active proposal leaves the built system as it is", () => {
  const built = data();
  const shown = model.applyProposals(built, []);
  assert.deepEqual(
    shown.nodes.map((n) => [n.id, n.change]),
    built.nodes.map((n) => [n.id, undefined])
  );
  assert.deepEqual(shown.active, []);
  assert.deepEqual(model.applyProposals(built, ["no-such-proposal"]).active, []);
});

function link(d, from, to, via) {
  return d.links.find((l) => l.from === from && l.to === to && l.via === via);
}

test("the built links are the facts' import edges between package nodes plus the content edges, each once", () => {
  const built = data();

  const importEdge = link(built, "pkg:internal/alpha", "pkg:internal/epsilon", "import");
  assert.equal(importEdge.label, "starts work", "a content import edge labels the facts edge it matches");
  assert.equal(built.links.filter((l) => l.from === "pkg:internal/alpha" && l.to === "pkg:internal/epsilon").length, 1);
  assert.ok(link(built, "pkg:cmd/tool", "pkg:internal/beta", "import"));
  assert.deepEqual(link(built, "container:ui", "container:server", "http").refs, ["code:internal/alpha"]);
  assert.equal(built.links.length, 11 + 11, "eleven imports and eleven non-import content edges");
});

test("an active proposal adds, changes, and removes edges, and edges of a removed node go with it", () => {
  const shown = model.applyProposals(data(), ["sample"]);

  assert.equal(link(shown, "pkg:internal/alpha", "pkg:internal/theta", "import").change, "added");
  assert.equal(link(shown, "pkg:internal/theta", "pkg:internal/delta", "import").change, "added");
  const handed = link(shown, "pkg:internal/alpha", "pkg:internal/epsilon", "import");
  assert.equal(handed.change, "changed");
  assert.equal(handed.label, "hands over");
  assert.equal(handed.was.label, "starts work");
  assert.equal(link(shown, "container:server", "external:api", "network").label, "streams");
  assert.equal(link(shown, "pkg:internal/delta", "pkg:internal/zeta", "import").change, "removed");
  assert.equal(link(shown, "pkg:internal/alpha", "pkg:internal/zeta", "import").change, "removed", "its from node stays, its to node goes");
  assert.equal(link(shown, "pkg:internal/zeta", "pkg:internal/eta", "import").change, "removed");
  assert.equal(link(shown, "pkg:cmd/tool", "pkg:internal/beta", "import").change, undefined);
});

test("an active proposal adds, changes, and removes flows", () => {
  const shown = model.applyProposals(data(), ["sample"]);
  const flows = Object.fromEntries(shown.flows.map((f) => [f.id, f]));

  assert.equal(flows["theta-run"].change, "added");
  assert.equal(flows.chat.change, "changed");
  assert.equal(flows.chat.title, "Chat turn with theta");
  assert.equal(flows.chat.steps.length, 7, "steps the change leaves out stay as built");
  assert.equal(flows.legacy.change, "removed");
  assert.equal(data().flows.find((f) => f.id === "chat").title, "Chat turn");
});

test("proposals stack: each active proposal adds its own parts", () => {
  const shown = model.applyProposals(data(), ["second", "sample"]);

  assert.deepEqual(shown.active, ["sample", "second"], "proposals apply in the order the data lists them");
  assert.equal(shown.nodeById["pkg:internal/iota"].proposal, "second");
  assert.equal(shown.nodeById["pkg:internal/theta"].proposal, "sample");
  assert.equal(link(shown, "pkg:internal/iota", "pkg:internal/eta", "import").change, "added");
});

test("each kind of change has its own style: added dashed, changed badged, removed struck through and faded", () => {
  assert.deepEqual(model.changeStyle("added"), { className: "change-added", label: "Added", dashed: true, badge: "", struck: false, faded: false });
  assert.deepEqual(model.changeStyle("changed"), { className: "change-changed", label: "Changed", dashed: false, badge: "Δ", struck: false, faded: false });
  assert.deepEqual(model.changeStyle("removed"), { className: "change-removed", label: "Removed", dashed: false, badge: "", struck: true, faded: true });
  assert.equal(model.changeStyle(undefined), null);
  assert.equal(model.changeStyle("renamed"), null);
});

test("the design proposal legend explains every kind of change in a fixed order", () => {
  assert.deepEqual(
    model.proposalLegend().map((entry) => [entry.change, entry.className, entry.text]),
    [
      ["added", "change-added", "Added by a design proposal (dashed outline)"],
      ["changed", "change-changed", "Changed by a design proposal (Δ badge)"],
      ["removed", "change-removed", "Removed by a design proposal (struck through)"],
    ]
  );
});
