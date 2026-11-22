import { expect, test } from "bun:test"
import { label, pressed } from "../web/src/shortcuts.ts"

// The web app's keyboard shortcuts, as T3 Code's: one table, which every
// handler reads, matched against a key pressed. "mod" is Cmd on a Mac and
// Ctrl elsewhere.

const none = { code: "", key: "", metaKey: false, ctrlKey: false, altKey: false, shiftKey: false, repeat: false, getModifierState: (_key: string) => false }
const mac = (keys: Partial<typeof none>) => pressed({ ...none, ...keys }, true)
const pc = (keys: Partial<typeof none>) => pressed({ ...none, ...keys }, false)

test("mod is Cmd on a Mac and Ctrl elsewhere, and the other one is not mod", () => {
  expect(mac({ code: "KeyK", key: "k", metaKey: true })).toBe("palette")
  expect(pc({ code: "KeyK", key: "k", ctrlKey: true })).toBe("palette")
  expect(mac({ code: "KeyK", key: "k", ctrlKey: true })).toBeUndefined()
  expect(pc({ code: "KeyK", key: "k", metaKey: true })).toBeUndefined()
  expect(mac({ code: "KeyK", key: "k" })).toBeUndefined()
})

test("every modifier must be as the table says: one more or one less is another key", () => {
  expect(mac({ code: "KeyB", key: "b", metaKey: true })).toBe("session list")
  expect(mac({ code: "KeyB", key: "∫", metaKey: true, altKey: true })).toBe("card")
  expect(mac({ code: "KeyB", key: "b", metaKey: true, shiftKey: true })).toBeUndefined()
  expect(mac({ code: "KeyM", key: "M", metaKey: true, shiftKey: true })).toBe("model")
  expect(mac({ code: "KeyM", key: "m", metaKey: true })).toBeUndefined()
})

test("a letter is the one the keyboard's layout types, and where Option or another alphabet types something else, the one at its place", () => {
  // Dvorak types k with the key where QWERTY has v.
  expect(mac({ code: "KeyV", key: "k", metaKey: true })).toBe("palette")
  // Option and B type ∫ on a Mac; a Russian layout types и there.
  expect(mac({ code: "KeyB", key: "∫", metaKey: true, altKey: true })).toBe("card")
  expect(pc({ code: "KeyK", key: "л", ctrlKey: true })).toBe("palette")
})

test("a new session is mod+shift+O, T3 Code's other key for it, since most browsers keep mod+N for a new window; mod+N works where the browser passes it on", () => {
  expect(mac({ code: "KeyO", key: "O", metaKey: true, shiftKey: true })).toBe("new session")
  expect(mac({ code: "KeyN", key: "n", metaKey: true })).toBe("new session")
})

test("the previous and next session are mod+alt+[ and ], since a Mac's browsers keep mod+shift+[ and ] for tabs, which work where the browser passes them on", () => {
  // Option and [ type “ on a Mac, and Shift and [ type {: the key is known by its place.
  expect(mac({ code: "BracketLeft", key: "“", metaKey: true, altKey: true })).toBe("previous session")
  expect(mac({ code: "BracketRight", key: "‘", metaKey: true, altKey: true })).toBe("next session")
  expect(mac({ code: "BracketLeft", key: "{", metaKey: true, shiftKey: true })).toBe("previous session")
  expect(pc({ code: "BracketRight", key: "}", ctrlKey: true, shiftKey: true })).toBe("next session")
})

test("mod and a digit from 1 to 9 jump to that session, the digit known by its place, as a French keyboard types & there", () => {
  expect(mac({ code: "Digit1", key: "1", metaKey: true })).toBe("session 1")
  expect(pc({ code: "Digit9", key: "9", ctrlKey: true })).toBe("session 9")
  expect(pc({ code: "Digit1", key: "&", ctrlKey: true })).toBe("session 1")
  expect(mac({ code: "Digit0", key: "0", metaKey: true })).toBeUndefined()
})

test("Escape stops and F2 renames, with no modifier", () => {
  expect(mac({ code: "Escape", key: "Escape" })).toBe("stop")
  expect(pc({ code: "F2", key: "F2" })).toBe("rename")
  expect(pc({ code: "F2", key: "F2", ctrlKey: true })).toBeUndefined()
})

test("AltGr, which some keyboards type with Ctrl and Alt, types a character and is no shortcut, and a key held down counts once", () => {
  expect(pc({ code: "KeyB", key: "”", ctrlKey: true, altKey: true, getModifierState: (key: string) => key === "AltGraph" })).toBeUndefined()
  expect(pc({ code: "KeyB", key: "b", ctrlKey: true, altKey: true })).toBe("card")
  expect(pc({ code: "KeyB", key: "b", ctrlKey: true, altKey: true, repeat: true })).toBeUndefined()
  expect(mac({ code: "KeyK", key: "k", metaKey: true, repeat: true })).toBeUndefined()
})

test("a shortcut is shown by its first key, as a Mac writes it, ⌥⇧⌘ before the key, and as other systems do, joined with +", () => {
  expect(label("palette", true)).toBe("⌘K")
  expect(label("card", true)).toBe("⌥⌘B")
  expect(label("model", true)).toBe("⇧⌘M")
  expect(label("previous session", true)).toBe("⌥⌘[")
  expect(label("card", false)).toBe("Ctrl+Alt+B")
  expect(label("new session", false)).toBe("Ctrl+Shift+O")
  expect(label("session 3", false)).toBe("Ctrl+3")
  expect(label("stop", true)).toBe("Esc")
  expect(label("rename", false)).toBe("F2")
})
