"use strict";

// The address bar: the fragment holds the view, its subject, the selected
// node, the flow step, and the active proposals; the proposal query
// parameter names one extra proposal script.

const test = require("node:test");
const assert = require("node:assert/strict");
const model = require("../explorer.js");

test("the fragment round-trips the view, its subject, the selection, the flow step, and the active proposals", () => {
  const state = { view: "flow", of: "chat", node: "pkg:internal/alpha", step: 3, proposals: ["sample", "second"] };
  const hash = model.formatHash(state);

  assert.equal(hash, "#view=flow&of=chat&node=pkg:internal/alpha&step=3&proposals=sample,second");
  assert.deepEqual(model.parseHash(hash), state);
  assert.equal(model.formatHash(model.parseHash(hash)), hash);
});

test("the fragment keeps awkward values intact", () => {
  const state = { view: "focus", of: "pkg:a&b=c#d", term: "Ledger Entry", proposals: ["with,comma", "plain"], panel: "proposals" };
  const hash = model.formatHash(state);

  assert.deepEqual(model.parseHash(hash), state);
  assert.ok(!hash.slice(1).includes("#"), "a value never closes the fragment early");
});

test("the fragment ignores unknown keys, bad steps, malformed escapes, and empty lists", () => {
  assert.deepEqual(model.parseHash("#view=context&zoom=3&step=abc"), { view: "context" });
  assert.deepEqual(model.parseHash("#step=0"), {});
  assert.deepEqual(model.parseHash("#step=-2"), {});
  assert.deepEqual(model.parseHash("#node=%E0%A4%A"), {});
  assert.deepEqual(model.parseHash("#proposals=,a,,a,b"), { proposals: ["a", "b"] });
  assert.deepEqual(model.parseHash("#proposals="), {});
  assert.equal(model.formatHash({ view: "context", proposals: [] }), "#view=context");
  assert.equal(model.formatHash({}), "");
});

test("the proposal parameter names a relative script inside the explorer directory", () => {
  assert.deepEqual(model.proposalPath("?proposal=examples/design-proposal.js"), { path: "examples/design-proposal.js" });
  assert.deepEqual(model.proposalPath("?x=1&proposal=data/proposals/my-feature.js"), { path: "data/proposals/my-feature.js" });
  assert.deepEqual(model.proposalPath("?proposal=examples%2Fdesign-proposal.js"), { path: "examples/design-proposal.js" });
  assert.equal(model.proposalPath(""), null);
  assert.equal(model.proposalPath("?other=1"), null);
});

test("the proposal parameter refuses absolute paths, schemes, parent directories, and anything that is not a plain script path", () => {
  const refused = [
    "/etc/evil.js",
    "//evil.example/x.js",
    "http://evil.example/x.js",
    "https:x.js",
    "javascript:alert(1)",
    "data:text/javascript,alert(1)",
    "file:///tmp/x.js",
    "C:\\evil.js",
    "\\\\server\\share\\x.js",
    "../outside.js",
    "examples/../../outside.js",
    "examples/./x.js",
    "examples//x.js",
    "examples/",
    "examples/x.txt",
    "examples/x.js?y=1",
    "examples/x.js#frag",
    "%2e%2e/outside.js",
    "%252e%252e/outside.js",
    "examples/%2e%2e/x.js",
    "examples/x y.js",
    "",
  ];
  for (const value of refused) {
    const result = model.proposalPath("?proposal=" + encodeURIComponent(value));
    assert.ok(result && result.error, `${JSON.stringify(value)} was not refused`);
    assert.equal(result.path, undefined);
  }
  const raw = model.proposalPath("?proposal=%E0%A4%A");
  assert.ok(raw && raw.error, "a malformed escape is refused");
});
