import { afterEach, expect, test } from "bun:test"
import { readFile, realpath, writeFile } from "node:fs/promises"
import { join } from "node:path"
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

test("a command that leaves something running is waited for until all it started has ended, and at its timeout moves to the background", async () => {
  const { call } = await project()

  expect(await call("shell", { command: "(sleep 0.4; echo late) & echo early" })).toBe("early\nlate")
  expect(await call("shell", { command: "sleep 30 & echo early", timeout: 1 })).toStartWith(
    "The command did not finish within 1 second, so it was moved to the background, where it is still running. It is background job 1,",
  )
})

test("a command that prints more than 5 MiB shows its end, and keeps all of it in a file it names", async () => {
  const { call } = await project()

  const result = await call("shell", { command: "yes 0123456789 | head -c 6000000; echo; echo the very end" })

  expect(result).toContain("\nthe very end\n\n[Only the end of the output is shown.]\n\n[The full output is saved at ")
  const kept = /The full output is saved at (\S+)\. Read that file/.exec(result)![1]!
  expect((await readFile(kept, "utf8")).length).toBe(6000000 + "\nthe very end\n".length)
  expect(result).not.toContain("[Output cut:")
})

test("a command whose output file cannot be made is not run, and the result says the folder cannot be written and what to do", async () => {
  const { run, call } = await project()
  // Stands for a data folder where tool output cannot be written.
  await writeFile(join(run.home, "tool-output"), "")

  expect(await call("shell", { command: "echo hi" })).toBe(
    `The command was not run: Evie could not write its output file in ${join(run.home, "tool-output", run.session)}, the folder ` +
      "where it keeps what tools print (ENOTDIR). Tell the owner that this folder cannot be written to: no command can run until it can.",
  )
})

test("a command that exits with a status that is not a death by a signal that ends a process says its status, and ends at once", async () => {
  const { call } = await project()

  // On macOS 145 is 128 and stop's number, and 148 is 128 and that of a child having ended.
  expect(await call("shell", { command: "exit 145", timeout: 3 })).toBe("[Exit status: 145]")
  expect(await call("shell", { command: "exit 148", timeout: 3 })).toBe("[Exit status: 148]")
  expect(await call("shell", { command: "kill -TERM $$" })).toBe("[Ended by signal SIGTERM]")
})
