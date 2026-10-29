"use strict";

// What the panel shows for a node and a design proposal, and where
// references lead.

const test = require("node:test");
const assert = require("node:assert/strict");
const model = require("../explorer.js");
const { fixtureObjects } = require("./fixture.js");

function data() {
  return model.mergeData(fixtureObjects());
}

test("a node's details list the flows it takes part in, with its steps", () => {
  const alpha = model.nodeDetails(data(), "pkg:internal/alpha");
  assert.deepEqual(alpha.flows, [{ id: "chat", title: "Chat turn", steps: [1, 2, 5, 7], change: "" }]);
  assert.deepEqual(model.nodeDetails(data(), "pkg:internal/gamma").flows, []);
});

test("a node's details list its diagrams, what it contains, and what contains it", () => {
  const d = data();
  assert.deepEqual(
    model.nodeDetails(d, "pkg:internal/delta").diagrams.map((g) => [g.id, g.kind]),
    [
      ["delta-lanes", "graph"],
      ["delta-plain", "graph"],
      ["delta-sequence", "sequence"],
      ["delta-states", "state"],
      ["delta-er", "er"],
    ]
  );
  assert.deepEqual(model.nodeDetails(d, "container:server").contains.map((n) => n.id), ["pkg:internal/alpha", "pkg:internal/epsilon", "pkg:internal/delta"]);
  assert.deepEqual(model.nodeDetails(d, "pkg:internal/alpha").containedBy, [{ id: "container:server", title: "server" }]);
});

test("a node's details split its import graph from its other connections", () => {
  const server = model.nodeDetails(data(), "container:server");
  assert.deepEqual(server.callers, []);
  assert.deepEqual(server.incoming.map((r) => [r.id, r.title, r.label, r.via]), [["container:ui", "Web UI", "calls", "http"]]);
  assert.deepEqual(server.outgoing.map((r) => [r.id, r.label, r.via]), [
    ["container:db", "reads and writes", "sql"],
    ["external:api", "asks", "network"],
  ]);
  assert.deepEqual(server.outgoing[0].refs, ["code:internal/beta"]);

  const delta = model.nodeDetails(data(), "pkg:internal/delta");
  assert.deepEqual(delta.outgoing.map((r) => r.id), ["external:bank"]);
  assert.deepEqual(delta.dependencies.map((p) => p.id), ["pkg:internal/beta", "pkg:internal/zeta"]);
});

test("with a proposal on, a node's details show what it changes", () => {
  const shown = model.applyProposals(data(), ["sample"]);
  const alpha = model.nodeDetails(shown, "pkg:internal/alpha");

  assert.equal(alpha.change, "changed");
  assert.deepEqual(alpha.changed, ["summary", "terms"]);
  assert.equal(alpha.proposal.title, "Add theta");
  assert.deepEqual(alpha.dependencies.map((p) => [p.id, p.change || ""]), [
    ["pkg:internal/epsilon", "changed"],
    ["pkg:internal/gamma", ""],
    ["pkg:internal/theta", "added"],
    ["pkg:internal/zeta", "removed"],
  ]);
  assert.deepEqual(alpha.terms.map((t) => t.term), ["Plugin", "Widget", "Ledger Entry"], "the changed terms, then terms its statements cite");
  assert.deepEqual(alpha.flows.map((f) => [f.id, f.title, f.change]), [
    ["chat", "Chat turn with theta", "changed"],
    ["theta-run", "Theta run", "added"],
  ]);
});

test("a proposal's details list its requirements, decisions, and every operation", () => {
  const details = model.proposalDetails(data(), "sample");

  assert.equal(details.title, "Add theta");
  assert.equal(details.status, "example");
  assert.equal(details.spec, "doc:docs/theta.spec.md");
  assert.deepEqual(details.requirements.map((s) => s.text), ["Theta must run nightly."]);
  assert.deepEqual(details.decisions.map((s) => s.text), ["Theta replaces zeta."]);
  assert.deepEqual(details.operations.map((o) => [o.what, o.op, o.label]), [
    ["node", "add", "theta"],
    ["node", "change", "alpha"],
    ["node", "remove", "zeta"],
    ["node", "change", "server"],
    ["edge", "add", "alpha → theta"],
    ["edge", "add", "theta → delta"],
    ["edge", "change", "alpha → epsilon"],
    ["edge", "remove", "delta → zeta"],
    ["edge", "change", "server → Model API"],
    ["flow", "add", "Theta run"],
    ["flow", "change", "Chat turn"],
    ["flow", "remove", "Legacy export"],
  ]);
  assert.equal(details.operations[0].target, "pkg:internal/theta");
  assert.equal(details.operations[9].target, "flow:theta-run");
  assert.equal(model.proposalDetails(data(), "nope"), null);
});

test("table, route, and command references jump to the package that defines them", () => {
  const d = data();
  assert.deepEqual(model.resolveRef("table:fixture.widgets", d).target, { type: "table", value: "fixture.widgets", node: "pkg:internal/beta" });
  assert.deepEqual(model.resolveRef("route:GET /api/alpha", d).target, { type: "route", value: "GET /api/alpha", node: "pkg:internal/alpha" });
  assert.deepEqual(model.resolveRef("route:/healthz", d).target, { type: "route", value: "/healthz", node: "pkg:internal/alpha" });
  assert.deepEqual(model.resolveRef("command:tool migrate up", d).target, { type: "command", value: "tool migrate up", node: "pkg:cmd/tool" });
  assert.deepEqual(model.resolveRef("command:tool", d).target, { type: "command", value: "tool", node: "pkg:cmd/tool" });
  assert.deepEqual(model.resolveRef("table:fixture.ghost", d).target, { type: "table", value: "fixture.ghost" });
});

test("glossary references carry their definition and ADR references their title", () => {
  const d = data();
  const term = model.resolveRef("term:Ledger Entry", d);
  assert.equal(term.title, "One recorded line.");
  assert.equal(term.definition, "One recorded line.");
  const adr = model.resolveRef("adr:0001", d);
  assert.equal(adr.text, "ADR 0001: Make the first decision");
});
