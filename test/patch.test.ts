import { afterEach, expect, test } from "bun:test"
import { files } from "../src/index.ts"
import { toolProject } from "./support/tool-project.ts"

let stop: (() => Promise<void>) | undefined
afterEach(async () => {
  await stop?.()
  stop = undefined
})

async function project(contents: Record<string, string> = {}) {
  const made = await toolProject([files()], contents)
  stop = made.run.stop
  const patch = (...lines: string[]) => made.call("patch", { patchText: ["*** Begin Patch", ...lines, "*** End Patch"].join("\n") })
  const exists = (name: string) => Bun.file(made.path(name)).exists()
  return { ...made, patch, exists }
}

const math = ["function add(a, b) {", "  return a + b", "}", "", "function sub(a, b) {", "  return a - b", "}", ""].join("\n")

test("patch changes lines in a file, finding them by what they say", async () => {
  const { patch, text } = await project({ "math.js": math })

  const result = await patch("*** Update File: math.js", "@@", " function sub(a, b) {", "-  return a - b", "+  return b - a", " }")

  expect(result).toBe("Success. Updated the following files:\nM math.js")
  expect(await text("math.js")).toBe(math.replace("return a - b", "return b - a"))
})

test("patch adds a file, creating any missing folders", async () => {
  const { patch, text } = await project()

  expect(await patch("*** Add File: src/deep/hello.txt", "+Hello", "+world")).toBe(
    "Success. Updated the following files:\nA src/deep/hello.txt",
  )
  expect(await text("src/deep/hello.txt")).toBe("Hello\nworld\n")
})

test("patch deletes a file", async () => {
  const { patch, exists } = await project({ "old.txt": "gone soon\n" })

  expect(await patch("*** Delete File: old.txt")).toBe("Success. Updated the following files:\nD old.txt")
  expect(await exists("old.txt")).toBe(false)
})

test("patch moves a file and changes it in one go", async () => {
  const { patch, text, exists } = await project({ "old/name.txt": "one\ntwo\n" })

  const result = await patch("*** Update File: old/name.txt", "*** Move to: new/name.txt", "@@", " one", "-two", "+2")

  expect(result).toBe("Success. Updated the following files:\nM new/name.txt (moved from old/name.txt)")
  expect(await text("new/name.txt")).toBe("one\n2\n")
  expect(await exists("old/name.txt")).toBe(false)
})

test("one patch can add, change and delete several files", async () => {
  const { patch, text, exists } = await project({ "math.js": math, "old.txt": "x\n" })

  const result = await patch(
    "*** Add File: new.txt",
    "+fresh",
    "*** Update File: math.js",
    "@@",
    "-  return a + b",
    "+  return a + b + 0",
    "*** Delete File: old.txt",
  )

  expect(result).toBe("Success. Updated the following files:\nA new.txt\nM math.js\nD old.txt")
  expect(await text("new.txt")).toBe("fresh\n")
  expect(await text("math.js")).toContain("return a + b + 0")
  expect(await exists("old.txt")).toBe(false)
})

test("a patch can change two places in one file, each found after a line that names where to look", async () => {
  const { patch, text } = await project({ "math.js": math })

  await patch(
    "*** Update File: math.js",
    "@@ function add(a, b) {",
    "-  return a + b",
    "+  return a + b // sum",
    "@@ function sub(a, b) {",
    "-  return a - b",
    "+  return a - b // difference",
  )

  expect(await text("math.js")).toBe(math.replace("return a + b", "return a + b // sum").replace("return a - b", "return a - b // difference"))
})

test("lines are found even when their whitespace or their quotes and dashes differ", async () => {
  const { patch, text } = await project({
    "trailing.txt": "keep   \nchange\t\n",
    "indent.py": "def f():\n        return 1\n",
    "copy.md": "She said “hello” — twice.\n",
  })

  await patch("*** Update File: trailing.txt", "@@", " keep", "-change", "+changed")
  await patch("*** Update File: indent.py", "@@", " def f():", "-    return 1", "+        return 2")
  await patch("*** Update File: copy.md", "@@", '-She said "hello" - twice.', "+She said nothing.")

  expect(await text("trailing.txt")).toBe("keep   \nchanged\n")
  expect(await text("indent.py")).toBe("def f():\n        return 2\n")
  expect(await text("copy.md")).toBe("She said nothing.\n")
})

test("lines added with nothing to find go at the end of the file", async () => {
  const { patch, text } = await project({ "list.txt": "one\ntwo\n" })

  await patch("*** Update File: list.txt", "@@", "+three")

  expect(await text("list.txt")).toBe("one\ntwo\nthree\n")
})

