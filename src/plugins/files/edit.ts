// The matching in this file (the three tiers, the mapping back to the file's
// own line endings, and the hint shown when nothing matches) is adapted from
// OpenCode's edit tool, packages/core/src/tool/plugin/edit.ts.
//
// MIT License
//
// Copyright (c) 2025 opencode
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

import { readFile, writeFile } from "node:fs/promises"
import type { Tool } from "../../core/types.ts"
import { displayPath, resolvePath, statOrMissing } from "./paths.ts"

export const edit: Tool = {
  name: "edit",
  description:
    "Change a file by finding exact text and replacing it. When the text comes from read output, keep its exact " +
    "indentation and leave out the line-number prefix, such as `12: `. Write line breaks as \\n; the file's own " +
    "line endings are kept. If there is no exact match, the edit tries again ignoring typographic variants of " +
    "quotes, dashes and spaces, and then trailing whitespace on each line. oldString must match exactly one place: " +
    "several matches fail unless replaceAll is true. Add more surrounding lines to make it unique, or set " +
    "replaceAll to true to change every occurrence, such as when renaming something.",
  inputSchema: {
    type: "object",
    properties: {
      path: { type: "string", description: "The file to edit" },
      oldString: { type: "string", description: "The exact text to find" },
      newString: { type: "string", description: "The text to put in its place; it must differ from oldString" },
      replaceAll: { type: "boolean", description: "Replace every occurrence of oldString. Defaults to false" },
    },
    required: ["path", "oldString", "newString"],
  },

  async run(input, context) {
    const path = input.path as string
    const oldString = input.oldString as string
    const newString = input.newString as string
    const replaceAll = input.replaceAll === true

    if (oldString === newString) return "No changes to apply: oldString and newString are identical."
    if (oldString === "") return "oldString must not be empty. Use write to create or overwrite a file."

    const target = resolvePath(path, context)
    if ("failure" in target) return target.failure
    const shown = displayPath(target.absolute, context)

    const info = await statOrMissing(target.absolute)
    if (!info) return `File not found: ${path}. Use write to create it.`
    if (info.isDirectory()) return `${shown} is a folder, not a file.`
    const source = await readFile(target.absolute, "utf8")

    const { matches, replace } = matchView(source, oldString, newString)
    if (matches.length === 0) {
      const hint = missHint(source, oldString, newString)
      return `Could not find oldString in ${shown}. It must match exactly, including whitespace and indentation.${hint ? `\n\n${hint}` : ""}`
    }
    if (matches.length > 1 && !replaceAll) {
      return (
        `Found ${matches.length} matches for oldString in ${shown}, but expected exactly one. Add more surrounding ` +
        `context to make oldString unique, or set replaceAll to true to replace every occurrence.`
      )
    }

    // Replace from the end, so earlier positions stay where they are.
    const replaced = matches
      .toReversed()
      .reduce((content, match) => content.slice(0, match.start) + replace(match) + content.slice(match.end), source)
    if (replaced === source) return `No changes to apply: the edit leaves ${shown} unchanged.`

    await writeFile(target.absolute, replaced)
    return `Edited ${shown} (${matches.length} replacement${matches.length === 1 ? "" : "s"}).`
  },
}

// ---- Finding the text -------------------------------------------------------------

type Match = { start: number; end: number }

// Typographic variants a model often writes in place of the plain character.
// Every replacement is one character for one, so positions do not move.
const normalizeForMatch = (value: string) =>
  value
    .replace(/[‘’‚‛]/g, "'")
    .replace(/[“”„‟]/g, '"')
    .replace(/[‐‑‒–—―−]/g, "-")
    .replace(/[  -   　]/g, " ")

function findOccurrences(content: string, search: string): Match[] {
  const result: Match[] = []
  let offset = 0
  while ((offset = content.indexOf(search, offset)) !== -1) {
    result.push({ start: offset, end: offset + search.length })
    offset += search.length
  }
  return result
}

