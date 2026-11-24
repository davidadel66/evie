import { statSync } from "node:fs"
import { homedir, tmpdir } from "node:os"
import { basename, dirname, isAbsolute, join, relative, resolve } from "node:path"
import type { CheckedToolCall } from "../../core/types.ts"
import { patchFiles } from "../files/paths.ts"
import { followLinks } from "../guard.ts"
import { cdTarget, isReadOnly, leavesForHome, partsOf, placesIn, plainWords, startOf } from "./shell.ts"

// The rules of the permissions plugin: what a tool call is about to do, and
// whether that is allowed, denied, or needs the owner's say-so. This file
// only decides. Asking and remembering are in index.ts.

export type Effect = "allow" | "deny" | "ask"

// The three ways of working. A mode is what happens when no rule matches.
//   read-only  nothing is changed, and nothing is asked
//   manual     reading is free; every change and every command is asked about
//   auto       everything is free, as with OpenCode's default agent, except
//              reading a .env file and working in a folder outside the
//              project and the temp folder, which ask
export const MODES = ["read-only", "manual", "auto"] as const
export type Mode = (typeof MODES)[number]

// One rule. `action` is what kind of thing is being done: "read" (read, grep,
// glob, and show, for each file it shows), "edit" (write, edit, patch),
// "shell", "web", "outside" (working in a folder outside the project and
// the temp folder, by any tool), or the name of any other tool. `match` is a
// pattern for what it is done to, and without it the rule covers every such
// action.
//
// For read, edit and outside the pattern is a path. `*` stands for anything within
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
  // For a read: whether it is a file a command names, which is the command's
  // to decide, outside auto mode.
  byCommand?: boolean
  // For working outside: it is in the home folder or the root itself, which
  // is asked about each time, with no always answer, since one would cover
  // nearly everything.
  noAlways?: boolean
}

const READ_TOOLS = ["read", "grep", "glob"]
const EDIT_TOOLS = ["write", "edit", "patch"]
// Reading YouTube's captions is using the web too: what it keeps, it keeps in
// Storage, as show does.
const WEB_TOOLS = ["web_search", "web_fetch", "youtube"]
// The actions the plugin understands. Anything else is a tool it knows nothing about.
const KNOWN = ["read", "edit", "shell", "web", "outside", "ask", "job_stop", "subagent", "subagent_send", "skill"]

const expandHome = (path: string) => path.replace(/^~(?=\/|$)/, homedir()).replace(/^(\$HOME\b|\$\{HOME\})/, homedir())

const fullPath = (path: string, folder: string | null) => followLinks(isAbsolute(path) ? resolve(path) : resolve(folder ?? homedir(), path))

export function isInside(folder: string | null, absolute: string): boolean {
  if (folder === null) return false
  const within = relative(folder, absolute)
  return within === "" || (!within.startsWith("..") && !isAbsolute(within))
}

// The system's temp folder, by each name it goes by, with links followed.
const TEMP = [...new Set(["/tmp", "/private/tmp", tmpdir(), process.env.TMPDIR ?? ""].filter((folder) => folder !== "").flatMap((folder) => [resolve(folder), followLinks(resolve(folder))]))]
const isTemp = (absolute: string) => TEMP.some((folder) => isInside(folder, absolute))

// A file that may hold secrets, which auto mode asks before reading: `.env`
// and `.env.*`, or any name ending so, but not `.env.example`. These are
// OpenCode's patterns for the same.
export const isEnvFile = (path: string) => /\.env(\.[^/]*)?$/i.test(basename(path)) && !/\.env\.example$/i.test(basename(path))

const isFolder = (absolute: string) => {
  try {
    return statSync(absolute).isDirectory()
  } catch {
    return false
  }
}

const HOME = [...new Set([homedir(), followLinks(homedir())])]
// The home folder and the root, which are never the folder an always answer
// is about: one for either would be an answer for nearly everything.
const isBroad = (absolute: string) => absolute === "/" || HOME.includes(absolute)

// Working outside the project and the temp folder, at `target`, which auto
// mode asks about once for each folder. A file, or a place that is not
// there, directly in the home folder or the root is asked about by itself,
// as if it were a folder. The home folder and the root themselves are asked
// about each time.
//
// A session with no project has the home folder as its own: it is judged as
// working there, so only what is outside the home folder asks, and the home
// folder itself only when it is named, not when a command just runs in it
// (`ranIn`).
function outside(target: string, folder: string | null, searched: boolean, ranIn = false): Act[] {
  if (isInside(folder, target) || isTemp(target)) return []
  const isDirectory = searched || isFolder(target)
  if (isDirectory && isBroad(target)) {
    if (folder === null && ranIn && HOME.includes(target)) return []
    return [{ action: "outside", target, inside: false, kind: "path", folder: true, noAlways: true }]
  }
  if (folder === null && HOME.some((home) => isInside(home, target))) return []
  return [{ action: "outside", target, inside: false, kind: "path", ...((isDirectory || isBroad(dirname(target))) && { folder: true }) }]
}

