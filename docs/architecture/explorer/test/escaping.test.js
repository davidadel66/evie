"use strict";

// Labels are data, never markup: the model passes them through verbatim for
// the page to set as text, and the page never parses data as HTML.

const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const model = require("../explorer.js");
const { fixtureObjects } = require("./fixture.js");

const HOSTILE = `<img src=x onerror="alert(1)"> & 'quotes' </script>`;

function hostileData() {
  const objects = fixtureObjects();
  const content = objects[1];
  content.nodes.find((n) => n.id === "container:server").title = HOSTILE;
  content.flows[0].steps[0].label = HOSTILE;
  content.edges.find((e) => e.from === "container:ui").label = HOSTILE;
  return model.mergeData(objects);
}

test("labels with markup characters come through the layouts verbatim", () => {
  const d = hostileData();
  const containers = model.layoutContainers(d);
  assert.equal(containers.nodes.find((n) => n.id === "container:server").title, HOSTILE);
  assert.equal(containers.edges.find((e) => e.from === "container:ui").label, HOSTILE);
  const flow = model.layoutSequence(d, d.flows.find((f) => f.id === "chat"));
  assert.equal(flow.steps[0].label, HOSTILE);
  assert.equal(flow.steps[0].lines.join(" "), HOSTILE, "wrapping splits at spaces and keeps every character");
  assert.equal(model.search(d, "onerror")[0].label, HOSTILE);
  assert.equal(model.breadcrumb(d, model.resolveState(d, { view: "focus", of: "container:server" })).pop().label, HOSTILE);
});

test("wrapping keeps markup characters as text", () => {
  assert.deepEqual(model.wrapText(`<b>bold</b> & "q"`, 500, 12), [`<b>bold</b> & "q"`]);
});

test("the viewer never parses data as markup or code", () => {
  const source = fs.readFileSync(path.join(__dirname, "..", "explorer.js"), "utf8");
  for (const sink of ["innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "DOMParser", "eval(", "new Function", "createContextualFragment"]) {
    assert.ok(!source.includes(sink), `explorer.js uses ${sink}`);
  }
});

test("hrefs and fragments encode what they carry", () => {
  const d = model.mergeData(fixtureObjects());
  assert.equal(model.resolveRef(`doc:docs/<a>.md#x"y`, d).href, "../../../docs/%3Ca%3E.md#x%22y");
  const hash = model.formatHash({ view: "focus", of: `pkg:"><svg>` });
  assert.ok(!/[<>"]/.test(hash), hash);
  assert.equal(model.parseHash(hash).of, `pkg:"><svg>`);
});
