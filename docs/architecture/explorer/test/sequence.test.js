"use strict";

// Flows and sequence diagrams: participants with lifelines, and numbered
// steps between them, top to bottom.

const test = require("node:test");
const assert = require("node:assert/strict");
const model = require("../explorer.js");
const { fixtureObjects } = require("./fixture.js");
const { assertNoOverlap } = require("./geometry.js");

function data() {
  return model.mergeData(fixtureObjects());
}

function chat(d) {
  return d.flows.find((f) => f.id === "chat");
}

test("a flow draws its participants left to right in order, each with a lifeline", () => {
  const d = data();
  const layout = model.layoutSequence(d, chat(d));

  assert.equal(layout.kind, "sequence");
  assert.deepEqual(layout.participants.map((p) => p.id), ["person:owner", "pkg:internal/alpha", "pkg:internal/epsilon", "pkg:internal/delta"]);
  assert.deepEqual(layout.participants.map((p) => p.title), ["Owner", "alpha", "epsilon", "delta"]);
  assertNoOverlap(layout.participants, "participants");
  for (let i = 1; i < layout.participants.length; i++) {
    assert.ok(layout.participants[i].lifeline > layout.participants[i - 1].lifeline);
  }
  for (const p of layout.participants) {
    assert.equal(p.lifeline, p.x + p.width / 2, `${p.id}'s lifeline runs down its middle`);
    assert.ok(p.lifelineTop >= p.y + p.height && p.lifelineBottom > p.lifelineTop);
  }
  assert.deepEqual(layout.nodes, layout.participants, "participants are the view's selectable nodes");
});

test("a flow's steps are numbered, run top to bottom, and start and end on their participants' lifelines", () => {
  const d = data();
  const layout = model.layoutSequence(d, chat(d));
  const life = Object.fromEntries(layout.participants.map((p) => [p.id, p]));

  assert.deepEqual(layout.steps.map((s) => s.number), [1, 2, 3, 4, 5, 6, 7]);
  assert.deepEqual(layout.steps.map((s) => s.kind), ["call", "call", "call", "async", "note", "return", "return"]);
  for (let i = 1; i < layout.steps.length; i++) {
    assert.ok(layout.steps[i].y > layout.steps[i - 1].y, `step ${i + 1} sits below step ${i}`);
  }
  for (const s of layout.steps) {
    const from = life[s.from];
    const to = life[s.to];
    assert.ok(s.y > from.lifelineTop && s.y < from.lifelineBottom, `step ${s.number} sits on the lifelines`);
    if (s.kind === "note") {
      assert.ok(s.box.x < Math.min(from.lifeline, to.lifeline) && s.box.x + s.box.width > Math.max(from.lifeline, to.lifeline), "a note spans its participants");
      continue;
    }
    assert.equal(s.x1, from.lifeline, `step ${s.number} starts on ${s.from}`);
    assert.equal(s.x2, to.lifeline, `step ${s.number} ends on ${s.to}`);
    assert.ok(s.d.startsWith(`M${s.x1},${s.y}`));
    if (s.self) {
      assert.ok(s.d.endsWith(`${s.x2},${s.y2}`) && s.y2 > s.y, "a self call loops back down to its own lifeline");
    } else {
      assert.ok(s.d.endsWith(`${s.x2},${s.y}`));
    }
  }
});

test("every step's label fits between the lifelines it joins", () => {
  const d = data();
  const layout = model.layoutSequence(d, chat(d));
  for (const s of layout.steps) {
    if (s.kind === "note" || s.self) continue;
    const span = Math.abs(s.x2 - s.x1);
    for (const line of s.lines) {
      assert.ok(model.textWidth(line, 12) <= span - 16, `step ${s.number}'s label line "${line}" fits in ${span}`);
    }
    assert.ok(s.lines.join(" ").length > 0);
  }
  const long = layout.steps.find((s) => s.number === 2);
  assert.ok(long.lines.length >= 2, "a long label wraps");
});

test("a sequence diagram on a node draws like a flow, and the layout is deterministic", () => {
  const d = data();
  const diagram = d.nodeById["pkg:internal/delta"].diagrams.find((g) => g.kind === "sequence");
  const layout = model.layoutSequence(d, diagram);
  assert.deepEqual(layout.participants.map((p) => p.id), ["pkg:internal/delta", "pkg:internal/beta"]);
  assert.equal(layout.steps.length, 2);
  assert.deepEqual(model.layoutSequence(data(), chat(data())), model.layoutSequence(d, chat(d)));
});

test("a proposal's changed and removed flows carry their change, and steps keep any change of their own", () => {
  const shown = model.applyProposals(data(), ["sample"]);
  const flows = model.flowIndex(shown);
  assert.deepEqual(
    flows.map((f) => [f.id, f.title, f.steps, f.change]),
    [
      ["chat", "Chat turn with theta", 7, "changed"],
      ["legacy", "Legacy export", 1, "removed"],
      ["theta-run", "Theta run", 1, "added"],
    ]
  );
  const layout = model.layoutSequence(shown, shown.flows.find((f) => f.id === "theta-run"));
  assert.equal(layout.participants.find((p) => p.id === "pkg:internal/theta").change, "added");
});

test("the flows index lists every flow by title, and none yet is an empty list", () => {
  assert.deepEqual(
    model.flowIndex(data()).map((f) => [f.id, f.participants, f.steps]),
    [
      ["chat", 4, 7],
      ["legacy", 2, 1],
    ]
  );
  assert.deepEqual(model.flowIndex(model.mergeData([])), []);
});

test("a message across many lifelines is labelled near its sender, where the view follows it", () => {
  const d = data();
  const ids = ["person:owner", "pkg:internal/alpha", "pkg:internal/epsilon", "pkg:internal/delta", "pkg:internal/zeta", "pkg:internal/beta"];
  const flow = {
    id: "wide",
    title: "Wide",
    participants: ids,
    steps: [
      { from: ids[0], to: ids[5], label: "Reaches across", kind: "call", refs: ["code:x"] },
      { from: ids[5], to: ids[0], label: "Comes back", kind: "return", refs: ["code:x"] },
      { from: ids[1], to: ids[2], label: "Next door", kind: "call", refs: ["code:x"] },
    ],
  };
  const layout = model.layoutSequence(d, flow);
  const [out, back, near] = layout.steps;
  assert.ok(out.labelX - out.x1 < (out.x2 - out.x1) / 3, "the label sits near the sender, not mid-span");
  assert.ok(back.x1 - back.labelX < (back.x1 - back.x2) / 3, "the same going right to left");
  assert.equal(near.labelX, (near.x1 + near.x2) / 2, "a short message keeps its label in the middle");
  for (const s of [out, back]) {
    assert.ok(Math.abs(s.labelX - s.x1) >= s.labelWidth / 2, "the label clears the sender's lifeline");
  }
});