// Where a word of a command points, in full: "nowhere" for the places that
// are not files, and "unknown" when it cannot be told, as for a place built
// from a variable.
function whereIs(place: string, runsIn: string | null): string {
  if (["/dev/null", "/dev/stdout", "/dev/stderr", "/dev/stdin", "/dev/tty"].includes(place)) return "nowhere"
  // Another user's home folder.
  if (/^~[^/]/.test(place)) return "unknown"
  const expanded = expandHome(place).replace(/^(\$TMPDIR\b|\$\{TMPDIR\})/, (found) => process.env.TMPDIR || found)
  // A path built from a variable.
  if (/[$`]/.test(expanded)) return expanded.includes("/") ? "unknown" : "nowhere"
  return fullPath(expanded, runsIn)
}

// What a command line reaches, as OpenCode's shell scan finds it: the
// folder it runs in, each folder a `cd` moves to, and each place a command
// names, with relative places taken from each folder the line may be in by
// then, since a `cd` inside brackets does not last. Each becomes an outside
// act when it is outside the project and the temp folder, and a read when it
// is a .env file. A place that cannot be told, such as one built from a
// variable, is left out, as OpenCode leaves it out: its value is not known
// until the command runs.
function reachedBy(parts: string[], runsIn: string, folder: string | null): Act[] {
  const heres = [runsIn]
  const acts = outside(runsIn, folder, true, true)
  for (const part of parts) {
    const to = cdTarget(part)
    if (to !== undefined) {
      for (const from of [...heres]) {
        const there = to === "~" ? homedir() : to === "-" ? "unknown" : whereIs(to, from)
        if (there === "unknown" || there === "nowhere") continue
        heres.push(there)
        acts.push(...outside(there, folder, true, true))
      }
      continue
    }
    for (const place of placesIn(part, false)) {
      for (const here of heres) {
        const at = whereIs(place, here)
        if (at === "unknown" || at === "nowhere") continue
        acts.push(...outside(at, folder, false))
        if (isEnvFile(at)) acts.push({ action: "read", target: at, inside: isInside(folder, at), kind: "path", byCommand: true })
      }
    }
  }
  return acts
}

// What a tool call does. The file tools take a path as it is written, with
// no `~`, so the same is done here.
export function actsOf(call: CheckedToolCall, folder: string | null): Act[] {
  const path = (action: string, given: string, searched = false): Act[] => {
    const target = fullPath(given, folder)
    return [{ action, target, inside: isInside(folder, target), kind: "path", ...(searched && { folder: true }) }, ...outside(target, folder, searched)]
  }
  const text = (value: unknown) => (typeof value === "string" ? value : "")
  const once = (acts: Act[]) => acts.filter((act, at) => acts.findIndex((other) => other.action === act.action && other.target === act.target && other.kind === act.kind) === at)

  if (READ_TOOLS.includes(call.name)) return path("read", text(call.input.path) || ".", call.name !== "read")
  // Showing a file reads it, and keeps a copy in Storage for the owner: it
  // is held to the rules for reading each file it is given.
  if (call.name === "show") {
    const given = Array.isArray(call.input.paths) ? call.input.paths.map(text).filter((each) => each !== "") : []
    return once((given.length > 0 ? given : ["."]).flatMap((each) => path("read", each)).map((act) => (act.action === "read" ? { ...act, shown: true } : act)))
  }
  if (call.name === "patch") {
    const files = patchFiles(text(call.input.patchText))
    // A patch in which no file can be found is not let through as harmless.
    return files.length > 0 ? once(files.flatMap((file) => path("edit", file))) : [{ action: "patch", target: "", kind: "other" }]
  }
  if (EDIT_TOOLS.includes(call.name)) return path("edit", text(call.input.path) || ".")
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
    // This is what manual and read-only mode go by.
    const left = !isInside(folder, runsIn ?? homedir()) || judged.some(({ part, outside }) => leavesForHome(part) || (outside && /^(cd|pushd)$/.test(plainWords(part).words[0] ?? "")))
    const commands = judged.map(({ part, names, outside }): Act => ({ action: "shell", target: part, inside: !left && !outside, kind: "command", names }))
    return once([...commands, ...reachedBy(parts, runsIn ?? homedir(), folder)])
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
  // Working outside the project is asked about here only in auto mode. In
  // the others, reading, changing and running commands there ask by themselves.
  if (act.action === "outside") return mode === "auto" ? "ask" : "allow"
  if (act.action === "read") {
    if (mode === "auto") return isEnvFile(act.target) ? "ask" : "allow"
    // A file shown from outside the project is copied in, so it asks as reading one does.
    return act.inside || act.byCommand ? "allow" : "ask"
  }
  if (act.action === "edit") return mode === "auto" ? "allow" : "ask"
  if (act.action === "shell") return mode === "auto" || (act.inside && isReadOnly(act.target)) ? "allow" : "ask"
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

// The folder an outside act is about: the place itself when it is a folder,
// or else the folder it is in.
const folderOf = (act: Act) => (act.folder ? act.target : dirname(act.target))

// The rule that would allow acts like this one from now on, and how to say
// it to the owner. The rule is never wider than the words.
export function alwaysRule(act: Act, folder: string | null, mode: Mode): { rule: Rule; label: string } {
  if (act.action === "outside") {
    const at = folderOf(act)
    return { rule: { action: "outside", match: `${at === "/" ? "" : at}/**`, effect: "allow" }, label: `working in ${at}` }
  }
  // In auto mode a read asks only about a file that may hold secrets, so the
  // answer is about that file alone.
  if (mode === "auto" && act.action === "read" && isEnvFile(act.target)) {
    return { rule: { action: "read", match: act.target, effect: "allow" }, label: `reading ${shown(act.target, folder)}` }
  }
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

// What auto mode asks about, as the owner is told it.
export const AUTO_ASKS = "reading a .env file or working in a folder outside the project and the temp folder"

const shown = (path: string, folder: string | null) => (folder !== null && isInside(folder, path) ? relative(folder, path) || "." : path)

// The question put to the owner about the acts that need their say-so.
export function questionFor(call: CheckedToolCall, asking: readonly Act[], folder: string | null): string {
  if (call.name === "shell") {
    const where = typeof call.input.workdir === "string" && call.input.workdir !== "" ? `\n(in ${call.input.workdir})` : ""
    // Why auto mode asks, when it is not the command itself.
    const folders = [...new Set(asking.filter((act) => act.action === "outside").map(folderOf))]
    const secrets = [...new Set(asking.filter((act) => act.action === "read").map((act) => shown(act.target, folder)))]
    const why = [
      ...(folders.length > 0 ? [folder === null ? `It works in ${folders.join(", ")}.` : `It works outside the project, in ${folders.join(", ")}.`] : []),
      ...(secrets.length > 0 ? [`It reads ${secrets.join(", ")}, which may hold secrets.`] : []),
    ]
    return `Run this command?\n\n  ${String(call.input.command).trim().split("\n").join("\n  ")}${where}${why.length > 0 ? `\n\n${why.join(" ")}` : ""}`
  }
  if (call.name === "show") {
    const files = [...new Set(asking.map((act) => shown(act.target, folder)))]
    const outside = asking.some((act) => !act.inside) ? ", which is outside the project" : ""
    return files.length === 1
      ? `Let evie show you ${files[0]}${outside}? A copy is kept in Storage.`
      : `Let evie show you these files${outside && " from outside the project"}? A copy of each is kept in Storage.\n\n  ${files.join("\n  ")}`
  }
  if (asking[0]?.kind === "path") {
    const doing = READ_TOOLS.includes(call.name) ? "read" : "change"
    const outside = asking.some((act) => !act.inside) ? " outside the project" : ""
    const files = [...new Set(asking.map((act) => shown(act.target, folder)))]
    return files.length === 1 ? `Let evie ${doing} ${files[0]}${outside && ", which is"}${outside}?` : `Let evie ${doing} these files${outside}?\n\n  ${files.join("\n  ")}`
  }
  if (call.name === "subagent") {
    const task = String(call.input.task ?? "").trim()
    return (
      `Let evie start a subagent (${String(call.input.description ?? "a task")}${typeof call.input.agent === "string" && call.input.agent.trim() !== "" ? `, as the agent "${call.input.agent.trim()}"` : ""})? It works without asking before each ` +
      `step, as in auto mode: it may change files and run any command, and asks only before ${AUTO_ASKS}.\n\n  ${task.length > 400 ? `${task.slice(0, 400)}…` : task}`
    )
  }
  if (call.name === "subagent_send") {
    const message = String(call.input.message ?? "").trim()
    return (
      "Let evie give its subagent more work? It works without asking before each step, as in auto mode: it may change " +
      `files and run any command, and asks only before ${AUTO_ASKS}.\n\n  ${message.length > 400 ? `${message.slice(0, 400)}…` : message}`
    )
  }
  const input = JSON.stringify(call.input)
  return `Let evie use the tool ${call.name}?\n\n  ${input.length > 300 ? `${input.slice(0, 300)}…` : input}`
}
