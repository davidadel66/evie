import { homedir, tmpdir } from "node:os"
import { basename, dirname, isAbsolute, join, relative, resolve } from "node:path"
import type { CheckedToolCall } from "../../core/types.ts"
import { patchFiles } from "../files/paths.ts"
import { followLinks } from "../guard.ts"
import { changesSystem, isOutwardFacing, isReadOnly, leavesForHome, overwrittenBy, partsOf, placesIn, plainWords, startOf } from "./shell.ts"

// The rules of the permissions plugin: what a tool call is about to do, and
// whether that is allowed, denied, or needs the owner's say-so. This file
// only decides. Asking and remembering are in index.ts.

export type Effect = "allow" | "deny" | "ask"

// The three ways of working. A mode is what happens when no rule matches.
//   read-only  nothing is changed, and nothing is asked
//   manual     reading is free; every change and every command is asked about
//   auto       reading is free anywhere, except showing a file from outside
//              the project, which copies it in; and changing files inside the
//              project and the temp folder; any command runs, except the
//              dangerous ones, which are asked about: those that send or
//              publish, delete or overwrite files elsewhere, or change the
//              system
export const MODES = ["read-only", "manual", "auto"] as const
export type Mode = (typeof MODES)[number]

// One rule. `action` is what kind of thing is being done: "read" (read, grep,
// glob, and show, for each file it shows), "edit" (write, edit, patch),
// "shell", "web", or the name of any other tool. `match` is a pattern for what it is done to, and without it
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
  // For a path: whether it is being shown, which keeps a copy of it in the
  // project's .evie folder, whose files are backed up.
  shown?: boolean
  // For a command: the files it names, in full.
  names?: string[]
  // For a command: whether it deletes, moves or overwrites files outside
  // the project and the temp folder, or may.
  writesOutside?: boolean
}

const READ_TOOLS = ["read", "grep", "glob"]
const EDIT_TOOLS = ["write", "edit", "patch"]
const WEB_TOOLS = ["web_search", "web_fetch"]
// The actions the plugin understands. Anything else is a tool it knows nothing about.
const KNOWN = ["read", "edit", "shell", "web", "ask", "job_stop", "subagent", "subagent_send", "skill"]

const expandHome = (path: string) => path.replace(/^~(?=\/|$)/, homedir()).replace(/^(\$HOME\b|\$\{HOME\})/, homedir())

const fullPath = (path: string, folder: string | null) => followLinks(isAbsolute(path) ? resolve(path) : resolve(folder ?? homedir(), path))

export function isInside(folder: string | null, absolute: string): boolean {
  if (folder === null) return false
  const within = relative(folder, absolute)
  return within === "" || (!within.startsWith("..") && !isAbsolute(within))
}

// The system's temp folder, by each name it goes by, with links followed.
const TEMP = [...new Set(["/tmp", "/private/tmp", tmpdir(), process.env.TMPDIR ?? ""].filter((folder) => folder !== "").flatMap((folder) => [resolve(folder), followLinks(resolve(folder))]))]
// `itself` says whether the folder itself counts, and not only what is in it.
const isTemp = (absolute: string, itself = true) => TEMP.some((folder) => isInside(folder, absolute) && (itself || absolute !== folder))

