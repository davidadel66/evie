import { afterEach, expect, test } from "bun:test"
import { realpath } from "node:fs/promises"
import { homedir } from "node:os"
import { shell } from "../src/index.ts"
import { toolProject } from "./support/tool-project.ts"

let stop: (() => Promise<void>) | undefined
afterEach(async () => {
  await stop?.()
  stop = undefined
})

async function project(contents: Record<string, string> = {}) {
  const made = await toolProject([shell()], contents)
  stop = made.run.stop
  return made
}

test("shell runs a command in the project folder and returns what it printed", async () => {
  const { run, call } = await project({ "hello.txt": "hi\n" })

  expect(await call("shell", { command: "cat hello.txt && pwd" })).toBe(`hi\n${await realpath(run.projectFolder)}`)
})

test("shell returns the output and the exit status of a command that fails, and the turn goes on", async () => {
  const { call } = await project()

  expect(await call("shell", { command: "echo out; echo problem >&2; exit 3" })).toBe("out\nproblem\n\n[Exit status: 3]")
})

test("shell runs in workdir when one is given", async () => {
  const { run, call } = await project({ "sub/inner.txt": "" })

  expect(await call("shell", { command: "pwd", workdir: "sub" })).toBe(`${await realpath(run.projectFolder)}/sub`)
  expect(await call("shell", { command: "pwd", workdir: "missing" })).toBe(
    'The folder "missing" does not exist, so the command was not run.',
  )
})

test("shell says when a command prints nothing", async () => {
  const { call } = await project()

  expect(await call("shell", { command: "true" })).toBe("[The command finished and printed nothing.]")
})

test("a command that waits for typed input does not block", async () => {
  const { call } = await project()

  expect(await call("shell", { command: "cat; echo done" })).toBe("done")
})

test("in the general session shell runs in the home folder", async () => {
  const { run, call } = await project()
  const general = await run.evie.send({ type: "open_session" })

  expect(await call("shell", { command: "pwd" }, general.session)).toBe(await realpath(homedir()))
})

test("a very long timeout is capped, and the command still runs", async () => {
  const { call } = await project()

  expect(await call("shell", { command: "echo ok", timeout: 9_999_999_999 })).toBe("ok")
})

test("a command that prints more than 2,000 lines shows its last lines, with how it ended", async () => {
  const { call } = await project()

  // 2,500 numbers, a blank line and the exit status make 2,502 lines.
  const result = await call("shell", { command: "seq 1 2500; exit 1" })

  expect(result).toStartWith("503\n504\n")
  expect(result).toContain("\n2500\n\n[Exit status: 1]\n\n[Output cut: showing the last 2000 of 2502 lines. The full output is saved at ")
})

test("a command that dies by a signal says which signal", async () => {
  const { call } = await project()

  expect(await call("shell", { command: "echo before; kill -KILL $$" })).toBe("before\n\n[Ended by signal SIGKILL]")
})
