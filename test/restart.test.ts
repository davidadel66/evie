import { afterEach, expect, test } from "bun:test"
import { chmod, mkdtemp, realpath, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { Database } from "bun:sqlite"
import { ask, jobs, shell, subagents, type Chunk, type EvieEvent, type Plugin, type Request } from "../src/index.ts"
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
// The model is a script of replies, or a plugin that stands in for it.
async function machine() {
  const home = await tempFolder("evie-home-")
  const projectFolder = await tempFolder("evie-project-")
  async function startAgain(script: ScriptedReply[] | Plugin, plugins: Plugin[] = [shell()], options: { questions?: boolean } = {}) {
    const model = Array.isArray(script) ? scriptedProvider(script) : { plugin: script, requests: [] }
    const run = await startEvie([model.plugin, ...plugins], { home, projectFolder, ...options })
    let closed = false
    const close = async (how?: { restart: true }) => {
      if (!closed) await run.evie.close(how)
      closed = true
    }
    cleanups.push(() => close())
    return { run, model, restart: () => close({ restart: true }), quit: () => close() }
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
// A turn that ends as turns do, not one closed off because Evie stopped in the middle of it.
const turnCompleted = (run: Run, session: string) =>
  until(run, (event) => event.type === "turn_end" && event.session === session && event.body.reason === "completed")

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

test("a background command that died by a signal while no Evie was running is told by that signal", async () => {
  const { home, startAgain } = await machine()
  const first = await startAgain([{ text: [], calls: [{ name: "shell", input: { command: "sleep 0.3; kill -TERM $$", background: true } }] }, { text: ["Started."] }])
  await say(first.run, "run it")
  const session = first.run.session
  await first.restart()
  await Bun.sleep(700)

  const second = await startAgain([{ text: ["It was stopped."] }])
  const woken = turnEnded(second.run, session)
  await second.run.evie.resumeWork()
  await woken

  const file = join(home, "tool-output", session, "job-1.log")
  expect(notes(second.run, session)).toEqual([`Background job 1 (sleep 0.3; kill -TERM $$) failed.\n\n[Ended by signal SIGTERM]\n\nThe full output is in ${file}.`])
})

// Whether any process of a group is there. Signal 0 only asks.
const groupThere = (group: number) => {
  try {
    process.kill(-group, 0)
    return true
  } catch {
    return false
  }
}

test("a job no plugin loaded now can pick up again is told as ended in a way not known, with what to do", async () => {
  const { startAgain } = await machine()
  const first = await startAgain([{ text: [], calls: [{ name: "shell", input: { command: "sleep 0.5", background: true } }] }, { text: ["Started."] }])
  await say(first.run, "run it")
  const session = first.run.session
  await first.restart()

  const second = await startAgain([{ text: ["It is not known."] }], [])
  const woken = turnEnded(second.run, session)
  await second.run.evie.resumeWork()
  await woken

  expect(notes(second.run, session)).toEqual([
    "Background job 1 (sleep 0.5) has ended, and how it ended is not known.\n\nEvie restarted while it was running, and could not " +
      'pick it up again: no plugin loaded now picks up work of the kind "shell". Check whether it took effect, and run it again only if it did not.',
  ])
})

test("a job that ended while a turn was running, cut off by a restart before it was told, is told at the next start", async () => {
  const { startAgain } = await machine()
  const first = await startAgain([{ text: [], calls: [{ name: "shell", input: { command: "sleep 0.3; echo done", background: true } }] }, { hang: true }])
  const session = first.run.session
  // Its note waits for the turn's next step, which never comes.
  const ended = until(first.run, (event) => event.type === "job" && event.job.status === "finished")
  await first.run.evie.send({ type: "send_message", session, text: "run it" })
  await ended
  await first.restart()

  const second = await startAgain([{ text: ["Carrying on."] }, { text: ["It is done."] }])
  const told = until(second.run, (event) => event.type === "note" && event.session === session && event.body.text.startsWith("Background job 1"))
  await second.run.evie.resumeWork()

  const note = await told
  expect(note.type === "note" && note.body.text).toStartWith("Background job 1 (sleep 0.3; echo done) has finished.\n\ndone")
})

test("a job handed over while Evie closes for good is told as stopped, and not told again at the next start", async () => {
  const { startAgain } = await machine()
  // A tool that hands its work over only once its turn is stopped, as Evie closes.
  let running = () => {}
  const started = new Promise<void>((resolve) => (running = resolve))
  const late: Plugin = {
    name: "late",
    setup(host) {
      host.tools.add({
        name: "late",
        description: "Hands work over late.",
        inputSchema: { type: "object" },
        run: (_input, context) =>
          new Promise((resolve) => {
            running()
            context.signal.addEventListener("abort", () => {
              let finish = (_result: string) => {}
              const done = new Promise<string>((settle) => (finish = settle))
              context.jobs.start({ kind: "late", title: "late work", done, stop: () => finish("It was stopped.") })
              resolve("Handed over.")
            })
          }),
      })
    },
  }
  const first = await startAgain([{ text: [], calls: [{ name: "late", input: {} }] }], [late])
  const session = first.run.session
  await first.run.evie.send({ type: "send_message", session, text: "go" })
  await started
  await first.quit()

  expect(notes(first.run, session)).toEqual(["Background job 1 (late work) was stopped because Evie was closed. Start it again if it is still needed."])
  const second = await startAgain([], [late])
  await second.run.evie.resumeWork()
  await Bun.sleep(200)
  expect(notes(second.run, session)).toEqual([])
})

// The process group of a command that prints its own first, once the whole line is there.
async function groupOf(file: string) {
  while (!(await Bun.file(file).text().catch(() => "")).includes("\n")) await Bun.sleep(5)
  return Number((await Bun.file(file).text()).split("\n")[0])
}

test("a background command that went while no Evie was running, without saying how it ended, is told as not known", async () => {
  const { home, startAgain } = await machine()
  const first = await startAgain([{ text: [], calls: [{ name: "shell", input: { command: "ps -o pgid= -p $$ | tr -d ' '; sleep 30", background: true } }] }, { text: ["Started."] }])
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
    `Background job 1 (ps -o pgid= -p $$ | tr -d ' '; sleep 30) has ended, and how it ended is not known.\n\n${group}\n\n` +
      "[Evie restarted while it was running, and it has since gone without leaving its exit status. Check whether it took " +
      `effect, and run it again only if it did not.]\n\nThe full output is in ${file}.`,
  ])
  expect(await ended).toMatchObject({ job: { id: "1", status: "unknown" } })
})

test("a process that now has a command's id, but started at another time, is not taken for the command", async () => {
  const { home, startAgain } = await machine()
  const first = await startAgain([{ text: [], calls: [{ name: "shell", input: { command: "ps -o pgid= -p $$ | tr -d ' '; sleep 30", background: true } }] }, { text: ["Started."] }])
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

  expect(notes(second.run, session)[0]).toStartWith("Background job 1 (ps -o pgid= -p $$ | tr -d ' '; sleep 30) has ended, and how it ended is not known.")
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

test("a command picked up again whose start time cannot be read is still watched while it runs, and can be stopped", async () => {
  const { home, startAgain } = await machine()
  const first = await startAgain([{ text: [], calls: [{ name: "shell", input: { command: "ps -o pgid= -p $$ | tr -d ' '; sleep 30", background: true } }] }, { text: ["Started."] }])
  await say(first.run, "serve it")
  const session = first.run.session
  const file = join(home, "tool-output", session, "job-1.log")
  const group = await groupOf(file)
  cleanups.push(async () => void (groupThere(group) && process.kill(-group, "SIGKILL")))
  await first.restart()
  // A ps that fails, put first on the path.
  const tools = await tempFolder("evie-tools-")
  await Bun.write(join(tools, "ps"), "#!/bin/sh\nexit 1\n")
  await chmod(join(tools, "ps"), 0o755)
  const path = process.env.PATH
  process.env.PATH = `${tools}:${path}`
  cleanups.push(async () => void (process.env.PATH = path))

  const second = await startAgain([{ text: ["Stopped."] }], [shell(), jobs()])
  await second.run.evie.resumeWork()
  await Bun.sleep(1200)
  expect((await second.run.evie.send({ type: "open_session", session })).jobs).toMatchObject([{ id: "1", status: "running" }])

  const stopped = until(second.run, (event) => event.type === "job" && event.job.status === "stopped")
  expect((await second.run.evie.send({ type: "run_command", session, name: "jobs", args: "stop 1" })).text).toStartWith("Stopping job 1")
  await stopped
  expect(groupThere(group)).toBe(false)
  expect(notes(second.run, session)).toEqual([
    `Background job 1 (ps -o pgid= -p $$ | tr -d ' '; sleep 30) was stopped by the owner.\n\n${group}\n\n[Stopped on request.]\n\nThe full output is in ${file}.`,
  ])
})

// ---- Turns cut off ------------------------------------------------------------------------------

const RESTARTED =
  "Evie's server restarted while you were working, and your turn was cut short. Carry on from where you left off. A " +
  "command or other action you started may have finished, or may still be running: before you repeat one that " +
  "changes something, check that it did not take effect and is not still running, and if you cannot tell, say so " +
  "rather than repeat it."

// Sends a message, and resolves once the model has been asked for this many replies in all.
async function untilAsked(run: Run, requests: unknown[], text: string, count = 1) {
  await run.evie.send({ type: "send_message", session: run.session, text })
  while (requests.length < count) await Bun.sleep(5)
}

test("a turn of the owner's session cut off by a restart is closed off and carried on by itself, with a note", async () => {
  const { startAgain } = await machine()
  const first = await startAgain([{ hang: true, text: ["Looking"] }])
  const session = first.run.session
  await untilAsked(first.run, first.model.requests, "find the bug")
  await first.restart()

  const second = await startAgain([{ text: ["Found it."] }])
  const carriedOn = turnCompleted(second.run, session)
  await second.run.evie.resumeWork()
  await carriedOn

  expect(of(second.run, session).map((event) => event.type)).toEqual(["turn_end", "turn_start", "note", "assistant_message", "turn_end"])
  expect(of(second.run, session)[0]).toMatchObject({ body: { reason: "error", error: "Evie stopped while this turn was running." } })
  expect(second.model.requests[0]?.messages).toEqual([
    { role: "user", text: "find the bug" },
    { role: "user", text: `[Note from Evie: ${RESTARTED}]` },
  ])
})

// Moves a session's events back in time, as if they had happened this many minutes earlier.
function happenedEarlier(home: string, session: string, minutes: number) {
  const db = new Database(join(home, "evie.db"))
  db.run("UPDATE event SET time = time - ? WHERE session = ?", [minutes * 60_000, session])
  db.close()
}

const LONG_AGO =
  "Evie's server restarted while you were working, and this turn was cut off more than an hour ago, so it was left " +
  "stopped rather than carried on now. It is left for the owner to carry on."

test("a turn cut off within the last hour is carried on, and one cut off longer ago is left stopped, with a note", async () => {
  for (const minutes of [59, 61]) {
    const { home, startAgain } = await machine()
    const first = await startAgain([{ hang: true }])
    const session = first.run.session
    await untilAsked(first.run, first.model.requests, "find the bug")
    await first.restart()
    happenedEarlier(home, session, minutes)

    const second = await startAgain([{ text: ["Found it."] }])
    await second.run.evie.resumeWork()
    await Bun.sleep(100)

    if (minutes === 59) expect(notes(second.run, session)).toEqual([RESTARTED])
    else expect(notes(second.run, session)).toEqual([LONG_AGO])
    expect(second.model.requests).toHaveLength(minutes === 59 ? 1 : 0)
  }
})

test("a turn is carried on after at most three restarts in five minutes, and then left stopped, with a note", async () => {
  const { startAgain } = await machine()
  const first = await startAgain([{ hang: true }])
  const session = first.run.session
  await untilAsked(first.run, first.model.requests, "do the thing")
  await first.restart()
  // Each restart cuts off the turn carried on after the one before.
  for (let restart = 1; restart <= 3; restart++) {
    const again = await startAgain([{ hang: true }])
    await again.run.evie.resumeWork()
    while (again.model.requests.length < 1) await Bun.sleep(5)
    await again.restart()
  }

  const last = await startAgain([])
  await last.run.evie.resumeWork()
  await Bun.sleep(100)

  expect(last.model.requests).toEqual([])
  expect(notes(last.run, session)).toEqual([
    "Evie's server restarted while you were working, and this turn had already been carried on after three restarts, so it was left stopped, in case it is what stops the server.",
  ])
  expect(await last.run.evie.send({ type: "list_sessions" })).toMatchObject([{ session, state: "active" }])
})

test("a turn cut off while it was waiting for an answer is left stopped, with a note, and the question is closed", async () => {
  const { startAgain } = await machine()
  const first = await startAgain([{ text: [], calls: [{ name: "ask", input: { question: "Which branch?", choices: ["main", "dev"] } }] }], [ask()], { questions: true })
  const session = first.run.session
  const asked = until(first.run, (event) => event.type === "question")
  await first.run.evie.send({ type: "send_message", session, text: "merge it" })
  await asked
  await first.restart()

  const second = await startAgain([], [ask()])
  await second.run.evie.resumeWork()
  await Bun.sleep(100)

  expect(second.model.requests).toEqual([])
  expect(of(second.run, session).map((event) => event.type)).toEqual(["answer", "tool_result", "turn_end", "note"])
  expect(notes(second.run, session)).toEqual([
    "Evie's server restarted while this turn was waiting for an answer to a question, so the turn was stopped and the question went unanswered. It is left for the owner to carry on.",
  ])
})

test("the questions of a turn carried on wait for the owner, who answers once a client opens the session", async () => {
  const { startAgain } = await machine()
  const first = await startAgain([{ hang: true }], [ask()])
  const session = first.run.session
  await untilAsked(first.run, first.model.requests, "merge it")
  await first.restart()

  const second = await startAgain([{ text: [], calls: [{ name: "ask", input: { question: "Which branch?", choices: ["main", "dev"] } }] }, { text: ["Merged into dev."] }], [ask()])
  const asked = until(second.run, (event) => event.type === "question" && event.session === session)
  await second.run.evie.resumeWork()
  await asked
  await Bun.sleep(50)
  const opened = await second.run.evie.send({ type: "open_session", session, questions: true })
  expect(opened.openQuestions).toMatchObject([{ text: "Which branch?" }])

  const done = turnCompleted(second.run, session)
  await second.run.evie.send({ type: "answer_question", session, id: opened.openQuestions[0]!.id, choice: "dev" })
  await done
  expect(of(second.run, session).flatMap((event) => (event.type === "tool_result" ? [event.body.text] : []))).toEqual(["The owner chose: dev"])
})

// ---- Subagents ----------------------------------------------------------------------------------

type Reply = { text?: string; calls?: { name: string; input: Record<string, unknown> }[] } | { hang: true }

// A stand-in for the model with one script for the main agent and one for
// subagents, which tells them apart by the system prompt a subagent is given.
function twoScripts(parent: Reply[], child: Reply[]) {
  const requests = { parent: [] as Request[], child: [] as Request[] }
  let calls = 0
  const plugin: Plugin = {
    name: "two-scripts",
    setup(host) {
      host.providers.add({
        name: "two-scripts",
        async *stream(request, signal): AsyncGenerator<Chunk> {
          const who = request.system?.includes("You are a subagent") ? "child" : "parent"
          requests[who].push(structuredClone(request))
          const reply = (who === "child" ? child : parent)[requests[who].length - 1]
          if (!reply) throw new Error(`the ${who} script has no reply for this request`)
          if ("hang" in reply) {
            await new Promise<void>((resolve) => (signal.aborted ? resolve() : signal.addEventListener("abort", () => resolve(), { once: true })))
            throw new Error("the request was cancelled")
          }
          if (reply.text) yield { type: "text", text: reply.text }
          for (const call of reply.calls ?? []) yield { type: "tool_call", id: `call_${++calls}`, name: call.name, arguments: JSON.stringify(call.input) }
          yield { type: "finish", reason: reply.calls?.length ? "tool_calls" : "stop" }
        },
      })
    },
  }
  return { plugin, requests }
}

const hand = { name: "subagent", input: { description: "look into it", task: "Find where the bug is." } }

test("a subagent cut off by a restart is carried on, its job is there again, and its report reaches the session that started it", async () => {
  const { home, startAgain } = await machine()
  const firstModel = twoScripts([{ calls: [hand] }, { text: "A subagent is looking." }], [{ hang: true }])
  const first = await startAgain(firstModel.plugin, [subagents({ dataFolder: home }), jobs()])
  const session = first.run.session
  await say(first.run, "where is the bug?")
  while (firstModel.requests.child.length < 1) await Bun.sleep(5)
  const child = stored(first.run.events).find((event) => event.session !== session)!.session
  await first.restart()

  const secondModel = twoScripts([{ text: "It is in parse.ts." }], [{ text: "The bug is in parse.ts, line 12." }])
  const second = await startAgain(secondModel.plugin, [subagents({ dataFolder: home }), jobs()])
  const reported = turnCompleted(second.run, session)
  await second.run.evie.resumeWork()
  const opened = await second.run.evie.send({ type: "open_session", session })
  expect(opened.jobs).toMatchObject([{ id: "1", kind: "subagent", status: "running", title: "subagent: look into it" }])
  await reported

  expect(secondModel.requests.child[0]?.messages.at(-1)?.text).toBe(`[Note from Evie: ${RESTARTED}]`)
  expect(notes(second.run, session)).toEqual([
    `Background job 1 (subagent: look into it) has finished.\n\nIts report:\n\nThe bug is in parse.ts, line 12.\n\n` +
      `To send this subagent a further message, use subagent_send with the id "${child}".`,
  ])
})

test("a subagent that finished before a restart, whose report the session that started it was not told, has it delivered", async () => {
  const { home, startAgain } = await machine()
  const firstModel = twoScripts([{ calls: [hand] }, { text: "A subagent is looking." }, { text: "Noted." }], [{ text: "The bug is in parse.ts, line 12." }])
  const first = await startAgain(firstModel.plugin, [subagents({ dataFolder: home }), jobs()])
  const session = first.run.session
  const told = until(first.run, (event) => event.type === "turn_end" && event.session === session && firstModel.requests.parent.length === 3)
  await say(first.run, "where is the bug?")
  await told
  await first.restart()
  // As if Evie had stopped between the subagent finishing and the report being told.
  const record = join(home, "tool-output", session, "job-1.json")
  await Bun.write(record, JSON.stringify({ ...JSON.parse(await Bun.file(record).text()), status: "running", told: false }))

  const secondModel = twoScripts([{ text: "It is in parse.ts." }], [])
  const second = await startAgain(secondModel.plugin, [subagents({ dataFolder: home }), jobs()])
  const reported = turnCompleted(second.run, session)
  await second.run.evie.resumeWork()
  await reported

  expect(secondModel.requests.child).toEqual([])
  expect(notes(second.run, session)[0]).toStartWith("Background job 1 (subagent: look into it) has finished.\n\nIts report:\n\nThe bug is in parse.ts, line 12.")
})

test("a subagent's turn cut off within the last hour is carried on, and one cut off longer ago is left stopped", async () => {
  for (const minutes of [59, 61]) {
    const { home, startAgain } = await machine()
    const firstModel = twoScripts([{ calls: [hand] }, { text: "A subagent is looking." }], [{ hang: true }])
    const first = await startAgain(firstModel.plugin, [subagents({ dataFolder: home }), jobs()])
    const session = first.run.session
    await say(first.run, "where is the bug?")
    while (firstModel.requests.child.length < 1) await Bun.sleep(5)
    const child = stored(first.run.events).find((event) => event.session !== session)!.session
    await first.restart()
    happenedEarlier(home, child, minutes)

    const secondModel = twoScripts([{ text: "Noted." }], [{ text: "The bug is in parse.ts, line 12." }])
    const second = await startAgain(secondModel.plugin, [subagents({ dataFolder: home }), jobs()])
    const reported = turnCompleted(second.run, session)
    await second.run.evie.resumeWork()
    await reported

    expect(secondModel.requests.child).toHaveLength(minutes === 59 ? 1 : 0)
    expect(notes(second.run, child)).toEqual([minutes === 59 ? RESTARTED : LONG_AGO])
    expect(notes(second.run, session)[0]).toStartWith(
      minutes === 59 ? "Background job 1 (subagent: look into it) has finished." : "Background job 1 (subagent: look into it) failed.\n\nIt failed: Evie stopped while this turn was running.\n\n",
    )
  }
})

test("a subagent cut off while waiting for an answer is left stopped, and the turn of the session it asked through is carried on", async () => {
  const { home, startAgain } = await machine()
  // A subagent is not given the ask tool, but its tools can still ask, as a permission check does.
  const confirm: Plugin = {
    name: "confirm",
    setup(host) {
      host.tools.add({
        name: "confirm",
        description: "Asks the owner to go ahead.",
        inputSchema: { type: "object" },
        run: async (_input, context) => `The owner said: ${await context.ask({ text: "Go ahead?", choices: ["yes", "no"] })}`,
      })
    },
  }
  const firstModel = twoScripts([{ calls: [hand] }, { hang: true }], [{ calls: [{ name: "confirm", input: {} }] }])
  const first = await startAgain(firstModel.plugin, [subagents({ dataFolder: home }), jobs(), confirm], { questions: true })
  const session = first.run.session
  const asked = until(first.run, (event) => event.type === "question")
  await first.run.evie.send({ type: "send_message", session, text: "where is the bug?" })
  await asked
  while (firstModel.requests.parent.length < 2) await Bun.sleep(5)
  const child = stored(first.run.events).find((event) => event.session !== session)!.session
  await first.restart()

  const secondModel = twoScripts([{ text: "Carrying on." }, { text: "Noted." }, { text: "Noted." }], [])
  const second = await startAgain(secondModel.plugin, [subagents({ dataFolder: home }), jobs(), confirm])
  const carriedOn = turnCompleted(second.run, session)
  await second.run.evie.resumeWork()
  await carriedOn

  expect(secondModel.requests.child).toEqual([])
  expect(notes(second.run, child)).toEqual([
    "Evie's server restarted while this turn was waiting for an answer to a question, so the turn was stopped and the question went unanswered. It is left for the owner to carry on.",
  ])
  expect(notes(second.run, session)[0]).toBe(RESTARTED)
  expect(of(second.run, session).filter((event) => event.type === "answer")).toMatchObject([{ body: { choice: null } }])
})
