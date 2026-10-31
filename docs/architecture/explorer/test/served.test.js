"use strict";

// The explorer served by evie serve (system-page.spec.md): code and doc
// links open GitHub at the build's commit, or master when the build has no
// commit stamp or had uncommitted changes, and master of the public
// repository when build.js is missing or names no usable repository; opened
// from disk they stay relative file links. History's commits, tickets, and
// pull requests open the build's repository, or the public one. Inside
// Evie's frame the page runs embedded.

const test = require("node:test");
const assert = require("node:assert/strict");
const model = require("../explorer.js");
const { fixtureObjects } = require("./fixture.js");

const REPO = "https://github.com/davidadel66/evie";
const COMMIT = "6c9c8b1a0b5f3e2d4c6a8b0e1f2a3b4c5d6e7f80";

// hrefs resolves one reference of each linked kind for a page served over
// http with build as build.js's information.
function hrefs(build, protocol = "http:") {
  const d = model.mergeData(fixtureObjects());
  const links = model.pageLinks(protocol, build);
  return {
    code: model.resolveRef("code:internal/alpha/alpha.go#Server.Start", d, links).href,
    dir: model.resolveRef("code:internal/alpha", d, links).href,
    doc: model.resolveRef("doc:docs/guide.md#getting-started", d, links).href,
    adr: model.resolveRef("adr:0001", d, links).href,
  };
}

const MASTER = {
  code: REPO + "/blob/master/internal/alpha/alpha.go",
  dir: REPO + "/blob/master/internal/alpha",
  doc: REPO + "/blob/master/docs/guide.md#getting-started",
  adr: REPO + "/blob/master/docs/adr/0001-first-decision.md",
};

const LOCAL = {
  code: "../../../internal/alpha/alpha.go",
  dir: "../../../internal/alpha",
  doc: "../../../docs/guide.md#getting-started",
  adr: "../../../docs/adr/0001-first-decision.md",
};

test("served from a clean stamped build, links open GitHub at the build's commit", () => {
  assert.deepEqual(hrefs({ repository: REPO, commit: COMMIT, modified: false }), {
    code: REPO + "/blob/" + COMMIT + "/internal/alpha/alpha.go",
    dir: REPO + "/blob/" + COMMIT + "/internal/alpha",
    doc: REPO + "/blob/" + COMMIT + "/docs/guide.md#getting-started",
    adr: REPO + "/blob/" + COMMIT + "/docs/adr/0001-first-decision.md",
  });
});

test("served from a build with uncommitted changes or no commit stamp, links open master", () => {
  assert.deepEqual(hrefs({ repository: REPO, commit: COMMIT, modified: true }), MASTER);
  assert.deepEqual(hrefs({ repository: REPO, commit: "", modified: false }), MASTER);
  assert.deepEqual(hrefs({ repository: REPO, commit: "not a commit", modified: false }), MASTER, "a malformed commit is no commit");
  assert.deepEqual(hrefs({ repository: REPO, commit: COMMIT + "0", modified: false }), MASTER, "a commit is at most 40 hex digits");
  assert.deepEqual(hrefs({ repository: REPO, commit: COMMIT.toUpperCase(), modified: false }), MASTER, "a commit is lower-case hex");
});

test("served without build information or a usable repository, links open master on the public repository", () => {
  assert.deepEqual(hrefs(null), MASTER, "build.js did not load");
  assert.deepEqual(hrefs(undefined), MASTER);
  for (const repository of ["", "github.com/davidadel66/evie", "http://github.com/davidadel66/evie", "javascript:alert(1)", "https://github.com/a/b\"><x", "https://github.com/a/../b", "https://"]) {
    assert.deepEqual(hrefs({ repository, commit: COMMIT, modified: false }), MASTER, `${JSON.stringify(repository)} is not a repository`);
  }
});

test("opened from disk, links stay relative file links", () => {
  assert.deepEqual(hrefs(null, "file:"), LOCAL);
  assert.deepEqual(hrefs({ repository: REPO, commit: COMMIT, modified: false }, "file:"), LOCAL, "from disk, build information changes nothing");
  const d = model.mergeData(fixtureObjects());
  assert.equal(model.resolveRef("code:internal/alpha/alpha.go", d).href, LOCAL.code, "without links, links are relative");
});

