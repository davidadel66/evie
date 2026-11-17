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

test("shell stops a command that runs past its timeout, and says so", async () => {
  const { call } = await project()
  const started = Date.now()

  const result = await call("shell", { command: "echo before; sleep 30; echo after", timeout: 1 })

  expect(result).toBe("before\n\n[Stopped: the command ran past its timeout of 1 second.]")
  expect(Date.now() - started).toBeLessThan(10_000)
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
