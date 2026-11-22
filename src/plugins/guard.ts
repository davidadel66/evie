import { homedir } from "node:os"
import { existsSync, lstatSync, realpathSync } from "node:fs"
import { basename, dirname, isAbsolute, join, relative, resolve } from "node:path"
import { dataFolder } from "../core/evie.ts"
import type { CheckedToolCall, Plugin } from "../core/types.ts"

// The built-in guard. It stops the most clearly destructive commands before
// they run, and keeps the tools out of a few private folders.
//
// It is a seat belt, not a lock. It matches the text of a command, so a
// command hidden in a script, or built from a variable, gets past it.
//
// The permissions plugin applies the same checks as rules that always deny,
// so the guard is only needed as a plugin of its own when permissions is
// switched off.

export type GuardOptions = {
  // The folder Evie keeps its state and keys in. Defaults to the usual place.
  dataFolder?: string
  // Let the file tools write the owner's agents and skills in the data
  // folder, as well as read them. Only for a caller that asks the owner
  // before each such write, as the permissions plugin does. By itself the
  // guard lets them be read and nothing more.
  definitionsCanBeWritten?: boolean
}

// Makes the check: given a tool call and the project's folder, it returns
// the reason the call is blocked, worded for the model, or undefined.
export function createGuard(options: GuardOptions = {}): (call: CheckedToolCall, projectFolder: string | null, session?: string) => string | undefined {
  const home = homedir()
  const privateFolders = [
    options.dataFolder ?? dataFolder({}),
    join(home, ".ssh"),
    join(home, ".aws"),
    join(home, ".gnupg"),
  ].flatMap((folder) => [...new Set([resolve(folder), followLinks(resolve(folder))])])

  const data = options.dataFolder ?? dataFolder({})

  return (call, projectFolder, session) => {
    const folder = projectFolder ?? home
    // The one part of the data folder the tools may read: the session's own
    // saved output, which the model is told to read from there.
    if (session !== undefined && ["read", "grep", "glob"].includes(call.name) && typeof call.input.path === "string" && isAbsolute(call.input.path)) {
      if (isSavedOutput(data, session, call.input.path)) return undefined
    }
    // The owner's agents and skills are theirs to have Evie read with the
    // file tools, and to write when something asks them first.
    if (READ_TOOLS.includes(call.name) || (options.definitionsCanBeWritten && FILE_TOOLS.includes(call.name))) {
      const named = filesNamed(call)
      if (named.length > 0 && named.every((path) => isAbsolute(path) && isDefinition(data, path))) return undefined
    }
    // A skill can hold scripts, so a command may name a file in the owner's
    // skills, where something asks the owner before it runs.
    const skills = options.definitionsCanBeWritten ? data : undefined
    const reason = call.name === "shell" ? shellReason(call, privateFolders, folder, skills) : fileReason(call, privateFolders, folder)
    if (reason === undefined) return undefined
    return `Blocked by the guard: ${reason} The command was not run. If the owner wants this done, they can run it themselves.`
  }
}

export function guard(options: GuardOptions = {}): Plugin {
  const blocked = createGuard(options)
  return {
    name: "guard",
    setup(host) {
      host.on("before_tool", (call, context) => {
        const deny = blocked(call, context.project?.folder ?? null, context.session)
        if (deny !== undefined) return { deny }
      })
    },
  }
}

// ---- Shell commands -----------------------------------------------------------------

// Where a command word can begin: at the start, on a new line, after a
// symbol that joins commands, or after a word that runs what follows it.
const STARTS = String.raw`(?:^|[;&|(\n]|\b(?:then|do|else|xargs|exec|nohup|time)\b)\s*`

