"use strict";

// loadRealData evaluates the explorer's own data files the way the page
// does: the manifest lists them and each registers one object. Not a test
// file.

const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

function loadRealData(extra) {
  const root = path.join(__dirname, "..");
  const objects = [];
  let listed = [];
  const context = vm.createContext({
    evieExplorer: {
      manifest: (list) => {
        listed = list;
      },
      register: (object) => objects.push(object),
    },
  });
  vm.runInContext(fs.readFileSync(path.join(root, "data", "manifest.js"), "utf8"), context);
  for (const file of listed) {
    vm.runInContext(fs.readFileSync(path.join(root, "data", file), "utf8"), context);
  }
  for (const file of extra || []) {
    vm.runInContext(fs.readFileSync(path.join(root, file), "utf8"), context);
  }
  // Objects built inside the vm context belong to another realm; a JSON
  // round trip gives plain objects of this one.
  return JSON.parse(JSON.stringify(objects));
}

module.exports = { loadRealData };
