import { expect, test } from "bun:test"
import { liveWords, runWords, stepFailed, stepKind, stepTarget, stepWords, type CallSaid } from "../web/src/step-words.ts"

// What evie's work says in plain words, apart from any screen, for the
// work log and the side chat alike: each tool call as a sentence, and a
// run of calls summed up, as T3 Code's work log says them. A call is known
// by its input in short, and, when the page has it, by its input whole.

// A call known by its input whole, as the model wrote it.
const whole = (name: string, fields: Record<string, unknown>): CallSaid => ({ name, input: "", fields })

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
  expect(stepWords({ name: "skill", input: "baking" })).toBe("Loaded the baking skill")
  expect(stepWords({ name: "ask", input: "Which bed first?" })).toBe("Asked you a question")
  expect(stepWords({ name: "job_stop", input: "3" })).toBe("Stopped background work")
  expect(stepWords({ name: "subagent_send", input: "s2" })).toBe("Sent a subagent a message")
  expect(stepWords({ name: "weather", input: "Cairo" })).toBe("Used weather")
})

test("while a call runs, its sentence says what it is doing", () => {
  expect(stepWords(whole("read", { path: "src/auth.ts" }), { running: true })).toBe("Reading src/auth.ts")
  expect(stepWords(whole("write", { path: "notes/beds.md", content: "North" }), { running: true })).toBe("Writing notes/beds.md")
  expect(stepWords(whole("shell", { command: "bun test" }), { running: true })).toBe("Running bun test")
  expect(stepWords(whole("subagent", { description: "Find the tests" }), { running: true })).toBe("Asking a subagent to find the tests")
  expect(stepWords(whole("ask", { question: "Which bed?" }), { running: true })).toBe("Asking you a question")
  expect(stepWords(whole("deploy", {}), { running: true })).toBe("Using deploy")
})

test("known whole, a call names what it worked on from its own field, a search where it looked, and a path from the project's folder", () => {
  expect(stepWords(whole("grep", { pattern: "TODO", path: "src" }))).toBe("Searched for TODO in src")
  expect(stepWords(whole("glob", { pattern: "*.ts", path: "/home/me/evie/web" }), { folder: "/home/me/evie" })).toBe("Looked for *.ts in web")
  expect(stepWords(whole("read", { path: "/home/me/evie/src/auth.ts" }), { folder: "/home/me/evie" })).toBe("Read src/auth.ts")
  expect(stepWords(whole("read", { path: "/elsewhere/notes.md" }), { folder: "/home/me/evie" })).toBe("Read /elsewhere/notes.md")
  expect(stepWords(whole("shell", { command: "bun test\n  --watch" }))).toBe("Ran bun test --watch")
  expect(stepWords(whole("skill", { name: "release" }))).toBe("Loaded the release skill")
})

test("a patch names the files it changes, the first and how many more past two, and a call known only by its result says what it did to something", () => {
  const patch = "*** Begin Patch *** Update File: a.ts @@ -x +y *** Add File: notes/b.md +hello *** End Patch"
  expect(stepWords({ name: "patch", input: patch })).toBe("Edited a.ts and notes/b.md")
  expect(stepWords({ name: "patch", input: `${patch} *** Delete File: c.ts` })).toBe("Edited a.ts +2 more")
  const lines = ["*** Begin Patch", "*** Update File: src/a.ts", "@@", "-a", "+b", "*** Add File: src/b.ts", "+b", "*** End Patch"].join("\n")
  expect(stepWords(whole("patch", { patchText: lines }), { running: true })).toBe("Editing src/a.ts and src/b.ts")
  expect(stepWords({ name: "read", input: "" })).toBe("Read a file")
  expect(stepWords({ name: "shell", input: "" })).toBe("Ran a command")
  expect(stepWords({ name: "subagent", input: "" })).toBe("Asked a subagent")
  expect(stepWords({ name: "grep", input: "" })).toBe("Searched the code")
})

test("a long input is cut short, on one line", () => {
  expect(stepWords({ name: "shell", input: `echo ${"a".repeat(100)}` })).toBe(`Ran echo ${"a".repeat(75)}…`)
})

test("while a run is carried out, its own row says the command's program, as T3 Code's live row does, and the run lists the call by the command alone", () => {
  const running = whole("shell", { command: "CI=1 ./node_modules/.bin/vitest run --reporter dot" })

  expect(liveWords(running)).toBe("Running vitest")
  expect(stepTarget(running)).toBe("CI=1 ./node_modules/.bin/vitest run --reporter dot")
  // Any other call says its sentence, and is listed by what it works on.
  expect(liveWords(whole("read", { path: "src/a.ts" }))).toBe("Reading src/a.ts")
  expect(stepTarget(whole("read", { path: "src/a.ts" }))).toBe("src/a.ts")
  expect(stepTarget(whole("ask", { question: "Which bed?" }))).toBe("Asking you a question")
})

