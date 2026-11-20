import { afterEach, expect, test } from "bun:test"
import { mkdtemp, readFile, realpath, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { files, jobs, permissions, runTerminal, shell, type EvieEvent, type Job, type Plugin } from "../src/index.ts"
import { say, startEvie, stored, type Run } from "./support/harness.ts"
import { scriptedProvider, type ScriptedCall, type ScriptedReply } from "./support/scripted-provider.ts"

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

// Resolves with the next event the client is sent that matches.
function until(run: { evie: Run["evie"] }, matches: (event: EvieEvent) => boolean) {
  return new Promise<EvieEvent>((resolve) => {
    const stop = run.evie.subscribe((event) => {
      if (!matches(event)) return
      stop()
      resolve(event)
    })
  })
}
const turnEnds = (run: Run, count: number) => {
  let seen = stored(run.events).filter((event) => event.type === "turn_end").length
  return seen >= count ? Promise.resolve() : until(run, (event) => event.type === "turn_end" && ++seen >= count).then(() => {})
}
const types = (run: Run): string[] => stored(run.events).map((event) => event.type)
const notes = (run: Run) => stored(run.events).flatMap((event) => (event.type === "note" ? [event.body.text] : []))
const jobEvents = (run: Run) => run.events.flatMap((event) => (event.type === "job" ? [`${event.job.id} ${event.job.status}`] : []))
const lastResult = (run: Run) => stored(run.events).findLast((event) => event.type === "tool_result")!.body.text as string

// A tool that hands a piece of work to the background. The test decides
// when the work ends and how, so nothing here depends on timing.
function backgroundWork() {
  const work: { finish(result: string): void; fail(reason: string): void; stopped: boolean; job: Job | undefined }[] = []
  const plugin: Plugin = {
    name: "background-work",
    setup(host) {
      host.tools.add({
        name: "start_work",
        description: "Starts some work in the background.",
        inputSchema: { type: "object" },
        run: async (input, context) => {
          let finish = (_result: string) => {}
          let fail = (_reason: string) => {}
          const done = new Promise<string>((resolve, reject) => {
            finish = resolve
            fail = (reason) => reject(new Error(reason))
          })
          const entry = { finish, fail, stopped: false, job: undefined as Job | undefined }
          work.push(entry)
          entry.job = context.jobs.start({
            kind: "work",
            title: String(input.title ?? "some work"),
            done,
            stop: () => {
              entry.stopped = true
              finish("It was stopped.")
            },
          })
          return `Started as job ${entry.job.id}.`
        },
      })
    },
  }
  return { plugin, work }
}

const startWork = (title = "some work"): ScriptedCall => ({ name: "start_work", input: { title } })

async function start(script: ScriptedReply[], plugins: Plugin[]) {
  const model = scriptedProvider(script)
  const run = await startEvie([model.plugin, ...plugins])
  cleanups.push(run.stop)
  const command = async (args: string) => (await run.evie.send({ type: "run_command", session: run.session, name: "jobs", args })).text
  return { run, model, command }
}

// ---- The jobs list --------------------------------------------------------------------------

test("work handed to the background outlives the call, and when it ends the model is told and woken", async () => {
  const background = backgroundWork()
  const { run, model } = await start([{ text: [], calls: [startWork("the build")] }, { text: ["Started it."] }, { text: ["The build passed."] }], [background.plugin])

  await say(run, "build it")
  expect(lastResult(run)).toBe("Started as job 1.")
  expect(jobEvents(run)).toEqual(["1 running"])

  // The turn is over. The work ends, and a new turn starts by itself.
  background.work[0]!.finish("All 12 steps passed.")
  await turnEnds(run, 2)

  expect(types(run)).toEqual([
    ...["turn_start", "user_message", "assistant_message", "tool_result", "assistant_message", "turn_end"],
    ...["turn_start", "note", "assistant_message", "turn_end"],
  ])
  expect(notes(run)).toEqual(["Background job 1 (the build) has finished.\n\nAll 12 steps passed."])
  expect(model.requests[2]?.messages.at(-1)).toEqual({ role: "user", text: "[Note from Evie: Background job 1 (the build) has finished.\n\nAll 12 steps passed.]" })
  expect(jobEvents(run)).toEqual(["1 running", "1 finished"])
})

test("a job that ends while a turn is running is told at the turn's next step, without a turn of its own", async () => {
  const background = backgroundWork()
  // A tool that ends the job while it runs, as if the two had overlapped.
  const overlaps: Plugin = {
    name: "overlaps",
    setup(host) {
      host.tools.add({
        name: "wait",
        description: "Waits.",
        inputSchema: { type: "object" },
        run: async () => {
          background.work[0]!.finish("Done early.")
          await Bun.sleep(1)
          return "waited"
        },
      })
    },
  }
  const { run, model } = await start(
    [{ text: [], calls: [startWork()] }, { text: [], calls: [{ name: "wait", input: {} }] }, { text: ["Both done."] }],
    [background.plugin, overlaps],
  )

  await say(run, "go")
  await Bun.sleep(5)

  expect(types(run)).toEqual(["turn_start", "user_message", "assistant_message", "tool_result", "assistant_message", "tool_result", "note", "assistant_message", "turn_end"])
  expect(model.requests).toHaveLength(3)
})

test("a job that fails says so", async () => {
  const background = backgroundWork()
  const { run } = await start([{ text: [], calls: [startWork("the deploy")] }, { text: ["Started."] }, { text: ["It failed."] }], [background.plugin])

  await say(run, "go")
  background.work[0]!.fail("the server refused the connection")
  await turnEnds(run, 2)

  expect(notes(run)).toEqual(["Background job 1 (the deploy) failed.\n\nthe server refused the connection"])
  expect(jobEvents(run)).toEqual(["1 running", "1 failed"])
})

test("/jobs lists the session's jobs, and /jobs stop stops one and tells the model without waking it", async () => {
  const background = backgroundWork()
  const { run, command } = await start(
    [{ text: [], calls: [startWork("the server")] }, { text: [], calls: [startWork("the watcher")] }, { text: ["Both running."] }, { text: ["Noted."] }],
    [background.plugin, jobs()],
  )

  expect(await command("")).toBe("Nothing has been run in the background in this session.")
  await say(run, "start both")
  expect(await command("")).toMatch(/^1 {2}running for \d+s {2}the server\n2 {2}running for \d+s {2}the watcher$/)

  expect(await command("stop 2")).toBe("Stopping job 2 (the watcher).")
  await Bun.sleep(5)
  expect(background.work[1]!.stopped).toBe(true)
  expect(await command("")).toMatch(/\n2 {2}stopped after \d+s {2}the watcher$/)
  expect(await command("stop 9")).toBe("There is no job 9. /jobs lists them.")

  // The model is told, but no turn starts for it: it reads the note with the owner's next message.
  expect(types(run).filter((type) => type === "turn_start")).toHaveLength(1)
  expect(notes(run)).toEqual(["Background job 2 (the watcher) was stopped by the owner.\n\nIt was stopped."])
  await say(run, "thanks")
  expect(types(run).slice(-5)).toEqual(["note", "turn_start", "user_message", "assistant_message", "turn_end"])
})

test("the model can stop a job itself, and is not told again that it stopped", async () => {
  const background = backgroundWork()
  const { run } = await start(
    [{ text: [], calls: [startWork("the server")] }, { text: [], calls: [{ name: "job_stop", input: { id: "1" } }] }, { text: ["Stopped it."] }, { text: [], calls: [{ name: "job_stop", input: { id: "7" } }] }, { text: ["ok"] }],
    [background.plugin, jobs()],
  )

  await say(run, "start then stop")
  expect(lastResult(run)).toStartWith("Job 1 (the server) is being stopped.")
  expect(background.work[0]!.stopped).toBe(true)
  await Bun.sleep(5)
  expect(notes(run)).toEqual([])

  await say(run, "stop another")
  expect(lastResult(run)).toBe('There is no job "7". No job is running.')
})

test("cancelling a turn leaves its background jobs running, and closing Evie stops them", async () => {
  const background = backgroundWork()
  const { run } = await start([{ text: [], calls: [startWork("the server")] }, { hang: true }], [background.plugin])

  const hanging = until(run, (event) => event.type === "tool_result")
  const ended = until(run, (event) => event.type === "turn_end")
  await run.evie.send({ type: "send_message", session: run.session, text: "start it" })
  await hanging
  await run.evie.send({ type: "cancel", session: run.session })
  await ended
  expect(background.work[0]!.stopped).toBe(false)

  await run.evie.close()
  expect(background.work[0]!.stopped).toBe(true)
  // Written to the log, so a session continued later does not go on waiting to hear from the job.
  expect(notes(run)).toEqual(["Background job 1 (the server) was stopped because Evie was closed. Start it again if it is still needed."])
})

// ---- Shell commands in the background ------------------------------------------------------------

test("a command started in the background returns at once, and the model is told its output when it ends", async () => {
  const { run } = await start(
    [{ text: [], calls: [{ name: "shell", input: { command: "echo one; sleep 0.3; echo two; exit 3", background: true } }] }, { text: ["Running."] }, { text: ["It ended."] }],
    [shell()],
  )
  const started = Date.now()

  await say(run, "run it")
  expect(Date.now() - started).toBeLessThan(250)
  const result = lastResult(run)
  expect(result).toStartWith("The command was started in the background. It is background job 1, and its output is being written to ")
  expect(result).toContain('To stop it, use job_stop with the id "1".')

  await turnEnds(run, 2)
  const file = join(run.home, "tool-output", run.session, "job-1.log")
  expect(notes(run)).toEqual([`Background job 1 (echo one; sleep 0.3; echo two; exit 3) failed.\n\none\ntwo\n\n[Exit status: 3]\n\nThe full output is in ${file}.`])
  expect(await readFile(file, "utf8")).toBe("one\ntwo\n")
})

test("a command still running at its timeout moves to the background and carries on", async () => {
  const { run } = await start(
    [{ text: [], calls: [{ name: "shell", input: { command: "echo before; sleep 1.4; echo after", timeout: 1 } }] }, { text: ["Still going."] }, { text: ["Finished."] }],
    [shell()],
  )

  await say(run, "run it")
  expect(lastResult(run)).toStartWith("The command did not finish within 1 second, so it was moved to the background, where it is still running. It is background job 1")

  await turnEnds(run, 2)
  expect(notes(run)[0]).toContain("before\nafter\n\nThe full output is in ")
})

test("a background command given a timeout is stopped when it runs past it", async () => {
  const { run } = await start(
    [{ text: [], calls: [{ name: "shell", input: { command: "echo started; sleep 30", background: true, timeout: 1 } }] }, { text: ["Running."] }, { text: ["It was stopped."] }],
    [shell()],
  )

  await say(run, "run it")
  await turnEnds(run, 2)

  expect(notes(run)[0]).toContain("started\n\n[Stopped: the command ran past its timeout of 1 second.]")
})

test("a background command is stopped when Evie closes", async () => {
  const { run } = await start(
    [{ text: [], calls: [{ name: "shell", input: { command: "echo $$; sleep 30", background: true } }] }, { text: ["Running."] }],
    [shell()],
  )
  await say(run, "run it")
  const file = join(run.home, "tool-output", run.session, "job-1.log")
  while ((await readFile(file, "utf8").catch(() => "")) === "") await Bun.sleep(5)
  const pid = Number((await readFile(file, "utf8")).trim())

  await run.evie.close()

  // Signal 0 only asks whether the process is there.
  expect(() => process.kill(pid, 0)).toThrow()
})

test("the model may read a job's output file, though it is in Evie's data folder", async () => {
  const home = await realpath(await mkdtemp(join(tmpdir(), "evie-home-")))
  cleanups.push(() => rm(home, { recursive: true, force: true }))
  const script: ScriptedReply[] = [{ text: [], calls: [{ name: "shell", input: { command: "echo from the job", background: true } }] }, { text: ["Running."] }, { text: ["Ended."] }]
  const run = await startEvie([scriptedProvider(script).plugin, shell(), files(), permissions({ mode: "auto", dataFolder: home })], { home })
  cleanups.push(run.stop)
  await say(run, "run it")
  await turnEnds(run, 2)

  const file = join(home, "tool-output", run.session, "job-1.log")
  script.push({ text: [], calls: [{ name: "read", input: { path: file } }] }, { text: ["Read."] })
  await say(run, "read the output")
  expect(lastResult(run)).toContain("from the job")

  // The rest of the data folder is still out of reach, however it is spelled.
  for (const path of [join(home, "evie.db"), join(home, "tool-output", "..", "evie.db")]) {
    script.push({ text: [], calls: [{ name: "read", input: { path } }] }, { text: ["No."] })
    await say(run, "read the database")
    expect(lastResult(run)).toStartWith("Blocked by the guard:")
  }
})

// ---- The terminal ---------------------------------------------------------------------------------

test("the terminal says when a job starts and ends, and shows the turn its ending causes", async () => {
  const background = backgroundWork()
  const model = scriptedProvider([{ text: [], calls: [startWork("the build")] }, { text: ["Started it."] }, { text: ["The build passed."] }])
  const run = await startEvie([model.plugin, background.plugin])
  cleanups.push(run.stop)
  let printed = ""
  await runTerminal({
    evie: run.evie,
    folder: run.projectFolder,
    lines: (async function* () {
      yield "build it"
      while (!printed.includes("Started it.")) await Bun.sleep(1)
      background.work[0]!.finish("All passed.")
      while (!printed.includes("The build passed.")) await Bun.sleep(1)
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain("[background] job 1 started: the build\n")
  expect(printed).toMatch(/\[background\] job 1 finished after \d+s: the build\nThe build passed\./)
})

// ---- Found in review -------------------------------------------------------------------------------

test("a session continued after Evie stopped carries on counting its jobs, so an id is never used twice", async () => {
  const home = await realpath(await mkdtemp(join(tmpdir(), "evie-home-")))
  const projectFolder = await realpath(await mkdtemp(join(tmpdir(), "evie-project-")))
  cleanups.push(() => rm(home, { recursive: true, force: true }), () => rm(projectFolder, { recursive: true, force: true }))
  const first = backgroundWork()
  const before = await startEvie([scriptedProvider([{ text: [], calls: [startWork("first")] }, { text: ["Started."] }]).plugin, first.plugin], { home, projectFolder })
  await say(before, "go")
  await before.stop()

  const second = backgroundWork()
  const after = await startEvie([scriptedProvider([{ text: [], calls: [startWork("second")] }, { text: ["Started."] }]).plugin, second.plugin], { home, projectFolder })
  cleanups.push(after.stop)
  const continued = await after.evie.send({ type: "open_session", folder: projectFolder, continue: true })
  await say(after, "go again", continued.session)

  expect(second.work[0]!.job).toMatchObject({ id: "2", session: before.session })
})

test("a background command that ends badly is a job that failed, and one that prints a great deal is told by its end", async () => {
  const { run } = await start(
    [{ text: [], calls: [{ name: "shell", input: { command: "seq 1 20000; echo the very last line; exit 2", background: true } }] }, { text: ["Running."] }, { text: ["It failed."] }],
    [shell()],
  )

  await say(run, "run it")
  await turnEnds(run, 2)

  expect(jobEvents(run)).toEqual(["1 running", "1 failed"])
  expect(notes(run)[0]).toContain("the very last line\n\n[Only the end of the output is shown.]\n[Exit status: 2]")
  expect(notes(run)[0]!.length).toBeLessThan(7000)
  // The file has all of it.
  expect((await readFile(join(run.home, "tool-output", run.session, "job-1.log"), "utf8")).split("\n")).toHaveLength(20002)
})

test("a session may read its own saved output and not another session's", async () => {
  const home = await realpath(await mkdtemp(join(tmpdir(), "evie-home-")))
  cleanups.push(() => rm(home, { recursive: true, force: true }))
  const script: ScriptedReply[] = [{ text: [], calls: [{ name: "shell", input: { command: "echo secret of the first session", background: true } }] }, { text: ["Running."] }, { text: ["Ended."] }]
  const run = await startEvie([scriptedProvider(script).plugin, shell(), files(), permissions({ mode: "auto", dataFolder: home })], { home })
  cleanups.push(run.stop)
  await say(run, "run it")
  await turnEnds(run, 2)
  const other = await run.evie.send({ type: "open_session", folder: run.projectFolder })

  script.push({ text: [], calls: [{ name: "read", input: { path: join(home, "tool-output", run.session, "job-1.log") } }] }, { text: ["No."] })
  await say(run, "read the other session's output", other.session)

  expect(lastResult(run)).toStartWith("Blocked by the guard:")
})

test("with waitForJobs the terminal stays until a background job has ended and Evie has dealt with it", async () => {
  const model = scriptedProvider([{ text: [], calls: [{ name: "shell", input: { command: "sleep 0.3; echo built", background: true } }] }, { text: ["Started it."] }, { text: ["The build is done."] }])
  const run = await startEvie([model.plugin, shell()])
  cleanups.push(run.stop)
  let printed = ""
  await runTerminal({
    evie: run.evie,
    folder: run.projectFolder,
    waitForJobs: true,
    lines: (async function* () {
      yield "build it"
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain("The build is done.")
})

test("/background moves the command Evie is waiting on to the background, and is only for when it is waiting", async () => {
  const { run } = await start(
    [{ text: [], calls: [{ name: "shell", input: { command: "echo started; sleep 0.6; echo finished" } }] }, { text: ["Carrying on."] }, { text: ["It finished."] }],
    [shell(), jobs()],
  )
  const background = async () => (await run.evie.send({ type: "run_command", session: run.session, name: "background", args: "" })).text
  expect(await background()).toBe("Nothing is running. This is for when Evie is waiting on a long command.")

  const calling = until(run, (event) => event.type === "assistant_message")
  await run.evie.send({ type: "send_message", session: run.session, text: "run it" })
  await calling
  await Bun.sleep(100)
  expect(await background()).toBe("Moved to the background. Evie carries on, and is told when it finishes.")

  // The call returns at once, the model carries on, and it is told when the command ends.
  await turnEnds(run, 1)
  expect(lastResult(run)).toStartWith("The owner moved the command to the background, where it is still running. It is background job 1")
  await turnEnds(run, 2)
  expect(notes(run)[0]).toContain("started\nfinished")

  // Once nothing is waited on, there is nothing to move.
  expect(await background()).toBe("Nothing is running. This is for when Evie is waiting on a long command.")
})
