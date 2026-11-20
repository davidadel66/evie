import { homedir } from "node:os"
import { basename, dirname, isAbsolute, join, relative, resolve } from "node:path"
import type { CheckedToolCall } from "../../core/types.ts"
import { patchFiles } from "../files/paths.ts"
import { followLinks } from "../guard.ts"
import { isOutwardFacing, isReadOnly, leavesForHome, partsOf, placesIn, plainWords, startOf } from "./shell.ts"

// The rules of the permissions plugin: what a tool call is about to do, and
// whether that is allowed, denied, or needs the owner's say-so. This file
// only decides. Asking and remembering are in index.ts.

export type Effect = "allow" | "deny" | "ask"

// The three ways of working. A mode is what happens when no rule matches.
//   read-only  nothing is changed, and nothing is asked
//   manual     reading is free; every change and every command is asked about
//   auto       free inside the project; asks before reaching outside it
export const MODES = ["read-only", "manual", "auto"] as const
export type Mode = (typeof MODES)[number]

// One rule. `action` is what kind of thing is being done: "read" (read, grep,
// glob), "edit" (write, edit, patch), "shell", "web", or the name of any
// other tool. `match` is a pattern for what it is done to, and without it
// the rule covers every such action.
//
// For read and edit the pattern is a path. `*` stands for anything within
// one folder and `**` for anything at all. A pattern with no slash, such as
// `.env` or `*.pem`, is a file name and matches it in any folder. Any other
// pattern that is not a full path is taken from the project folder.
//
// For shell the pattern is the command, and `*` stands for anything. A
// pattern ending in a space and `*`, such as `git push *`, covers the
// command by itself and with anything after it, but not `git pushx`.
export type Rule = { action: string; match?: string; effect: Effect }

// One thing a tool call does, in the terms the rules use. A call can do
// several: a patch changes several files, and a command line can hold
// several commands.
export type Act = {
  action: string
  target: string
  // Whether it stays inside the project.
  inside?: boolean
  kind: "path" | "command" | "other"
  // For a path: whether it is a folder being searched, not one file.
  folder?: boolean
  // For a command: the files it names, in full.
  names?: string[]
}

const READ_TOOLS = ["read", "grep", "glob"]
const EDIT_TOOLS = ["write", "edit", "patch"]
const WEB_TOOLS = ["web_search", "web_fetch"]
// The actions the plugin understands. Anything else is a tool it knows nothing about.
const KNOWN = ["read", "edit", "shell", "web", "ask"]

const expandHome = (path: string) => path.replace(/^~(?=\/|$)/, homedir()).replace(/^(\$HOME\b|\$\{HOME\})/, homedir())

const fullPath = (path: string, folder: string | null) => followLinks(isAbsolute(path) ? resolve(path) : resolve(folder ?? homedir(), path))

export function isInside(folder: string | null, absolute: string): boolean {
  if (folder === null) return false
  const within = relative(folder, absolute)
  return within === "" || (!within.startsWith("..") && !isAbsolute(within))
}

// Where a word of a command points, in full: "nowhere" for the places that
// are not files, and "unknown" when it cannot be told, which is treated as
// outside the project.
function whereIs(place: string, runsIn: string | null): string {
  if (["/dev/null", "/dev/stdout", "/dev/stderr", "/dev/stdin"].includes(place)) return "nowhere"
  // Another user's home folder.
  if (/^~[^/]/.test(place)) return "unknown"
  const expanded = expandHome(place)
  // A path built from a variable.
  if (expanded.includes("$")) return expanded.includes("/") ? "unknown" : "nowhere"
  return fullPath(expanded, runsIn)
}

