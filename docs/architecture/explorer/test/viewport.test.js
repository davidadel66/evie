"use strict";

// The viewport: fitting a drawing or a part of it to the canvas, zooming
// about a point, easing between two views, and what hovering a node lights.

const test = require("node:test");
const assert = require("node:assert/strict");
const model = require("../explorer.js");
const { fixtureObjects } = require("./fixture.js");

const near = (a, b) => Math.abs(a - b) < 1e-6;

test("fitting a drawing centres it with padding and keeps the canvas's shape", () => {
  const box = model.fitBox({ x: 0, y: 0, width: 1000, height: 500 }, 800, 600, { pad: 20, maxScale: 2 });
  assert.ok(near(box.width / box.height, 800 / 600), "the view has the canvas's aspect");
  assert.ok(box.x <= -20 && box.x + box.width >= 1020, "the drawing and its padding fit across");
  assert.ok(near(box.x + box.width / 2, 500) && near(box.y + box.height / 2, 250), "the drawing is centred");
  assert.ok(near(box.width, 1040), "the wider side decides the scale");
});

test("fitting a small drawing stops at the largest scale, and an empty one gives a sane view", () => {
  const box = model.fitBox({ x: 10, y: 10, width: 100, height: 50 }, 800, 600, { pad: 0, maxScale: 1.25 });
  assert.ok(near(800 / box.width, 1.25));
  const empty = model.fitBox({ x: 0, y: 0, width: 0, height: 0 }, 800, 600, {});
  assert.ok(empty.width > 0 && empty.height > 0);
});

test("zooming about a point keeps that point still and respects the limits", () => {
  const box = { x: 0, y: 0, width: 800, height: 600 };
  const zoomed = model.zoomBox(box, 2, 200, 150, 800, { minScale: 0.1, maxScale: 4 });
  assert.ok(near(zoomed.width, 400) && near(zoomed.height, 300));
  // The point (200, 150) sat a quarter of the way across and down, and still does.
  assert.ok(near((200 - zoomed.x) / zoomed.width, 0.25) && near((150 - zoomed.y) / zoomed.height, 0.25));
  const capped = model.zoomBox(box, 100, 400, 300, 800, { minScale: 0.1, maxScale: 4 });
  assert.ok(near(800 / capped.width, 4));
  const floored = model.zoomBox(box, 0.001, 400, 300, 800, { minScale: 0.1, maxScale: 4 });
  assert.ok(near(800 / floored.width, 0.1));
});

test("easing between two views starts at one and ends at the other", () => {
  const a = { x: 0, y: 0, width: 100, height: 50 };
  const b = { x: 100, y: 40, width: 300, height: 150 };
  assert.deepEqual(model.lerpBox(a, b, 0), a);
  assert.deepEqual(model.lerpBox(a, b, 1), b);
  assert.deepEqual(model.lerpBox(a, b, 0.5), { x: 50, y: 20, width: 200, height: 100 });
  assert.equal(model.ease(0), 0);
  assert.equal(model.ease(1), 1);
  assert.ok(model.ease(0.25) < 0.25 && model.ease(0.75) > 0.75, "it starts slow and ends slow");
});

test("hovering a node lights it, its neighbours, and the edges between them", () => {
  const layout = model.layoutGroup(model.mergeData(fixtureObjects()), "runtime");
  const lit = model.neighbours(layout, "pkg:internal/epsilon");
  assert.deepEqual(lit.nodes, ["pkg:internal/alpha", "pkg:internal/delta", "pkg:internal/epsilon"]);
  assert.deepEqual(lit.edges.map((i) => [layout.edges[i].from, layout.edges[i].to]), [
    ["pkg:internal/alpha", "pkg:internal/epsilon"],
    ["pkg:internal/epsilon", "pkg:internal/delta"],
  ]);
  assert.deepEqual(model.neighbours(layout, "nope"), { nodes: [], edges: [] });
});
