import { expect, test } from "bun:test"
import { stepKind, stepSentence } from "../web/src/step-words.ts"

// A tool call in plain words, apart from any screen: one sentence saying
// what it did, as T3 Code's work log says it, and the kind of thing it
// did, which an icon shows. Its input is the model's, as JSON text or read.

test("a call is one sentence saying what it did, and while it runs what it is doing", () => {
  expect(stepSentence("read", { path: "src/auth.ts" })).toBe("Read src/auth.ts")
  expect(stepSentence("read", { path: "src/auth.ts" }, { running: true })).toBe("Reading src/auth.ts")
  expect(stepSentence("edit", { path: "README.md", oldString: "a", newString: "b" })).toBe("Edited README.md")
  expect(stepSentence("write", { path: "recipes/naan.md", content: "Flour" })).toBe("Wrote recipes/naan.md")
  expect(stepSentence("write", { path: "notes/beds.md", content: "North" }, { running: true })).toBe("Writing notes/beds.md")
  expect(stepSentence("shell", { command: "bun test" })).toBe("Ran bun test")
  expect(stepSentence("shell", { command: "bun test" }, { running: true })).toBe("Running bun test")
})

test("the input can be the JSON text the model wrote, and input it wrote badly still makes a sentence", () => {
  expect(stepSentence("read", '{"path":"src/auth.ts"}')).toBe("Read src/auth.ts")
  expect(stepSentence("read", "{not json")).toBe("Read a file")
  expect(stepSentence("shell", "{}", { running: true })).toBe("Running a command")
  expect(stepSentence("shell", { command: "bun test\n  --watch" })).toBe("Ran bun test --watch")
})

test("each of evie's tools has its own sentence, and a tool it does not know says it was used", () => {
  expect(stepSentence("grep", { pattern: "TODO", path: "src" })).toBe("Searched TODO in src")
  expect(stepSentence("grep", { pattern: "TODO" })).toBe("Searched TODO")
  expect(stepSentence("glob", { pattern: "**/*.ts" })).toBe("Searched files **/*.ts")
  expect(stepSentence("web_search", { query: "sqlite backup" })).toBe("Searched the web for sqlite backup")
  expect(stepSentence("web_fetch", { url: "https://sqlite.org/backup.html" })).toBe("Fetched https://sqlite.org/backup.html")
  expect(stepSentence("subagent", { description: "find the tests", task: "…" })).toBe("Asked a subagent to find the tests")
  expect(stepSentence("subagent", { description: "find the tests" }, { running: true })).toBe("Asking a subagent to find the tests")
  expect(stepSentence("subagent_send", { id: "s2", message: "Carry on" })).toBe("Messaged a subagent")
  expect(stepSentence("ask", { question: "Which bed?", choices: ["North", "South"] })).toBe("Asked a question")
  expect(stepSentence("skill", { name: "release" })).toBe("Read the skill release")
  expect(stepSentence("job_stop", { id: "1" })).toBe("Stopped job 1")
  expect(stepSentence("deploy", { target: "production" })).toBe("Used deploy")
})

test("a patch says the files it changed: the first, and how many more", () => {
  const patch = (files: string[]) => ({ patchText: ["*** Begin Patch", ...files.map((file) => `*** Update File: ${file}\n@@\n-a\n+b`), "*** End Patch"].join("\n") })

  expect(stepSentence("patch", patch(["src/a.ts"]))).toBe("Edited src/a.ts")
  expect(stepSentence("patch", patch(["src/a.ts", "src/b.ts", "src/c.ts"]), { running: true })).toBe("Editing src/a.ts +2 more")
})

test("a path inside the project's folder is given from there, as the tools take it, and one outside it in full", () => {
  expect(stepSentence("read", { path: "/home/me/evie/src/auth.ts" }, { folder: "/home/me/evie" })).toBe("Read src/auth.ts")
  expect(stepSentence("read", { path: "/elsewhere/notes.md" }, { folder: "/home/me/evie" })).toBe("Read /elsewhere/notes.md")
})

test("each call has the kind of thing it did, for its icon", () => {
  const kinds = ["read", "skill", "edit", "write", "patch", "shell", "grep", "glob", "web_search", "web_fetch", "subagent", "subagent_send", "ask", "job_stop", "deploy"].map(stepKind)

  expect(kinds).toEqual(["read", "read", "edit", "edit", "edit", "command", "search", "search", "web", "web", "agent", "agent", "question", "other", "other"])
})
