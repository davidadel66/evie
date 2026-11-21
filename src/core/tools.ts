import { lstat, mkdir, readdir, rmdir, unlink, writeFile } from "node:fs/promises"
import { join } from "node:path"
import type { CheckedToolCall, Tool, ToolCall, ToolContext } from "./types.ts"

// What every tool call goes through, whichever tool it is for: find the
// tool, check and repair the input, ask the plugins, run it, let the plugins
// see the result, and cap the output. The result
// is always text for the model. A problem the model can fix is returned as
// that text; only a bug in a tool throws.

// Thrown when the turn was cancelled before a tool got to run.
export class CancelledBeforeStart extends Error {}

export const MAX_LINES = 2000
export const MAX_BYTES = 50 * 1024
// A tool that pages its own output, such as read, ends with a short note
// saying how to get the next part. This leaves room for it.
const NOTE_LINES = 2

export type ToolRunOptions = {
  // The tools the model was offered in this step.
  tools: readonly Tool[]
  context: ToolContext
  // The before_tool hooks: the reason one denied the call, or undefined.
  before(call: CheckedToolCall): Promise<string | undefined>
  // The after_tool hooks: the result text as they left it.
  after(call: CheckedToolCall, text: string): Promise<string>
  // Where output too long to show is saved in full.
  outputFolder: string
}

export async function runToolCall(call: ToolCall, options: ToolRunOptions): Promise<string> {
  const tool = options.tools.find((candidate) => candidate.name === call.name)
  if (!tool) {
    const names = options.tools.map((candidate) => candidate.name).join(", ") || "none"
    return `There is no tool named "${call.name}". The tools are: ${names}.`
  }

  const parsed = parseInput(call.arguments)
  if (typeof parsed === "string") return `The input for ${tool.name} ${parsed} Send the call again with the input as one JSON object.`

  const problems = repairAndCheck(parsed, tool.inputSchema)
  if (problems.length > 0) {
    return `The input for ${tool.name} is not right: ${problems.join("; ")}. Send the call again with the input fixed.`
  }

  const checked: CheckedToolCall = { id: call.id, name: tool.name, input: parsed }
  const denied = await options.before(checked)
  if (denied !== undefined) return denied
  // The turn may have been cancelled while the hooks ran.
  if (options.context.signal.aborted) throw new CancelledBeforeStart()

  let output: string
  try {
    output = await tool.run(parsed, options.context)
  } catch (cause) {
    throw new Error(`The tool "${tool.name}" broke: ${cause instanceof Error ? cause.message : String(cause)}`)
  }
  output = await options.after(checked, output)
  return capOutput(output, join(options.outputFolder, `${safeName(call.id)}.txt`), tool.longOutput ?? "start")
}

// ---- The input ------------------------------------------------------------------

function parseInput(text: string): Record<string, unknown> | string {
  if (text.trim() === "") return {}
  let value: unknown
  try {
    value = JSON.parse(text)
  } catch {
    return "was not valid JSON."
  }
  if (typeof value !== "object" || value === null || Array.isArray(value)) return "was not a JSON object."
  return value as Record<string, unknown>
}

type Schema = { properties?: Record<string, { type?: string }>; required?: string[] }

// Fixes input that is wrong in an obvious way, such as a number sent as
// text, so a small slip does not cost the model a step. Then lists what is
// still wrong. Only the plain parts of a JSON schema are understood: the
// required names and each property's type.
function repairAndCheck(input: Record<string, unknown>, inputSchema: Record<string, unknown>): string[] {
  const schema = inputSchema as Schema
  const problems: string[] = []
  for (const name of schema.required ?? []) {
    if (input[name] === undefined || input[name] === null) problems.push(`"${name}" is required`)
  }
  for (const [name, property] of Object.entries(schema.properties ?? {})) {
    const value = input[name]
    if (value === undefined || value === null) {
      delete input[name]
      continue
    }
    switch (property.type) {
      case "number":
      case "integer": {
        const number = typeof value === "string" && value.trim() !== "" ? Number(value) : value
        if (typeof number !== "number" || !Number.isFinite(number)) problems.push(`"${name}" must be a number`)
        else if (property.type === "integer" && !Number.isInteger(number)) problems.push(`"${name}" must be a whole number`)
        else input[name] = number
        break
      }
      case "boolean": {
        const boolean = value === "true" ? true : value === "false" ? false : value
        if (typeof boolean !== "boolean") problems.push(`"${name}" must be true or false`)
        else input[name] = boolean
        break
      }
      case "string": {
        if (typeof value === "number" || typeof value === "boolean") input[name] = String(value)
        else if (typeof value !== "string") problems.push(`"${name}" must be text`)
        break
      }
    }
  }
  return problems
}

