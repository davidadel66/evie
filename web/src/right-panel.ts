// The right panel's room, apart from its screen, as T3 Code's right panel
// has it: how wide it is beside the conversation, and where it is at each
// width of the window.

// How wide it is: 540px at first, or as wide as the owner made it; at
// least 360px; at most 70% of the window; and never so wide that the
// conversation beside it, in the `row` they share, has less than 360px.
const FIRST_WIDTH = 540
const LEAST_WIDTH = 360
// Seven tenths, worked out in whole numbers so that 70% of 1440 is 1008, not 1007.99.
const MOST_OF_WINDOW = { of: 7, in: 10 }
const LEAST_CONVERSATION = 360
export function panelWidth(wanted: number | undefined, room: { window: number; row: number }): number {
  const most = Math.max(LEAST_WIDTH, Math.min(Math.floor((room.window * MOST_OF_WINDOW.of) / MOST_OF_WINDOW.in), Math.floor(room.row) - LEAST_CONVERSATION))
  return Math.min(Math.max(wanted ?? FIRST_WIDTH, LEAST_WIDTH), most)
}

// Where the panel is at a window's width, as T3 Code's: beside the
// conversation above 980px; at 980px or narrower, a sheet that slides over
// it from the right; and at phone width, as wide.ts and the stylesheet's
// `wide` name it, T3 Code's narrower sheet, in which the tree takes the
// diff's place and a diff is always stacked.
export type PanelPlace = "beside" | "sheet" | "phone"
export const BESIDE = "(min-width: 981px)"
export const PHONE = "(max-width: 767px)"
export function panelPlace(window: number): PanelPlace {
  return window > 980 ? "beside" : window >= 768 ? "sheet" : "phone"
}

// How wide the file tree beside the diff is, in a panel this wide, as T3
// Code's: 256px at first, or 40% of the panel when that is less, or as
// wide as the owner made it; between 160px and 560px; and never so wide
// that the diff has less than 256px, but for the tree's least.
const TREE_FIRST = 256
const TREE_SHARE = { of: 2, in: 5 }
const TREE_LEAST = 160
const TREE_MOST = 560
const LEAST_DIFF = 256
export function treeWidth(wanted: number | undefined, panel: number): number {
  const first = Math.min(TREE_FIRST, Math.floor((panel * TREE_SHARE.of) / TREE_SHARE.in))
  const most = Math.max(TREE_LEAST, Math.min(TREE_MOST, panel - LEAST_DIFF))
  return Math.min(Math.max(wanted ?? first, TREE_LEAST), most)
}
