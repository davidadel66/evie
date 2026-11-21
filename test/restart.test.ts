import { afterEach, expect, test } from "bun:test"
import { mkdtemp, realpath, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { shell, type EvieEvent, type Plugin } from "../src/index.ts"
import { say, startEvie, stored, type Run } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"

// Evie closed the way a restart closes it, and started again on the same
// data folder: what was running carries on, and the new run picks it up.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function tempFolder(prefix: string) {
  const folder = await realpath(await mkdtemp(join(tmpdir(), prefix)))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  return folder
}

// One data folder and one project folder, with Evie started in them as many
// times as a test needs. `restart` closes a run the way a restart does,
// leaving its work running; anything still open at the end is closed for good.
async function machine() {
  const home = await tempFolder("evie-home-")
  const projectFolder = await tempFolder("evie-project-")
  async function startAgain(script: ScriptedReply[], plugins: Plugin[] = [shell()]) {
    const model = scriptedProvider(script)
    const run = await startEvie([model.plugin, ...plugins], { home, projectFolder })
    let closed = false
    const close = async (how?: { restart: true }) => {
      if (!closed) await run.evie.close(how)
      closed = true
    }
    cleanups.push(() => close())
    return { run, model, restart: () => close({ restart: true }) }
  }
  return { home, projectFolder, startAgain }
}

function until(run: Run, matches: (event: EvieEvent) => boolean) {
  return new Promise<EvieEvent>((resolve) => {
    const stop = run.evie.subscribe((event) => {
      if (!matches(event)) return
      stop()
      resolve(event)
    })
  })
}
const of = (run: Run, session: string) => stored(run.events).filter((event) => event.session === session)
const notes = (run: Run, session: string) => of(run, session).flatMap((event) => (event.type === "note" ? [event.body.text] : []))
const turnEnded = (run: Run, session: string) => until(run, (event) => event.type === "turn_end" && event.session === session)

// ---- Background commands ------------------------------------------------------------------------

test("a background command keeps running across a restart, is found again, and its end is told once", async () => {
  const { home, startAgain } = await machine()
  const first = await startAgain([{ text: [], calls: [{ name: "shell", input: { command: "sleep 0.6; echo built", background: true } }] }, { text: ["Started."] }])
  await say(first.run, "build it")
  const session = first.run.session
  await first.restart()

  const second = await startAgain([{ text: ["The build is done."] }])
  const woken = turnEnded(second.run, session)
  await second.run.evie.resumeWork()
  const opened = await second.run.evie.send({ type: "open_session", session })
  expect(opened.jobs).toMatchObject([{ id: "1", status: "running", title: "sleep 0.6; echo built" }])

  await woken
  const file = join(home, "tool-output", session, "job-1.log")
  expect(notes(second.run, session)).toEqual([`Background job 1 (sleep 0.6; echo built) has finished.\n\nbuilt\n\nThe full output is in ${file}.`])
  await second.restart()

  // Told once: the run after that has nothing to tell.
  const third = await startAgain([])
  await third.run.evie.resumeWork()
  await Bun.sleep(300)
  expect(notes(third.run, session)).toEqual([])
})

test("a background command that ends while no Evie is running is told at the next start, which wakes its session", async () => {
  const { home, startAgain } = await machine()
  const first = await startAgain([{ text: [], calls: [{ name: "shell", input: { command: "sleep 0.3; echo tests failed; exit 4", background: true } }] }, { text: ["Started."] }])
  await say(first.run, "test it")
  const session = first.run.session
  await first.restart()
  await Bun.sleep(700)

  const second = await startAgain([{ text: ["Four tests failed."] }])
  const woken = turnEnded(second.run, session)
  await second.run.evie.resumeWork()
  await woken

  const file = join(home, "tool-output", session, "job-1.log")
  expect(notes(second.run, session)).toEqual([`Background job 1 (sleep 0.3; echo tests failed; exit 4) failed.\n\ntests failed\n\n[Exit status: 4]\n\nThe full output is in ${file}.`])
  expect(second.model.requests[0]?.messages.at(-1)?.text).toContain("tests failed")
})

// The process group of a command that prints its own first.
async function groupOf(file: string) {
  while ((await Bun.file(file).text().catch(() => "")).trim() === "") await Bun.sleep(5)
  return Number((await Bun.file(file).text()).trim().split("\n")[0])
}