test("the page loads build information only when it is not opened from disk", () => {
  assert.equal(model.buildScript("file:"), null);
  assert.equal(model.buildScript("http:"), "build.js");
  assert.equal(model.buildScript("https:"), "build.js");
});

test("GitHub links still refuse paths that could leave the repository and encode what they carry", () => {
  const d = model.mergeData(fixtureObjects());
  const links = model.pageLinks("https:", { repository: REPO, commit: COMMIT, modified: false });
  for (const ref of ["code:javascript:alert(1)", "code:../outside.go", "doc:/etc/passwd", "code:docs/../../x", "doc:https://example.com/x"]) {
    assert.equal(model.resolveRef(ref, d, links).href, undefined, `${ref} produced a link`);
  }
  assert.equal(model.resolveRef(`doc:docs/<a> b.md#x"y`, d, links).href, REPO + "/blob/" + COMMIT + "/docs/%3Ca%3E%20b.md#x%22y");
});

test("the embedded flag in the address puts the page in embedded mode", () => {
  assert.equal(model.isEmbedded("?embedded=1"), true);
  assert.equal(model.isEmbedded("?proposal=examples/design-proposal.js&embedded=1"), true);
  assert.equal(model.isEmbedded(""), false);
  assert.equal(model.isEmbedded("?embedded=0"), false);
  assert.equal(model.isEmbedded("?embedded"), false);
  assert.equal(model.isEmbedded("?notembedded=1"), false);
});

test("served, History's sources open GitHub at the build's commit, and its commits, tickets, and pull requests open the build's repository", () => {
  const d = model.mergeData(fixtureObjects());
  const fork = "https://github.com/example/fork";
  const links = model.pageLinks("http:", { repository: fork, commit: COMMIT, modified: false });

  assert.equal(links.repository, fork);
  assert.equal(model.resolveSource("doc:docs/export.spec.md", d, links).href, fork + "/blob/" + COMMIT + "/docs/export.spec.md");
  assert.equal(model.resolveSource("code:internal/alpha", d, links).href, fork + "/blob/" + COMMIT + "/internal/alpha");
  assert.equal(model.resolveSource("#12", d, links).href, fork + "/issues/12");

  const [october, september] = model.historyTimeline(d, links);
  assert.equal(october.entries[0].merge.href, fork + "/commit/abc1234");
  assert.deepEqual(
    [september.entries[0].commits.from.href, september.entries[0].commits.to.href],
    [fork + "/commit/1111111", fork + "/commit/2222222"]
  );
  assert.equal(model.nodeDetails(d, "pkg:internal/beta", links).history[0].merge.href, fork + "/commit/abc1234");
});

test("served without a usable repository, History's sources open master and its GitHub links the public repository", () => {
  const d = model.mergeData(fixtureObjects());
  for (const build of [null, undefined, { repository: "javascript:alert(1)", commit: COMMIT, modified: false }]) {
    const links = model.pageLinks("http:", build);
    assert.equal(links.repository, REPO, `${JSON.stringify(build)} names no repository`);
    assert.equal(model.resolveSource("doc:docs/export.spec.md", d, links).href, REPO + "/blob/master/docs/export.spec.md");
    assert.equal(model.resolveSource("#12", d, links).href, REPO + "/issues/12");
    assert.equal(model.historyTimeline(d, links)[0].entries[0].merge.href, REPO + "/commit/abc1234");
  }
});

test("opened from disk, History's GitHub links open the public repository and its sources stay relative", () => {
  const d = model.mergeData(fixtureObjects());
  const links = model.pageLinks("file:", null);
  assert.equal(links.repository, REPO);
  assert.equal(model.resolveSource("doc:docs/export.spec.md", d, links).href, "../../../docs/export.spec.md");
  assert.equal(model.resolveSource("#12", d, links).href, REPO + "/issues/12");
  assert.equal(model.historyTimeline(d, links)[0].entries[0].merge.href, REPO + "/commit/abc1234");
});