test("nothing is written when any part of the patch does not fit, and the result quotes the lines it looked for", async () => {
  const { patch, text, exists } = await project({ "math.js": math, "other.txt": "alpha\nbeta\n" })

  const result = await patch(
    "*** Add File: new.txt",
    "+fresh",
    "*** Update File: math.js",
    "@@",
    "-  return a + b",
    "+  return 0",
    "*** Update File: other.txt",
    "@@",
    " alpha",
    "-gamma",
    "+delta",
  )

  expect(result).toBe(
    "Failed to find expected lines in other.txt:\nalpha\ngamma\n\n" +
      "Nothing was changed. Read the file, then send the patch again with lines that match it.",
  )
  expect(await text("math.js")).toBe(math)
  expect(await exists("new.txt")).toBe(false)
})

test("a patch that is not well formed names the line that is wrong", async () => {
  const { call, patch } = await project({ "math.js": math })

  expect(await call("patch", { patchText: "*** Update File: math.js\n@@\n-a\n+b\n*** End Patch" })).toBe(
    'The patch is not valid: the first line must be "*** Begin Patch". Nothing was changed.',
  )
  expect(await call("patch", { patchText: "*** Begin Patch\n*** Update File: math.js\n@@\n-a\n+b" })).toBe(
    'The patch is not valid: the last line must be "*** End Patch". Nothing was changed.',
  )
  expect(await patch("*** Update File: math.js", "@@", "-  return a + b", "this line has no marker", "+  return 0")).toBe(
    "The patch is not valid at line 5: a line in a change must start with a space, + or -, but this one is " +
      '"this line has no marker". Nothing was changed.',
  )
  expect(await patch("*** Rename File: math.js")).toBe(
    'The patch is not valid at line 2: "*** Rename File: math.js" is not something a patch can do. Use ' +
      '"*** Add File: ", "*** Delete File: " or "*** Update File: ". Nothing was changed.',
  )
})

test("changing or deleting a file that does not exist, or adding one that does, says so", async () => {
  const { patch } = await project({ "here.txt": "x\n" })

  expect(await patch("*** Update File: missing.txt", "@@", "-a", "+b")).toBe(
    "File not found: missing.txt. Nothing was changed. To create it, use *** Add File.",
  )
  expect(await patch("*** Delete File: missing.txt")).toBe("File not found: missing.txt. Nothing was changed.")
  expect(await patch("*** Add File: here.txt", "+y")).toBe(
    "here.txt already exists. Nothing was changed. To change it, use *** Update File.",
  )
})

test("a later part of a patch sees what an earlier part did to the same file", async () => {
  const { patch, text } = await project({ "list.txt": "one\ntwo\n" })

  await patch("*** Update File: list.txt", "@@", "-one", "+1", "*** Update File: list.txt", "@@", " 1", "-two", "+2")

  expect(await text("list.txt")).toBe("1\n2\n")
})

test("patch keeps a file's Windows line endings", async () => {
  const { patch, text } = await project({ "dos.txt": "alpha\r\nbeta\r\ngamma\r\n" })

  await patch("*** Update File: dos.txt", "@@", " alpha", "-beta", "+BETA", "+extra")

  expect(await text("dos.txt")).toBe("alpha\r\nBETA\r\nextra\r\ngamma\r\n")
})

test("a patch wrapped the way a shell would pass it is still read", async () => {
  const { call, text } = await project({ "list.txt": "one\n" })

  const wrapped = ["<<'EOF'", "*** Begin Patch", "*** Update File: list.txt", "@@", "-one", "+1", "*** End Patch", "EOF"].join("\n")

  expect(await call("patch", { patchText: wrapped })).toBe("Success. Updated the following files:\nM list.txt")
  expect(await text("list.txt")).toBe("1\n")
})

test("lines added after a line that says where to look go straight after it, and several additions keep their order", async () => {
  const { patch, text } = await project({ "list.txt": "one\ntwo\nthree\n" })

  await patch("*** Update File: list.txt", "@@ one", "+one and a half", "@@", "+four", "@@", "+five")

  expect(await text("list.txt")).toBe("one\none and a half\ntwo\nthree\nfour\nfive\n")
})

test("a file cannot be moved onto one that already exists", async () => {
  const { patch, text } = await project({ "a.txt": "from a\n", "b.txt": "already here\n" })

  expect(await patch("*** Update File: a.txt", "*** Move to: b.txt", "@@", "-from a", "+moved")).toBe(
    "a.txt cannot be moved to b.txt, because that file already exists. Nothing was changed.",
  )
  expect(await text("b.txt")).toBe("already here\n")
})

test("a line of a file that looks like a patch instruction is still treated as a line of the file", async () => {
  const notes = "Example:\n*** Update File: demo.txt\nend\n"
  const { patch, text } = await project({ "notes.txt": notes })

  await patch("*** Update File: notes.txt", "@@", " Example:", " *** Update File: demo.txt", "-end", "+END")

  expect(await text("notes.txt")).toBe(notes.replace("end\n", "END\n"))
})