// What a tool call does. The file tools take a path as it is written, with
// no `~`, so the same is done here.
export function actsOf(call: CheckedToolCall, folder: string | null): Act[] {
  const path = (action: string, given: string, searched = false): Act => {
    const target = fullPath(given, folder)
    return { action, target, inside: isInside(folder, target), kind: "path", ...(searched && { folder: true }) }
  }
  const text = (value: unknown) => (typeof value === "string" ? value : "")

  if (READ_TOOLS.includes(call.name)) return [path("read", text(call.input.path) || ".", call.name !== "read")]
  if (call.name === "patch") {
    const files = patchFiles(text(call.input.patchText))
    // A patch in which no file can be found is not let through as harmless.
    return files.length > 0 ? files.map((file) => path("edit", file)) : [{ action: "patch", target: "", kind: "other" }]
  }
  if (EDIT_TOOLS.includes(call.name)) return [path("edit", text(call.input.path) || ".")]
  if (call.name === "shell") {
    const workdir = text(call.input.workdir)
    const runsIn = workdir === "" ? folder : fullPath(expandHome(workdir), folder)
    const parts = partsOf(text(call.input.command))
    const judged = parts.map((part) => {
      const places = placesIn(part).map((place) => whereIs(place, runsIn))
      return { part, names: places.filter((place) => place !== "nowhere" && place !== "unknown"), outside: places.some((place) => place === "unknown" || (place !== "nowhere" && !isInside(folder, place))) }
    })
    // A command reaches outside the project when it runs in a folder
    // outside it or names a place outside it. Once one command in the line
    // has changed folder to somewhere outside, so have the ones after it.
    const left = !isInside(folder, runsIn ?? homedir()) || judged.some(({ part, outside }) => leavesForHome(part) || (outside && /^(cd|pushd)$/.test(plainWords(part).words[0] ?? "")))
    return judged.map(({ part, names, outside }) => ({ action: "shell", target: part, inside: !left && !outside, kind: "command", names }))
  }
  if (WEB_TOOLS.includes(call.name)) return [{ action: "web", target: text(call.input.url) || text(call.input.query), kind: "other" }]
  return [{ action: call.name, target: "", kind: "other" }]
}

// ---- Matching -------------------------------------------------------------------------

const plainOf = (part: string) => plainWords(part).words.join(" ")
const escaped = (text: string) => text.replace(/[.+?^${}()|[\]\\]/g, "\\$&")
// File names differ only by case on a Mac, so there a rule must not either.
const anyCase = process.platform === "darwin" ? "i" : ""

function pathMatches(pattern: string, target: string, folder: string | null): boolean {
  const glob = (text: string) => text.split("**").map((piece) => piece.split("*").map(escaped).join("[^/]*")).join("[\\s\\S]*")
  // A bare file name matches that name in any folder.
  if (!pattern.includes("/")) return new RegExp(`^${glob(pattern)}$`, anyCase).test(basename(target))
  const expanded = expandHome(pattern)
  const written = isAbsolute(expanded) ? expanded : resolve(folder ?? homedir(), expanded)
  // A file is compared by where it really is, so the pattern is too: the
  // part of it before any `*` is followed through links.
  const star = written.indexOf("*")
  const cut = star === -1 ? written.length : written.lastIndexOf("/", star)
  const followed = cut <= 0 ? written : followLinks(written.slice(0, cut)) + written.slice(cut)
  return [written, followed].some(
    (full) =>
      new RegExp(`^${glob(full)}$`, anyCase).test(target) ||
      // `folder/**` covers the folder itself as well as what is in it.
      (full.endsWith("/**") && new RegExp(`^${glob(full.slice(0, -3))}$`, anyCase).test(target)),
  )
}

function commandMatches(pattern: string, command: string): boolean {
  const tidy = pattern.trim()
  const open = tidy.endsWith(" *")
  const body = (open ? tidy.slice(0, -2) : tidy).split("*").map(escaped).join("[\\s\\S]*")
  return new RegExp(`^${body}${open ? "(\\s[\\s\\S]*)?" : ""}$`).test(command.trim())
}

function matches(rule: Rule, act: Act, folder: string | null): boolean {
  if (rule.action !== "*" && rule.action !== act.action) return false
  if (rule.match === undefined) return true
  if (act.kind === "path") return pathMatches(rule.match, act.target, folder)
  return commandMatches(rule.match, act.target)
}

// What happens to an act that no rule covers.
function byMode(act: Act, mode: Mode): Effect {
  if (act.action === "web" || act.action === "ask") return "allow"
  if (act.action === "read") return act.inside ? "allow" : "ask"
  if (act.action === "edit") return mode === "auto" && act.inside ? "allow" : "ask"
  if (act.action === "shell") {
    if (isReadOnly(act.target)) return act.inside ? "allow" : "ask"
    if (mode === "manual") return "ask"
    return act.inside && !isOutwardFacing(act.target) ? "allow" : "ask"
  }
  // A tool the plugin knows nothing about.
  return mode === "auto" ? "allow" : "ask"
}

