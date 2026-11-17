import { afterEach, expect, test } from "bun:test"
import { mkdir, readFile, writeFile } from "node:fs/promises"
import { join } from "node:path"
import { files } from "../src/index.ts"
import { say, startEvie, stored } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"

let stop: (() => Promise<void>) | undefined
afterEach(async () => {
  await stop?.()
  stop = undefined
})

// Starts Evie with the files plugin and a project holding the given files.
// `call` has the model call one tool, and returns the text it gets back.
async function project(contents: Record<string, string> = {}) {
  const script: ScriptedReply[] = []
  const run = await startEvie([scriptedProvider(script).plugin, files()])
  stop = run.stop
  const path = (name: string) => join(run.projectFolder, name)
  for (const [name, text] of Object.entries(contents)) {
    await mkdir(join(path(name), ".."), { recursive: true })
    await writeFile(path(name), text)
  }
  async function call(name: string, input: Record<string, unknown>, session = run.session) {
    script.push({ text: [], calls: [{ name, input }] }, { text: ["done"] })
    await say(run, "go", session)
    const results = stored(run.events).flatMap((event) => (event.type === "tool_result" ? [event.body.text] : []))
    return results.at(-1)!
  }
  return { run, call, path, text: (name: string) => readFile(path(name), "utf8") }
}

// ---- read -------------------------------------------------------------------------

test("read returns a file's lines, each with its line number", async () => {
  const { call } = await project({ "notes.txt": "alpha\nbeta\ngamma\n" })

  expect(await call("read", { path: "notes.txt" })).toBe("1: alpha\n2: beta\n3: gamma")
})

test("read takes a part of a file with offset and limit, and says how to read on", async () => {
  const { call } = await project({ "notes.txt": "one\ntwo\nthree\nfour\nfive\n" })

  expect(await call("read", { path: "notes.txt", offset: 2, limit: 2 })).toBe(
    "2: two\n3: three\n\n[Showing lines 2 to 3 of 5. Read on with offset: 4.]",
  )
})

test("read returns the first 2,000 lines of a long file by default", async () => {
  const long = Array.from({ length: 2100 }, (_, index) => `line ${index + 1}`).join("\n")
  const { call } = await project({ "long.txt": long })

  const result = await call("read", { path: "long.txt" })

  expect(result).toContain("2000: line 2000")
  expect(result).not.toContain("2001: line 2001")
  expect(result).toEndWith("[Showing lines 1 to 2000 of 2100. Read on with offset: 2001.]")
})

test("read on a missing file suggests up to three similar names", async () => {
  const { call } = await project({
    "src/config.ts": "",
    "src/config.test.ts": "",
    "src/Config.json": "",
    "src/configuration.md": "",
    "src/other.ts": "",
  })

  const result = await call("read", { path: "src/config.js" })

  expect(result).toStartWith("File not found: src/config.js. Did you mean: ")
  const named = result.split("Did you mean: ")[1]!.replace("?", "").split(", ")
  expect(named).toHaveLength(3)
  expect(named).toContain("config.ts")
  expect(named).not.toContain("other.ts")
})

test("read on a missing file in a missing folder says so", async () => {
  const { call } = await project()

  expect(await call("read", { path: "nowhere/file.txt" })).toMatch(/^File not found: nowhere\/file\.txt\. The folder .*nowhere does not exist either\.$/)
})

test("read on a folder lists what is in it", async () => {
  const { call } = await project({ "src/a.ts": "", "src/lib/b.ts": "" })

  expect(await call("read", { path: "src" })).toBe("src is a folder. It contains:\na.ts\nlib/")
})

test("read says when a file is empty, binary, or shorter than the line asked for", async () => {
  const { call, path } = await project({ "empty.txt": "", "short.txt": "only\n" })
  await writeFile(path("image.bin"), new Uint8Array([137, 80, 78, 71, 0, 0, 1, 2]))

  expect(await call("read", { path: "empty.txt" })).toBe("[empty.txt is empty.]")
  expect(await call("read", { path: "image.bin" })).toBe("image.bin looks like a binary file, so it is not shown.")
  expect(await call("read", { path: "short.txt", offset: 5 })).toBe("short.txt has only 1 line, so there is nothing at line 5.")
})

test("read accepts a full path, and in the general session only a full path", async () => {
  const { run, call, path } = await project({ "notes.txt": "alpha\n" })
  const general = await run.evie.send({ type: "open_session" })

  expect(await call("read", { path: path("notes.txt") })).toBe("1: alpha")
  expect(await call("read", { path: path("notes.txt") }, general.session)).toBe("1: alpha")
  expect(await call("read", { path: "notes.txt" }, general.session)).toBe(
    'This session is not in a project, so there is no folder to take "notes.txt" from. Give the full path.',
  )
})

test("read shows a file with Windows line endings without them", async () => {
  const { call } = await project({ "dos.txt": "alpha\r\nbeta\r\n" })

  expect(await call("read", { path: "dos.txt" })).toBe("1: alpha\n2: beta")
})

// ---- write ------------------------------------------------------------------------

test("write creates a file and any missing folders", async () => {
  const { call, text } = await project()

  expect(await call("write", { path: "src/deep/new.ts", content: "export const a = 1\n" })).toBe(
    "Created src/deep/new.ts (1 line).",
  )
  expect(await text("src/deep/new.ts")).toBe("export const a = 1\n")
})

