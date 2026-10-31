"use strict";

// The explorer served by evie serve (system-page.spec.md): code and doc
// links open GitHub at the build's commit, or master when the build has no
// commit stamp or had uncommitted changes; opened from disk they stay
// relative file links. Inside Evie's frame the page runs embedded.

const test = require("node:test");
const assert = require("node:assert/strict");
const model = require("../explorer.js");
const { fixtureObjects } = require("./fixture.js");

const REPO = "https://github.com/davidadel66/evie";
const COMMIT = "6c9c8b1a0b5f3e2d4c6a8b0e1f2a3b4c5d6e7f80";

function hrefs(build) {
  const d = model.mergeData(fixtureObjects());
  const root = model.linkRoot(build);
  return {
    code: model.resolveRef("code:internal/alpha/alpha.go#Server.Start", d, root).href,
    dir: model.resolveRef("code:internal/alpha", d, root).href,
    doc: model.resolveRef("doc:docs/guide.md#getting-started", d, root).href,
    adr: model.resolveRef("adr:0001", d, root).href,
  };
}

test("served from a clean stamped build, links open GitHub at the build's commit", () => {
  assert.deepEqual(hrefs({ repository: REPO, commit: COMMIT, modified: false }), {
    code: REPO + "/blob/" + COMMIT + "/internal/alpha/alpha.go",
    dir: REPO + "/blob/" + COMMIT + "/internal/alpha",
    doc: REPO + "/blob/" + COMMIT + "/docs/guide.md#getting-started",
    adr: REPO + "/blob/" + COMMIT + "/docs/adr/0001-first-decision.md",
  });
});

test("served from a build with uncommitted changes or no commit stamp, links open master", () => {
  const master = {
    code: REPO + "/blob/master/internal/alpha/alpha.go",
    dir: REPO + "/blob/master/internal/alpha",
    doc: REPO + "/blob/master/docs/guide.md#getting-started",
    adr: REPO + "/blob/master/docs/adr/0001-first-decision.md",
  };
  assert.deepEqual(hrefs({ repository: REPO, commit: COMMIT, modified: true }), master);
  assert.deepEqual(hrefs({ repository: REPO, commit: "", modified: false }), master);
  assert.deepEqual(hrefs({ repository: REPO, commit: "not a commit", modified: false }), master, "a malformed commit is no commit");
});

test("opened from disk, or without usable build information, links stay relative file links", () => {
  const local = {
    code: "../../../internal/alpha/alpha.go",
    dir: "../../../internal/alpha",
    doc: "../../../docs/guide.md#getting-started",
    adr: "../../../docs/adr/0001-first-decision.md",
  };
  assert.deepEqual(hrefs(null), local);
  assert.deepEqual(hrefs(undefined), local);
  for (const repository of ["", "github.com/davidadel66/evie", "http://github.com/davidadel66/evie", "javascript:alert(1)", "https://github.com/a/b\"><x", "https://github.com/a/../b", "https://"]) {
    assert.deepEqual(hrefs({ repository, commit: COMMIT, modified: false }), local, `${JSON.stringify(repository)} is not a repository`);
  }
  const d = model.mergeData(fixtureObjects());
  assert.equal(model.resolveRef("code:internal/alpha/alpha.go", d).href, local.code, "without a root, links are relative");
});

test("the page loads build information only when it is not opened from disk", () => {
  assert.equal(model.buildScript("file:"), null);
  assert.equal(model.buildScript("http:"), "build.js");
  assert.equal(model.buildScript("https:"), "build.js");
});

test("GitHub links still refuse paths that could leave the repository and encode what they carry", () => {
  const d = model.mergeData(fixtureObjects());
  const root = model.linkRoot({ repository: REPO, commit: COMMIT, modified: false });
  for (const ref of ["code:javascript:alert(1)", "code:../outside.go", "doc:/etc/passwd", "code:docs/../../x", "doc:https://example.com/x"]) {
    assert.equal(model.resolveRef(ref, d, root).href, undefined, `${ref} produced a link`);
  }
  assert.equal(model.resolveRef(`doc:docs/<a> b.md#x"y`, d, root).href, REPO + "/blob/" + COMMIT + "/docs/%3Ca%3E%20b.md#x%22y");
});

test("the embedded flag in the address puts the page in embedded mode", () => {
  assert.equal(model.isEmbedded("?embedded=1"), true);
  assert.equal(model.isEmbedded("?proposal=examples/design-proposal.js&embedded=1"), true);
  assert.equal(model.isEmbedded(""), false);
  assert.equal(model.isEmbedded("?embedded=0"), false);
  assert.equal(model.isEmbedded("?embedded"), false);
  assert.equal(model.isEmbedded("?notembedded=1"), false);
});