// Each rule: what it looks for in a command, and why that is blocked.
const RULES: { matches: RegExp; reason: string }[] = [
  { matches: /\b(curl|wget)\b[^|\n]*\|\s*(sudo\s+)?(ba|z|da|fi)?sh\b/, reason: "this runs code from the internet that nobody has read." },
  { matches: /\bgit\b[^;&|]*\breset\b[^;&|]*--hard\b/, reason: "this discards changes that have not been saved in git." },
  { matches: /\bgit\s+clean\b[^;&|]*\s(-[a-zA-Z]*f|--force\b)/, reason: "this discards files that git is not tracking." },
  { matches: /\bgit\s+checkout\b[^;&|]*\s\.(\s|$)/, reason: "this discards every change in the folder." },
  // Taking everything back out of the staging area loses nothing, so that is let through.
  { matches: /\bgit\s+restore\b(?![^;&|]*--staged\b)[^;&|]*\s\.(\s|$)/, reason: "this discards every change in the folder." },
  { matches: /\bgit\s+push\b[^;&|]*\s(--force(?!-with-lease)\b|-[a-zA-Z]*f\b)/, reason: "a forced push rewrites history that others may have." },
  { matches: /\bmkfs(\.\w+)?\b/, reason: "this erases a disk." },
  { matches: /\bdd\b[^;&|]*\bof=\/dev\//, reason: "this writes straight onto a disk." },
  { matches: /\bdiskutil\s+(erase|partition|zero|secureerase|reformat)/i, reason: "this erases a disk." },
  { matches: new RegExp(`${STARTS}(shutdown|reboot|halt|poweroff)\\b`), reason: "this stops the machine." },
  { matches: new RegExp(`${STARTS}sudo\\b`), reason: "this runs as administrator, which nothing Evie does should need." },
  { matches: /\bch(mod|own)\s+(-\S*R\S*|--recursive)\b[^;&|]*\s(\/|~\/?|\$HOME\/?|\$\{HOME\}\/?)(\s|$)/, reason: "this changes permissions across the whole machine or home folder." },
  { matches: /:\s*\(\s*\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;\s*:/, reason: "this is a fork bomb, which freezes the machine." },
]

function shellReason(call: CheckedToolCall, privateFolders: string[], projectFolder: string, skillsIn?: string): string | undefined {
  const command = typeof call.input.command === "string" ? call.input.command : ""
  const home = homedir()

  for (const rule of RULES) if (rule.matches.test(command)) return rule.reason
  if (deletesBroadly(command, home)) return "this deletes far more than a folder inside the project."

  // A private folder, written the ways a shell command would write it.
  const spelled = command.replaceAll("${HOME}", home).replaceAll("$HOME", home).replace(/(^|[\s"'=:(])~(?=\/|\s|$)/g, `$1${home}`)
  // Words that name a file in the owner's skills are set aside before the
  // private folders are looked for, so that a skill's script can be run.
  // Each must be a plain full path that really is inside the skills folder.
  // This is for running and reading only: a command that writes, copies,
  // links, deletes or changes folder gets no such allowance, since from
  // inside the skills folder it could reach the rest of the data folder, or
  // change a skill without the question that writing one always brings.
  const onlyRunsOrReads = !/[<>]|(^|[\s;&|(])(cd|pushd|ln|cp|mv|rm|tee|dd|install|touch|mkdir|rmdir|chmod|chown|sed|perl|awk|truncate|rsync|tar|unzip)(\s|$)/.test(command)
  const checked =
    skillsIn === undefined || !onlyRunsOrReads
      ? spelled
      : spelled
          .split(/(\s+)/)
          .map((word) => {
            const path = word.replace(/^["']|["']$/g, "")
            return /^\/[\w./@+-]+$/.test(path) && isSkill(skillsIn, path) ? "" : word
          })
          .join("")
  for (const folder of privateFolders) {
    if (checked.includes(folder)) return `${folder} is a private folder.`
  }
  if (typeof call.input.workdir === "string" && insidePrivate(call.input.workdir, privateFolders, projectFolder)) {
    return `${call.input.workdir} is a private folder.`
  }
  return undefined
}

// True for a recursive rm aimed at something broad: the root, the home
// folder, a folder near the top of the disk, the parent of where it runs, or
// everything where it runs. Deleting a named folder inside the project is fine.
function deletesBroadly(command: string, home: string): boolean {
  for (const part of command.split(/&&|\|\||[;|&\n]/)) {
    const words = part.trim().split(/\s+/)
    if (words[0] !== "rm") continue
    const flags = words.filter((word) => word.startsWith("-"))
    const recursive = flags.some((flag) => flag === "--recursive" || (!flag.startsWith("--") && /[rR]/.test(flag)))
    if (!recursive) continue

    for (const raw of words.slice(1).filter((word) => !word.startsWith("-"))) {
      const target = raw.replace(/^["']|["']$/g, "").replaceAll("${HOME}", home).replaceAll("$HOME", home).replace(/^~(?=\/|$)/, home)
      const tidy = target.replace(/\/+$/, "") || "/"
      if (["/", "/*", ".", "*", "./*", ".git", "./.git"].includes(tidy)) return true
      if (tidy === ".." || tidy.startsWith("../") || tidy.includes("/../") || tidy.endsWith("/..")) return true
      if (tidy === home || tidy === `${home}/*`) return true
      // A folder one or two levels below the root, such as /usr or /usr/local.
      if (tidy.startsWith("/") && tidy.split("/").filter(Boolean).length <= 2) return true
    }
  }
  return false
}

// ---- The file tools -------------------------------------------------------------------

const READ_TOOLS = ["read", "grep", "glob"]
const FILE_TOOLS = [...READ_TOOLS, "write", "edit", "patch"]

// The files a call to one of the file tools names.
function filesNamed(call: CheckedToolCall): string[] {
  const paths: string[] = []
  if (typeof call.input.path === "string") paths.push(call.input.path)
  // A patch names its files inside its text.
  if (typeof call.input.patchText === "string") {
    // The patch tool reads each line with the space around it taken off.
    for (const match of call.input.patchText.matchAll(/^[ \t]*\*\*\* (?:Add File|Update File|Delete File|Move to):[ \t]*(.+?)[ \t]*$/gm)) {
      paths.push(match[1]!)
    }
  }
  return paths
}

function fileReason(call: CheckedToolCall, privateFolders: string[], projectFolder: string): string | undefined {
  const paths = filesNamed(call)
  const found = paths.find((path) => insidePrivate(path, privateFolders, projectFolder))
  return found === undefined ? undefined : `${found} is in a private folder.`
}

function insidePrivate(path: string, privateFolders: string[], projectFolder: string): boolean {
  const expanded = path.replace(/^~(?=\/|$)/, homedir())
  const written = isAbsolute(expanded) ? resolve(expanded) : resolve(projectFolder, expanded)
  // Checked as written and as it really is, so a link into a private folder is caught.
  return [written, followLinks(written)].some((absolute) =>
    privateFolders.some((folder) => {
      const inside = relative(folder, absolute)
      return inside === "" || (!inside.startsWith("..") && !isAbsolute(inside))
    }),
  )
}

// Where a path really is, with any link in it followed. A path that does
// not exist yet is followed as far as it does exist.
export function followLinks(absolute: string): string {
  try {
    return realpathSync(absolute)
  } catch {
    const parent = dirname(absolute)
    return parent === absolute ? absolute : join(followLinks(parent), basename(absolute))
  }
}

// True for a path inside a folder where one session's own files are kept
// in the data folder: tool output that was too long to show, and the output
// of its background jobs; and the files the owner attached to its messages
// with no project to keep them in. Links are followed, so nothing outside
// them can be reached through them.
export function isSavedOutput(data: string, session: string, path: string): boolean {
  return ["tool-output", "attachments"].some((kind) => {
    const inside = relative(followLinks(resolve(data, kind, session)), followLinks(resolve(path)))
    return inside === "" || (!inside.startsWith("..") && !isAbsolute(inside))
  })
}

// True for a path inside the owner's agents or skills in the data folder.
// Links are followed, so nothing else in the data folder is reached through them.
export function isDefinition(data: string, path: string): boolean {
  // A link that points at nothing would be written through to wherever it points.
  if (isBrokenLink(resolve(path))) return false
  const real = followLinks(resolve(path))
  return ["agents", "skills"].some((kind) => {
    const inside = relative(followLinks(resolve(data, kind)), real)
    return inside === "" || (!inside.startsWith("..") && !isAbsolute(inside))
  })
}

function isBrokenLink(absolute: string): boolean {
  try {
    return lstatSync(absolute).isSymbolicLink() && !existsSync(absolute)
  } catch {
    return false
  }
}

// True for a path inside the owner's skills in the data folder.
function isSkill(data: string, path: string): boolean {
  // A ".." is folded away as text before links are followed, so a path
  // with one can end up somewhere other than it looks.
  if (/(^|\/)\.\.(\/|$)/.test(path)) return false
  if (isBrokenLink(resolve(path))) return false
  const inside = relative(followLinks(resolve(data, "skills")), followLinks(resolve(path)))
  return inside !== "" && !inside.startsWith("..") && !isAbsolute(inside)
}
