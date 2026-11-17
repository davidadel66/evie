import { afterEach, expect, test } from "bun:test"
import { files } from "../src/index.ts"
import { toolProject } from "./support/tool-project.ts"

let stop: (() => Promise<void>) | undefined
afterEach(async () => {
  await stop?.()
  stop = undefined
})

async function project(contents: Record<string, string> = {}, options?: Parameters<typeof files>[0]) {
  const made = await toolProject([files(options)], contents)
  stop = made.run.stop
  return made
}

const code = {
  "src/app.ts": 'import { start } from "./server"\n\n// TODO: read the port from settings\nstart(8080)\n',
  "src/server.ts": "export function start(port: number) {\n  // TODO: handle errors\n  listen(port)\n}\n",
  "docs/notes.md": "# Notes\n\nTODO: write the guide\n",
}

// ---- grep -------------------------------------------------------------------------

test("grep finds matching lines, grouped by file, with line numbers", async () => {
  const { call } = await project(code)

  expect(await call("grep", { pattern: "TODO" })).toBe(
    [
      "Found 3 matches in 3 files.",
      "",
      "docs/notes.md:",
      "  3: TODO: write the guide",
      "",
      "src/app.ts:",
      "  3: // TODO: read the port from settings",
      "",
      "src/server.ts:",
      "  2:   // TODO: handle errors",
    ].join("\n"),
  )
})

test("grep searches under a path, and only in files that match include", async () => {
  const { call } = await project(code)

  expect(await call("grep", { pattern: "TODO", path: "src" })).toStartWith("Found 2 matches in 2 files.")
  expect(await call("grep", { pattern: "TODO", include: "*.md" })).toBe(
    "Found 1 match in 1 file.\n\ndocs/notes.md:\n  3: TODO: write the guide",
  )
})

test("grep takes a regular expression", async () => {
  const { call } = await project(code)

  expect(await call("grep", { pattern: "start\\(\\d+\\)" })).toBe("Found 1 match in 1 file.\n\nsrc/app.ts:\n  4: start(8080)")
})

test("grep returns at most 100 matches and says to narrow the search when there are more", async () => {
  const many = Array.from({ length: 150 }, (_, index) => `needle ${index + 1}`).join("\n")
  const { call } = await project({ "big.txt": many })

  const result = await call("grep", { pattern: "needle" })

  expect(result).toStartWith("Found more than 100 matches. Showing the first 100.")
  expect(result).toContain("  100: needle 100")
  expect(result).not.toContain("needle 101")
  expect(result).toEndWith("[There are more matches. Narrow the pattern or the path to see the rest.]")
})

test("grep says when nothing matches, and when the pattern is not a valid regular expression", async () => {
  const { call } = await project(code)

  expect(await call("grep", { pattern: "nothing-like-this" })).toBe('No matches for "nothing-like-this".')
  expect(await call("grep", { pattern: "start(" })).toStartWith('The pattern "start(" is not a valid regular expression:')
})

test("grep says how to fix a missing ripgrep", async () => {
  const { call } = await project(code, { ripgrep: "/no/such/ripgrep" })

  expect(await call("grep", { pattern: "TODO" })).toBe(
    "grep needs ripgrep, and it is not installed. Install it (for example `brew install ripgrep`), then try again.",
  )
})

// ---- glob -------------------------------------------------------------------------

test("glob finds files whose names match a pattern", async () => {
  const { call } = await project(code)

  expect(await call("glob", { pattern: "**/*.ts" })).toBe("Found 2 files.\n\nsrc/app.ts\nsrc/server.ts")
  expect(await call("glob", { pattern: "*.ts", path: "docs" })).toBe('No files match "*.ts".')
})

test("glob returns at most 100 files and says to narrow the search when there are more", async () => {
  const many = Object.fromEntries(Array.from({ length: 120 }, (_, index) => [`f${String(index).padStart(3, "0")}.txt`, ""]))
  const { call } = await project(many)

  const result = await call("glob", { pattern: "*.txt" })

  expect(result).toStartWith("Found more than 100 files. Showing the first 100.")
  expect(result).toContain("f099.txt")
  expect(result).not.toContain("f100.txt")
  expect(result).toEndWith("[There are more files. Narrow the pattern or the path to see the rest.]")
})

test("searching in the general session needs a full path", async () => {
  const { run, call, path } = await project(code)
  const general = await run.evie.send({ type: "open_session" })

  expect(await call("glob", { pattern: "*.md", path: path("docs") }, general.session)).toContain("notes.md")
  expect(await call("grep", { pattern: "TODO" }, general.session)).toBe(
    "This session is not in a project, so there is no folder to search. Give the full path of a folder as path.",
  )
})
