"use strict";

// Geometry assertions shared by the layout tests. Not a test file.

const assert = require("node:assert/strict");

const EPS = 0.51;

function overlaps(a, b) {
  return a.x < b.x + b.width - EPS && b.x < a.x + a.width - EPS && a.y < b.y + b.height - EPS && b.y < a.y + a.height - EPS;
}

function insideOrOn(p, r) {
  return p.x >= r.x - EPS && p.x <= r.x + r.width + EPS && p.y >= r.y - EPS && p.y <= r.y + r.height + EPS;
}

function onBoundary(p, r) {
  const near = (a, b) => Math.abs(a - b) <= EPS;
  return insideOrOn(p, r) && (near(p.x, r.x) || near(p.x, r.x + r.width) || near(p.y, r.y) || near(p.y, r.y + r.height));
}

// assertNoOverlap fails when any two boxes of the list overlap.
function assertNoOverlap(boxes, what) {
  for (let i = 0; i < boxes.length; i++) {
    for (let j = i + 1; j < boxes.length; j++) {
      assert.ok(!overlaps(boxes[i], boxes[j]), `${what}: ${boxes[i].id} and ${boxes[j].id} overlap`);
    }
  }
}

// assertEdgesOnNodes fails unless every edge starts on the boundary of its
// from node and ends on the boundary of its to node.
function assertEdgesOnNodes(layout) {
  const byId = new Map(layout.nodes.map((n) => [n.id, n]));
  assert.ok(layout.edges.length > 0, "the layout has edges");
  for (const e of layout.edges) {
    const from = byId.get(e.from);
    const to = byId.get(e.to);
    assert.ok(from, `edge ${e.from}->${e.to} has no from node`);
    assert.ok(to, `edge ${e.from}->${e.to} has no to node`);
    assert.ok(onBoundary({ x: e.x1, y: e.y1 }, from), `edge ${e.from}->${e.to} starts off ${e.from}: ${e.x1},${e.y1}`);
    assert.ok(onBoundary({ x: e.x2, y: e.y2 }, to), `edge ${e.from}->${e.to} ends off ${e.to}: ${e.x2},${e.y2}`);
    assert.match(e.d, /^M[-\d.]+,[-\d.]+/, `edge ${e.from}->${e.to} has a path`);
    assert.ok(e.d.startsWith(`M${e.x1},${e.y1}`), `edge ${e.from}->${e.to} path starts at its start point`);
    assert.ok(e.d.endsWith(`${e.x2},${e.y2}`), `edge ${e.from}->${e.to} path ends at its end point`);
  }
}

// assertInside fails unless every box lies within the layout's extent.
function assertWithinLayout(layout) {
  for (const n of layout.nodes) {
    assert.ok(n.x >= 0 && n.y >= 0, `${n.id} starts before the origin`);
    assert.ok(n.x + n.width <= layout.width + EPS, `${n.id} runs past the layout's width`);
    assert.ok(n.y + n.height <= layout.height + EPS, `${n.id} runs past the layout's height`);
  }
}

// samplePath returns points along an SVG path made of M and C commands.
function samplePath(d) {
  const tokens = d.match(/[MC]|-?[\d.]+,-?[\d.]+/g);
  const points = [];
  let current = null;
  let i = 0;
  const xy = (t) => t.split(",").map(Number);
  while (i < tokens.length) {
    const cmd = tokens[i++];
    if (cmd === "M") {
      const [x, y] = xy(tokens[i++]);
      current = { x, y };
      points.push(current);
    } else if (cmd === "C") {
      const [c0, c1, p] = [xy(tokens[i]), xy(tokens[i + 1]), xy(tokens[i + 2])];
      i += 3;
      for (let k = 1; k <= 24; k++) {
        const t = k / 24;
        const u = 1 - t;
        points.push({
          x: u * u * u * current.x + 3 * u * u * t * c0[0] + 3 * u * t * t * c1[0] + t * t * t * p[0],
          y: u * u * u * current.y + 3 * u * u * t * c0[1] + 3 * u * t * t * c1[1] + t * t * t * p[1],
        });
      }
      current = { x: p[0], y: p[1] };
    } else {
      throw new Error("unexpected path token " + cmd);
    }
  }
  return points;
}

function strictlyInside(p, r, inset) {
  const m = inset == null ? 1 : inset;
  return p.x > r.x + m && p.x < r.x + r.width - m && p.y > r.y + m && p.y < r.y + r.height - m;
}

// assertEdgesAvoidNodes fails when an edge's path runs through a node other
// than the two it joins.
function assertEdgesAvoidNodes(layout, skip) {
  for (const e of layout.edges) {
    if (skip && skip(e)) continue;
    for (const p of samplePath(e.d)) {
      for (const n of layout.nodes) {
        if (n.id === e.from || n.id === e.to) continue;
        assert.ok(!strictlyInside(p, n), `edge ${e.from}->${e.to} runs through ${n.id} at ${p.x.toFixed(1)},${p.y.toFixed(1)}`);
      }
    }
  }
}

function segmentsCross(p, q, r, s) {
  const d = (a, b, c) => (b.x - a.x) * (c.y - a.y) - (b.y - a.y) * (c.x - a.x);
  const d1 = d(r, s, p);
  const d2 = d(r, s, q);
  const d3 = d(p, q, r);
  const d4 = d(p, q, s);
  return d1 * d2 < 0 && d3 * d4 < 0;
}

// crossings counts the pairs of edges, joining four different nodes, whose
// paths cross.
function crossings(layout) {
  const paths = layout.edges.map((e) => ({ e, points: samplePath(e.d) }));
  let count = 0;
  for (let i = 0; i < paths.length; i++) {
    for (let j = i + 1; j < paths.length; j++) {
      const a = paths[i];
      const b = paths[j];
      if ([a.e.from, a.e.to].some((id) => id === b.e.from || id === b.e.to)) continue;
      let crossed = false;
      for (let k = 0; k + 1 < a.points.length && !crossed; k++) {
        for (let m = 0; m + 1 < b.points.length && !crossed; m++) {
          crossed = segmentsCross(a.points[k], a.points[k + 1], b.points[m], b.points[m + 1]);
        }
      }
      if (crossed) count++;
    }
  }
  return count;
}

function center(n) {
  return { x: n.x + n.width / 2, y: n.y + n.height / 2 };
}

module.exports = { overlaps, insideOrOn, onBoundary, assertNoOverlap, assertEdgesOnNodes, assertWithinLayout, assertEdgesAvoidNodes, samplePath, strictlyInside, crossings, center };

// labelBoxes returns the boxes of a layout's edge labels.
function labelBoxes(layout) {
  return layout.edges
    .filter((e) => e.labelLines && e.labelLines.length)
    .map((e) => ({
      id: `${e.from}->${e.to}:${e.label}`,
      x: e.labelX - e.labelWidth / 2,
      y: e.labelY - (e.labelLines.length * 14 + 6) / 2,
      width: e.labelWidth,
      height: e.labelLines.length * 14 + 6,
    }));
}

module.exports.labelBoxes = labelBoxes;