// Where a file that a command deletes, moves or overwrites is, in full,
// with the command run in `runsIn`, which is null when that cannot be told:
// "nowhere" for the places that are not files, and "unknown" when it cannot
// be told, which is treated as outside.
function whereWritten(place: string, runsIn: string | null): string {
  if (["/dev/null", "/dev/stdout", "/dev/stderr"].includes(place)) return "nowhere"
  if (/^~[^/]/.test(place)) return "unknown"
  const expanded = expandHome(place).replace(/^(\$TMPDIR\b|\$\{TMPDIR\})/, (found) => process.env.TMPDIR || found)
  if (/[$`]/.test(expanded) || (runsIn === null && !isAbsolute(expanded))) return "unknown"
  return fullPath(expanded, runsIn)
}

// Whether a command line deletes, moves or overwrites files outside the
// project and the temp folder, judged one command at a time. A command
// after a `cd` is judged as if run both where the line started and where
// it went, since a `cd` inside brackets does not last.
function writesOutsideIn(parts: string[], runsIn: string, folder: string | null): boolean[] {
  const places: (string | null)[] = [runsIn]
  return parts.map((part) => {
    const { words } = plainWords(part)
    if (words[0] === "cd" || words[0] === "pushd") {
      const to = words.slice(1).find((word) => word === "-" || !word.startsWith("-"))
      for (const from of [...places]) {
        const there = to === undefined || to === "~" ? homedir() : to === "-" ? "unknown" : whereWritten(to, from)
        places.push(there === "unknown" || there === "nowhere" ? null : there)
      }
      return false
    }
    const { files, folders } = overwrittenBy(part)
    const safe = (place: string, itself: boolean) =>
      places.every((from) => {
        const at = whereWritten(place, from)
        return at === "nowhere" || (at !== "unknown" && ((isInside(folder, at) && (itself || at !== folder)) || isTemp(at, itself)))
      })
    return !files.every((file) => safe(file, false)) || !folders.every((place) => safe(place, true))
  })
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
  // Showing a file reads it, and keeps a copy in Storage for the owner: it
  // is held to the rules for reading each file it is given.
  if (call.name === "show") {
    const given = Array.isArray(call.input.paths) ? call.input.paths.map(text).filter((each) => each !== "") : []
    return (given.length > 0 ? given.map((each) => path("read", each)) : [path("read", ".")]).map((act) => ({ ...act, shown: true }))
  }
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
    const writes = writesOutsideIn(parts, runsIn ?? homedir(), folder)
    return judged.map(({ part, names, outside }, at) => ({ action: "shell", target: part, inside: !left && !outside, kind: "command", names, writesOutside: writes[at]! }))
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
  // Using the web, asking the owner and stopping a job change nothing of the owner's.
  // Loading a skill only puts its instructions in front of the model.
  if (["web", "ask", "job_stop", "skill"].includes(act.action)) return "allow"
  // A subagent works without asking before each step, so in manual mode
  // starting one is asked about: that answer is the owner's say-so for its
  // work. In read-only mode a subagent is read-only too, so starting one
  // changes nothing. Giving a subagent more work is the same as starting one.
  if (act.action === "subagent" || act.action === "subagent_send") return mode === "manual" ? "ask" : "allow"
  // A file shown from outside the project is copied in, so it asks in every mode.
  if (act.action === "read") return act.inside || (mode === "auto" && !act.shown) ? "allow" : "ask"
  if (act.action === "edit") return mode === "auto" && (act.inside || isTemp(act.target, false)) ? "allow" : "ask"
  if (act.action === "shell") {
    // The dangerous ones: what leaves the computer, what deletes or
    // overwrites files elsewhere, and what changes the system.
    if (mode === "auto") return isOutwardFacing(act.target) || act.writesOutside || changesSystem(act.target) ? "ask" : "allow"
    return act.inside && isReadOnly(act.target) ? "allow" : "ask"
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
  // command allows that; for any other, the mode decides.
  if (act.kind === "command" && effect === "allow" && decides && act.inside === false && decides.match?.trim() !== act.target) effect = byMode(act, mode)
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
    // Outside the project only a rule for exactly the command allows it.
    const start = act.inside === false ? undefined : startOf(act.target)
    if (start === undefined) return { rule: { action: "shell", match: act.target, effect: "allow" }, label: `exactly the command "${act.target}"` }
    return { rule: { action: "shell", match: `${start} *`, effect: "allow" }, label: `commands that start with "${start}"` }
  }
  const label = act.action === "web" ? "using the web" : act.action === "subagent" ? "starting subagents" : act.action === "subagent_send" ? "giving subagents more work" : `the tool ${act.action}`
  return { rule: { action: act.action, effect: "allow" }, label }
}

const shown = (path: string, folder: string | null) => (folder !== null && isInside(folder, path) ? relative(folder, path) || "." : path)

// The question put to the owner about the acts that need their say-so.
export function questionFor(call: CheckedToolCall, asking: readonly Act[], folder: string | null): string {
  if (call.name === "shell") {
    const where = typeof call.input.workdir === "string" && call.input.workdir !== "" ? `\n(in ${call.input.workdir})` : ""
    return `Run this command?\n\n  ${String(call.input.command).trim().split("\n").join("\n  ")}${where}`
  }
  if (call.name === "show") {
    const files = [...new Set(asking.map((act) => shown(act.target, folder)))]
    const outside = asking.some((act) => !act.inside) ? ", which is outside the project" : ""
    return files.length === 1
      ? `Let evie show you ${files[0]}${outside}? A copy is kept in Storage.`
      : `Let evie show you these files${outside && " from outside the project"}? A copy of each is kept in Storage.\n\n  ${files.join("\n  ")}`
  }
  if (asking[0]?.kind === "path") {
    const doing = asking[0].action === "read" ? "read" : "change"
    const outside = asking.some((act) => !act.inside) ? " outside the project" : ""
    const files = [...new Set(asking.map((act) => shown(act.target, folder)))]
    return files.length === 1 ? `Let evie ${doing} ${files[0]}${outside && ", which is"}${outside}?` : `Let evie ${doing} these files${outside}?\n\n  ${files.join("\n  ")}`
  }
  if (call.name === "subagent") {
    const task = String(call.input.task ?? "").trim()
    return (
      `Let evie start a subagent (${String(call.input.description ?? "a task")}${typeof call.input.agent === "string" && call.input.agent.trim() !== "" ? `, as the agent "${call.input.agent.trim()}"` : ""})? It works without asking before each ` +
      `step, as in auto mode: it may change files in the project and run commands, and asks only before what is dangerous.\n\n  ${task.length > 400 ? `${task.slice(0, 400)}…` : task}`
    )
  }
  if (call.name === "subagent_send") {
    const message = String(call.input.message ?? "").trim()
    return (
      "Let evie give its subagent more work? It works without asking before each step, as in auto mode: it may change " +
      `files in the project and run commands, and asks only before what is dangerous.\n\n  ${message.length > 400 ? `${message.slice(0, 400)}…` : message}`
    )
  }
  const input = JSON.stringify(call.input)
  return `Let evie use the tool ${call.name}?\n\n  ${input.length > 300 ? `${input.slice(0, 300)}…` : input}`
}