// The effect for one act. A rule that denies always wins. Otherwise the last
// rule that matches decides, and with no match the mode does.
//
// In read-only mode nothing is asked and no rule can allow a change: what
// would change something, or would need asking, is denied.
export function effectFor(act: Act, rules: readonly Rule[], mode: Mode, folder: string | null): Effect {
  const matching = rules.filter((rule) => matches(rule, act, folder))
  if (matching.some((rule) => rule.effect === "deny")) return "deny"

  if (act.kind === "command") {
    const denies = rules.filter((rule) => rule.effect === "deny")
    // A rule that denies a command is held against its plain form too, so
    // `git -C . push` does not get past a rule for `git push`.
    const plain = plainOf(act.target)
    if (denies.some((rule) => rule.action === "shell" && rule.match !== undefined && commandMatches(rule.match, plain))) return "deny"
    // A rule that denies reading or changing a file also stops a command
    // that names that file, so `cat .env` does not get past a rule for `.env`.
    const fileRules = denies.filter((rule) => (rule.action === "read" || rule.action === "edit") && rule.match !== undefined)
    if (fileRules.some((rule) => (act.names ?? []).some((file) => pathMatches(rule.match!, file, folder)))) return "deny"
  }

  const decides = matching.at(-1)
  let effect = decides?.effect ?? byMode(act, mode)
  // A rule that allows a kind of command was not written, or answered, with
  // reaching outside the project in mind. Only a rule for exactly this
  // command allows that.
  if (act.kind === "command" && effect === "allow" && decides && act.inside === false && decides.match?.trim() !== act.target) effect = "ask"
  if (mode !== "read-only") return effect
  const changes = act.action === "edit" || (act.action === "shell" && !isReadOnly(act.target)) || !KNOWN.includes(act.action)
  return changes || effect === "ask" ? "deny" : effect
}

// ---- Wording ----------------------------------------------------------------------------

// The rule that would allow acts like this one from now on, and how to say
// it to the owner. The rule is never wider than the words.
export function alwaysRule(act: Act, folder: string | null): { rule: Rule; label: string } {
  if (act.kind === "path") {
    const doing = act.action === "read" ? "reading" : "changing"
    if (act.inside && folder !== null) return { rule: { action: act.action, match: `${folder}/**`, effect: "allow" }, label: `${doing} files inside the project` }
    if (act.folder) return { rule: { action: act.action, match: `${act.target === "/" ? "" : act.target}/**`, effect: "allow" }, label: `${doing} anything under ${act.target}` }
    const parent = dirname(act.target)
    return { rule: { action: act.action, match: join(parent, "*"), effect: "allow" }, label: `${doing} files directly in ${parent}` }
  }
  if (act.kind === "command") {
    const start = startOf(act.target)
    if (start === undefined) return { rule: { action: "shell", match: act.target, effect: "allow" }, label: `exactly the command "${act.target}"` }
    return { rule: { action: "shell", match: `${start} *`, effect: "allow" }, label: `commands that start with "${start}"` }
  }
  return { rule: { action: act.action, effect: "allow" }, label: act.action === "web" ? "using the web" : `the tool ${act.action}` }
}

const shown = (path: string, folder: string | null) => (folder !== null && isInside(folder, path) ? relative(folder, path) || "." : path)

// The question put to the owner about the acts that need their say-so.
export function questionFor(call: CheckedToolCall, asking: readonly Act[], folder: string | null): string {
  if (call.name === "shell") {
    const where = typeof call.input.workdir === "string" && call.input.workdir !== "" ? `\n(in ${call.input.workdir})` : ""
    return `Run this command?\n\n  ${String(call.input.command).trim().split("\n").join("\n  ")}${where}`
  }
  if (asking[0]?.kind === "path") {
    const doing = asking[0].action === "read" ? "read" : "change"
    const outside = asking.some((act) => !act.inside) ? " outside the project" : ""
    const files = [...new Set(asking.map((act) => shown(act.target, folder)))]
    return files.length === 1 ? `Let Evie ${doing} ${files[0]}${outside && ", which is"}${outside}?` : `Let Evie ${doing} these files${outside}?\n\n  ${files.join("\n  ")}`
  }
  const input = JSON.stringify(call.input)
  return `Let Evie use the tool ${call.name}?\n\n  ${input.length > 300 ? `${input.slice(0, 300)}…` : input}`
}
