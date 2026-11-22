import { expect, test } from "bun:test"
import { panelPlace, panelWidth, treeWidth } from "../web/src/right-panel.ts"

// The right panel's room, as T3 Code's has it: how wide it is beside the
// conversation, and where it is at each width of the window.

test("the right panel is 540px wide at first, and as wide as it was made, but at least 360px, at most 70% of the window, and never so wide that the conversation has less than 360px", () => {
  const wide = { window: 1440, row: 1184 }
  expect(panelWidth(undefined, wide)).toBe(540)
  expect(panelWidth(700, wide)).toBe(700)
  // 70% of the window is 1008px, but the conversation keeps 360px of the 1184.
  expect(panelWidth(1100, wide)).toBe(824)
  expect(panelWidth(1100, { window: 1440, row: 1440 })).toBe(1008)
  expect(panelWidth(200, wide)).toBe(360)
  // Narrower, the 540px it was made gives way to the conversation, down to 360px.
  expect(panelWidth(undefined, { window: 1024, row: 768 })).toBe(408)
  expect(panelWidth(undefined, { window: 990, row: 600 })).toBe(360)
})

test("the right panel sits beside the conversation above 980px, slides over it from the right up to there, and at phone width is T3 Code's narrower sheet", () => {
  expect(panelPlace(1440)).toBe("beside")
  expect(panelPlace(981)).toBe("beside")
  expect(panelPlace(980)).toBe("sheet")
  expect(panelPlace(768)).toBe("sheet")
  expect(panelPlace(767)).toBe("phone")
  expect(panelPlace(390)).toBe("phone")
})

test("the file tree beside the diff is 256px at first, or 40% of the panel when that is less, as wide as it was made between 160px and 560px, and always leaves the diff 256px", () => {
  expect(treeWidth(undefined, 540)).toBe(216)
  expect(treeWidth(undefined, 800)).toBe(256)
  expect(treeWidth(300, 800)).toBe(300)
  expect(treeWidth(100, 800)).toBe(160)
  expect(treeWidth(700, 1000)).toBe(560)
  // The diff keeps 256px of a 540px panel, so the tree has at most 284px.
  expect(treeWidth(400, 540)).toBe(284)
  // A panel too narrow for both gives the tree its least.
  expect(treeWidth(undefined, 360)).toBe(160)
})