test("each call has the kind of thing it did, for its icon", () => {
  const kinds = ["read", "skill", "edit", "write", "patch", "shell", "grep", "glob", "web_search", "web_fetch", "subagent", "subagent_send", "ask", "job_stop", "deploy"].map(stepKind)

  expect(kinds).toEqual(["read", "read", "edit", "edit", "edit", "command", "search", "search", "web", "web", "agent", "agent", "question", "other", "other"])
})

test("a call failed when its result says a command ended badly or could not start, the owner or a rule refused it, or the tool broke; one stopped with its turn did not", () => {
  expect(stepFailed("3 tests failed\n\n[Exit status: 1]")).toBe(true)
  expect(stepFailed("[Ended by signal SIGKILL]")).toBe(true)
  expect(stepFailed("[The command could not be started: no such shell]")).toBe(true)
  expect(stepFailed("[Stopped: the command ran past its timeout of 120 seconds.]")).toBe(true)
  expect(stepFailed("The owner said no to this. Do not try another way to do the same thing. Ask what they would like instead.")).toBe(true)
  expect(stepFailed("Not allowed: one of the owner's rules denies this.")).toBe(true)
  expect(stepFailed("Boom. This is a bug in evie or a plugin, not a problem with the input.")).toBe(true)
  expect(stepFailed("12 tests passed")).toBe(false)
  expect(stepFailed("[Stopped: the turn was cancelled.]")).toBe(false)
  expect(stepFailed("Cancelled: the turn was stopped while this tool was running.")).toBe(false)
  expect(stepFailed(undefined)).toBe(false)
})

test("a run of one call says it; a longer run counts its calls by kind, the two that matter most in the order they came, then the rest as other actions", () => {
  const call = (name: string, input = "x") => ({ name, input })
  expect(runWords([call("edit", "usage.html")])).toBe("Edited usage.html")
  expect(runWords([call("read"), call("read"), call("shell")])).toBe("Read 2 files and ran 1 command")
  // Changes count the files changed, each once.
  expect(runWords([call("edit", "a.ts"), call("write", "b.ts"), call("patch", "*** Update File: c.ts")])).toBe("Changed 3 files")
  expect(runWords([call("edit", "a.ts"), call("edit", "a.ts"), call("write", "b.ts")])).toBe("Changed 2 files")
  // Commands, changes and subagents come before reading and searching; a tool evie does not know last.
  expect(runWords([call("grep"), call("read"), call("shell"), call("shell")])).toBe("Searched code 1 time, ran 2 commands and performed 1 other action")
  expect(runWords([call("weather"), call("read"), call("shell")])).toBe("Read 1 file, ran 1 command and performed 1 other action")
  expect(runWords([call("weather"), call("web_search"), call("web_search"), call("subagent")])).toBe("Searched the web 2 times, asked 1 subagent and performed 1 other action")
  expect(runWords([call("weather"), call("deploy")])).toBe("Used 2 tools")
  // A skill and a message to a subagent are other actions.
  expect(runWords([call("read"), call("skill"), call("subagent_send")])).toBe("Read 1 file and used 2 tools")
})

test("a patch to three files counts as three files changed", () => {
  const lines = ["*** Begin Patch", "*** Update File: a.ts", "*** Update File: b.ts", "*** Add File: c.ts", "*** End Patch"].join("\n")
  expect(runWords([whole("patch", { patchText: lines }), whole("shell", { command: "bun test" })])).toBe("Changed 3 files and ran 1 command")
})

test("a show call names what it showed, the first and how many more past two, counts its files in a run, and failed when nothing could be shown", () => {
  expect(stepWords(whole("show", { paths: ["out/chart.png"] }))).toBe("Showed out/chart.png")
  expect(stepWords(whole("show", { paths: ["/p/design/logo.svg", "/p/out/card.pdf"] }), { folder: "/p" })).toBe("Showed design/logo.svg and out/card.pdf")
  expect(stepWords(whole("show", { paths: ["a.png", "b.svg", "c.md"] }), { running: true })).toBe("Showing a.png +2 more")
  expect(stepWords({ name: "show", input: "" })).toBe("Showed a file")
  expect(stepKind("show")).toBe("show")
  expect(runWords([whole("show", { paths: ["a.png", "b.svg"] }), whole("write", { path: "c.md" }), whole("show", { paths: ["c.md"] })])).toBe("Showed 3 files and changed 1 file")
  expect(stepFailed("Not shown: notes.txt is not a kind of file evie can show.")).toBe(true)
  expect(stepFailed('Shown to the owner under your reply:\n- ".evie/shown/2026-11-22/a.png" (image/png, 33 bytes), already in Storage')).toBe(false)
})
