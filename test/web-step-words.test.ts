import { expect, test } from "bun:test"
import { runWords, stepWords } from "../web/src/step-words.ts"

// What evie's work says in plain words, apart from any screen: each tool
// call as a sentence, and a run of calls summed up, as T3 Code's work log
// says them.

test("a call says what it did to what, in plain words", () => {
  expect(stepWords({ name: "read", input: "README.md" })).toBe("Read README.md")
  expect(stepWords({ name: "edit", input: "README.md" })).toBe("Edited README.md")
  expect(stepWords({ name: "write", input: "recipes/naan.md" })).toBe("Wrote recipes/naan.md")
  expect(stepWords({ name: "shell", input: "bun test" })).toBe("Ran bun test")
  expect(stepWords({ name: "subagent", input: "Find recipes with no time" })).toBe("Asked a subagent to find recipes with no time")
  expect(stepWords({ name: "grep", input: "TODO" })).toBe("Searched for TODO")
  expect(stepWords({ name: "glob", input: "**/*.md" })).toBe("Looked for **/*.md")
  expect(stepWords({ name: "web_search", input: "naan recipes" })).toBe("Searched the web for naan recipes")
  expect(stepWords({ name: "web_fetch", input: "https://example.com/naan" })).toBe("Read https://example.com/naan")
  expect(stepWords({ name: "skill", input: "baking" })).toBe("Used the baking skill")
  expect(stepWords({ name: "ask", input: "Which bed first?" })).toBe("Asked you a question")
  expect(stepWords({ name: "job_stop", input: "3" })).toBe("Stopped background work")
  expect(stepWords({ name: "weather", input: "Cairo" })).toBe("Used weather")
})

test("a patch names the files it changes, and a call known only by its result says what it did to something", () => {
  const patch = "*** Begin Patch *** Update File: a.ts @@ -x +y *** Add File: notes/b.md +hello *** End Patch"
  expect(stepWords({ name: "patch", input: patch })).toBe("Edited a.ts and notes/b.md")
  expect(stepWords({ name: "patch", input: `${patch} *** Delete File: c.ts` })).toBe("Edited a.ts, notes/b.md and c.ts")
  expect(stepWords({ name: "read", input: "" })).toBe("Read a file")
  expect(stepWords({ name: "shell", input: "" })).toBe("Ran a command")
  expect(stepWords({ name: "subagent", input: "" })).toBe("Asked a subagent")
})

test("a long input is cut short, on one line", () => {
  expect(stepWords({ name: "shell", input: `echo ${"a".repeat(100)}` })).toBe(`Ran echo ${"a".repeat(75)}…`)
})

test("a run of one call says it; a longer run counts its calls by kind, two kinds at most, then the rest as other actions", () => {
  const call = (name: string, input = "x") => ({ name, input })
  expect(runWords([call("edit", "usage.html")])).toBe("Edited usage.html")
  expect(runWords([call("read"), call("read"), call("shell")])).toBe("Read 2 files and ran 1 command")
  expect(runWords([call("edit"), call("write"), call("patch")])).toBe("Changed 3 files")
  expect(runWords([call("grep"), call("read"), call("shell"), call("shell")])).toBe("Searched code 1 time, read 1 file and performed 2 other actions")
  expect(runWords([call("weather"), call("read"), call("shell")])).toBe("Read 1 file, ran 1 command and performed 1 other action")
})
