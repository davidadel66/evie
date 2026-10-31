"use strict";

// History: the changelog's entries as a month-grouped timeline, newest
// first, and each package's own entries.

const test = require("node:test");
const assert = require("node:assert/strict");
const model = require("../explorer.js");
const { fixtureObjects } = require("./fixture.js");
const { loadRealData } = require("./real-data.js");

function data() {
  return model.mergeData(fixtureObjects());
}

test("the timeline groups entries by month, newest first, and keeps same-day entries in their written order", () => {
  const months = model.historyTimeline(data());

  assert.deepEqual(
    months.map((m) => [m.key, m.label, m.entries.map((e) => e.title)]),
    [
      ["2026-10", "October 2026", ["Teach the tool to export", "Retire the old store"]],
      ["2026-09", "September 2026", ["Start the fixture"]],
    ]
  );
});

test("each timeline card carries its date, title, branch, merge commit or range, text, and sources", () => {
  const [october, september] = model.historyTimeline(data());
  const [exported] = october.entries;

  assert.equal(exported.id, "2026-10-20-teach-the-tool-to-export");
  assert.equal(exported.date, "2026-10-20");
  assert.equal(exported.dateLabel, "Oct 20, 2026");
  assert.equal(exported.branch, "feat/export");
  assert.deepEqual(exported.merge, { sha: "abc1234", href: "https://github.com/davidadel66/evie/commit/abc1234" });
  assert.equal(exported.commits, null);
  assert.deepEqual(exported.text, ["The tool writes its widgets as files.", "Owners asked to keep copies."]);
  assert.deepEqual(exported.sources, ["doc:docs/export.spec.md", "#12"]);

  const [started] = september.entries;
  assert.equal(started.merge, null);
  assert.deepEqual(started.commits, {
    from: { sha: "1111111", href: "https://github.com/davidadel66/evie/commit/1111111" },
    to: { sha: "2222222", href: "https://github.com/davidadel66/evie/commit/2222222" },
  });
});

test("the timeline is newest first even when the facts are not", () => {
  const objects = fixtureObjects();
  objects[0].history.reverse();
  const months = model.historyTimeline(model.mergeData(objects));

  assert.deepEqual(
    months.map((m) => m.entries.map((e) => e.date)),
    [["2026-10-20", "2026-10-20"], ["2026-09-30"]]
  );
});

test("a touched package that exists names its node; a removed one or a path that is not a package names none", () => {
  const [october, september] = model.historyTimeline(data());

  assert.deepEqual(october.entries[1].touches, [
    { path: "internal/gone", node: null },
    { path: "internal/beta", node: "pkg:internal/beta" },
  ]);
  assert.deepEqual(september.entries[0].touches, [
    { path: "internal/alpha", node: "pkg:internal/alpha" },
    { path: "scripts/build", node: null },
  ]);
});

test("a package's details list the History entries that touched it, newest first", () => {
  const d = data();

  assert.deepEqual(
    model.nodeDetails(d, "pkg:internal/beta").history.map((e) => [e.id, e.dateLabel, e.title]),
    [
      ["2026-10-20-teach-the-tool-to-export", "Oct 20, 2026", "Teach the tool to export"],
      ["2026-10-20-retire-the-old-store", "Oct 20, 2026", "Retire the old store"],
    ]
  );
  assert.deepEqual(model.nodeDetails(d, "pkg:internal/gamma").history, []);
  assert.deepEqual(model.nodeDetails(d, "system:main").history, [], "only packages have History");
});

test("search finds History entries by title, text, and branch, and opens the entry in History", () => {
  const d = data();
  const byTitle = model.search(d, "retire");
  assert.deepEqual(byTitle.map((r) => [r.type, r.id]), [["history", "2026-10-20-retire-the-old-store"]]);
  assert.equal(byTitle[0].label, "Retire the old store");
  assert.equal(byTitle[0].detail, "Oct 20, 2026 · chore/retire-store");
  assert.deepEqual(byTitle[0].state, { view: "history", entry: "2026-10-20-retire-the-old-store" });

  assert.deepEqual(model.search(d, "keep copies").map((r) => r.id), ["2026-10-20-teach-the-tool-to-export"]);
  assert.deepEqual(model.search(d, "feat/export").map((r) => r.id), ["2026-10-20-teach-the-tool-to-export"]);
});

