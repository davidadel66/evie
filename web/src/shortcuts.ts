// The web app's keyboard shortcuts, as T3 Code's, in one table that every
// handler reads, the command palette shows beside its rows, and the README
// lists. "mod" is Cmd on a Mac and Ctrl elsewhere. Where a browser keeps
// T3 Code's key for itself, the key it never takes comes first, and T3
// Code's is kept after it, for where the browser passes it on, as in a
// window of its own.
// The shortcuts are not changed in the settings.

export type Command =
  | "palette"
  | "new session"
  | "previous session"
  | "next session"
  | `session ${1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9}`
  | "session list"
  | "card"
  | "model"
  | "stop"
  | "rename"

export type Shortcut = { command: Command; keys: readonly string[]; does: string }

export const SHORTCUTS: readonly Shortcut[] = [
  { command: "palette", keys: ["mod+k"], does: "Open the command palette" },
  // Most browsers open a new window on mod+N, before the page hears of it.
  { command: "new session", keys: ["mod+shift+o", "mod+n"], does: "A new session" },
  // A Mac's browsers move between tabs on mod+shift+[ and ].
  { command: "previous session", keys: ["mod+alt+[", "mod+shift+["], does: "The session above in the list" },
  { command: "next session", keys: ["mod+alt+]", "mod+shift+]"], does: "The session below in the list" },
  ...([1, 2, 3, 4, 5, 6, 7, 8, 9] as const).map((at) => ({ command: `session ${at}` as const, keys: [`mod+${at}`], does: `The list's session ${at}` })),
  { command: "session list", keys: ["mod+b"], does: "Show or hide the session list" },
  { command: "card", keys: ["mod+alt+b"], does: "Show or hide the project card" },
  { command: "model", keys: ["mod+shift+m"], does: "Pick a model" },
  { command: "stop", keys: ["escape"], does: "Stop evie, in the message box" },
  { command: "rename", keys: ["f2"], does: "Rename the session" },
]

// A key pressed, as a keyboard event gives it.
export type Keys = {
  code: string
  key: string
  metaKey: boolean
  ctrlKey: boolean
  altKey: boolean
  shiftKey: boolean
  repeat: boolean
  getModifierState(key: string): boolean
}

// The command a key pressed is the shortcut for, or nothing. AltGr, which
// some keyboards report as Ctrl and Alt, types a character, so it is no
// shortcut; and a key held down counts once.
export function pressed(keys: Keys, mac: boolean): Command | undefined {
  if (keys.repeat || keys.getModifierState("AltGraph")) return undefined
  const key = keyOf(keys)
  const mod = mac ? keys.metaKey : keys.ctrlKey
  const other = mac ? keys.ctrlKey : keys.metaKey
  if (other) return undefined
  return SHORTCUTS.find((shortcut) =>
    shortcut.keys.some((each) => {
      const parts = each.split("+")
      return parts.at(-1) === key && parts.includes("mod") === mod && parts.includes("shift") === keys.shiftKey && parts.includes("alt") === keys.altKey
    }),
  )?.command
}

// Which key it is, as T3 Code tells, adapted from its keybindingMatching.ts
// (MIT licence, notice in LICENSES/t3code-MIT.txt): a letter is the one the layout types,
// or where Option or another alphabet types something else, the one at its
// place; a digit or a bracket is the one at its place, since Shift, Option
// or the layout can type another character there; any other key by its name.
function keyOf({ key, code }: Keys): string {
  const typed = key.toLowerCase()
  if (/^[a-z]$/.test(typed)) return typed
  const letter = /^Key([A-Z])$/.exec(code)?.[1]
  if (letter !== undefined) return letter.toLowerCase()
  return PLACES[code] ?? typed
}
const PLACES: Record<string, string> = {
  ...Object.fromEntries([0, 1, 2, 3, 4, 5, 6, 7, 8, 9].map((digit) => [`Digit${digit}`, `${digit}`])),
  BracketLeft: "[",
  BracketRight: "]",
}

// How a shortcut is shown, beside a palette row or in a button's title: by
// its first key, as a Mac writes it, with ⌥⇧⌘ before the key, or as other
// systems do, joined with a plus, as T3 Code shows them.
export function label(command: Command, mac: boolean): string {
  const parts = SHORTCUTS.find((shortcut) => shortcut.command === command)!.keys[0]!.split("+")
  const key = parts.at(-1)!
  const name = key === "escape" ? "Esc" : key.toUpperCase()
  const has = (modifier: string) => parts.includes(modifier)
  if (mac) return `${has("alt") ? "⌥" : ""}${has("shift") ? "⇧" : ""}${has("mod") ? "⌘" : ""}${name}`
  return [...(has("mod") ? ["Ctrl"] : []), ...(has("alt") ? ["Alt"] : []), ...(has("shift") ? ["Shift"] : []), name].join("+")
}

// Whether this browser is on a Mac, where mod is Cmd.
export const onMac = () => typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.userAgent)
