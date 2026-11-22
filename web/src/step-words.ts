// A tool call in plain words, apart from any screen: one sentence saying
// what it did, "Read src/auth.ts" or "Ran bun test", or while it runs what
// it is doing, "Running bun test", as T3 Code's work log says it (MIT
// licence, notice in LICENSES/t3code-MIT.txt), for evie's own tools; and
// the kind of thing it did, which an icon shows.

// What a call did, which its icon shows.
export type StepKind = "read" | "edit" | "command" | "search" | "web" | "agent" | "question" | "other"

// A call's sentence. `input` is the call's input, as the JSON text the
// model wrote, which may not be JSON, or as read. With `running`, it says
// what the call is doing. A path inside the project's `folder` is given
// from there, as the tools take it. It is on one line, however the input
// was written.
export function stepSentence(name: string, input: unknown, how: { running?: boolean; folder?: string } = {}): string {
  const fields = fieldsOf(input)
  const text = (field: string) => {
    const value = fields[field]
    return typeof value === "string" && value.trim() !== "" ? oneLine(value) : undefined
  }
  const path = () => {
    const given = text("path")
    return given === undefined ? undefined : fromFolder(given, how.folder)
  }
  const said = (done: string, doing: string, what: string | undefined, none: string) => `${how.running ? doing : done} ${what ?? none}`.trim()
  const where = path() === undefined ? "" : ` in ${path()}`
  switch (name) {
    case "read":
      return said("Read", "Reading", path(), "a file")
    case "edit":
      return said("Edited", "Editing", path(), "a file")
    case "write":
      return said("Wrote", "Writing", path(), "a file")
    case "patch": {
      const files = patchFiles(typeof fields.patchText === "string" ? fields.patchText : "")
      return said("Edited", "Editing", files.length === 0 ? undefined : files.length === 1 ? files[0] : `${files[0]} +${files.length - 1} more`, "files")
    }
    case "shell":
      return said("Ran", "Running", text("command"), "a command")
    case "grep":
      return said("Searched", "Searching", text("pattern") === undefined ? undefined : `${text("pattern")}${where}`, "the files")
    case "glob":
      return said("Searched files", "Searching files", text("pattern") === undefined ? undefined : `${text("pattern")}${where}`, "by name")
    case "web_search":
      return said("Searched the web for", "Searching the web for", text("query"), "something")
    case "web_fetch":
      return said("Fetched", "Fetching", text("url"), "a page")
    case "subagent": {
      const task = text("description")
      return said("Asked a subagent", "Asking a subagent", task === undefined ? undefined : `to ${task}`, "to help")
    }
    case "subagent_send":
      return how.running ? "Messaging a subagent" : "Messaged a subagent"
    case "ask":
      return how.running ? "Asking a question" : "Asked a question"
    case "skill":
      return said("Read the skill", "Reading the skill", text("name"), "")
    case "job_stop":
      return said("Stopped job", "Stopping job", text("id"), "")
    default:
      return how.running ? `Using ${name}` : `Used ${name}`
  }
}

// The kind of thing a call of a tool does, by the tool's name.
export function stepKind(name: string): StepKind {
  switch (name) {
    case "read":
    case "skill":
      return "read"
    case "edit":
    case "write":
    case "patch":
      return "edit"
    case "shell":
      return "command"
    case "grep":
    case "glob":
      return "search"
    case "web_search":
    case "web_fetch":
      return "web"
    case "subagent":
    case "subagent_send":
      return "agent"
    case "ask":
      return "question"
    default:
      return "other"
  }
}

// The input's fields, or none when it is not an object, or JSON text of one.
export function fieldsOf(input: unknown): Record<string, unknown> {
  let read = input
  if (typeof input === "string") {
    try {
      read = JSON.parse(input)
    } catch {
      return {}
    }
  }
  return typeof read === "object" && read !== null && !Array.isArray(read) ? (read as Record<string, unknown>) : {}
}

const oneLine = (text: string) => text.replace(/\s+/g, " ").trim()

// A path from the project's folder when it is inside it, as the tools
// name files back to the model; otherwise as given.
function fromFolder(path: string, folder: string | undefined): string {
  if (folder === undefined || !path.startsWith("/")) return path
  const top = folder.replace(/\/+$/, "")
  return path.startsWith(`${top}/`) ? path.slice(top.length + 1) : path
}

// The files a patch names, as the patch tool reads them.
const patchFiles = (patchText: string) => [...patchText.matchAll(/^[ \t]*\*\*\* (?:Add File|Update File|Delete File):[ \t]*(.+?)[ \t]*$/gm)].map((found) => found[1]!)