test("search ranks a History entry below a package of the same name", () => {
  const objects = fixtureObjects();
  objects[0].history[0].title = "gamma";
  const results = model.search(model.mergeData(objects), "gamma");
  assert.deepEqual(results.map((r) => r.type), ["node", "history"]);
});

test("the History view has its own breadcrumb and draws the timeline", () => {
  const d = data();
  const state = model.resolveState(d, { view: "history" });
  assert.deepEqual(state, { view: "history", proposals: [] });
  assert.deepEqual(model.breadcrumb(d, state), [{ label: "History" }]);

  const layout = model.layoutView(d, state);
  assert.equal(layout.kind, "history");
  assert.deepEqual(layout.months.map((m) => m.key), ["2026-10", "2026-09"]);
  assert.equal(layout.empty, null);
});

test("History with no entries says so", () => {
  const objects = fixtureObjects();
  objects[0].history = [];
  const layout = model.layoutView(model.mergeData(objects), { view: "history" });
  assert.deepEqual(layout.months, []);
  assert.equal(layout.empty.title, "No History yet");
});

test("the address holds the History view and its selected entry, and drops an entry that does not exist", () => {
  const hash = model.formatHash({ view: "history", entry: "2026-10-20-retire-the-old-store" });
  assert.equal(hash, "#view=history&entry=2026-10-20-retire-the-old-store");
  assert.deepEqual(model.parseHash(hash), { view: "history", entry: "2026-10-20-retire-the-old-store" });

  const d = data();
  assert.equal(model.resolveState(d, model.parseHash(hash)).entry, "2026-10-20-retire-the-old-store");
  const missing = model.resolveState(d, { view: "history", entry: "2020-01-01-gone" });
  assert.equal(missing.view, "history");
  assert.equal(missing.entry, undefined);
  assert.match(missing.notice, /no History entry 2020-01-01-gone/);
  assert.equal(model.resolveState(d, { view: "flows", entry: "2026-10-20-retire-the-old-store" }).entry, undefined, "an entry belongs to the History view");
});

test("changing the selection or closing the panel keeps the view part of the address, the selected History entry included", () => {
  const entry = "2026-10-20-retire-the-old-store";
  const history = { view: "history", entry, node: "pkg:internal/beta", term: "Widget", panel: "proposals", proposals: ["sample"] };

  assert.equal(model.formatHash(model.viewState(history)), "#view=history&entry=" + entry + "&proposals=sample", "closing the panel");
  assert.equal(
    model.formatHash(Object.assign(model.viewState(history), { node: "pkg:internal/alpha" })),
    "#view=history&node=pkg:internal/alpha&entry=" + entry + "&proposals=sample",
    "selecting another node"
  );
  assert.deepEqual(model.viewState({ view: "flow", of: "chat", step: 2, node: "pkg:internal/alpha", proposals: [] }), { view: "flow", of: "chat", step: 2, proposals: [] });
  assert.deepEqual(model.viewState({ view: "context" }), { view: "context" });
});

test("a History source that is a ticket or pull-request number opens it on GitHub; any other source resolves as a reference", () => {
  const d = data();
  assert.deepEqual(model.resolveSource("#12", d), { kind: "issue", text: "#12", href: "https://github.com/davidadel66/evie/issues/12" });
  assert.deepEqual(model.resolveSource("doc:docs/export.spec.md", d), model.resolveRef("doc:docs/export.spec.md", d));
  assert.deepEqual(model.resolveSource("pkg:internal/beta", d), model.resolveRef("pkg:internal/beta", d));
  assert.equal(model.resolveSource("#12a", d).href, undefined, "a malformed number gets no link");
});

test("the real History lists every changelog entry once, by month newest first, with existing packages linked", () => {
  const d = model.mergeData(loadRealData());
  const months = model.historyTimeline(d);
  const entries = months.flatMap((m) => m.entries);

  assert.ok(entries.length > 0, "CHANGELOG.md has entries");
  assert.equal(entries.length, d.facts.history.length);
  assert.equal(new Set(entries.map((e) => e.id)).size, entries.length, "entry ids are unique");
  const keys = months.map((m) => m.key);
  assert.deepEqual(keys, [...keys].sort().reverse(), "months are newest first");
  assert.equal(new Set(keys).size, keys.length, "each month appears once");
  for (const e of entries) {
    for (const t of e.touches) {
      assert.equal(t.node !== null, !!d.packageByPath[t.path], `${e.id} touches ${t.path}`);
    }
  }
});
