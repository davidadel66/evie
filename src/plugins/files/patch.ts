// The patch format, its parsing and the way lines are found in a file are
// adapted from OpenAI Codex, codex-rs/apply-patch (parser.rs, seek_sequence.rs
// and file_update.rs), rewritten from Rust in TypeScript.
//
// Copyright 2025 OpenAI. Licensed under the Apache License, Version 2.0. A
// copy of the licence is in LICENSES/codex-Apache-2.0.txt, and Codex's notice
// in LICENSES/codex-NOTICE.txt.

import { mkdir, readFile, rm, writeFile } from "node:fs/promises"
import { dirname } from "node:path"
import type { Tool, ToolContext } from "../../core/types.ts"
import { displayPath, resolvePath, statOrMissing } from "./paths.ts"

export const patch: Tool = {
  name: "patch",
  description: [
    "Change files with a patch. One patch can add, change, delete and move several files. The whole patch is",
    "checked against the files first, and nothing is written unless every part fits.",
    "",
    "The patch is plain text in this form:",
    "",
    "*** Begin Patch",
    "*** Add File: path/to/new.txt",
    "+every line of the new file starts with a plus",
    "*** Update File: path/to/existing.txt",
    "@@ an optional line that says where to look, such as a function's first line",
    " a line that stays the same starts with a space",
    "-a line to remove starts with a minus",
    "+a line to add starts with a plus",
    "*** Delete File: path/to/old.txt",
    "*** End Patch",
    "",
    "There are no line numbers: lines are found by what they say, so give a few unchanged lines around each",
    'change. Start each separate change in a file with a line beginning "@@". To rename a file, put',
    '"*** Move to: new/path" straight after its "*** Update File: " line. A relative path is taken from the',
    "project folder.",
  ].join("\n"),
  inputSchema: {
    type: "object",
    properties: { patchText: { type: "string", description: "The whole patch, from *** Begin Patch to *** End Patch" } },
    required: ["patchText"],
  },

  async run(input, context) {
    const parsed = parsePatch(input.patchText as string)
    if ("failure" in parsed) return `${parsed.failure} Nothing was changed.`

    const planned = await plan(parsed.hunks, context)
    if ("failure" in planned) return planned.failure

    // Everything fits. Only now is anything written.
    const done: string[] = []
    for (const [absolute, content] of planned.files) {
      try {
        if (content === null) await rm(absolute, { force: true })
        else {
          await mkdir(dirname(absolute), { recursive: true })
          await writeFile(absolute, content)
        }
        done.push(displayPath(absolute, context))
      } catch (cause) {
        const reason = cause instanceof Error ? cause.message : String(cause)
        const before = done.length > 0 ? done.join(", ") : "nothing"
        return `The patch could not be written in full: ${reason}. Completed before the failure: ${before}.`
      }
    }
    return `Success. Updated the following files:\n${planned.summary.join("\n")}`
  },
}

// ---- Reading the patch --------------------------------------------------------------

// One change inside a file: the lines to find and what replaces them.
// `steps` is the change as written, a line at a time: " " for a line that
// stays, "-" for one removed and "+" for one added.
type Chunk = {
  context?: string
  oldLines: string[]
  newLines: string[]
  steps: { kind: " " | "-" | "+"; text: string }[]
  endOfFile: boolean
}

type Hunk =
  | { kind: "add"; path: string; content: string }
  | { kind: "delete"; path: string }
  | { kind: "update"; path: string; moveTo?: string; chunks: Chunk[] }

const BEGIN = "*** Begin Patch"
const END = "*** End Patch"
const ADD = "*** Add File: "
const DELETE = "*** Delete File: "
const UPDATE = "*** Update File: "
const MOVE = "*** Move to: "
const END_OF_FILE = "*** End of File"