test("write replaces everything in an existing file", async () => {
  const { call, text } = await project({ "notes.txt": "old\ncontent\n" })

  expect(await call("write", { path: "notes.txt", content: "new\ncontent\nhere" })).toBe("Overwrote notes.txt (3 lines).")
  expect(await text("notes.txt")).toBe("new\ncontent\nhere")
})

test("write refuses a folder, and needs both a path and content", async () => {
  const { call } = await project({ "src/a.ts": "" })

  expect(await call("write", { path: "src", content: "x" })).toBe("src is a folder, not a file. Give the path of a file inside it.")
  expect(await call("write", { path: "a.txt" })).toBe(
    'The input for write is not right: "content" is required. Send the call again with the input fixed.',
  )
})

// ---- edit -------------------------------------------------------------------------

const source = ["function add(a, b) {", "  return a + b", "}", "", "function sub(a, b) {", "  return a - b", "}", ""].join("\n")

test("edit replaces text that matches exactly one place", async () => {
  const { call, text } = await project({ "math.js": source })

  expect(await call("edit", { path: "math.js", oldString: "return a + b", newString: "return a + b + 0" })).toBe(
    "Edited math.js (1 replacement).",
  )
  expect(await text("math.js")).toBe(source.replace("return a + b", "return a + b + 0"))
})

test("edit refuses text that matches several places, and says how many", async () => {
  const { call, text } = await project({ "math.js": source })

  expect(await call("edit", { path: "math.js", oldString: "(a, b) {", newString: "(x, y) {" })).toBe(
    "Found 2 matches for oldString in math.js, but expected exactly one. Add more surrounding context to make " +
      "oldString unique, or set replaceAll to true to replace every occurrence.",
  )
  expect(await text("math.js")).toBe(source)
})

test("edit with replaceAll changes every match", async () => {
  const { call, text } = await project({ "math.js": source })

  expect(await call("edit", { path: "math.js", oldString: "(a, b) {", newString: "(x, y) {", replaceAll: true })).toBe(
    "Edited math.js (2 replacements).",
  )
  expect(await text("math.js")).toBe(source.replaceAll("(a, b) {", "(x, y) {"))
})

test("edit that finds nothing shows the closest lines in the file", async () => {
  const { call, text } = await project({ "math.js": source })

  const result = await call("edit", {
    path: "math.js",
    oldString: "function sub(a, b) {\n  return a * b\n}",
    newString: "function sub(a, b) {\n  return b - a\n}",
  })

  expect(result).toBe(
    "Could not find oldString in math.js. It must match exactly, including whitespace and indentation.\n\n" +
      "The closest lines in the file:\n5: function sub(a, b) {\n6:   return a - b\n7: }",
  )
  expect(await text("math.js")).toBe(source)
})

test("edit that finds nothing says when the new text is already in the file", async () => {
  const { call } = await project({ "math.js": source })

  const result = await call("edit", { path: "math.js", oldString: "return a * b * c", newString: "function sub(a, b) {\n  return a - b" })

  expect(result).toEndWith("newString is already in the file, so this edit may already be applied.")
})

test("edit refuses an empty oldString, identical strings and a missing file", async () => {
  const { call } = await project({ "math.js": source })

  expect(await call("edit", { path: "math.js", oldString: "", newString: "x" })).toBe(
    "oldString must not be empty. Use write to create or overwrite a file.",
  )
  expect(await call("edit", { path: "math.js", oldString: "x", newString: "x" })).toBe(
    "No changes to apply: oldString and newString are identical.",
  )
  expect(await call("edit", { path: "nope.js", oldString: "a", newString: "b" })).toBe(
    "File not found: nope.js. Use write to create it.",
  )
})

test("edit matches plain quotes and dashes against typographic ones in the file", async () => {
  const { call, text } = await project({ "copy.md": "She said “hello” — twice.\n" })

  expect(await call("edit", { path: "copy.md", oldString: 'said "hello" - twice', newString: "said nothing" })).toBe(
    "Edited copy.md (1 replacement).",
  )
  expect(await text("copy.md")).toBe("She said nothing.\n")
})

test("edit matches lines whose trailing whitespace differs", async () => {
  const { call, text } = await project({ "list.txt": "first   \nsecond\t\nthird\n" })

  expect(await call("edit", { path: "list.txt", oldString: "first\nsecond", newString: "1\n2" })).toBe(
    "Edited list.txt (1 replacement).",
  )
  expect(await text("list.txt")).toBe("1\n2\nthird\n")
})

test("edit keeps a file's Windows line endings", async () => {
  const { call, text } = await project({ "dos.txt": "alpha\r\nbeta\r\ngamma\r\n" })

  expect(await call("edit", { path: "dos.txt", oldString: "alpha\nbeta", newString: "one\ntwo\nthree" })).toBe(
    "Edited dos.txt (1 replacement).",
  )
  expect(await text("dos.txt")).toBe("one\r\ntwo\r\nthree\r\ngamma\r\n")
})

test("a file system refusal is returned for the model to act on, and the turn goes on", async () => {
  const { run, call } = await project({ "notes.txt": "a file\n" })

  const result = await call("write", { path: "notes.txt/inside.txt", content: "x" })

  expect(result).toStartWith("write could not finish: ")
  expect(stored(run.events).at(-1)).toMatchObject({ type: "turn_end", body: { reason: "completed" } })
})
