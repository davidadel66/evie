import { expect, test } from "bun:test"
import { panelPlace, panelWidth } from "../web/src/right-panel.ts"

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

test("the right panel sits beside the conversation above 980px, slides over it from the right up to there, and fills the screen at phone width", () => {
  expect(panelPlace(1440)).toBe("beside")
  expect(panelPlace(981)).toBe("beside")
  expect(panelPlace(980)).toBe("sheet")
  expect(panelPlace(768)).toBe("sheet")
  expect(panelPlace(767)).toBe("screen")
  expect(panelPlace(390)).toBe("screen")
})
