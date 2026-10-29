"use strict";

// The real extracted facts in the panel: routes, commands, and tables reach
// the packages that own them. Run with:
// node --test 'docs/architecture/explorer/test/*.test.js'

const test = require("node:test");
const assert = require("node:assert/strict");
const model = require("../explorer.js");
const { loadRealData } = require("./real-data.js");

function sectionTitles(data, id) {
  return model.packageDetails(data, id).facts.map((section) => section.key);
}

test("the panel lists the web package's routes with their guards", () => {
  const data = model.mergeData(loadRealData());
  const [routes] = model.packageDetails(data, "pkg:internal/web").facts;

  assert.equal(routes.key, "routes");
  const chat = routes.items.find((item) => item.title === "POST /api/chat");
  assert.ok(chat, "POST /api/chat is listed");
  assert.equal(chat.detail[0], "guard guard");
  const status = routes.items.find((item) => item.title === "/api/chats/status");
  assert.equal(status.detail[0], "guard managementRoute");
});

test("the panel lists commands for each command package", () => {
  const data = model.mergeData(loadRealData());
  for (const [pkg, command] of [
    ["cmd/evie", "evie workflows propose"],
    ["cmd/finance", "finance sync"],
    ["cmd/todo", "todo add"],
    ["cmd/ytscribe", "ytscribe fetch"],
  ]) {
    const [commands] = model.packageDetails(data, "pkg:" + pkg).facts;
    assert.equal(commands.key, "commands", pkg);
    assert.ok(
      commands.items.some((item) => item.title === command),
      `${pkg} lists ${command}`
    );
  }
});

test("the panel lists tables for the eviedb, finance, and youtube packages", () => {
  const data = model.mergeData(loadRealData());
  for (const [pkg, table] of [
    ["internal/eviedb", "evie.workspaces"],
    ["internal/finance", "finance.transactions"],
    ["internal/youtube", "youtube.videos"],
  ]) {
    assert.deepEqual(sectionTitles(data, "pkg:" + pkg), ["databases"], pkg);
    const [tables] = model.packageDetails(data, "pkg:" + pkg).facts;
    assert.ok(
      tables.items.some((item) => item.title === table),
      `${pkg} lists ${table}`
    );
  }
});
