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
