"use strict";

// Search finds nodes by name or package path, glossary terms by term or
// avoided synonym, flows, and diagrams, best matches first.

const test = require("node:test");
const assert = require("node:assert/strict");
const model = require("../explorer.js");
const { fixtureObjects } = require("./fixture.js");

function data() {
  return model.mergeData(fixtureObjects());
}

test("search finds a package by name, exact matches first, and says where it leads", () => {
  const results = model.search(data(), "Alpha");
  assert.deepEqual(results.slice(0, 2).map((r) => [r.type, r.id]), [
    ["node", "pkg:internal/alpha"],
    ["node", "pkg:internal/alphatest"],
  ]);
  assert.equal(results[0].detail, "internal/alpha");
  assert.deepEqual(results[0].state, { view: "focus", of: "pkg:internal/alpha", node: "pkg:internal/alpha" });
});

test("search finds a package by its path", () => {
  assert.equal(model.search(data(), "internal/epsilon")[0].id, "pkg:internal/epsilon");
  assert.equal(model.search(data(), "cmd/to")[0].id, "pkg:cmd/tool");
});

test("search finds context and container nodes and opens them where they are drawn", () => {
  assert.deepEqual(model.search(data(), "bank")[0].state, { view: "context", node: "external:bank" });
  assert.deepEqual(model.search(data(), "web ui")[0].state, { view: "containers", node: "container:ui" });
});

test("search finds glossary terms by term and by avoided synonym, with their definitions", () => {
  const ledger = model.search(data(), "ledger");
  assert.deepEqual(ledger.map((r) => [r.type, r.id]), [["term", "Ledger Entry"]]);
  assert.equal(ledger[0].detail, "One recorded line.");
  assert.deepEqual(ledger[0].state, { term: "Ledger Entry" });
  const synonym = model.search(data(), "skill");
  assert.equal(synonym[0].id, "Plugin");
  assert.match(synonym[0].label, /Plugin/);
});

test("search finds flows and diagrams", () => {
  assert.deepEqual(model.search(data(), "chat")[0].state, { view: "flow", of: "chat" });
  const diagram = model.search(data(), "lifecycle")[0];
  assert.equal(diagram.type, "diagram");
  assert.deepEqual(diagram.state, { view: "diagram", of: "delta-states", node: "pkg:internal/delta" });
});

test("search ignores case and blank queries, and caps its results", () => {
  assert.deepEqual(model.search(data(), "   "), []);
  assert.deepEqual(model.search(data(), "zzz-no-match"), []);
  assert.equal(model.search(data(), "a", 3).length, 3);
  assert.deepEqual(model.search(data(), "GAMMA").map((r) => r.id), ["pkg:internal/gamma"]);
});