// ---- The output -----------------------------------------------------------------

// Output over the limit is saved in full and the model is shown the start,
// or the end for a tool that asks for it, with a note saying how much there
// is and where the rest is. Nothing is silently cut off.
async function capOutput(output: string, savePath: string, keep: "start" | "end"): Promise<string> {
  const lines = output.split("\n")
  const bytes = Buffer.byteLength(output)
  if (lines.length <= MAX_LINES + NOTE_LINES && bytes <= MAX_BYTES) return output

  let shown = (keep === "end" ? lines.slice(-MAX_LINES) : lines.slice(0, MAX_LINES)).join("\n")
  // Set when even the one line nearest the kept end is too long to show whole.
  let partLine = false
  if (Buffer.byteLength(shown) > MAX_BYTES && keep === "end") {
    // Cut to the last bytes that fit, starting after a character the cut
    // split in two: the bytes that carry on a character come first.
    const buffer = Buffer.from(shown)
    let from = buffer.length - MAX_BYTES
    while ((buffer[from]! & 0xc0) === 0x80) from++
    shown = buffer.subarray(from).toString("utf8")
    // Then on to the start of the first whole line, when there is one.
    const firstBreak = shown.indexOf("\n")
    if (firstBreak >= 0 && firstBreak < shown.length - 1) shown = shown.slice(firstBreak + 1)
    else partLine = true
  } else if (Buffer.byteLength(shown) > MAX_BYTES) {
    // Cut to the byte limit, dropping a character the cut split in two.
    shown = Buffer.from(shown).subarray(0, MAX_BYTES).toString("utf8").replace(/\uFFFD+$/, "")
    // Then back to the end of the last whole line, when there is one.
    const lastBreak = shown.lastIndexOf("\n")
    if (lastBreak > 0) shown = shown.slice(0, lastBreak)
    else partLine = true
  }
  const end = keep === "end" ? "last" : "first"
  const amount = partLine
    ? `showing the ${end} ${Buffer.byteLength(shown)} of ${bytes} bytes`
    : `showing the ${end} ${shown.split("\n").length} of ${lines.length} lines`

  let where: string
  try {
    await mkdir(join(savePath, ".."), { recursive: true })
    await writeFile(savePath, output)
    where = `The full output is saved at ${savePath}. Read that file with offset and limit to see more.`
  } catch (cause) {
    where = `The full output could not be saved (${cause instanceof Error ? cause.message : String(cause)}).`
  }
  return `${shown}\n\n[Output cut: ${amount}. ${where}]`
}

const safeName = (id: string) => id.replace(/[^A-Za-z0-9_-]/g, "_") || "call"

// Saved output is kept for a week, long enough to come back to, and then
// deleted so that the folder does not grow for ever.
const KEEP_OUTPUT_FOR = 7 * 24 * 60 * 60 * 1000

// Deletes the files in a folder of saved output last changed more than a
// week ago, then every folder inside it left empty. The folder itself is
// kept. Anything that cannot be read or deleted is left as it is: tidying
// up must never stop Evie from starting.
export async function deleteOldOutput(folder: string): Promise<void> {
  const oldest = Date.now() - KEEP_OUTPUT_FOR
  const tidy = async (inside: string) => {
    let entries
    try {
      entries = await readdir(inside, { withFileTypes: true })
    } catch {
      return
    }
    for (const entry of entries) {
      const path = join(inside, entry.name)
      try {
        if (entry.isDirectory()) {
          await tidy(path)
          // Refused, as it should be, while anything is left inside.
          await rmdir(path)
        } else if ((await lstat(path)).mtimeMs < oldest) {
          await unlink(path)
        }
      } catch {
        // Left as it is.
      }
    }
  }
  await tidy(folder)
}