function parsePatch(text: string): { hunks: Hunk[] } | { failure: string } {
  let lines = text.trim().split(/\r?\n/)
  // A model sometimes wraps the patch the way a shell would be handed it.
  if (/^<<\s*['"]?EOF['"]?$/.test(lines[0] ?? "") && lines.length >= 4 && lines.at(-1)!.trim().endsWith("EOF")) {
    lines = lines.slice(1, -1)
  }
  if (lines[0]?.trim() !== BEGIN) return { failure: `The patch is not valid: the first line must be "${BEGIN}".` }
  if (lines.at(-1)?.trim() !== END) return { failure: `The patch is not valid: the last line must be "${END}".` }

  const invalid = (index: number, message: string) => ({ failure: `The patch is not valid at line ${index + 1}: ${message}.` })
  const isFileMarker = (line: string) => line.startsWith(ADD) || line.startsWith(DELETE) || line.startsWith(UPDATE)

  const hunks: Hunk[] = []
  const last = lines.length - 1
  let at = 1
  while (at < last) {
    const line = lines[at]!.trim()

    if (line.startsWith(ADD)) {
      const added: string[] = []
      at += 1
      while (at < last && lines[at]!.startsWith("+")) added.push(lines[at++]!.slice(1))
      hunks.push({ kind: "add", path: line.slice(ADD.length), content: added.length > 0 ? `${added.join("\n")}\n` : "" })
      continue
    }

    if (line.startsWith(DELETE)) {
      hunks.push({ kind: "delete", path: line.slice(DELETE.length) })
      at += 1
      continue
    }

    if (line.startsWith(UPDATE)) {
      const hunk: Hunk = { kind: "update", path: line.slice(UPDATE.length), chunks: [] }
      const startedAt = at
      at += 1
      if (at < last && lines[at]!.trim().startsWith(MOVE)) hunk.moveTo = lines[at++]!.trim().slice(MOVE.length)

      let chunk: Chunk | undefined
      const open = (context?: string) => {
        chunk = { oldLines: [], newLines: [], steps: [], endOfFile: false, ...(context === undefined ? {} : { context }) }
        hunk.chunks.push(chunk)
        return chunk
      }
      // Inside a file's changes a line counts as the start of the next file
      // only when it begins at the margin: indented, it is a line of the file.
      while (at < last && !isFileMarker(lines[at]!.trimEnd())) {
        const raw = lines[at]!
        if (raw.trim() === "@@") open()
        else if (raw.startsWith("@@ ")) open(raw.slice(3))
        else if (raw.trim() === END_OF_FILE) {
          if (!chunk) return invalid(at, `"${END_OF_FILE}" has no change before it`)
          chunk.endOfFile = true
          chunk = undefined
        } else {
          // The first change in a file may leave out its "@@" line.
          const current = chunk ?? (hunk.chunks.length === 0 ? open() : undefined)
          if (!current) return invalid(at, `a new change in a file must start with a line beginning "@@", but this one is "${raw}"`)
          const text = raw.slice(1)
          if (raw === "" || raw.startsWith(" ")) {
            current.oldLines.push(text)
            current.newLines.push(text)
            current.steps.push({ kind: " ", text })
          } else if (raw.startsWith("+")) {
            current.newLines.push(text)
            current.steps.push({ kind: "+", text })
          } else if (raw.startsWith("-")) {
            current.oldLines.push(text)
            current.steps.push({ kind: "-", text })
          } else return invalid(at, `a line in a change must start with a space, + or -, but this one is "${raw}"`)
        }
        at += 1
      }
      const empty = hunk.chunks.some((part) => part.oldLines.length === 0 && part.newLines.length === 0)
      if (empty || (hunk.chunks.length === 0 && hunk.moveTo === undefined)) {
        return invalid(startedAt, `the change to ${hunk.path} has nothing in it`)
      }
      hunks.push(hunk)
      continue
    }

    return invalid(at, `"${line}" is not something a patch can do. Use "${ADD}", "${DELETE}" or "${UPDATE}"`)
  }

  if (hunks.length === 0) return { failure: "The patch is not valid: it changes nothing." }
  return { hunks }
}

// ---- Checking the patch against the files ---------------------------------------------

const NOTHING_CHANGED = "Nothing was changed."
const TRY_AGAIN = `${NOTHING_CHANGED} Read the file, then send the patch again with lines that match it.`

// Works out what every file would hold after the patch, without writing
// anything. A later part of the patch sees what an earlier part did.
async function plan(
  hunks: Hunk[],
  context: ToolContext,
): Promise<{ files: Map<string, string | null>; summary: string[] } | { failure: string }> {
  // What each touched file will hold; null means it will be deleted.
  const files = new Map<string, string | null>()
  const summary = new Set<string>()

  const current = async (absolute: string): Promise<string | undefined> => {
    if (files.has(absolute)) return files.get(absolute) ?? undefined
    const info = await statOrMissing(absolute)
    return info ? readFile(absolute, "utf8") : undefined
  }

  for (const hunk of hunks) {
    const target = resolvePath(hunk.path, context)
    if ("failure" in target) return target
    const shown = displayPath(target.absolute, context)
    const existing = await current(target.absolute)

    if (hunk.kind === "add") {
      if (existing !== undefined) return { failure: `${shown} already exists. ${NOTHING_CHANGED} To change it, use *** Update File.` }
      files.set(target.absolute, hunk.content)
      summary.add(`A ${shown}`)
      continue
    }

    if (existing === undefined) {
      const hint = hunk.kind === "update" ? " To create it, use *** Add File." : ""
      return { failure: `File not found: ${shown}. ${NOTHING_CHANGED}${hint}` }
    }

    if (hunk.kind === "delete") {
      files.set(target.absolute, null)
      summary.add(`D ${shown}`)
      continue
    }

    const updated = applyChunks(existing, hunk.chunks, shown)
    if (typeof updated !== "string") return updated
    if (hunk.moveTo === undefined) {
      files.set(target.absolute, updated)
      summary.add(`M ${shown}`)
    } else {
      const destination = resolvePath(hunk.moveTo, context)
      if ("failure" in destination) return destination
      if (destination.absolute !== target.absolute && (await current(destination.absolute)) !== undefined) {
        const taken = displayPath(destination.absolute, context)
        return { failure: `${shown} cannot be moved to ${taken}, because that file already exists. ${NOTHING_CHANGED}` }
      }
      files.set(target.absolute, null)
      files.set(destination.absolute, updated)
      summary.add(`M ${displayPath(destination.absolute, context)} (moved from ${shown})`)
    }
  }
  return { files, summary: [...summary] }
}

// Applies one file's changes to its text. Each change is found after the one
// before it, so the changes must come in the order they appear in the file.
function applyChunks(content: string, chunks: Chunk[], shown: string): string | { failure: string } {
  const ending = content.split("\r\n").length - 1 > content.split("\n").length / 2 ? "\r\n" : "\n"
  const lines = content.split(/\r?\n/)
  if (lines.at(-1) === "") lines.pop()

  // Where each change goes: a position, how many lines it replaces, and the new lines.
  const replacements: { at: number; remove: number; lines: string[] }[] = []
  let from = 0

  for (const chunk of chunks) {
    if (chunk.context !== undefined) {
      const found = seek(lines, [chunk.context], from, false)
      if (found === undefined) return { failure: `Failed to find the line "${chunk.context}" in ${shown}.\n\n${TRY_AGAIN}` }
      from = found + 1
    }

    // Lines added with nothing to find go straight after the line that says
    // where to look, or at the end of the file when there is none.
    if (chunk.oldLines.length === 0) {
      replacements.push({ at: chunk.context === undefined ? lines.length : from, remove: 0, lines: chunk.newLines })
      continue
    }

    let pattern = chunk.oldLines
    let steps = chunk.steps
    let found = seek(lines, pattern, from, chunk.endOfFile)
    // A change that reaches the end of the file often ends with an empty
    // line standing for the final line break, which the file's lines do not have.
    if (found === undefined && pattern.at(-1) === "" && steps.at(-1)?.kind === " ") {
      pattern = pattern.slice(0, -1)
      steps = steps.slice(0, -1)
      found = seek(lines, pattern, from, chunk.endOfFile)
    }
    if (found === undefined) {
      return { failure: `Failed to find expected lines in ${shown}:\n${chunk.oldLines.join("\n")}\n\n${TRY_AGAIN}` }
    }
    // A line that stays is kept exactly as the file has it, so a forgiving
    // match does not quietly rewrite its whitespace.
    const replacement: string[] = []
    let matched = found
    for (const step of steps) {
      if (step.kind === "+") replacement.push(step.text)
      else if (step.kind === " ") replacement.push(lines[matched++]!)
      else matched += 1
    }
    replacements.push({ at: found, remove: pattern.length, lines: replacement })
    from = found + pattern.length
  }

  // Applied from the end, so earlier positions stay where they are. Changes
  // at the same position are applied last-written first, which leaves them
  // in the order they were written.
  const ordered = replacements.map((change, written) => ({ ...change, written }))
  ordered.sort((a, b) => b.at - a.at || b.written - a.written)
  for (const change of ordered) lines.splice(change.at, change.remove, ...change.lines)
  return lines.length > 0 ? `${lines.join(ending)}${ending}` : ""
}

// Typographic variants a model often writes in place of the plain character.
const plain = (line: string) =>
  line
    .trim()
    .replace(/[‐‑‒–—―−]/g, "-")
    .replace(/[‘’‚‛]/g, "'")
    .replace(/[“”„‟]/g, '"')
    .replace(/[  -   　]/g, " ")

// Finds where a run of lines starts in a file, at or after `start`. It tries
// four times, each more forgiving: exactly; ignoring whitespace at the ends
// of lines; ignoring whitespace at both ends; and also ignoring typographic
// variants of quotes, dashes and spaces. `atEnd` looks at the end of the file first.
function seek(lines: string[], pattern: string[], start: number, atEnd: boolean): number | undefined {
  if (pattern.length === 0) return start
  if (pattern.length > lines.length) return undefined
  const lastStart = lines.length - pattern.length
  const first = atEnd ? Math.max(lastStart, start) : start

  const tiers: ((line: string) => string)[] = [(line) => line, (line) => line.trimEnd(), (line) => line.trim(), plain]
  for (const tidy of tiers) {
    for (let at = first; at <= lastStart; at++) {
      if (pattern.every((wanted, offset) => tidy(lines[at + offset]!) === tidy(wanted))) return at
    }
  }
  return undefined
}