test("a background command that went while no Evie was running, without saying how it ended, is told as not known", async () => {
  const { home, startAgain } = await machine()
  const first = await startAgain([{ text: [], calls: [{ name: "shell", input: { command: "ps -o pgid= -p $$; sleep 30", background: true } }] }, { text: ["Started."] }])
  await say(first.run, "serve it")
  const session = first.run.session
  const file = join(home, "tool-output", session, "job-1.log")
  const group = await groupOf(file)
  await first.restart()
  // Stopped outright, so its shell writes nothing.
  process.kill(-group, "SIGKILL")
  await Bun.sleep(100)

  const second = await startAgain([{ text: ["It has gone."] }])
  const woken = turnEnded(second.run, session)
  const ended = until(second.run, (event) => event.type === "job" && event.job.status !== "running")
  await second.run.evie.resumeWork()
  await woken

  expect(notes(second.run, session)).toEqual([
    `Background job 1 (ps -o pgid= -p $$; sleep 30) has ended, and how it ended is not known.\n\n${group}\n\n` +
      `[Evie restarted while it was running, and it has since gone without leaving its exit status.]\n\nThe full output is in ${file}.`,
  ])
  expect(await ended).toMatchObject({ job: { id: "1", status: "unknown" } })
})

test("a process that now has a command's id, but started at another time, is not taken for the command", async () => {
  const { home, startAgain } = await machine()
  const first = await startAgain([{ text: [], calls: [{ name: "shell", input: { command: "ps -o pgid= -p $$; sleep 30", background: true } }] }, { text: ["Started."] }])
  await say(first.run, "serve it")
  const session = first.run.session
  const group = await groupOf(join(home, "tool-output", session, "job-1.log"))
  cleanups.push(async () => void process.kill(-group, "SIGKILL"))
  await first.restart()
  // As if the command had gone and another process been given its id.
  const record = join(home, "tool-output", session, "job-1.json")
  const kept = JSON.parse(await Bun.file(record).text())
  await Bun.write(record, JSON.stringify({ ...kept, keep: { ...kept.keep, processStart: "Mon Jan  1 00:00:00 2001" } }))

  const second = await startAgain([{ text: ["It has gone."] }])
  const woken = turnEnded(second.run, session)
  await second.run.evie.resumeWork()
  await woken

  expect(notes(second.run, session)[0]).toStartWith("Background job 1 (ps -o pgid= -p $$; sleep 30) has ended, and how it ended is not known.")
  // The other process is left alone.
  expect(() => process.kill(group, 0)).not.toThrow()
})

// Sends a message and resolves once the output file of the session's first job has this in it.
async function untilPrinted(run: Run, text: string, home: string, printed: string) {
  await run.evie.send({ type: "send_message", session: run.session, text })
  const file = join(home, "tool-output", run.session, "job-1.log")
  while (!(await Bun.file(file).text().catch(() => "")).includes(printed)) await Bun.sleep(5)
  return file
}

test("a command a call was waiting on when Evie restarted becomes a background job, and the call's result says so", async () => {
  const { home, startAgain } = await machine()
  const first = await startAgain([{ text: [], calls: [{ name: "shell", input: { command: "echo started; sleep 0.6; echo finished" } }] }])
  const session = first.run.session
  const file = await untilPrinted(first.run, "build it", home, "started")
  await first.restart()

  const second = await startAgain([{ text: ["Carrying on."] }, { text: ["It finished."] }])
  const told = until(second.run, (event) => event.type === "note" && event.session === session && event.body.text.startsWith("Background job 1"))
  await second.run.evie.resumeWork()
  await told

  const results = of(second.run, session).flatMap((event) => (event.type === "tool_result" ? [event.body.text] : []))
  expect(results).toEqual([
    "Evie restarted while this was running, so it was moved to the background, where it carries on as job 1, " +
      `and its output is being written to ${file}. You will be told when it ends, with its output. Do not start another copy while it is running.`,
  ])
  expect(notes(second.run, session).at(-1)).toBe(`Background job 1 (echo started; sleep 0.6; echo finished) has finished.\n\nstarted\nfinished\n\nThe full output is in ${file}.`)
})