// Matches whole lines, ignoring trailing whitespace on each one.
function findLineOccurrences(content: string, search: string): Match[] {
  const trailingNewline = search.endsWith("\n")
  const expected = search.split("\n")
  if (trailingNewline) expected.pop()

  const lines = [...content.matchAll(/[^\n]*(?:\n|$)/g)]
    .filter((match) => match[0] !== "")
    .map((match) => {
      const newline = match[0].endsWith("\n")
      const text = newline ? match[0].slice(0, -1) : match[0]
      return { start: match.index, end: match.index + match[0].length, text, contentEnd: match.index + text.length, newline }
    })

  const result: Match[] = []
  for (let index = 0; index + expected.length <= lines.length; index++) {
    const actual = lines.slice(index, index + expected.length)
    const same = actual.every(
      (line, offset) => normalizeForMatch(line.text.trimEnd()) === normalizeForMatch(expected[offset]!.trimEnd()),
    )
    if (!same) continue
    const last = actual.at(-1)!
    if (trailingNewline && !last.newline) continue
    const candidate = { start: actual[0]!.start, end: trailingNewline ? last.end : last.contentEnd }
    if (!result.some((match) => match.end > candidate.start && match.start < candidate.end)) result.push(candidate)
  }
  return result
}

// The line ending used most in a piece of text, or the fallback when it has none.
function lineEnding(text: string, fallback: string): string {
  const crlf = text.split("\r\n").length - 1
  const lf = text.split("\n").length - 1 - crlf
  if (crlf === lf) return fallback
  return crlf > lf ? "\r\n" : "\n"
}

// Matching is done on the view of the file that read shows the model, where
// every line ends with \n. The positions are then mapped back to the real
// file, and the replacement is written with the line endings of the part it
// replaces, so a file with \r\n endings keeps them.
function matchView(source: string, oldString: string, newString: string) {
  const view = source.replaceAll("\r\n", "\n")
  // Where each removed \r was, as a position in the view.
  const removed = [...source.matchAll(/\r\n/g)].map((match, index) => match.index - index)
  const toSource = (position: number) => position + removed.filter((at) => at < position).length

  const search = oldString.replaceAll("\r\n", "\n")
  const replacement = newString.replaceAll("\r\n", "\n")

  // Three tiers, each tried only when the one before found nothing.
  let found = findOccurrences(view, search)
  if (found.length === 0) found = findOccurrences(normalizeForMatch(view), normalizeForMatch(search))
  if (found.length === 0) found = findLineOccurrences(view, search)

  const fileEnding = lineEnding(source, "\n")
  return {
    matches: found.map((match) => ({ start: toSource(match.start), end: toSource(match.end) })),
    replace: (match: Match) =>
      replacement.replaceAll("\n", lineEnding(source.slice(match.start, match.end), fileEnding)),
  }
}

// ---- When nothing matches -------------------------------------------------------------

const HINT_LINES = 12
const HINT_LINE_LENGTH = 200

// A short hint so the next attempt can succeed without reading the file
// again: either newString is already there (the edit may already be done),
// or the lines closest to oldString, numbered as read shows them. Each line
// of oldString votes for where it starts, compared ignoring whitespace; at
// least half must agree.
function missHint(source: string, oldString: string, newString: string): string | undefined {
  const view = source.replaceAll("\r\n", "\n")
  const replacement = newString.replaceAll("\r\n", "\n")
  if (replacement.trim().length >= 20 && normalizeForMatch(view).includes(normalizeForMatch(replacement))) {
    return "newString is already in the file, so this edit may already be applied."
  }

  const key = (line: string) => normalizeForMatch(line).replace(/\s+/g, " ").trim()
  const distinct = (line: string) => /[\p{L}\p{N}]/u.test(line)
  const lines = view.split("\n")
  const search = oldString.replaceAll("\r\n", "\n").split("\n")

  const positions = new Map<string, number[]>()
  lines.forEach((line, index) => {
    const value = key(line)
    if (!distinct(value)) return
    positions.set(value, [...(positions.get(value) ?? []), index])
  })

  const votes = new Map<number, number>()
  let wanted = 0
  search.forEach((line, offset) => {
    const value = key(line)
    if (!distinct(value)) return
    wanted += 1
    for (const index of positions.get(value) ?? []) votes.set(index - offset, (votes.get(index - offset) ?? 0) + 1)
  })

  const [start, count] = [...votes].reduce((best, vote) => (vote[1] > best[1] ? vote : best), [0, 0])
  if (count === 0 || count * 2 < wanted) return undefined

  const matched = search.flatMap((line, offset) => {
    const value = key(line)
    const index = start + offset
    return value !== "" && index >= 0 && index < lines.length && key(lines[index]!) === value ? [index] : []
  })
  const from = matched[0]!
  const to = Math.min(matched.at(-1)! + 1, from + HINT_LINES)
  return [
    "The closest lines in the file:",
    ...lines
      .slice(from, to)
      .map((line, index) => `${from + index + 1}: ${line.length > HINT_LINE_LENGTH ? `${line.slice(0, HINT_LINE_LENGTH)}…` : line}`),
  ].join("\n")
}
