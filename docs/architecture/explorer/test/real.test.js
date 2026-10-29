"use strict";

// Every view of the explorer's own data draws soundly: nodes apart and edges
// on their nodes. Group and focus views also keep edges clear of other
// nodes, which their layouts guarantee for any data.

const test = require("node:test");
const assert = require("node:assert/strict");
const model = require("../explorer.js");
const { loadRealData } = require("./real-data.js");
const { assertNoOverlap, assertEdgesOnNodes, assertWithinLayout, assertEdgesAvoidNodes } = require("./geometry.js");

function sound(layout, what, avoid) {
  assertNoOverlap(layout.nodes, what);
  assertWithinLayout(layout);
  if (layout.edges.length) {
    assertEdgesOnNodes(layout);
    if (avoid) assertEdgesAvoidNodes(layout);
  }
}

test("every group view of the real data draws soundly", () => {
  const data = model.mergeData(loadRealData());
  for (const g of model.GROUPS) {
    const layout = model.layoutGroup(data, g.id);
    assert.ok(layout.nodes.length > 0, `${g.id} has packages`);
    sound(layout, g.id, true);
  }
});

test("the real context and containers views draw soundly", () => {
  const data = model.mergeData(loadRealData());
  sound(model.layoutContext(data), "context");
  sound(model.layoutContainers(data), "containers");
});

test("every real node's focus view, inside view, and diagrams draw soundly", () => {
  const data = model.mergeData(loadRealData());
  for (const node of data.nodes) {
    sound(model.layoutFocus(data, node.id), "focus " + node.id, true);
    if (node.contains.length) sound(model.layoutInside(data, node.id), "inside " + node.id, true);
    for (const diagram of node.diagrams) {
      const layout = model.layoutDiagram(data, diagram);
      if (layout.kind === "sequence" || layout.kind === "er") {
        assertNoOverlap(layout.nodes, diagram.id);
      } else {
        sound(layout, diagram.id);
      }
    }
  }
});

test("every real flow draws as a sequence diagram with every step", () => {
  const data = model.mergeData(loadRealData());
  for (const flow of data.flows) {
    const layout = model.layoutSequence(data, flow);
    assert.equal(layout.steps.length, flow.steps.length, flow.id);
    assertNoOverlap(layout.participants, flow.id);
  }
});
