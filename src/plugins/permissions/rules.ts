import { homedir } from "node:os"
import { dirname, isAbsolute, relative, resolve } from "node:path"
import type { CheckedToolCall } from "../../core/types.ts"
import { isOutwardFacing, isReadOnly, partsOf, startOf } from "./shell.ts"

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
// other tool. `match` is a pattern for what it is done to: a path for read
// and edit, the command for shell, the address or search for web. A `*` in
// it stands for anything. Without `match` the rule covers every such action.
export type Rule = { action: string; match?: string; effect: Effect }

// One thing a tool call does, in the terms the rules use. A call can do
// several: a patch changes several files, and a command line can hold
// several commands.
export type Act = {
  action: string
  target: string
  // For a path: whether it is inside the project.
  inside?: boolean
  kind: "path" | "command" | "other"
}

const READ_TOOLS = ["read", "grep", "glob"]
const EDIT_TOOLS = ["write", "edit", "patch"]
const WEB_TOOLS = ["web_search", "web_fetch"]

const expandHome = (path: string) => path.replace(/^~(?=\/|$)/, homedir())
const absolutePath = (path: string, folder: string | null) => {
  const expanded = expandHome(path)
  return isAbsolute(expanded) ? resolve(expanded) : resolve(folder ?? homedir(), expanded)
}
export function isInside(folder: string | null, absolute: string): boolean {
  if (folder === null) return false
  const within = relative(folder, absolute)
  return within === "" || (!within.startsWith("..") && !isAbsolute(within))
}

// What a tool call does.
export function actsOf(call: CheckedToolCall, folder: string | null): Act[] {
  const path = (action: string, given: string): Act => {
    const target = absolutePath(given, folder)
    return { action, target, inside: isInside(folder, target), kind: "path" }
  }
  const text = (value: unknown) => (typeof value === "string" ? value : "")

  if (READ_TOOLS.includes(call.name)) return [path("read", text(call.input.path) || ".")]
  if (call.name === "patch") {
    const files = [...text(call.input.patchText).matchAll(/^\*\*\* (?:Add File|Update File|Delete File|Move to): (.+)$/gm)].map((found) => found[1]!.trim())
    // A patch that names no file changes nothing, and the tool says so itself.
    return files.map((file) => path("edit", file))
  }
  if (EDIT_TOOLS.includes(call.name)) return [path("edit", text(call.input.path) || ".")]
  if (call.name === "shell") {
    // A command run in a folder outside the project reaches outside it.
    const workdir = text(call.input.workdir)
    const inside = workdir === "" ? folder !== null : isInside(folder, absolutePath(workdir, folder))
    return partsOf(text(call.input.command)).map((part) => ({ action: "shell", target: part, inside, kind: "command" }))
  }
  if (WEB_TOOLS.includes(call.name)) return [{ action: "web", target: text(call.input.url) || text(call.input.query), kind: "other" }]
  return [{ action: call.name, target: "", kind: "other" }]
}

// ---- Matching -------------------------------------------------------------------------

function matches(rule: Rule, act: Act, folder: string | null): boolean {
  if (rule.action !== "*" && rule.action !== act.action) return false
  if (rule.match === undefined) return true
  // A path pattern that is not a full path is taken from the project folder.
  const pattern = act.kind === "path" ? absolutePath(rule.match, folder) : rule.match.trim()
  const asRegex = new RegExp(`^${pattern.split("*").map((piece) => piece.replace(/[.+?^${}()|[\]\\]/g, "\\$&")).join("[\\s\\S]*")}$`)
  return asRegex.test(act.target)
}

// What happens to an act that no rule covers.
function byMode(act: Act, mode: Mode): Effect {
  if (act.action === "web" || act.action === "ask") return "allow"
  if (act.action === "read") return act.inside ? "allow" : mode === "read-only" ? "deny" : "ask"
  if (act.action === "edit") return mode === "read-only" ? "deny" : mode === "auto" && act.inside ? "allow" : "ask"
  if (act.action === "shell") {
    if (isReadOnly(act.target)) return act.inside ? "allow" : mode === "read-only" ? "deny" : "ask"
    if (mode === "read-only") return "deny"
    if (mode === "manual") return "ask"
    return act.inside && !isOutwardFacing(act.target) ? "allow" : "ask"
  }
  // A tool the plugin knows nothing about.
  return mode === "read-only" ? "deny" : mode === "auto" ? "allow" : "ask"
}

// The effect for one act. A rule that denies always wins. Otherwise the last
// rule that matches decides, and with no match the mode does. In read-only
// mode a rule cannot allow a change: read-only means nothing is changed.
export function effectFor(act: Act, rules: readonly Rule[], mode: Mode, folder: string | null): Effect {
  const matching = rules.filter((rule) => matches(rule, act, folder))
  if (matching.some((rule) => rule.effect === "deny")) return "deny"
  const changes = act.action === "edit" || (act.action === "shell" && !isReadOnly(act.target))
  if (mode === "read-only" && changes) return "deny"
  return matching.at(-1)?.effect ?? byMode(act, mode)
}

// ---- Wording ----------------------------------------------------------------------------

// The rule that would allow acts like this one from now on, and how to say
// it to the owner.
export function alwaysRule(act: Act, folder: string | null): { rule: Rule; label: string } {
  if (act.kind === "path") {
    const doing = act.action === "read" ? "reading" : "changing"
    if (act.inside && folder !== null) return { rule: { action: act.action, match: `${folder}/*`, effect: "allow" }, label: `${doing} files inside the project` }
    const parent = dirname(act.target)
    return { rule: { action: act.action, match: `${parent}/*`, effect: "allow" }, label: `${doing} files in ${parent}` }
  }
  if (act.kind === "command") {
    const start = startOf(act.target)
    return { rule: { action: "shell", match: `${start}*`, effect: "allow" }, label: `commands that start with "${start}"` }
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
