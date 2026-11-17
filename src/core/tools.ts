import { mkdir, writeFile } from "node:fs/promises"
import { join } from "node:path"
import type { Tool, ToolCall, ToolContext } from "./types.ts"

// What every tool call goes through, whichever tool it is for: find the
// tool, check and repair the input, run it, and cap the output. The result
// is always text for the model. A problem the model can fix is returned as
// that text; only a bug in a tool throws.

export const MAX_LINES = 2000
export const MAX_BYTES = 50 * 1024
// A tool that pages its own output, such as read, ends with a short note
// saying how to get the next part. This leaves room for it.
const NOTE_LINES = 2

export type ToolRunOptions = {
  tools: readonly Tool[]
  context: ToolContext
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

  let output: string
  try {
    output = await tool.run(parsed, options.context)
  } catch (cause) {
    throw new Error(`The tool "${tool.name}" broke: ${cause instanceof Error ? cause.message : String(cause)}`)
  }
  return capOutput(output, join(options.outputFolder, `${safeName(call.id)}.txt`))
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
// with a note saying how much there is and where the rest is. Nothing is
// silently cut off.
async function capOutput(output: string, savePath: string): Promise<string> {
  const lines = output.split("\n")
  const bytes = Buffer.byteLength(output)
  if (lines.length <= MAX_LINES + NOTE_LINES && bytes <= MAX_BYTES) return output

  let shown = lines.slice(0, MAX_LINES).join("\n")
  if (Buffer.byteLength(shown) > MAX_BYTES) {
    // Cut to the byte limit, then back to the end of the last whole line.
    shown = Buffer.from(shown).subarray(0, MAX_BYTES).toString("utf8")
    const lastBreak = shown.lastIndexOf("\n")
    if (lastBreak > 0) shown = shown.slice(0, lastBreak)
  }
  const shownLines = shown.split("\n").length

  let where: string
  try {
    await mkdir(join(savePath, ".."), { recursive: true })
    await writeFile(savePath, output)
    where = `The full output is saved at ${savePath}. Read that file with offset and limit to see more.`
  } catch (cause) {
    where = `The full output could not be saved (${cause instanceof Error ? cause.message : String(cause)}).`
  }
  return `${shown}\n\n[Output cut: showing the first ${shownLines} of ${lines.length} lines. ${where}]`
}

const safeName = (id: string) => id.replace(/[^A-Za-z0-9_-]/g, "_") || "call"
