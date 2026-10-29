"use strict";

// Every view of the explorer's own data draws soundly: nodes apart, edges on
// their nodes and clear of the others.

const test = require("node:test");
const assert = require("node:assert/strict");
const model = require("../explorer.js");
const { loadRealData } = require("./real-data.js");
const { assertNoOverlap, assertEdgesOnNodes, assertWithinLayout, assertEdgesAvoidNodes } = require("./geometry.js");

function sound(layout, what) {
  assertNoOverlap(layout.nodes, what);
  assertWithinLayout(layout);
  if (layout.edges.length) {
    assertEdgesOnNodes(layout);
    assertEdgesAvoidNodes(layout);
  }
}

test("every group view of the real data draws soundly", () => {
  const data = model.mergeData(loadRealData());
  for (const g of model.GROUPS) {
    const layout = model.layoutGroup(data, g.id);
    assert.ok(layout.nodes.length > 0, `${g.id} has packages`);
    sound(layout, g.id);
  }
});
