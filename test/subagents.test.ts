import { afterEach, expect, test } from "bun:test"
import { mkdir, mkdtemp, realpath, rm, symlink, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { files, jobs, permissions, runTerminal, subagents, type Chunk, type EvieEvent, type Plugin, type Request } from "../src/index.ts"
import { say, startEvie, stored, type Run } from "./support/harness.ts"

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

type Reply = { text?: string; calls?: { name: string; input: Record<string, unknown> }[]; error?: string } | { hang: true }

// A stand-in for the model that keeps one script for the main agent and one
// for subagents, since the two run at the same time. A request is a
// subagent's when its system prompt says so.
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
          if (reply.error !== undefined) yield { type: "finish", reason: "error", error: reply.error }
          else yield { type: "finish", reason: reply.calls?.length ? "tool_calls" : "stop" }
        },
      })
    },
  }
  return { plugin, requests }
}

const hand = (task: string, extra: Record<string, unknown> = {}) => ({ name: "subagent", input: { description: "look into it", task, ...extra } })

// `trusted` says which project folders the owner has trusted, for a project's own agents.
async function start(parent: Reply[], child: Reply[], plugins: Plugin[] = [], options: Parameters<typeof startEvie>[1] & { trusted?: (folder: string) => boolean } = {}) {
  const model = twoScripts(parent, child)
  // Its own data folder, so that no test reads the agents of whoever runs it.
  const home = options.home ?? (await realpath(await mkdtemp(join(tmpdir(), "evie-home-"))))
  if (!options.home) cleanups.push(() => rm(home, { recursive: true, force: true }))
  const { trusted, ...rest } = options
  const run = await startEvie([model.plugin, subagents({ dataFolder: home, ...(trusted && { trusted }) }), jobs(), ...plugins], { ...rest, home })
  cleanups.push(run.stop)
  // Has the main agent call one tool in a new turn, and returns what the call came back with.
  async function call(name: string, input: Record<string, unknown>) {
    parent.push({ calls: [{ name, input }] }, { text: "ok" })
    await say(run, "do it")
    return results(run).at(-1)!
  }
  return { run, requests: model.requests, parent, call }
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
// Waits until the main session has ended this many turns.
const turns = async (run: Run, count: number) => {
  const ended = () => stored(run.events).filter((event) => event.type === "turn_end" && event.session === run.session).length
  while (ended() < count) await until(run, (event) => event.type === "turn_end")
}
const ofParent = (run: Run) => stored(run.events).filter((event) => event.session === run.session)
const ofChildren = (run: Run) => stored(run.events).filter((event) => event.session !== run.session)
const notes = (run: Run) => ofParent(run).flatMap((event) => (event.type === "note" ? [event.body.text] : []))
const results = (run: Run) => ofParent(run).flatMap((event) => (event.type === "tool_result" ? [event.body.text] : []))
const childId = (run: Run) => ofChildren(run)[0]!.session

// ---- Starting one, and its report -----------------------------------------------------------------

test("a subagent starts with only its task, works in its own conversation, and its report wakes the one that started it", async () => {
  const { run, requests } = await start(
    [{ calls: [hand("Find where sessions are created.")] }, { text: "I have a subagent looking." }, { text: "It is in projects.ts." }],
    [{ text: "Sessions are created in src/core/projects.ts, in createSession." }],
  )

  await say(run, "where are sessions created?")
  // The call returned at once, and the turn went on without the report.
  expect(results(run)[0]).toStartWith("A subagent has started on this, as background job 1.")
  await turns(run, 2)

  // The subagent saw its task and nothing of the conversation that led to it.
  expect(requests.child[0]?.messages).toEqual([{ role: "user", text: "Find where sessions are created." }])
  // Its conversation is its own, kept under its own session, in the same project.
  expect(ofChildren(run).map((event) => event.type)).toEqual(["turn_start", "user_message", "assistant_message", "turn_end"])
  expect(ofParent(run).some((event) => event.type === "assistant_message" && event.body.text.includes("createSession"))).toBe(false)

  const child = childId(run)
  expect(notes(run)).toEqual([
    `Background job 1 (subagent: look into it) has finished.\n\nIts report:\n\nSessions are created in src/core/projects.ts, in createSession.\n\n` +
      `To send this subagent a further message, use subagent_send with the id "${child}".`,
  ])
  expect(requests.parent[2]?.messages.at(-1)?.text).toContain("Its report:")
})

test("a subagent is told what it is, and is not offered the tools to start subagents or to ask the owner", async () => {
  const askStandIn: Plugin = { name: "ask-stand-in", setup: (host) => host.tools.add({ name: "ask", description: "Asks.", inputSchema: { type: "object" }, run: async () => "asked" }) }
  const { run, requests } = await start([{ calls: [hand("Do the thing.")] }, { text: "Started." }, { text: "Done." }], [{ text: "Did it." }], [askStandIn])

  await say(run, "go")
  await turns(run, 2)

  expect(requests.parent[0]?.tools.map((tool) => tool.name)).toEqual(["subagent", "subagent_send", "job_stop", "ask"])
  expect(requests.child[0]?.tools.map((tool) => tool.name)).toEqual(["job_stop"])
  expect(requests.child[0]?.system).toContain("Your last reply is your report")
  expect(requests.parent[0]?.system ?? "").not.toContain("You are a subagent")
})

test("a subagent cannot start a subagent of its own, even through a tool that tries", async () => {
  // A tool that reaches for the core directly, to stand for a plugin that offers subagents another way.
  const reaches: Plugin = {
    name: "reaches",
    setup(host) {
      host.tools.add({
        name: "nest",
        description: "Tries to start a subagent.",
        inputSchema: { type: "object" },
        run: async (_input, context) => context.children.start({ text: "go deeper" }).then(() => "started", (cause: Error) => cause.message),
      })
    },
  }
  const { run } = await start([{ calls: [hand("Try to nest.")] }, { text: "Started." }, { text: "ok" }], [{ calls: [{ name: "nest", input: {} }] }, { text: "Could not." }], [reaches])

  await say(run, "go")
  await turns(run, 2)

  expect(ofChildren(run).find((event) => event.type === "tool_result")?.body).toMatchObject({ text: "A subagent cannot start subagents of its own." })
})

test("a subagent uses the model of the one that started it, or another when one is asked for", async () => {
  const { run, requests } = await start(
    [{ calls: [hand("First."), hand("Second.", { model: "other/model" })] }, { text: "Two started." }, { text: "One in." }, { text: "Both in." }],
    [{ text: "A." }, { text: "B." }],
    [],
    { model: "main/model" },
  )

  await say(run, "go")
  await turns(run, 2)
  while (notes(run).length < 2) await until(run, (event) => event.type === "note")

  expect(requests.child.map((request) => request.model).toSorted()).toEqual(["main/model", "other/model"])
})

test("only so many subagents work at once", async () => {
  const { run } = await start(
    [{ calls: Array.from({ length: 9 }, (_, index) => hand(`Task ${index + 1}.`)) }, { text: "Started what I could." }],
    Array.from({ length: 9 }, () => ({ hang: true as const })),
  )

  await say(run, "go")

  expect(results(run).filter((text) => text.startsWith("A subagent has started"))).toHaveLength(8)
  expect(results(run).at(-1)).toBe("8 subagents are already working, which is the most at once. Wait for one to report before starting another.")
})

// ---- Follow-ups ------------------------------------------------------------------------------------

test("a further message to a subagent that has finished builds on its conversation, and its next report comes back", async () => {
  const { run, requests, parent, call } = await start(
    [{ calls: [hand("Find the session code.")] }, { text: "Started." }, { text: "Got it." }],
    [{ text: "It is in projects.ts." }, { text: "createSession is on line 80." }],
  )
  await say(run, "go")
  await turns(run, 2)
  const child = childId(run)

  // The turn the second report wakes needs a reply too; it comes after the two of the call's own turn.
  const sent = call("subagent_send", { id: child, message: "Which line?" })
  parent.push({ text: "Line 80, then." })
  expect(await sent).toMatch(/^The message was sent, and the subagent is working on it as background job 2\./)
  await turns(run, 4)

  expect(requests.child[1]?.messages).toEqual([
    { role: "user", text: "Find the session code." },
    { role: "assistant", text: "It is in projects.ts." },
    { role: "user", text: "Which line?" },
  ])
  expect(notes(run)[1]).toContain("Background job 2 (subagent: look into it) has finished.\n\nIts report:\n\ncreateSession is on line 80.")
})

test("each round of a subagent's work is a job that names the subagent's session, so that a client can read its conversation", async () => {
  const { run, parent, call } = await start(
    [{ calls: [hand("Find the session code.")] }, { text: "Started." }, { text: "Got it." }],
    [{ text: "It is in projects.ts." }, { text: "createSession is on line 80." }],
  )
  await say(run, "go")
  await turns(run, 2)
  const child = childId(run)
  const sent = call("subagent_send", { id: child, message: "Which line?" })
  parent.push({ text: "Line 80, then." })
  await sent
  await turns(run, 4)

  const told = run.events.flatMap((event) => (event.type === "job" ? [`${event.job.id} ${event.job.status} ${event.job.subagent}`] : []))
  expect(told).toEqual([`1 running ${child}`, `1 finished ${child}`, `2 running ${child}`, `2 finished ${child}`])
  const { jobs: listed } = await run.evie.send({ type: "open_session", session: run.session })
  expect(listed.map((job) => [job.id, job.subagent])).toEqual([
    ["1", child],
    ["2", child],
  ])
})

test("a message to a subagent that is still working joins what it is doing, and one report follows", async () => {
  // A tool the subagent waits in until the test lets it go, so the message arrives while it works.
  let release = () => {}
  let waitingNow = () => {}
  const waiting = new Promise<void>((resolve) => (waitingNow = resolve))
  const slow: Plugin = {
    name: "slow",
    setup(host) {
      host.tools.add({
        name: "wait",
        description: "Waits.",
        inputSchema: { type: "object" },
        run: async () => {
          waitingNow()
          await new Promise<void>((resolve) => (release = resolve))
          return "waited"
        },
      })
    },
  }
  const { run, requests, call } = await start(
    [{ calls: [hand("Look around.")] }, { text: "Started." }],
    [{ calls: [{ name: "wait", input: {} }] }, { text: "Looked around, and at the tests too." }],
    [slow],
  )
  await say(run, "go")
  await waiting
  const child = childId(run)

  expect(await call("subagent_send", { id: child, message: "Look at the tests too." })).toStartWith("The message was sent. The subagent was still working")
  release()
  // One report, for the one round of work.
  while (notes(run).length < 1) await until(run, (event) => event.type === "note")
  expect(requests.child[1]?.messages.at(-1)).toEqual({ role: "user", text: "Look at the tests too." })
  expect(run.events.filter((event) => event.type === "job" && event.job.status === "running")).toHaveLength(1)
})

test("a message can only be sent to a subagent of one's own", async () => {
  const { call } = await start([], [])

  expect(await call("subagent_send", { id: "no-such-subagent", message: "hello" })).toStartWith('The message was not sent: "no-such-subagent" is not a subagent of this session.')
})

// ---- Stopping and failing ------------------------------------------------------------------------------

test("a subagent can be stopped like any job, and one that fails says why", async () => {
  const stopped = await start([{ calls: [hand("Work for a long time.")] }, { text: "Started." }], [{ hang: true }], [])
  await say(stopped.run, "go")
  expect((await stopped.run.evie.send({ type: "run_command", session: stopped.run.session, name: "jobs", args: "stop 1" })).text).toBe("Stopping job 1 (subagent: look into it).")
  while (notes(stopped.run).length < 1) await until(stopped.run, (event) => event.type === "note")
  expect(notes(stopped.run)[0]).toStartWith("Background job 1 (subagent: look into it) was stopped by the owner.")

  const failed = await start([{ calls: [hand("Do the thing.")] }, { text: "Started." }, { text: "It failed." }], [{ error: "The model is not available." }])
  await say(failed.run, "go")
  await turns(failed.run, 2)
  expect(notes(failed.run)[0]).toStartWith("Background job 1 (subagent: look into it) failed.\n\nIt failed: The model is not available.")
})

// ---- Sessions ----------------------------------------------------------------------------------------------

test("evie --continue reopens the owner's session and never a subagent's, and a subagent can be sent a message after a restart", async () => {
  const home = await realpath(await mkdtemp(join(tmpdir(), "evie-home-")))
  const projectFolder = await realpath(await mkdtemp(join(tmpdir(), "evie-project-")))
  cleanups.push(() => rm(home, { recursive: true, force: true }), () => rm(projectFolder, { recursive: true, force: true }))
  const first = await start([{ calls: [hand("Find it.", { model: "other/model" })] }, { text: "Started." }, { text: "Found." }], [{ text: "It is in a.ts." }], [], { home, projectFolder })
  await say(first.run, "go")
  await turns(first.run, 2)
  const child = childId(first.run)
  await first.run.evie.close()

  // The subagent's session has the newest events, and is still not the one continued.
  const second = await start([], [{ text: "On line 3." }], [], { home, projectFolder })
  const continued = await second.run.evie.send({ type: "open_session", folder: projectFolder, continue: true })
  expect(continued).toMatchObject({ session: first.run.session, resumed: true })

  second.parent.push({ calls: [{ name: "subagent_send", input: { id: child, message: "Which line?" } }] }, { text: "Asked." }, { text: "Line 3." })
  await say(second.run, "ask it", continued.session)
  while (!stored(second.run.events).some((event) => event.type === "note")) await until(second.run, (event) => event.type === "note")

  expect(second.requests.child[0]?.messages.map((message) => message.text)).toEqual(["Find it.", "It is in a.ts.", "Which line?"])
  // It carries on with the model it was given, not the main agent's.
  expect(second.requests.child[0]?.model).toBe("other/model")
})

test("a session's subagents are listed from what is stored, newest first, each named by its task's opening words, after a restart too, when its later rounds are named the same way", async () => {
  const home = await realpath(await mkdtemp(join(tmpdir(), "evie-home-")))
  const projectFolder = await realpath(await mkdtemp(join(tmpdir(), "evie-project-")))
  cleanups.push(() => rm(home, { recursive: true, force: true }), () => rm(projectFolder, { recursive: true, force: true }))
  const first = await start(
    [{ calls: [hand("Find where sessions are made.\nLook in src/core first.")] }, { calls: [hand("Read the tests.", { description: "the tests" })] }, { text: "Two started." }, { text: "One." }, { text: "Two." }],
    [{ text: "In projects.ts." }, { text: "They are in test/." }],
    [],
    { home, projectFolder },
  )
  await say(first.run, "go")
  const ended = () => first.run.events.filter((event) => event.type === "job" && event.job.status === "finished").length
  while (ended() < 2) await until(first.run, (event) => event.type === "job")
  const [older, newer] = [...new Set(ofChildren(first.run).map((event) => event.session))] as [string, string]
  const listed = await first.run.evie.send({ type: "list_subagents", session: first.run.session })
  expect(listed.map(({ session, name }) => ({ session, name }))).toEqual([
    { session: newer, name: "Read the tests." },
    { session: older, name: "Find where sessions are made." },
  ])
  expect(listed[0]!.started).toBeGreaterThanOrEqual(listed[1]!.started)
  expect(await first.run.evie.send({ type: "list_subagents", session: older })).toEqual([])
  await first.run.evie.close()

  const second = await start([], [{ text: "On line 80." }], [], { home, projectFolder })
  const continued = await second.run.evie.send({ type: "open_session", folder: projectFolder, continue: true })
  expect(await second.run.evie.send({ type: "list_subagents", session: continued.session })).toEqual(listed)
  second.parent.push({ calls: [{ name: "subagent_send", input: { id: older, message: "Which line?" } }] }, { text: "Asked." }, { text: "Line 80." })
  await say(second.run, "ask it", continued.session)
  while (!second.run.events.some((event) => event.type === "job" && event.job.status !== "running")) await until(second.run, (event) => event.type === "job")
  const round = second.run.events.find((event) => event.type === "job")
  expect(round?.type === "job" && round.job.title).toBe("subagent: Find where sessions are made.")
  await expect(second.run.evie.send({ type: "list_subagents", session: "no-such-session" })).rejects.toThrow('There is no session with the id "no-such-session".')
})

// ---- Permissions ---------------------------------------------------------------------------------------------

// A stand-in for the edit tool, which only records that it ran.
function editStandIn() {
  const ran: string[] = []
  const plugin: Plugin = {
    name: "edit-stand-in",
    setup: (host) => host.tools.add({ name: "edit", description: "Edits.", inputSchema: { type: "object" }, run: async (input) => (ran.push(String(input.path)), "edited") }),
  }
  return { plugin, ran }
}

test("in manual mode starting a subagent is asked about, and that answer covers its work inside the project", async () => {
  const edits = editStandIn()
  const { run } = await start(
    [{ calls: [hand("Tidy the notes.")] }, { text: "Started." }, { text: "Tidied." }],
    [{ calls: [{ name: "edit", input: { path: "notes.md" } }] }, { text: "Tidied notes.md." }],
    [edits.plugin, permissions()],
    { questions: true },
  )
  const asked: string[] = []
  run.evie.subscribe((event) => {
    if (event.type !== "question") return
    asked.push(event.body.text)
    void run.evie.send({ type: "answer_question", session: event.session, id: event.body.id, choice: "Yes, once" })
  })

  await say(run, "go")
  await turns(run, 2)

  expect(asked).toEqual([
    "Let evie start a subagent (look into it)? It works without asking before each step: it may change files and run commands inside the project.\n\n  Tidy the notes.",
  ])
  expect(edits.ran).toEqual(["notes.md"])
})

test("what a subagent still has to ask is put to the owner in the session that started it, and a read-only session has read-only subagents", async () => {
  const edits = editStandIn()
  const auto = await start(
    [{ calls: [hand("Change a file elsewhere.")] }, { text: "Started." }, { text: "Done." }],
    [{ calls: [{ name: "edit", input: { path: "/somewhere/else/notes.md" } }] }, { text: "Was not allowed." }],
    [edits.plugin, permissions({ mode: "auto" })],
    { questions: true },
  )
  const asked: { session: string; text: string }[] = []
  auto.run.evie.subscribe((event) => {
    if (event.type !== "question") return
    asked.push({ session: event.session, text: event.body.text })
    void auto.run.evie.send({ type: "answer_question", session: event.session, id: event.body.id, choice: "No" })
  })
  await say(auto.run, "go")
  await turns(auto.run, 2)
  expect(asked).toEqual([{ session: auto.run.session, text: "A subagent asks: Let evie change /somewhere/else/notes.md, which is outside the project?" }])
  expect(edits.ran).toEqual([])

  const readOnly = await start(
    [{ calls: [hand("Tidy the notes.")] }, { text: "Started." }, { text: "It could not." }],
    [{ calls: [{ name: "edit", input: { path: "notes.md" } }] }, { text: "I am not allowed to change files." }],
    [edits.plugin, permissions({ mode: "read-only" })],
  )
  await say(readOnly.run, "go")
  await turns(readOnly.run, 2)
  expect(ofChildren(readOnly.run).find((event) => event.type === "tool_result")?.body).toMatchObject({ text: expect.stringContaining("read-only mode") })
  expect(edits.ran).toEqual([])
})

// ---- Found in review -----------------------------------------------------------------------------------------

test("a subagent's mode is settled when it is given work: more work in manual mode is asked about, and read-only does not wear off", async () => {
  const edits = editStandIn()
  const { run, parent, call } = await start(
    [{ calls: [hand("Tidy the notes.")] }, { text: "Started." }, { text: "It could not." }],
    [{ calls: [{ name: "edit", input: { path: "notes.md" } }] }, { text: "Not allowed." }, { calls: [{ name: "edit", input: { path: "notes.md" } }] }, { text: "Still not allowed." }, { calls: [{ name: "edit", input: { path: "notes.md" } }] }, { text: "Tidied." }],
    [edits.plugin, permissions({ mode: "read-only" })],
    { questions: true },
  )
  const asked: string[] = []
  let answer = "Yes, once"
  run.evie.subscribe((event) => {
    if (event.type !== "question") return
    asked.push(event.body.text.split("\n")[0]!)
    void run.evie.send({ type: "answer_question", session: event.session, id: event.body.id, choice: answer })
  })
  const mode = (name: string) => run.evie.send({ type: "run_command", session: run.session, name: "mode", args: name })
  const send = async (message: string) => {
    const sent = call("subagent_send", { id: childId(run), message })
    parent.push({ text: "Noted." })
    return sent
  }

  // Started from a read-only session: nothing is asked, and it can change nothing.
  await say(run, "go")
  await turns(run, 2)
  expect(edits.ran).toEqual([])

  // The owner moves to manual. Giving the subagent more work is refused: it stays read-only.
  await mode("manual")
  answer = "No"
  expect(await call("subagent_send", { id: childId(run), message: "Try again." })).toStartWith("The owner said no to this.")
  expect(edits.ran).toEqual([])

  // Agreed to, it works without asking before each step.
  answer = "Yes, once"
  expect(await send("Try again.")).toStartWith("The message was sent")
  await turns(run, 5)
  expect(asked).toEqual([
    "Let evie give its subagent more work? It works without asking before each step: it may change files and run commands inside the project.",
    "Let evie give its subagent more work? It works without asking before each step: it may change files and run commands inside the project.",
  ])
  expect(edits.ran).toEqual(["notes.md"])

  // Back in read-only, the subagent is read-only at once, whatever it was given before.
  await mode("read-only")
  await send("And once more.")
  await turns(run, 7)
  expect(edits.ran).toEqual(["notes.md"])
})

test("a subagent stopped and then sent a message reports on the message", async () => {
  const { run, parent } = await start([{ calls: [hand("Work for a long time.")] }, { text: "Started." }], [{ hang: true }, { text: "Here is the shorter answer." }])
  await say(run, "go")
  const child = childId(run)

  parent.push({ calls: [{ name: "job_stop", input: { id: "1" } }, { name: "subagent_send", input: { id: child, message: "Give me the short version." } }] }, { text: "Asked for less." }, { text: "Got it." })
  await say(run, "that is taking too long")
  await turns(run, 3)

  expect(results(run).at(-1)).toMatch(/^The message was sent, and the subagent is working on it as background job 2\./)
  expect(notes(run)).toHaveLength(1)
  expect(notes(run)[0]).toContain("Background job 2 (subagent: look into it) has finished.\n\nIts report:\n\nHere is the shorter answer.")
})

test("a round in which a subagent says nothing does not report what it said in an earlier round", async () => {
  const { run, parent, call } = await start([{ calls: [hand("Find it.")] }, { text: "Started." }, { text: "Found." }], [{ text: "It is in a.ts." }, { text: "" }])
  await say(run, "go")
  await turns(run, 2)

  const sent = call("subagent_send", { id: childId(run), message: "Anything else?" })
  parent.push({ text: "Nothing more." })
  await sent
  await turns(run, 4)

  expect(notes(run)[1]).toContain("Its report:\n\n(It ended without saying anything.)")
})

test("when Evie closes with a subagent still working, the session that started it records that it was stopped", async () => {
  const { run } = await start([{ calls: [hand("Work for a long time.")] }, { text: "Started." }], [{ hang: true }])
  await say(run, "go")

  await run.evie.close()

  expect(notes(run)).toEqual(["Background job 1 (subagent: look into it) was stopped because Evie was closed. Start it again if it is still needed."])
})

test("the terminal takes two questions that are open at once one after the other", async () => {
  // A tool that asks two things at the same time, as two subagents might.
  const answers: (string | null)[] = []
  const asksTwice: Plugin = {
    name: "asks-twice",
    setup(host) {
      host.tools.add({
        name: "decide",
        description: "Asks two questions.",
        inputSchema: { type: "object" },
        run: async (_input, context) => {
          answers.push(...(await Promise.all([context.ask({ text: "First?", choices: ["A", "B"] }), context.ask({ text: "Second?", choices: ["C", "D"] })])))
          return "decided"
        },
      })
    },
  }
  const model = twoScripts([{ calls: [{ name: "decide", input: {} }] }, { text: "Decided." }], [])
  const run = await startEvie([model.plugin, asksTwice])
  cleanups.push(run.stop)
  let printed = ""
  await runTerminal({
    evie: run.evie,
    folder: run.projectFolder,
    lines: (async function* () {
      yield "decide"
      while (!printed.includes("First?")) await Bun.sleep(1)
      yield "2"
      while (!printed.includes("Second?")) await Bun.sleep(1)
      yield "1"
    })(),
    write: (text) => (printed += text),
  })

  expect(answers).toEqual(["B", "C"])
  expect(printed.indexOf("[question] Second?")).toBeGreaterThan(printed.indexOf("[answered] B"))
})

// ---- Agents the owner has defined ---------------------------------------------------------------------------

const REVIEWER = `---
name: reviewer
description: Reviews a change for bugs. Give it the diff.
mode: read-only
model: review/model
---
You review code changes. Report bugs, most serious first.
`

async function writeAgent(folder: string, file: string, text: string) {
  await mkdir(folder, { recursive: true })
  await writeFile(join(folder, file), text)
}

test("the main agent is told which agents the owner has defined, and can start a subagent as one", async () => {
  const { run, requests } = await start([{ calls: [hand("Review the diff.", { agent: "reviewer" })] }, { text: "A reviewer is on it." }, { text: "It found nothing." }], [{ text: "No bugs." }])
  await writeAgent(join(run.home, "agents"), "reviewer.md", REVIEWER)

  await say(run, "review my change")
  await turns(run, 2)

  expect(requests.parent[0]?.system).toContain('Agents the owner has defined. Give one\'s name as "agent" to the subagent tool to start a subagent as it:\n- reviewer: Reviews a change for bugs. Give it the diff. (reads only)')
  // The subagent has the agent's instructions and its model, and is listed by its name.
  expect(requests.child[0]?.system).toEndWith('You are working as the agent "reviewer". Its instructions:\n\nYou review code changes. Report bugs, most serious first.')
  expect(requests.child[0]?.model).toBe("review/model")
  expect(notes(run)[0]).toStartWith("Background job 1 (subagent: reviewer: look into it) has finished.")
  // A subagent is not told the list: it starts none.
  expect(requests.child[0]?.system).not.toContain("Agents the owner has defined")
})

test("an agent that only reads cannot change anything, whatever mode the session is in", async () => {
  const edits = editStandIn()
  const { run } = await start(
    [{ calls: [hand("Review and fix.", { agent: "reviewer" })] }, { text: "Started." }, { text: "It could not fix it." }],
    [{ calls: [{ name: "edit", input: { path: "notes.md" } }] }, { text: "I may only read." }],
    [edits.plugin, permissions({ mode: "auto" })],
  )
  await writeAgent(join(run.home, "agents"), "reviewer.md", REVIEWER)

  await say(run, "go")
  await turns(run, 2)

  expect(ofChildren(run).find((event) => event.type === "tool_result")?.body).toMatchObject({ text: expect.stringContaining("read-only mode") })
  expect(edits.ran).toEqual([])
})

test("naming an agent that does not exist says which ones do, and a model the owner asks for wins over the agent's", async () => {
  const { run, requests } = await start(
    [{ calls: [hand("Plan it.", { agent: "planner" })] }, { text: "No such agent." }, { calls: [hand("Review it.", { agent: "Reviewer", model: "asked/for" })] }, { text: "Started." }, { text: "Done." }],
    [{ text: "Fine." }],
  )
  await writeAgent(join(run.home, "agents"), "reviewer.md", REVIEWER)

  await say(run, "plan")
  expect(results(run)[0]).toBe('There is no agent named "planner". The agents are: reviewer. Leave "agent" out for a general subagent.')

  await say(run, "review")
  await turns(run, 3)
  expect(requests.child[0]?.model).toBe("asked/for")
})

test("a project's own agents are used only when the owner has trusted the project, and then win over the owner's of the same name", async () => {
  const projectAgent = "---\ndescription: The project's own reviewer.\n---\nReview the way this project likes.\n"
  const untrusted = await start([{ text: "ok" }], [])
  await writeAgent(join(untrusted.run.home, "agents"), "reviewer.md", REVIEWER)
  await writeAgent(join(untrusted.run.projectFolder, ".evie", "agents"), "reviewer.md", projectAgent)
  await writeAgent(join(untrusted.run.projectFolder, ".evie", "agents"), "planner.md", "Plan things.\n")
  await say(untrusted.run, "hi")
  expect(untrusted.requests.parent[0]?.system).toContain("- reviewer: Reviews a change for bugs.")
  expect(untrusted.requests.parent[0]?.system).not.toContain("planner")

  const trusted = await start([{ text: "ok" }], [], [], { trusted: () => true })
  await writeAgent(join(trusted.run.home, "agents"), "reviewer.md", REVIEWER)
  await writeAgent(join(trusted.run.projectFolder, ".evie", "agents"), "reviewer.md", projectAgent)
  await writeAgent(join(trusted.run.projectFolder, ".evie", "agents"), "planner.md", "Plan things.\n")
  await say(trusted.run, "hi")
  expect(trusted.requests.parent[0]?.system).toContain("- planner: (no description)\n- reviewer: The project's own reviewer.")
})

test("an agent written during a session can be used at once", async () => {
  const { run, parent, requests } = await start([{ text: "No agents yet." }], [{ text: "Planned." }])
  await say(run, "hi")
  expect(requests.parent[0]?.system ?? "").not.toContain("Agents the owner has defined")

  await writeAgent(join(run.home, "agents"), "planner.md", "---\ndescription: Plans work.\n---\nWrite a plan.\n")
  parent.push({ calls: [hand("Plan the release.", { agent: "planner" })] }, { text: "Started." }, { text: "Here is the plan." })
  await say(run, "plan it")
  await turns(run, 3)

  expect(requests.child[0]?.system).toContain("Write a plan.")
})

test("Evie may write the owner's agents with the file tools, and is asked every time, with no always", async () => {
  const home = await realpath(await mkdtemp(join(tmpdir(), "evie-home-")))
  cleanups.push(() => rm(home, { recursive: true, force: true }))
  const agentFile = join(home, "agents", "tester.md")
  const write = { name: "write", input: { path: agentFile, content: "---\ndescription: Runs tests.\n---\nRun the tests.\n" } }
  const { run, call } = await start([], [], [files(), permissions({ mode: "auto", dataFolder: home })], { home, questions: true })
  const asked: { text: string; choices: string[] }[] = []
  run.evie.subscribe((event) => {
    if (event.type !== "question") return
    asked.push({ text: event.body.text, choices: event.body.choices })
    void run.evie.send({ type: "answer_question", session: event.session, id: event.body.id, choice: "Yes, once" })
  })

  expect(await call("write", write.input)).toStartWith("Created")
  expect(await call("write", write.input)).not.toStartWith("Blocked")
  // Asked both times, even in auto mode, and never offered "always".
  expect(asked).toHaveLength(2)
  expect(asked[0]?.choices).toEqual(["Yes, once", "No"])
  expect(asked[0]?.text).toContain("It applies in every project from now on, so it is asked about each time.")

  // Reading it back is free, and the rest of the data folder is still closed.
  expect(await call("read", { path: agentFile })).toContain("Run the tests.")
  expect(await call("write", { path: join(home, "evie.json"), content: "{}" })).toStartWith("Blocked by the guard:")
  expect(await call("read", { path: join(home, "agents", "..", "evie.db") })).toStartWith("Blocked by the guard:")
  expect(asked).toHaveLength(2)
})

// ---- Found in review of agents ---------------------------------------------------------------------------------

test("a slip in an agent's mode holds it to reading, and never quietly lets it write", async () => {
  const slips = ["Read-Only", "readonly", "read_only", "read-only # reviewers only read", "manual"]
  const { run, requests } = await start([{ text: "ok" }], [])
  for (const [index, mode] of slips.entries()) await writeAgent(join(run.home, "agents"), `agent${index}.md`, `---\ndescription: An agent.\nmode: ${mode}\n---\nDo things.\n`)
  // A file an editor saved with a mark at the start, and a blank line before its header.
  await writeAgent(join(run.home, "agents"), "marked.md", "\uFEFF\n---\ndescription: Saved by an editor.\nmode: read-only\n---\nDo things.\n")
  await writeAgent(join(run.home, "agents"), "free.md", "---\ndescription: May write.\nmode: auto\n---\nDo things.\n")

  await say(run, "hi")

  const list = requests.parent[0]!.system!
  for (const index of slips.keys()) expect(list).toContain(`- agent${index}: An agent. (reads only)`)
  expect(list).toContain("- marked: Saved by an editor. (reads only)")
  expect(list).toContain("- free: May write.\n")
})

test("a trusted project's agent cannot lift the limit on the owner's agent of the same name", async () => {
  const { run, requests } = await start([{ text: "ok" }], [], [], { trusted: () => true })
  await writeAgent(join(run.home, "agents"), "reviewer.md", REVIEWER)
  await writeAgent(join(run.projectFolder, ".evie", "agents"), "reviewer.md", "---\ndescription: The project's reviewer.\n---\nReview and fix.\n")

  await say(run, "hi")

  expect(requests.parent[0]?.system).toContain("- reviewer: The project's reviewer. (reads only)")
})

test("by itself the guard lets the owner's agents be read and not written", async () => {
  const home = await realpath(await mkdtemp(join(tmpdir(), "evie-home-")))
  cleanups.push(() => rm(home, { recursive: true, force: true }))
  await writeAgent(join(home, "agents"), "reviewer.md", REVIEWER)
  const { guard } = await import("../src/index.ts")
  const { call } = await start([], [], [files(), guard({ dataFolder: home })], { home })

  expect(await call("read", { path: join(home, "agents", "reviewer.md") })).toContain("You review code changes.")
  expect(await call("write", { path: join(home, "agents", "new.md"), content: "x" })).toStartWith("Blocked by the guard:")
})

test("an agent file that is a link to somewhere else is not used, so it cannot hand the model another file", async () => {
  const { run, requests } = await start([{ text: "ok" }], [])
  await writeFile(join(run.home, ".env"), "OPENROUTER_API_KEY=secret-value\n")
  await writeAgent(join(run.home, "agents"), "reviewer.md", REVIEWER)
  await symlink(join(run.home, ".env"), join(run.home, "agents", "leak.md"))

  await say(run, "hi")

  const system = requests.parent[0]!.system!
  expect(system).toContain("- reviewer:")
  expect(system).not.toContain("leak")
  expect(system).not.toContain("secret-value")
})
